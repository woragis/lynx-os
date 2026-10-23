package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

func TestIdentityRepositoryRevokesSessionsInPostgres(t *testing.T) {
	databaseURL := os.Getenv("LYNX_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LYNX_TEST_DATABASE_URL to run the PostgreSQL authorization integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to test PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("lynx_auth_test_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminPool.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		_, _ = adminPool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		adminPool.Close()
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	testPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		_, _ = adminPool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		adminPool.Close()
		t.Fatalf("connect to isolated schema: %v", err)
	}
	t.Cleanup(func() {
		testPool.Close()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = adminPool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE")
		adminPool.Close()
	})

	setup := []string{
		`CREATE TABLE admin_users (
			id TEXT PRIMARY KEY, email TEXT NOT NULL, password_hash TEXT NOT NULL,
			display_name TEXT NOT NULL, role TEXT NOT NULL, active BOOLEAN NOT NULL,
			created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL, last_login_at TIMESTAMPTZ
		)`,
		`CREATE TABLE admin_sessions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL REFERENCES admin_users(id), family_id TEXT NOT NULL,
			refresh_token_hash CHAR(64) NOT NULL UNIQUE, expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL, last_used_at TIMESTAMPTZ NOT NULL, ip_address TEXT, user_agent TEXT
		)`,
		`CREATE TABLE admin_invitations (
			id TEXT PRIMARY KEY, email TEXT NOT NULL, display_name TEXT NOT NULL, role TEXT NOT NULL,
			token_hash CHAR(64) NOT NULL UNIQUE, invited_by TEXT NOT NULL REFERENCES admin_users(id),
			expires_at TIMESTAMPTZ NOT NULL, accepted_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL
		)`,
	}
	for _, statement := range setup {
		if _, err := testPool.Exec(ctx, statement); err != nil {
			t.Fatalf("create test schema table: %v", err)
		}
	}
	_, err = testPool.Exec(ctx, `
		INSERT INTO admin_users (id,email,password_hash,display_name,role,active,created_at,updated_at)
		VALUES ('owner','owner@lynx.test','hash','Owner','owner',true,now(),now()),
		       ('operator','operator@lynx.test','hash','Operator','operator',true,now(),now())`)
	if err != nil {
		t.Fatalf("insert test administrators: %v", err)
	}
	for _, session := range []struct{ id, userID string }{
		{"owner-session-1", "owner"}, {"owner-session-2", "owner"},
		{"operator-session-1", "operator"}, {"operator-session-2", "operator"},
	} {
		_, err := testPool.Exec(ctx, `INSERT INTO admin_sessions
			(id,user_id,family_id,refresh_token_hash,expires_at,created_at,last_used_at,ip_address,user_agent)
			VALUES ($1,$2,$1,$3,now()+interval '1 day',now(),now(),'127.0.0.1','test')`,
			session.id, session.userID, strings.Repeat("a", 64-len(session.id))+session.id)
		if err != nil {
			t.Fatalf("insert test session %s: %v", session.id, err)
		}
	}

	repository := NewIdentityRepository(testPool)
	for index, role := range []domain.Role{domain.RoleAdmin, domain.RoleOperator, domain.RoleViewer} {
		tokenHash := strings.Repeat(fmt.Sprint(index+1), 64)
		invitation := domain.Invitation{
			ID: fmt.Sprintf("invitation-%d", index), Email: fmt.Sprintf("%s-%d@lynx.test", role, index),
			DisplayName: string(role), Role: role, TokenHash: tokenHash, InvitedBy: "owner",
			ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(),
		}
		if err := repository.CreateInvitation(ctx, invitation); err != nil {
			t.Fatalf("create %s invitation: %v", role, err)
		}
		user, err := repository.AcceptInvitation(ctx, tokenHash, "bcrypt-hash")
		if err != nil {
			t.Fatalf("accept %s invitation: %v", role, err)
		}
		if user.Role != role || !user.Active || user.Email != invitation.Email {
			t.Fatalf("invitation created the wrong account: %#v", user)
		}
		if _, err := repository.AcceptInvitation(ctx, tokenHash, "another-hash"); !errors.Is(err, domain.ErrInvalidOneTimeToken) {
			t.Fatalf("accepted invitation token must be single-use, got %v", err)
		}
	}

	updated, err := repository.UpdateAdmin(ctx, "operator", "Viewer", domain.RoleViewer, true)
	if err != nil {
		t.Fatalf("change operator role: %v", err)
	}
	if updated.Role != domain.RoleViewer || !updated.Active {
		t.Fatalf("updated role was not persisted: %#v", updated)
	}
	_, changedUser, err := repository.GetActiveSession(ctx, "operator-session-1")
	if err != nil {
		t.Fatalf("existing session should reflect the updated role immediately: %v", err)
	}
	if changedUser.Role != domain.RoleViewer || changedUser.Role.Allows(domain.PermissionManageIntegrations) {
		t.Fatalf("existing session retained elevated permissions after role change: %#v", changedUser)
	}

	if _, err := repository.UpdateAdmin(ctx, "operator", "Operator", domain.RoleOperator, false); err != nil {
		t.Fatalf("deactivate operator: %v", err)
	}
	for _, sessionID := range []string{"operator-session-1", "operator-session-2"} {
		if _, _, err := repository.GetActiveSession(ctx, sessionID); !errors.Is(err, domain.ErrSessionRevoked) {
			t.Fatalf("deactivated user's session %s was not revoked: %v", sessionID, err)
		}
	}
	if _, _, err := repository.GetActiveSession(ctx, "owner-session-1"); err != nil {
		t.Fatalf("deactivating another user must not revoke the owner's session: %v", err)
	}
	if _, err := repository.UpdateAdmin(ctx, "owner", "Owner", domain.RoleAdmin, false); !errors.Is(err, domain.ErrCannotModifyOwner) {
		t.Fatalf("owner modification should be rejected, got %v", err)
	}
	if err := repository.RevokeSession(ctx, "owner", "owner-session-1"); err != nil {
		t.Fatalf("revoke one session: %v", err)
	}
	if _, _, err := repository.GetActiveSession(ctx, "owner-session-1"); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("individually revoked session remained active: %v", err)
	}
	if _, _, err := repository.GetActiveSession(ctx, "owner-session-2"); err != nil {
		t.Fatalf("individual revocation should leave other sessions active: %v", err)
	}
	if err := repository.RevokeAllSessions(ctx, "owner"); err != nil {
		t.Fatalf("revoke all owner sessions: %v", err)
	}
	if _, _, err := repository.GetActiveSession(ctx, "owner-session-2"); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("revoke-all left a session active: %v", err)
	}
}

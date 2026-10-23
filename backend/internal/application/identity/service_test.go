package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginCreatesSessionAndOnlyStoresRefreshHash(t *testing.T) {
	repository := newFakeRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository.usersByEmail["owner@lynx.test"] = domain.User{
		ID: "user-1", Email: "owner@lynx.test", PasswordHash: string(hash), DisplayName: "Owner", Role: domain.RoleOwner, Active: true,
	}
	service := newTestService(t, repository, &fakeLimiter{})

	result, err := service.Login(context.Background(), " OWNER@LYNX.TEST ", "a-long-test-password", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" || result.User.Role != domain.RoleOwner {
		t.Fatalf("Login returned incomplete session: %#v", result)
	}
	stored := repository.sessions[result.Session.ID]
	if stored.RefreshTokenHash == result.RefreshToken || stored.RefreshTokenHash == "" {
		t.Fatal("repository must store the refresh token hash, not the raw token")
	}
}

func TestLoginDoesNotRevealWhetherAccountExists(t *testing.T) {
	repository := newFakeRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository.usersByEmail["owner@lynx.test"] = domain.User{ID: "user-1", Email: "owner@lynx.test", PasswordHash: string(hash), Active: true}
	service := newTestService(t, repository, &fakeLimiter{})

	_, unknownErr := service.Login(context.Background(), "missing@lynx.test", "invalid-password", "127.0.0.1", "")
	_, wrongPasswordErr := service.Login(context.Background(), "owner@lynx.test", "invalid-password", "127.0.0.1", "")
	if !errors.Is(unknownErr, domain.ErrInvalidCredentials) || !errors.Is(wrongPasswordErr, domain.ErrInvalidCredentials) {
		t.Fatalf("credential failures differ: unknown=%v wrong-password=%v", unknownErr, wrongPasswordErr)
	}
}

func TestRefreshRotationRevokesPreviousAccessSession(t *testing.T) {
	repository := newFakeRepository()
	hash, _ := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	repository.usersByEmail["owner@lynx.test"] = domain.User{ID: "user-1", Email: "owner@lynx.test", PasswordHash: string(hash), Active: true}
	service := newTestService(t, repository, &fakeLimiter{})

	first, err := service.Login(context.Background(), "owner@lynx.test", "a-long-test-password", "127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Refresh(context.Background(), first.RefreshToken, "127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Authenticate(context.Background(), first.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("previous access token should fail after refresh rotation, got %v", err)
	}
	if _, _, err := service.Authenticate(context.Background(), second.AccessToken); err != nil {
		t.Fatalf("new access token should be valid: %v", err)
	}
	if _, err := service.Refresh(context.Background(), first.RefreshToken, "127.0.0.1", ""); !errors.Is(err, domain.ErrRefreshReuse) {
		t.Fatalf("reused refresh token should be detected, got %v", err)
	}
	if _, _, err := service.Authenticate(context.Background(), second.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("refresh reuse should revoke the token family, got %v", err)
	}
}

func TestPasswordResetUsesHiddenSingleUseTokenAndDoesNotEnumerateAccounts(t *testing.T) {
	repository := newFakeRepository()
	mailer := &fakeResetMailer{}
	service := newTestServiceWithMailer(t, repository, &fakeLimiter{}, mailer)

	if err := service.RequestPasswordReset(context.Background(), "missing@lynx.test", "127.0.0.1"); err != nil {
		t.Fatalf("unknown account should get the same generic success: %v", err)
	}
	if mailer.calls != 0 {
		t.Fatal("unknown account must not receive a reset email")
	}
	repository.usersByEmail["owner@lynx.test"] = domain.User{ID: "user-1", Email: "owner@lynx.test", Active: true}
	if err := service.RequestPasswordReset(context.Background(), "OWNER@LYNX.TEST", "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if mailer.calls != 1 || len(mailer.token) < 40 || repository.resetTokenHash == mailer.token || repository.resetTokenHash == "" {
		t.Fatal("reset must email a random token while persisting only its hash")
	}
	if err := service.ResetPassword(context.Background(), mailer.token, "a-new-secure-password"); err != nil {
		t.Fatal(err)
	}
	if repository.resetCompletedHash == "" || repository.resetCompletedHash == mailer.token {
		t.Fatal("password reset must pass only the token hash to persistence")
	}
}

func TestInvitationAndTelegramLinkTokensAreShortLivedAndStoredHashed(t *testing.T) {
	repository := newFakeRepository()
	service := newTestService(t, repository, &fakeLimiter{})

	if _, err := service.Invite(context.Background(), "owner-1", "new@lynx.test", "Nova Pessoa", domain.RoleOwner); !errors.Is(err, domain.ErrInvalidRole) {
		t.Fatalf("owner role must not be assignable by invitation: %v", err)
	}
	token, err := service.Invite(context.Background(), "owner-1", "new@lynx.test", "Nova Pessoa", domain.RoleOperator)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || repository.invitation.TokenHash == token || repository.invitation.Role != domain.RoleOperator {
		t.Fatal("invitation should persist only a hash and restrict the role")
	}
	code, err := service.CreateTelegramLinkCode(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 8 || repository.telegramCodeHash == code {
		t.Fatal("Telegram link code must be short-lived and persisted as a hash")
	}
	if err := service.LinkTelegramIdentity(context.Background(), "876916572", code); err != nil {
		t.Fatalf("valid Telegram code should be consumed: %v", err)
	}
	if repository.telegramLinkedUserID != "876916572" {
		t.Fatal("Telegram identity should be linked to the numeric Telegram user ID")
	}
}

func TestOwnerCannotDeactivateTheirOwnAccountThroughAdminUpdate(t *testing.T) {
	repository := newFakeRepository()
	service := newTestService(t, repository, &fakeLimiter{})
	_, err := service.UpdateAdmin(context.Background(), "owner-1", "owner-1", "Owner", domain.RoleAdmin, false)
	if !errors.Is(err, domain.ErrCannotDeactivateSelf) {
		t.Fatalf("expected self-deactivation to be rejected, got %v", err)
	}
}

func TestDeactivatingAdministratorRevokesAllAccessSessions(t *testing.T) {
	repository := newFakeRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository.usersByEmail["operator@lynx.test"] = domain.User{
		ID: "operator-1", Email: "operator@lynx.test", PasswordHash: string(hash),
		DisplayName: "Operator", Role: domain.RoleOperator, Active: true,
	}
	service := newTestService(t, repository, &fakeLimiter{})
	first, err := service.Login(context.Background(), "operator@lynx.test", "a-long-test-password", "127.0.0.1", "browser one")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(context.Background(), "operator@lynx.test", "a-long-test-password", "127.0.0.1", "browser two")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := service.UpdateAdmin(context.Background(), "owner-1", "operator-1", "Operator", domain.RoleOperator, false); err != nil {
		t.Fatalf("deactivate administrator: %v", err)
	}
	for _, accessToken := range []string{first.AccessToken, second.AccessToken} {
		if _, _, err := service.Authenticate(context.Background(), accessToken); !errors.Is(err, domain.ErrSessionRevoked) {
			t.Fatalf("session remained usable after account deactivation: %v", err)
		}
	}
}

func newTestService(t *testing.T, repository *fakeRepository, limiter *fakeLimiter) *Service {
	return newTestServiceWithMailer(t, repository, limiter, nil)
}

func newTestServiceWithMailer(t *testing.T, repository *fakeRepository, limiter *fakeLimiter, mailer domain.PasswordResetMailer) *Service {
	t.Helper()
	service, err := NewService(repository, limiter, fakeTokens{repository: repository}, mailer)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	return service
}

type fakeResetMailer struct {
	calls int
	token string
}

func (m *fakeResetMailer) SendPasswordReset(_ context.Context, _ string, token string, _ time.Time) error {
	m.calls++
	m.token = token
	return nil
}

type fakeLimiter struct{ blocked bool }

func (l *fakeLimiter) Allow(context.Context, string) (bool, error) { return !l.blocked, nil }
func (l *fakeLimiter) RecordFailure(context.Context, string) error { return nil }
func (l *fakeLimiter) Reset(context.Context, string) error         { return nil }

type fakeTokens struct{ repository *fakeRepository }

func (fakeTokens) Sign(claims domain.AccessClaims, _ time.Time) (string, error) {
	return claims.UserID + ":" + claims.SessionID, nil
}
func (t fakeTokens) Verify(value string) (domain.AccessClaims, error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 2 {
		session, ok := t.repository.sessions[parts[1]]
		if ok && session.UserID == parts[0] && session.RevokedAt == nil {
			return domain.AccessClaims{UserID: parts[0], SessionID: parts[1]}, nil
		}
	}
	return domain.AccessClaims{}, domain.ErrSessionRevoked
}

type fakeRepository struct {
	usersByEmail         map[string]domain.User
	sessions             map[string]domain.Session
	byHash               map[string]string
	resetTokenHash       string
	resetCompletedHash   string
	invitation           domain.Invitation
	telegramCodeHash     string
	telegramLinkedUserID string
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{usersByEmail: map[string]domain.User{}, sessions: map[string]domain.Session{}, byHash: map[string]string{}}
}
func (r *fakeRepository) CreateInitialOwner(context.Context, domain.User) error { return nil }
func (r *fakeRepository) FindByEmail(_ context.Context, email string) (domain.User, error) {
	user, ok := r.usersByEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}
func (r *fakeRepository) FindByID(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}
func (r *fakeRepository) TouchLastLogin(context.Context, string) error { return nil }
func (r *fakeRepository) CreateSession(_ context.Context, session domain.Session) error {
	r.sessions[session.ID] = session
	r.byHash[session.RefreshTokenHash] = session.ID
	return nil
}
func (r *fakeRepository) GetActiveSession(_ context.Context, id string) (domain.Session, domain.User, error) {
	session, ok := r.sessions[id]
	if !ok || session.RevokedAt != nil {
		return domain.Session{}, domain.User{}, domain.ErrSessionRevoked
	}
	for _, user := range r.usersByEmail {
		if user.ID == session.UserID && user.Active {
			return session, user, nil
		}
	}
	return domain.Session{}, domain.User{}, domain.ErrSessionRevoked
}
func (r *fakeRepository) RotateSession(_ context.Context, oldHash string, next domain.Session) error {
	id, ok := r.byHash[oldHash]
	if !ok {
		return domain.ErrInvalidRefresh
	}
	old := r.sessions[id]
	if old.RevokedAt != nil {
		for sessionID, session := range r.sessions {
			if session.FamilyID == old.FamilyID {
				now := time.Now()
				session.RevokedAt = &now
				r.sessions[sessionID] = session
			}
		}
		return domain.ErrRefreshReuse
	}
	now := time.Now()
	old.RevokedAt = &now
	r.sessions[id] = old
	next.UserID = old.UserID
	next.FamilyID = old.FamilyID
	r.sessions[next.ID] = next
	r.byHash[next.RefreshTokenHash] = next.ID
	return nil
}
func (r *fakeRepository) RevokeSession(_ context.Context, userID, id string) error {
	session := r.sessions[id]
	if session.UserID == userID {
		now := time.Now()
		session.RevokedAt = &now
		r.sessions[id] = session
	}
	return nil
}
func (r *fakeRepository) RevokeSessionByRefreshHash(_ context.Context, hash string) error {
	id := r.byHash[hash]
	return r.RevokeSession(context.Background(), r.sessions[id].UserID, id)
}
func (r *fakeRepository) RevokeAllSessions(_ context.Context, userID string) error {
	for id, session := range r.sessions {
		if session.UserID == userID {
			_ = r.RevokeSession(context.Background(), userID, id)
		}
	}
	return nil
}
func (r *fakeRepository) ListSessions(context.Context, string) ([]domain.Session, error) {
	return nil, nil
}
func (r *fakeRepository) CreatePasswordResetToken(_ context.Context, _, tokenHash string, _ time.Time) error {
	r.resetTokenHash = tokenHash
	return nil
}
func (r *fakeRepository) CompletePasswordReset(_ context.Context, tokenHash, _ string) error {
	r.resetCompletedHash = tokenHash
	return nil
}
func (r *fakeRepository) CreateInvitation(_ context.Context, invitation domain.Invitation) error {
	r.invitation = invitation
	return nil
}
func (r *fakeRepository) AcceptInvitation(_ context.Context, tokenHash, passwordHash string) (domain.User, error) {
	if tokenHash != r.invitation.TokenHash {
		return domain.User{}, domain.ErrInvalidOneTimeToken
	}
	return domain.User{ID: "new-user", Email: r.invitation.Email, PasswordHash: passwordHash, DisplayName: r.invitation.DisplayName, Role: r.invitation.Role, Active: true}, nil
}
func (r *fakeRepository) ListAdmins(context.Context) ([]domain.AdminSummary, error) {
	return nil, nil
}
func (r *fakeRepository) UpdateAdmin(_ context.Context, id, displayName string, role domain.Role, active bool) (domain.AdminSummary, error) {
	for email, user := range r.usersByEmail {
		if user.ID != id {
			continue
		}
		if user.Role == domain.RoleOwner {
			return domain.AdminSummary{}, domain.ErrCannotModifyOwner
		}
		user.DisplayName = displayName
		user.Role = role
		user.Active = active
		r.usersByEmail[email] = user
		if !active {
			_ = r.RevokeAllSessions(context.Background(), id)
		}
		return domain.AdminSummary{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role, Active: user.Active}, nil
	}
	return domain.AdminSummary{}, domain.ErrUserNotFound
}
func (r *fakeRepository) CreateTelegramLinkCode(_ context.Context, _, _, tokenHash string, _ time.Time) error {
	r.telegramCodeHash = tokenHash
	return nil
}
func (r *fakeRepository) ConsumeTelegramLinkCode(_ context.Context, codeHash, telegramUserID string) error {
	if codeHash != r.telegramCodeHash {
		return domain.ErrInvalidOneTimeToken
	}
	r.telegramLinkedUserID = telegramUserID
	return nil
}

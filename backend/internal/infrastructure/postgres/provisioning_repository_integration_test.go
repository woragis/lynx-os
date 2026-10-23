package postgres

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

func TestProvisioningRepositoryIdempotencyAndAudit(t *testing.T) {
	databaseURL := os.Getenv("LYNX_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set LYNX_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("lynx_provisioning_test_%d", time.Now().UnixNano())
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adminPool.Close()
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		_, _ = adminPool.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE")
		adminPool.Close()
	})
	for _, ddl := range []string{
		`CREATE TABLE admin_users(id TEXT PRIMARY KEY)`,
		`CREATE TABLE integrations(id TEXT PRIMARY KEY)`,
		`CREATE TABLE provisioning_records(id TEXT PRIMARY KEY,idempotency_key TEXT NOT NULL,integration_id TEXT NOT NULL REFERENCES integrations(id),payload_hash CHAR(64) NOT NULL,payload_ciphertext TEXT NOT NULL,payload_iv TEXT NOT NULL,payload_auth_tag TEXT NOT NULL,response_ciphertext TEXT,response_iv TEXT,response_auth_tag TEXT,status TEXT NOT NULL,operation TEXT NOT NULL,actor_user_id TEXT REFERENCES admin_users(id),request_id TEXT,created_at TIMESTAMPTZ NOT NULL DEFAULT now(),updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),UNIQUE(integration_id,idempotency_key))`,
		`CREATE TABLE audit_logs(id TEXT PRIMARY KEY,action TEXT NOT NULL,integration_id TEXT REFERENCES integrations(id),request_id TEXT,result TEXT NOT NULL,actor_user_id TEXT REFERENCES admin_users(id),resource_type TEXT,resource_id TEXT)`,
	} {
		if _, err := pool.Exec(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO integrations(id) VALUES('saas'); INSERT INTO admin_users(id) VALUES('owner')`); err != nil {
		t.Fatal(err)
	}
	repository := NewIntegrationRepository(pool)
	entry := domain.ProvisioningRecord{ID: "op-1", IntegrationID: "saas", IdempotencyKey: "request-key-001", PayloadHash: strings.Repeat("a", 64), Payload: domain.SealedSecret{Ciphertext: "encrypted-input", IV: "iv", AuthTag: "tag"}, Status: "pending", Operation: "resource.update", ActorUserID: "owner", RequestID: "request-1"}
	stored, created, err := repository.StartProvisioning(ctx, entry)
	if err != nil || !created || stored.ID != entry.ID {
		t.Fatalf("start operation: created=%v record=%#v err=%v", created, stored, err)
	}
	if _, _, err := repository.StartProvisioning(ctx, domain.ProvisioningRecord{ID: "op-2", IntegrationID: "saas", IdempotencyKey: entry.IdempotencyKey, PayloadHash: "different-hash", Payload: entry.Payload}); err != domain.ErrIdempotencyConflict {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	response := domain.SealedSecret{Ciphertext: "encrypted-response", IV: "response-iv", AuthTag: "response-tag"}
	if err := repository.CompleteProvisioning(ctx, entry.ID, response, domain.AuditEntry{ID: "audit-1", ActorUserID: "owner", IntegrationID: "saas", Action: "resource.update", ResourceType: "customer", ResourceID: "customer-1", RequestID: "request-1", Result: "completed"}); err != nil {
		t.Fatal(err)
	}
	replayed, created, err := repository.StartProvisioning(ctx, domain.ProvisioningRecord{ID: "op-3", IntegrationID: "saas", IdempotencyKey: entry.IdempotencyKey, PayloadHash: entry.PayloadHash, Payload: entry.Payload})
	if err != nil || created || replayed.Status != "completed" || replayed.Response == nil || replayed.Response.Ciphertext != response.Ciphertext {
		t.Fatalf("replay lost persisted response: %#v created=%v err=%v", replayed, created, err)
	}
	var actor, requestID, action, result string
	var resourceType, resourceID string
	if err := pool.QueryRow(ctx, `SELECT actor_user_id,request_id,action,result,resource_type,resource_id FROM audit_logs WHERE id='audit-1'`).Scan(&actor, &requestID, &action, &result, &resourceType, &resourceID); err != nil {
		t.Fatal(err)
	}
	if actor != "owner" || requestID != "request-1" || action != "resource.update" || result != "completed" || resourceType != "customer" || resourceID != "customer-1" {
		t.Fatalf("unexpected audit record: %s %s %s %s %s %s", actor, requestID, action, result, resourceType, resourceID)
	}
}

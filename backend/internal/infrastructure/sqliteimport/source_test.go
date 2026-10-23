package sqliteimport

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestSourceReadsSnapshotWithoutWriteAccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE Integration(id TEXT,name TEXT,slug TEXT,baseUrl TEXT,appUrl TEXT,encryptedApiKey TEXT,apiKeyIv TEXT,apiKeyAuthTag TEXT,active BOOLEAN,revokedAt DATETIME,createdAt DATETIME,updatedAt DATETIME)`,
		`CREATE TABLE ProvisioningRecord(id TEXT,idempotencyKey TEXT,integrationId TEXT,payloadHash TEXT,payloadCiphertext TEXT,payloadIv TEXT,payloadAuthTag TEXT,responseCiphertext TEXT,responseIv TEXT,responseAuthTag TEXT,status TEXT,createdAt DATETIME)`,
		`CREATE TABLE AuditLog(id TEXT,action TEXT,integrationId TEXT,requestId TEXT,result TEXT,createdAt DATETIME)`,
		`CREATE TABLE TelegramConversation(chatId TEXT,state TEXT,draftJson TEXT,updatedAt DATETIME)`,
		`INSERT INTO Integration VALUES('i-1','EasyBill','easybill','https://easybill.test','https://app.easybill.test','cipher','iv','tag',1,NULL,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`,
		`INSERT INTO TelegramConversation VALUES('private-id','COLLECTING','{}','2026-01-01T00:00:00Z')`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	source, err := OpenSource(path)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if err := source.Integrity(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Fingerprint(); err != nil {
		t.Fatal(err)
	}
	dataset, err := source.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Integrations) != 1 || dataset.Integrations[0].Name != "EasyBill" || dataset.Integrations[0].AppURL != "https://app.easybill.test" || len(dataset.TelegramConversations) != 1 {
		t.Fatalf("unexpected source inventory: %#v", dataset)
	}
	if _, err := source.db.Exec(`DELETE FROM Integration`); err == nil {
		t.Fatal("SQLite source was not opened read-only")
	}
	if dataset.Integrations[0].CreatedAt.IsZero() || dataset.TelegramConversations[0].UpdatedAt.IsZero() {
		t.Fatal("timestamps were not read")
	}
}

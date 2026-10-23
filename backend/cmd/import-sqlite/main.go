package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	app "github.com/lynx/lynx-os/backend/internal/application/sqliteimport"
	"github.com/lynx/lynx-os/backend/internal/infrastructure/cryptox"
	pg "github.com/lynx/lynx-os/backend/internal/infrastructure/sqliteimport"
)

func main() {
	sourcePath := flag.String("source", "legacy/lynx-os.db", "path to the preserved SQLite source database (backend/legacy/lynx-os.db by default from backend/)")
	apply := flag.Bool("apply", false, "write the validated import in one PostgreSQL transaction; default is dry-run")
	inventoryOnly := flag.Bool("inventory-only", false, "check SQLite integrity and print table counts without requiring PostgreSQL or an encryption key")
	flag.Parse()
	if flag.NArg() != 0 {
		log.Fatal("unexpected positional arguments")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	source, err := pg.OpenSource(*sourcePath)
	if err != nil {
		log.Fatal("could not open SQLite source read-only")
	}
	defer source.Close()
	if *inventoryOnly {
		if err := source.Integrity(ctx); err != nil {
			log.Fatal("SQLite integrity check failed")
		}
		before, err := source.Fingerprint()
		if err != nil {
			log.Fatal("could not fingerprint SQLite source")
		}
		data, err := source.Read(ctx)
		if err != nil {
			log.Fatal("could not read SQLite source tables")
		}
		after, err := source.Fingerprint()
		if err != nil || before != after {
			log.Fatal("SQLite source changed during inventory")
		}
		counts := app.Counts{Integrations: len(data.Integrations), Operations: len(data.Operations), AuditLogs: len(data.AuditLogs), TelegramConversations: len(data.TelegramConversations)}
		fmt.Printf("SQLite integrity=ok source_fingerprint=%s integrations=%d operations=%d audit_logs=%d telegram_conversations=%d\n", before, counts.Integrations, counts.Operations, counts.AuditLogs, counts.TelegramConversations)
		return
	}
	url := os.Getenv("LYNX_IMPORT_DATABASE_URL")
	if url == "" {
		log.Fatal("LYNX_IMPORT_DATABASE_URL must explicitly identify the import destination")
	}
	keyHex := os.Getenv("LYNX_ENCRYPTION_KEY")
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		log.Fatal("LYNX_ENCRYPTION_KEY must contain 64 hexadecimal characters")
	}
	cipher, err := cryptox.NewAESGCM(key)
	if err != nil {
		log.Fatal("could not initialize encrypted data migration")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		log.Fatal("could not connect to the explicit PostgreSQL import destination")
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatal("PostgreSQL import destination is unavailable")
	}
	target := pg.NewTarget(pool)
	importer := app.New(source, target, cipher)
	if *apply {
		counts, err := importer.Apply(ctx)
		if err != nil {
			log.Fatalf("SQLite import failed: %v", err)
		}
		fmt.Printf("Import completed. integrations=%d operations=%d audit_logs=%d telegram_conversations_archived=%d\n", counts.Integrations, counts.Operations, counts.AuditLogs, counts.TelegramConversations)
		return
	}
	fingerprint, counts, err := importer.Preview(ctx)
	if err != nil {
		log.Fatalf("SQLite dry-run failed: %v", err)
	}
	fmt.Printf("Dry-run valid. source_fingerprint=%s integrations=%d operations=%d audit_logs=%d telegram_conversations_to_archive=%d\nNo data was changed. Review the counts and run with --apply when ready.\n", fingerprint, counts.Integrations, counts.Operations, counts.AuditLogs, counts.TelegramConversations)
}

package sqliteimport

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	app "github.com/lynx/lynx-os/backend/internal/application/sqliteimport"
	_ "github.com/mattn/go-sqlite3"
)

type Source struct {
	path string
	db   *sql.DB
}

func OpenSource(path string) (*Source, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(absolute); err != nil {
		return nil, err
	}
	uri := (&url.URL{Scheme: "file", Path: absolute}).String() + "?mode=ro&_query_only=1"
	db, err := sql.Open("sqlite3", uri)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &Source{path: absolute, db: db}, nil
}
func (s *Source) Close() error { return s.db.Close() }

func (s *Source) Integrity(ctx context.Context) error {
	var result string
	if err := s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&result); err != nil {
		return err
	}
	if result != "ok" {
		return errors.New("SQLite integrity check failed")
	}
	return nil
}
func (s *Source) Fingerprint() (string, error) {
	hasher := sha256.New()
	for _, path := range []string{s.path, s.path + "-wal"} {
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, _ = hasher.Write([]byte(filepath.Base(path)))
		_, _ = hasher.Write(data)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func (s *Source) Read(ctx context.Context) (app.Dataset, error) {
	var dataset app.Dataset
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,slug,baseUrl,COALESCE(appUrl,''),encryptedApiKey,apiKeyIv,apiKeyAuthTag,active,revokedAt,createdAt,updatedAt FROM Integration`)
	if err != nil {
		return dataset, fmt.Errorf("read SQLite integrations: %w", err)
	}
	for rows.Next() {
		var item app.LegacyIntegration
		var revoked sql.NullTime
		if err := rows.Scan(&item.ID, &item.Name, &item.Slug, &item.BaseURL, &item.AppURL, &item.Ciphertext, &item.IV, &item.AuthTag, &item.Active, &revoked, &item.CreatedAt, &item.UpdatedAt); err != nil {
			rows.Close()
			return dataset, err
		}
		if revoked.Valid {
			item.RevokedAt = &revoked.Time
		}
		dataset.Integrations = append(dataset.Integrations, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dataset, err
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id,idempotencyKey,integrationId,payloadHash,payloadCiphertext,payloadIv,payloadAuthTag,responseCiphertext,responseIv,responseAuthTag,status,createdAt FROM ProvisioningRecord`)
	if err != nil {
		return dataset, fmt.Errorf("read SQLite provisioning records: %w", err)
	}
	for rows.Next() {
		var item app.LegacyOperation
		var rc, ri, rt sql.NullString
		if err := rows.Scan(&item.ID, &item.IdempotencyKey, &item.IntegrationID, &item.PayloadHash, &item.PayloadCiphertext, &item.PayloadIV, &item.PayloadAuthTag, &rc, &ri, &rt, &item.Status, &item.CreatedAt); err != nil {
			rows.Close()
			return dataset, err
		}
		item.ResponseCiphertext, item.ResponseIV, item.ResponseAuthTag = rc.String, ri.String, rt.String
		dataset.Operations = append(dataset.Operations, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dataset, err
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT id,action,COALESCE(integrationId,''),COALESCE(requestId,''),result,createdAt FROM AuditLog`)
	if err != nil {
		return dataset, fmt.Errorf("read SQLite audit logs: %w", err)
	}
	for rows.Next() {
		var item app.LegacyAudit
		if err := rows.Scan(&item.ID, &item.Action, &item.IntegrationID, &item.RequestID, &item.Result, &item.CreatedAt); err != nil {
			rows.Close()
			return dataset, err
		}
		dataset.AuditLogs = append(dataset.AuditLogs, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dataset, err
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT chatId,state,draftJson,updatedAt FROM TelegramConversation`)
	if err != nil {
		return dataset, fmt.Errorf("read SQLite Telegram conversations: %w", err)
	}
	for rows.Next() {
		var item app.LegacyTelegram
		if err := rows.Scan(&item.ChatID, &item.State, &item.DraftJSON, &item.UpdatedAt); err != nil {
			rows.Close()
			return dataset, err
		}
		dataset.TelegramConversations = append(dataset.TelegramConversations, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return dataset, err
	}
	rows.Close()
	return dataset, nil
}

var _ app.Source = (*Source)(nil)

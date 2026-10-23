package sqliteimport

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	integration "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

var ErrTargetNotEmpty = errors.New("PostgreSQL import destination is not empty")

type SecretCipher interface {
	Encrypt([]byte) (integration.SealedSecret, error)
	Decrypt(integration.SealedSecret) ([]byte, error)
}
type Source interface {
	Integrity(context.Context) error
	Fingerprint() (string, error)
	Read(context.Context) (Dataset, error)
}
type Target interface {
	Counts(context.Context) (Counts, error)
	Import(context.Context, Dataset, string, Counts) error
}

type Counts struct {
	Integrations          int `json:"integrations"`
	Operations            int `json:"operations"`
	AuditLogs             int `json:"auditLogs"`
	TelegramConversations int `json:"telegramConversations"`
	DestinationOccupied   int `json:"-"`
}
type Dataset struct {
	Integrations          []LegacyIntegration
	Operations            []LegacyOperation
	AuditLogs             []LegacyAudit
	TelegramConversations []LegacyTelegram
}
type LegacyIntegration struct {
	ID, Name, Slug, BaseURL, AppURL, Ciphertext, IV, AuthTag string
	Active                                                   bool
	RevokedAt                                                *time.Time
	CreatedAt, UpdatedAt                                     time.Time
}
type LegacyOperation struct {
	ID, IdempotencyKey, IntegrationID, PayloadHash, PayloadCiphertext, PayloadIV, PayloadAuthTag, ResponseCiphertext, ResponseIV, ResponseAuthTag, Status, Operation string
	CreatedAt                                                                                                                                                        time.Time
}
type LegacyAudit struct {
	ID, Action, IntegrationID, RequestID, Result string
	CreatedAt                                    time.Time
}
type LegacyTelegram struct {
	ChatID, State, DraftJSON string
	EncryptedDraft           integration.SealedSecret
	UpdatedAt                time.Time
}

type Importer struct {
	source Source
	target Target
	cipher SecretCipher
}

func New(source Source, target Target, cipher SecretCipher) *Importer {
	return &Importer{source: source, target: target, cipher: cipher}
}

func (i *Importer) Preview(ctx context.Context) (string, Counts, error) {
	if err := i.source.Integrity(ctx); err != nil {
		return "", Counts{}, err
	}
	fingerprint, err := i.source.Fingerprint()
	if err != nil {
		return "", Counts{}, err
	}
	data, err := i.source.Read(ctx)
	if err != nil {
		return "", Counts{}, err
	}
	finalFingerprint, err := i.source.Fingerprint()
	if err != nil {
		return "", Counts{}, err
	}
	if fingerprint != finalFingerprint {
		return "", Counts{}, errors.New("SQLite source changed during inventory; retry from a stable copy")
	}
	counts := count(data)
	targetCounts, err := i.target.Counts(ctx)
	if err != nil {
		return "", Counts{}, err
	}
	if targetCounts != (Counts{}) {
		return "", Counts{}, ErrTargetNotEmpty
	}
	if err := i.normalize(&data); err != nil {
		return "", Counts{}, err
	}
	return fingerprint, counts, nil
}

func (i *Importer) Apply(ctx context.Context) (Counts, error) {
	fingerprint, counts, err := i.Preview(ctx)
	if err != nil {
		return Counts{}, err
	}
	before, err := i.source.Fingerprint()
	if err != nil {
		return Counts{}, err
	}
	data, err := i.source.Read(ctx)
	if err != nil {
		return Counts{}, err
	}
	after, err := i.source.Fingerprint()
	if err != nil {
		return Counts{}, err
	}
	if before != fingerprint || after != fingerprint {
		return Counts{}, errors.New("SQLite source changed between dry-run and import; no target data was written")
	}
	if err := i.normalize(&data); err != nil {
		return Counts{}, err
	}
	if err := i.target.Import(ctx, data, fingerprint, counts); err != nil {
		return Counts{}, err
	}
	return counts, nil
}

func count(data Dataset) Counts {
	return Counts{Integrations: len(data.Integrations), Operations: len(data.Operations), AuditLogs: len(data.AuditLogs), TelegramConversations: len(data.TelegramConversations)}
}

func (i *Importer) normalize(data *Dataset) error {
	for idx := range data.Integrations {
		item := &data.Integrations[idx]
		secret, err := normalizeSecret(item.Ciphertext, item.IV, item.AuthTag)
		if err != nil {
			return fmt.Errorf("integration secret encoding is invalid")
		}
		if item.RevokedAt == nil && item.Ciphertext == "" {
			return errors.New("an active integration has no encrypted API key")
		}
		if item.RevokedAt == nil && item.Ciphertext != "" {
			if _, err := i.cipher.Decrypt(secret); err != nil {
				return fmt.Errorf("LYNX_ENCRYPTION_KEY cannot decrypt an integration secret")
			}
		}
		item.Ciphertext, item.IV, item.AuthTag = secret.Ciphertext, secret.IV, secret.AuthTag
	}
	for idx := range data.Operations {
		item := &data.Operations[idx]
		payload, err := normalizeSecret(item.PayloadCiphertext, item.PayloadIV, item.PayloadAuthTag)
		if err != nil {
			return errors.New("provisioning payload encoding is invalid")
		}
		plain, err := i.cipher.Decrypt(payload)
		if err != nil {
			return errors.New("LYNX_ENCRYPTION_KEY cannot decrypt a provisioning payload")
		}
		var command integration.Command
		if err := json.Unmarshal(plain, &command); err != nil || command.Operation == "" || command.Input == nil {
			return errors.New("a provisioning payload cannot be converted to the v1 Go command format")
		}
		command.ContractVersion = "1"
		item.Operation = command.Operation
		canonical, err := json.Marshal(command)
		if err != nil {
			return err
		}
		sealed, err := i.cipher.Encrypt(canonical)
		if err != nil {
			return err
		}
		hash := sha256.Sum256(canonical)
		item.PayloadHash = hex.EncodeToString(hash[:])
		item.PayloadCiphertext, item.PayloadIV, item.PayloadAuthTag = sealed.Ciphertext, sealed.IV, sealed.AuthTag
		status := strings.ToLower(item.Status)
		if status != "pending" && status != "completed" {
			status = "failed"
		}
		item.Status = status
		if item.ResponseCiphertext != "" {
			oldResponse, err := normalizeSecret(item.ResponseCiphertext, item.ResponseIV, item.ResponseAuthTag)
			if err != nil {
				return errors.New("provisioning response encoding is invalid")
			}
			responseBytes, err := i.cipher.Decrypt(oldResponse)
			if err != nil {
				return errors.New("LYNX_ENCRYPTION_KEY cannot decrypt a provisioning response")
			}
			var response integration.Response
			if err := json.Unmarshal(responseBytes, &response); err != nil || response.ContractVersion != "1" || response.Operation == "" {
				response = integration.Response{ContractVersion: "1", RequestID: item.IdempotencyKey, Operation: command.Operation, Status: status, Data: json.RawMessage(responseBytes)}
			}
			response.Replayed = false
			normalized, err := json.Marshal(response)
			if err != nil {
				return err
			}
			sealedResponse, err := i.cipher.Encrypt(normalized)
			if err != nil {
				return err
			}
			item.ResponseCiphertext, item.ResponseIV, item.ResponseAuthTag = sealedResponse.Ciphertext, sealedResponse.IV, sealedResponse.AuthTag
		} else if item.Status == "completed" {
			item.Status = "failed"
		}
	}
	for idx := range data.TelegramConversations {
		legacy := &data.TelegramConversations[idx]
		payload, err := json.Marshal(map[string]string{"chatId": legacy.ChatID, "state": legacy.State, "draftJson": legacy.DraftJSON})
		if err != nil {
			return err
		}
		sealed, err := i.cipher.Encrypt(payload)
		if err != nil {
			return err
		}
		legacy.EncryptedDraft = sealed
	}
	return nil
}

func normalizeSecret(ciphertext, iv, tag string) (integration.SealedSecret, error) {
	decode := func(value string) (string, error) {
		if value == "" {
			return "", nil
		}
		raw, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			raw, err = base64.RawStdEncoding.DecodeString(value)
		}
		if err != nil {
			return "", err
		}
		return base64.RawStdEncoding.EncodeToString(raw), nil
	}
	c, err := decode(ciphertext)
	if err != nil {
		return integration.SealedSecret{}, err
	}
	n, err := decode(iv)
	if err != nil {
		return integration.SealedSecret{}, err
	}
	t, err := decode(tag)
	if err != nil {
		return integration.SealedSecret{}, err
	}
	return integration.SealedSecret{Ciphertext: c, IV: n, AuthTag: t}, nil
}

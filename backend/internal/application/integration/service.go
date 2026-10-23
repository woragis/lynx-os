package integration

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var (
	ErrInvalidIntegration = errors.New("integration data is invalid")
	ErrIntegrationRevoked = errors.New("integration is revoked")
	ErrSaaSValidation     = errors.New("SaaS contract validation failed")
	ErrInvalidOperation   = errors.New("provisioning operation is invalid")
	ErrSaaSExecution      = errors.New("SaaS operation failed")
)

type Service struct {
	repository domain.Repository
	operations domain.OperationRepository
	cipher     domain.SecretCipher
	contract   *ContractService
}

type CreateInput struct {
	Name    string
	Slug    string
	BaseURL string
	APIKey  string
}

func NewService(repository domain.Repository, operations domain.OperationRepository, cipher domain.SecretCipher, contract *ContractService) *Service {
	return &Service{repository: repository, operations: operations, cipher: cipher, contract: contract}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (domain.Record, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(input.Slug)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.APIKey = strings.TrimSpace(input.APIKey)
	if input.Name == "" || len(input.Name) > 120 || !slugPattern.MatchString(input.Slug) || len(input.Slug) > 80 || strings.TrimSpace(input.APIKey) == "" {
		return domain.Record{}, ErrInvalidIntegration
	}
	if _, err := s.contract.Discover(ctx, input.BaseURL, input.APIKey); err != nil {
		return domain.Record{}, fmt.Errorf("%w: %v", ErrSaaSValidation, err)
	}
	sealed, err := s.cipher.Encrypt([]byte(input.APIKey))
	if err != nil {
		return domain.Record{}, err
	}
	id, err := newID()
	if err != nil {
		return domain.Record{}, err
	}
	now := time.Now().UTC()
	return s.repository.Create(ctx, domain.Record{
		ID: id, Name: input.Name, Slug: input.Slug, BaseURL: input.BaseURL,
		Active: true, CreatedAt: now, UpdatedAt: now,
	}, sealed)
}

func (s *Service) List(ctx context.Context) ([]domain.Record, error) {
	return s.repository.List(ctx)
}

func (s *Service) Capabilities(ctx context.Context, id string) (domain.Capabilities, error) {
	item, sealed, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.Capabilities{}, err
	}
	if !item.Active || item.RevokedAt != nil {
		return domain.Capabilities{}, ErrIntegrationRevoked
	}
	apiKey, err := s.cipher.Decrypt(sealed)
	if err != nil {
		return domain.Capabilities{}, err
	}
	capabilities, err := s.contract.Discover(ctx, item.BaseURL, string(apiKey))
	if err != nil {
		return domain.Capabilities{}, fmt.Errorf("%w: %v", ErrSaaSExecution, err)
	}
	return capabilities, nil
}

func (s *Service) RotateKey(ctx context.Context, id, apiKey string) (domain.Record, error) {
	apiKey = strings.TrimSpace(apiKey)
	current, _, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.Record{}, err
	}
	if !current.Active || current.RevokedAt != nil {
		return domain.Record{}, ErrIntegrationRevoked
	}
	if strings.TrimSpace(apiKey) == "" {
		return domain.Record{}, ErrInvalidIntegration
	}
	if _, err := s.contract.Discover(ctx, current.BaseURL, apiKey); err != nil {
		return domain.Record{}, fmt.Errorf("%w: %v", ErrSaaSValidation, err)
	}
	sealed, err := s.cipher.Encrypt([]byte(apiKey))
	if err != nil {
		return domain.Record{}, err
	}
	return s.repository.ReplaceSecret(ctx, id, sealed)
}

func (s *Service) Revoke(ctx context.Context, id string) (domain.Record, error) {
	return s.repository.Revoke(ctx, id)
}

func (s *Service) Execute(ctx context.Context, id, actorID, requestID string, command domain.Command, idempotencyKey string) (domain.Response, error) {
	item, sealed, err := s.repository.Get(ctx, id)
	if err != nil {
		return domain.Response{}, err
	}
	if !item.Active || item.RevokedAt != nil {
		return domain.Response{}, ErrIntegrationRevoked
	}
	apiKey, err := s.cipher.Decrypt(sealed)
	if err != nil {
		return domain.Response{}, err
	}
	encodedCommand, err := json.Marshal(command)
	if err != nil {
		return domain.Response{}, err
	}
	if idempotencyKey == "" {
		response, executeErr := s.contract.Execute(ctx, item.BaseURL, string(apiKey), command, "")
		if executeErr != nil {
			if errors.Is(executeErr, domain.ErrInvalidCommand) || errors.Is(executeErr, domain.ErrIdempotencyKeyRequired) || errors.Is(executeErr, domain.ErrOperationUnsupported) {
				return domain.Response{}, fmt.Errorf("%w: %v", ErrInvalidOperation, executeErr)
			}
			return domain.Response{}, fmt.Errorf("%w: %v", ErrSaaSExecution, executeErr)
		}
		return response, nil
	}
	payloadHash := sha256.Sum256(encodedCommand)
	sealedPayload, err := s.cipher.Encrypt(encodedCommand)
	if err != nil {
		return domain.Response{}, err
	}
	operationID, err := newID()
	if err != nil {
		return domain.Response{}, err
	}
	record, created, err := s.operations.StartProvisioning(ctx, domain.ProvisioningRecord{
		ID: operationID, IntegrationID: id, IdempotencyKey: idempotencyKey,
		PayloadHash: hex.EncodeToString(payloadHash[:]), Payload: sealedPayload,
		Status: "pending", Operation: command.Operation, ActorUserID: actorID, RequestID: requestID,
	})
	if err != nil {
		return domain.Response{}, err
	}
	if !created {
		if record.PayloadHash != hex.EncodeToString(payloadHash[:]) {
			return domain.Response{}, domain.ErrIdempotencyConflict
		}
		if record.Status == "pending" {
			return domain.Response{}, domain.ErrOperationInProgress
		}
		if record.Status == "completed" && record.Response != nil {
			plain, decryptErr := s.cipher.Decrypt(*record.Response)
			if decryptErr != nil {
				return domain.Response{}, decryptErr
			}
			var replay domain.Response
			if err := json.Unmarshal(plain, &replay); err != nil {
				return domain.Response{}, err
			}
			replay.Replayed = true
			return replay, nil
		}
	}
	response, err := s.contract.Execute(ctx, item.BaseURL, string(apiKey), command, idempotencyKey)
	if err != nil {
		_ = s.operations.FailProvisioning(ctx, record.ID, auditEntry(record.ID, actorID, id, command, requestID, "failed"))
		if errors.Is(err, domain.ErrInvalidCommand) || errors.Is(err, domain.ErrIdempotencyKeyRequired) || errors.Is(err, domain.ErrOperationUnsupported) {
			return domain.Response{}, fmt.Errorf("%w: %v", ErrInvalidOperation, err)
		}
		return domain.Response{}, fmt.Errorf("%w: %v", ErrSaaSExecution, err)
	}
	encodedResponse, err := json.Marshal(response)
	if err != nil {
		_ = s.operations.FailProvisioning(ctx, record.ID, auditEntry(record.ID, actorID, id, command, requestID, "failed"))
		return domain.Response{}, err
	}
	sealedResponse, err := s.cipher.Encrypt(encodedResponse)
	if err != nil {
		_ = s.operations.FailProvisioning(ctx, record.ID, auditEntry(record.ID, actorID, id, command, requestID, "failed"))
		return domain.Response{}, err
	}
	if err := s.operations.CompleteProvisioning(ctx, record.ID, sealedResponse, auditEntry(record.ID, actorID, id, command, requestID, "completed")); err != nil {
		return domain.Response{}, err
	}
	return response, nil
}

func auditEntry(operationID, actorID, integrationID string, command domain.Command, requestID, result string) domain.AuditEntry {
	resourceType, _ := command.Input["resourceType"].(string)
	resourceID, _ := command.Input["id"].(string)
	return domain.AuditEntry{ID: "audit-" + operationID + "-" + result, ActorUserID: actorID, IntegrationID: integrationID,
		Action: command.Operation, ResourceType: resourceType, ResourceID: resourceID, RequestID: requestID, Result: result}
}

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

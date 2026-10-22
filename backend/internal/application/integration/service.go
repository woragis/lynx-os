package integration

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
)

type Service struct {
	repository domain.Repository
	cipher     domain.SecretCipher
	contract   *ContractService
}

type CreateInput struct {
	Name    string
	Slug    string
	BaseURL string
	APIKey  string
}

func NewService(repository domain.Repository, cipher domain.SecretCipher, contract *ContractService) *Service {
	return &Service{repository: repository, cipher: cipher, contract: contract}
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

func newID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

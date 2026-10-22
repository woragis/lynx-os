package integration

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Record struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Slug      string     `json:"slug"`
	BaseURL   string     `json:"baseUrl"`
	Active    bool       `json:"active"`
	RevokedAt *time.Time `json:"revokedAt,omitempty"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type SealedSecret struct {
	Ciphertext string
	IV         string
	AuthTag    string
}

type Repository interface {
	Create(context.Context, Record, SealedSecret) (Record, error)
	List(context.Context) ([]Record, error)
	Get(context.Context, string) (Record, SealedSecret, error)
	ReplaceSecret(context.Context, string, SealedSecret) (Record, error)
	Revoke(context.Context, string) (Record, error)
}

type SecretCipher interface {
	Encrypt([]byte) (SealedSecret, error)
	Decrypt(SealedSecret) ([]byte, error)
}

var (
	ErrUnsupportedContractVersion = errors.New("SaaS contract version is not supported")
	ErrOperationUnsupported       = errors.New("operation is not advertised by SaaS")
	ErrInvalidCommand             = errors.New("command is invalid")
	ErrIdempotencyKeyRequired     = errors.New("idempotency key is required for writes")
	ErrIntegrationNotFound        = errors.New("integration not found")
)

type ServiceDescriptor struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type ResourceCapabilities struct {
	Operations []string                   `json:"operations"`
	Fields     map[string]json.RawMessage `json:"fields,omitempty"`
	Purge      json.RawMessage            `json:"purge,omitempty"`
}

type Capabilities struct {
	ContractVersion string                          `json:"contractVersion"`
	Service         ServiceDescriptor               `json:"service"`
	Operations      map[string]json.RawMessage      `json:"operations"`
	Resources       map[string]ResourceCapabilities `json:"resources"`
	Bootstrap       json.RawMessage                 `json:"bootstrap,omitempty"`
	Lifecycle       json.RawMessage                 `json:"lifecycle,omitempty"`
}

type Command struct {
	ContractVersion string         `json:"contractVersion"`
	Operation       string         `json:"operation"`
	Input           map[string]any `json:"input"`
}

type Response struct {
	ContractVersion string          `json:"contractVersion"`
	RequestID       string          `json:"requestId"`
	Operation       string          `json:"operation"`
	Status          string          `json:"status"`
	Data            json.RawMessage `json:"data"`
	Replayed        bool            `json:"replayed"`
}

type SaaSContract interface {
	Discover(context.Context, string, string) (Capabilities, error)
	Execute(context.Context, string, string, Command, string) (Response, error)
}

package telegram

import (
	"context"
	"errors"
	"time"

	identity "github.com/lynx/lynx-os/backend/internal/domain/identity"
	integration "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

var (
	ErrConversationNotFound = errors.New("telegram conversation not found")
	ErrUpdateAlreadyHandled = errors.New("telegram update already handled")
)

type Conversation struct {
	TelegramUserID string
	AdminUserID    string
	State          string
	BotMessageID   *int64
	ExpiresAt      time.Time
}

type ConversationRepository interface {
	GetConversation(context.Context, string) (Conversation, integration.SealedSecret, error)
	SaveConversation(context.Context, Conversation, integration.SealedSecret) error
	DeleteConversation(context.Context, string) error
	GetUpdateOffset(context.Context) (int64, error)
	BeginUpdate(context.Context, int64, string) (bool, error)
	CompleteUpdate(context.Context, int64, string, bool) error
}

type Leader interface {
	TryAcquireLeader(context.Context) (release func(), acquired bool, err error)
}

type IdentityRepository interface {
	FindByTelegramUserID(context.Context, string) (identity.User, error)
	FindByID(context.Context, string) (identity.User, error)
}

type IdentityLinker interface {
	LinkTelegramIdentity(context.Context, string, string) error
}

type IntegrationGateway interface {
	List(context.Context) ([]integration.Record, error)
	Capabilities(context.Context, string) (integration.Capabilities, error)
	Execute(context.Context, string, string, string, integration.Command, string) (integration.Response, error)
}

type DraftExtractor interface {
	Extract(context.Context, string, Draft, []System) (Draft, error)
}

type System struct{ ID, Name, Slug string }

type Draft struct {
	Intent            string  `json:"intent,omitempty"`
	AssistantReply    string  `json:"assistantReply,omitempty"`
	IntegrationName   *string `json:"integrationName"`
	AccountName       *string `json:"accountName"`
	AccountTaxID      *string `json:"accountTaxId"`
	AccountEmail      *string `json:"accountEmail"`
	AccountAddress    *string `json:"accountAddress"`
	CustomerTradeName *string `json:"customerTradeName"`
	CustomerLegalName *string `json:"customerLegalName"`
	CustomerTaxID     *string `json:"customerTaxId"`
	CustomerEmail     *string `json:"customerEmail"`
	CustomerPhone     *string `json:"customerPhone"`
	CustomerTaxRegime *string `json:"customerTaxRegime"`
	RelationshipType  *string `json:"relationshipType"`
	CustomerStatus    *string `json:"customerStatus"`
	UserName          *string `json:"userName"`
	UserEmail         *string `json:"userEmail"`
	UserPhone         *string `json:"userPhone"`
}

type ConversationData struct {
	Draft            Draft          `json:"draft"`
	IntegrationID    string         `json:"integrationId,omitempty"`
	IdempotencyKey   string         `json:"idempotencyKey,omitempty"`
	PendingOperation string         `json:"pendingOperation,omitempty"`
	PendingInput     map[string]any `json:"pendingInput,omitempty"`
	ResourceType     string         `json:"resourceType,omitempty"`
	AccountID        string         `json:"accountId,omitempty"`
	ResourceFields   []string       `json:"resourceFields,omitempty"`
}

type Update struct {
	ID       int64
	Message  *Message
	Callback *Callback
}

type Message struct {
	Text     string
	FromID   int64
	ChatID   int64
	ChatType string
}

type Callback struct {
	ID       string
	FromID   int64
	ChatID   int64
	ChatType string
	Data     string
}

type Button struct{ Text, Data string }
type Reply struct {
	Text            string
	Buttons         [][]Button
	CallbackQueryID string
	ChatID          int64
	MessageID       *int64
	DeleteMessageID *int64
}

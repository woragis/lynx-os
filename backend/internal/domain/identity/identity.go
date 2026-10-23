package identity

import (
	"context"
	"errors"
	"time"
)

type Role string

type Permission string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

const (
	PermissionViewIntegrations   Permission = "integrations:view"
	PermissionManageIntegrations Permission = "integrations:manage"
	PermissionRunOperations      Permission = "operations:run"
	PermissionManageAdmins       Permission = "admins:manage"
)

func (role Role) Allows(permission Permission) bool {
	switch permission {
	case PermissionViewIntegrations:
		return role == RoleOwner || role == RoleAdmin || role == RoleOperator || role == RoleViewer
	case PermissionManageIntegrations:
		return role == RoleOwner || role == RoleAdmin
	case PermissionRunOperations:
		return role == RoleOwner || role == RoleAdmin || role == RoleOperator
	case PermissionManageAdmins:
		return role == RoleOwner
	default:
		return false
	}
}

var (
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrInvalidRefresh         = errors.New("refresh token is invalid")
	ErrRefreshReuse           = errors.New("refresh token reuse detected")
	ErrSessionRevoked         = errors.New("session is revoked")
	ErrUserInactive           = errors.New("user is inactive")
	ErrOwnerAlreadyExists     = errors.New("initial owner already exists")
	ErrUserNotFound           = errors.New("administrator not found")
	ErrTooManyAttempts        = errors.New("too many login attempts")
	ErrRateLimiterUnavailable = errors.New("login rate limiter unavailable")
	ErrInvalidOneTimeToken    = errors.New("one-time token is invalid or expired")
	ErrEmailAlreadyUsed       = errors.New("email is already in use")
	ErrInvalidRole            = errors.New("administrator role is invalid")
	ErrCannotModifyOwner      = errors.New("owner account cannot be modified")
	ErrCannotDeactivateSelf   = errors.New("current account cannot be deactivated here")
	ErrMailerUnavailable      = errors.New("email delivery is unavailable")
	ErrTelegramAlreadyLinked  = errors.New("telegram identity is already linked")
)

type User struct {
	ID           string
	Email        string
	PasswordHash string
	DisplayName  string
	Role         Role
	Active       bool
	CreatedAt    time.Time
	LastLoginAt  *time.Time
}

type Session struct {
	ID               string
	FamilyID         string
	UserID           string
	RefreshTokenHash string
	ExpiresAt        time.Time
	CreatedAt        time.Time
	LastUsedAt       time.Time
	RevokedAt        *time.Time
	IPAddress        string
	UserAgent        string
}

type AdminSummary struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"displayName"`
	Role        Role       `json:"role"`
	Active      bool       `json:"active"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
}

type Invitation struct {
	ID          string
	Email       string
	DisplayName string
	Role        Role
	TokenHash   string
	InvitedBy   string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

type Repository interface {
	CreateInitialOwner(context.Context, User) error
	FindByEmail(context.Context, string) (User, error)
	FindByID(context.Context, string) (User, error)
	TouchLastLogin(context.Context, string) error
	CreateSession(context.Context, Session) error
	GetActiveSession(context.Context, string) (Session, User, error)
	RotateSession(context.Context, string, Session) error
	RevokeSession(context.Context, string, string) error
	RevokeSessionByRefreshHash(context.Context, string) error
	RevokeAllSessions(context.Context, string) error
	ListSessions(context.Context, string) ([]Session, error)
	CreatePasswordResetToken(context.Context, string, string, time.Time) error
	CompletePasswordReset(context.Context, string, string) error
	CreateInvitation(context.Context, Invitation) error
	AcceptInvitation(context.Context, string, string) (User, error)
	ListAdmins(context.Context) ([]AdminSummary, error)
	UpdateAdmin(context.Context, string, string, Role, bool) (AdminSummary, error)
	CreateTelegramLinkCode(context.Context, string, string, string, time.Time) error
	ConsumeTelegramLinkCode(context.Context, string, string) error
	FindByTelegramUserID(context.Context, string) (User, error)
}

type PasswordResetMailer interface {
	SendPasswordReset(context.Context, string, string, time.Time) error
}

type LoginRateLimiter interface {
	Allow(context.Context, string) (bool, error)
	RecordFailure(context.Context, string) error
	Reset(context.Context, string) error
}

type AccessClaims struct {
	UserID    string
	SessionID string
}

type AccessTokenManager interface {
	Sign(AccessClaims, time.Time) (string, error)
	Verify(string) (AccessClaims, error)
}

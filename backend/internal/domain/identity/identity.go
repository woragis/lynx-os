package identity

import (
	"context"
	"errors"
	"time"
)

type Role string

const (
	RoleOwner    Role = "owner"
	RoleAdmin    Role = "admin"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

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

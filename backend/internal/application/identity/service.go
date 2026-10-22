package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	"golang.org/x/crypto/bcrypt"
)

const (
	accessTokenTTL  = 10 * time.Minute
	refreshTokenTTL = 14 * 24 * time.Hour
)

type Service struct {
	repository domain.Repository
	rateLimit  domain.LoginRateLimiter
	tokens     domain.AccessTokenManager
	now        func() time.Time
	dummyHash  []byte
}

type PublicUser struct {
	ID          string      `json:"id"`
	Email       string      `json:"email"`
	DisplayName string      `json:"displayName"`
	Role        domain.Role `json:"role"`
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
	User         PublicUser
	Session      domain.Session
}

func NewService(repository domain.Repository, rateLimit domain.LoginRateLimiter, tokens domain.AccessTokenManager) (*Service, error) {
	if tokens == nil {
		return nil, errors.New("access token manager is required")
	}
	dummyHash, err := bcrypt.GenerateFromPassword([]byte("lynx-invalid-account-password"), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("initialize password verifier: %w", err)
	}
	if rateLimit == nil {
		return nil, errors.New("login rate limiter is required")
	}
	return &Service{repository: repository, rateLimit: rateLimit, tokens: tokens, now: time.Now, dummyHash: dummyHash}, nil
}

func CreateInitialOwner(ctx context.Context, repository domain.Repository, email, password, displayName string) error {
	email = normalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	if !validEmail(email) || !validPassword(password) || displayName == "" || len(displayName) > 120 {
		return ErrInvalidAccount
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	id, err := randomID()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return repository.CreateInitialOwner(ctx, domain.User{
		ID: id, Email: email, PasswordHash: string(hash), DisplayName: displayName,
		Role: domain.RoleOwner, Active: true, CreatedAt: now,
	})
}

func (s *Service) Login(ctx context.Context, email, password, ipAddress, userAgent string) (Tokens, error) {
	email = normalizeEmail(email)
	rateKey := loginRateKey(ipAddress, email)
	allowed, err := s.rateLimit.Allow(ctx, rateKey)
	if err != nil {
		return Tokens{}, domain.ErrRateLimiterUnavailable
	}
	if !allowed {
		return Tokens{}, domain.ErrTooManyAttempts
	}
	user, err := s.repository.FindByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return Tokens{}, err
	}
	if err != nil || !user.Active {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		if s.rateLimit.RecordFailure(ctx, rateKey) != nil {
			return Tokens{}, domain.ErrRateLimiterUnavailable
		}
		return Tokens{}, domain.ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		if s.rateLimit.RecordFailure(ctx, rateKey) != nil {
			return Tokens{}, domain.ErrRateLimiterUnavailable
		}
		return Tokens{}, domain.ErrInvalidCredentials
	}
	if err := s.rateLimit.Reset(ctx, rateKey); err != nil {
		return Tokens{}, domain.ErrRateLimiterUnavailable
	}
	now := s.now().UTC()
	if err := s.repository.TouchLastLogin(ctx, user.ID); err != nil {
		return Tokens{}, err
	}
	session, refresh, err := s.newSession(user.ID, "", ipAddress, userAgent, now)
	if err != nil {
		return Tokens{}, err
	}
	if err := s.repository.CreateSession(ctx, session); err != nil {
		return Tokens{}, err
	}
	access, err := s.tokens.Sign(domain.AccessClaims{UserID: user.ID, SessionID: session.ID}, now.Add(accessTokenTTL))
	if err != nil {
		_ = s.repository.RevokeSession(ctx, user.ID, session.ID)
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: refresh, ExpiresIn: int64(accessTokenTTL.Seconds()), User: publicUser(user), Session: session}, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken, ipAddress, userAgent string) (Tokens, error) {
	if len(refreshToken) < 40 || len(refreshToken) > 256 {
		return Tokens{}, domain.ErrInvalidRefresh
	}
	now := s.now().UTC()
	newSession, newRefresh, err := s.newSession("", "", ipAddress, userAgent, now)
	if err != nil {
		return Tokens{}, err
	}
	oldHash := hashToken(refreshToken)
	newSession.FamilyID = ""
	if err := s.repository.RotateSession(ctx, oldHash, newSession); err != nil {
		return Tokens{}, err
	}
	session, user, err := s.repository.GetActiveSession(ctx, newSession.ID)
	if err != nil {
		return Tokens{}, err
	}
	access, err := s.tokens.Sign(domain.AccessClaims{UserID: user.ID, SessionID: session.ID}, now.Add(accessTokenTTL))
	if err != nil {
		_ = s.repository.RevokeSession(ctx, user.ID, session.ID)
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: newRefresh, ExpiresIn: int64(accessTokenTTL.Seconds()), User: publicUser(user), Session: session}, nil
}

func (s *Service) Authenticate(ctx context.Context, accessToken string) (PublicUser, domain.Session, error) {
	claims, err := s.tokens.Verify(accessToken)
	if err != nil || claims.SessionID == "" || claims.UserID == "" {
		return PublicUser{}, domain.Session{}, domain.ErrSessionRevoked
	}
	session, user, err := s.repository.GetActiveSession(ctx, claims.SessionID)
	if err != nil || session.UserID != claims.UserID || !user.Active {
		return PublicUser{}, domain.Session{}, domain.ErrSessionRevoked
	}
	return publicUser(user), session, nil
}

func (s *Service) Logout(ctx context.Context, userID, sessionID string) error {
	return s.repository.RevokeSession(ctx, userID, sessionID)
}

func (s *Service) LogoutRefresh(ctx context.Context, refreshToken string) error {
	if len(refreshToken) < 40 || len(refreshToken) > 256 {
		return domain.ErrInvalidRefresh
	}
	return s.repository.RevokeSessionByRefreshHash(ctx, hashToken(refreshToken))
}

func (s *Service) LogoutAll(ctx context.Context, userID string) error {
	return s.repository.RevokeAllSessions(ctx, userID)
}

func (s *Service) Sessions(ctx context.Context, userID string) ([]domain.Session, error) {
	return s.repository.ListSessions(ctx, userID)
}

func (s *Service) newSession(userID, familyID, ipAddress, userAgent string, now time.Time) (domain.Session, string, error) {
	id, err := randomID()
	if err != nil {
		return domain.Session{}, "", err
	}
	if familyID == "" {
		familyID = id
	}
	refreshBytes := make([]byte, 32)
	if _, err := rand.Read(refreshBytes); err != nil {
		return domain.Session{}, "", err
	}
	refresh := base64.RawURLEncoding.EncodeToString(refreshBytes)
	return domain.Session{
		ID: id, FamilyID: familyID, UserID: userID, RefreshTokenHash: hashToken(refresh),
		ExpiresAt: now.Add(refreshTokenTTL), CreatedAt: now, LastUsedAt: now,
		IPAddress: trimBounded(ipAddress, 64), UserAgent: trimBounded(userAgent, 256),
	}, refresh, nil
}

var ErrInvalidAccount = errors.New("initial administrator details are invalid")

func validPassword(value string) bool { return len(value) >= 12 && len(value) <= 72 }
func validEmail(value string) bool {
	parts := strings.Split(value, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".") && len(value) <= 254
}
func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }
func publicUser(user domain.User) PublicUser {
	return PublicUser{ID: user.ID, Email: user.Email, DisplayName: user.DisplayName, Role: user.Role}
}
func hashToken(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func loginRateKey(ipAddress, email string) string {
	value := strings.TrimSpace(ipAddress) + "\x00" + normalizeEmail(email)
	hash := sha256.Sum256([]byte(value))
	return "lynx:auth:login:" + hex.EncodeToString(hash[:])
}
func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
func trimBounded(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}

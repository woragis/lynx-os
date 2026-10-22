package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
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
	mailer     domain.PasswordResetMailer
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

func NewService(repository domain.Repository, rateLimit domain.LoginRateLimiter, tokens domain.AccessTokenManager, mailer domain.PasswordResetMailer) (*Service, error) {
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
	return &Service{repository: repository, rateLimit: rateLimit, tokens: tokens, mailer: mailer, now: time.Now, dummyHash: dummyHash}, nil
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

func (s *Service) RequestPasswordReset(ctx context.Context, email, ipAddress string) error {
	for _, key := range passwordResetRateKeys(email, ipAddress) {
		allowed, err := s.rateLimit.Allow(ctx, key)
		if err != nil {
			return domain.ErrRateLimiterUnavailable
		}
		if !allowed {
			return nil
		}
	}
	for _, key := range passwordResetRateKeys(email, ipAddress) {
		if err := s.rateLimit.RecordFailure(ctx, key); err != nil {
			return domain.ErrRateLimiterUnavailable
		}
	}
	user, err := s.repository.FindByEmail(ctx, normalizeEmail(email))
	if errors.Is(err, domain.ErrUserNotFound) || (err == nil && !user.Active) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.mailer == nil {
		return domain.ErrMailerUnavailable
	}
	token, err := randomToken()
	if err != nil {
		return err
	}
	expiresAt := s.now().UTC().Add(15 * time.Minute)
	if err := s.repository.CreatePasswordResetToken(ctx, user.ID, hashToken(token), expiresAt); err != nil {
		return err
	}
	return s.mailer.SendPasswordReset(ctx, user.Email, token, expiresAt)
}

func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	if len(token) < 40 || len(token) > 256 || !validPassword(password) {
		return domain.ErrInvalidOneTimeToken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repository.CompletePasswordReset(ctx, hashToken(token), string(hash))
}

func (s *Service) Invite(ctx context.Context, actorID, email, displayName string, role domain.Role) (string, error) {
	email = normalizeEmail(email)
	displayName = strings.TrimSpace(displayName)
	if !validEmail(email) || displayName == "" || len(displayName) > 120 {
		return "", ErrInvalidAccount
	}
	if role != domain.RoleAdmin && role != domain.RoleOperator && role != domain.RoleViewer {
		return "", domain.ErrInvalidRole
	}
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	id, err := randomID()
	if err != nil {
		return "", err
	}
	now := s.now().UTC()
	err = s.repository.CreateInvitation(ctx, domain.Invitation{
		ID: id, Email: email, DisplayName: displayName, Role: role,
		TokenHash: hashToken(token), InvitedBy: actorID,
		ExpiresAt: now.Add(24 * time.Hour), CreatedAt: now,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) AcceptInvitation(ctx context.Context, token, password string) error {
	if len(token) < 40 || len(token) > 256 || !validPassword(password) {
		return domain.ErrInvalidOneTimeToken
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = s.repository.AcceptInvitation(ctx, hashToken(token), string(hash))
	return err
}

func (s *Service) Admins(ctx context.Context) ([]domain.AdminSummary, error) {
	return s.repository.ListAdmins(ctx)
}

func (s *Service) UpdateAdmin(ctx context.Context, actorID, targetID, displayName string, role domain.Role, active bool) (domain.AdminSummary, error) {
	if actorID == targetID && !active {
		return domain.AdminSummary{}, domain.ErrCannotDeactivateSelf
	}
	if role != domain.RoleAdmin && role != domain.RoleOperator && role != domain.RoleViewer {
		return domain.AdminSummary{}, domain.ErrInvalidRole
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || len(displayName) > 120 {
		return domain.AdminSummary{}, ErrInvalidAccount
	}
	return s.repository.UpdateAdmin(ctx, targetID, displayName, role, active)
}

func (s *Service) CreateTelegramLinkCode(ctx context.Context, userID string) (string, error) {
	value := make([]byte, 4)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	code := fmt.Sprintf("%08d", binary.BigEndian.Uint32(value)%100_000_000)
	id, err := randomID()
	if err != nil {
		return "", err
	}
	if err := s.repository.CreateTelegramLinkCode(ctx, id, userID, hashToken(code), s.now().UTC().Add(10*time.Minute)); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Service) LinkTelegramIdentity(ctx context.Context, telegramUserID, code string) error {
	if len(telegramUserID) == 0 || len(telegramUserID) > 32 || len(code) != 8 {
		return domain.ErrInvalidOneTimeToken
	}
	return s.repository.ConsumeTelegramLinkCode(ctx, hashToken(code), telegramUserID)
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
func passwordResetRateKeys(email, ipAddress string) []string {
	return []string{
		"lynx:auth:reset:ip:" + hashToken(strings.TrimSpace(ipAddress)),
		"lynx:auth:reset:email:" + hashToken(normalizeEmail(email)),
	}
}
func randomID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
func trimBounded(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}

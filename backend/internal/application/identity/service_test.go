package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
	"golang.org/x/crypto/bcrypt"
)

func TestLoginCreatesSessionAndOnlyStoresRefreshHash(t *testing.T) {
	repository := newFakeRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository.usersByEmail["owner@lynx.test"] = domain.User{
		ID: "user-1", Email: "owner@lynx.test", PasswordHash: string(hash), DisplayName: "Owner", Role: domain.RoleOwner, Active: true,
	}
	service := newTestService(t, repository, &fakeLimiter{})

	result, err := service.Login(context.Background(), " OWNER@LYNX.TEST ", "a-long-test-password", "127.0.0.1", "test-agent")
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" || result.User.Role != domain.RoleOwner {
		t.Fatalf("Login returned incomplete session: %#v", result)
	}
	stored := repository.sessions[result.Session.ID]
	if stored.RefreshTokenHash == result.RefreshToken || stored.RefreshTokenHash == "" {
		t.Fatal("repository must store the refresh token hash, not the raw token")
	}
}

func TestLoginDoesNotRevealWhetherAccountExists(t *testing.T) {
	repository := newFakeRepository()
	hash, err := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	repository.usersByEmail["owner@lynx.test"] = domain.User{ID: "user-1", Email: "owner@lynx.test", PasswordHash: string(hash), Active: true}
	service := newTestService(t, repository, &fakeLimiter{})

	_, unknownErr := service.Login(context.Background(), "missing@lynx.test", "invalid-password", "127.0.0.1", "")
	_, wrongPasswordErr := service.Login(context.Background(), "owner@lynx.test", "invalid-password", "127.0.0.1", "")
	if !errors.Is(unknownErr, domain.ErrInvalidCredentials) || !errors.Is(wrongPasswordErr, domain.ErrInvalidCredentials) {
		t.Fatalf("credential failures differ: unknown=%v wrong-password=%v", unknownErr, wrongPasswordErr)
	}
}

func TestRefreshRotationRevokesPreviousAccessSession(t *testing.T) {
	repository := newFakeRepository()
	hash, _ := bcrypt.GenerateFromPassword([]byte("a-long-test-password"), bcrypt.MinCost)
	repository.usersByEmail["owner@lynx.test"] = domain.User{ID: "user-1", Email: "owner@lynx.test", PasswordHash: string(hash), Active: true}
	service := newTestService(t, repository, &fakeLimiter{})

	first, err := service.Login(context.Background(), "owner@lynx.test", "a-long-test-password", "127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Refresh(context.Background(), first.RefreshToken, "127.0.0.1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Authenticate(context.Background(), first.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("previous access token should fail after refresh rotation, got %v", err)
	}
	if _, _, err := service.Authenticate(context.Background(), second.AccessToken); err != nil {
		t.Fatalf("new access token should be valid: %v", err)
	}
	if _, err := service.Refresh(context.Background(), first.RefreshToken, "127.0.0.1", ""); !errors.Is(err, domain.ErrRefreshReuse) {
		t.Fatalf("reused refresh token should be detected, got %v", err)
	}
	if _, _, err := service.Authenticate(context.Background(), second.AccessToken); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("refresh reuse should revoke the token family, got %v", err)
	}
}

func newTestService(t *testing.T, repository *fakeRepository, limiter *fakeLimiter) *Service {
	t.Helper()
	service, err := NewService(repository, limiter, fakeTokens{repository: repository})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	return service
}

type fakeLimiter struct{ blocked bool }

func (l *fakeLimiter) Allow(context.Context, string) (bool, error) { return !l.blocked, nil }
func (l *fakeLimiter) RecordFailure(context.Context, string) error { return nil }
func (l *fakeLimiter) Reset(context.Context, string) error         { return nil }

type fakeTokens struct{ repository *fakeRepository }

func (fakeTokens) Sign(claims domain.AccessClaims, _ time.Time) (string, error) {
	return claims.UserID + ":" + claims.SessionID, nil
}
func (t fakeTokens) Verify(value string) (domain.AccessClaims, error) {
	parts := strings.SplitN(value, ":", 2)
	if len(parts) == 2 {
		session, ok := t.repository.sessions[parts[1]]
		if ok && session.UserID == parts[0] && session.RevokedAt == nil {
			return domain.AccessClaims{UserID: parts[0], SessionID: parts[1]}, nil
		}
	}
	return domain.AccessClaims{}, domain.ErrSessionRevoked
}

type fakeRepository struct {
	usersByEmail map[string]domain.User
	sessions     map[string]domain.Session
	byHash       map[string]string
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{usersByEmail: map[string]domain.User{}, sessions: map[string]domain.Session{}, byHash: map[string]string{}}
}
func (r *fakeRepository) CreateInitialOwner(context.Context, domain.User) error { return nil }
func (r *fakeRepository) FindByEmail(_ context.Context, email string) (domain.User, error) {
	user, ok := r.usersByEmail[email]
	if !ok {
		return domain.User{}, domain.ErrUserNotFound
	}
	return user, nil
}
func (r *fakeRepository) FindByID(context.Context, string) (domain.User, error) {
	return domain.User{}, domain.ErrUserNotFound
}
func (r *fakeRepository) TouchLastLogin(context.Context, string) error { return nil }
func (r *fakeRepository) CreateSession(_ context.Context, session domain.Session) error {
	r.sessions[session.ID] = session
	r.byHash[session.RefreshTokenHash] = session.ID
	return nil
}
func (r *fakeRepository) GetActiveSession(_ context.Context, id string) (domain.Session, domain.User, error) {
	session, ok := r.sessions[id]
	if !ok || session.RevokedAt != nil {
		return domain.Session{}, domain.User{}, domain.ErrSessionRevoked
	}
	for _, user := range r.usersByEmail {
		if user.ID == session.UserID && user.Active {
			return session, user, nil
		}
	}
	return domain.Session{}, domain.User{}, domain.ErrSessionRevoked
}
func (r *fakeRepository) RotateSession(_ context.Context, oldHash string, next domain.Session) error {
	id, ok := r.byHash[oldHash]
	if !ok {
		return domain.ErrInvalidRefresh
	}
	old := r.sessions[id]
	if old.RevokedAt != nil {
		for sessionID, session := range r.sessions {
			if session.FamilyID == old.FamilyID {
				now := time.Now()
				session.RevokedAt = &now
				r.sessions[sessionID] = session
			}
		}
		return domain.ErrRefreshReuse
	}
	now := time.Now()
	old.RevokedAt = &now
	r.sessions[id] = old
	next.UserID = old.UserID
	next.FamilyID = old.FamilyID
	r.sessions[next.ID] = next
	r.byHash[next.RefreshTokenHash] = next.ID
	return nil
}
func (r *fakeRepository) RevokeSession(_ context.Context, userID, id string) error {
	session := r.sessions[id]
	if session.UserID == userID {
		now := time.Now()
		session.RevokedAt = &now
		r.sessions[id] = session
	}
	return nil
}
func (r *fakeRepository) RevokeSessionByRefreshHash(_ context.Context, hash string) error {
	id := r.byHash[hash]
	return r.RevokeSession(context.Background(), r.sessions[id].UserID, id)
}
func (r *fakeRepository) RevokeAllSessions(_ context.Context, userID string) error {
	for id, session := range r.sessions {
		if session.UserID == userID {
			_ = r.RevokeSession(context.Background(), userID, id)
		}
	}
	return nil
}
func (r *fakeRepository) ListSessions(context.Context, string) ([]domain.Session, error) {
	return nil, nil
}

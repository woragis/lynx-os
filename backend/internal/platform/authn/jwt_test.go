package authn

import (
	"strings"
	"testing"
	"time"

	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

func TestJWTManagerSignsAndVerifiesShortLivedAccessClaims(t *testing.T) {
	manager, err := NewJWTManager([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Sign(domain.AccessClaims{UserID: "user-1", SessionID: "session-1"}, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Verify(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "user-1" || claims.SessionID != "session-1" {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

func TestJWTManagerRejectsExpiredAndTamperedTokens(t *testing.T) {
	manager, err := NewJWTManager([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	expired, err := manager.Sign(domain.AccessClaims{UserID: "user-1", SessionID: "session-1"}, time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Verify(expired); err == nil {
		t.Fatal("expired token was accepted")
	}
	if _, err := manager.Verify(expired + "x"); err == nil {
		t.Fatal("tampered token was accepted")
	}
}

func TestJWTManagerRejectsShortSigningKey(t *testing.T) {
	if _, err := NewJWTManager([]byte("too-short")); err == nil {
		t.Fatal("short signing key was accepted")
	}
}

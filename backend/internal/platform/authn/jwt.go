package authn

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	domain "github.com/lynx/lynx-os/backend/internal/domain/identity"
)

type JWTManager struct {
	key []byte
	now func() time.Time
}

type claims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func NewJWTManager(key []byte) (*JWTManager, error) {
	if len(key) < 32 {
		return nil, errors.New("JWT signing key must contain at least 32 bytes")
	}
	return &JWTManager{key: append([]byte(nil), key...), now: time.Now}, nil
}

func (m *JWTManager) Sign(input domain.AccessClaims, expiresAt time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{
		SessionID: input.SessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: input.UserID, Issuer: "lynx-os",
			IssuedAt: jwt.NewNumericDate(m.now()), ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	})
	return token.SignedString(m.key)
}

func (m *JWTManager) Verify(raw string) (domain.AccessClaims, error) {
	parsed, err := jwt.ParseWithClaims(raw, &claims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected JWT signing method")
		}
		return m.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("lynx-os"), jwt.WithTimeFunc(m.now))
	if err != nil || parsed == nil || !parsed.Valid {
		return domain.AccessClaims{}, errors.New("access token is invalid")
	}
	value, ok := parsed.Claims.(*claims)
	if !ok || value.Subject == "" || value.SessionID == "" {
		return domain.AccessClaims{}, errors.New("access token claims are invalid")
	}
	return domain.AccessClaims{UserID: value.Subject, SessionID: value.SessionID}, nil
}

var _ domain.AccessTokenManager = (*JWTManager)(nil)

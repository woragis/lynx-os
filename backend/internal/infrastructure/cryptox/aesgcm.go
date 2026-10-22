package cryptox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"

	domain "github.com/lynx/lynx-os/backend/internal/domain/integration"
)

type AESGCM struct{ aead cipher.AEAD }

func NewAESGCM(key []byte) (*AESGCM, error) {
	if len(key) != 32 {
		return nil, errors.New("encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AESGCM{aead: aead}, nil
}

func (c *AESGCM) Encrypt(plaintext []byte) (domain.SealedSecret, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return domain.SealedSecret{}, err
	}
	sealed := c.aead.Seal(nil, nonce, plaintext, nil)
	cut := len(sealed) - c.aead.Overhead()
	return domain.SealedSecret{
		Ciphertext: base64.RawStdEncoding.EncodeToString(sealed[:cut]),
		IV:         base64.RawStdEncoding.EncodeToString(nonce),
		AuthTag:    base64.RawStdEncoding.EncodeToString(sealed[cut:]),
	}, nil
}

func (c *AESGCM) Decrypt(secret domain.SealedSecret) ([]byte, error) {
	ciphertext, err := base64.RawStdEncoding.DecodeString(secret.Ciphertext)
	if err != nil {
		return nil, errors.New("encrypted secret is malformed")
	}
	nonce, err := base64.RawStdEncoding.DecodeString(secret.IV)
	if err != nil || len(nonce) != c.aead.NonceSize() {
		return nil, errors.New("encrypted secret nonce is malformed")
	}
	tag, err := base64.RawStdEncoding.DecodeString(secret.AuthTag)
	if err != nil || len(tag) != c.aead.Overhead() {
		return nil, errors.New("encrypted secret tag is malformed")
	}
	return c.aead.Open(nil, nonce, append(ciphertext, tag...), nil)
}

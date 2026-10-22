package cryptox

import (
	"bytes"
	"strings"
	"testing"
)

func TestAESGCMEncryptDecrypt(t *testing.T) {
	cipher, err := NewAESGCM(bytes.Repeat([]byte("k"), 32))
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("sensitive integration api key")
	sealed, err := cipher.Encrypt(plain)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := cipher.Decrypt(sealed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, plain) {
		t.Fatalf("decrypted plaintext mismatch: %q", decoded)
	}
	if sealed.Ciphertext == string(plain) || sealed.IV == "" || sealed.AuthTag == "" {
		t.Fatal("secret was not sealed with a nonce and authentication tag")
	}
}

func TestAESGCMRejectsTamperingAndWrongKey(t *testing.T) {
	first, _ := NewAESGCM(bytes.Repeat([]byte("a"), 32))
	second, _ := NewAESGCM(bytes.Repeat([]byte("b"), 32))
	sealed, err := first.Encrypt([]byte("private"))
	if err != nil {
		t.Fatal(err)
	}
	sealed.AuthTag = strings.Repeat("A", len(sealed.AuthTag))
	if _, err := first.Decrypt(sealed); err == nil {
		t.Fatal("modified authentication tag was accepted")
	}
	sealed, _ = first.Encrypt([]byte("private"))
	if _, err := second.Decrypt(sealed); err == nil {
		t.Fatal("ciphertext encrypted with another key was accepted")
	}
}

func TestAESGCMRequiresA256Key(t *testing.T) {
	if _, err := NewAESGCM([]byte("short")); err == nil {
		t.Fatal("invalid key size was accepted")
	}
}

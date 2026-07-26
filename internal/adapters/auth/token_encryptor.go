package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// TokenEncryptor protects refresh-token successors persisted for the bounded
// D-2 replay grace window using AES-256-GCM.
type TokenEncryptor struct {
	aead cipher.AEAD
}

// NewTokenEncryptor constructs an AES-256-GCM encryptor from exactly 32 key
// bytes. The key must come from ENCRYPTION_KEY and must not be logged.
func NewTokenEncryptor(key []byte) (*TokenEncryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM: %w", err)
	}
	return &TokenEncryptor{aead: aead}, nil
}

// NewTokenEncryptorFromBase64Key decodes the base64 ENCRYPTION_KEY value.
func NewTokenEncryptorFromBase64Key(encoded string) (*TokenEncryptor, error) {
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode encryption key: %w", err)
	}
	return NewTokenEncryptor(key)
}

func (e *TokenEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	if e == nil || e.aead == nil {
		return nil, fmt.Errorf("token encryptor is not configured")
	}
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate AES-GCM nonce: %w", err)
	}
	return e.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (e *TokenEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if e == nil || e.aead == nil {
		return nil, fmt.Errorf("token encryptor is not configured")
	}
	if len(ciphertext) < e.aead.NonceSize() {
		return nil, fmt.Errorf("encrypted token is malformed")
	}
	nonce, ciphertext := ciphertext[:e.aead.NonceSize()], ciphertext[e.aead.NonceSize():]
	plaintext, err := e.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt token: %w", err)
	}
	return plaintext, nil
}

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/google/uuid"
)

// TokenConfig holds token generation settings.
type TokenConfig struct {
	// SecretLen in bytes (must be ≥32 for sufficient entropy)
	SecretLen int
}

// DefaultTokenConfig returns recommended settings (32 bytes = 256 bits entropy).
func DefaultTokenConfig() TokenConfig {
	return TokenConfig{
		SecretLen: 32,
	}
}

// SecretGenerator generates tokens in format rp_at_<id>.<secret> (access) or
// rp_rt_<id>.<secret> (refresh). Implements ports.SecretGenerator.
type SecretGenerator struct {
	cfg TokenConfig
}

// NewSecretGenerator constructs a token generator with the given config.
func NewSecretGenerator(cfg TokenConfig) *SecretGenerator {
	if cfg.SecretLen < 32 {
		// Force at least 32 bytes (256 bits) of entropy
		cfg.SecretLen = 32
	}
	return &SecretGenerator{cfg: cfg}
}

// GenerateAccessToken generates an access token in format rp_at_<id>.<secret>.
func (g *SecretGenerator) GenerateAccessToken() (string, error) {
	return g.generateToken("rp_at")
}

// GenerateRefreshToken generates a refresh token in format rp_rt_<id>.<secret>.
func (g *SecretGenerator) GenerateRefreshToken() (string, error) {
	return g.generateToken("rp_rt")
}

// generateToken creates a token with the given prefix and format <prefix>_<id>.<secret>.
func (g *SecretGenerator) generateToken(prefix string) (string, error) {
	// Generate a UUID for the public ID part
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generating UUID: %w", err)
	}

	// Generate random secret using crypto/rand (≥32 bytes)
	secretBytes := make([]byte, g.cfg.SecretLen)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", fmt.Errorf("generating secret: %w", err)
	}

	// Encode secret in base64url (no padding)
	secretB64 := base64.URLEncoding.EncodeToString(secretBytes)

	// Format: <prefix>_<id>.<secret>
	token := fmt.Sprintf("%s_%s.%s", prefix, id.String(), secretB64)
	return token, nil
}

// GenerateToken generates a token in the default access token format.
// Implements ports.SecretGenerator.
func (g *SecretGenerator) GenerateToken() (string, error) {
	return g.GenerateAccessToken()
}

// HashTokenSecret hashes a token secret (or full token) for database storage.
// This should hash only the secret part, not the full token (which is public).
// Returns SHA-256 hash as []byte for storage in the DB as bytea.
func HashTokenSecret(secret string) []byte {
	hash := sha256.Sum256([]byte(secret))
	return hash[:]
}

// ExtractSecretFromToken extracts the secret part from a token string.
// Token format: rp_at_<id>.<secret> or rp_rt_<id>.<secret>
// Returns the secret part (after the last dot).
func ExtractSecretFromToken(token string) (string, error) {
	lastDot := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			lastDot = i
			break
		}
	}
	if lastDot == -1 {
		return "", fmt.Errorf("invalid token format: no dot separator found")
	}
	return token[lastDot+1:], nil
}

// ExtractIDFromToken extracts the ID part from a token string.
// Token format: rp_at_<id>.<secret> or rp_rt_<id>.<secret>
// Returns the ID part (between the last _ before the . and the .).
func ExtractIDFromToken(token string) (string, error) {
	// Find the position of the dot
	dotPos := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			dotPos = i
			break
		}
	}
	if dotPos == -1 {
		return "", fmt.Errorf("invalid token format: no dot separator found")
	}

	// Find the last underscore before the dot
	underscorePos := -1
	for i := dotPos - 1; i >= 0; i-- {
		if token[i] == '_' {
			underscorePos = i
			break
		}
	}
	if underscorePos == -1 {
		return "", fmt.Errorf("invalid token format: no underscore separator found")
	}

	return token[underscorePos+1 : dotPos], nil
}

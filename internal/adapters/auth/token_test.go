package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
)

// TestSecretGeneratorAccessToken verifies access token format and structure.
func TestSecretGeneratorAccessToken(t *testing.T) {
	g := NewSecretGenerator(DefaultTokenConfig())

	token, err := g.GenerateAccessToken()
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	// Token should start with "rp_at_"
	if !strings.HasPrefix(token, "rp_at_") {
		t.Errorf("Token should start with 'rp_at_', got %q", token)
	}

	// Should have exactly one dot
	dotCount := strings.Count(token, ".")
	if dotCount != 1 {
		t.Errorf("Token should have exactly 1 dot, got %d", dotCount)
	}

	// Extract ID and secret
	id, err := ExtractIDFromToken(token)
	if err != nil {
		t.Errorf("ExtractIDFromToken failed: %v", err)
	}
	if id == "" {
		t.Error("Extracted ID should not be empty")
	}

	secret, err := ExtractSecretFromToken(token)
	if err != nil {
		t.Errorf("ExtractSecretFromToken failed: %v", err)
	}
	if secret == "" {
		t.Error("Extracted secret should not be empty")
	}

	// Secret should be base64url-encoded
	_, err = base64.URLEncoding.DecodeString(secret)
	if err != nil {
		t.Errorf("Secret is not valid base64url: %v", err)
	}
}

// TestSecretGeneratorRefreshToken verifies refresh token format and structure.
func TestSecretGeneratorRefreshToken(t *testing.T) {
	g := NewSecretGenerator(DefaultTokenConfig())

	token, err := g.GenerateRefreshToken()
	if err != nil {
		t.Fatalf("GenerateRefreshToken failed: %v", err)
	}

	// Token should start with "rp_rt_"
	if !strings.HasPrefix(token, "rp_rt_") {
		t.Errorf("Token should start with 'rp_rt_', got %q", token)
	}

	// Should have exactly one dot
	dotCount := strings.Count(token, ".")
	if dotCount != 1 {
		t.Errorf("Token should have exactly 1 dot, got %d", dotCount)
	}

	// Extract ID and secret
	id, err := ExtractIDFromToken(token)
	if err != nil {
		t.Errorf("ExtractIDFromToken failed: %v", err)
	}
	if id == "" {
		t.Error("Extracted ID should not be empty")
	}

	secret, err := ExtractSecretFromToken(token)
	if err != nil {
		t.Errorf("ExtractSecretFromToken failed: %v", err)
	}
	if secret == "" {
		t.Error("Extracted secret should not be empty")
	}
}

// TestSecretGeneratorGenerateToken verifies the default GenerateToken implementation.
func TestSecretGeneratorGenerateToken(t *testing.T) {
	g := NewSecretGenerator(DefaultTokenConfig())

	token, err := g.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	// Should be an access token (default)
	if !strings.HasPrefix(token, "rp_at_") {
		t.Errorf("Default token should be access token, got %q", token)
	}
}

// TestSecretGeneratorCollisionResistance verifies that 10,000 tokens are unique.
func TestSecretGeneratorCollisionResistance(t *testing.T) {
	g := NewSecretGenerator(DefaultTokenConfig())
	tokens := make(map[string]bool)

	for i := 0; i < 10000; i++ {
		token, err := g.GenerateAccessToken()
		if err != nil {
			t.Fatalf("GenerateAccessToken %d failed: %v", i, err)
		}
		if tokens[token] {
			t.Fatalf("Collision detected at iteration %d: %q", i, token)
		}
		tokens[token] = true
	}

	if len(tokens) != 10000 {
		t.Errorf("Expected 10000 unique tokens, got %d", len(tokens))
	}
}

// TestHashTokenSecret verifies SHA-256 hashing for token storage.
func TestHashTokenSecret(t *testing.T) {
	secret1 := "my-secret-token-data"
	hash1 := HashTokenSecret(secret1)

	// Should return 32 bytes (SHA-256 output)
	if len(hash1) != 32 {
		t.Errorf("Hash should be 32 bytes, got %d", len(hash1))
	}

	// Same secret should produce same hash
	hash1b := HashTokenSecret(secret1)
	if !bytesEqual(hash1, hash1b) {
		t.Error("Same secret should produce same hash")
	}

	// Different secret should produce different hash
	secret2 := "different-secret"
	hash2 := HashTokenSecret(secret2)
	if bytesEqual(hash1, hash2) {
		t.Error("Different secrets should produce different hashes")
	}

	// Should be deterministic (hash it again)
	hash1c := HashTokenSecret(secret1)
	if !bytesEqual(hash1, hash1c) {
		t.Error("Hash should be deterministic")
	}
}

// TestExtractSecretFromToken tests secret extraction from various token formats.
func TestExtractSecretFromToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantErr bool
		wantSec string
	}{
		{
			name:    "valid access token",
			token:   "rp_at_550e8400-e29b-41d4-a716-446655440000.AAAAAAAAAAAAAAAAAAAAAA",
			wantErr: false,
			wantSec: "AAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			name:    "valid refresh token",
			token:   "rp_rt_550e8400-e29b-41d4-a716-446655440000.AAAAAAAAAAAAAAAAAAAAAA",
			wantErr: false,
			wantSec: "AAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			name:    "no dot separator",
			token:   "rp_at_550e8400-e29b-41d4-a716-446655440000",
			wantErr: true,
		},
		{
			name:    "empty string",
			token:   "",
			wantErr: true,
		},
		{
			name:    "multiple dots",
			token:   "rp_at_550e8400.e29b.41d4.a716.AAAAAAAAAAAAAAAAAAAAAA",
			wantErr: false,
			wantSec: "AAAAAAAAAAAAAAAAAAAAAA", // Extracts last part after dot
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret, err := ExtractSecretFromToken(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Error("ExtractSecretFromToken should have failed")
				}
			} else {
				if err != nil {
					t.Errorf("ExtractSecretFromToken failed: %v", err)
				}
				if secret != tt.wantSec {
					t.Errorf("Expected secret %q, got %q", tt.wantSec, secret)
				}
			}
		})
	}
}

// TestExtractIDFromToken tests ID extraction from various token formats.
func TestExtractIDFromToken(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		wantID  string
		wantErr bool
	}{
		{
			name:    "valid access token",
			token:   "rp_at_550e8400-e29b-41d4-a716-446655440000.AAAAAAAAAAAAAAAAAAAAAA",
			wantID:  "550e8400-e29b-41d4-a716-446655440000",
			wantErr: false,
		},
		{
			name:    "valid refresh token",
			token:   "rp_rt_abcd1234-5678-90ab-cdef-1234567890ab.AAAAAAAAAAAAAAAAAAAAAA",
			wantID:  "abcd1234-5678-90ab-cdef-1234567890ab",
			wantErr: false,
		},
		{
			name:    "no dot separator",
			token:   "rp_at_550e8400-e29b-41d4-a716-446655440000",
			wantErr: true,
		},
		{
			name:    "empty string",
			token:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := ExtractIDFromToken(tt.token)
			if tt.wantErr {
				if err == nil {
					t.Error("ExtractIDFromToken should have failed")
				}
			} else {
				if err != nil {
					t.Errorf("ExtractIDFromToken failed: %v", err)
				}
				if id != tt.wantID {
					t.Errorf("Expected ID %q, got %q", tt.wantID, id)
				}
			}
		})
	}
}

// TestSecretGeneratorEntropyMinimum verifies that at least 32 bytes of entropy are used.
func TestSecretGeneratorEntropyMinimum(t *testing.T) {
	// Create a hasher with less than 32 bytes (should be enforced to 32)
	cfg := TokenConfig{SecretLen: 16}
	g := NewSecretGenerator(cfg)

	token, err := g.GenerateAccessToken()
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}

	secret, err := ExtractSecretFromToken(token)
	if err != nil {
		t.Fatalf("ExtractSecretFromToken failed: %v", err)
	}

	// Decode base64url to get raw bytes
	secretBytes, err := base64.URLEncoding.DecodeString(secret)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}

	// Should be at least 32 bytes (minimum enforced)
	if len(secretBytes) < 32 {
		t.Errorf("Secret should be at least 32 bytes, got %d", len(secretBytes))
	}
}

// TestSecretGeneratorFormat verifies that tokens have the exact expected format.
func TestSecretGeneratorFormat(t *testing.T) {
	g := NewSecretGenerator(DefaultTokenConfig())

	tests := []struct {
		name     string
		generate func() (string, error)
		prefix   string
	}{
		{
			name:     "access token",
			generate: g.GenerateAccessToken,
			prefix:   "rp_at_",
		},
		{
			name:     "refresh token",
			generate: g.GenerateRefreshToken,
			prefix:   "rp_rt_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := tt.generate()
			if err != nil {
				t.Fatalf("Generate failed: %v", err)
			}

			// Must start with correct prefix
			if !strings.HasPrefix(token, tt.prefix) {
				t.Errorf("Token should start with %q, got %q", tt.prefix, token[:len(tt.prefix)])
			}

			// Must have exactly one dot
			if strings.Count(token, ".") != 1 {
				t.Errorf("Token should have exactly 1 dot: %s", token)
			}

			// Must be at least prefix_id.secret format
			if len(token) < len(tt.prefix)+36+2 { // prefix + uuid + dot + minimal secret
				t.Errorf("Token too short: %s", token)
			}
		})
	}
}

// TestSecretGeneratorDifferentInstances verifies independence of generator instances.
func TestSecretGeneratorDifferentInstances(t *testing.T) {
	g1 := NewSecretGenerator(DefaultTokenConfig())
	g2 := NewSecretGenerator(DefaultTokenConfig())

	token1, err := g1.GenerateAccessToken()
	if err != nil {
		t.Fatalf("g1.GenerateAccessToken failed: %v", err)
	}

	token2, err := g2.GenerateAccessToken()
	if err != nil {
		t.Fatalf("g2.GenerateAccessToken failed: %v", err)
	}

	// Tokens from different instances should be different
	if token1 == token2 {
		t.Error("Tokens from different instances should be different")
	}
}

// TestHashTokenSecretDeterministic verifies SHA-256 hashing is deterministic.
func TestHashTokenSecretDeterministic(t *testing.T) {
	secret := "test-secret-for-hashing"

	// Hash it multiple times
	hashes := [][]byte{
		HashTokenSecret(secret),
		HashTokenSecret(secret),
		HashTokenSecret(secret),
	}

	// All should be equal
	for i := 1; i < len(hashes); i++ {
		if !bytesEqual(hashes[0], hashes[i]) {
			t.Errorf("Hash %d differs from hash 0", i)
		}
	}
}

// TestHashTokenSecretMatchesSHA256 verifies that HashTokenSecret uses SHA-256.
func TestHashTokenSecretMatchesSHA256(t *testing.T) {
	secret := "my-token-secret"

	hash := HashTokenSecret(secret)

	// Manually compute SHA-256
	expectedHash := sha256.Sum256([]byte(secret))

	if !bytesEqual(hash, expectedHash[:]) {
		t.Errorf("Hash does not match expected SHA-256:\ngot: %x\nexp: %x", hash, expectedHash)
	}
}

// TestExtractionsAreSymmetric tests that ID and secret extraction work together.
func TestExtractionsAreSymmetric(t *testing.T) {
	g := NewSecretGenerator(DefaultTokenConfig())

	for i := 0; i < 100; i++ {
		token, err := g.GenerateAccessToken()
		if err != nil {
			t.Fatalf("GenerateAccessToken failed: %v", err)
		}

		id, err := ExtractIDFromToken(token)
		if err != nil {
			t.Fatalf("ExtractIDFromToken failed: %v", err)
		}

		secret, err := ExtractSecretFromToken(token)
		if err != nil {
			t.Fatalf("ExtractSecretFromToken failed: %v", err)
		}

		// Reconstruct the token manually
		reconstructed := fmt.Sprintf("rp_at_%s.%s", id, secret)
		if reconstructed != token {
			t.Errorf("Reconstructed token %q != original %q", reconstructed, token)
		}
	}
}

// Helper function to compare byte slices.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

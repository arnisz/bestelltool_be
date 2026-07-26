// Package auth provides authentication adapters.
// All cryptography lives here - domain and application code only see ports.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Config holds the parameters for Argon2id hashing.
type Argon2Config struct {
	// Memory in KiB (OWASP minimum: 19456 KiB ≈ 19 MiB)
	Memory uint32
	// Time (iterations): OWASP minimum 2
	Time uint32
	// Parallelism (OWASP minimum 1)
	Parallelism uint8
	// SaltLen in bytes (typically 16)
	SaltLen uint32
}

// DefaultArgon2Config returns OWASP minimum parameters.
func DefaultArgon2Config() Argon2Config {
	return Argon2Config{
		Memory:      19456,
		Time:        2,
		Parallelism: 1,
		SaltLen:     16,
	}
}

// Argon2Hasher implements ports.PasswordHasher using Argon2id with PHC encoding.
type Argon2Hasher struct {
	cfg Argon2Config
}

// NewArgon2Hasher constructs a hasher with the given config.
func NewArgon2Hasher(cfg Argon2Config) *Argon2Hasher {
	return &Argon2Hasher{cfg: cfg}
}

// Hash generates a random salt and returns a PHC-formatted Argon2id hash.
// Format: $argon2id$v=19$m=...,t=...,p=...$<base64-salt>$<base64-hash>
func (h *Argon2Hasher) Hash(password string) (string, error) {
	salt := make([]byte, h.cfg.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		h.cfg.Time,
		h.cfg.Memory,
		h.cfg.Parallelism,
		32, // 32-byte output
	)

	// PHC format: $argon2id$v=19$m=<memory>,t=<time>,p=<parallelism>$<base64-salt>$<base64-hash>
	phcHash := fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		h.cfg.Memory,
		h.cfg.Time,
		h.cfg.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return phcHash, nil
}

// Verify checks whether password matches the PHC-encoded hash and reports
// whether the hash parameters are outdated (SEC-02).
// Returns (ok, needsRehash, err).
func (h *Argon2Hasher) Verify(password, hash string) (ok bool, needsRehash bool, err error) {
	params, salt, hashBytes, err := h.parsePHC(hash)
	if err != nil {
		return false, false, err
	}

	// Recompute the hash with stored parameters
	computed := argon2.IDKey(
		[]byte(password),
		salt,
		params.Time,
		params.Memory,
		params.Parallelism,
		32,
	)

	// Constant-time comparison (SEC-02)
	ok = subtle.ConstantTimeCompare(computed, hashBytes) == 1

	// Check if parameters are outdated (needsRehash for transparent upgrade)
	needsRehash = params.Memory != h.cfg.Memory ||
		params.Time != h.cfg.Time ||
		params.Parallelism != h.cfg.Parallelism

	return ok, needsRehash, nil
}

// phcParams holds parsed Argon2id parameters from a PHC string.
type phcParams struct {
	Memory      uint32
	Time        uint32
	Parallelism uint8
}

// parsePHC parses a PHC-formatted Argon2id hash and returns parameters, salt, and hash bytes.
func (h *Argon2Hasher) parsePHC(phcHash string) (phcParams, []byte, []byte, error) {
	// Format: $argon2id$v=19$m=<m>,t=<t>,p=<p>$<base64-salt>$<base64-hash>
	parts := strings.Split(phcHash, "$")
	if len(parts) != 6 {
		return phcParams{}, nil, nil, fmt.Errorf("invalid PHC format: expected 6 parts, got %d", len(parts))
	}

	if parts[0] != "" || parts[1] != "argon2id" || parts[2] != "v=19" {
		return phcParams{}, nil, nil, fmt.Errorf("invalid PHC format: wrong header or version")
	}

	// Parse m,t,p from parts[3]
	params, err := h.parseParams(parts[3])
	if err != nil {
		return phcParams{}, nil, nil, err
	}

	// Decode base64 salt and hash
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("invalid base64 salt: %w", err)
	}

	hashBytes, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return phcParams{}, nil, nil, fmt.Errorf("invalid base64 hash: %w", err)
	}

	return params, salt, hashBytes, nil
}

// parseParams parses "m=<m>,t=<t>,p=<p>" format.
func (h *Argon2Hasher) parseParams(paramStr string) (phcParams, error) {
	parts := strings.Split(paramStr, ",")
	if len(parts) != 3 {
		return phcParams{}, fmt.Errorf("invalid parameters: expected 3 fields, got %d", len(parts))
	}

	var params phcParams
	var err error

	// Parse m
	if !strings.HasPrefix(parts[0], "m=") {
		return phcParams{}, fmt.Errorf("invalid parameter: expected m=, got %s", parts[0])
	}
	mVal, err := strconv.ParseUint(parts[0][2:], 10, 32)
	if err != nil {
		return phcParams{}, fmt.Errorf("invalid memory value: %w", err)
	}
	params.Memory = uint32(mVal)

	// Parse t
	if !strings.HasPrefix(parts[1], "t=") {
		return phcParams{}, fmt.Errorf("invalid parameter: expected t=, got %s", parts[1])
	}
	tVal, err := strconv.ParseUint(parts[1][2:], 10, 32)
	if err != nil {
		return phcParams{}, fmt.Errorf("invalid time value: %w", err)
	}
	params.Time = uint32(tVal)

	// Parse p
	if !strings.HasPrefix(parts[2], "p=") {
		return phcParams{}, fmt.Errorf("invalid parameter: expected p=, got %s", parts[2])
	}
	pVal, err := strconv.ParseUint(parts[2][2:], 10, 8)
	if err != nil {
		return phcParams{}, fmt.Errorf("invalid parallelism value: %w", err)
	}
	params.Parallelism = uint8(pVal)

	return params, nil
}

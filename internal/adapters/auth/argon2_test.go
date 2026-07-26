package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestArgon2Hash(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Config())

	password := "correct-password-123"
	hash1, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	// Verify the hash starts with the correct PHC header
	if !strings.HasPrefix(hash1, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("Hash has wrong PHC format: %s", hash1[:50])
	}

	// Two hashes of the same password should be different (different salts)
	hash2, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Second hash failed: %v", err)
	}
	if hash1 == hash2 {
		t.Error("Two hashes should be different (random salt)")
	}
}

func TestArgon2Verify(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Config())
	password := "my-secret-password"

	hash, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	// Correct password should verify
	ok, needsRehash, err := h.Verify(password, hash)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if !ok {
		t.Error("Correct password should verify")
	}
	if needsRehash {
		t.Error("Correct parameters should not need rehash")
	}

	// Wrong password should not verify
	ok, _, err = h.Verify("wrong-password", hash)
	if err != nil {
		t.Fatalf("Verify with wrong password failed: %v", err)
	}
	if ok {
		t.Error("Wrong password should not verify")
	}

	// Empty password should not verify
	ok, _, err = h.Verify("", hash)
	if err != nil {
		t.Fatalf("Verify with empty password failed: %v", err)
	}
	if ok {
		t.Error("Empty password should not verify")
	}
}

func TestArgon2NeedsRehash(t *testing.T) {
	oldCfg := DefaultArgon2Config()
	oldHash, err := NewArgon2Hasher(oldCfg).Hash("password")
	if err != nil {
		t.Fatalf("Old hash failed: %v", err)
	}

	// Verify with same config should not need rehash
	h := NewArgon2Hasher(oldCfg)
	_, needsRehash, err := h.Verify("password", oldHash)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if needsRehash {
		t.Error("Same parameters should not need rehash")
	}

	// Verify with higher memory should need rehash
	newCfg := DefaultArgon2Config()
	newCfg.Memory = 32768 // Increased
	h = NewArgon2Hasher(newCfg)
	_, needsRehash, err = h.Verify("password", oldHash)
	if err != nil {
		t.Fatalf("Verify with new config failed: %v", err)
	}
	if !needsRehash {
		t.Error("Higher memory parameters should need rehash")
	}

	// Verify with higher time should need rehash
	newCfg = DefaultArgon2Config()
	newCfg.Time = 3
	h = NewArgon2Hasher(newCfg)
	_, needsRehash, err = h.Verify("password", oldHash)
	if err != nil {
		t.Fatalf("Verify with new time failed: %v", err)
	}
	if !needsRehash {
		t.Error("Higher time parameters should need rehash")
	}

	// Verify with higher parallelism should need rehash
	newCfg = DefaultArgon2Config()
	newCfg.Parallelism = 2
	h = NewArgon2Hasher(newCfg)
	_, needsRehash, err = h.Verify("password", oldHash)
	if err != nil {
		t.Fatalf("Verify with new parallelism failed: %v", err)
	}
	if !needsRehash {
		t.Error("Higher parallelism should need rehash")
	}
}

// TestArgon2MalformedPHC checks that invalid PHC strings are rejected.
func TestArgon2MalformedPHC(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Config())

	tests := []struct {
		name   string
		hash   string
		wantOK bool
	}{
		{"empty string", "", false},
		{"too few parts", "$argon2id$v=19$m=19456", false},
		{"wrong algorithm", "$argon2i$v=19$m=19456,t=2,p=1$salt$hash", false},
		{"wrong version", "$argon2id$v=18$m=19456,t=2,p=1$salt$hash", false},
		{"missing params", "$argon2id$v=19$$salt$hash", false},
		{"malformed params", "$argon2id$v=19$m=abc,t=2,p=1$" + base64.RawStdEncoding.EncodeToString([]byte("salt")) + "$" + base64.RawStdEncoding.EncodeToString([]byte("hash")), false},
		{"invalid base64 salt", "$argon2id$v=19$m=19456,t=2,p=1$!!!$" + base64.RawStdEncoding.EncodeToString([]byte("hash")), false},
		{"invalid base64 hash", "$argon2id$v=19$m=19456,t=2,p=1$" + base64.RawStdEncoding.EncodeToString([]byte("salt")) + "$!!!", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := h.Verify("password", tt.hash)
			if tt.wantOK {
				if err != nil {
					t.Errorf("Verify failed: %v", err)
				}
			} else {
				if err == nil {
					t.Error("Verify should have failed for malformed hash")
				}
			}
		})
	}
}

// TestArgon2ConstantTimeComparison verifies that password comparison is constant-time.
// (This test can't truly prove constant-time behavior, but it at least verifies
// that wrong passwords don't verify.)
func TestArgon2ConstantTimeComparison(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Config())
	password := "correct-password"

	hash, err := h.Hash(password)
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	// Verify with many wrong passwords to ensure none match
	wrongPasswords := []string{
		"correct-passwor",   // Missing last char
		"correct-password-", // Extra char
		"",                  // Empty
		"a",                 // Single char
		"CORRECT-PASSWORD",  // Different case
		"123",
		"correct password", // Space instead of dash
	}

	for _, wrong := range wrongPasswords {
		ok, _, err := h.Verify(wrong, hash)
		if err != nil {
			t.Errorf("Verify failed for %q: %v", wrong, err)
		}
		if ok {
			t.Errorf("Wrong password %q should not verify", wrong)
		}
	}
}

// TestArgon2ConfigurableParameters verifies that different configs produce different hashes.
func TestArgon2ConfigurableParameters(t *testing.T) {
	password := "test-password"

	cfg1 := DefaultArgon2Config()
	hash1, err := NewArgon2Hasher(cfg1).Hash(password)
	if err != nil {
		t.Fatalf("Hash1 failed: %v", err)
	}

	// Same config should verify
	cfg2 := DefaultArgon2Config()
	ok, needsRehash, err := NewArgon2Hasher(cfg2).Verify(password, hash1)
	if err != nil || !ok || needsRehash {
		t.Error("Same config should verify without rehash")
	}

	// Different memory should be detected
	cfg3 := DefaultArgon2Config()
	cfg3.Memory = 32768
	_, needsRehash, _ = NewArgon2Hasher(cfg3).Verify(password, hash1)
	if !needsRehash {
		t.Error("Different memory should trigger rehash")
	}

	// Different time should be detected
	cfg4 := DefaultArgon2Config()
	cfg4.Time = 3
	_, needsRehash, _ = NewArgon2Hasher(cfg4).Verify(password, hash1)
	if !needsRehash {
		t.Error("Different time should trigger rehash")
	}

	// Different parallelism should be detected
	cfg5 := DefaultArgon2Config()
	cfg5.Parallelism = 4
	_, needsRehash, _ = NewArgon2Hasher(cfg5).Verify(password, hash1)
	if !needsRehash {
		t.Error("Different parallelism should trigger rehash")
	}
}

// TestArgon2SaltVariation verifies that different invocations produce different salts.
func TestArgon2SaltVariation(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Config())
	password := "same-password"

	hashes := make(map[string]bool)
	for i := 0; i < 5; i++ {
		hash, err := h.Hash(password)
		if err != nil {
			t.Fatalf("Hash %d failed: %v", i, err)
		}
		if hashes[hash] {
			t.Errorf("Duplicate hash detected (iteration %d)", i)
		}
		hashes[hash] = true
	}

	if len(hashes) != 5 {
		t.Errorf("Expected 5 unique hashes, got %d", len(hashes))
	}
}

// TestArgon2PHCFormat verifies the exact PHC format.
func TestArgon2PHCFormat(t *testing.T) {
	h := NewArgon2Hasher(DefaultArgon2Config())
	hash, err := h.Hash("test")
	if err != nil {
		t.Fatalf("Hash failed: %v", err)
	}

	// Expected format: $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("Expected 6 parts, got %d", len(parts))
	}

	if parts[0] != "" {
		t.Errorf("Part 0 should be empty, got %q", parts[0])
	}
	if parts[1] != "argon2id" {
		t.Errorf("Part 1 should be 'argon2id', got %q", parts[1])
	}
	if parts[2] != "v=19" {
		t.Errorf("Part 2 should be 'v=19', got %q", parts[2])
	}
	if parts[3] != "m=19456,t=2,p=1" {
		t.Errorf("Part 3 should be 'm=19456,t=2,p=1', got %q", parts[3])
	}

	// Salt and hash parts should be valid base64
	_, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		t.Errorf("Part 4 (salt) is not valid base64: %v", err)
	}
	_, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		t.Errorf("Part 5 (hash) is not valid base64: %v", err)
	}
}

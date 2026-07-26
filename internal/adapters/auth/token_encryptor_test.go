package auth

import "testing"

func TestTokenEncryptorRoundTripAndRejectsTampering(t *testing.T) {
	encryptor, err := NewTokenEncryptor(make([]byte, 32))
	if err != nil {
		t.Fatalf("NewTokenEncryptor() error = %v", err)
	}
	ciphertext, err := encryptor.Encrypt([]byte("rp_rt_id.secret"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	plaintext, err := encryptor.Decrypt(ciphertext)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(plaintext) != "rp_rt_id.secret" {
		t.Fatalf("plaintext = %q", plaintext)
	}
	ciphertext[len(ciphertext)-1] ^= 1
	if _, err := encryptor.Decrypt(ciphertext); err == nil {
		t.Fatal("Decrypt() accepted altered ciphertext")
	}
}

package accounts

import (
	"encoding/base64"
	"strings"
	"testing"
)

const testKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestNewAccountNormalizesAndValidates(t *testing.T) {
	acc, err := NewAccount("  User@Example.COM ", "test-password", true)
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if acc.Email != "user@example.com" {
		t.Errorf("Email = %q, want normalized lowercase/trimmed", acc.Email)
	}
	if !acc.Active {
		t.Error("Active = false, want true")
	}

	if _, err := NewAccount("not-an-email", "x", true); err == nil {
		t.Error("expected error for invalid email")
	}
	if _, err := NewAccount("a@b.cl", "", true); err == nil {
		t.Error("expected error for empty password")
	}
}

func TestPasswordEncryptionRoundTrip(t *testing.T) {
	t.Setenv(EncryptionKeyEnv, testKey)
	const email = "user@example.com"
	plain := "s3cret..#áé"

	encrypted, err := EncryptPassword(email, plain)
	if err != nil {
		t.Fatalf("EncryptPassword: %v", err)
	}
	if strings.Contains(encrypted, plain) {
		t.Error("ciphertext contains the plaintext password")
	}

	got, err := DecryptPassword(email, encrypted)
	if err != nil {
		t.Fatalf("DecryptPassword: %v", err)
	}
	if got != plain {
		t.Errorf("round-trip = %q, want %q", got, plain)
	}

	again, err := EncryptPassword(email, plain)
	if err != nil {
		t.Fatalf("EncryptPassword: %v", err)
	}
	if again == encrypted {
		t.Error("two encryptions produced the same ciphertext, nonce is not random")
	}

	if _, err := DecryptPassword("other@example.com", encrypted); err == nil {
		t.Error("expected failure decrypting with a different account email")
	}
}

func TestPasswordEncryptionRejectsBadKey(t *testing.T) {
	t.Setenv(EncryptionKeyEnv, "")
	if _, err := EncryptPassword("user@example.com", "x"); err == nil {
		t.Error("expected error when the key is unset")
	}
	t.Setenv(EncryptionKeyEnv, base64.StdEncoding.EncodeToString([]byte("too-short")))
	if _, err := EncryptPassword("user@example.com", "x"); err == nil {
		t.Error("expected error when the key is not 32 bytes")
	}
}

func TestRedactDropsSecret(t *testing.T) {
	acc := Account{Email: "user@example.com", Password: "test-password", JobID: "2690", Active: true}
	view := acc.Redact()
	if view.Email != acc.Email || view.JobID != acc.JobID || view.Active != acc.Active {
		t.Errorf("Redact dropped a public field: %+v", view)
	}
}

func TestMask(t *testing.T) {
	cases := map[string]string{
		"user@example.com": "us***@example.com",
		"a@b.cl":           "a***@b.cl",
		"nope":             "***",
	}
	for in, want := range cases {
		if got := Mask(in); got != want {
			t.Errorf("Mask(%q) = %q, want %q", in, got, want)
		}
	}
}

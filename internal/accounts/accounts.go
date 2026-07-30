package accounts

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// EncryptionKeyEnv names the environment variable holding the AES-256 key used
// to protect passwords at rest, as 32 random bytes in standard base64.
const EncryptionKeyEnv = "ACCOUNTS_ENCRYPTION_KEY"

// ErrNotFound is returned when an account does not exist in the store.
var ErrNotFound = errors.New("account not found")

// Store persists Buk credentials. List returns accounts with their plaintext
// password decoded (used by the marcaje job); API responses must redact it.
type Store interface {
	Seed(context.Context, []Account) error
	List(context.Context) ([]Account, error)
	Save(context.Context, Account) (Account, error)
	SetActive(context.Context, string, bool) (Account, error)
	Delete(context.Context, string) error
}

// Account is a Buk login (corporate email + password) plus the scraped job id.
// Password is the plaintext credential held in memory; stores keep it
// encrypted at rest. It is never serialized in API list responses.
type Account struct {
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	JobID     string    `json:"job_id"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// View is the password-free projection returned by the API.
type View struct {
	Email     string    `json:"email"`
	JobID     string    `json:"job_id"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Response is the list envelope returned by the API.
type Response struct {
	Count int    `json:"count"`
	Items []View `json:"items"`
}

// Redact projects the account without its secret.
func (a Account) Redact() View {
	return View{
		Email:     a.Email,
		JobID:     a.JobID,
		Active:    a.Active,
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
	}
}

// NewAccount builds a normalized, timestamped account from an email and password.
func NewAccount(email, password string, active bool) (Account, error) {
	normalized := NormalizeEmail(email)
	if !validEmail(normalized) {
		return Account{}, errors.New("invalid email")
	}
	if strings.TrimSpace(password) == "" {
		return Account{}, errors.New("password is required")
	}
	now := time.Now().UTC()
	return Account{
		Email:     normalized,
		Password:  password,
		Active:    active,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// NormalizeEmail lowercases and trims an email for use as a stable key.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// EncryptPassword returns the at-rest form of a plaintext password: the random
// nonce followed by the AES-256-GCM ciphertext, base64 encoded. The account
// email is authenticated as additional data, so a ciphertext cannot be moved
// from one account to another.
func EncryptPassword(email, plain string) (string, error) {
	gcm, err := passwordCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), []byte(NormalizeEmail(email)))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptPassword reverses EncryptPassword. It fails, rather than returning
// garbage, when the key is wrong or the stored value was tampered with.
func DecryptPassword(email, encrypted string) (string, error) {
	gcm, err := passwordCipher()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", fmt.Errorf("stored password is not valid base64: %w", err)
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("stored password is shorter than a nonce")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(NormalizeEmail(email)))
	if err != nil {
		return "", fmt.Errorf("decrypt password for %s: %w", Mask(email), err)
	}
	return string(plain), nil
}

func passwordCipher() (cipher.AEAD, error) {
	encoded := strings.TrimSpace(os.Getenv(EncryptionKeyEnv))
	if encoded == "" {
		return nil, fmt.Errorf("%s is not set", EncryptionKeyEnv)
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %w", EncryptionKeyEnv, err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s must decode to 32 bytes, got %d", EncryptionKeyEnv, len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("build aes cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

// Mask returns a log-safe rendering of an email, keeping the domain and the
// first two characters of the local part.
func Mask(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	local, domain := email[:at], email[at:]
	if len(local) <= 2 {
		return local[:1] + "***" + domain
	}
	return local[:2] + "***" + domain
}

func validEmail(email string) bool {
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return false
	}
	return strings.Contains(email[at:], ".")
}

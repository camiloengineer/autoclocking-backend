package accounts

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

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
// Password is the plaintext credential held in memory; stores keep it base64
// encoded at rest. It is never serialized in API list responses.
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

// EncodePassword returns the at-rest (base64) form of a plaintext password.
func EncodePassword(plain string) string {
	return base64.StdEncoding.EncodeToString([]byte(plain))
}

// DecodePassword reverses EncodePassword.
func DecodePassword(encoded string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(raw), nil
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

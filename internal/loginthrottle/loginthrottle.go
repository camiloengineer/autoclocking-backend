package loginthrottle

import (
	"sync"
	"time"
)

const (
	// MaxConsecutiveFailures is how many wrong-password attempts an email may
	// accumulate before the next attempt is refused (fail twice, wait on the third).
	MaxConsecutiveFailures = 2
	// Cooldown is how long a blocked email must wait before trying again.
	Cooldown = 5 * time.Minute
)

type entry struct {
	failures     int
	blockedUntil time.Time
}

// Throttle counts consecutive failed logins per email in memory so repeated
// attempts are stopped before they reach Buk and trip its account lockout.
type Throttle struct {
	mu      sync.Mutex
	entries map[string]*entry
	now     func() time.Time
}

// New builds an empty throttle backed by the wall clock.
func New() *Throttle {
	return &Throttle{entries: map[string]*entry{}, now: time.Now}
}

// NewWithClock builds a throttle with an injectable clock, useful for tests.
func NewWithClock(now func() time.Time) *Throttle {
	return &Throttle{entries: map[string]*entry{}, now: now}
}

// Check reports whether the email is currently blocked and, if so, how long
// it must keep waiting. An expired block clears the counter entirely.
func (t *Throttle) Check(email string) (time.Duration, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	current, exists := t.entries[email]
	if !exists || current.blockedUntil.IsZero() {
		return 0, false
	}

	remaining := current.blockedUntil.Sub(t.now())
	if remaining <= 0 {
		delete(t.entries, email)
		return 0, false
	}
	return remaining, true
}

// Fail records a failed attempt and returns how many attempts remain before
// the email gets blocked; reaching zero starts the cooldown.
func (t *Throttle) Fail(email string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	current, exists := t.entries[email]
	if !exists {
		current = &entry{}
		t.entries[email] = current
	}

	current.failures++
	remaining := MaxConsecutiveFailures - current.failures
	if remaining <= 0 {
		current.blockedUntil = t.now().Add(Cooldown)
		return 0
	}
	return remaining
}

// Reset clears the counter after a successful login.
func (t *Throttle) Reset(email string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, email)
}

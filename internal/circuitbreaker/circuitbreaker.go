package circuitbreaker

import (
	"sync"
	"time"
)

const (
	StateClosed   = "CLOSED"
	StateOpen     = "OPEN"
	StateHalfOpen = "HALF_OPEN"
)

// CircuitBreaker prevents cascading failures.
type CircuitBreaker struct {
	mu              sync.Mutex
	threshold       int
	resetTimeout    time.Duration
	failureCount    int
	lastFailureTime time.Time
	state           string
}

// New creates a new CircuitBreaker.
func New(threshold int, resetTimeoutSeconds int) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:    threshold,
		resetTimeout: time.Duration(resetTimeoutSeconds) * time.Second,
		state:        StateClosed,
	}
}

// CanExecute determines if an operation can be executed.
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if !cb.lastFailureTime.IsZero() && time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful operation.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0
	cb.state = StateClosed
}

// RecordFailure records a failed operation.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.failureCount >= cb.threshold {
		cb.state = StateOpen
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() map[string]interface{} {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	var lastFailureStr string
	if !cb.lastFailureTime.IsZero() {
		lastFailureStr = cb.lastFailureTime.Format(time.RFC3339)
	}

	return map[string]interface{}{
		"state":             cb.state,
		"failure_count":     cb.failureCount,
		"threshold":         cb.threshold,
		"last_failure_time": lastFailureStr,
	}
}

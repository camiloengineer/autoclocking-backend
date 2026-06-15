package delay

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sync"
)

// Manager handles random delays to avoid automated detection.
type Manager struct {
	mu           sync.Mutex
	registry     map[string]int
	coincidences int
}

// New creates a new delay manager.
func New() *Manager {
	return &Manager{
		registry: make(map[string]int),
	}
}

// GetRandomDelay returns a random delay between 1 and 20 minutes.
func (m *Manager) GetRandomDelay(rut string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	const maxAttempts = 10
	attempts := 0
	var delayMinutes int

	for attempts < maxAttempts {
		delayMinutes = rand.IntN(20) + 1 // 1 to 20

		if len(m.registry) == 0 {
			break
		}

		// Check for coincidence
		coincidence := false
		for _, v := range m.registry {
			if v == delayMinutes {
				coincidence = true
				break
			}
		}

		if !coincidence {
			break
		}

		attempts++
	}

	if attempts == maxAttempts {
		m.coincidences++
		slog.Warn(fmt.Sprintf("Could not avoid coincidence after %d attempts. Using delay of %d minutes.", maxAttempts, delayMinutes))
	}

	m.registry[rut] = delayMinutes
	return delayMinutes
}

// Statistics returns statistics about the applied delays.
func (m *Manager) Statistics() map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create a copy of the registry to safely return it
	delays := make(map[string]int, len(m.registry))
	for k, v := range m.registry {
		delays[k] = v
	}

	return map[string]interface{}{
		"total_ruts":   len(m.registry),
		"coincidences": m.coincidences,
		"delays":       delays,
	}
}

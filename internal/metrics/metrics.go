package metrics

import (
	"sync"
	"time"
)

// Collector gathers metrics for monitoring.
type Collector struct {
	mu            sync.Mutex
	RUTsProcessed int
	Successes     int
	Errors        int
	TotalDuration float64
	DelaysApplied int
	StartTime     time.Time
}

// New creates a new metrics collector.
func New() *Collector {
	return &Collector{
		StartTime: time.Now(),
	}
}

// RecordRUTStart logs the start of a RUT processing.
func (c *Collector) RecordRUTStart() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.RUTsProcessed++
}

// RecordSuccess logs a successful operation.
func (c *Collector) RecordSuccess(durationSeconds float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Successes++
	c.TotalDuration += durationSeconds
}

// RecordError logs a failed operation.
func (c *Collector) RecordError() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Errors++
}

// RecordDelayApplied logs that a random delay was applied.
func (c *Collector) RecordDelayApplied() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.DelaysApplied++
}

// Summary returns the gathered metrics.
func (c *Collector) Summary() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	totalTime := time.Since(c.StartTime).Seconds()
	avgDuration := 0.0
	if c.Successes > 0 {
		avgDuration = c.TotalDuration / float64(c.Successes)
	}

	successRate := 0.0
	if c.RUTsProcessed > 0 {
		successRate = float64(c.Successes) / float64(c.RUTsProcessed)
	}

	return map[string]interface{}{
		"ruts_processed":                 c.RUTsProcessed,
		"successes":                      c.Successes,
		"errors":                         c.Errors,
		"success_rate":                   successRate,
		"average_duration_seconds":       avgDuration,
		"total_execution_time_seconds":   totalTime,
		"delays_applied":                 c.DelaysApplied,
	}
}

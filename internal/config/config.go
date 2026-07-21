package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type ExecutionConfig struct {
	RetryAttempts           int
	RetryDelaySeconds       int
	CircuitBreakerThreshold int
	EnableMetrics           bool
}

type Config struct {
	ClockInActive bool
	DebugMode     bool
	Execution     ExecutionConfig
}

// Load loads the execution configuration from environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	clockInActive := strings.ToLower(os.Getenv("CLOCK_IN_ACTIVE")) == "true"
	debugMode := strings.ToLower(os.Getenv("DEBUG_MODE")) == "true"

	retryAttempts, _ := strconv.Atoi(getEnvOrDefault("RETRY_ATTEMPTS", "3"))
	if retryAttempts < 0 || retryAttempts > 10 {
		retryAttempts = 3
	}

	retryDelay, _ := strconv.Atoi(getEnvOrDefault("RETRY_DELAY_SECONDS", "30"))
	if retryDelay < 1 || retryDelay > 300 {
		retryDelay = 30
	}

	cbThreshold, _ := strconv.Atoi(getEnvOrDefault("CIRCUIT_BREAKER_THRESHOLD", "3"))

	enableMetrics := strings.ToLower(getEnvOrDefault("ENABLE_METRICS", "true")) == "true"

	return &Config{
		ClockInActive: clockInActive,
		DebugMode:     debugMode,
		Execution: ExecutionConfig{
			RetryAttempts:           retryAttempts,
			RetryDelaySeconds:       retryDelay,
			CircuitBreakerThreshold: cbThreshold,
			EnableMetrics:           enableMetrics,
		},
	}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

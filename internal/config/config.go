package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	ActiveRUTs    []string
	Execution     ExecutionConfig
}

// Load loads the configuration from environment variables.
func Load() (*Config, error) {
	_ = godotenv.Load() // Ignore error if .env doesn't exist

	clockInActive := strings.ToLower(os.Getenv("CLOCK_IN_ACTIVE")) == "true"
	debugMode := strings.ToLower(os.Getenv("DEBUG_MODE")) == "true"

	ruts, err := loadRUTs(true)
	if err != nil {
		return nil, fmt.Errorf("failed to load RUTs: %w", err)
	}

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
		ActiveRUTs:    ruts,
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

func LoadInitialRUTs() ([]string, error) {
	_ = godotenv.Load()
	return loadRUTs(false)
}

func loadRUTs(required bool) ([]string, error) {
	rutsB64 := os.Getenv("ACTIVE_RUTS_B64")
	rutsEnv := os.Getenv("ACTIVE_RUTS")

	if rutsB64 == "" && rutsEnv == "" {
		if !required {
			return nil, nil
		}
		return nil, fmt.Errorf("neither ACTIVE_RUTS_B64 nor ACTIVE_RUTS are configured")
	}

	var jsonStr string
	if rutsB64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(rutsB64)
		if err != nil {
			return nil, fmt.Errorf("error decoding base64 RUTs: %w", err)
		}
		jsonStr = string(decoded)
	} else {
		jsonStr = rutsEnv
	}

	var ruts []string
	if err := json.Unmarshal([]byte(jsonStr), &ruts); err != nil {
		return nil, fmt.Errorf("error parsing RUTs JSON: %w", err)
	}

	if len(ruts) == 0 {
		return nil, fmt.Errorf("ACTIVE_RUTS is empty")
	}
	if len(ruts) > 10 {
		return nil, fmt.Errorf("maximum 10 RUTs allowed to prevent abuse")
	}

	return ruts, nil
}

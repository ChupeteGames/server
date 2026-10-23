package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config contains the environment-driven settings used to start the API.
type Config struct {
	Port              string
	GinMode           string
	ReadHeaderTimeout time.Duration
	ShutdownTimeout   time.Duration
}

// Load reads environment settings and applies validated defaults for optional values.
func Load() (Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		// Keep local startup simple while still allowing deployments to choose a port.
		port = "8080"
	}
	if number, err := strconv.Atoi(port); err != nil || number < 1 || number > 65535 {
		return Config{}, fmt.Errorf("PORT debe ser un número entre 1 y 65535")
	}

	mode := os.Getenv("GIN_MODE")
	if mode == "" {
		// Use Gin's production mode by default to avoid debug output in deployments.
		mode = "release"
	}
	// Restrict Gin's mode to its supported values to fail fast on configuration mistakes.
	if mode != "debug" && mode != "release" && mode != "test" {
		return Config{}, fmt.Errorf("GIN_MODE debe ser debug, release o test")
	}

	// Bound header reads and graceful shutdown by default; both limits can be tuned through the environment.
	readHeaderTimeout, err := durationFromEnv("READ_HEADER_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := durationFromEnv("SHUTDOWN_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Port:              port,
		GinMode:           mode,
		ReadHeaderTimeout: readHeaderTimeout,
		ShutdownTimeout:   shutdownTimeout,
	}, nil
}

// durationFromEnv parses a positive duration, or returns the supplied default when unset.
func durationFromEnv(name string, defaultValue time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s debe ser una duración positiva (por ejemplo, 5s)", name)
	}
	return duration, nil
}

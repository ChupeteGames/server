package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// DefaultDatabaseURL es la cadena de conexión de desarrollo local definida en
// docker-compose.yml y docs/06_devops_despliegue.md. Se aplica cuando la
// variable de entorno DATABASE_URL no está configurada explícitamente.
const DefaultDatabaseURL = "postgres://game_user:local_dev_only_change_me@localhost:5432/prediction_fps?sslmode=disable"

// DefaultJWTSecret es la clave de firma HMAC-SHA256 por defecto para desarrollo local.
// En entornos productivos debe sobrescribirse mediante la variable de entorno JWT_SECRET.
const DefaultJWTSecret = "local_dev_jwt_secret_change_me_in_production_32b"

// DefaultDedicatedServerHost y DefaultDedicatedServerPort son los datos por defecto
// de la instancia de Unreal Dedicated Server según ADR-05.
const (
	DefaultDedicatedServerHost = "127.0.0.1"
	DefaultDedicatedServerPort = 7777
)

// Config contains the environment-driven settings used to start the API (HTTP + gRPC).
type Config struct {
	Port                string
	GRPCPort            string
	GinMode             string
	DatabaseURL         string
	JWTSecret           string
	DedicatedServerHost string
	DedicatedServerPort int
	ReadHeaderTimeout   time.Duration
	ShutdownTimeout     time.Duration
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

	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		// Puerto por defecto para el servidor gRPC que consume el Unreal Dedicated Server (docs/06).
		grpcPort = "9090"
	}
	if number, err := strconv.Atoi(grpcPort); err != nil || number < 1 || number > 65535 {
		return Config{}, fmt.Errorf("GRPC_PORT debe ser un número entre 1 y 65535")
	}
	if grpcPort == port {
		return Config{}, fmt.Errorf("GRPC_PORT y PORT no pueden usar el mismo puerto (%s)", port)
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

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		// Usa las credenciales del contenedor Docker local por defecto para facilitar el arranque.
		databaseURL = DefaultDatabaseURL
	}
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return Config{}, fmt.Errorf("DATABASE_URL debe comenzar con postgres:// o postgresql://")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = DefaultJWTSecret
	}
	if len(jwtSecret) < 16 {
		return Config{}, fmt.Errorf("JWT_SECRET debe tener al menos 16 caracteres")
	}

	dedicatedServerHost := os.Getenv("DEDICATED_SERVER_HOST")
	if dedicatedServerHost == "" {
		dedicatedServerHost = DefaultDedicatedServerHost
	}

	dedicatedServerPort := DefaultDedicatedServerPort
	if pStr := os.Getenv("DEDICATED_SERVER_PORT"); pStr != "" {
		p, err := strconv.Atoi(pStr)
		if err != nil || p < 1 || p > 65535 {
			return Config{}, fmt.Errorf("DEDICATED_SERVER_PORT debe ser un número entre 1 y 65535")
		}
		dedicatedServerPort = p
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
		Port:                port,
		GRPCPort:            grpcPort,
		GinMode:             mode,
		DatabaseURL:         databaseURL,
		JWTSecret:           jwtSecret,
		DedicatedServerHost: dedicatedServerHost,
		DedicatedServerPort: dedicatedServerPort,
		ReadHeaderTimeout:   readHeaderTimeout,
		ShutdownTimeout:     shutdownTimeout,
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

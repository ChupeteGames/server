package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("GRPC_PORT", "")
	t.Setenv("GIN_MODE", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("READ_HEADER_TIMEOUT", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.GRPCPort != "9090" {
		t.Errorf("expected default gRPC port 9090, got %q", cfg.GRPCPort)
	}
	if cfg.GinMode != "release" {
		t.Errorf("expected default Gin mode release, got %q", cfg.GinMode)
	}
	if cfg.DatabaseURL != DefaultDatabaseURL {
		t.Errorf("expected default database URL %q, got %q", DefaultDatabaseURL, cfg.DatabaseURL)
	}
	if cfg.JWTSecret != DefaultJWTSecret {
		t.Errorf("expected default JWT secret %q, got %q", DefaultJWTSecret, cfg.JWTSecret)
	}
	if cfg.ReadHeaderTimeout != 5*time.Second || cfg.ShutdownTimeout != 5*time.Second {
		t.Errorf("expected 5s timeout defaults, got read-header=%s shutdown=%s", cfg.ReadHeaderTimeout, cfg.ShutdownTimeout)
	}
}

func TestLoadRejectsInvalidPort(t *testing.T) {
	t.Setenv("PORT", "70000")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an out-of-range port")
	}
}

func TestLoadRejectsInvalidGRPCPort(t *testing.T) {
	t.Setenv("GRPC_PORT", "invalid")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an invalid GRPC_PORT")
	}
}

func TestLoadRejectsCollidingPorts(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("GRPC_PORT", "8080")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error when PORT and GRPC_PORT are identical")
	}
}

func TestLoadRejectsInvalidDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://localhost:3306/db")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an invalid DATABASE_URL scheme")
	}
}

func TestLoadRejectsShortJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "short")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for a JWT_SECRET shorter than 16 characters")
	}
}

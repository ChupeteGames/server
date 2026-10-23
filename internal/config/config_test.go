package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("PORT", "")
	t.Setenv("GIN_MODE", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("READ_HEADER_TIMEOUT", "")
	t.Setenv("SHUTDOWN_TIMEOUT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an unexpected error: %v", err)
	}
	if cfg.Port != "8080" {
		t.Errorf("expected default port 8080, got %q", cfg.Port)
	}
	if cfg.GinMode != "release" {
		t.Errorf("expected default Gin mode release, got %q", cfg.GinMode)
	}
	if cfg.DatabaseURL != DefaultDatabaseURL {
		t.Errorf("expected default database URL %q, got %q", DefaultDatabaseURL, cfg.DatabaseURL)
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

func TestLoadRejectsInvalidDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "mysql://localhost:3306/db")

	if _, err := Load(); err == nil {
		t.Fatal("expected an error for an invalid DATABASE_URL scheme")
	}
}

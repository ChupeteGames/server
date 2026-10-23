package postgres

import (
	"context"
	"testing"
	"time"

	"server/internal/config"
)

func TestNewRepositoryNilPoolReturnsUnavailableError(t *testing.T) {
	repo := NewRepository(nil)
	if err := repo.Ping(context.Background()); err == nil {
		t.Fatal("expected an error when pinging a repository with a nil pool")
	}
}

func TestNewPoolRejectsMalformedURL(t *testing.T) {
	if _, err := NewPool(context.Background(), "://invalid-url"); err == nil {
		t.Fatal("expected NewPool to fail for a malformed database URL")
	}
}

func TestNewPoolAndPingIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pool, err := NewPool(ctx, config.DefaultDatabaseURL)
	if err != nil {
		t.Skipf("saltando test de integración (PostgreSQL local no accesible): %v", err)
	}
	defer pool.Close()

	repo := NewRepository(pool)
	if err := repo.Ping(ctx); err != nil {
		t.Fatalf("expected Ping() against local PostgreSQL 17 to succeed, got: %v", err)
	}
}

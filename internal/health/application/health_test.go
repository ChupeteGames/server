package application

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"server/internal/health/domain"
)

// stubPinger simula el Puerto de Salida (domain.Pinger) para probar el Caso de Uso
// de forma 100% aislada sin necesitar una base de datos real.
type stubPinger struct {
	err error
}

func (s *stubPinger) Ping(_ context.Context) error {
	return s.err
}

func TestHealthUseCaseReturnsHealthyWhenPingerSucceeds(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := NewHealthUseCase(logger, &stubPinger{err: nil})

	result := uc.Execute(context.Background(), "trace-ok-1")

	if !result.IsHealthy() || result.Status != domain.StatusHealthy {
		t.Fatalf("expected healthy status %q, got %q", domain.StatusHealthy, result.Status)
	}
}

func TestHealthUseCaseReturnsUnhealthyWhenPingerFails(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	uc := NewHealthUseCase(logger, &stubPinger{err: errors.New("connection refused")})

	result := uc.Execute(context.Background(), "trace-err-1")

	if result.IsHealthy() || result.Status != domain.StatusUnhealthy {
		t.Fatalf("expected unhealthy status %q, got %q", domain.StatusUnhealthy, result.Status)
	}
}

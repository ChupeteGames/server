package application

import (
	"context"
	"server/kit/logging"

	"server/internal/health/domain"
)

// HealthUseCase exposes the application operation required by inbound adapters.
type HealthUseCase interface {
	// Execute returns the domain health status and accepts request metadata for trace propagation.
	Execute(ctx context.Context, traceID string) domain.Health
}

type healthUseCase struct {
	log logging.Logger
}

// NewHealthUseCase constructs the health-check use case.
func NewHealthUseCase(log logging.Logger) HealthUseCase {
	return &healthUseCase{log: log}
}

// Execute returns the domain health status; context and traceID are available for downstream calls.
func (uc *healthUseCase) Execute(ctx context.Context, traceID string) domain.Health {
	uc.log.InfoContext(ctx, "executing health check", "trace_id", traceID)

	return domain.NewHealthyStatus()
}

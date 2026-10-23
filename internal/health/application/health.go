package application

import (
	"context"

	"server/internal/health/domain"
	"server/kit/logging"
)

// HealthUseCase expone la operación de la Capa de Aplicación requerida por los
// adaptadores de entrada (Primary Ports, como el handler HTTP de Gin).
type HealthUseCase interface {
	// Execute evalúa el estado de salud del sistema (incluyendo la conexión a base de datos
	// si se inyectó un Pinger) y propaga el trace_id en los logs estructurados.
	Execute(ctx context.Context, traceID string) domain.Health
}

type healthUseCase struct {
	log     logging.Logger
	pingers []domain.Pinger
}

// NewHealthUseCase construye el caso de uso de chequeo de salud aplicando Inyección de Dependencias.
// Recibe el logger estructurado y cero o más implementaciones del puerto domain.Pinger
// (por ejemplo, el repositorio de PostgreSQL ubicado en infrastructure/storage/postgres).
func NewHealthUseCase(log logging.Logger, pingers ...domain.Pinger) HealthUseCase {
	return &healthUseCase{
		log:     log,
		pingers: pingers,
	}
}

// Execute verifica cada dependencia registrada (domain.Pinger) usando el contexto de la petición.
// Si todas responden correctamente, devuelve StatusHealthy ("ok"); si alguna falla,
// registra el error con su trace_id y devuelve StatusUnhealthy ("error").
func (uc *healthUseCase) Execute(ctx context.Context, traceID string) domain.Health {
	uc.log.InfoContext(ctx, "executing health check", "trace_id", traceID)

	for _, pinger := range uc.pingers {
		if pinger == nil {
			continue
		}
		if err := pinger.Ping(ctx); err != nil {
			uc.log.ErrorContext(
				ctx,
				"storage health check failed",
				"trace_id", traceID,
				"domain_error", domain.ErrStorageUnavailable.Error(),
				"error", err,
			)
			return domain.NewUnhealthyStatus()
		}
	}

	return domain.NewHealthyStatus()
}

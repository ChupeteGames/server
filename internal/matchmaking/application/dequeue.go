package application

import (
	"context"

	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// DequeueUseCase retira a un jugador de la cola de matchmaking si aún no fue emparejado en una partida.
type DequeueUseCase struct {
	logger logging.Logger
	queue  domain.MatchmakingQueue
}

// NewDequeueUseCase construye el caso de uso para cancelar la búsqueda de partida.
func NewDequeueUseCase(
	logger logging.Logger,
	queue domain.MatchmakingQueue,
) *DequeueUseCase {
	return &DequeueUseCase{
		logger: logger,
		queue:  queue,
	}
}

// Execute cancela la búsqueda de partida del jugador.
func (uc *DequeueUseCase) Execute(
	ctx context.Context,
	traceID string,
	playerID string,
) error {
	if playerID == "" {
		return domain.ErrPlayerIDRequired
	}

	if err := uc.queue.Dequeue(ctx, playerID); err != nil {
		uc.logger.ErrorContext(ctx, "fallo al cancelar cola de matchmaking", "trace_id", traceID, "player_id", playerID, "error", err)
		return err
	}

	uc.logger.InfoContext(ctx, "jugador retirado de la cola de matchmaking exitosamente", "trace_id", traceID, "player_id", playerID)
	return nil
}

package application

import (
	"context"

	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// MarkMatchStartedUseCase actualiza el estado de la partida a 'in_progress' cuando
// el Unreal Dedicated Server notifica el inicio de la Ronda 1.
type MarkMatchStartedUseCase struct {
	logger    logging.Logger
	matchRepo domain.MatchRepository
}

// NewMarkMatchStartedUseCase construye el caso de uso.
func NewMarkMatchStartedUseCase(
	logger logging.Logger,
	matchRepo domain.MatchRepository,
) *MarkMatchStartedUseCase {
	return &MarkMatchStartedUseCase{
		logger:    logger,
		matchRepo: matchRepo,
	}
}

// Execute marca la partida como iniciada en el repositorio.
func (uc *MarkMatchStartedUseCase) Execute(
	ctx context.Context,
	traceID string,
	matchID string,
	serverInstanceID string,
) error {
	if matchID == "" {
		return domain.ErrMatchNotFound
	}

	if err := uc.matchRepo.MarkMatchStarted(ctx, matchID); err != nil {
		uc.logger.ErrorContext(
			ctx,
			"error marcando partida como iniciada",
			"trace_id", traceID,
			"match_id", matchID,
			"server_instance_id", serverInstanceID,
			"error", err,
		)
		return err
	}

	uc.logger.InfoContext(
		ctx,
		"partida marcada como en curso (in_progress) por Unreal Dedicated Server",
		"trace_id", traceID,
		"match_id", matchID,
		"server_instance_id", serverInstanceID,
	)

	return nil
}

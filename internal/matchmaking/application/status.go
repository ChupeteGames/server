package application

import (
	"context"

	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// GetStatusUseCase consulta el estado de matchmaking del jugador:
// 1. Si está en cola buscando rivales (QUEUED).
// 2. Si ya se le asignó una partida y tiene ticket de ingreso (MATCH_FOUND).
// 3. Si no se encuentra en ninguna cola activa (IDLE).
type GetStatusUseCase struct {
	logger    logging.Logger
	matchRepo domain.MatchRepository
	queue     domain.MatchmakingQueue
}

// NewGetStatusUseCase construye el caso de uso para consultar el estado del matchmaking.
func NewGetStatusUseCase(
	logger logging.Logger,
	matchRepo domain.MatchRepository,
	queue domain.MatchmakingQueue,
) *GetStatusUseCase {
	return &GetStatusUseCase{
		logger:    logger,
		matchRepo: matchRepo,
		queue:     queue,
	}
}

// Execute verifica el estado en memoria y, de ser necesario, en la base de datos persistente.
func (uc *GetStatusUseCase) Execute(
	ctx context.Context,
	traceID string,
	playerID string,
) (domain.MatchmakingResult, error) {
	if playerID == "" {
		return domain.MatchmakingResult{}, domain.ErrPlayerIDRequired
	}

	// 1. Verificamos el estado en memoria
	status, entry, storedResult, err := uc.queue.GetStatus(ctx, playerID)
	if err == nil {
		if status == domain.StatusMatchFound && storedResult != nil {
			return *storedResult, nil
		}
		if status == domain.StatusQueued && entry != nil {
			return domain.MatchmakingResult{
				Status: domain.StatusQueued,
				Mode:   entry.Mode,
			}, nil
		}
	}

	// 2. Si en memoria figura IDLE (o el servidor reinició), consultamos PostgreSQL
	activeTicket, activeMatch, err := uc.matchRepo.FindActiveTicketByPlayerID(ctx, playerID)
	if err == nil && activeTicket != nil && activeMatch != nil {
		return domain.MatchmakingResult{
			Status:       domain.StatusMatchFound,
			Mode:         activeMatch.Mode,
			MatchID:      activeMatch.ID,
			ServerHost:   activeMatch.ServerHost,
			ServerPort:   activeMatch.ServerPort,
			JoinTicket:   activeTicket.TicketToken,
			AssignedTeam: activeTicket.AssignedTeam,
			ExpiresAt:    &activeTicket.ExpiresAt,
		}, nil
	}

	// 3. El jugador no está en cola ni tiene partidas pendientes
	return domain.MatchmakingResult{
		Status: domain.StatusIdle,
	}, nil
}

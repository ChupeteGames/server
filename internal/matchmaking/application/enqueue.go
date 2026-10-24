package application

import (
	"context"
	"time"

	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// EnqueueUseCase orquesta la entrada de un jugador a la cola de emparejamiento.
// Cuando se reúnen los jugadores necesarios (ej. 4 para 2v2), crea la partida en base de datos,
// asigna bandos (TEAM_A y TEAM_B) y emite los tickets de acceso temporal para el Dedicated Server.
type EnqueueUseCase struct {
	logger              logging.Logger
	matchRepo           domain.MatchRepository
	queue               domain.MatchmakingQueue
	dedicatedServerHost string
	dedicatedServerPort int
}

// NewEnqueueUseCase construye el caso de uso con sus puertos e infraestructura configurada.
func NewEnqueueUseCase(
	logger logging.Logger,
	matchRepo domain.MatchRepository,
	queue domain.MatchmakingQueue,
	serverHost string,
	serverPort int,
) *EnqueueUseCase {
	if serverHost == "" {
		serverHost = "127.0.0.1"
	}
	if serverPort <= 0 {
		serverPort = 7777
	}
	return &EnqueueUseCase{
		logger:              logger,
		matchRepo:           matchRepo,
		queue:               queue,
		dedicatedServerHost: serverHost,
		dedicatedServerPort: serverPort,
	}
}

// Execute procesa la solicitud de encolado del jugador para el modo especificado.
func (uc *EnqueueUseCase) Execute(
	ctx context.Context,
	traceID string,
	playerID string,
	rawMode string,
) (domain.MatchmakingResult, error) {
	if playerID == "" {
		return domain.MatchmakingResult{}, domain.ErrPlayerIDRequired
	}

	mode, err := domain.ValidateMode(rawMode)
	if err != nil {
		uc.logger.ErrorContext(ctx, "modo de juego inválido al encolar", "trace_id", traceID, "player_id", playerID, "mode", rawMode)
		return domain.MatchmakingResult{}, err
	}

	// 1. Verificamos si el jugador ya tiene una partida asignada en memoria
	status, _, storedResult, err := uc.queue.GetStatus(ctx, playerID)
	if err == nil && status == domain.StatusMatchFound && storedResult != nil {
		uc.logger.InfoContext(ctx, "jugador ya poseía una partida lista en memoria", "trace_id", traceID, "player_id", playerID, "match_id", storedResult.MatchID)
		return *storedResult, nil
	}

	// 2. Verificamos si el jugador ya tiene un ticket activo y válido en base de datos
	activeTicket, activeMatch, err := uc.matchRepo.FindActiveTicketByPlayerID(ctx, playerID)
	if err == nil && activeTicket != nil && activeMatch != nil {
		result := domain.MatchmakingResult{
			Status:       domain.StatusMatchFound,
			Mode:         activeMatch.Mode,
			MatchID:      activeMatch.ID,
			ServerHost:   activeMatch.ServerHost,
			ServerPort:   activeMatch.ServerPort,
			JoinTicket:   activeTicket.TicketToken,
			AssignedTeam: activeTicket.AssignedTeam,
			ExpiresAt:    &activeTicket.ExpiresAt,
		}
		uc.logger.InfoContext(ctx, "jugador ya poseía una partida activa en base de datos", "trace_id", traceID, "player_id", playerID, "match_id", activeMatch.ID)
		return result, nil
	}

	// 3. Encolamos al jugador en memoria
	now := time.Now().UTC()
	entry := domain.PlayerQueueEntry{
		PlayerID:   playerID,
		Mode:       mode,
		EnqueuedAt: now,
	}

	isMatchFormed, matchedPlayers, err := uc.queue.Enqueue(ctx, entry)
	if err != nil {
		uc.logger.ErrorContext(ctx, "error encolando jugador", "trace_id", traceID, "player_id", playerID, "error", err)
		return domain.MatchmakingResult{}, err
	}

	// Si no se alcanzó el cupo necesario para el modo, el jugador queda en espera
	if !isMatchFormed {
		uc.logger.InfoContext(ctx, "jugador encolado exitosamente en espera de rivales", "trace_id", traceID, "player_id", playerID, "mode", string(mode))
		return domain.MatchmakingResult{
			Status: domain.StatusQueued,
			Mode:   mode,
		}, nil
	}

	// 4. Si se completó el cupo (ej. 4 jugadores para 2v2), conformamos la partida
	matchID := domain.GenerateTicketToken() // Usamos formato UUID v4
	match := domain.Match{
		ID:         matchID,
		Mode:       mode,
		Status:     domain.MatchStatusCreated,
		ServerHost: uc.dedicatedServerHost,
		ServerPort: uc.dedicatedServerPort,
		CreatedAt:  now,
	}

	playersPerTeam := domain.RequiredPlayersPerTeam(mode)
	ticketExpiresAt := now.Add(10 * time.Minute)

	tickets := make([]domain.MatchTicket, len(matchedPlayers))
	var currentPlayerResult domain.MatchmakingResult

	for i, p := range matchedPlayers {
		team := domain.TeamSideA
		if i >= playersPerTeam {
			team = domain.TeamSideB
		}

		ticket := domain.MatchTicket{
			TicketToken:  domain.GenerateTicketToken(),
			MatchID:      matchID,
			PlayerID:     p.PlayerID,
			AssignedTeam: team,
			ExpiresAt:    ticketExpiresAt,
			CreatedAt:    now,
		}
		tickets[i] = ticket

		res := domain.MatchmakingResult{
			Status:       domain.StatusMatchFound,
			Mode:         mode,
			MatchID:      matchID,
			ServerHost:   uc.dedicatedServerHost,
			ServerPort:   uc.dedicatedServerPort,
			JoinTicket:   ticket.TicketToken,
			AssignedTeam: team,
			ExpiresAt:    &ticketExpiresAt,
		}

		uc.queue.StoreMatchResult(ctx, p.PlayerID, res)

		if p.PlayerID == playerID {
			currentPlayerResult = res
		}
	}

	// Persistimos la partida y los tickets en una transacción ACID de PostgreSQL
	if err := uc.matchRepo.CreateMatchWithTickets(ctx, match, tickets); err != nil {
		uc.logger.ErrorContext(ctx, "fallo al persistir partida y tickets en base de datos", "trace_id", traceID, "match_id", matchID, "error", err)
		return domain.MatchmakingResult{}, err
	}

	uc.logger.InfoContext(
		ctx,
		"partida creada y tickets asignados exitosamente",
		"trace_id", traceID,
		"match_id", matchID,
		"mode", string(mode),
		"players_count", len(matchedPlayers),
	)

	return currentPlayerResult, nil
}

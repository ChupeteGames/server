package application

import (
	"context"

	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// ValidateJoinTicketUseCase orquesta la validación de un ticket de ingreso cuando un jugador
// se conecta al Unreal Dedicated Server (PreLogin/PostLogin).
// Verifica la autenticidad, vigencia y unicidad del ticket, consumiéndolo y devolviendo
// los datos del jugador (equipo asignado, skin y efecto de muerte equipado, y MMR).
type ValidateJoinTicketUseCase struct {
	logger    logging.Logger
	matchRepo domain.MatchRepository
}

// NewValidateJoinTicketUseCase construye el caso de uso para validar tickets en gRPC.
func NewValidateJoinTicketUseCase(
	logger logging.Logger,
	matchRepo domain.MatchRepository,
) *ValidateJoinTicketUseCase {
	return &ValidateJoinTicketUseCase{
		logger:    logger,
		matchRepo: matchRepo,
	}
}

// Execute valida y consume el ticket de ingreso para la partida dada.
func (uc *ValidateJoinTicketUseCase) Execute(
	ctx context.Context,
	traceID string,
	matchID string,
	ticketToken string,
) (domain.ValidatedTicket, error) {
	if matchID == "" || ticketToken == "" {
		return domain.ValidatedTicket{
			Valid:      false,
			ReasonCode: "INVALID_ARGUMENTS",
		}, nil
	}

	result, err := uc.matchRepo.ValidateAndConsumeTicket(ctx, matchID, ticketToken)
	if err != nil {
		uc.logger.ErrorContext(
			ctx,
			"rechazo o error al validar ticket de ingreso",
			"trace_id", traceID,
			"match_id", matchID,
			"ticket_token", ticketToken,
			"error", err,
		)
		return result, err
	}

	uc.logger.InfoContext(
		ctx,
		"ticket de ingreso validado y consumido exitosamente",
		"trace_id", traceID,
		"match_id", matchID,
		"player_id", result.PlayerID,
		"assigned_team", string(result.AssignedTeam),
		"skin", result.EquippedSkinCode,
	)

	return result, nil
}

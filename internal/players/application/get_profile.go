package application

import (
	"context"

	"server/internal/players/domain"
	"server/kit/logging"
)

// GetMyProfileUseCase define el Puerto de Entrada (Primary Port) de la Capa de Aplicación
// para consultar el perfil completo del jugador autenticado (GET /v1/players/me).
type GetMyProfileUseCase interface {
	Execute(ctx context.Context, traceID, playerID string) (domain.PlayerProfile, error)
}

type getMyProfileUseCase struct {
	log     logging.Logger
	players domain.PlayerRepository
}

// NewGetMyProfileUseCase construye el caso de uso inyectando el repositorio de jugadores.
func NewGetMyProfileUseCase(log logging.Logger, players domain.PlayerRepository) GetMyProfileUseCase {
	return &getMyProfileUseCase{
		log:     log,
		players: players,
	}
}

// Execute obtiene el perfil completo del jugador a partir de su ID autenticado y registra la traza.
func (uc *getMyProfileUseCase) Execute(ctx context.Context, traceID, playerID string) (domain.PlayerProfile, error) {
	uc.log.InfoContext(ctx, "fetching player profile", "trace_id", traceID, "player_id", playerID)

	profile, err := uc.players.GetProfileByID(ctx, playerID)
	if err != nil {
		uc.log.ErrorContext(ctx, "failed to fetch player profile", "trace_id", traceID, "player_id", playerID, "error", err)
		return domain.PlayerProfile{}, err
	}

	return profile, nil
}

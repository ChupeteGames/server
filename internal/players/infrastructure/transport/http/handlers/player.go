package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	authhandlers "server/internal/auth/infrastructure/transport/http/handlers"
	healthhandlers "server/internal/health/infrastructure/transport/http/handlers"
	"server/internal/players/application"
	"server/internal/players/domain"
	"server/kit/logging"
)

// careerStatsDTO mapea las estadísticas de carrera al formato JSON exacto de OpenAPI.
type careerStatsDTO struct {
	MatchesPlayed int `json:"matches_played"`
	MatchesWon    int `json:"matches_won"`
	MatchesDrawn  int `json:"matches_drawn"`
	TotalKills    int `json:"total_kills"`
	TotalDeaths   int `json:"total_deaths"`
	MutualKills   int `json:"mutual_kills"`
	DuelsWon      int `json:"duels_won"`
}

// playerProfileResponse es el DTO de salida para GET /v1/players/me (components/schemas/PlayerProfile).
type playerProfileResponse struct {
	PlayerID                string         `json:"player_id"`
	DisplayName             string         `json:"display_name"`
	Level                   int            `json:"level"`
	XP                      int64          `json:"xp"`
	MMR                     int            `json:"mmr"`
	WalletBalance           int64          `json:"wallet_balance"`
	EquippedSkinCode        string         `json:"equipped_skin_code"`
	EquippedDeathEffectCode string         `json:"equipped_death_effect_code"`
	OwnedCosmetics          []string       `json:"owned_cosmetics"`
	CareerStats             careerStatsDTO `json:"career_stats"`
}

type errorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	TraceID   string `json:"trace_id,omitempty"`
}

// PlayerHandler traduce las peticiones HTTP de perfil de jugador al caso de uso correspondiente.
type PlayerHandler struct {
	logger       logging.Logger
	getMyProfile application.GetMyProfileUseCase
}

// NewPlayerHandler construye el adaptador HTTP para el contexto de Jugadores.
func NewPlayerHandler(logger logging.Logger, getMyProfile application.GetMyProfileUseCase) *PlayerHandler {
	return &PlayerHandler{
		logger:       logger,
		getMyProfile: getMyProfile,
	}
}

// GetMe maneja GET /v1/players/me devolviendo el perfil del jugador autenticado mediante JWT.
func (h *PlayerHandler) GetMe(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)
	playerID := c.GetString(authhandlers.PlayerIDContextKey)

	profile, err := h.getMyProfile.Execute(c.Request.Context(), traceID, playerID)
	if err != nil {
		if errors.Is(err, domain.ErrPlayerNotFound) {
			c.JSON(http.StatusUnauthorized, errorResponse{
				ErrorCode: "PLAYER_NOT_FOUND",
				Message:   err.Error(),
				TraceID:   traceID,
			})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "unexpected error fetching player profile", "trace_id", traceID, "player_id", playerID, "error", err)
		c.JSON(http.StatusInternalServerError, errorResponse{
			ErrorCode: "INTERNAL_ERROR",
			Message:   "error interno al obtener el perfil del jugador",
			TraceID:   traceID,
		})
		return
	}

	owned := profile.OwnedCosmetics
	if owned == nil {
		owned = []string{}
	}

	c.JSON(http.StatusOK, playerProfileResponse{
		PlayerID:                profile.PlayerID,
		DisplayName:             profile.DisplayName,
		Level:                   profile.Level,
		XP:                      profile.XP,
		MMR:                     profile.MMR,
		WalletBalance:           profile.WalletBalance,
		EquippedSkinCode:        profile.EquippedSkinCode,
		EquippedDeathEffectCode: profile.EquippedDeathEffectCode,
		OwnedCosmetics:          owned,
		CareerStats: careerStatsDTO{
			MatchesPlayed: profile.CareerStats.MatchesPlayed,
			MatchesWon:    profile.CareerStats.MatchesWon,
			MatchesDrawn:  profile.CareerStats.MatchesDrawn,
			TotalKills:    profile.CareerStats.TotalKills,
			TotalDeaths:   profile.CareerStats.TotalDeaths,
			MutualKills:   profile.CareerStats.MutualKills,
			DuelsWon:      profile.CareerStats.DuelsWon,
		},
	})
}

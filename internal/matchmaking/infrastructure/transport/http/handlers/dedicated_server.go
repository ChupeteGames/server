package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	healthhandlers "server/internal/health/infrastructure/transport/http/handlers"
	"server/internal/matchmaking/application"
	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// validateTicketRequest es el DTO recibido en POST /v1/internal/matches/validate-ticket.
// Espeja los campos de ValidateJoinTicketRequest de contracts/proto/game_backend.proto
// para permitir que el Unreal Dedicated Server valide tickets mediante el módulo HTTP nativo
// del motor (Plan B / Spike técnico de docs/03_arquitectura_red.md y docs/07_riesgos_y_decisiones.md).
type validateTicketRequest struct {
	TraceID    string `json:"trace_id"`
	MatchID    string `json:"match_id"`
	JoinTicket string `json:"join_ticket"`
}

// validateTicketResponse es el DTO devuelto al Unreal Dedicated Server tras validar un ticket.
type validateTicketResponse struct {
	Valid                   bool   `json:"valid"`
	ReasonCode              string `json:"reason_code"`
	PlayerID                string `json:"player_id,omitempty"`
	DisplayName             string `json:"display_name,omitempty"`
	AssignedTeam            string `json:"assigned_team,omitempty"`
	EquippedSkinCode        string `json:"equipped_skin_code,omitempty"`
	EquippedDeathEffectCode string `json:"equipped_death_effect_code,omitempty"`
	CurrentMMR              int    `json:"current_mmr,omitempty"`
}

// markMatchStartedRequest es el DTO recibido en POST /v1/internal/matches/mark-started.
type markMatchStartedRequest struct {
	TraceID          string `json:"trace_id"`
	MatchID          string `json:"match_id"`
	ServerInstanceID string `json:"server_instance_id"`
}

// operationResultResponse modela la respuesta de operaciones del servidor dedicado.
type operationResultResponse struct {
	Success    bool   `json:"success"`
	ResultCode string `json:"result_code"`
	Message    string `json:"message"`
}

// DedicatedServerHandler es el Adaptador Primario HTTP para la comunicación servidor-a-servidor
// entre el Unreal Dedicated Server y el Backend Go. Invoca exactamente los mismos casos de uso
// de aplicación que el adaptador gRPC, manteniendo intacta la Arquitectura Hexagonal.
type DedicatedServerHandler struct {
	logger           logging.Logger
	validateTicketUC *application.ValidateJoinTicketUseCase
	markStartedUC    *application.MarkMatchStartedUseCase
}

// NewDedicatedServerHandler construye el controlador HTTP para el servidor dedicado de Unreal.
func NewDedicatedServerHandler(
	logger logging.Logger,
	validateTicketUC *application.ValidateJoinTicketUseCase,
	markStartedUC *application.MarkMatchStartedUseCase,
) *DedicatedServerHandler {
	return &DedicatedServerHandler{
		logger:           logger,
		validateTicketUC: validateTicketUC,
		markStartedUC:    markStartedUC,
	}
}

// PostValidateTicket atiende POST /v1/internal/matches/validate-ticket para validar y consumir
// un join_ticket cuando un jugador ingresa al Unreal Dedicated Server (PreLogin/PostLogin).
func (h *DedicatedServerHandler) PostValidateTicket(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)

	var req validateTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{
			ErrorCode: "INVALID_REQUEST_BODY",
			Message:   "el cuerpo de la solicitud debe ser un JSON válido con match_id y join_ticket",
			TraceID:   traceID,
		})
		return
	}

	if req.TraceID != "" {
		traceID = req.TraceID
		c.Header("X-Trace-ID", traceID)
	}

	result, err := h.validateTicketUC.Execute(c.Request.Context(), traceID, req.MatchID, req.JoinTicket)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "error interno en PostValidateTicket", "trace_id", traceID, "error", err)
		c.JSON(http.StatusInternalServerError, errorResponse{
			ErrorCode: "INTERNAL_ERROR",
			Message:   "error interno validando ticket de ingreso",
			TraceID:   traceID,
		})
		return
	}

	c.JSON(http.StatusOK, validateTicketResponse{
		Valid:                   result.Valid,
		ReasonCode:              result.ReasonCode,
		PlayerID:                result.PlayerID,
		DisplayName:             result.DisplayName,
		AssignedTeam:            string(result.AssignedTeam),
		EquippedSkinCode:        result.EquippedSkinCode,
		EquippedDeathEffectCode: result.EquippedDeathEffectCode,
		CurrentMMR:              result.CurrentMMR,
	})
}

// PostMarkMatchStarted atiende POST /v1/internal/matches/mark-started cuando el Unreal Dedicated Server
// inicia la Ronda 1 de la partida.
func (h *DedicatedServerHandler) PostMarkMatchStarted(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)

	var req markMatchStartedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{
			ErrorCode: "INVALID_REQUEST_BODY",
			Message:   "el cuerpo de la solicitud debe ser un JSON válido con match_id",
			TraceID:   traceID,
		})
		return
	}

	if req.TraceID != "" {
		traceID = req.TraceID
		c.Header("X-Trace-ID", traceID)
	}

	if err := h.markStartedUC.Execute(c.Request.Context(), traceID, req.MatchID, req.ServerInstanceID); err != nil {
		switch {
		case errors.Is(err, domain.ErrMatchNotFound):
			c.JSON(http.StatusNotFound, operationResultResponse{
				Success:    false,
				ResultCode: "MATCH_NOT_FOUND",
				Message:    "la partida solicitada no existe",
			})
		case errors.Is(err, domain.ErrMatchNotActive):
			c.JSON(http.StatusConflict, operationResultResponse{
				Success:    false,
				ResultCode: "MATCH_NOT_ACTIVE",
				Message:    err.Error(),
			})
		default:
			h.logger.ErrorContext(c.Request.Context(), "error interno en PostMarkMatchStarted", "trace_id", traceID, "error", err)
			c.JSON(http.StatusInternalServerError, operationResultResponse{
				Success:    false,
				ResultCode: "INTERNAL_ERROR",
				Message:    "error interno marcando inicio de partida",
			})
		}
		return
	}

	c.JSON(http.StatusOK, operationResultResponse{
		Success:    true,
		ResultCode: "MATCH_STARTED",
		Message:    "partida iniciada correctamente en el servidor",
	})
}

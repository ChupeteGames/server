package handlers

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	authhandlers "server/internal/auth/infrastructure/transport/http/handlers"
	healthhandlers "server/internal/health/infrastructure/transport/http/handlers"
	"server/internal/matchmaking/application"
	"server/internal/matchmaking/domain"
	"server/kit/logging"
)

// enqueueRequest es el DTO recibido en POST /v1/matchmaking/enqueue.
type enqueueRequest struct {
	Mode string `json:"mode"`
}

// matchmakingResponse es el DTO devuelto tanto en POST /v1/matchmaking/enqueue como en GET /v1/matchmaking/status.
type matchmakingResponse struct {
	Status       string  `json:"status"`
	Mode         string  `json:"mode,omitempty"`
	MatchID      string  `json:"match_id,omitempty"`
	ServerHost   string  `json:"server_host,omitempty"`
	ServerPort   int     `json:"server_port,omitempty"`
	JoinTicket   string  `json:"join_ticket,omitempty"`
	AssignedTeam string  `json:"assigned_team,omitempty"`
	ExpiresAt    *string `json:"expires_at,omitempty"`
}

// dequeueResponse es el DTO devuelto en DELETE /v1/matchmaking/dequeue.
type dequeueResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// errorResponse modela la respuesta uniforme de error del backend.
type errorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	TraceID   string `json:"trace_id,omitempty"`
}

// MatchmakingHandler es el controlador HTTP de entrada para el emparejamiento de partidas.
type MatchmakingHandler struct {
	logger    logging.Logger
	enqueueUC *application.EnqueueUseCase
	statusUC  *application.GetStatusUseCase
	dequeueUC *application.DequeueUseCase
}

// NewMatchmakingHandler construye el controlador con los casos de uso inyectados.
func NewMatchmakingHandler(
	logger logging.Logger,
	enqueueUC *application.EnqueueUseCase,
	statusUC *application.GetStatusUseCase,
	dequeueUC *application.DequeueUseCase,
) *MatchmakingHandler {
	return &MatchmakingHandler{
		logger:    logger,
		enqueueUC: enqueueUC,
		statusUC:  statusUC,
		dequeueUC: dequeueUC,
	}
}

// PostEnqueue atiende POST /v1/matchmaking/enqueue para ingresar a la cola o recibir la partida si ya se completó el cupo.
func (h *MatchmakingHandler) PostEnqueue(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)
	playerID := c.GetString(authhandlers.PlayerIDContextKey)

	if playerID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{
			ErrorCode: "UNAUTHORIZED",
			Message:   domain.ErrPlayerIDRequired.Error(),
			TraceID:   traceID,
		})
		return
	}

	var req enqueueRequest
	// Permitimos body vacío para asumir el modo por defecto ("2v2")
	_ = c.ShouldBindJSON(&req)

	result, err := h.enqueueUC.Execute(c.Request.Context(), traceID, playerID, req.Mode)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidMode):
			c.JSON(http.StatusBadRequest, errorResponse{
				ErrorCode: "INVALID_MODE",
				Message:   err.Error(),
				TraceID:   traceID,
			})
		case errors.Is(err, domain.ErrAlreadyInQueue):
			c.JSON(http.StatusConflict, errorResponse{
				ErrorCode: "ALREADY_IN_QUEUE",
				Message:   err.Error(),
				TraceID:   traceID,
			})
		default:
			h.logger.ErrorContext(c.Request.Context(), "error interno en enqueue", "trace_id", traceID, "error", err)
			c.JSON(http.StatusInternalServerError, errorResponse{
				ErrorCode: "INTERNAL_ERROR",
				Message:   "error procesando la solicitud de emparejamiento",
				TraceID:   traceID,
			})
		}
		return
	}

	c.JSON(http.StatusOK, toMatchmakingResponse(result))
}

// GetStatus atiende GET /v1/matchmaking/status para consultar el estado actual del emparejamiento.
func (h *MatchmakingHandler) GetStatus(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)
	playerID := c.GetString(authhandlers.PlayerIDContextKey)

	if playerID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{
			ErrorCode: "UNAUTHORIZED",
			Message:   domain.ErrPlayerIDRequired.Error(),
			TraceID:   traceID,
		})
		return
	}

	result, err := h.statusUC.Execute(c.Request.Context(), traceID, playerID)
	if err != nil {
		h.logger.ErrorContext(c.Request.Context(), "error interno consultando estado de matchmaking", "trace_id", traceID, "error", err)
		c.JSON(http.StatusInternalServerError, errorResponse{
			ErrorCode: "INTERNAL_ERROR",
			Message:   "error consultando estado de emparejamiento",
			TraceID:   traceID,
		})
		return
	}

	c.JSON(http.StatusOK, toMatchmakingResponse(result))
}

// DeleteDequeue atiende DELETE /v1/matchmaking/dequeue para cancelar la búsqueda de partida.
func (h *MatchmakingHandler) DeleteDequeue(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)
	playerID := c.GetString(authhandlers.PlayerIDContextKey)

	if playerID == "" {
		c.JSON(http.StatusUnauthorized, errorResponse{
			ErrorCode: "UNAUTHORIZED",
			Message:   domain.ErrPlayerIDRequired.Error(),
			TraceID:   traceID,
		})
		return
	}

	if err := h.dequeueUC.Execute(c.Request.Context(), traceID, playerID); err != nil {
		if errors.Is(err, domain.ErrNotInQueue) {
			c.JSON(http.StatusNotFound, errorResponse{
				ErrorCode: "NOT_IN_QUEUE",
				Message:   err.Error(),
				TraceID:   traceID,
			})
			return
		}
		h.logger.ErrorContext(c.Request.Context(), "error interno al cancelar matchmaking", "trace_id", traceID, "error", err)
		c.JSON(http.StatusInternalServerError, errorResponse{
			ErrorCode: "INTERNAL_ERROR",
			Message:   "error cancelando búsqueda de partida",
			TraceID:   traceID,
		})
		return
	}

	c.JSON(http.StatusOK, dequeueResponse{
		Status:  "DEQUEUED",
		Message: "búsqueda de partida cancelada exitosamente",
	})
}

func toMatchmakingResponse(res domain.MatchmakingResult) matchmakingResponse {
	resp := matchmakingResponse{
		Status: string(res.Status),
		Mode:   string(res.Mode),
	}

	if res.Status == domain.StatusMatchFound {
		resp.MatchID = res.MatchID
		resp.ServerHost = res.ServerHost
		resp.ServerPort = res.ServerPort
		resp.JoinTicket = res.JoinTicket
		resp.AssignedTeam = string(res.AssignedTeam)
		if res.ExpiresAt != nil {
			formatted := res.ExpiresAt.Format(time.RFC3339)
			resp.ExpiresAt = &formatted
		}
	}

	return resp
}

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/health/application"
	"server/kit/logging"
)

// TraceIDContextKey is the Gin context key used to share request trace IDs with handlers.
const TraceIDContextKey = "trace_id"

// healthResponse es el DTO (Data Transfer Object) de salida de la capa de transporte HTTP.
// Mantiene las etiquetas de serialización JSON aisladas en infrastructure/transport/http
// para no contaminar las entidades puras de domain/.
type healthResponse struct {
	Status string `json:"status"`
}

// HealthHandler translates health use-case results into HTTP responses.
type HealthHandler struct {
	logger logging.Logger
	health application.HealthUseCase
}

// NewHealthHandler constructs the health handler with its logger and application use case.
func NewHealthHandler(logger logging.Logger, health application.HealthUseCase) *HealthHandler {
	return &HealthHandler{logger: logger, health: health}
}

// GetHealth returns the current health status as a JSON response.
// Si tanto la API como PostgreSQL responden con éxito, devuelve 200 OK con {"status":"ok"}.
// Si alguna dependencia falla, devuelve 503 Service Unavailable.
func (h *HealthHandler) GetHealth(c *gin.Context) {
	traceID := c.GetString(TraceIDContextKey)
	h.logger.InfoContext(c.Request.Context(), "health check starting", "trace_id", traceID)

	result := h.health.Execute(c.Request.Context(), traceID)
	h.logger.InfoContext(c.Request.Context(), "health check completed", "trace_id", traceID, "status", result.Status)

	statusCode := http.StatusOK
	if !result.IsHealthy() {
		statusCode = http.StatusServiceUnavailable
	}

	c.JSON(statusCode, healthResponse{Status: string(result.Status)})
}

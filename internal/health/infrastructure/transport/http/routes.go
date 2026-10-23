package http

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/health/application"
	"server/internal/health/infrastructure/transport/http/handlers"
	"server/kit/logging"
)

const (
	traceIDHeaderKey = "X-Trace-ID"
)

// NewRouter creates the HTTP router and registers all routes for this transport.
func NewRouter(logger logging.Logger, health application.HealthUseCase) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), traceIDMiddleware(logger))
	// Return 405 for registered paths called with an unsupported HTTP method.
	router.HandleMethodNotAllowed = true

	healthHandler := handlers.NewHealthHandler(logger, health)
	router.GET("/health", healthHandler.GetHealth)

	return router
}

// traceIDMiddleware accepts an incoming trace ID or creates one and returns it in the response.
func traceIDMiddleware(logger logging.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader(traceIDHeaderKey)
		if traceID == "" {
			traceIDBytes := make([]byte, 16)
			if _, err := rand.Read(traceIDBytes); err != nil {
				logger.ErrorContext(c.Request.Context(), "failed to generate trace ID", "error", err)
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
				return
			}
			traceID = hex.EncodeToString(traceIDBytes)
		}

		c.Set(handlers.TraceIDContextKey, traceID)
		c.Header(traceIDHeaderKey, traceID)
		c.Next()
	}
}

package http

import (
	"github.com/gin-gonic/gin"

	"server/internal/matchmaking/application"
	"server/internal/matchmaking/infrastructure/transport/http/handlers"
	"server/kit/logging"
)

// RegisterRoutes registra los endpoints protegidos del módulo de emparejamiento (matchmaking)
// bajo el middleware de autenticación Bearer JWT.
func RegisterRoutes(
	router *gin.Engine,
	logger logging.Logger,
	authMiddleware gin.HandlerFunc,
	enqueueUC *application.EnqueueUseCase,
	statusUC *application.GetStatusUseCase,
	dequeueUC *application.DequeueUseCase,
) {
	handler := handlers.NewMatchmakingHandler(logger, enqueueUC, statusUC, dequeueUC)

	// Endpoints REST protegidos para el cliente de Unreal Engine
	router.POST("/v1/matchmaking/enqueue", authMiddleware, handler.PostEnqueue)
	router.GET("/v1/matchmaking/status", authMiddleware, handler.GetStatus)
	router.DELETE("/v1/matchmaking/dequeue", authMiddleware, handler.DeleteDequeue)
}

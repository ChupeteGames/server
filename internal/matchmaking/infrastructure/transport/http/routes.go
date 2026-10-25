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

// RegisterDedicatedServerRoutes registra los endpoints HTTP internos consumidos por el
// Unreal Dedicated Server para validar tickets de ingreso y notificar el inicio de partida,
// delegando en los mismos casos de uso que el servidor gRPC (Plan B / Spike de docs/03 y docs/07).
func RegisterDedicatedServerRoutes(
	router *gin.Engine,
	logger logging.Logger,
	validateTicketUC *application.ValidateJoinTicketUseCase,
	markStartedUC *application.MarkMatchStartedUseCase,
) {
	dsHandler := handlers.NewDedicatedServerHandler(logger, validateTicketUC, markStartedUC)

	router.POST("/v1/internal/matches/validate-ticket", dsHandler.PostValidateTicket)
	router.POST("/v1/internal/matches/mark-started", dsHandler.PostMarkMatchStarted)
}

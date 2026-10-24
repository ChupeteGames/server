package http

import (
	"github.com/gin-gonic/gin"

	"server/internal/players/application"
	"server/internal/players/infrastructure/transport/http/handlers"
	"server/kit/logging"
)

// RegisterRoutes registra los endpoints protegidos de jugadores bajo el middleware de autenticación Bearer JWT.
func RegisterRoutes(
	router *gin.Engine,
	logger logging.Logger,
	authMiddleware gin.HandlerFunc,
	getMyProfileUseCase application.GetMyProfileUseCase,
) {
	playerHandler := handlers.NewPlayerHandler(logger, getMyProfileUseCase)
	router.GET("/v1/players/me", authMiddleware, playerHandler.GetMe)
}

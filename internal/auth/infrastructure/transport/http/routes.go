package http

import (
	"github.com/gin-gonic/gin"

	"server/internal/auth/application"
	"server/internal/auth/infrastructure/transport/http/handlers"
	"server/kit/logging"
)

// RegisterRoutes registra los endpoints públicos de autenticación en el router HTTP Gin.
func RegisterRoutes(router *gin.Engine, logger logging.Logger, loginUseCase application.LoginUseCase) {
	authHandler := handlers.NewAuthHandler(logger, loginUseCase)
	router.POST("/v1/auth/login", authHandler.PostLogin)
}

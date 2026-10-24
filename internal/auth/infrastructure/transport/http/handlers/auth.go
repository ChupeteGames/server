package handlers

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/auth/application"
	"server/internal/auth/domain"
	healthhandlers "server/internal/health/infrastructure/transport/http/handlers"
	"server/kit/logging"
)

const (
	// PlayerIDContextKey es la clave en el contexto de Gin donde el middleware JWT guarda el ID del jugador autenticado.
	PlayerIDContextKey  = "player_id"
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

// loginRequest es el DTO de entrada para POST /v1/auth/login (según contracts/openapi/openapi.yaml).
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// loginResponse es el DTO de salida para POST /v1/auth/login.
type loginResponse struct {
	AccessToken string `json:"access_token"`
	PlayerID    string `json:"player_id"`
	DisplayName string `json:"display_name"`
}

// errorResponse es el DTO estándar de error definido en components/schemas/ErrorResponse de OpenAPI.
type errorResponse struct {
	ErrorCode string `json:"error_code"`
	Message   string `json:"message"`
	TraceID   string `json:"trace_id,omitempty"`
}

// AuthHandler traduce las peticiones HTTP de autenticación al caso de uso de aplicación.
type AuthHandler struct {
	logger logging.Logger
	login  application.LoginUseCase
}

// NewAuthHandler construye el adaptador HTTP de autenticación.
func NewAuthHandler(logger logging.Logger, login application.LoginUseCase) *AuthHandler {
	return &AuthHandler{
		logger: logger,
		login:  login,
	}
}

// PostLogin maneja POST /v1/auth/login.
func (h *AuthHandler) PostLogin(c *gin.Context) {
	traceID := c.GetString(healthhandlers.TraceIDContextKey)

	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse{
			ErrorCode: "INVALID_REQUEST_BODY",
			Message:   "el cuerpo de la solicitud debe ser un JSON válido con username y password",
			TraceID:   traceID,
		})
		return
	}

	session, err := h.login.Execute(c.Request.Context(), traceID, req.Username, req.Password)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidUsername), errors.Is(err, domain.ErrInvalidPassword):
			c.JSON(http.StatusBadRequest, errorResponse{
				ErrorCode: "VALIDATION_ERROR",
				Message:   err.Error(),
				TraceID:   traceID,
			})
		case errors.Is(err, domain.ErrInvalidCredentials):
			c.JSON(http.StatusUnauthorized, errorResponse{
				ErrorCode: "INVALID_CREDENTIALS",
				Message:   err.Error(),
				TraceID:   traceID,
			})
		default:
			h.logger.ErrorContext(c.Request.Context(), "unexpected error during login", "trace_id", traceID, "error", err)
			c.JSON(http.StatusInternalServerError, errorResponse{
				ErrorCode: "INTERNAL_ERROR",
				Message:   "error interno al procesar el inicio de sesión",
				TraceID:   traceID,
			})
		}
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		AccessToken: session.AccessToken,
		PlayerID:    session.PlayerID,
		DisplayName: session.DisplayName,
	})
}

// BearerAuthMiddleware valida el header `Authorization: Bearer <token>` utilizando el puerto
// domain.TokenManager e inyecta el `player_id` resultante en el contexto de Gin para los endpoints protegidos.
func BearerAuthMiddleware(logger logging.Logger, tokenValidator domain.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetString(healthhandlers.TraceIDContextKey)
		authHeader := c.GetHeader(authorizationHeader)

		if !strings.HasPrefix(authHeader, bearerPrefix) {
			logger.InfoContext(c.Request.Context(), "missing or malformed bearer header", "trace_id", traceID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse{
				ErrorCode: "UNAUTHORIZED",
				Message:   domain.ErrInvalidToken.Error(),
				TraceID:   traceID,
			})
			return
		}

		rawToken := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerPrefix))
		playerID, err := tokenValidator.ValidateToken(rawToken)
		if err != nil {
			logger.InfoContext(c.Request.Context(), "invalid or expired JWT token", "trace_id", traceID)
			c.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse{
				ErrorCode: "UNAUTHORIZED",
				Message:   domain.ErrInvalidToken.Error(),
				TraceID:   traceID,
			})
			return
		}

		c.Set(PlayerIDContextKey, playerID)
		c.Next()
	}
}

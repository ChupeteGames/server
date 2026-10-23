package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"server/internal/config"
	"server/internal/health/application"
	postgresstorage "server/internal/health/infrastructure/storage/postgres"
	httptransport "server/internal/health/infrastructure/transport/http"
)

// main es el Composition Root (punto de ensamblaje) de nuestra Arquitectura Hexagonal:
//  1. Carga la configuración de entorno (PORT, GIN_MODE, DATABASE_URL).
//  2. Inicializa el Adaptador Secundario (pool pgx hacia PostgreSQL 17) y verifica la conexión.
//  3. Inyecta el repositorio en la Capa de Aplicación (HealthUseCase).
//  4. Conecta el Caso de Uso al Adaptador Primario (Router HTTP Gin) y levanta el servidor.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("no se pudo cargar la configuración: %v", err)
	}

	// NotifyContext convierte las señales del SO (Ctrl+C / SIGTERM) en cancelación de contexto
	// para permitir un apagado ordenado (graceful shutdown) tanto de HTTP como de PostgreSQL.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Apply the configured mode before creating the Gin engine.
	gin.SetMode(cfg.GinMode)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Inicializamos el pool de conexiones a PostgreSQL (infrastructure/storage/postgres)
	// y verificamos inmediatamente la conectividad contra localhost:5432.
	dbPool, err := postgresstorage.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("no se pudo conectar a PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	logger.InfoContext(ctx, "conexión a PostgreSQL establecida correctamente")

	// Inyección de dependencias siguiendo la regla: Infrastructure -> Application -> Domain.
	healthRepo := postgresstorage.NewRepository(dbPool)
	healthUseCase := application.NewHealthUseCase(logger, healthRepo)

	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httptransport.NewRouter(logger, healthUseCase),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		log.Printf("API escuchando en :%s", cfg.Port)
		serverErrors <- server.ListenAndServe()
	}()

	// Either report an unexpected server failure or stop accepting requests and drain active connections.
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("no se pudo iniciar la API: %v", err)
		}
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Fatalf("no se pudo cerrar la API correctamente: %v", err)
		}
	}
}

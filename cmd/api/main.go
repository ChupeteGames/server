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
	httptransport "server/internal/health/infrastructure/transport/http"
)

// main loads the application configuration, wires dependencies, and serves HTTP.
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("no se pudo cargar la configuración: %v", err)
	}

	// Apply the configured mode before creating the Gin engine.
	gin.SetMode(cfg.GinMode)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	healthUseCase := application.NewHealthUseCase(logger)
	server := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           httptransport.NewRouter(logger, healthUseCase),
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	// NotifyContext converts OS termination signals into a cancellation signal for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

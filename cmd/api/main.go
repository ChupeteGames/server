package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"

	authapp "server/internal/auth/application"
	"server/internal/auth/infrastructure/security"
	authpostgres "server/internal/auth/infrastructure/storage/postgres"
	authhttp "server/internal/auth/infrastructure/transport/http"
	authhandlers "server/internal/auth/infrastructure/transport/http/handlers"
	"server/internal/config"
	healthapp "server/internal/health/application"
	healthpostgres "server/internal/health/infrastructure/storage/postgres"
	healthhttp "server/internal/health/infrastructure/transport/http"
	playersapp "server/internal/players/application"
	playerspostgres "server/internal/players/infrastructure/storage/postgres"
	playershttp "server/internal/players/infrastructure/transport/http"
	grpctransport "server/internal/shared/transport/grpc"
)

// main es el Composition Root (punto de ensamblaje) de nuestra Arquitectura Hexagonal:
//  1. Carga la configuración de entorno (PORT, GRPC_PORT, GIN_MODE, DATABASE_URL, JWT_SECRET).
//  2. Inicializa el Adaptador Secundario (pool pgx hacia PostgreSQL 17) y verifica la conexión.
//  3. Ensambla los repositorios, servicios de seguridad (bcrypt + JWT) y casos de uso de:
//     - Health (`GET /health`)
//     - Auth (`POST /v1/auth/login`)
//     - Players (`GET /v1/players/me`)
//  4. Levanta en paralelo los dos Adaptadores Primarios de entrada:
//     - Servidor HTTP (Gin) en :8080 (para Clientes de Unreal Engine).
//     - Servidor gRPC en :9090 (para el Unreal Dedicated Server).
func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("no se pudo cargar la configuración: %v", err)
	}

	// NotifyContext convierte las señales del SO (Ctrl+C / SIGTERM) en cancelación de contexto
	// para permitir un apagado ordenado (graceful shutdown) de HTTP, gRPC y PostgreSQL.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Apply the configured mode before creating the Gin engine.
	gin.SetMode(cfg.GinMode)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Inicializamos el pool de conexiones a PostgreSQL (infrastructure/storage/postgres)
	// y verificamos inmediatamente la conectividad contra localhost:5432.
	dbPool, err := healthpostgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("no se pudo conectar a PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	logger.InfoContext(ctx, "conexión a PostgreSQL establecida correctamente")

	// -------------------------------------------------------------------------
	// Ensamblaje de Dependencias (Infrastructure -> Application -> Domain)
	// -------------------------------------------------------------------------

	// 1. Contexto: Health
	healthRepo := healthpostgres.NewRepository(dbPool)
	healthUseCase := healthapp.NewHealthUseCase(logger, healthRepo)

	// 2. Contexto: Auth (Login/Registro transaccional + Bcrypt + JWT HS256)
	userRepo := authpostgres.NewUserRepository(dbPool)
	passwordHasher := security.NewBcryptHasher()
	jwtManager := security.NewJWTManager(cfg.JWTSecret)
	loginUseCase := authapp.NewLoginUseCase(logger, userRepo, passwordHasher, jwtManager)

	// 3. Contexto: Players (Perfil competitivo, MMR, billetera y loadout)
	playerRepo := playerspostgres.NewPlayerRepository(dbPool)
	getMyProfileUseCase := playersapp.NewGetMyProfileUseCase(logger, playerRepo)

	// -------------------------------------------------------------------------
	// Registro de Rutas en el Adaptador Primario HTTP (Gin)
	// -------------------------------------------------------------------------
	router := healthhttp.NewRouter(logger, healthUseCase)
	authhttp.RegisterRoutes(router, logger, loginUseCase)
	bearerMiddleware := authhandlers.BearerAuthMiddleware(logger, jwtManager)
	playershttp.RegisterRoutes(router, logger, bearerMiddleware, getMyProfileUseCase)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
	}

	// Configuración del Adaptador Primario gRPC para el Unreal Dedicated Server.
	grpcListener, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	if err != nil {
		log.Fatalf("no se pudo abrir el puerto gRPC :%s: %v", cfg.GRPCPort, err)
	}
	grpcAdapter := grpctransport.NewGameBackendAdapter(logger)
	grpcServer := grpctransport.NewServer(logger, grpcAdapter)

	serverErrors := make(chan error, 2)

	go func() {
		log.Printf("API HTTP escuchando en :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- err
		}
	}()

	go func() {
		log.Printf("Servidor gRPC escuchando en :%s", cfg.GRPCPort)
		if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serverErrors <- err
		}
	}()

	// Esperamos o bien un error inesperado en cualquiera de los dos servidores,
	// o bien la señal de apagado del sistema operativo para detener ambos limpiamente.
	select {
	case err := <-serverErrors:
		log.Fatalf("error fatal en servidor de transporte: %v", err)
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()

		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-shutdownContext.Done():
			grpcServer.Stop()
		}

		if err := httpServer.Shutdown(shutdownContext); err != nil {
			log.Fatalf("no se pudo cerrar la API HTTP correctamente: %v", err)
		}
	}
}

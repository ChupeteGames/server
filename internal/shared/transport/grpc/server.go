package grpctransport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	matchmakingdomain "server/internal/matchmaking/domain"
	"server/internal/shared/pb/gamev1"
	"server/kit/logging"
)

const (
	// TraceIDMetadataKey es la clave en los metadatos gRPC (equivalente al header HTTP X-Trace-ID)
	// para propagar la trazabilidad entre el Unreal Dedicated Server y el servidor Go.
	TraceIDMetadataKey = "x-trace-id"
)

type traceIDContextKeyType struct{}

var traceIDContextKey = traceIDContextKeyType{}

// traceIDCarrier permite extraer el campo trace_id directamente del payload Protobuf
// cuando el Unreal Dedicated Server lo envía dentro del mensaje (ej. ValidateJoinTicketRequest).
type traceIDCarrier interface {
	GetTraceId() string
}

// TraceIDFromContext recupera el trace_id inyectado por el interceptor gRPC en el contexto.
func TraceIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(traceIDContextKey).(string); ok {
		return v
	}
	return ""
}

// ValidateTicketRunner define el puerto primario invocado por gRPC para validar tickets de ingreso.
type ValidateTicketRunner interface {
	Execute(ctx context.Context, traceID string, matchID string, ticketToken string) (matchmakingdomain.ValidatedTicket, error)
}

// MarkMatchStartedRunner define el puerto primario invocado por gRPC para notificar inicio de partida.
type MarkMatchStartedRunner interface {
	Execute(ctx context.Context, traceID string, matchID string, serverInstanceID string) error
}

// GameBackendAdapter es el Adaptador Primario (Input Adapter) gRPC para GameBackendService.
// Expone el contrato gRPC oficial consumido por el Unreal Dedicated Server y delega
// en los casos de uso de aplicación correspondientes (Matchmaking en MVP-008, Matches en MVP-015).
type GameBackendAdapter struct {
	gamev1.UnimplementedGameBackendServiceServer
	logger             logging.Logger
	validateTicketUC   ValidateTicketRunner
	markMatchStartedUC MarkMatchStartedRunner
}

// NewGameBackendAdapter construye el adaptador gRPC inyectando los casos de uso requeridos.
func NewGameBackendAdapter(
	logger logging.Logger,
	validateTicketUC ValidateTicketRunner,
	markMatchStartedUC MarkMatchStartedRunner,
) *GameBackendAdapter {
	return &GameBackendAdapter{
		logger:             logger,
		validateTicketUC:   validateTicketUC,
		markMatchStartedUC: markMatchStartedUC,
	}
}

// ValidateJoinTicket recibe la solicitud de validación de ticket desde el Unreal Dedicated Server (PreLogin/PostLogin).
func (a *GameBackendAdapter) ValidateJoinTicket(ctx context.Context, req *gamev1.ValidateJoinTicketRequest) (*gamev1.ValidateJoinTicketResponse, error) {
	traceID := resolveTraceID(ctx, req)
	a.logger.InfoContext(ctx, "gRPC ValidateJoinTicket received", "trace_id", traceID, "match_id", req.GetMatchId())

	if a.validateTicketUC == nil {
		return nil, status.Errorf(codes.Unimplemented, "ValidateJoinTicket aún no implementado (caso de uso no configurado)")
	}

	result, err := a.validateTicketUC.Execute(ctx, traceID, req.GetMatchId(), req.GetJoinTicket())
	if err != nil {
		a.logger.ErrorContext(ctx, "error interno en ValidateJoinTicket", "trace_id", traceID, "error", err)
		return nil, status.Errorf(codes.Internal, "error interno validando ticket: %v", err)
	}

	var assignedTeam gamev1.TeamSide
	switch result.AssignedTeam {
	case matchmakingdomain.TeamSideA:
		assignedTeam = gamev1.TeamSide_TEAM_SIDE_A
	case matchmakingdomain.TeamSideB:
		assignedTeam = gamev1.TeamSide_TEAM_SIDE_B
	default:
		assignedTeam = gamev1.TeamSide_TEAM_SIDE_UNSPECIFIED
	}

	return &gamev1.ValidateJoinTicketResponse{
		Valid:                   result.Valid,
		ReasonCode:              result.ReasonCode,
		PlayerId:                result.PlayerID,
		DisplayName:             result.DisplayName,
		AssignedTeam:            assignedTeam,
		EquippedSkinCode:        result.EquippedSkinCode,
		EquippedDeathEffectCode: result.EquippedDeathEffectCode,
		CurrentMmr:              int32(result.CurrentMMR),
	}, nil
}

// MarkMatchStarted recibe el aviso de inicio de la Ronda 1 desde el Unreal Dedicated Server.
func (a *GameBackendAdapter) MarkMatchStarted(ctx context.Context, req *gamev1.MarkMatchStartedRequest) (*gamev1.OperationResult, error) {
	traceID := resolveTraceID(ctx, req)
	a.logger.InfoContext(ctx, "gRPC MarkMatchStarted received", "trace_id", traceID, "match_id", req.GetMatchId())

	if a.markMatchStartedUC == nil {
		return nil, status.Errorf(codes.Unimplemented, "MarkMatchStarted aún no implementado (caso de uso no configurado)")
	}

	err := a.markMatchStartedUC.Execute(ctx, traceID, req.GetMatchId(), req.GetServerInstanceId())
	if err != nil {
		if errors.Is(err, matchmakingdomain.ErrMatchNotFound) {
			return &gamev1.OperationResult{
				Success:    false,
				ResultCode: "MATCH_NOT_FOUND",
				Message:    "la partida solicitada no existe",
			}, nil
		}
		a.logger.ErrorContext(ctx, "error en MarkMatchStarted", "trace_id", traceID, "error", err)
		return &gamev1.OperationResult{
			Success:    false,
			ResultCode: "ERROR",
			Message:    err.Error(),
		}, nil
	}

	return &gamev1.OperationResult{
		Success:    true,
		ResultCode: "MATCH_STARTED",
		Message:    "partida iniciada correctamente en el servidor",
	}, nil
}

// ReportMatchResult recibe el acta final de la partida desde el Unreal Dedicated Server.
func (a *GameBackendAdapter) ReportMatchResult(ctx context.Context, req *gamev1.ReportMatchResultRequest) (*gamev1.ReportMatchResultResponse, error) {
	traceID := resolveTraceID(ctx, req)
	a.logger.InfoContext(
		ctx,
		"gRPC ReportMatchResult received",
		"trace_id", traceID,
		"match_id", req.GetMatchId(),
		"operation_id", req.GetOperationId(),
	)
	return nil, status.Errorf(codes.Unimplemented, "ReportMatchResult aún no implementado (pendiente MVP-015)")
}

// NewServer crea y configura el servidor gRPC registrando:
//  1. El interceptor unario de trazabilidad (trace_id) y logging estructurado.
//  2. El servicio estándar grpc.health.v1.Health para chequeos de salud gRPC.
//  3. El servicio GameBackendService generado desde contracts/proto/game_backend.proto.
func NewServer(logger logging.Logger, backendService gamev1.GameBackendServiceServer) *grpc.Server {
	if backendService == nil {
		backendService = NewGameBackendAdapter(logger, nil, nil)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(traceIDUnaryInterceptor(logger)),
	)

	// Registramos el servicio estándar de Health de gRPC como SERVING.
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus(gamev1.GameBackendService_ServiceDesc.ServiceName, grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(grpcServer, healthServer)

	// Registramos el contrato principal del juego (Unreal Dedicated Server <-> Go).
	gamev1.RegisterGameBackendServiceServer(grpcServer, backendService)

	// Habilitamos gRPC Server Reflection para que herramientas como Postman,
	// Insomnia o grpcurl descubran automáticamente los servicios y mensajes en :9090.
	reflection.Register(grpcServer)

	return grpcServer
}

// traceIDUnaryInterceptor extrae el trace_id del payload o de los metadatos entrantes de gRPC
// (o genera uno nuevo si no vino ninguno), lo devuelve en los headers de respuesta gRPC y
// deja trazabilidad estructurada de cada llamada RPC.
func traceIDUnaryInterceptor(logger logging.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		traceID := extractTraceID(ctx, req)
		ctx = context.WithValue(ctx, traceIDContextKey, traceID)
		_ = grpc.SetHeader(ctx, metadata.Pairs(TraceIDMetadataKey, traceID))

		logger.InfoContext(ctx, "gRPC request started", "method", info.FullMethod, "trace_id", traceID)
		resp, err := handler(ctx, req)
		if err != nil {
			logger.ErrorContext(ctx, "gRPC request finished with error", "method", info.FullMethod, "trace_id", traceID, "error", err)
			return nil, err
		}
		logger.InfoContext(ctx, "gRPC request completed", "method", info.FullMethod, "trace_id", traceID)
		return resp, nil
	}
}

func extractTraceID(ctx context.Context, req any) string {
	// 1. Priorizamos el campo trace_id del mensaje Protobuf si el Dedicated Server lo envió.
	if carrier, ok := req.(traceIDCarrier); ok {
		if id := carrier.GetTraceId(); id != "" {
			return id
		}
	}

	// 2. Si no vino en el mensaje, buscamos en los metadatos gRPC (x-trace-id).
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get(TraceIDMetadataKey); len(values) > 0 && values[0] != "" {
			return values[0]
		}
	}

	// 3. Si tampoco vino en metadatos, generamos un identificador aleatorio de 16 bytes (32 hex).
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "trace-fallback-0000000000000000"
	}
	return hex.EncodeToString(buf)
}

func resolveTraceID(ctx context.Context, req traceIDCarrier) string {
	if id := TraceIDFromContext(ctx); id != "" {
		return id
	}
	if req != nil && req.GetTraceId() != "" {
		return req.GetTraceId()
	}
	return ""
}

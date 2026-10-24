package grpctransport

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	matchmakingdomain "server/internal/matchmaking/domain"
	"server/internal/shared/pb/gamev1"
)

type mockValidateTicketRunner struct {
	result matchmakingdomain.ValidatedTicket
	err    error
}

func (m *mockValidateTicketRunner) Execute(_ context.Context, _ string, _, _ string) (matchmakingdomain.ValidatedTicket, error) {
	return m.result, m.err
}

type mockMarkMatchStartedRunner struct {
	err error
}

func (m *mockMarkMatchStartedRunner) Execute(_ context.Context, _ string, _, _ string) error {
	return m.err
}

func TestGRPCServerStartsAndServesHealthAndUnimplementedWhenNil(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	srv := NewServer(logger, NewGameBackendAdapter(logger, nil, nil))
	defer srv.Stop()

	go func() {
		_ = srv.Serve(lis)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create in-memory gRPC client: %v", err)
	}
	defer conn.Close()

	// 1. Verificar servicio de salud de gRPC
	healthClient := grpc_health_v1.NewHealthClient(conn)
	healthResp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: gamev1.GameBackendService_ServiceDesc.ServiceName,
	})
	if err != nil {
		t.Fatalf("gRPC health check failed: %v", err)
	}
	if healthResp.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("expected SERVING status, got %v", healthResp.GetStatus())
	}

	// 2. Verificar llamada con use cases nil responde Unimplemented y propaga trace_id
	gameClient := gamev1.NewGameBackendServiceClient(conn)
	var responseHeaders metadata.MD
	_, err = gameClient.ValidateJoinTicket(
		ctx,
		&gamev1.ValidateJoinTicketRequest{
			TraceId:    "unreal-trace-999",
			MatchId:    "match-123",
			JoinTicket: "ticket-abc",
		},
		grpc.Header(&responseHeaders),
	)
	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("expected Unimplemented status code when use case is nil, got %v (err=%v)", status.Code(err), err)
	}

	if gotTrace := responseHeaders.Get(TraceIDMetadataKey); len(gotTrace) == 0 || gotTrace[0] != "unreal-trace-999" {
		t.Fatalf("expected response header %q to be 'unreal-trace-999', got %v", TraceIDMetadataKey, gotTrace)
	}

	if !strings.Contains(logBuf.String(), "trace_id=unreal-trace-999") {
		t.Fatalf("expected structured log to contain trace_id=unreal-trace-999, got %q", logBuf.String())
	}
}

func TestGRPCServerValidateJoinTicketAndMarkStartedSuccess(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	mockValidate := &mockValidateTicketRunner{
		result: matchmakingdomain.ValidatedTicket{
			Valid:                   true,
			ReasonCode:              "OK",
			PlayerID:                "player-abc-123",
			DisplayName:             "ChupeteMaster",
			AssignedTeam:            matchmakingdomain.TeamSideA,
			EquippedSkinCode:        "bean_knight",
			EquippedDeathEffectCode: "gore_fountain",
			CurrentMMR:              1250,
		},
	}
	mockMarkStarted := &mockMarkMatchStartedRunner{}

	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	srv := NewServer(logger, NewGameBackendAdapter(logger, mockValidate, mockMarkStarted))
	defer srv.Stop()

	go func() {
		_ = srv.Serve(lis)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer conn.Close()

	gameClient := gamev1.NewGameBackendServiceClient(conn)

	// 1. Probar ValidateJoinTicket exitoso
	valResp, err := gameClient.ValidateJoinTicket(ctx, &gamev1.ValidateJoinTicketRequest{
		TraceId:    "trace-grpc-val",
		MatchId:    "match-xyz",
		JoinTicket: "ticket-valid-token",
	})
	if err != nil {
		t.Fatalf("error en ValidateJoinTicket: %v", err)
	}
	if !valResp.GetValid() || valResp.GetReasonCode() != "OK" {
		t.Fatalf("se esperaba ticket válido con código OK, se obtuvo: %+v", valResp)
	}
	if valResp.GetPlayerId() != "player-abc-123" || valResp.GetAssignedTeam() != gamev1.TeamSide_TEAM_SIDE_A {
		t.Fatalf("datos de respuesta erróneos: %+v", valResp)
	}
	if valResp.GetEquippedSkinCode() != "bean_knight" || valResp.GetCurrentMmr() != 1250 {
		t.Fatalf("skin o MMR no coinciden: %+v", valResp)
	}

	// 2. Probar MarkMatchStarted exitoso
	startResp, err := gameClient.MarkMatchStarted(ctx, &gamev1.MarkMatchStartedRequest{
		TraceId:          "trace-grpc-start",
		MatchId:          "match-xyz",
		ServerInstanceId: "dedicated-server-01",
	})
	if err != nil {
		t.Fatalf("error en MarkMatchStarted: %v", err)
	}
	if !startResp.GetSuccess() || startResp.GetResultCode() != "MATCH_STARTED" {
		t.Fatalf("se esperaba éxito al iniciar partida, se obtuvo: %+v", startResp)
	}
}

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

	"server/internal/shared/pb/gamev1"
)

func TestGRPCServerStartsAndServesHealthAndGameBackendContract(t *testing.T) {
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to open TCP listener: %v", err)
	}
	defer listener.Close()

	srv := NewServer(logger, NewGameBackendAdapter(logger))
	defer srv.Stop()

	go func() {
		_ = srv.Serve(listener)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	conn, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to create gRPC client: %v", err)
	}
	defer conn.Close()

	// 1. Verificar que el servicio de salud de gRPC reporte SERVING.
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

	// 2. Verificar llamada al contrato GameBackendService y propagación de trace_id.
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
		t.Fatalf("expected Unimplemented status code before MVP-008, got %v (err=%v)", status.Code(err), err)
	}

	if gotTrace := responseHeaders.Get(TraceIDMetadataKey); len(gotTrace) == 0 || gotTrace[0] != "unreal-trace-999" {
		t.Fatalf("expected response header %q to be 'unreal-trace-999', got %v", TraceIDMetadataKey, gotTrace)
	}

	if !strings.Contains(logBuf.String(), "trace_id=unreal-trace-999") {
		t.Fatalf("expected structured log to contain trace_id=unreal-trace-999, got %q", logBuf.String())
	}
}

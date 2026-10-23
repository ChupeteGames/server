package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"server/internal/health/application"
	"server/internal/health/domain"
)

type staticHealthReader struct {
	result  domain.Health
	traceID string
}

func (r *staticHealthReader) Execute(_ context.Context, traceID string) domain.Health {
	r.traceID = traceID
	return r.result
}

func TestHealthEndpoint(t *testing.T) {
	useCase := &staticHealthReader{result: domain.NewHealthyStatus()}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	handler := NewRouter(logger, useCase)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	request.Header.Set(traceIDHeaderKey, "trace-test-123")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("expected application/json content type, got %q", contentType)
	}
	if body := strings.TrimSpace(response.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("expected health response body, got %q", body)
	}
	if useCase.traceID != "trace-test-123" {
		t.Fatalf("expected trace ID to reach use case, got %q", useCase.traceID)
	}
	if response.Header().Get(traceIDHeaderKey) != "trace-test-123" {
		t.Fatal("expected trace ID to be returned in the response header")
	}
	if !strings.Contains(logs.String(), "trace_id=trace-test-123") {
		t.Fatalf("expected trace ID in structured log, got %q", logs.String())
	}
}

func TestHealthEndpointRejectsNonGetMethod(t *testing.T) {
	handler := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), &staticHealthReader{result: domain.NewHealthyStatus()})
	request := httptest.NewRequest(http.MethodPost, "/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, response.Code)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, allow)
	}
}

func TestHealthEndpointGeneratesTraceIDWhenMissing(t *testing.T) {
	useCase := &staticHealthReader{result: domain.NewHealthyStatus()}
	handler := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), useCase)
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	traceID := response.Header().Get(traceIDHeaderKey)
	if len(traceID) != 32 {
		t.Fatalf("expected generated 32-character trace ID, got %q", traceID)
	}
	if useCase.traceID != traceID {
		t.Fatalf("expected generated trace ID %q to reach use case, got %q", traceID, useCase.traceID)
	}
}

var _ application.HealthUseCase = (*staticHealthReader)(nil)

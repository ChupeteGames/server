package logging

import "context"

// Logger is the structured logging capability used by inbound adapters.
type Logger interface {
	// InfoContext writes an informational message with structured attributes.
	InfoContext(ctx context.Context, msg string, args ...any)
	// ErrorContext writes an error message with structured attributes.
	ErrorContext(ctx context.Context, msg string, args ...any)
}

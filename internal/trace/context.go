package trace

import (
	"context"
	"log/slog"
	"strings"
)

type contextKey string

const (
	requestIDKey   contextKey = "request_id"
	operationIDKey contextKey = "operation_id"
)

func WithRequestID(ctx context.Context, requestID string) context.Context {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, requestID)
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func WithOperationID(ctx context.Context, operationID string) context.Context {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return ctx
	}
	return context.WithValue(ctx, operationIDKey, operationID)
}

func OperationID(ctx context.Context) string {
	value, _ := ctx.Value(operationIDKey).(string)
	return value
}

func LogAttrs(ctx context.Context) []slog.Attr {
	attrs := make([]slog.Attr, 0, 2)
	if requestID := RequestID(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}
	if operationID := OperationID(ctx); operationID != "" {
		attrs = append(attrs, slog.String("operation_id", operationID))
	}
	return attrs
}

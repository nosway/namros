package trace

import (
	"context"
	"testing"
)

func TestContextCorrelationIDs(t *testing.T) {
	ctx := WithRequestID(context.Background(), " request-1 ")
	ctx = WithOperationID(ctx, " operation-1 ")

	if got := RequestID(ctx); got != "request-1" {
		t.Fatalf("RequestID() = %q, want request-1", got)
	}
	if got := OperationID(ctx); got != "operation-1" {
		t.Fatalf("OperationID() = %q, want operation-1", got)
	}
	attrs := LogAttrs(ctx)
	if len(attrs) != 2 || attrs[0].Key != "request_id" || attrs[1].Key != "operation_id" {
		t.Fatalf("LogAttrs() = %+v", attrs)
	}
}

func TestContextCorrelationIDsIgnoreEmptyValues(t *testing.T) {
	ctx := WithRequestID(context.Background(), " ")
	ctx = WithOperationID(ctx, "")

	if got := RequestID(ctx); got != "" {
		t.Fatalf("RequestID() = %q, want empty", got)
	}
	if got := OperationID(ctx); got != "" {
		t.Fatalf("OperationID() = %q, want empty", got)
	}
	if attrs := LogAttrs(ctx); len(attrs) != 0 {
		t.Fatalf("LogAttrs() = %+v, want empty", attrs)
	}
}

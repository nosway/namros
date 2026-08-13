package gateway

import (
	"testing"

	"github.com/nosway/namros/internal/config"
)

func TestDataBudgetRejectsByteLimitAndReleases(t *testing.T) {
	budget := newDataBudget(config.Config{
		GatewayDataBudgetBytes:        10,
		GatewayDataBudgetUnknownBytes: 4,
	})
	release, reason, ok := budget.tryAcquire(6, true)
	if !ok {
		t.Fatalf("tryAcquire(first) rejected reason=%q", reason)
	}
	if _, reason, ok := budget.tryAcquire(5, true); ok || reason != "bytes_limit" {
		t.Fatalf("tryAcquire(over budget) ok=%v reason=%q, want bytes_limit", ok, reason)
	}
	release()
	if release, reason, ok := budget.tryAcquire(10, true); !ok {
		t.Fatalf("tryAcquire(after release) rejected reason=%q", reason)
	} else {
		release()
	}
}

func TestDataBudgetRejectsRequestLimit(t *testing.T) {
	budget := newDataBudget(config.Config{
		GatewayDataBudgetMaxRequests:  1,
		GatewayDataBudgetUnknownBytes: config.DefaultGatewayDataBudgetUnknownBytes,
	})
	release, reason, ok := budget.tryAcquire(0, true)
	if !ok {
		t.Fatalf("tryAcquire(first) rejected reason=%q", reason)
	}
	defer release()
	if _, reason, ok := budget.tryAcquire(0, true); ok || reason != "request_limit" {
		t.Fatalf("tryAcquire(over request limit) ok=%v reason=%q, want request_limit", ok, reason)
	}
}

func TestDataBudgetUnknownSizeReservation(t *testing.T) {
	budget := newDataBudget(config.Config{
		GatewayDataBudgetBytes:        8,
		GatewayDataBudgetUnknownBytes: 4,
	})
	first, reason, ok := budget.tryAcquire(0, false)
	if !ok {
		t.Fatalf("tryAcquire(first unknown) rejected reason=%q", reason)
	}
	defer first()
	second, reason, ok := budget.tryAcquire(0, false)
	if !ok {
		t.Fatalf("tryAcquire(second unknown) rejected reason=%q", reason)
	}
	defer second()
	if _, reason, ok := budget.tryAcquire(0, false); ok || reason != "bytes_limit" {
		t.Fatalf("tryAcquire(third unknown) ok=%v reason=%q, want bytes_limit", ok, reason)
	}
}

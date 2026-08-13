package workerscheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
)

func TestBudgetNilAndDisabledAllowWork(t *testing.T) {
	if release, reason, ok := (*Budget)(nil).TryAcquire(BudgetScope{}); !ok || reason != "" {
		t.Fatalf("nil budget acquire = ok:%v reason:%q, want allowed", ok, reason)
	} else {
		release()
	}
	if budget := NewBudget(BudgetConfig{}); budget != nil {
		t.Fatalf("NewBudget(disabled) = %#v, want nil", budget)
	}
}

func TestBudgetEnforcesGlobalLimit(t *testing.T) {
	budget := NewBudget(BudgetConfig{MaxConcurrent: 1})
	release, reason, ok := budget.TryAcquire(BudgetScope{WorkerKind: "gc", ShardID: "orphans"})
	if !ok {
		t.Fatalf("first acquire denied: %s", reason)
	}
	defer release()
	if _, reason, ok := budget.TryAcquire(BudgetScope{WorkerKind: "lifecycle", ShardID: "buckets"}); ok || reason != "global_concurrency" {
		t.Fatalf("second acquire = ok:%v reason:%q, want global throttle", ok, reason)
	}
}

func TestBudgetEnforcesTenantLimit(t *testing.T) {
	budget := NewBudget(BudgetConfig{MaxConcurrentPerTenant: 1})
	release, reason, ok := budget.TryAcquire(BudgetScope{TenantID: "tenant-a"})
	if !ok {
		t.Fatalf("first tenant acquire denied: %s", reason)
	}
	defer release()
	if _, reason, ok := budget.TryAcquire(BudgetScope{TenantID: "tenant-a"}); ok || reason != "tenant_concurrency" {
		t.Fatalf("same tenant acquire = ok:%v reason:%q, want tenant throttle", ok, reason)
	}
	if releaseOther, reason, ok := budget.TryAcquire(BudgetScope{TenantID: "tenant-b"}); !ok {
		t.Fatalf("different tenant acquire denied: %s", reason)
	} else {
		releaseOther()
	}
}

func TestBudgetEnforcesPoolLimit(t *testing.T) {
	budget := NewBudget(BudgetConfig{MaxConcurrentPerPool: 1})
	release, reason, ok := budget.TryAcquire(BudgetScope{PoolID: "pool-a"})
	if !ok {
		t.Fatalf("first pool acquire denied: %s", reason)
	}
	defer release()
	if _, reason, ok := budget.TryAcquire(BudgetScope{PoolID: "pool-a"}); ok || reason != "pool_concurrency" {
		t.Fatalf("same pool acquire = ok:%v reason:%q, want pool throttle", ok, reason)
	}
	if releaseOther, reason, ok := budget.TryAcquire(BudgetScope{PoolID: "pool-b"}); !ok {
		t.Fatalf("different pool acquire denied: %s", reason)
	} else {
		releaseOther()
	}
}

func TestBudgetReleaseRestoresCapacityAndSnapshot(t *testing.T) {
	budget := NewBudget(BudgetConfig{
		MaxConcurrent:          1,
		MaxConcurrentPerTenant: 1,
		MaxConcurrentPerPool:   1,
	})
	release, reason, ok := budget.TryAcquire(BudgetScope{TenantID: "tenant-a", PoolID: "pool-a"})
	if !ok {
		t.Fatalf("acquire denied: %s", reason)
	}
	snapshot := budget.Snapshot()
	if snapshot.InUse != 1 || snapshot.InUseByTenant["tenant-a"] != 1 || snapshot.InUseByPool["pool-a"] != 1 {
		t.Fatalf("snapshot while acquired = %+v", snapshot)
	}
	release()
	release()
	snapshot = budget.Snapshot()
	if snapshot.InUse != 0 || len(snapshot.InUseByTenant) != 0 || len(snapshot.InUseByPool) != 0 {
		t.Fatalf("snapshot after release = %+v, want empty", snapshot)
	}
	if releaseAgain, reason, ok := budget.TryAcquire(BudgetScope{TenantID: "tenant-a", PoolID: "pool-a"}); !ok {
		t.Fatalf("reacquire denied after release: %s", reason)
	} else {
		releaseAgain()
	}
}

func TestRunnerRunOnceReturnsThrottleWithoutLeaseOrOperation(t *testing.T) {
	repo := memory.New()
	budget := NewBudget(BudgetConfig{MaxConcurrent: 1})
	release, reason, ok := budget.TryAcquire(BudgetScope{WorkerKind: "gc", ShardID: "orphans"})
	if !ok {
		t.Fatalf("seed budget acquire denied: %s", reason)
	}
	defer release()
	runner := Runner{
		Repository: repo,
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
		Budget:     budget,
	}
	_, err := runner.RunOnce(t.Context(), "", func(context.Context, model.WorkerLease) (Result, error) {
		t.Fatal("work should not run while worker budget is exhausted")
		return Result{}, nil
	})
	if !errors.Is(err, ErrThrottled) {
		t.Fatalf("RunOnce(throttled) error = %v, want ErrThrottled", err)
	}
	records, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want none", records)
	}
	if _, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	}); err != nil {
		t.Fatalf("AcquireWorkerLease(after throttled runner) error = %v", err)
	}
}

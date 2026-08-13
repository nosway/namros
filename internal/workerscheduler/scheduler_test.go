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

func TestRunnerRunOnceRecordsSuccessAndReleasesLease(t *testing.T) {
	repo := memory.New()
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	runner := Runner{
		Repository: repo,
		WorkerKind: "gc",
		ShardID:    "shard-1",
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
		Clock: func() time.Time {
			now = now.Add(time.Second)
			return now
		},
	}

	record, err := runner.RunOnce(t.Context(), "cursor-0", func(ctx context.Context, lease model.WorkerLease) (Result, error) {
		if lease.LeaseID != "gc/shard-1" || lease.Cursor != "cursor-0" {
			t.Fatalf("lease = %+v", lease)
		}
		return Result{Cursor: "cursor-1", Scanned: 5, Processed: 4, Skipped: 1}, nil
	})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.Status != model.WorkerOperationSucceeded || record.Cursor != "cursor-1" || record.Processed != 4 {
		t.Fatalf("record = %+v", record)
	}

	next, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-1",
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(after runner release) error = %v", err)
	}
	if next.Generation != 2 || next.OwnerID != "gateway-b" {
		t.Fatalf("next lease = %+v, want generation 2 owner gateway-b", next)
	}
}

func TestRunnerRunOnceCanHoldLease(t *testing.T) {
	repo := memory.New()
	runner := Runner{
		Repository: repo,
		WorkerKind: "gc",
		ShardID:    "shard-1",
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
		HoldLease:  true,
	}

	if _, err := runner.RunOnce(t.Context(), "", func(context.Context, model.WorkerLease) (Result, error) {
		return Result{Scanned: 1}, nil
	}); err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if _, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-1",
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("AcquireWorkerLease(contender) error = %v, want CAS conflict", err)
	}
	renewed, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-1",
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(renew owner) error = %v", err)
	}
	if renewed.OwnerID != "gateway-a" || renewed.Generation != 1 {
		t.Fatalf("renewed lease = %+v, want same owner generation 1", renewed)
	}
}

func TestRunnerRunOnceReturnsLeaseConflictWithoutOperation(t *testing.T) {
	repo := memory.New()
	_, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-1",
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(seed) error = %v", err)
	}
	runner := Runner{Repository: repo, WorkerKind: "gc", ShardID: "shard-1", OwnerID: "gateway-b", TTL: time.Minute}
	_, err = runner.RunOnce(t.Context(), "", func(context.Context, model.WorkerLease) (Result, error) {
		t.Fatal("work should not run when lease is held")
		return Result{}, nil
	})
	if !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("RunOnce(conflict) error = %v, want CAS conflict", err)
	}
	records, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records = %+v, want none", records)
	}
}

func TestRunnerRunOnceReturnsPausedControlWithoutOperation(t *testing.T) {
	repo := memory.New()
	if _, err := repo.PutWorkerControl(t.Context(), meta.PutWorkerControlRequest{
		WorkerKind: "gc",
		ShardID:    "shard-1",
		State:      model.WorkerControlPaused,
		Reason:     "operator maintenance",
	}); err != nil {
		t.Fatalf("PutWorkerControl(paused) error = %v", err)
	}
	runner := Runner{Repository: repo, WorkerKind: "gc", ShardID: "shard-1", OwnerID: "gateway-a", TTL: time.Minute}
	_, err := runner.RunOnce(t.Context(), "", func(context.Context, model.WorkerLease) (Result, error) {
		t.Fatal("work should not run while paused")
		return Result{}, nil
	})
	if !errors.Is(err, meta.ErrWorkerPaused) {
		t.Fatalf("RunOnce(paused) error = %v, want ErrWorkerPaused", err)
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
		ShardID:    "shard-1",
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	}); err != nil {
		t.Fatalf("AcquireWorkerLease(after paused runner) error = %v", err)
	}
}

func TestRunnerRunOnceReturnsCanceledControlWithoutOperation(t *testing.T) {
	repo := memory.New()
	if _, err := repo.PutWorkerControl(t.Context(), meta.PutWorkerControlRequest{
		WorkerKind: "lifecycle",
		ShardID:    "bucket-a",
		State:      model.WorkerControlCanceled,
	}); err != nil {
		t.Fatalf("PutWorkerControl(canceled) error = %v", err)
	}
	runner := Runner{Repository: repo, WorkerKind: "lifecycle", ShardID: "bucket-a", OwnerID: "gateway-a", TTL: time.Minute}
	_, err := runner.RunOnce(t.Context(), "", func(context.Context, model.WorkerLease) (Result, error) {
		t.Fatal("work should not run while canceled")
		return Result{}, nil
	})
	if !errors.Is(err, meta.ErrWorkerCanceled) {
		t.Fatalf("RunOnce(canceled) error = %v, want ErrWorkerCanceled", err)
	}
}

func TestRunnerRunOnceRecordsRetryableFailure(t *testing.T) {
	repo := memory.New()
	boom := errors.New("temporary backend error")
	runner := Runner{Repository: repo, WorkerKind: "lifecycle", ShardID: "bucket-a", OwnerID: "gateway-a", TTL: time.Minute}

	record, err := runner.RunOnce(t.Context(), "k=10", func(context.Context, model.WorkerLease) (Result, error) {
		return Result{Cursor: "k=11", Scanned: 10, Processed: 8, Retryable: 2}, boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("RunOnce() error = %v, want boom", err)
	}
	if record.Status != model.WorkerOperationRetryPending || record.LastError != boom.Error() || record.Retryable != 2 {
		t.Fatalf("record = %+v, want retry_pending with error", record)
	}
}

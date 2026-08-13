package workerops

import (
	"context"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
)

func TestSummarizeWorkerBacklogIncludesLeasesRetryableAndLastError(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewWithClock(func() time.Time {
		return time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	})
	lease, err := repo.AcquireWorkerLease(ctx, meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "gateway-a",
		TTL:        5 * time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease() error = %v", err)
	}
	if _, err := repo.PutWorkerOperation(ctx, meta.PutWorkerOperationRequest{
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "gateway-a",
		LeaseID:    lease.LeaseID,
		Status:     model.WorkerOperationRetryPending,
		Retryable:  2,
		LastError:  "temporary storage error",
		StartedAt:  time.Date(2026, 8, 10, 2, 59, 30, 0, time.UTC),
		FinishedAt: time.Date(2026, 8, 10, 2, 59, 40, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutWorkerOperation(retry) error = %v", err)
	}
	if _, err := repo.PutWorkerOperation(ctx, meta.PutWorkerOperationRequest{
		WorkerKind: "lifecycle",
		ShardID:    "bucket-a",
		OwnerID:    "gateway-b",
		Status:     model.WorkerOperationSucceeded,
		Processed:  3,
		FinishedAt: time.Date(2026, 8, 10, 2, 58, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("PutWorkerOperation(success) error = %v", err)
	}

	snapshot, err := Summarize(ctx, repo, Config{
		Now: func() time.Time { return time.Date(2026, 8, 10, 3, 1, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if snapshot.SchemaVersion != "namros.worker.backlog.v1" || snapshot.Status != "ok" || snapshot.OperationCount != 2 || snapshot.LeaseCount != 1 {
		t.Fatalf("snapshot envelope = %+v", snapshot)
	}
	if snapshot.Totals.BacklogOperations != 1 || snapshot.Totals.RetryPendingOperations != 1 || snapshot.Totals.Retryable != 2 || snapshot.Totals.LeasesFresh != 1 {
		t.Fatalf("snapshot totals = %+v", snapshot.Totals)
	}
	var gcWorker WorkerSummary
	for _, worker := range snapshot.Workers {
		if worker.WorkerKind == "gc" && worker.ShardID == "orphans" {
			gcWorker = worker
		}
	}
	if gcWorker.WorkerKind == "" {
		t.Fatalf("gc worker missing from snapshot: %+v", snapshot.Workers)
	}
	if gcWorker.OwnerID != "gateway-a" || gcWorker.LeaseID != "gc/orphans" || !gcWorker.LeaseFresh || gcWorker.LeaseAgeSeconds != 60 || gcWorker.LeaseExpiresInSeconds != 240 {
		t.Fatalf("gc worker lease summary = %+v", gcWorker)
	}
	if gcWorker.CurrentStatus != "retry_pending" || gcWorker.BacklogOperations != 1 || gcWorker.Retryable != 2 || gcWorker.LastError != "temporary storage error" {
		t.Fatalf("gc worker backlog summary = %+v", gcWorker)
	}
}

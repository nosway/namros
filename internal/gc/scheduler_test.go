package gc

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/workerscheduler"
)

func TestSchedulerRunOnceRecordsWorkerOperationAndGCOperation(t *testing.T) {
	now := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("delete-me", storage.DeleteReasonPublishFailed),
	})
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker: Worker{
			Storage:        store,
			Orphans:        store,
			OperationStore: repo,
			Now:            func() time.Time { return now },
		},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
		Status: status,
		Clock:  func() time.Time { return now },
	}

	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Skipped {
		t.Fatalf("RunOnce() skipped = true, reason=%q", result.SkipReason)
	}
	if result.WorkerOperation.OperationID == "" || result.WorkerOperation.WorkerKind != DefaultWorkerKind || result.WorkerOperation.ShardID != DefaultWorkerShardID {
		t.Fatalf("worker operation = %+v", result.WorkerOperation)
	}
	if result.WorkerOperation.Status != model.WorkerOperationSucceeded || result.WorkerOperation.Scanned != 1 || result.WorkerOperation.Processed != 1 {
		t.Fatalf("worker operation counters = %+v", result.WorkerOperation)
	}
	if result.GCOperation.Status != model.GCOperationSucceeded || result.GCOperation.Deleted != 1 {
		t.Fatalf("gc operation = %+v", result.GCOperation)
	}

	workerRecords, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{
		WorkerKind: DefaultWorkerKind,
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(workerRecords) != 1 || workerRecords[0].OperationID != result.WorkerOperation.OperationID {
		t.Fatalf("worker records = %+v", workerRecords)
	}
	gcRecords, err := repo.ListGCOperations(t.Context(), meta.ListGCOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCOperations() error = %v", err)
	}
	if len(gcRecords) != 1 || gcRecords[0].Status != model.GCOperationSucceeded || gcRecords[0].Deleted != 1 {
		t.Fatalf("gc records = %+v", gcRecords)
	}

	snapshot := status.Snapshot()
	if !snapshot.Enabled || snapshot.OwnerID != "gateway-a" || snapshot.WorkerKind != DefaultWorkerKind || snapshot.ShardID != DefaultWorkerShardID {
		t.Fatalf("status config = %+v", snapshot)
	}
	if snapshot.Runs != 1 || snapshot.Successes != 1 || snapshot.LastOperationID != result.WorkerOperation.OperationID || snapshot.LastDeleted != 1 {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceSkipsWhenLeaseHeld(t *testing.T) {
	repo := memory.New()
	if _, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: DefaultWorkerKind,
		ShardID:    DefaultWorkerShardID,
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
	}); err != nil {
		t.Fatalf("AcquireWorkerLease(seed) error = %v", err)
	}
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("still-owned", storage.DeleteReasonManualGC),
	})
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker:     Worker{Storage: store, Orphans: store},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-b",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
		Status: status,
	}

	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !result.Skipped || result.SkipReason != "lease_held" {
		t.Fatalf("RunOnce() result = %+v, want lease-held skip", result)
	}
	remaining, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining candidates = %+v, want original candidate", remaining)
	}
	workerRecords, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(workerRecords) != 0 {
		t.Fatalf("worker records = %+v, want none", workerRecords)
	}
	snapshot := status.Snapshot()
	if snapshot.Runs != 1 || snapshot.Skipped != 1 || !snapshot.LastSkipped || snapshot.LastSkipReason != "lease_held" {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceSkipsWhenPaused(t *testing.T) {
	repo := memory.New()
	if _, err := repo.PutWorkerControl(t.Context(), meta.PutWorkerControlRequest{
		WorkerKind: DefaultWorkerKind,
		ShardID:    DefaultWorkerShardID,
		State:      model.WorkerControlPaused,
		Reason:     "maintenance",
	}); err != nil {
		t.Fatalf("PutWorkerControl(paused) error = %v", err)
	}
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("still-paused", storage.DeleteReasonManualGC),
	})
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker:     Worker{Storage: store, Orphans: store},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
		Status: status,
	}

	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !result.Skipped || result.SkipReason != "paused" {
		t.Fatalf("RunOnce() result = %+v, want paused skip", result)
	}
	remaining, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining candidates = %+v, want original candidate", remaining)
	}
	snapshot := status.Snapshot()
	if snapshot.Runs != 1 || snapshot.Skipped != 1 || !snapshot.LastSkipped || snapshot.LastSkipReason != "paused" {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceSkipsWhenThrottled(t *testing.T) {
	repo := memory.New()
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("still-throttled", storage.DeleteReasonManualGC),
	})
	budget := workerscheduler.NewBudget(workerscheduler.BudgetConfig{MaxConcurrent: 1})
	release, reason, ok := budget.TryAcquire(workerscheduler.BudgetScope{WorkerKind: DefaultWorkerKind, ShardID: DefaultWorkerShardID})
	if !ok {
		t.Fatalf("seed budget acquire denied: %s", reason)
	}
	defer release()
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker:     Worker{Storage: store, Orphans: store},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
		Status: status,
		Budget: budget,
	}

	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if !result.Skipped || result.SkipReason != "throttled" {
		t.Fatalf("RunOnce() result = %+v, want throttled skip", result)
	}
	remaining, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining candidates = %+v, want original candidate", remaining)
	}
	records, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{WorkerKind: DefaultWorkerKind, Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("worker records = %+v, want none", records)
	}
	snapshot := status.Snapshot()
	if snapshot.Runs != 1 || snapshot.Skipped != 1 || !snapshot.LastSkipped || snapshot.LastSkipReason != "throttled" {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceMapsRetryableGCToWorkerRetryPending(t *testing.T) {
	repo := memory.New()
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("retry-me", storage.DeleteReasonMultipartAborted),
	})
	store.deleteErrors["retry-me"] = storage.ErrUnavailable
	scheduler := Scheduler{
		Worker: Worker{
			Storage:        store,
			Orphans:        store,
			OperationStore: repo,
		},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
	}

	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.GCOperation.Status != model.GCOperationRetryPending || result.WorkerOperation.Status != model.WorkerOperationRetryPending {
		t.Fatalf("result statuses = gc:%q worker:%q", result.GCOperation.Status, result.WorkerOperation.Status)
	}
	if result.WorkerOperation.Retryable != 1 || result.WorkerOperation.LastError == "" {
		t.Fatalf("worker operation retry fields = %+v", result.WorkerOperation)
	}
}

func TestSchedulerRunOnceHoldsLeaseForShard(t *testing.T) {
	repo := memory.New()
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("delete-me", storage.DeleteReasonPublishFailed),
	})
	schedulerA := Scheduler{
		Worker:     Worker{Storage: store, Orphans: store},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
	}
	schedulerB := schedulerA
	schedulerB.Config.OwnerID = "gateway-b"

	if result, err := schedulerA.RunOnce(t.Context()); err != nil || result.WorkerOperation.Status != model.WorkerOperationSucceeded {
		t.Fatalf("schedulerA RunOnce() result = %+v error = %v", result, err)
	}
	result, err := schedulerB.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("schedulerB RunOnce() error = %v", err)
	}
	if !result.Skipped || result.SkipReason != "lease_held" {
		t.Fatalf("schedulerB result = %+v, want lease-held skip", result)
	}
	if result, err := schedulerA.RunOnce(t.Context()); err != nil || result.Skipped {
		t.Fatalf("schedulerA renew RunOnce() result = %+v error = %v", result, err)
	}
}

func TestSchedulerRunOnceRecordsWorkerErrorAndHoldsLease(t *testing.T) {
	repo := memory.New()
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("candidate", storage.DeleteReasonManualGC),
	})
	scheduler := Scheduler{
		Worker: Worker{
			Storage: store,
		},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			Limit:    10,
		},
	}

	result, err := scheduler.RunOnce(t.Context())
	if err == nil || !strings.Contains(err.Error(), "orphan tracker is required") {
		t.Fatalf("RunOnce() error = %v, want orphan tracker required", err)
	}
	if result.WorkerOperation.Status != model.WorkerOperationFailed || result.WorkerOperation.LastError == "" {
		t.Fatalf("worker operation = %+v, want failed with last error", result.WorkerOperation)
	}
	if _, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: DefaultWorkerKind,
		ShardID:    DefaultWorkerShardID,
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("AcquireWorkerLease(contender) error = %v, want CAS conflict", err)
	}
}

func TestSchedulerResultFromGCRecordKeepsWorkerFailureStatus(t *testing.T) {
	boom := errors.New("boom")
	result := schedulerResultFromGCRecord(OperationRecord{Scanned: 1}, boom)
	if result.Status != model.WorkerOperationFailed || result.LastError != boom.Error() {
		t.Fatalf("scheduler result = %+v, want failed boom", result)
	}
}

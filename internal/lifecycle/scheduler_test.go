package lifecycle

import (
	"errors"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/workerscheduler"
)

func TestSchedulerRunOnceRecordsWorkerOperationAndLifecycleWork(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	bucket := createLifecycleWorkerBucket(t, repo)
	putLifecycleTestObject(t, repo, bucket.BucketID, "logs/current.txt", storage.SegmentRef{SegmentID: "segment-current", SizeBytes: 5})
	clock = now
	putExpirationLifecycleRule(t, repo, bucket.BucketID, "logs/", 30, 0)
	status := NewSchedulerStatus()
	store := &workerStore{}
	scheduler := Scheduler{
		Worker: Worker{
			Metadata: repo,
			Storage:  store,
			Now:      func() time.Time { return now },
		},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			BucketID: bucket.BucketID,
			LeaseTTL: time.Minute,
			Interval: time.Minute,
			MaxKeys:  100,
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
	if result.WorkerOperation.Status != model.WorkerOperationSucceeded || result.WorkerOperation.Scanned != 1 || result.WorkerOperation.Processed != 1 || result.WorkerOperation.Cursor != bucket.BucketID {
		t.Fatalf("worker operation counters = %+v", result.WorkerOperation)
	}
	if result.LifecycleWorker.Planned != 1 || result.LifecycleWorker.Processed != 1 || result.LifecycleWorker.Failed != 0 {
		t.Fatalf("lifecycle result = %+v", result.LifecycleWorker)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "segment-current" {
		t.Fatalf("deleted segments = %+v", store.deleted)
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
	snapshot := status.Snapshot()
	if !snapshot.Enabled || snapshot.OwnerID != "gateway-a" || snapshot.WorkerKind != DefaultWorkerKind || snapshot.ShardID != DefaultWorkerShardID || snapshot.BucketID != bucket.BucketID {
		t.Fatalf("status config = %+v", snapshot)
	}
	if snapshot.Runs != 1 || snapshot.Successes != 1 || snapshot.LastOperationID != result.WorkerOperation.OperationID || snapshot.LastProcessed != 1 {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceSkipsWhenLeaseHeld(t *testing.T) {
	repo := memory.New()
	bucket := createLifecycleWorkerBucket(t, repo)
	if _, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: DefaultWorkerKind,
		ShardID:    DefaultWorkerShardID,
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
	}); err != nil {
		t.Fatalf("AcquireWorkerLease(seed) error = %v", err)
	}
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker:     Worker{Metadata: repo, Storage: &workerStore{}},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-b",
			BucketID: bucket.BucketID,
			LeaseTTL: time.Minute,
			Interval: time.Minute,
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
	records, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{WorkerKind: DefaultWorkerKind, Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("worker records = %+v, want none", records)
	}
	snapshot := status.Snapshot()
	if snapshot.Runs != 1 || snapshot.Skipped != 1 || !snapshot.LastSkipped || snapshot.LastSkipReason != "lease_held" {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceSkipsWhenPaused(t *testing.T) {
	repo := memory.New()
	bucket := createLifecycleWorkerBucket(t, repo)
	if _, err := repo.PutWorkerControl(t.Context(), meta.PutWorkerControlRequest{
		WorkerKind: DefaultWorkerKind,
		ShardID:    DefaultWorkerShardID,
		State:      model.WorkerControlPaused,
		Reason:     "maintenance",
	}); err != nil {
		t.Fatalf("PutWorkerControl(paused) error = %v", err)
	}
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker:     Worker{Metadata: repo, Storage: &workerStore{}},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			BucketID: bucket.BucketID,
			LeaseTTL: time.Minute,
			Interval: time.Minute,
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
	records, err := repo.ListWorkerOperations(t.Context(), meta.ListWorkerOperationsRequest{WorkerKind: DefaultWorkerKind, Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("worker records = %+v, want none", records)
	}
	snapshot := status.Snapshot()
	if snapshot.Runs != 1 || snapshot.Skipped != 1 || !snapshot.LastSkipped || snapshot.LastSkipReason != "paused" {
		t.Fatalf("status snapshot = %+v", snapshot)
	}
}

func TestSchedulerRunOnceSkipsWhenThrottled(t *testing.T) {
	repo := memory.New()
	bucket := createLifecycleWorkerBucket(t, repo)
	budget := workerscheduler.NewBudget(workerscheduler.BudgetConfig{MaxConcurrent: 1})
	release, reason, ok := budget.TryAcquire(workerscheduler.BudgetScope{WorkerKind: DefaultWorkerKind, ShardID: DefaultWorkerShardID})
	if !ok {
		t.Fatalf("seed budget acquire denied: %s", reason)
	}
	defer release()
	status := NewSchedulerStatus()
	scheduler := Scheduler{
		Worker:     Worker{Metadata: repo, Storage: &workerStore{}},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			BucketID: bucket.BucketID,
			LeaseTTL: time.Minute,
			Interval: time.Minute,
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

func TestSchedulerRunOnceHoldsLeaseForShard(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	bucket := createLifecycleWorkerBucket(t, repo)
	putLifecycleTestObject(t, repo, bucket.BucketID, "logs/current.txt", storage.SegmentRef{SegmentID: "segment-current", SizeBytes: 5})
	clock = now
	putExpirationLifecycleRule(t, repo, bucket.BucketID, "logs/", 30, 0)
	store := &workerStore{}
	schedulerA := Scheduler{
		Worker:     Worker{Metadata: repo, Storage: store, Now: func() time.Time { return now }},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			BucketID: bucket.BucketID,
			LeaseTTL: time.Minute,
			Interval: time.Minute,
		},
		Clock: func() time.Time { return now },
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
	if _, err := repo.AcquireWorkerLease(t.Context(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: DefaultWorkerKind,
		ShardID:    DefaultWorkerShardID,
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("AcquireWorkerLease(contender) error = %v, want CAS conflict", err)
	}
}

func TestSchedulerRunOnceMapsLifecycleFailuresToRetryPending(t *testing.T) {
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	bucket := createLifecycleWorkerBucket(t, repo)
	putLifecycleTestObject(t, repo, bucket.BucketID, "logs/current.txt", storage.SegmentRef{SegmentID: "segment-current", SizeBytes: 5})
	clock = now
	putExpirationLifecycleRule(t, repo, bucket.BucketID, "logs/", 30, 0)
	scheduler := Scheduler{
		Worker:     Worker{Metadata: repo, Now: func() time.Time { return now }},
		Repository: repo,
		Config: SchedulerConfig{
			OwnerID:  "gateway-a",
			BucketID: bucket.BucketID,
			LeaseTTL: time.Minute,
			Interval: time.Minute,
		},
		Clock: func() time.Time { return now },
	}

	result, err := scheduler.RunOnce(t.Context())
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.WorkerOperation.Status != model.WorkerOperationRetryPending || result.WorkerOperation.Retryable != 1 || result.WorkerOperation.LastError == "" {
		t.Fatalf("worker operation = %+v, want retry_pending with retryable failure", result.WorkerOperation)
	}
	if result.LifecycleWorker.Failed != 1 {
		t.Fatalf("lifecycle result = %+v, want failed segment delete", result.LifecycleWorker)
	}
}

func TestNormalizeSchedulerConfigValidatesRequiredFields(t *testing.T) {
	if _, err := normalizeSchedulerConfig(SchedulerConfig{}); err == nil {
		t.Fatal("normalizeSchedulerConfig() error = nil, want missing owner")
	}
	if _, err := normalizeSchedulerConfig(SchedulerConfig{OwnerID: "gateway-a"}); err == nil {
		t.Fatal("normalizeSchedulerConfig() error = nil, want missing bucket")
	}
	if _, err := normalizeSchedulerConfig(SchedulerConfig{OwnerID: "gateway-a", BucketID: "bucket-1", LeaseTTL: time.Minute, Interval: time.Minute}); err != nil {
		t.Fatalf("normalizeSchedulerConfig(valid) error = %v", err)
	}
}

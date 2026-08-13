package gc

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func TestWorkerRecordsDeletedSkippedAndRetryableAttempts(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("delete-me", storage.DeleteReasonPublishFailed),
		gcCandidate("protected", storage.DeleteReasonManualGC),
		gcCandidate("retry-me", storage.DeleteReasonMultipartAborted),
	})
	store.deleteErrors["retry-me"] = storage.ErrUnavailable
	worker := Worker{
		Storage: store,
		Orphans: store,
		Admission: func(_ context.Context, ref storage.SegmentRef) error {
			if ref.SegmentID == "protected" {
				return storage.ErrProtected
			}
			return nil
		},
		Now: func() time.Time { return now },
	}

	record, err := worker.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.Scanned != 3 || record.Deleted != 1 || record.Skipped != 1 || record.Retryable != 1 {
		t.Fatalf("record = %+v", record)
	}
	if record.Status != model.GCOperationRetryPending {
		t.Fatalf("record status = %q, want retry_pending", record.Status)
	}
	if record.StartedAt.IsZero() || record.FinishedAt.IsZero() {
		t.Fatalf("record timestamps not set: %+v", record)
	}
	if len(record.Attempts) != 3 {
		t.Fatalf("attempts = %+v", record.Attempts)
	}
	if got := record.Attempts[0]; got.SegmentID != "delete-me" || got.Status != AttemptDeleted || got.Retryable {
		t.Fatalf("delete attempt = %+v", got)
	}
	if got := record.Attempts[1]; got.SegmentID != "protected" || got.Status != AttemptSkipped || got.Retryable || got.Error == "" {
		t.Fatalf("protected attempt = %+v", got)
	}
	if got := record.Attempts[2]; got.SegmentID != "retry-me" || got.Status != AttemptRetryable || !got.Retryable || got.Error == "" {
		t.Fatalf("retry attempt = %+v", got)
	}

	remaining, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(remaining) != 2 || remaining[0].Ref.SegmentID != "protected" || remaining[1].Ref.SegmentID != "retry-me" {
		t.Fatalf("remaining candidates = %+v", remaining)
	}
}

func TestWorkerDeletesRetryableCandidateOnLaterRun(t *testing.T) {
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("retry-me", storage.DeleteReasonMultipartAborted),
	})
	store.deleteErrors["retry-me"] = storage.ErrUnavailable
	worker := Worker{Storage: store, Orphans: store}

	first, err := worker.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce(first) error = %v", err)
	}
	if first.Retryable != 1 {
		t.Fatalf("first record = %+v", first)
	}
	delete(store.deleteErrors, "retry-me")
	second, err := worker.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce(second) error = %v", err)
	}
	if second.Deleted != 1 || second.Retryable != 0 {
		t.Fatalf("second record = %+v", second)
	}
	if second.Status != model.GCOperationSucceeded {
		t.Fatalf("second status = %q, want succeeded", second.Status)
	}
	remaining, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("remaining candidates = %+v, want none", remaining)
	}
}

func TestWorkerPersistsOperationRecordWhenStoreConfigured(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("delete-me", storage.DeleteReasonPublishFailed),
		gcCandidate("retry-me", storage.DeleteReasonMultipartAborted),
	})
	store.deleteErrors["retry-me"] = storage.ErrUnavailable
	operationStore := memory.NewWithClock(func() time.Time { return now })

	record, err := Worker{
		Storage:        store,
		Orphans:        store,
		OperationStore: operationStore,
		Now:            func() time.Time { return now },
	}.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.FinishedAt.IsZero() {
		t.Fatalf("returned record FinishedAt is zero: %+v", record)
	}
	records, err := operationStore.ListGCOperations(t.Context(), meta.ListGCOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCOperations() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("operation records = %+v, want one", records)
	}
	persisted := records[0]
	if persisted.Scanned != 2 || persisted.Deleted != 1 || persisted.Retryable != 1 || persisted.FinishedAt.IsZero() {
		t.Fatalf("persisted record = %+v", persisted)
	}
	if persisted.Status != model.GCOperationRetryPending {
		t.Fatalf("persisted status = %q, want retry_pending", persisted.Status)
	}
	if len(persisted.Attempts) != 2 || persisted.Attempts[1].Status != model.GCOperationAttemptRetryable || !persisted.Attempts[1].Retryable {
		t.Fatalf("persisted attempts = %+v", persisted.Attempts)
	}
}

func TestWorkerMarksResumeOfPreviousRetryableOperation(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("retry-me", storage.DeleteReasonMultipartAborted),
	})
	store.deleteErrors["retry-me"] = storage.ErrUnavailable
	operationStore := memory.NewWithClock(func() time.Time { return now })
	worker := Worker{
		Storage:        store,
		Orphans:        store,
		OperationStore: operationStore,
		Now:            func() time.Time { return now },
	}

	first, err := worker.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce(first) error = %v", err)
	}
	if first.Status != model.GCOperationRetryPending || first.ResumeOfOperationID != "" {
		t.Fatalf("first record = %+v, want retry_pending without resume marker", first)
	}
	records, err := operationStore.ListGCOperations(t.Context(), meta.ListGCOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListGCOperations(first) error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("operation records = %+v, want one", records)
	}
	firstOperationID := records[0].OperationID
	delete(store.deleteErrors, "retry-me")

	second, err := worker.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce(second) error = %v", err)
	}
	if second.Status != model.GCOperationSucceeded || second.ResumeOfOperationID != firstOperationID {
		t.Fatalf("second record = %+v, want succeeded resuming %q", second, firstOperationID)
	}
	records, err = operationStore.ListGCOperations(t.Context(), meta.ListGCOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListGCOperations(second) error = %v", err)
	}
	if records[0].ResumeOfOperationID != firstOperationID || records[0].Status != model.GCOperationSucceeded {
		t.Fatalf("persisted resumed record = %+v, want resume marker %q", records[0], firstOperationID)
	}
}

func TestWorkerRecordsMetrics(t *testing.T) {
	store := newGCWorkerStore([]storage.GCCandidate{
		gcCandidate("delete-me", storage.DeleteReasonPublishFailed),
		gcCandidate("retry-me", storage.DeleteReasonMultipartAborted),
	})
	store.deleteErrors["retry-me"] = storage.ErrUnavailable
	metrics := NewMetrics()
	worker := Worker{
		Storage: store,
		Orphans: store,
		Metrics: metrics,
	}

	if _, err := worker.RunOnce(t.Context(), 10); err != nil {
		t.Fatalf("RunOnce(first) error = %v", err)
	}
	delete(store.deleteErrors, "retry-me")
	if _, err := worker.RunOnce(t.Context(), 10); err != nil {
		t.Fatalf("RunOnce(second) error = %v", err)
	}
	_, err := Worker{Storage: store, Metrics: metrics}.RunOnce(t.Context(), 10)
	if err == nil {
		t.Fatal("RunOnce(missing orphan tracker) error = nil, want error")
	}

	snapshot := metrics.Snapshot()
	if snapshot.Runs != 3 || snapshot.Errors != 1 {
		t.Fatalf("metrics runs/errors = %+v, want 3/1", snapshot)
	}
	if snapshot.Scanned != 3 || snapshot.Deleted != 2 || snapshot.Retryable != 1 {
		t.Fatalf("metrics counters = %+v", snapshot)
	}
	if snapshot.Statuses[model.GCOperationRetryPending] != 1 || snapshot.Statuses[model.GCOperationSucceeded] != 1 || snapshot.Statuses[model.GCOperationFailed] != 1 {
		t.Fatalf("metrics statuses = %+v", snapshot.Statuses)
	}
	snapshot.Statuses[model.GCOperationSucceeded] = 99
	if got := metrics.Snapshot().Statuses[model.GCOperationSucceeded]; got != 1 {
		t.Fatalf("snapshot status map was not cloned: got %d", got)
	}
}

func TestWorkerMarksSharedObjectReleasePendingInsteadOfDeleting(t *testing.T) {
	store := newGCWorkerStore([]storage.GCCandidate{
		{
			Ref: storage.SegmentRef{
				SegmentID:      "segment-shared",
				SharedObjectID: "shared-1",
				SizeBytes:      1,
			},
			Reason:    storage.DeleteReasonManualGC,
			CreatedAt: time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC),
		},
	})
	releaseStore := memory.New()
	record, err := Worker{
		Storage:              store,
		Orphans:              store,
		SharedObjectReleases: releaseStore,
	}.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.Deleted != 0 || record.Skipped != 1 || record.Retryable != 0 {
		t.Fatalf("record = %+v, want shared release skipped", record)
	}
	if len(record.Attempts) != 1 || record.Attempts[0].Status != AttemptSkipped || record.Attempts[0].SharedObjectID != "shared-1" {
		t.Fatalf("attempts = %+v, want skipped shared release", record.Attempts)
	}
	remaining, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(remaining) != 1 || remaining[0].Ref.SegmentID != "segment-shared" {
		t.Fatalf("remaining candidates = %+v, want shared candidate retained", remaining)
	}
	releases, err := releaseStore.ListSharedObjectReleases(t.Context(), meta.ListSharedObjectReleasesRequest{
		SharedObjectID: "shared-1",
		Status:         model.SharedObjectReleasePending,
	})
	if err != nil {
		t.Fatalf("ListSharedObjectReleases() error = %v", err)
	}
	if len(releases) != 1 || releases[0].SegmentID != "segment-shared" || releases[0].Reason != storage.DeleteReasonManualGC {
		t.Fatalf("shared releases = %+v, want pending release for segment-shared", releases)
	}
}

func TestWorkerFailsClosedForSharedObjectWithoutReleaseStore(t *testing.T) {
	store := newGCWorkerStore([]storage.GCCandidate{
		{
			Ref: storage.SegmentRef{
				SegmentID:      "segment-shared",
				SharedObjectID: "shared-1",
				SizeBytes:      1,
			},
			Reason:    storage.DeleteReasonManualGC,
			CreatedAt: time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC),
		},
	})
	record, err := Worker{
		Storage: store,
		Orphans: store,
	}.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.Deleted != 0 || record.Retryable != 1 || record.Status != model.GCOperationRetryPending {
		t.Fatalf("record = %+v, want retryable fail-closed shared release", record)
	}
	if len(record.Attempts) != 1 || record.Attempts[0].Status != AttemptRetryable || !record.Attempts[0].Retryable {
		t.Fatalf("attempts = %+v, want retryable", record.Attempts)
	}
}

func gcCandidate(segmentID string, reason storage.DeleteReason) storage.GCCandidate {
	return storage.GCCandidate{
		Ref: storage.SegmentRef{
			SegmentID: segmentID,
			SizeBytes: 1,
		},
		Reason:    reason,
		CreatedAt: time.Date(2026, 7, 5, 8, 0, 0, 0, time.UTC),
	}
}

type gcWorkerStore struct {
	candidates   map[string]storage.GCCandidate
	order        []string
	deleteErrors map[string]error
}

func newGCWorkerStore(candidates []storage.GCCandidate) *gcWorkerStore {
	store := &gcWorkerStore{
		candidates:   make(map[string]storage.GCCandidate),
		deleteErrors: make(map[string]error),
	}
	for _, candidate := range candidates {
		store.candidates[candidate.Ref.SegmentID] = candidate
		store.order = append(store.order, candidate.Ref.SegmentID)
	}
	return store
}

func (s *gcWorkerStore) PutSegment(context.Context, storage.PutSegmentRequest) (storage.SegmentRef, error) {
	return storage.SegmentRef{}, nil
}

func (s *gcWorkerStore) GetSegment(context.Context, storage.SegmentRef, uint64, uint64) (io.ReadCloser, error) {
	return nil, storage.ErrNotFound
}

func (s *gcWorkerStore) DeleteSegment(_ context.Context, ref storage.SegmentRef, _ storage.DeleteReason) error {
	if err := s.deleteErrors[ref.SegmentID]; err != nil {
		return err
	}
	if _, ok := s.candidates[ref.SegmentID]; !ok {
		return storage.ErrNotFound
	}
	delete(s.candidates, ref.SegmentID)
	return nil
}

func (s *gcWorkerStore) MarkOrphan(context.Context, storage.SegmentRef, storage.DeleteReason) error {
	return errors.New("not implemented")
}

func (s *gcWorkerStore) ListGCCandidates(_ context.Context, limit int) ([]storage.GCCandidate, error) {
	out := make([]storage.GCCandidate, 0, len(s.candidates))
	for _, segmentID := range s.order {
		candidate, ok := s.candidates[segmentID]
		if !ok {
			continue
		}
		out = append(out, candidate)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

package gc

import (
	"context"
	"errors"
	"testing"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func TestMetadataOrphanTrackerUsesRepositoryCandidateQueue(t *testing.T) {
	repo := memory.New()
	tracker := MetadataOrphanTracker{Store: repo}
	ref := storage.SegmentRef{
		SegmentID: "segment-meta",
		SizeBytes: 64,
		StorageClass: storage.StorageClassSnapshot{
			Parameters: map[string]string{"redundancy": "replicated"},
		},
	}

	if err := tracker.MarkOrphan(t.Context(), ref, storage.DeleteReasonPublishFailed); err != nil {
		t.Fatalf("MarkOrphan() error = %v", err)
	}
	ref.StorageClass.Parameters["redundancy"] = "mutated"
	candidates, err := tracker.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].Ref.SegmentID != "segment-meta" || candidates[0].Reason != storage.DeleteReasonPublishFailed {
		t.Fatalf("candidates = %+v", candidates)
	}
	if candidates[0].Ref.StorageClass.Parameters["redundancy"] != "replicated" {
		t.Fatalf("candidate ref was mutated: %+v", candidates[0].Ref.StorageClass.Parameters)
	}
	if err := tracker.DeleteGCCandidate(t.Context(), storage.SegmentRef{SegmentID: "segment-meta"}); err != nil {
		t.Fatalf("DeleteGCCandidate() error = %v", err)
	}
	records, err := repo.ListGCCandidates(t.Context(), meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("repo ListGCCandidates() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("records after ack = %+v, want none", records)
	}
}

func TestWorkerAcksMetadataCandidateAfterDelete(t *testing.T) {
	repo := memory.New()
	tracker := MetadataOrphanTracker{Store: repo}
	store := newGCWorkerStore(nil)
	ref := storage.SegmentRef{SegmentID: "segment-delete", SizeBytes: 1}
	store.candidates[ref.SegmentID] = storage.GCCandidate{
		Ref:    ref,
		Reason: storage.DeleteReasonManualGC,
	}
	store.order = append(store.order, ref.SegmentID)
	if err := tracker.MarkOrphan(t.Context(), ref, storage.DeleteReasonManualGC); err != nil {
		t.Fatalf("MarkOrphan() error = %v", err)
	}

	record, err := Worker{
		Storage: store,
		Orphans: tracker,
	}.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.Deleted != 1 || record.Retryable != 0 || record.Status != model.GCOperationSucceeded {
		t.Fatalf("record = %+v, want deleted success", record)
	}
	records, err := repo.ListGCCandidates(t.Context(), meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("metadata candidates after delete = %+v, want none", records)
	}
}

func TestWorkerMarksAckFailureRetryable(t *testing.T) {
	tracker := failingAckTracker{
		candidates: []storage.GCCandidate{gcCandidate("segment-delete", storage.DeleteReasonManualGC)},
		err:        errors.New("metadata unavailable"),
	}
	store := newGCWorkerStore(tracker.candidates)

	record, err := Worker{
		Storage: store,
		Orphans: tracker,
	}.RunOnce(t.Context(), 10)
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if record.Deleted != 0 || record.Retryable != 1 || record.Status != model.GCOperationRetryPending {
		t.Fatalf("record = %+v, want retryable ack failure", record)
	}
}

type failingAckTracker struct {
	candidates []storage.GCCandidate
	err        error
}

func (t failingAckTracker) MarkOrphan(context.Context, storage.SegmentRef, storage.DeleteReason) error {
	return nil
}

func (t failingAckTracker) ListGCCandidates(context.Context, int) ([]storage.GCCandidate, error) {
	return append([]storage.GCCandidate(nil), t.candidates...), nil
}

func (t failingAckTracker) DeleteGCCandidate(context.Context, storage.SegmentRef) error {
	return t.err
}

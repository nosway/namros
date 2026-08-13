package gc

import (
	"context"
	"errors"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type AdmissionFunc func(ctx context.Context, ref storage.SegmentRef) error

type OperationStore interface {
	PutGCOperation(context.Context, meta.PutGCOperationRequest) (model.GCOperationRecord, error)
}

type OperationHistoryStore interface {
	ListGCOperations(context.Context, meta.ListGCOperationsRequest) ([]model.GCOperationRecord, error)
}

type SharedObjectReleaseStore interface {
	PutSharedObjectRelease(context.Context, meta.PutSharedObjectReleaseRequest) (model.SharedObjectRelease, error)
}

type Worker struct {
	Storage              storage.SegmentStore
	Orphans              storage.OrphanTracker
	Admission            AdmissionFunc
	OperationStore       OperationStore
	SharedObjectReleases SharedObjectReleaseStore
	Metrics              *Metrics
	Now                  func() time.Time
}

type AttemptStatus string

const (
	AttemptDeleted   AttemptStatus = "deleted"
	AttemptSkipped   AttemptStatus = "skipped"
	AttemptRetryable AttemptStatus = "retryable"
)

type AttemptRecord struct {
	SegmentID      string
	SharedObjectID string
	Reason         storage.DeleteReason
	Status         AttemptStatus
	Retryable      bool
	Error          string
}

type OperationRecord struct {
	ResumeOfOperationID string
	Status              model.GCOperationStatus
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Deleted             int
	Skipped             int
	Retryable           int
	Attempts            []AttemptRecord
}

func (w Worker) RunOnce(ctx context.Context, limit int) (record OperationRecord, err error) {
	now := time.Now
	if w.Now != nil {
		now = w.Now
	}
	record = OperationRecord{StartedAt: now().UTC()}
	defer func() {
		record.FinishedAt = now().UTC()
		if record.Status == "" {
			record.Status = statusForRecord(record, err)
		}
		w.Metrics.ObserveRun(record, err)
	}()
	if w.Storage == nil {
		return record, errors.New("segment store is required")
	}
	if w.Orphans == nil {
		return record, errors.New("orphan tracker is required")
	}
	if w.OperationStore != nil {
		if history, ok := w.OperationStore.(OperationHistoryStore); ok {
			resumeOf, err := retryableOperationToResume(ctx, history)
			if err != nil {
				return record, err
			}
			record.ResumeOfOperationID = resumeOf
		}
	}
	candidates, err := w.Orphans.ListGCCandidates(ctx, limit)
	if err != nil {
		return record, err
	}
	for _, candidate := range candidates {
		record.Scanned++
		reason := candidate.Reason
		if reason == "" {
			reason = storage.DeleteReasonManualGC
		}
		attempt := AttemptRecord{
			SegmentID:      candidate.Ref.SegmentID,
			SharedObjectID: candidate.Ref.SharedObjectID,
			Reason:         reason,
		}
		if w.Admission != nil {
			if err := w.Admission(ctx, candidate.Ref); err != nil {
				attempt.Status = AttemptSkipped
				attempt.Error = err.Error()
				record.Skipped++
				record.Attempts = append(record.Attempts, attempt)
				continue
			}
		}
		if candidate.Ref.SharedObjectID != "" {
			if w.SharedObjectReleases == nil {
				attempt.Status = AttemptRetryable
				attempt.Retryable = true
				attempt.Error = "shared object release store is unavailable"
				record.Retryable++
				record.Attempts = append(record.Attempts, attempt)
				continue
			}
			if _, err := w.SharedObjectReleases.PutSharedObjectRelease(ctx, meta.PutSharedObjectReleaseRequest{
				SharedObjectID: candidate.Ref.SharedObjectID,
				SegmentRef:     candidate.Ref,
				Reason:         reason,
				Status:         model.SharedObjectReleasePending,
			}); err != nil {
				attempt.Status = AttemptRetryable
				attempt.Retryable = true
				attempt.Error = err.Error()
				record.Retryable++
				record.Attempts = append(record.Attempts, attempt)
				continue
			}
			attempt.Status = AttemptSkipped
			attempt.Error = "shared object release pending"
			record.Skipped++
			record.Attempts = append(record.Attempts, attempt)
			continue
		}
		if err := w.Storage.DeleteSegment(ctx, candidate.Ref, reason); err != nil && !errors.Is(err, storage.ErrNotFound) {
			attempt.Status = AttemptRetryable
			attempt.Retryable = true
			attempt.Error = err.Error()
			record.Retryable++
			record.Attempts = append(record.Attempts, attempt)
			continue
		}
		if acker, ok := w.Orphans.(CandidateAcker); ok {
			if err := acker.DeleteGCCandidate(ctx, candidate.Ref); err != nil {
				attempt.Status = AttemptRetryable
				attempt.Retryable = true
				attempt.Error = err.Error()
				record.Retryable++
				record.Attempts = append(record.Attempts, attempt)
				continue
			}
		}
		attempt.Status = AttemptDeleted
		record.Deleted++
		record.Attempts = append(record.Attempts, attempt)
	}
	if record.FinishedAt.IsZero() {
		record.FinishedAt = now().UTC()
	}
	record.Status = statusForRecord(record, nil)
	if w.OperationStore != nil {
		if _, err := w.OperationStore.PutGCOperation(ctx, putGCOperationRequest(record)); err != nil {
			return record, err
		}
	}
	return record, nil
}

func statusForRecord(record OperationRecord, err error) model.GCOperationStatus {
	if err != nil {
		return model.GCOperationFailed
	}
	if record.Retryable > 0 {
		return model.GCOperationRetryPending
	}
	return model.GCOperationSucceeded
}

func retryableOperationToResume(ctx context.Context, history OperationHistoryStore) (string, error) {
	records, err := history.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: 1})
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", nil
	}
	if records[0].Status == model.GCOperationRetryPending || records[0].Retryable > 0 || hasRetryableAttempt(records[0].Attempts) {
		return records[0].OperationID, nil
	}
	return "", nil
}

func hasRetryableAttempt(attempts []model.GCOperationAttempt) bool {
	for _, attempt := range attempts {
		if attempt.Retryable || attempt.Status == model.GCOperationAttemptRetryable {
			return true
		}
	}
	return false
}

func putGCOperationRequest(record OperationRecord) meta.PutGCOperationRequest {
	attempts := make([]model.GCOperationAttempt, 0, len(record.Attempts))
	for _, attempt := range record.Attempts {
		attempts = append(attempts, model.GCOperationAttempt{
			SegmentID:      attempt.SegmentID,
			SharedObjectID: attempt.SharedObjectID,
			Reason:         attempt.Reason,
			Status:         model.GCOperationAttemptStatus(attempt.Status),
			Retryable:      attempt.Retryable,
			Error:          attempt.Error,
		})
	}
	return meta.PutGCOperationRequest{
		ResumeOfOperationID: record.ResumeOfOperationID,
		Status:              record.Status,
		StartedAt:           record.StartedAt,
		FinishedAt:          record.FinishedAt,
		Scanned:             record.Scanned,
		Deleted:             record.Deleted,
		Skipped:             record.Skipped,
		Retryable:           record.Retryable,
		Attempts:            attempts,
	}
}

package drain

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type Worker struct {
	Metadata meta.Repository
	Storage  storage.SegmentStore
	Now      func() time.Time
}

type MoveObjectVersionRequest struct {
	ResumeOfOperationID string
	PoolID              string
	SourceVolumeID      string
	TargetVolumeID      string
	OwnerID             string
	BucketID            string
	Key                 string
	VersionID           string
}

type MoveObjectVersionResult struct {
	Version              model.ObjectVersion
	PreviousSegmentRefs  []storage.SegmentRef
	PublishedSegmentRefs []storage.SegmentRef
	Operation            model.VolumeDrainOperationRecord
}

type ReclaimOperationRequest struct {
	Operation model.VolumeDrainOperationRecord
	OwnerID   string
	Reason    storage.DeleteReason
}

type ReclaimOperationResult struct {
	Operation model.VolumeDrainOperationRecord
	Queued    int
	Skipped   int
	Protected int
	Retryable int
}

func (w Worker) MoveObjectVersion(ctx context.Context, req MoveObjectVersionRequest) (MoveObjectVersionResult, error) {
	if err := ctx.Err(); err != nil {
		return MoveObjectVersionResult{}, err
	}
	if w.Metadata == nil {
		return MoveObjectVersionResult{}, errors.New("metadata repository is required")
	}
	if w.Storage == nil {
		return MoveObjectVersionResult{}, errors.New("segment store is required")
	}
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return MoveObjectVersionResult{}, fmt.Errorf("%w: bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	if req.SourceVolumeID == "" {
		return MoveObjectVersionResult{}, fmt.Errorf("%w: source volume id is required", meta.ErrInvalidArgument)
	}

	startedAt := w.now()
	version, err := w.Metadata.GetObjectVersion(ctx, req.BucketID, req.Key, req.VersionID)
	if err != nil {
		return MoveObjectVersionResult{}, err
	}
	refs := objectVersionSegmentRefs(version)
	if len(refs) == 0 {
		record, persistErr := w.putOperation(ctx, req, operationDraft{
			startedAt:  startedAt,
			finishedAt: w.now(),
			status:     model.VolumeDrainOperationSucceeded,
			cursor:     req.VersionID,
		})
		return MoveObjectVersionResult{Version: version, Operation: record}, persistErr
	}

	draft := operationDraft{
		startedAt: startedAt,
		cursor:    req.VersionID,
		scanned:   len(refs),
	}
	replacementRefs := make([]storage.SegmentRef, 0, len(refs))
	for _, ref := range refs {
		if meta.SegmentRefVolumeID(ref) != req.SourceVolumeID {
			replacementRefs = append(replacementRefs, storage.CloneSegmentRef(ref))
			draft.skipped++
			draft.attempts = append(draft.attempts, model.VolumeDrainAttempt{
				BucketID:        req.BucketID,
				Key:             req.Key,
				VersionID:       req.VersionID,
				SourceSegmentID: ref.SegmentID,
				SourceRef:       storage.CloneSegmentRef(ref),
				Status:          model.VolumeDrainAttemptSkipped,
				Error:           "segment ref is not on source volume",
			})
			continue
		}
		copiedRef, err := w.copySegment(ctx, version, ref)
		attempt := model.VolumeDrainAttempt{
			BucketID:        req.BucketID,
			Key:             req.Key,
			VersionID:       req.VersionID,
			SourceSegmentID: ref.SegmentID,
			SourceRef:       storage.CloneSegmentRef(ref),
		}
		if err != nil {
			attempt.Status = model.VolumeDrainAttemptRetryable
			attempt.Retryable = true
			attempt.Error = err.Error()
			draft.retryable++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		attempt.TargetSegmentID = copiedRef.SegmentID
		attempt.TargetRef = storage.CloneSegmentRef(copiedRef)
		if req.TargetVolumeID != "" && meta.SegmentRefVolumeID(copiedRef) != req.TargetVolumeID {
			attempt.Status = model.VolumeDrainAttemptRetryable
			attempt.Retryable = true
			attempt.Error = fmt.Sprintf("copied segment landed on volume %q, want %q", meta.SegmentRefVolumeID(copiedRef), req.TargetVolumeID)
			draft.retryable++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		replacementRefs = append(replacementRefs, copiedRef)
		attempt.Status = model.VolumeDrainAttemptCopied
		draft.copied++
		draft.attempts = append(draft.attempts, attempt)
	}
	if draft.retryable > 0 {
		draft.status = model.VolumeDrainOperationRetryPending
		draft.finishedAt = w.now()
		record, persistErr := w.putOperation(ctx, req, draft)
		if persistErr != nil {
			return MoveObjectVersionResult{Operation: record}, persistErr
		}
		return MoveObjectVersionResult{Operation: record}, errors.New("volume drain copy has retryable failures")
	}
	if draft.copied == 0 {
		draft.status = model.VolumeDrainOperationSucceeded
		draft.finishedAt = w.now()
		record, persistErr := w.putOperation(ctx, req, draft)
		return MoveObjectVersionResult{Version: version, PublishedSegmentRefs: replacementRefs, Operation: record}, persistErr
	}

	published, err := w.Metadata.PublishObjectVersionRefs(ctx, meta.PublishObjectVersionRefsRequest{
		BucketID:               req.BucketID,
		Key:                    req.Key,
		VersionID:              req.VersionID,
		ExpectedSourceVolumeID: req.SourceVolumeID,
		SegmentRefs:            replacementRefs,
	})
	if err != nil {
		draft.finishedAt = w.now()
		sourceRef := firstSegmentRef(refs)
		targetRef := firstSegmentRef(replacementRefs)
		if errors.Is(err, meta.ErrObjectLocked) {
			draft.status = model.VolumeDrainOperationRetryPending
			draft.protected++
			draft.attempts = append(draft.attempts, model.VolumeDrainAttempt{
				BucketID:        req.BucketID,
				Key:             req.Key,
				VersionID:       req.VersionID,
				SourceSegmentID: sourceRef.SegmentID,
				SourceRef:       sourceRef,
				TargetSegmentID: targetRef.SegmentID,
				TargetRef:       targetRef,
				Status:          model.VolumeDrainAttemptProtected,
				Protected:       true,
				Error:           err.Error(),
			})
		} else {
			draft.status = model.VolumeDrainOperationRetryPending
			draft.retryable++
			draft.attempts = append(draft.attempts, model.VolumeDrainAttempt{
				BucketID:        req.BucketID,
				Key:             req.Key,
				VersionID:       req.VersionID,
				SourceSegmentID: sourceRef.SegmentID,
				SourceRef:       sourceRef,
				TargetSegmentID: targetRef.SegmentID,
				TargetRef:       targetRef,
				Status:          model.VolumeDrainAttemptRetryable,
				Retryable:       true,
				Error:           err.Error(),
			})
		}
		record, persistErr := w.putOperation(ctx, req, draft)
		if persistErr != nil {
			return MoveObjectVersionResult{Operation: record}, persistErr
		}
		return MoveObjectVersionResult{Operation: record}, err
	}

	draft.status = model.VolumeDrainOperationSucceeded
	draft.finishedAt = w.now()
	record, err := w.putOperation(ctx, req, draft)
	return MoveObjectVersionResult{
		Version:              published.Version,
		PreviousSegmentRefs:  storage.CloneSegmentRefs(published.PreviousSegmentRefs),
		PublishedSegmentRefs: storage.CloneSegmentRefs(replacementRefs),
		Operation:            record,
	}, err
}

func (w Worker) ReclaimOperation(ctx context.Context, req ReclaimOperationRequest) (ReclaimOperationResult, error) {
	if err := ctx.Err(); err != nil {
		return ReclaimOperationResult{}, err
	}
	if w.Metadata == nil {
		return ReclaimOperationResult{}, errors.New("metadata repository is required")
	}
	if w.Storage == nil {
		return ReclaimOperationResult{}, errors.New("segment store is required")
	}
	operation := meta.CloneVolumeDrainOperationRecord(req.Operation)
	if operation.SourceVolumeID == "" {
		return ReclaimOperationResult{}, fmt.Errorf("%w: source volume id is required", meta.ErrInvalidArgument)
	}
	reason := req.Reason
	if reason == "" {
		reason = storage.DeleteReasonVolumeDrained
	}
	ownerID := req.OwnerID
	if ownerID == "" {
		ownerID = operation.OwnerID
	}
	startedAt := w.now()
	draft := operationDraft{
		startedAt: startedAt,
		cursor:    operation.OperationID,
	}
	result := ReclaimOperationResult{}
	for _, sourceAttempt := range operation.Attempts {
		if sourceAttempt.Status != model.VolumeDrainAttemptCopied {
			continue
		}
		sourceRef := storage.CloneSegmentRef(sourceAttempt.SourceRef)
		targetRef := storage.CloneSegmentRef(sourceAttempt.TargetRef)
		if sourceRef.SegmentID == "" || targetRef.SegmentID == "" {
			draft.skipped++
			result.Skipped++
			draft.attempts = append(draft.attempts, model.VolumeDrainAttempt{
				BucketID:        sourceAttempt.BucketID,
				Key:             sourceAttempt.Key,
				VersionID:       sourceAttempt.VersionID,
				SourceSegmentID: sourceAttempt.SourceSegmentID,
				TargetSegmentID: sourceAttempt.TargetSegmentID,
				SourceRef:       sourceRef,
				TargetRef:       targetRef,
				Status:          model.VolumeDrainAttemptSkipped,
				Error:           "copied attempt is missing source or target ref",
			})
			continue
		}
		draft.scanned++
		attempt := model.VolumeDrainAttempt{
			BucketID:        sourceAttempt.BucketID,
			Key:             sourceAttempt.Key,
			VersionID:       sourceAttempt.VersionID,
			SourceSegmentID: sourceRef.SegmentID,
			SourceRef:       sourceRef,
			TargetSegmentID: targetRef.SegmentID,
			TargetRef:       targetRef,
		}
		if meta.SegmentRefVolumeID(sourceRef) != operation.SourceVolumeID {
			attempt.Status = model.VolumeDrainAttemptSkipped
			attempt.Error = "source ref is not on drain source volume"
			draft.skipped++
			result.Skipped++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		if err := w.validateTargetSegment(ctx, targetRef); err != nil {
			attempt.Status = model.VolumeDrainAttemptRetryable
			attempt.Retryable = true
			attempt.Error = err.Error()
			draft.retryable++
			result.Retryable++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		protected, err := w.sourceRefProtected(ctx, sourceRef)
		if err != nil {
			attempt.Status = model.VolumeDrainAttemptRetryable
			attempt.Retryable = true
			attempt.Error = err.Error()
			draft.retryable++
			result.Retryable++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		if protected {
			attempt.Status = model.VolumeDrainAttemptProtected
			attempt.Protected = true
			attempt.Error = "source segment has an active protected ref"
			draft.protected++
			result.Protected++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		if _, err := w.Metadata.PutGCCandidate(ctx, meta.PutGCCandidateRequest{
			SegmentRef: sourceRef,
			Reason:     reason,
		}); err != nil {
			attempt.Status = model.VolumeDrainAttemptRetryable
			attempt.Retryable = true
			attempt.Error = err.Error()
			draft.retryable++
			result.Retryable++
			draft.attempts = append(draft.attempts, attempt)
			continue
		}
		attempt.Status = model.VolumeDrainAttemptQueuedGC
		draft.attempts = append(draft.attempts, attempt)
		result.Queued++
	}
	if draft.retryable > 0 || draft.protected > 0 {
		draft.status = model.VolumeDrainOperationRetryPending
	} else {
		draft.status = model.VolumeDrainOperationSucceeded
	}
	draft.finishedAt = w.now()
	record, err := w.Metadata.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{
		ResumeOfOperationID: operation.OperationID,
		PoolID:              operation.PoolID,
		SourceVolumeID:      operation.SourceVolumeID,
		TargetVolumeID:      operation.TargetVolumeID,
		OwnerID:             ownerID,
		Status:              draft.status,
		Cursor:              draft.cursor,
		StartedAt:           draft.startedAt,
		FinishedAt:          draft.finishedAt,
		Scanned:             draft.scanned,
		Skipped:             draft.skipped,
		Protected:           draft.protected,
		Retryable:           draft.retryable,
		Attempts:            draft.attempts,
	})
	result.Operation = record
	return result, err
}

type operationDraft struct {
	status     model.VolumeDrainOperationStatus
	cursor     string
	startedAt  time.Time
	finishedAt time.Time
	scanned    int
	copied     int
	skipped    int
	protected  int
	retryable  int
	attempts   []model.VolumeDrainAttempt
}

func (w Worker) copySegment(ctx context.Context, version model.ObjectVersion, ref storage.SegmentRef) (storage.SegmentRef, error) {
	reader, err := w.Storage.GetSegment(ctx, ref, 0, ref.SizeBytes)
	if err != nil {
		return storage.SegmentRef{}, err
	}
	storageClass := ref.StorageClass
	if storageClass.StorageClassID == "" {
		storageClass = version.StorageClass
	}
	copied, err := w.Storage.PutSegment(ctx, storage.PutSegmentRequest{
		Reader:       reader,
		SizeBytes:    ref.SizeBytes,
		StorageClass: storageClass,
	})
	closeErr := reader.Close()
	if err != nil {
		return storage.SegmentRef{}, err
	}
	if closeErr != nil {
		return storage.SegmentRef{}, closeErr
	}
	return storage.CloneSegmentRef(copied), nil
}

func (w Worker) validateTargetSegment(ctx context.Context, ref storage.SegmentRef) error {
	if ref.SegmentID == "" {
		return fmt.Errorf("%w: target segment id is required", storage.ErrInvalidArgument)
	}
	if validator, ok := w.Storage.(storage.SegmentValidator); ok {
		return validator.ValidateSegment(ctx, ref)
	}
	reader, err := w.Storage.GetSegment(ctx, ref, 0, 0)
	if err != nil {
		return err
	}
	return reader.Close()
}

func (w Worker) sourceRefProtected(ctx context.Context, ref storage.SegmentRef) (bool, error) {
	refs, err := w.Metadata.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		SegmentID:  ref.SegmentID,
		ActiveOnly: true,
		Limit:      1,
	})
	if err != nil {
		return false, err
	}
	return len(refs) > 0, nil
}

func (w Worker) putOperation(ctx context.Context, req MoveObjectVersionRequest, draft operationDraft) (model.VolumeDrainOperationRecord, error) {
	if draft.finishedAt.IsZero() {
		draft.finishedAt = w.now()
	}
	return w.Metadata.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{
		ResumeOfOperationID: req.ResumeOfOperationID,
		PoolID:              req.PoolID,
		SourceVolumeID:      req.SourceVolumeID,
		TargetVolumeID:      req.TargetVolumeID,
		OwnerID:             req.OwnerID,
		Status:              draft.status,
		Cursor:              draft.cursor,
		StartedAt:           draft.startedAt,
		FinishedAt:          draft.finishedAt,
		Scanned:             draft.scanned,
		Copied:              draft.copied,
		Skipped:             draft.skipped,
		Protected:           draft.protected,
		Retryable:           draft.retryable,
		Attempts:            draft.attempts,
	})
}

func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func objectVersionSegmentRefs(version model.ObjectVersion) []storage.SegmentRef {
	if len(version.SegmentRefs) > 0 {
		return storage.CloneSegmentRefs(version.SegmentRefs)
	}
	if version.SegmentRef.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{storage.CloneSegmentRef(version.SegmentRef)}
}

func firstSegmentRef(refs []storage.SegmentRef) storage.SegmentRef {
	if len(refs) == 0 {
		return storage.SegmentRef{}
	}
	return storage.CloneSegmentRef(refs[0])
}

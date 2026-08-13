package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type AbortWorker struct {
	Metadata meta.Repository
	Storage  storage.SegmentStore
	Orphans  storage.OrphanTracker
	Now      func() time.Time
}

type AbortWorkerResult struct {
	Planned       int
	Aborted       int
	Skipped       int
	DeleteFailed  int
	OrphansMarked int
	Failed        int
}

type ExpirationWorker struct {
	Metadata meta.Repository
	Storage  storage.SegmentStore
	Orphans  storage.OrphanTracker
	Now      func() time.Time
}

type ExpirationWorkerResult struct {
	Planned       int
	Deleted       int
	Skipped       int
	DeleteFailed  int
	OrphansMarked int
	Failed        int
}

type Worker struct {
	Metadata meta.Repository
	Storage  storage.SegmentStore
	Orphans  storage.OrphanTracker
	Now      func() time.Time
}

type WorkerRequest struct {
	Plan PlanRequest
}

type WorkerResult struct {
	Expiration    ExpirationWorkerResult
	Abort         AbortWorkerResult
	Planned       int
	Processed     int
	Skipped       int
	DeleteFailed  int
	OrphansMarked int
	Failed        int
}

func (w Worker) RunOnce(ctx context.Context, req WorkerRequest) (WorkerResult, error) {
	if w.Metadata == nil {
		return WorkerResult{}, errors.New("metadata repository is required")
	}
	planReq := req.Plan
	if planReq.Now.IsZero() {
		planReq.Now = w.now()
	}
	expiration, err := ExpirationWorker{
		Metadata: w.Metadata,
		Storage:  w.Storage,
		Orphans:  w.Orphans,
		Now:      w.Now,
	}.RunOnce(ctx, planReq)
	result := WorkerResult{Expiration: expiration}
	result.addExpiration(expiration)
	if err != nil {
		return result, err
	}
	abort, err := AbortWorker{
		Metadata: w.Metadata,
		Storage:  w.Storage,
		Orphans:  w.Orphans,
		Now:      w.Now,
	}.RunOnce(ctx, planReq)
	result.Abort = abort
	result.addAbort(abort)
	return result, err
}

func (r *WorkerResult) addExpiration(result ExpirationWorkerResult) {
	r.Planned += result.Planned
	r.Processed += result.Deleted
	r.Skipped += result.Skipped
	r.DeleteFailed += result.DeleteFailed
	r.OrphansMarked += result.OrphansMarked
	r.Failed += result.Failed
}

func (r *WorkerResult) addAbort(result AbortWorkerResult) {
	r.Planned += result.Planned
	r.Processed += result.Aborted
	r.Skipped += result.Skipped
	r.DeleteFailed += result.DeleteFailed
	r.OrphansMarked += result.OrphansMarked
	r.Failed += result.Failed
}

func (w Worker) now() time.Time {
	if w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w AbortWorker) RunOnce(ctx context.Context, req PlanRequest) (AbortWorkerResult, error) {
	if w.Metadata == nil {
		return AbortWorkerResult{}, errors.New("metadata repository is required")
	}
	if req.Now.IsZero() {
		if w.Now != nil {
			req.Now = w.Now().UTC()
		} else {
			req.Now = time.Now().UTC()
		}
	}
	plan, err := BuildPlan(ctx, w.Metadata, req)
	if err != nil {
		return AbortWorkerResult{}, err
	}
	result := AbortWorkerResult{}
	for _, action := range plan.Actions {
		if action.Kind != ActionAbortIncompleteMultipart {
			continue
		}
		result.Planned++
		if action.Status != ActionEligible {
			result.Skipped++
			continue
		}
		if w.Storage == nil {
			result.Failed++
			continue
		}
		parts, err := w.Metadata.AbortMultipartUpload(ctx, meta.MultipartUploadRequest{
			BucketID: action.BucketID,
			Key:      action.Key,
			UploadID: action.UploadID,
		})
		if err != nil {
			if errors.Is(err, meta.ErrNotFound) {
				result.Skipped++
				continue
			}
			result.Failed++
			continue
		}
		result.Aborted++
		for _, part := range parts {
			if err := w.Storage.DeleteSegment(ctx, part.SegmentRef, storage.DeleteReasonMultipartAborted); err != nil && !errors.Is(err, storage.ErrNotFound) {
				result.DeleteFailed++
				if w.Orphans != nil && part.SegmentRef.SegmentID != "" {
					if markErr := w.Orphans.MarkOrphan(ctx, part.SegmentRef, storage.DeleteReasonMultipartAborted); markErr == nil {
						result.OrphansMarked++
					} else {
						result.Failed++
					}
				} else {
					result.Failed++
				}
			}
		}
	}
	return result, nil
}

func (w ExpirationWorker) RunOnce(ctx context.Context, req PlanRequest) (ExpirationWorkerResult, error) {
	if w.Metadata == nil {
		return ExpirationWorkerResult{}, errors.New("metadata repository is required")
	}
	if req.Now.IsZero() {
		if w.Now != nil {
			req.Now = w.Now().UTC()
		} else {
			req.Now = time.Now().UTC()
		}
	}
	plan, err := BuildPlan(ctx, w.Metadata, req)
	if err != nil {
		return ExpirationWorkerResult{}, err
	}
	result := ExpirationWorkerResult{}
	for _, action := range plan.Actions {
		switch action.Kind {
		case ActionExpireCurrentObject, ActionExpireNoncurrentVersion, ActionExpireDeleteMarker:
		default:
			continue
		}
		result.Planned++
		if action.Status != ActionEligible {
			result.Skipped++
			continue
		}
		deleteReq := meta.DeleteObjectRequest{
			BucketID: action.BucketID,
			Key:      action.Key,
		}
		if action.Kind != ActionExpireCurrentObject {
			deleteReq.VersionID = action.VersionID
		}
		deleted, err := w.Metadata.DeleteObject(ctx, deleteReq)
		if err != nil {
			if errors.Is(err, meta.ErrNotFound) {
				result.Skipped++
				continue
			}
			result.Failed++
			continue
		}
		result.Deleted++
		w.deleteDeletedVersionSegments(ctx, deleted.DeletedVersion, &result)
	}
	return result, nil
}

func (w ExpirationWorker) deleteDeletedVersionSegments(ctx context.Context, version model.ObjectVersion, result *ExpirationWorkerResult) {
	if version.DeleteMarker {
		return
	}
	refs := versionSegmentRefs(version)
	for _, ref := range refs {
		if ref.SegmentID == "" {
			continue
		}
		if w.Storage == nil {
			result.Failed++
			continue
		}
		if err := w.Storage.DeleteSegment(ctx, ref, storage.DeleteReasonManualGC); err != nil && !errors.Is(err, storage.ErrNotFound) {
			result.DeleteFailed++
			if w.Orphans != nil {
				if markErr := w.Orphans.MarkOrphan(ctx, ref, storage.DeleteReasonManualGC); markErr == nil {
					result.OrphansMarked++
				} else {
					result.Failed++
				}
			} else {
				result.Failed++
			}
		}
	}
}

func versionSegmentRefs(version model.ObjectVersion) []storage.SegmentRef {
	if len(version.SegmentRefs) > 0 {
		return storage.CloneSegmentRefs(version.SegmentRefs)
	}
	if version.SegmentRef.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{storage.CloneSegmentRef(version.SegmentRef)}
}

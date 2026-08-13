package gateway

import (
	"context"
	"errors"
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type segmentDeleteAdmission struct {
	Admitted      bool
	ProtectedRefs []model.ProtectedRef
	Err           error
}

type orphanGCResult struct {
	Scanned          int
	Deleted          int
	SkippedProtected int
	Failed           int
	LastError        error
}

func (h s3Handler) runOrphanGCOnce(ctx context.Context, limit int) orphanGCResult {
	var result orphanGCResult
	if h.deps.Orphans == nil {
		result.Failed = 1
		result.LastError = errors.New("orphan tracker is unavailable")
		return result
	}
	if h.deps.Storage == nil {
		result.Failed = 1
		result.LastError = errors.New("segment store is unavailable")
		return result
	}
	candidates, err := h.deps.Orphans.ListGCCandidates(ctx, limit)
	if err != nil {
		result.Failed = 1
		result.LastError = err
		return result
	}
	for _, candidate := range candidates {
		result.Scanned++
		admission := h.admitSegmentDelete(ctx, candidate.Ref)
		if admission.Err != nil {
			result.Failed++
			result.LastError = admission.Err
			continue
		}
		if !admission.Admitted {
			result.SkippedProtected++
			continue
		}
		reason := candidate.Reason
		if reason == "" {
			reason = storage.DeleteReasonManualGC
		}
		if err := h.deps.Storage.DeleteSegment(ctx, candidate.Ref, reason); err != nil && !errors.Is(err, storage.ErrNotFound) {
			result.Failed++
			result.LastError = err
			continue
		}
		result.Deleted++
	}
	return result
}

func (h s3Handler) segmentDeleteBlockedByProtectedRef(c *gin.Context, ref storage.SegmentRef) bool {
	return !h.admitSegmentDelete(c.Request.Context(), ref).Admitted
}

func protectedRefDeleteAdmission(metadata meta.Repository) storage.DeleteAdmissionFunc {
	return func(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
		admission := admitProtectedRefSegmentDelete(ctx, metadata, ref)
		if admission.Err != nil {
			return admission.Err
		}
		if !admission.Admitted {
			return fmt.Errorf("%w: active protected ref for segment %q", storage.ErrProtected, ref.SegmentID)
		}
		_ = reason
		return nil
	}
}

func (h s3Handler) admitSegmentDelete(ctx context.Context, ref storage.SegmentRef) segmentDeleteAdmission {
	return admitProtectedRefSegmentDelete(ctx, h.deps.Metadata, ref)
}

func admitProtectedRefSegmentDelete(ctx context.Context, metadata meta.Repository, ref storage.SegmentRef) segmentDeleteAdmission {
	if ref.SegmentID == "" {
		return segmentDeleteAdmission{Admitted: true}
	}
	if metadata == nil {
		return segmentDeleteAdmission{Err: errors.New("metadata repository is unavailable")}
	}
	refs, err := metadata.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		SegmentID:  ref.SegmentID,
		ActiveOnly: true,
		Limit:      1,
	})
	if err != nil {
		return segmentDeleteAdmission{Err: fmt.Errorf("check protected refs for segment %q: %w", ref.SegmentID, err)}
	}
	return segmentDeleteAdmission{
		Admitted:      len(refs) == 0,
		ProtectedRefs: refs,
	}
}

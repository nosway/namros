package meta

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func BuildGCCandidate(existing model.GCCandidateRecord, req PutGCCandidateRequest, now time.Time) (model.GCCandidateRecord, error) {
	segmentID := strings.TrimSpace(req.SegmentRef.SegmentID)
	if segmentID == "" {
		return model.GCCandidateRecord{}, fmt.Errorf("%w: gc candidate segment id is required", ErrInvalidArgument)
	}
	reason := req.Reason
	if reason == "" {
		reason = storage.DeleteReasonManualGC
	}
	now = now.UTC()
	createdAt := existing.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return model.GCCandidateRecord{
		SegmentID:  segmentID,
		SegmentRef: storage.CloneSegmentRef(req.SegmentRef),
		Reason:     reason,
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	}, nil
}

func CloneGCCandidateRecord(in model.GCCandidateRecord) model.GCCandidateRecord {
	out := in
	out.SegmentRef = storage.CloneSegmentRef(in.SegmentRef)
	return out
}

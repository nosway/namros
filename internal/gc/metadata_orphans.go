package gc

import (
	"context"
	"errors"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type CandidateStore interface {
	PutGCCandidate(context.Context, meta.PutGCCandidateRequest) (model.GCCandidateRecord, error)
	ListGCCandidates(context.Context, meta.ListGCCandidatesRequest) ([]model.GCCandidateRecord, error)
	DeleteGCCandidate(context.Context, string) error
}

type CandidateAcker interface {
	DeleteGCCandidate(context.Context, storage.SegmentRef) error
}

type MetadataOrphanTracker struct {
	Store CandidateStore
}

func (t MetadataOrphanTracker) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if t.Store == nil {
		return errors.New("gc candidate store is unavailable")
	}
	_, err := t.Store.PutGCCandidate(ctx, meta.PutGCCandidateRequest{
		SegmentRef: ref,
		Reason:     reason,
	})
	return err
}

func (t MetadataOrphanTracker) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if t.Store == nil {
		return nil, errors.New("gc candidate store is unavailable")
	}
	records, err := t.Store.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]storage.GCCandidate, 0, len(records))
	for _, record := range records {
		out = append(out, storage.GCCandidate{
			Ref:       storage.CloneSegmentRef(record.SegmentRef),
			Reason:    record.Reason,
			CreatedAt: record.CreatedAt,
		})
	}
	return out, nil
}

func (t MetadataOrphanTracker) DeleteGCCandidate(ctx context.Context, ref storage.SegmentRef) error {
	if t.Store == nil {
		return errors.New("gc candidate store is unavailable")
	}
	err := t.Store.DeleteGCCandidate(ctx, ref.SegmentID)
	if errors.Is(err, meta.ErrNotFound) {
		return nil
	}
	return err
}

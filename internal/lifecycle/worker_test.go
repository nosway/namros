package lifecycle

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func TestAbortWorkerAbortsOldMultipartAndDeletesParts(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -10)
	repo := memory.NewWithClock(func() time.Time { return clock })
	ctx := t.Context()
	bucket := createLifecycleWorkerBucket(t, repo)
	upload := createLifecycleWorkerUpload(t, repo, bucket.BucketID, "uploads/old.bin", "segment-old")
	clock = now
	putAbortLifecycleRule(t, repo, bucket.BucketID, "uploads/", 3)

	store := &workerStore{}
	result, err := AbortWorker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now },
	}.RunOnce(ctx, PlanRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Planned != 1 || result.Aborted != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "segment-old" {
		t.Fatalf("deleted segments = %+v", store.deleted)
	}
	got, err := repo.GetMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      upload.Key,
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("GetMultipartUpload() error = %v", err)
	}
	if got.State != model.MultipartUploadAborted {
		t.Fatalf("upload state = %q, want aborted", got.State)
	}

	result, err = AbortWorker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now },
	}.RunOnce(ctx, PlanRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("RunOnce(retry) error = %v", err)
	}
	if result.Planned != 0 || result.Aborted != 0 || result.Failed != 0 {
		t.Fatalf("retry result = %+v, want no work", result)
	}
}

func TestAbortWorkerMarksOrphanWhenPartDeleteFails(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -10)
	repo := memory.NewWithClock(func() time.Time { return clock })
	bucket := createLifecycleWorkerBucket(t, repo)
	createLifecycleWorkerUpload(t, repo, bucket.BucketID, "uploads/fail.bin", "segment-fail")
	clock = now
	putAbortLifecycleRule(t, repo, bucket.BucketID, "uploads/", 3)

	store := &workerStore{deleteErr: storage.ErrUnavailable}
	orphans := &workerOrphans{}
	result, err := AbortWorker{
		Metadata: repo,
		Storage:  store,
		Orphans:  orphans,
		Now:      func() time.Time { return now },
	}.RunOnce(t.Context(), PlanRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Planned != 1 || result.Aborted != 1 || result.DeleteFailed != 1 || result.OrphansMarked != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(orphans.marked) != 1 || orphans.marked[0] != "segment-fail" {
		t.Fatalf("marked orphans = %+v", orphans.marked)
	}
}

func TestExpirationWorkerExpiresUnversionedCurrentObject(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	ctx := t.Context()
	bucket := createLifecycleWorkerBucket(t, repo)
	version := putLifecycleTestObject(t, repo, bucket.BucketID, "logs/current.txt", storage.SegmentRef{SegmentID: "segment-current", SizeBytes: 5})
	clock = now
	putExpirationLifecycleRule(t, repo, bucket.BucketID, "logs/", 30, 0)

	store := &workerStore{}
	result, err := ExpirationWorker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now },
	}.RunOnce(ctx, PlanRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Planned != 1 || result.Deleted != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "segment-current" {
		t.Fatalf("deleted segments = %+v", store.deleted)
	}
	if _, err := repo.GetObjectVersion(ctx, bucket.BucketID, "logs/current.txt", version.VersionID); err == nil {
		t.Fatalf("GetObjectVersion(deleted) error = nil, want not found")
	}
}

func TestExpirationWorkerExpiresNoncurrentVersion(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	ctx := t.Context()
	bucket := createLifecycleWorkerBucket(t, repo)
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	first := putLifecycleTestObject(t, repo, bucket.BucketID, "logs/versioned.txt", storage.SegmentRef{SegmentID: "segment-first", SizeBytes: 5})
	clock = now.AddDate(0, 0, -2)
	second := putLifecycleTestObject(t, repo, bucket.BucketID, "logs/versioned.txt", storage.SegmentRef{SegmentID: "segment-second", SizeBytes: 6})
	clock = now
	putExpirationLifecycleRule(t, repo, bucket.BucketID, "logs/", 0, 7)

	store := &workerStore{}
	result, err := ExpirationWorker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now },
	}.RunOnce(ctx, PlanRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("RunOnce() error = %v", err)
	}
	if result.Planned != 1 || result.Deleted != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v", result)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "segment-first" {
		t.Fatalf("deleted segments = %+v", store.deleted)
	}
	if _, err := repo.GetObjectVersion(ctx, bucket.BucketID, "logs/versioned.txt", first.VersionID); err == nil {
		t.Fatalf("GetObjectVersion(first deleted) error = nil, want not found")
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "logs/versioned.txt")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if head.VersionID != second.VersionID {
		t.Fatalf("head version = %q, want %q", head.VersionID, second.VersionID)
	}
}

func createLifecycleWorkerBucket(t *testing.T, repo meta.Repository) model.Bucket {
	t.Helper()
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "worker-" + t.Name(),
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	return bucket
}

func createLifecycleWorkerUpload(t *testing.T, repo meta.Repository, bucketID, key, segmentID string) model.MultipartUpload {
	t.Helper()
	upload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{
		BucketID: bucketID,
		Key:      key,
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	if _, _, err := repo.PutMultipartPart(t.Context(), meta.PutMultipartPartRequest{
		BucketID:   bucketID,
		Key:        key,
		UploadID:   upload.UploadID,
		PartNumber: 1,
		SizeBytes:  5,
		ETag:       `"` + segmentID + `"`,
		SegmentRef: storage.SegmentRef{
			SegmentID: segmentID,
			SizeBytes: 5,
		},
	}); err != nil {
		t.Fatalf("PutMultipartPart() error = %v", err)
	}
	return upload
}

func putAbortLifecycleRule(t *testing.T, repo meta.Repository, bucketID, prefix string, days int) {
	t.Helper()
	if _, err := repo.PutBucketLifecycle(t.Context(), meta.BucketLifecycleRequest{
		BucketID: bucketID,
		Configuration: model.BucketLifecycleConfiguration{
			Rules: []model.LifecycleRule{{
				ID:     "abort-old-mpu",
				Status: model.LifecycleRuleEnabled,
				Prefix: prefix,
				AbortIncompleteMultipartUpload: model.LifecycleAbortIncompleteMultipartUpload{
					DaysAfterInitiation: days,
				},
			}},
		},
	}); err != nil {
		t.Fatalf("PutBucketLifecycle() error = %v", err)
	}
}

func putExpirationLifecycleRule(t *testing.T, repo meta.Repository, bucketID, prefix string, currentDays, noncurrentDays int) {
	t.Helper()
	rule := model.LifecycleRule{
		ID:     "expire",
		Status: model.LifecycleRuleEnabled,
		Prefix: prefix,
	}
	if currentDays > 0 {
		rule.Expiration = model.LifecycleExpiration{Days: currentDays}
	}
	if noncurrentDays > 0 {
		rule.NoncurrentVersionExpiration = model.LifecycleNoncurrentVersionExpiration{NoncurrentDays: noncurrentDays}
	}
	if _, err := repo.PutBucketLifecycle(t.Context(), meta.BucketLifecycleRequest{
		BucketID: bucketID,
		Configuration: model.BucketLifecycleConfiguration{
			Rules: []model.LifecycleRule{rule},
		},
	}); err != nil {
		t.Fatalf("PutBucketLifecycle() error = %v", err)
	}
}

type workerStore struct {
	deleted   []string
	deleteErr error
}

func (s *workerStore) PutSegment(context.Context, storage.PutSegmentRequest) (storage.SegmentRef, error) {
	return storage.SegmentRef{}, nil
}

func (s *workerStore) GetSegment(context.Context, storage.SegmentRef, uint64, uint64) (io.ReadCloser, error) {
	return nil, storage.ErrNotFound
}

func (s *workerStore) DeleteSegment(_ context.Context, ref storage.SegmentRef, _ storage.DeleteReason) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deleted = append(s.deleted, ref.SegmentID)
	return nil
}

type workerOrphans struct {
	marked []string
}

func (o *workerOrphans) MarkOrphan(_ context.Context, ref storage.SegmentRef, _ storage.DeleteReason) error {
	o.marked = append(o.marked, ref.SegmentID)
	return nil
}

func (o *workerOrphans) ListGCCandidates(context.Context, int) ([]storage.GCCandidate, error) {
	return nil, nil
}

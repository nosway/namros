package quota_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/quota"
)

func TestReconcilerReconcileTenantUsage(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 11, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	if _, err := repo.CreateTenant(ctx, meta.CreateTenantRequest{
		TenantID:    "tenant-usage-reconcile",
		DisplayName: "Tenant Usage Reconcile",
	}); err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	photos, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-usage-reconcile",
		Name:     "usage-photos",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(photos) error = %v", err)
	}
	logs, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-usage-reconcile",
		Name:     "usage-logs",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(logs) error = %v", err)
	}
	for _, object := range []struct {
		bucketID string
		key      string
		size     int64
	}{
		{bucketID: photos.BucketID, key: "a.bin", size: 10},
		{bucketID: photos.BucketID, key: "b.bin", size: 20},
		{bucketID: logs.BucketID, key: "c.bin", size: 30},
	} {
		if _, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
			BucketID:  object.bucketID,
			Key:       object.key,
			SizeBytes: object.size,
			ETag:      `"` + object.key + `"`,
		}); err != nil {
			t.Fatalf("PutObjectVersion(%s) error = %v", object.key, err)
		}
	}
	if _, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: photos.BucketID,
		Key:      "upload-a",
	}); err != nil {
		t.Fatalf("CreateMultipartUpload(upload-a) error = %v", err)
	}
	if _, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: logs.BucketID,
		Key:      "upload-b",
	}); err != nil {
		t.Fatalf("CreateMultipartUpload(upload-b) error = %v", err)
	}
	aborted, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: photos.BucketID,
		Key:      "upload-aborted",
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(upload-aborted) error = %v", err)
	}
	if _, err := repo.AbortMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: aborted.BucketID,
		Key:      aborted.Key,
		UploadID: aborted.UploadID,
	}); err != nil {
		t.Fatalf("AbortMultipartUpload() error = %v", err)
	}

	result, err := quota.Reconciler{
		Repository: repo,
		Now:        func() time.Time { return now },
	}.ReconcileTenantUsage(ctx, quota.ReconcileTenantUsageRequest{
		TenantID:         "tenant-usage-reconcile",
		PageSize:         1,
		ReconciliationID: "usage-reconcile-1",
	})
	if err != nil {
		t.Fatalf("ReconcileTenantUsage() error = %v", err)
	}
	if result.BucketsScanned != 2 || result.VersionsScanned != 3 || result.UploadsScanned != 2 {
		t.Fatalf("scan counters = %+v, want 2 buckets, 3 versions, 2 active uploads", result)
	}
	if result.Usage.ObjectBytes != 60 || result.Usage.ObjectCount != 3 || result.Usage.ActiveUploads != 2 {
		t.Fatalf("usage = %+v, want bytes=60 objects=3 active_uploads=2", result.Usage)
	}
	if !result.Usage.ReconciledAt.Equal(now) || result.Usage.ReconciliationID != "usage-reconcile-1" {
		t.Fatalf("usage reconciliation metadata = %+v", result.Usage)
	}
	got, err := repo.GetTenantUsage(ctx, "tenant-usage-reconcile")
	if err != nil {
		t.Fatalf("GetTenantUsage() error = %v", err)
	}
	if got.ObjectBytes != result.Usage.ObjectBytes || got.ObjectCount != result.Usage.ObjectCount || got.ActiveUploads != result.Usage.ActiveUploads {
		t.Fatalf("stored usage = %+v, result = %+v", got, result.Usage)
	}
}

func TestReconcilerReconcileTenantUsageValidation(t *testing.T) {
	repo := memory.New()
	if _, err := (quota.Reconciler{}).ReconcileTenantUsage(t.Context(), quota.ReconcileTenantUsageRequest{
		TenantID: "tenant-a",
	}); err == nil {
		t.Fatalf("ReconcileTenantUsage(nil repo) error = nil")
	}
	if _, err := (quota.Reconciler{Repository: repo}).ReconcileTenantUsage(t.Context(), quota.ReconcileTenantUsageRequest{}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("ReconcileTenantUsage(empty tenant) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := (quota.Reconciler{Repository: repo}).ReconcileTenantUsage(t.Context(), quota.ReconcileTenantUsageRequest{
		TenantID: "missing-tenant",
	}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("ReconcileTenantUsage(missing tenant) error = %v, want ErrNotFound", err)
	}
}

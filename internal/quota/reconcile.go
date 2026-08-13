package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
)

const defaultReconcilePageSize = 1000

type Reconciler struct {
	Repository meta.Repository
	Now        func() time.Time
}

type ReconcileTenantUsageRequest struct {
	TenantID         string
	PageSize         int
	ReconciliationID string
}

type ReconcileTenantUsageResult struct {
	Usage           model.TenantUsage
	BucketsScanned  int
	VersionsScanned int
	UploadsScanned  int
}

func (r Reconciler) ReconcileTenantUsage(ctx context.Context, req ReconcileTenantUsageRequest) (ReconcileTenantUsageResult, error) {
	if r.Repository == nil {
		return ReconcileTenantUsageResult{}, fmt.Errorf("metadata repository is required")
	}
	if req.TenantID == "" {
		return ReconcileTenantUsageResult{}, fmt.Errorf("%w: tenant id is required", meta.ErrInvalidArgument)
	}
	if _, err := r.Repository.GetTenant(ctx, req.TenantID); err != nil {
		return ReconcileTenantUsageResult{}, err
	}
	pageSize := normalizePageSize(req.PageSize)
	buckets, err := r.Repository.ListBuckets(ctx, req.TenantID)
	if err != nil {
		return ReconcileTenantUsageResult{}, err
	}
	result := ReconcileTenantUsageResult{BucketsScanned: len(buckets)}
	var objectBytes int64
	var objectCount int64
	var activeUploads int64
	for _, bucket := range buckets {
		bytes, count, versions, err := r.reconcileBucketVersions(ctx, bucket.BucketID, pageSize)
		if err != nil {
			return ReconcileTenantUsageResult{}, err
		}
		objectBytes += bytes
		objectCount += count
		result.VersionsScanned += versions
		uploads, scanned, err := r.reconcileBucketUploads(ctx, bucket.BucketID, pageSize)
		if err != nil {
			return ReconcileTenantUsageResult{}, err
		}
		activeUploads += uploads
		result.UploadsScanned += scanned
	}
	usage, err := r.Repository.PutTenantUsage(ctx, meta.TenantUsageRequest{
		TenantID:         req.TenantID,
		ObjectBytes:      objectBytes,
		ObjectCount:      objectCount,
		ActiveUploads:    activeUploads,
		ReconciledAt:     r.now(),
		ReconciliationID: req.ReconciliationID,
	})
	if err != nil {
		return ReconcileTenantUsageResult{}, err
	}
	result.Usage = usage
	return result, nil
}

func (r Reconciler) reconcileBucketVersions(ctx context.Context, bucketID string, pageSize int) (int64, int64, int, error) {
	var objectBytes int64
	var objectCount int64
	var scanned int
	keyMarker := ""
	versionIDMarker := ""
	for {
		page, err := r.Repository.ListObjectVersions(ctx, meta.ListObjectVersionsRequest{
			BucketID:        bucketID,
			KeyMarker:       keyMarker,
			VersionIDMarker: versionIDMarker,
			MaxKeys:         pageSize,
		})
		if err != nil {
			return 0, 0, 0, err
		}
		for _, entry := range page.Versions {
			version := entry.Version
			scanned++
			if version.State != model.ObjectVersionCommitted || version.DeleteMarker {
				continue
			}
			if version.SizeBytes < 0 {
				return 0, 0, 0, fmt.Errorf("%w: object version %q has negative size", meta.ErrInvalidArgument, version.VersionID)
			}
			objectBytes += version.SizeBytes
			objectCount++
		}
		if !page.IsTruncated {
			return objectBytes, objectCount, scanned, nil
		}
		keyMarker = page.NextKeyMarker
		versionIDMarker = page.NextVersionIDMarker
	}
}

func (r Reconciler) reconcileBucketUploads(ctx context.Context, bucketID string, pageSize int) (int64, int, error) {
	var active int64
	var scanned int
	keyMarker := ""
	uploadIDMarker := ""
	for {
		page, err := r.Repository.ListMultipartUploads(ctx, meta.ListMultipartUploadsRequest{
			BucketID:       bucketID,
			KeyMarker:      keyMarker,
			UploadIDMarker: uploadIDMarker,
			MaxUploads:     pageSize,
		})
		if err != nil {
			return 0, 0, err
		}
		for _, upload := range page.Uploads {
			scanned++
			if upload.State == "" || upload.State == model.MultipartUploadActive {
				active++
			}
		}
		if !page.IsTruncated {
			return active, scanned, nil
		}
		keyMarker = page.NextKeyMarker
		uploadIDMarker = page.NextUploadIDMarker
	}
}

func (r Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

func normalizePageSize(pageSize int) int {
	if pageSize <= 0 {
		return defaultReconcilePageSize
	}
	if pageSize > defaultReconcilePageSize {
		return defaultReconcilePageSize
	}
	return pageSize
}

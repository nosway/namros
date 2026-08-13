package meta

import (
	"fmt"
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

func BuildBucketQuota(existing model.BucketQuota, req BucketQuotaRequest, now time.Time) (model.BucketQuota, error) {
	if req.BucketID == "" {
		return model.BucketQuota{}, fmt.Errorf("%w: bucket id is required", ErrInvalidArgument)
	}
	if req.MaxObjectSizeBytes < 0 {
		return model.BucketQuota{}, fmt.Errorf("%w: max object size cannot be negative", ErrInvalidArgument)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	quota := model.BucketQuota{
		BucketID:           req.BucketID,
		MaxObjectSizeBytes: req.MaxObjectSizeBytes,
		CreatedAt:          existing.CreatedAt,
		UpdatedAt:          now,
	}
	if quota.CreatedAt.IsZero() {
		quota.CreatedAt = now
	}
	return quota, nil
}

func CloneBucketQuotaRecord(in model.BucketQuota) model.BucketQuota {
	return in
}

func BuildTenantQuota(existing model.TenantQuota, req TenantQuotaRequest, now time.Time) (model.TenantQuota, error) {
	if req.TenantID == "" {
		return model.TenantQuota{}, fmt.Errorf("%w: tenant id is required", ErrInvalidArgument)
	}
	if req.MaxBytes < 0 {
		return model.TenantQuota{}, fmt.Errorf("%w: max bytes cannot be negative", ErrInvalidArgument)
	}
	if req.MaxObjects < 0 {
		return model.TenantQuota{}, fmt.Errorf("%w: max objects cannot be negative", ErrInvalidArgument)
	}
	if req.MaxActiveUploads < 0 {
		return model.TenantQuota{}, fmt.Errorf("%w: max active uploads cannot be negative", ErrInvalidArgument)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	quota := model.TenantQuota{
		TenantID:         req.TenantID,
		MaxBytes:         req.MaxBytes,
		MaxObjects:       req.MaxObjects,
		MaxActiveUploads: req.MaxActiveUploads,
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        now,
	}
	if quota.CreatedAt.IsZero() {
		quota.CreatedAt = now
	}
	return quota, nil
}

func CloneTenantQuotaRecord(in model.TenantQuota) model.TenantQuota {
	return in
}

func BuildTenantUsage(existing model.TenantUsage, req TenantUsageRequest, now time.Time) (model.TenantUsage, error) {
	if req.TenantID == "" {
		return model.TenantUsage{}, fmt.Errorf("%w: tenant id is required", ErrInvalidArgument)
	}
	if req.ObjectBytes < 0 {
		return model.TenantUsage{}, fmt.Errorf("%w: object bytes cannot be negative", ErrInvalidArgument)
	}
	if req.ObjectCount < 0 {
		return model.TenantUsage{}, fmt.Errorf("%w: object count cannot be negative", ErrInvalidArgument)
	}
	if req.ActiveUploads < 0 {
		return model.TenantUsage{}, fmt.Errorf("%w: active uploads cannot be negative", ErrInvalidArgument)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	reconciledAt := req.ReconciledAt
	if reconciledAt.IsZero() {
		reconciledAt = now
	}
	usage := model.TenantUsage{
		TenantID:          req.TenantID,
		ObjectBytes:       req.ObjectBytes,
		ObjectCount:       req.ObjectCount,
		ActiveUploads:     req.ActiveUploads,
		ReconciledAt:      reconciledAt.UTC(),
		UpdatedAt:         now,
		CreatedAt:         existing.CreatedAt,
		ReconciliationID:  req.ReconciliationID,
		ReconciliationErr: req.ReconciliationErr,
	}
	if usage.CreatedAt.IsZero() {
		usage.CreatedAt = now
	}
	return usage, nil
}

func CloneTenantUsageRecord(in model.TenantUsage) model.TenantUsage {
	return in
}

func ApplyTenantActiveUploadDelta(existing model.TenantUsage, tenantID string, delta int64, now time.Time) (model.TenantUsage, error) {
	if tenantID == "" {
		return model.TenantUsage{}, fmt.Errorf("%w: tenant id is required", ErrInvalidArgument)
	}
	if existing.TenantID != "" && existing.TenantID != tenantID {
		return model.TenantUsage{}, fmt.Errorf("%w: tenant usage record belongs to %q", ErrInvalidArgument, existing.TenantID)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	activeUploads := existing.ActiveUploads + delta
	if activeUploads < 0 {
		activeUploads = 0
	}
	usage := existing
	usage.TenantID = tenantID
	usage.ActiveUploads = activeUploads
	usage.UpdatedAt = now
	if usage.CreatedAt.IsZero() {
		usage.CreatedAt = now
	}
	return usage, nil
}

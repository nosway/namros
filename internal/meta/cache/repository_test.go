package cache

import (
	"context"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
)

func TestRepositoryCachesAccessKeysUntilTTLAndInvalidatesOnWrite(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	base := memory.New()
	counted := &countingRepository{Repository: base}
	repo := NewWithClock(counted, time.Minute, func() time.Time { return now })
	ctx := t.Context()

	if _, err := repo.PutAccessKey(ctx, meta.PutAccessKeyRequest{
		TenantID:    "tenant-1",
		AccessKeyID: "ak-1",
		SecretHash:  "hash-1",
		Status:      model.AccessKeyActive,
		Permissions: []string{"s3:GetObject"},
	}); err != nil {
		t.Fatalf("PutAccessKey() error = %v", err)
	}
	first, err := repo.GetAccessKey(ctx, "ak-1")
	if err != nil {
		t.Fatalf("GetAccessKey(first) error = %v", err)
	}
	first.Permissions[0] = "mutated"
	second, err := repo.GetAccessKey(ctx, "ak-1")
	if err != nil {
		t.Fatalf("GetAccessKey(second) error = %v", err)
	}
	if counted.getAccessKeyCalls != 1 {
		t.Fatalf("underlying GetAccessKey calls = %d, want 1", counted.getAccessKeyCalls)
	}
	if second.Permissions[0] != "s3:GetObject" {
		t.Fatalf("cached access key was mutated: %+v", second)
	}
	if _, err := repo.PutAccessKey(ctx, meta.PutAccessKeyRequest{
		TenantID:    "tenant-1",
		AccessKeyID: "ak-1",
		SecretHash:  "hash-2",
		Status:      model.AccessKeyDisabled,
	}); err != nil {
		t.Fatalf("PutAccessKey(update) error = %v", err)
	}
	updated, err := repo.GetAccessKey(ctx, "ak-1")
	if err != nil {
		t.Fatalf("GetAccessKey(updated) error = %v", err)
	}
	if updated.Status != model.AccessKeyDisabled {
		t.Fatalf("updated status = %q, want disabled", updated.Status)
	}
	if counted.getAccessKeyCalls != 2 {
		t.Fatalf("underlying GetAccessKey calls after invalidation = %d, want 2", counted.getAccessKeyCalls)
	}
	now = now.Add(2 * time.Minute)
	if _, err := repo.GetAccessKey(ctx, "ak-1"); err != nil {
		t.Fatalf("GetAccessKey(after ttl) error = %v", err)
	}
	if counted.getAccessKeyCalls != 3 {
		t.Fatalf("underlying GetAccessKey calls after ttl = %d, want 3", counted.getAccessKeyCalls)
	}
}

func TestRepositoryCachesBucketsByNameAndInvalidatesOnBucketConfigWrite(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	base := memory.New()
	counted := &countingRepository{Repository: base}
	repo := NewWithClock(counted, time.Minute, func() time.Time { return now })
	ctx := t.Context()

	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "cached-bucket",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	first, err := repo.GetBucketByName(ctx, "cached-bucket")
	if err != nil {
		t.Fatalf("GetBucketByName(first) error = %v", err)
	}
	first.CORSRules = []model.CORSRule{{AllowedOrigins: []string{"mutated"}}}
	second, err := repo.GetBucketByName(ctx, "cached-bucket")
	if err != nil {
		t.Fatalf("GetBucketByName(second) error = %v", err)
	}
	if counted.getBucketByNameCalls != 1 {
		t.Fatalf("underlying GetBucketByName calls = %d, want 1", counted.getBucketByNameCalls)
	}
	if len(second.CORSRules) != 0 {
		t.Fatalf("cached bucket was mutated: %+v", second)
	}
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	updated, err := repo.GetBucketByName(ctx, "cached-bucket")
	if err != nil {
		t.Fatalf("GetBucketByName(updated) error = %v", err)
	}
	if updated.VersioningState != model.BucketVersioningEnabled {
		t.Fatalf("updated versioning = %q, want Enabled", updated.VersioningState)
	}
	if counted.getBucketByNameCalls != 2 {
		t.Fatalf("underlying GetBucketByName calls after invalidation = %d, want 2", counted.getBucketByNameCalls)
	}
}

type countingRepository struct {
	meta.Repository
	getAccessKeyCalls    int
	getBucketByNameCalls int
}

func (r *countingRepository) GetAccessKey(ctx context.Context, accessKeyID string) (model.AccessKey, error) {
	r.getAccessKeyCalls++
	return r.Repository.GetAccessKey(ctx, accessKeyID)
}

func (r *countingRepository) GetBucketByName(ctx context.Context, name string) (model.Bucket, error) {
	r.getBucketByNameCalls++
	return r.Repository.GetBucketByName(ctx, name)
}

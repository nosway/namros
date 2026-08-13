package cache

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
)

type Repository struct {
	meta.Repository

	ttl time.Duration
	now func() time.Time

	mu            sync.Mutex
	accessKeys    map[string]entry[model.AccessKey]
	bucketsByName map[string]entry[model.Bucket]
}

type entry[T any] struct {
	value     T
	expiresAt time.Time
}

func New(repo meta.Repository, ttl time.Duration) *Repository {
	return NewWithClock(repo, ttl, func() time.Time { return time.Now().UTC() })
}

func NewWithClock(repo meta.Repository, ttl time.Duration, now func() time.Time) *Repository {
	return &Repository{
		Repository:    repo,
		ttl:           ttl,
		now:           now,
		accessKeys:    make(map[string]entry[model.AccessKey]),
		bucketsByName: make(map[string]entry[model.Bucket]),
	}
}

func (r *Repository) GetAccessKey(ctx context.Context, accessKeyID string) (model.AccessKey, error) {
	if r.cacheEnabled() {
		if value, ok := r.getAccessKey(accessKeyID); ok {
			return value, nil
		}
	}
	value, err := r.Repository.GetAccessKey(ctx, accessKeyID)
	if err != nil {
		return model.AccessKey{}, err
	}
	if r.cacheEnabled() {
		r.setAccessKey(accessKeyID, value)
	}
	return clone(value), nil
}

func (r *Repository) PutAccessKey(ctx context.Context, req meta.PutAccessKeyRequest) (model.AccessKey, error) {
	value, err := r.Repository.PutAccessKey(ctx, req)
	if err != nil {
		return model.AccessKey{}, err
	}
	r.mu.Lock()
	delete(r.accessKeys, req.AccessKeyID)
	r.mu.Unlock()
	return value, nil
}

func (r *Repository) GetBucketByName(ctx context.Context, name string) (model.Bucket, error) {
	if r.cacheEnabled() {
		if value, ok := r.getBucketByName(name); ok {
			return value, nil
		}
	}
	value, err := r.Repository.GetBucketByName(ctx, name)
	if err != nil {
		return model.Bucket{}, err
	}
	if r.cacheEnabled() {
		r.setBucketByName(name, value)
	}
	return clone(value), nil
}

func (r *Repository) CreateBucket(ctx context.Context, req meta.CreateBucketRequest) (model.Bucket, error) {
	value, err := r.Repository.CreateBucket(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) DeleteBucket(ctx context.Context, bucketID string) error {
	if err := r.Repository.DeleteBucket(ctx, bucketID); err != nil {
		return err
	}
	r.invalidateBuckets()
	return nil
}

func (r *Repository) PutBucketVersioning(ctx context.Context, req meta.PutBucketVersioningRequest) (model.Bucket, error) {
	value, err := r.Repository.PutBucketVersioning(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) PutBucketCORS(ctx context.Context, req meta.BucketCORSRequest) (model.Bucket, error) {
	value, err := r.Repository.PutBucketCORS(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) DeleteBucketCORS(ctx context.Context, bucketID string) (model.Bucket, error) {
	value, err := r.Repository.DeleteBucketCORS(ctx, bucketID)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) PutBucketLifecycle(ctx context.Context, req meta.BucketLifecycleRequest) (model.Bucket, error) {
	value, err := r.Repository.PutBucketLifecycle(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) DeleteBucketLifecycle(ctx context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	value, err := r.Repository.DeleteBucketLifecycle(ctx, bucketID, audit)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) PutBucketEncryption(ctx context.Context, req meta.BucketEncryptionRequest) (model.Bucket, error) {
	value, err := r.Repository.PutBucketEncryption(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) DeleteBucketEncryption(ctx context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	value, err := r.Repository.DeleteBucketEncryption(ctx, bucketID, audit)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) PutBucketPolicy(ctx context.Context, req meta.BucketPolicyRequest) (model.Bucket, error) {
	value, err := r.Repository.PutBucketPolicy(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) DeleteBucketPolicy(ctx context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	value, err := r.Repository.DeleteBucketPolicy(ctx, bucketID, audit)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) PutBucketObjectLock(ctx context.Context, req meta.BucketObjectLockRequest) (model.Bucket, error) {
	value, err := r.Repository.PutBucketObjectLock(ctx, req)
	if err != nil {
		return model.Bucket{}, err
	}
	r.invalidateBuckets()
	return value, nil
}

func (r *Repository) cacheEnabled() bool {
	return r != nil && r.Repository != nil && r.ttl > 0
}

func (r *Repository) getAccessKey(accessKeyID string) (model.AccessKey, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	got, ok := r.accessKeys[accessKeyID]
	if !ok || !r.now().UTC().Before(got.expiresAt) {
		if ok {
			delete(r.accessKeys, accessKeyID)
		}
		return model.AccessKey{}, false
	}
	return clone(got.value), true
}

func (r *Repository) setAccessKey(accessKeyID string, value model.AccessKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.accessKeys[accessKeyID] = entry[model.AccessKey]{
		value:     clone(value),
		expiresAt: r.now().UTC().Add(r.ttl),
	}
}

func (r *Repository) getBucketByName(name string) (model.Bucket, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	got, ok := r.bucketsByName[name]
	if !ok || !r.now().UTC().Before(got.expiresAt) {
		if ok {
			delete(r.bucketsByName, name)
		}
		return model.Bucket{}, false
	}
	return clone(got.value), true
}

func (r *Repository) setBucketByName(name string, value model.Bucket) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bucketsByName[name] = entry[model.Bucket]{
		value:     clone(value),
		expiresAt: r.now().UTC().Add(r.ttl),
	}
}

func (r *Repository) invalidateBuckets() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.bucketsByName)
}

func clone[T any](value T) T {
	encoded, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out T
	if err := json.Unmarshal(encoded, &out); err != nil {
		return value
	}
	return out
}

var _ meta.Repository = (*Repository)(nil)

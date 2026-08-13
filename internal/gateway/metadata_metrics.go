package gateway

import (
	"context"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/opsmetrics"
)

type metricsRepository struct {
	meta.Repository
	metrics *opsmetrics.GatewayMetrics
}

func newMetricsRepository(next meta.Repository, metrics *opsmetrics.GatewayMetrics) meta.Repository {
	if next == nil || metrics == nil {
		return next
	}
	if _, ok := next.(*metricsRepository); ok {
		return next
	}
	return &metricsRepository{Repository: next, metrics: metrics}
}

func (r *metricsRepository) CreateBucket(ctx context.Context, req meta.CreateBucketRequest) (model.Bucket, error) {
	start := time.Now()
	out, err := r.Repository.CreateBucket(ctx, req)
	r.metrics.ObserveMetadataDuration("create_bucket", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetAccessKey(ctx context.Context, accessKeyID string) (model.AccessKey, error) {
	start := time.Now()
	out, err := r.Repository.GetAccessKey(ctx, accessKeyID)
	r.metrics.ObserveMetadataDuration("get_access_key", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetBucketByName(ctx context.Context, name string) (model.Bucket, error) {
	start := time.Now()
	out, err := r.Repository.GetBucketByName(ctx, name)
	r.metrics.ObserveMetadataDuration("get_bucket_by_name", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) ListBuckets(ctx context.Context, tenantID string) ([]model.Bucket, error) {
	start := time.Now()
	out, err := r.Repository.ListBuckets(ctx, tenantID)
	r.metrics.ObserveMetadataDuration("list_buckets", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) DeleteBucket(ctx context.Context, bucketID string) error {
	start := time.Now()
	err := r.Repository.DeleteBucket(ctx, bucketID)
	r.metrics.ObserveMetadataDuration("delete_bucket", time.Since(start), err)
	return err
}

func (r *metricsRepository) BeginPutObject(ctx context.Context, req meta.BeginPutObjectRequest) (model.PendingObjectVersion, error) {
	start := time.Now()
	out, err := r.Repository.BeginPutObject(ctx, req)
	r.metrics.ObserveMetadataDuration("begin_put_object", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) CommitObjectVersion(ctx context.Context, req meta.CommitObjectVersionRequest) (model.ObjectHead, error) {
	start := time.Now()
	out, err := r.Repository.CommitObjectVersion(ctx, req)
	r.metrics.ObserveMetadataDuration("commit_object_version", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) PutObjectVersion(ctx context.Context, req meta.PutObjectVersionRequest) (meta.PutObjectVersionResult, error) {
	start := time.Now()
	out, err := r.Repository.PutObjectVersion(ctx, req)
	r.metrics.ObserveMetadataDuration("put_object_version", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetObjectHead(ctx context.Context, bucketID, key string) (model.ObjectHead, error) {
	start := time.Now()
	out, err := r.Repository.GetObjectHead(ctx, bucketID, key)
	r.metrics.ObserveMetadataDuration("get_object_head", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetObjectVersion(ctx context.Context, bucketID, key, versionID string) (model.ObjectVersion, error) {
	start := time.Now()
	out, err := r.Repository.GetObjectVersion(ctx, bucketID, key, versionID)
	r.metrics.ObserveMetadataDuration("get_object_version", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) DeleteObject(ctx context.Context, req meta.DeleteObjectRequest) (model.DeleteResult, error) {
	start := time.Now()
	out, err := r.Repository.DeleteObject(ctx, req)
	r.metrics.ObserveMetadataDuration("delete_object", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) ListObjects(ctx context.Context, req meta.ListObjectsRequest) (model.ListObjectsResult, error) {
	start := time.Now()
	out, err := r.Repository.ListObjects(ctx, req)
	r.metrics.ObserveMetadataDuration("list_objects", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) PutAdminAuditEvent(ctx context.Context, req meta.PutAdminAuditEventRequest) (model.AuditEvent, error) {
	start := time.Now()
	out, err := r.Repository.PutAdminAuditEvent(ctx, req)
	r.metrics.ObserveMetadataDuration("put_admin_audit_event", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) PutAdminAuditEvents(ctx context.Context, req meta.PutAdminAuditEventsRequest) ([]model.AuditEvent, error) {
	start := time.Now()
	out, err := r.Repository.PutAdminAuditEvents(ctx, req)
	r.metrics.ObserveMetadataDuration("put_admin_audit_events", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) CreateMultipartUpload(ctx context.Context, req meta.CreateMultipartUploadRequest) (model.MultipartUpload, error) {
	start := time.Now()
	out, err := r.Repository.CreateMultipartUpload(ctx, req)
	r.metrics.ObserveMetadataDuration("create_multipart_upload", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetMultipartUpload(ctx context.Context, req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	start := time.Now()
	out, err := r.Repository.GetMultipartUpload(ctx, req)
	r.metrics.ObserveMetadataDuration("get_multipart_upload", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) PutMultipartPart(ctx context.Context, req meta.PutMultipartPartRequest) (model.MultipartPart, *model.MultipartPart, error) {
	start := time.Now()
	out, replaced, err := r.Repository.PutMultipartPart(ctx, req)
	r.metrics.ObserveMetadataDuration("put_multipart_part", time.Since(start), err)
	return out, replaced, err
}

func (r *metricsRepository) ListMultipartParts(ctx context.Context, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	start := time.Now()
	out, err := r.Repository.ListMultipartParts(ctx, req)
	r.metrics.ObserveMetadataDuration("list_multipart_parts", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetMultipartParts(ctx context.Context, req meta.GetMultipartPartsRequest) ([]model.MultipartPart, error) {
	start := time.Now()
	out, err := r.Repository.GetMultipartParts(ctx, req)
	r.metrics.ObserveMetadataDuration("get_multipart_parts", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) GetMultipartCompletion(ctx context.Context, req meta.MultipartUploadRequest) (model.MultipartCompletionRecord, error) {
	start := time.Now()
	out, err := r.Repository.GetMultipartCompletion(ctx, req)
	r.metrics.ObserveMetadataDuration("get_multipart_completion", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) PrepareMultipartCompletion(ctx context.Context, req meta.PrepareMultipartCompletionRequest) (model.MultipartCompletionRecord, error) {
	start := time.Now()
	out, err := r.Repository.PrepareMultipartCompletion(ctx, req)
	r.metrics.ObserveMetadataDuration("prepare_multipart_completion", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) MarkMultipartCompletionPublished(ctx context.Context, req meta.MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error) {
	start := time.Now()
	out, err := r.Repository.MarkMultipartCompletionPublished(ctx, req)
	r.metrics.ObserveMetadataDuration("mark_multipart_completion_published", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) MarkMultipartCompletionCompleted(ctx context.Context, req meta.MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error) {
	start := time.Now()
	out, err := r.Repository.MarkMultipartCompletionCompleted(ctx, req)
	r.metrics.ObserveMetadataDuration("mark_multipart_completion_completed", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) CompleteMultipartUpload(ctx context.Context, req meta.CompleteMultipartUploadRequest) (model.MultipartUpload, error) {
	start := time.Now()
	out, err := r.Repository.CompleteMultipartUpload(ctx, req)
	r.metrics.ObserveMetadataDuration("complete_multipart_upload", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) AbortMultipartUpload(ctx context.Context, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	start := time.Now()
	out, err := r.Repository.AbortMultipartUpload(ctx, req)
	r.metrics.ObserveMetadataDuration("abort_multipart_upload", time.Since(start), err)
	return out, err
}

func (r *metricsRepository) CleanupMultipartUploadParts(ctx context.Context, req meta.CleanupMultipartUploadPartsRequest) (meta.CleanupMultipartUploadPartsResult, error) {
	start := time.Now()
	out, err := r.Repository.CleanupMultipartUploadParts(ctx, req)
	r.metrics.ObserveMetadataDuration("cleanup_multipart_upload_parts", time.Since(start), err)
	return out, err
}

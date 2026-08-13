package gateway

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/s3api/s3err"
)

func (h s3Handler) bucketQuotaAllowsObjectWrite(c *gin.Context, bucket model.Bucket, sizeBytes uint64) bool {
	start := time.Now()
	if err := h.checkBucketObjectSizeQuota(c.Request.Context(), bucket.BucketID, sizeBytes); err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			h.deps.GatewayMetrics.ObserveAdmissionDecision("bucket_quota", "not_configured", true, time.Since(start))
			return true
		}
		if errors.Is(err, errBucketQuotaExceeded) {
			h.deps.GatewayMetrics.ObserveAdmissionDecision("bucket_quota", "max_object_size", false, time.Since(start))
			writeS3Error(c, s3err.AccessDenied(err.Error()))
			return false
		}
		h.deps.GatewayMetrics.ObserveAdmissionDecision("bucket_quota", "metadata_error", false, time.Since(start))
		writeS3Error(c, s3err.ServiceUnavailable(err.Error()))
		return false
	}
	h.deps.GatewayMetrics.ObserveAdmissionDecision("bucket_quota", "allowed", true, time.Since(start))
	return true
}

func (h s3Handler) checkBucketObjectSizeQuota(ctx context.Context, bucketID string, sizeBytes uint64) error {
	if h.deps.Metadata == nil {
		return errors.New("metadata repository is unavailable")
	}
	quota, err := h.deps.Metadata.GetBucketQuota(ctx, bucketID)
	if err != nil {
		return err
	}
	if quota.MaxObjectSizeBytes > 0 && sizeBytes > uint64(quota.MaxObjectSizeBytes) {
		return fmt.Errorf("%w: object size %d exceeds bucket max object size %d", errBucketQuotaExceeded, sizeBytes, quota.MaxObjectSizeBytes)
	}
	return nil
}

var errBucketQuotaExceeded = errors.New("bucket quota exceeded")

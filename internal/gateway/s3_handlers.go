package gateway

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/encryption"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/s3api/routing"
	"github.com/nosway/namros/internal/s3api/s3err"
	"github.com/nosway/namros/internal/s3api/xmlresp"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storageclass"
	"github.com/nosway/namros/internal/trace"
)

type s3Handler struct {
	cfg              config.Config
	deps             Dependencies
	dataBudget       *dataBudget
	requestLimiter   *requestLimiter
	bandwidthLimiter *bandwidthLimiter
}

func (h s3Handler) handle(c *gin.Context) {
	if c.Request.Method == http.MethodOptions {
		h.handleCORSPreflight(c)
		return
	}
	req, err := routing.ParseRequest(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidRequest(err.Error()))
		return
	}
	if err, ok := validateCompatibilityHeaders(c.Request, req.Operation); ok {
		writeS3Error(c, err)
		return
	}
	if err, ok := h.validateEditionFeature(c.Request, req.Operation); ok {
		writeS3Error(c, err)
		return
	}
	releaseDrain, err := h.deps.GatewayDrain.Admit(req.Operation)
	if err != nil {
		writeS3Error(c, s3err.ServiceUnavailable(err.Error()))
		return
	}
	if releaseDrain != nil {
		defer releaseDrain()
	}
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	releaseLimit, ok := h.reserveRequestLimit(c, principal.TenantID, req.Operation)
	if !ok {
		return
	}
	if releaseLimit != nil {
		defer releaseLimit()
	}
	if h.bandwidthLimiter != nil && c.Request.Body != nil {
		c.Request.Body = h.bandwidthLimiter.wrapUpload(c.Request.Body)
	}
	switch req.Operation {
	case routing.OperationListBuckets:
		h.listBuckets(c)
	case routing.OperationCreateBucket:
		h.createBucket(c, req)
	case routing.OperationHeadBucket:
		h.headBucket(c, req)
	case routing.OperationDeleteBucket:
		h.deleteBucket(c, req)
	case routing.OperationGetBucketLocation:
		h.getBucketLocation(c, req)
	case routing.OperationGetBucketVersioning:
		h.getBucketVersioning(c, req)
	case routing.OperationPutBucketVersioning:
		h.putBucketVersioning(c, req)
	case routing.OperationGetBucketCORS:
		h.getBucketCORS(c, req)
	case routing.OperationPutBucketCORS:
		h.putBucketCORS(c, req)
	case routing.OperationDeleteBucketCORS:
		h.deleteBucketCORS(c, req)
	case routing.OperationGetBucketLifecycle:
		h.getBucketLifecycle(c, req)
	case routing.OperationPutBucketLifecycle:
		h.putBucketLifecycle(c, req)
	case routing.OperationDeleteBucketLifecycle:
		h.deleteBucketLifecycle(c, req)
	case routing.OperationGetBucketEncryption:
		h.getBucketEncryption(c, req)
	case routing.OperationPutBucketEncryption:
		h.putBucketEncryption(c, req)
	case routing.OperationDeleteBucketEncryption:
		h.deleteBucketEncryption(c, req)
	case routing.OperationGetBucketObjectLock:
		h.getBucketObjectLock(c, req)
	case routing.OperationPutBucketObjectLock:
		h.putBucketObjectLock(c, req)
	case routing.OperationGetBucketPolicy:
		h.getBucketPolicy(c, req)
	case routing.OperationPutBucketPolicy:
		h.putBucketPolicy(c, req)
	case routing.OperationDeleteBucketPolicy:
		h.deleteBucketPolicy(c, req)
	case routing.OperationGetBucketACL:
		h.getBucketACL(c, req)
	case routing.OperationPutBucketACL:
		h.putBucketACL(c, req)
	case routing.OperationListObjectVersions:
		h.listObjectVersions(c, req)
	case routing.OperationListObjectsV2:
		h.listObjectsV2(c, req)
	case routing.OperationListObjects:
		h.listObjectsV1(c, req)
	case routing.OperationPutObject:
		h.putObject(c, req)
	case routing.OperationCopyObject:
		h.copyObject(c, req)
	case routing.OperationHeadObject:
		h.headObject(c, req)
	case routing.OperationGetObject:
		h.getObject(c, req)
	case routing.OperationDeleteObject:
		h.deleteObject(c, req)
	case routing.OperationDeleteObjects:
		h.deleteObjects(c, req)
	case routing.OperationGetObjectTagging:
		h.getObjectTagging(c, req)
	case routing.OperationPutObjectTagging:
		h.putObjectTagging(c, req)
	case routing.OperationDeleteObjectTagging:
		h.deleteObjectTagging(c, req)
	case routing.OperationGetObjectRetention:
		h.getObjectRetention(c, req)
	case routing.OperationPutObjectRetention:
		h.putObjectRetention(c, req)
	case routing.OperationGetObjectLegalHold:
		h.getObjectLegalHold(c, req)
	case routing.OperationPutObjectLegalHold:
		h.putObjectLegalHold(c, req)
	case routing.OperationGetObjectACL:
		h.getObjectACL(c, req)
	case routing.OperationPutObjectACL:
		h.putObjectACL(c, req)
	case routing.OperationCreateMultipartUpload:
		h.createMultipartUpload(c, req)
	case routing.OperationListMultipartUploads:
		h.listMultipartUploads(c, req)
	case routing.OperationUploadPart:
		if c.GetHeader("x-amz-copy-source") != "" {
			h.uploadPartCopy(c, req)
			return
		}
		h.uploadPart(c, req)
	case routing.OperationListParts:
		h.listParts(c, req)
	case routing.OperationCompleteMultipart:
		h.completeMultipartUpload(c, req)
	case routing.OperationAbortMultipart:
		h.abortMultipartUpload(c, req)
	default:
		if req.Operation == routing.OperationUnsupported {
			writeS3Error(c, s3err.NotImplemented("S3 operation is not implemented"))
			return
		}
		writeS3Error(c, s3err.NotImplemented(string(req.Operation)+" is not implemented"))
	}
}

func (h s3Handler) handleCORSPreflight(c *gin.Context) {
	req, err := routing.ParseRequest(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidRequest(err.Error()))
		return
	}
	rule, ok := matchingCORSRule(c.Request, h.deps.Metadata, req.Bucket, c.Request.Header.Get("Access-Control-Request-Method"))
	if !ok {
		writeS3Error(c, s3err.AccessDenied("CORS preflight is not allowed"))
		return
	}
	writeCORSHeaders(c.Writer.Header(), c.Request, rule, true)
	c.Status(http.StatusOK)
}

func corsResponseMiddleware(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			req, err := routing.ParseRequest(c.Request)
			if err == nil && req.Bucket != "" {
				if rule, ok := matchingCORSRule(c.Request, deps.Metadata, req.Bucket, c.Request.Method); ok {
					writeCORSHeaders(c.Writer.Header(), c.Request, rule, false)
				}
			}
		}
		c.Next()
	}
}

func (h s3Handler) createMultipartUpload(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	tags, err := objectTagsFromHeader(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	objectLockRetention, objectLockLegalHold, err := objectLockStateFromRequest(c.Request, bucket)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	storageClass, err := storageClassFromRequest(c.Request, standardStorageClass())
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	serverSideEncryption, err := serverSideEncryptionFromRequest(c.Request, bucket.DefaultEncryption)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	upload, err := h.deps.Metadata.CreateMultipartUpload(c.Request.Context(), meta.CreateMultipartUploadRequest{
		BucketID:             bucket.BucketID,
		Key:                  req.Key,
		ContentType:          contentType(c.Request),
		StorageClass:         storageClass,
		ServerSideEncryption: serverSideEncryption,
		UserMetadata:         userMetadata(c.Request),
		Tags:                 tags,
		ObjectLockRetention:  objectLockRetention,
		ObjectLockLegalHold:  objectLockLegalHold,
	})
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	writeServerSideEncryptionHeaders(c, upload.ServerSideEncryption)
	_ = xmlresp.Write(c.Writer, http.StatusOK, initiateMultipartUploadResult{
		Bucket:   bucket.Name,
		Key:      req.Key,
		UploadID: upload.UploadID,
	})
}

func (h s3Handler) listMultipartUploads(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	query := c.Request.URL.Query()
	maxUploads, err := parseMaxUploads(query.Get("max-uploads"))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	result, err := h.deps.Metadata.ListMultipartUploads(c.Request.Context(), meta.ListMultipartUploadsRequest{
		BucketID:       bucket.BucketID,
		Prefix:         query.Get("prefix"),
		Delimiter:      query.Get("delimiter"),
		KeyMarker:      query.Get("key-marker"),
		UploadIDMarker: query.Get("upload-id-marker"),
		MaxUploads:     maxUploads,
	})
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	uploads := make([]multipartUploadEntry, 0, len(result.Uploads))
	for _, upload := range result.Uploads {
		uploads = append(uploads, multipartUploadEntry{
			Key:          upload.Key,
			UploadID:     upload.UploadID,
			StorageClass: storageClassID(upload.StorageClass),
			Initiated:    upload.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	commonPrefixes := make([]commonPrefixEntry, 0, len(result.CommonPrefixes))
	for _, prefix := range result.CommonPrefixes {
		commonPrefixes = append(commonPrefixes, commonPrefixEntry{Prefix: prefix})
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, listMultipartUploadsResult{
		Bucket:             bucket.Name,
		Prefix:             query.Get("prefix"),
		Delimiter:          query.Get("delimiter"),
		KeyMarker:          query.Get("key-marker"),
		UploadIDMarker:     query.Get("upload-id-marker"),
		NextKeyMarker:      result.NextKeyMarker,
		NextUploadIDMarker: result.NextUploadIDMarker,
		MaxUploads:         maxUploads,
		IsTruncated:        result.IsTruncated,
		Uploads:            uploads,
		CommonPrefixes:     commonPrefixes,
	})
}

func (h s3Handler) uploadPart(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	partNumber, err := strconv.Atoi(req.Subresources[routing.SubresourcePartNumber])
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument("invalid partNumber"))
		return
	}
	uploadID := req.Subresources[routing.SubresourceUploadID]
	upload, err := h.deps.Metadata.GetMultipartUpload(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: uploadID,
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	if upload.State != model.MultipartUploadActive {
		writeS3Error(c, s3err.NoSuchUpload("multipart upload does not exist"))
		return
	}
	payloadReader, payloadSize, err := requestPayload(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidRequest(err.Error()))
		return
	}
	releaseBudget, ok := h.reserveDataBudget(c, payloadSize, requestPayloadSizeKnown(c.Request))
	if !ok {
		return
	}
	defer releaseBudget()
	md5Hash := md5.New()
	prepared, err := h.prepareSegment(c.Request.Context(), io.TeeReader(payloadReader, md5Hash), payloadSize, upload.ServerSideEncryption, segmentEncryptionContext(bucket.BucketID, req.Key))
	if err != nil {
		writeS3Error(c, s3err.AccessDenied(err.Error()))
		return
	}
	segmentRef, err := h.deps.Storage.PutSegment(c.Request.Context(), storage.PutSegmentRequest{
		Reader:       prepared.Reader,
		SizeBytes:    prepared.SizeBytes,
		StorageClass: upload.StorageClass,
	})
	if err != nil {
		writeS3Error(c, storageError(err))
		return
	}
	segmentRef.Encryption = prepared.Envelope
	etag := `"` + hex.EncodeToString(md5Hash.Sum(nil)) + `"`
	_, previous, err := h.deps.Metadata.PutMultipartPart(c.Request.Context(), meta.PutMultipartPartRequest{
		BucketID:   bucket.BucketID,
		Key:        req.Key,
		UploadID:   uploadID,
		PartNumber: partNumber,
		SizeBytes:  int64(segmentLogicalSize(segmentRef)),
		ETag:       etag,
		SegmentRef: segmentRef,
	})
	if err != nil {
		h.markOrphan(c, segmentRef, storage.DeleteReasonMultipartAborted)
		writeS3Error(c, uploadError(err))
		return
	}
	if previous != nil {
		if err := h.deps.Storage.DeleteSegment(c.Request.Context(), previous.SegmentRef, storage.DeleteReasonMultipartAborted); err != nil && !errors.Is(err, storage.ErrNotFound) {
			h.markOrphan(c, previous.SegmentRef, storage.DeleteReasonMultipartAborted)
		}
	}
	c.Header("ETag", etag)
	c.Status(http.StatusOK)
}

func (h s3Handler) uploadPartCopy(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	partNumber, err := strconv.Atoi(req.Subresources[routing.SubresourcePartNumber])
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument("invalid partNumber"))
		return
	}
	uploadID := req.Subresources[routing.SubresourceUploadID]
	upload, err := h.deps.Metadata.GetMultipartUpload(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: uploadID,
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	if upload.State != model.MultipartUploadActive {
		writeS3Error(c, s3err.NoSuchUpload("multipart upload does not exist"))
		return
	}
	sourceBucketName, sourceKey, err := parseCopySource(c.GetHeader("x-amz-copy-source"))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	sourceBucket, err := h.bucketByName(c, sourceBucketName)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	sourceHead, err := h.deps.Metadata.GetObjectHead(c.Request.Context(), sourceBucket.BucketID, sourceKey)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	copyRange, err := parseCopySourceRange(c.GetHeader("x-amz-copy-source-range"), uint64(sourceHead.SizeBytes))
	if err != nil {
		writeS3Error(c, s3err.InvalidRange(err.Error()))
		return
	}
	releaseBudget, ok := h.reserveDataBudget(c, copyDataBudgetBytes(copyRange.readLength), true)
	if !ok {
		return
	}
	defer releaseBudget()
	readers, err := h.openObjectReaders(c, sourceHead, copyRange.offset, copyRange.readLength)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	defer closeAll(readers)
	sourceReader := io.Reader(bytes.NewReader(nil))
	if len(readers) > 0 {
		sourceReader = io.MultiReader(readClosersAsReaders(readers)...)
	}
	md5Hash := md5.New()
	prepared, err := h.prepareSegment(c.Request.Context(), io.TeeReader(sourceReader, md5Hash), copyRange.readLength, upload.ServerSideEncryption, segmentEncryptionContext(bucket.BucketID, req.Key))
	if err != nil {
		writeS3Error(c, s3err.AccessDenied(err.Error()))
		return
	}
	segmentRef, err := h.deps.Storage.PutSegment(c.Request.Context(), storage.PutSegmentRequest{
		Reader:       prepared.Reader,
		SizeBytes:    prepared.SizeBytes,
		StorageClass: upload.StorageClass,
	})
	if err != nil {
		writeS3Error(c, storageError(err))
		return
	}
	segmentRef.Encryption = prepared.Envelope
	etag := `"` + hex.EncodeToString(md5Hash.Sum(nil)) + `"`
	_, previous, err := h.deps.Metadata.PutMultipartPart(c.Request.Context(), meta.PutMultipartPartRequest{
		BucketID:   bucket.BucketID,
		Key:        req.Key,
		UploadID:   uploadID,
		PartNumber: partNumber,
		SizeBytes:  int64(segmentLogicalSize(segmentRef)),
		ETag:       etag,
		SegmentRef: segmentRef,
	})
	if err != nil {
		h.markOrphan(c, segmentRef, storage.DeleteReasonMultipartAborted)
		writeS3Error(c, uploadError(err))
		return
	}
	if previous != nil {
		if err := h.deps.Storage.DeleteSegment(c.Request.Context(), previous.SegmentRef, storage.DeleteReasonMultipartAborted); err != nil && !errors.Is(err, storage.ErrNotFound) {
			h.markOrphan(c, previous.SegmentRef, storage.DeleteReasonMultipartAborted)
		}
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, uploadPartCopyResult{
		LastModified: time.Now().UTC().Format(time.RFC3339),
		ETag:         etag,
	})
}

func (h s3Handler) listParts(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	uploadID := req.Subresources[routing.SubresourceUploadID]
	upload, err := h.deps.Metadata.GetMultipartUpload(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: uploadID,
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	if upload.State != model.MultipartUploadActive {
		writeS3Error(c, s3err.NoSuchUpload("multipart upload does not exist"))
		return
	}
	parts, err := h.deps.Metadata.ListMultipartParts(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: uploadID,
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	entries := make([]partEntry, 0, len(parts))
	for _, part := range parts {
		entries = append(entries, partEntry{
			PartNumber:   part.PartNumber,
			LastModified: part.UpdatedAt.UTC().Format(time.RFC3339),
			ETag:         part.ETag,
			Size:         part.SizeBytes,
		})
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, listPartsResult{
		Bucket:   bucket.Name,
		Key:      req.Key,
		UploadID: upload.UploadID,
		Part:     entries,
	})
}

func (h s3Handler) completeMultipartUpload(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	uploadID := req.Subresources[routing.SubresourceUploadID]
	upload, err := h.deps.Metadata.GetMultipartUpload(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: uploadID,
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	switch upload.State {
	case model.MultipartUploadCompleted:
		h.cleanupMultipartUploadPartsBestEffort(c, meta.MultipartUploadRequest{
			BucketID: bucket.BucketID,
			Key:      req.Key,
			UploadID: uploadID,
		})
		h.writeCompleteMultipartResult(c, bucket.Name, req.Key, upload.CompletedETag)
		return
	case model.MultipartUploadActive:
	case model.MultipartUploadAborted:
		writeS3Error(c, s3err.NoSuchUpload("multipart upload does not exist"))
		return
	default:
		writeS3Error(c, s3err.NoSuchUpload("multipart upload does not exist"))
		return
	}
	if completion, err := h.deps.Metadata.GetMultipartCompletion(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: uploadID,
	}); err == nil {
		h.resumeMultipartCompletion(c, bucket, completion)
		return
	} else if !errors.Is(err, meta.ErrNotFound) {
		writeS3Error(c, uploadError(err))
		return
	}
	requested, err := parseCompleteMultipartUpload(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidRequest(err.Error()))
		return
	}
	parts, err := h.deps.Metadata.GetMultipartParts(c.Request.Context(), meta.GetMultipartPartsRequest{
		BucketID:    bucket.BucketID,
		Key:         req.Key,
		UploadID:    uploadID,
		PartNumbers: completePartNumbers(requested),
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	selected, err := selectCompleteParts(requested, parts)
	if err != nil {
		writeS3Error(c, s3err.InvalidPart(err.Error()))
		return
	}
	etag, err := multipartETag(selected)
	if err != nil {
		writeS3Error(c, s3err.InvalidRequest(err.Error()))
		return
	}
	segmentRefs, totalSize := partSegmentRefs(selected)
	if err := storageClassObjectSizeAdmission(upload.StorageClass, totalSize); err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if !h.bucketQuotaAllowsObjectWrite(c, bucket, totalSize) {
		return
	}
	if err := h.validateCompleteSegments(c.Request.Context(), segmentRefs); err != nil {
		writeS3Error(c, storageError(err))
		return
	}
	pending, err := h.deps.Metadata.BeginPutObject(c.Request.Context(), meta.BeginPutObjectRequest{
		BucketID:             bucket.BucketID,
		Key:                  req.Key,
		SizeBytes:            int64(totalSize),
		ETag:                 etag,
		ContentType:          upload.ContentType,
		StorageClass:         upload.StorageClass,
		ServerSideEncryption: upload.ServerSideEncryption,
		SegmentRefs:          segmentRefs,
		UserMetadata:         upload.UserMetadata,
		Tags:                 upload.Tags,
		ObjectLockRetention:  upload.ObjectLockRetention,
		ObjectLockLegalHold:  upload.ObjectLockLegalHold,
	})
	if err != nil {
		if errors.Is(err, meta.ErrObjectManifestTooLarge) {
			h.abortMultipartUploadAndMarkOrphans(c, meta.MultipartUploadRequest{
				BucketID: bucket.BucketID,
				Key:      req.Key,
				UploadID: uploadID,
			}, storage.DeleteReasonPublishFailed)
		}
		writeS3Error(c, objectError(err))
		return
	}
	completion, err := h.deps.Metadata.PrepareMultipartCompletion(c.Request.Context(), meta.PrepareMultipartCompletionRequest{
		BucketID:              bucket.BucketID,
		Key:                   req.Key,
		UploadID:              uploadID,
		ObjectVersionID:       pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
		ETag:                  etag,
		SizeBytes:             int64(totalSize),
		PartCount:             len(selected),
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	h.resumeMultipartCompletion(c, bucket, completion)
}

func (h s3Handler) abortMultipartUpload(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	parts, err := h.deps.Metadata.AbortMultipartUpload(c.Request.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: req.Subresources[routing.SubresourceUploadID],
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	for _, part := range parts {
		if err := h.deps.Storage.DeleteSegment(c.Request.Context(), part.SegmentRef, storage.DeleteReasonMultipartAborted); err != nil && !errors.Is(err, storage.ErrNotFound) {
			h.markOrphan(c, part.SegmentRef, storage.DeleteReasonMultipartAborted)
		}
	}
	h.cleanupMultipartUploadPartsBestEffort(c, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      req.Key,
		UploadID: req.Subresources[routing.SubresourceUploadID],
	})
	c.Status(http.StatusNoContent)
}

func (h s3Handler) writeCompleteMultipartResult(c *gin.Context, bucketName, key, etag string) {
	_ = xmlresp.Write(c.Writer, http.StatusOK, completeMultipartUploadResult{
		Location: "/" + bucketName + "/" + key,
		Bucket:   bucketName,
		Key:      key,
		ETag:     etag,
	})
}

func (h s3Handler) resumeMultipartCompletion(c *gin.Context, bucket model.Bucket, completion model.MultipartCompletionRecord) {
	ctx := c.Request.Context()
	stateReq := meta.MultipartCompletionStateRequest{
		BucketID: completion.BucketID,
		Key:      completion.Key,
		UploadID: completion.UploadID,
	}
	head, err := h.committedMultipartCompletionHead(ctx, completion)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	_, _ = h.deps.Metadata.MarkMultipartCompletionPublished(ctx, stateReq)
	completed, err := h.deps.Metadata.CompleteMultipartUpload(ctx, meta.CompleteMultipartUploadRequest{
		BucketID:        completion.BucketID,
		Key:             completion.Key,
		UploadID:        completion.UploadID,
		ObjectVersionID: completion.ObjectVersionID,
		ETag:            completion.ETag,
		SizeBytes:       completion.SizeBytes,
		PartCount:       completion.PartCount,
	})
	if err != nil {
		writeS3Error(c, uploadError(err))
		return
	}
	_, _ = h.deps.Metadata.MarkMultipartCompletionCompleted(ctx, stateReq)
	h.cleanupMultipartUploadPartsBestEffort(c, meta.MultipartUploadRequest{
		BucketID: completion.BucketID,
		Key:      completion.Key,
		UploadID: completion.UploadID,
	})
	c.Header("x-amz-version-id", head.VersionID)
	writeServerSideEncryptionHeaders(c, head.ServerSideEncryption)
	h.writeCompleteMultipartResult(c, bucket.Name, completion.Key, completed.CompletedETag)
}

func (h s3Handler) committedMultipartCompletionHead(ctx context.Context, completion model.MultipartCompletionRecord) (model.ObjectHead, error) {
	version, err := h.deps.Metadata.GetObjectVersion(ctx, completion.BucketID, completion.Key, completion.ObjectVersionID)
	if err == nil && version.State == model.ObjectVersionCommitted {
		return objectHeadFromVersion(version), nil
	}
	if err != nil && !errors.Is(err, meta.ErrNotFound) {
		return model.ObjectHead{}, err
	}
	head, commitErr := h.deps.Metadata.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              completion.BucketID,
		Key:                   completion.Key,
		VersionID:             completion.ObjectVersionID,
		ExpectedHeadVersionID: completion.ExpectedHeadVersionID,
	})
	if commitErr == nil {
		return head, nil
	}
	if !errors.Is(commitErr, meta.ErrCASConflict) {
		return model.ObjectHead{}, commitErr
	}
	current, headErr := h.deps.Metadata.GetObjectHead(ctx, completion.BucketID, completion.Key)
	if headErr != nil {
		return model.ObjectHead{}, commitErr
	}
	if current.VersionID != completion.ObjectVersionID {
		return model.ObjectHead{}, commitErr
	}
	return current, nil
}

func (h s3Handler) listBuckets(c *gin.Context) {
	principal, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		writeS3Error(c, s3err.AccessDenied("authenticated principal is required"))
		return
	}
	buckets, err := h.deps.Metadata.ListBuckets(c.Request.Context(), principal.TenantID)
	if err != nil {
		writeS3Error(c, metadataError(err))
		return
	}
	entries := make([]bucketEntry, 0, len(buckets))
	for _, bucket := range buckets {
		entries = append(entries, bucketEntry{
			Name:         bucket.Name,
			CreationDate: bucket.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, listBucketsResult{
		Owner: ownerEntry{
			ID:          principal.TenantID,
			DisplayName: principal.DisplayName,
		},
		Buckets: bucketsEntry{Buckets: entries},
	})
}

func (h s3Handler) createBucket(c *gin.Context, req routing.Request) {
	principal, ok := auth.PrincipalFromContext(c.Request.Context())
	if !ok {
		writeS3Error(c, s3err.AccessDenied("authenticated principal is required"))
		return
	}
	objectLockEnabled, err := bucketObjectLockEnabledForCreate(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	_, err = h.deps.Metadata.CreateBucket(c.Request.Context(), meta.CreateBucketRequest{
		TenantID:          principal.TenantID,
		Name:              req.Bucket,
		Region:            h.cfg.Region,
		ObjectLockEnabled: objectLockEnabled,
	})
	if err != nil {
		if errors.Is(err, meta.ErrAlreadyExists) {
			writeS3Error(c, s3err.BucketAlreadyOwnedByYou("bucket already exists"))
			return
		}
		writeS3Error(c, bucketError(err))
		return
	}
	c.Header("Location", "/"+req.Bucket)
	c.Status(http.StatusOK)
}

func (h s3Handler) headBucket(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Header("x-amz-bucket-region", bucket.Region)
	c.Status(http.StatusOK)
}

func (h s3Handler) deleteBucket(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if err := h.deps.Metadata.DeleteBucket(c.Request.Context(), bucket.BucketID); err != nil {
		switch {
		case errors.Is(err, meta.ErrBucketNotEmpty):
			writeS3Error(c, s3err.BucketNotEmpty("bucket is not empty"))
		default:
			writeS3Error(c, bucketError(err))
		}
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) getBucketLocation(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, locationConstraint{
		Value: bucket.Region,
	})
}

func (h s3Handler) getBucketVersioning(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, bucketVersioningConfiguration{
		Status: string(bucket.VersioningState),
	})
}

func (h s3Handler) putBucketVersioning(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	state, err := parseBucketVersioning(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if _, err := h.deps.Metadata.PutBucketVersioning(c.Request.Context(), meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    state,
	}); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) getBucketCORS(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	rules, err := h.deps.Metadata.GetBucketCORS(c.Request.Context(), bucket.BucketID)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if len(rules) == 0 {
		writeS3Error(c, s3err.NoSuchCORSConfiguration("bucket CORS configuration does not exist"))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, corsConfigurationFromModel(rules))
}

func (h s3Handler) putBucketCORS(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	rules, err := parseCORSConfiguration(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if _, err := h.deps.Metadata.PutBucketCORS(c.Request.Context(), meta.BucketCORSRequest{
		BucketID: bucket.BucketID,
		Rules:    rules,
	}); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) deleteBucketCORS(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if _, err := h.deps.Metadata.DeleteBucketCORS(c.Request.Context(), bucket.BucketID); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) getBucketLifecycle(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	configuration, err := h.deps.Metadata.GetBucketLifecycle(c.Request.Context(), bucket.BucketID)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			writeS3Error(c, s3err.NoSuchLifecycleConfiguration("bucket lifecycle configuration does not exist"))
			return
		}
		writeS3Error(c, bucketError(err))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, lifecycleConfigurationFromModel(configuration))
}

func (h s3Handler) putBucketLifecycle(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	configuration, err := parseLifecycleConfiguration(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if _, err := h.deps.Metadata.PutBucketLifecycle(c.Request.Context(), meta.BucketLifecycleRequest{
		BucketID:      bucket.BucketID,
		Configuration: configuration,
		Audit:         requestAuditContext(c, "PutBucketLifecycle"),
	}); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) deleteBucketLifecycle(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if _, err := h.deps.Metadata.DeleteBucketLifecycle(c.Request.Context(), bucket.BucketID, requestAuditContext(c, "DeleteBucketLifecycle")); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) getBucketEncryption(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	encryption, err := h.deps.Metadata.GetBucketEncryption(c.Request.Context(), bucket.BucketID)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			writeS3Error(c, s3err.ServerSideEncryptionConfigurationNotFound("bucket encryption configuration does not exist"))
			return
		}
		writeS3Error(c, bucketError(err))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, bucketEncryptionConfigurationFromModel(encryption))
}

func (h s3Handler) putBucketEncryption(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	encryption, err := parseBucketEncryptionConfiguration(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if encryption.Algorithm == model.ServerSideEncryptionAWSKMS {
		if err := edition.Require(h.cfg.Edition, edition.FeatureSSEKMS); err != nil {
			writeS3Error(c, s3err.NotImplemented(err.Error()))
			return
		}
	}
	if _, err := h.deps.Metadata.PutBucketEncryption(c.Request.Context(), meta.BucketEncryptionRequest{
		BucketID:   bucket.BucketID,
		Encryption: encryption,
		Audit:      requestAuditContext(c, "PutBucketEncryption"),
	}); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) deleteBucketEncryption(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if _, err := h.deps.Metadata.DeleteBucketEncryption(c.Request.Context(), bucket.BucketID, requestAuditContext(c, "DeleteBucketEncryption")); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) getBucketPolicy(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	policy, err := h.deps.Metadata.GetBucketPolicy(c.Request.Context(), bucket.BucketID)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			writeS3Error(c, s3err.NoSuchBucketPolicy("bucket policy does not exist"))
			return
		}
		writeS3Error(c, bucketError(err))
		return
	}
	writeJSON(c, http.StatusOK, policy)
}

func (h s3Handler) putBucketPolicy(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	policy, err := auth.ParsePolicyDocument(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if _, err := h.deps.Metadata.PutBucketPolicy(c.Request.Context(), meta.BucketPolicyRequest{
		BucketID: bucket.BucketID,
		Policy:   policy,
		Audit:    requestAuditContext(c, "PutBucketPolicy"),
	}); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) deleteBucketPolicy(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if _, err := h.deps.Metadata.DeleteBucketPolicy(c.Request.Context(), bucket.BucketID, requestAuditContext(c, "DeleteBucketPolicy")); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) getBucketObjectLock(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	configuration, err := h.deps.Metadata.GetBucketObjectLock(c.Request.Context(), bucket.BucketID)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if !configuration.Enabled {
		writeS3Error(c, s3err.ObjectLockConfigurationNotFound("bucket object lock configuration does not exist"))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, bucketObjectLockConfigurationFromModel(configuration))
}

func (h s3Handler) putBucketObjectLock(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	configuration, err := parseBucketObjectLockConfiguration(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if _, err := h.deps.Metadata.PutBucketObjectLock(c.Request.Context(), meta.BucketObjectLockRequest{
		BucketID:      bucket.BucketID,
		Configuration: configuration,
		Audit:         requestAuditContext(c, "PutBucketObjectLock"),
	}); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) getBucketACL(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	h.writeACL(c, bucket.TenantID)
}

func (h s3Handler) putBucketACL(c *gin.Context, req routing.Request) {
	if _, err := h.bucketByName(c, req.Bucket); err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) listObjectVersions(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	query := c.Request.URL.Query()
	maxKeys, err := parseMaxKeys(query.Get("max-keys"))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	result, err := h.deps.Metadata.ListObjectVersions(c.Request.Context(), meta.ListObjectVersionsRequest{
		BucketID:        bucket.BucketID,
		Prefix:          query.Get("prefix"),
		Delimiter:       query.Get("delimiter"),
		KeyMarker:       query.Get("key-marker"),
		VersionIDMarker: query.Get("version-id-marker"),
		MaxKeys:         maxKeys,
	})
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	versions := make([]objectVersionEntry, 0, len(result.Versions))
	for _, entry := range result.Versions {
		version := entry.Version
		versions = append(versions, objectVersionEntry{
			Key:          version.Key,
			VersionID:    version.VersionID,
			IsLatest:     entry.IsLatest,
			LastModified: versionTimestamp(version).UTC().Format(time.RFC3339),
			ETag:         version.ETag,
			Size:         version.SizeBytes,
			StorageClass: storageClassID(version.StorageClass),
		})
	}
	deleteMarkers := make([]deleteMarkerEntry, 0, len(result.DeleteMarkers))
	for _, entry := range result.DeleteMarkers {
		version := entry.Version
		deleteMarkers = append(deleteMarkers, deleteMarkerEntry{
			Key:          version.Key,
			VersionID:    version.VersionID,
			IsLatest:     entry.IsLatest,
			LastModified: versionTimestamp(version).UTC().Format(time.RFC3339),
		})
	}
	commonPrefixes := make([]commonPrefixEntry, 0, len(result.CommonPrefixes))
	for _, prefix := range result.CommonPrefixes {
		commonPrefixes = append(commonPrefixes, commonPrefixEntry{Prefix: prefix})
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, listVersionsResult{
		Name:                bucket.Name,
		Prefix:              query.Get("prefix"),
		KeyMarker:           query.Get("key-marker"),
		VersionIDMarker:     query.Get("version-id-marker"),
		NextKeyMarker:       result.NextKeyMarker,
		NextVersionIDMarker: result.NextVersionIDMarker,
		Delimiter:           query.Get("delimiter"),
		MaxKeys:             maxKeys,
		IsTruncated:         result.IsTruncated,
		Versions:            versions,
		DeleteMarkers:       deleteMarkers,
		CommonPrefixes:      commonPrefixes,
	})
}

func (h s3Handler) listObjectsV2(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	query := c.Request.URL.Query()
	maxKeys, err := parseMaxKeys(query.Get("max-keys"))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	result, err := h.deps.Metadata.ListObjects(c.Request.Context(), meta.ListObjectsRequest{
		BucketID:          bucket.BucketID,
		Prefix:            query.Get("prefix"),
		Delimiter:         query.Get("delimiter"),
		ContinuationToken: query.Get("continuation-token"),
		MaxKeys:           maxKeys,
	})
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if !h.recordAccessAudit(c, model.AuditActionListObjects, bucket.BucketID, "", "", map[string]string{
		"api":          "ListObjectsV2",
		"prefix":       query.Get("prefix"),
		"delimiter":    query.Get("delimiter"),
		"max_keys":     strconv.Itoa(maxKeys),
		"key_count":    strconv.Itoa(len(result.Contents) + len(result.CommonPrefixes)),
		"is_truncated": strconv.FormatBool(result.IsTruncated),
	}) {
		return
	}
	contents := make([]objectEntry, 0, len(result.Contents))
	for _, head := range result.Contents {
		contents = append(contents, objectEntry{
			Key:          head.Key,
			LastModified: head.LastModified.UTC().Format(time.RFC3339),
			ETag:         head.ETag,
			Size:         head.SizeBytes,
			StorageClass: storageClassID(head.StorageClass),
		})
	}
	commonPrefixes := make([]commonPrefixEntry, 0, len(result.CommonPrefixes))
	for _, prefix := range result.CommonPrefixes {
		commonPrefixes = append(commonPrefixes, commonPrefixEntry{Prefix: prefix})
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, listBucketResult{
		Name:                  bucket.Name,
		Prefix:                query.Get("prefix"),
		Delimiter:             query.Get("delimiter"),
		MaxKeys:               maxKeys,
		KeyCount:              len(contents) + len(commonPrefixes),
		IsTruncated:           result.IsTruncated,
		NextContinuationToken: result.NextContinuationToken,
		Contents:              contents,
		CommonPrefixes:        commonPrefixes,
	})
}

func (h s3Handler) listObjectsV1(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	query := c.Request.URL.Query()
	maxKeys, err := parseMaxKeys(query.Get("max-keys"))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	result, err := h.deps.Metadata.ListObjects(c.Request.Context(), meta.ListObjectsRequest{
		BucketID:          bucket.BucketID,
		Prefix:            query.Get("prefix"),
		Delimiter:         query.Get("delimiter"),
		ContinuationToken: query.Get("marker"),
		MaxKeys:           maxKeys,
	})
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if !h.recordAccessAudit(c, model.AuditActionListObjects, bucket.BucketID, "", "", map[string]string{
		"api":          "ListObjects",
		"prefix":       query.Get("prefix"),
		"delimiter":    query.Get("delimiter"),
		"max_keys":     strconv.Itoa(maxKeys),
		"key_count":    strconv.Itoa(len(result.Contents) + len(result.CommonPrefixes)),
		"is_truncated": strconv.FormatBool(result.IsTruncated),
	}) {
		return
	}
	contents := make([]objectEntry, 0, len(result.Contents))
	for _, head := range result.Contents {
		contents = append(contents, objectEntry{
			Key:          head.Key,
			LastModified: head.LastModified.UTC().Format(time.RFC3339),
			ETag:         head.ETag,
			Size:         head.SizeBytes,
			StorageClass: storageClassID(head.StorageClass),
		})
	}
	commonPrefixes := make([]commonPrefixEntry, 0, len(result.CommonPrefixes))
	for _, prefix := range result.CommonPrefixes {
		commonPrefixes = append(commonPrefixes, commonPrefixEntry{Prefix: prefix})
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, listBucketV1Result{
		Name:           bucket.Name,
		Prefix:         query.Get("prefix"),
		Marker:         query.Get("marker"),
		NextMarker:     result.NextContinuationToken,
		Delimiter:      query.Get("delimiter"),
		MaxKeys:        maxKeys,
		IsTruncated:    result.IsTruncated,
		Contents:       contents,
		CommonPrefixes: commonPrefixes,
	})
}

func (h s3Handler) putObject(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	if c.GetHeader("If-None-Match") == "*" {
		_, oldHeadErr := h.deps.Metadata.GetObjectHead(c.Request.Context(), bucket.BucketID, req.Key)
		if oldHeadErr == nil {
			writeS3Error(c, s3err.PreconditionFailed("object already exists"))
			return
		} else if !errors.Is(oldHeadErr, meta.ErrNotFound) {
			writeS3Error(c, objectError(oldHeadErr))
			return
		}
	}
	tags, err := objectTagsFromHeader(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	objectLockRetention, objectLockLegalHold, err := objectLockStateFromRequest(c.Request, bucket)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	md5Hash := md5.New()
	payloadReader, payloadSize, err := requestPayload(c.Request)
	if err != nil {
		writeS3Error(c, s3err.InvalidRequest(err.Error()))
		return
	}
	releaseBudget, ok := h.reserveDataBudget(c, payloadSize, requestPayloadSizeKnown(c.Request))
	if !ok {
		return
	}
	defer releaseBudget()
	storageClass, err := storageClassFromRequestWithSize(c.Request, standardStorageClass(), payloadSize, requestPayloadSizeKnown(c.Request))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if !h.bucketQuotaAllowsObjectWrite(c, bucket, payloadSize) {
		return
	}
	serverSideEncryption, err := serverSideEncryptionFromRequest(c.Request, bucket.DefaultEncryption)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	prepared, err := h.prepareSegment(c.Request.Context(), io.TeeReader(payloadReader, md5Hash), payloadSize, serverSideEncryption, segmentEncryptionContext(bucket.BucketID, req.Key))
	if err != nil {
		writeS3Error(c, s3err.AccessDenied(err.Error()))
		return
	}
	segmentRef, err := h.deps.Storage.PutSegment(c.Request.Context(), storage.PutSegmentRequest{
		Reader:       prepared.Reader,
		SizeBytes:    prepared.SizeBytes,
		StorageClass: storageClass,
	})
	if err != nil {
		writeS3Error(c, storageError(err))
		return
	}
	segmentRef.Encryption = prepared.Envelope
	logicalSize := segmentLogicalSize(segmentRef)
	etag := `"` + hex.EncodeToString(md5Hash.Sum(nil)) + `"`
	published, err := h.deps.Metadata.PutObjectVersion(c.Request.Context(), meta.PutObjectVersionRequest{
		BucketID:             bucket.BucketID,
		Key:                  req.Key,
		SizeBytes:            int64(logicalSize),
		ETag:                 etag,
		ContentType:          contentType(c.Request),
		StorageClass:         storageClass,
		ServerSideEncryption: prepared.Encryption,
		SegmentRef:           segmentRef,
		UserMetadata:         userMetadata(c.Request),
		Tags:                 tags,
		ObjectLockRetention:  objectLockRetention,
		ObjectLockLegalHold:  objectLockLegalHold,
	})
	if err != nil {
		h.markOrphan(c, segmentRef, storage.DeleteReasonPublishFailed)
		writeS3Error(c, objectError(err))
		return
	}
	head := published.Head
	if bucket.VersioningState != model.BucketVersioningEnabled && published.ReplacedHeadFound {
		h.deleteObjectSegments(c, published.ReplacedHead, storage.DeleteReasonObjectOverwritten)
	}
	c.Header("ETag", etag)
	c.Header("x-amz-version-id", head.VersionID)
	c.Header("x-amz-storage-class", storageClassID(head.StorageClass))
	writeServerSideEncryptionHeaders(c, head.ServerSideEncryption)
	c.Status(http.StatusOK)
}

func (h s3Handler) copyObject(c *gin.Context, req routing.Request) {
	destBucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	sourceBucketName, sourceKey, err := parseCopySource(c.GetHeader("x-amz-copy-source"))
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	sourceBucket, err := h.bucketByName(c, sourceBucketName)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	sourceHead, err := h.deps.Metadata.GetObjectHead(c.Request.Context(), sourceBucket.BucketID, sourceKey)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	releaseBudget, ok := h.reserveDataBudget(c, copyDataBudgetBytes(uint64(sourceHead.SizeBytes)), true)
	if !ok {
		return
	}
	defer releaseBudget()
	readers, err := h.openObjectReaders(c, sourceHead, 0, uint64(sourceHead.SizeBytes))
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	defer closeAll(readers)
	sourceReader := io.Reader(bytes.NewReader(nil))
	if len(readers) > 0 {
		sourceReader = io.MultiReader(readClosersAsReaders(readers)...)
	}
	md5Hash := md5.New()
	storageClass := sourceHead.StorageClass
	if storageClass.StorageClassID == "" {
		storageClass = standardStorageClass()
	}
	storageClass, err = storageClassFromRequestWithSize(c.Request, storageClass, uint64(sourceHead.SizeBytes), true)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if !h.bucketQuotaAllowsObjectWrite(c, destBucket, uint64(sourceHead.SizeBytes)) {
		return
	}
	encryptionFallback := sourceHead.ServerSideEncryption
	if destBucket.DefaultEncryption.Algorithm != "" {
		encryptionFallback = destBucket.DefaultEncryption
	}
	serverSideEncryption, err := serverSideEncryptionFromRequest(c.Request, encryptionFallback)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	prepared, err := h.prepareSegment(c.Request.Context(), io.TeeReader(sourceReader, md5Hash), uint64(sourceHead.SizeBytes), serverSideEncryption, segmentEncryptionContext(destBucket.BucketID, req.Key))
	if err != nil {
		writeS3Error(c, s3err.AccessDenied(err.Error()))
		return
	}
	segmentRef, err := h.deps.Storage.PutSegment(c.Request.Context(), storage.PutSegmentRequest{
		Reader:       prepared.Reader,
		SizeBytes:    prepared.SizeBytes,
		StorageClass: storageClass,
	})
	if err != nil {
		writeS3Error(c, storageError(err))
		return
	}
	segmentRef.Encryption = prepared.Envelope
	contentTypeValue := sourceHead.ContentType
	metadata := sourceHead.UserMetadata
	tags := sourceHead.Tags
	if strings.EqualFold(c.GetHeader("x-amz-metadata-directive"), "REPLACE") {
		contentTypeValue = contentType(c.Request)
		metadata = userMetadata(c.Request)
	}
	tagDirective := strings.ToUpper(strings.TrimSpace(c.GetHeader("x-amz-tagging-directive")))
	switch {
	case tagDirective == "", tagDirective == "COPY":
		if c.GetHeader("x-amz-tagging") != "" {
			tags, err = objectTagsFromHeader(c.Request)
			if err != nil {
				writeS3Error(c, s3err.InvalidArgument(err.Error()))
				return
			}
		}
	case tagDirective == "REPLACE":
		tags, err = objectTagsFromHeader(c.Request)
		if err != nil {
			writeS3Error(c, s3err.InvalidArgument(err.Error()))
			return
		}
	default:
		writeS3Error(c, s3err.InvalidArgument("x-amz-tagging-directive must be COPY or REPLACE"))
		return
	}
	objectLockRetention, objectLockLegalHold, err := objectLockStateFromRequest(c.Request, destBucket)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	etag := `"` + hex.EncodeToString(md5Hash.Sum(nil)) + `"`
	published, err := h.deps.Metadata.PutObjectVersion(c.Request.Context(), meta.PutObjectVersionRequest{
		BucketID:             destBucket.BucketID,
		Key:                  req.Key,
		SizeBytes:            int64(segmentLogicalSize(segmentRef)),
		ETag:                 etag,
		ContentType:          contentTypeValue,
		StorageClass:         storageClass,
		ServerSideEncryption: prepared.Encryption,
		SegmentRef:           segmentRef,
		UserMetadata:         metadata,
		Tags:                 tags,
		ObjectLockRetention:  objectLockRetention,
		ObjectLockLegalHold:  objectLockLegalHold,
	})
	if err != nil {
		h.markOrphan(c, segmentRef, storage.DeleteReasonPublishFailed)
		writeS3Error(c, objectError(err))
		return
	}
	head := published.Head
	if destBucket.VersioningState != model.BucketVersioningEnabled && published.ReplacedHeadFound {
		h.deleteObjectSegments(c, published.ReplacedHead, storage.DeleteReasonObjectOverwritten)
	}
	c.Header("x-amz-version-id", head.VersionID)
	c.Header("x-amz-storage-class", storageClassID(head.StorageClass))
	writeServerSideEncryptionHeaders(c, head.ServerSideEncryption)
	_ = xmlresp.Write(c.Writer, http.StatusOK, copyObjectResult{
		ETag:         head.ETag,
		LastModified: head.LastModified.UTC().Format(time.RFC3339),
	})
}

func (h s3Handler) headObject(c *gin.Context, req routing.Request) {
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	if !h.recordAccessAudit(c, model.AuditActionHeadObject, head.BucketID, head.Key, head.VersionID, map[string]string{
		"size_bytes":    strconv.FormatInt(head.SizeBytes, 10),
		"storage_class": storageClassID(head.StorageClass),
	}) {
		return
	}
	writeObjectHeaders(c, head)
	c.Status(http.StatusOK)
}

func (h s3Handler) getObject(c *gin.Context, req routing.Request) {
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	if err := h.checkDecryptAdmission(c.Request.Context(), head.ServerSideEncryption); err != nil {
		writeS3Error(c, s3err.AccessDenied(err.Error()))
		return
	}
	rangeSpec, err := parseRange(c.GetHeader("Range"), uint64(head.SizeBytes))
	if err != nil {
		writeS3Error(c, s3err.InvalidRange(err.Error()))
		return
	}
	releaseBudget, ok := h.reserveDataBudget(c, rangeSpec.contentLength, true)
	if !ok {
		return
	}
	defer releaseBudget()
	readers, err := h.openObjectReaders(c, head, rangeSpec.offset, rangeSpec.contentLength)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	defer closeAll(readers)
	if !h.recordAccessAudit(c, model.AuditActionGetObject, head.BucketID, head.Key, head.VersionID, map[string]string{
		"range":          c.GetHeader("Range"),
		"range_status":   strconv.Itoa(rangeSpec.status),
		"content_length": strconv.FormatUint(rangeSpec.contentLength, 10),
		"storage_class":  storageClassID(head.StorageClass),
	}) {
		return
	}
	writeObjectHeaders(c, head)
	if rangeSpec.contentRange != "" {
		c.Header("Content-Range", rangeSpec.contentRange)
	}
	c.Header("Content-Length", strconv.FormatUint(rangeSpec.contentLength, 10))
	c.Writer.WriteHeader(rangeSpec.status)
	_, _ = io.Copy(h.downloadWriter(c.Writer), io.MultiReader(readClosersAsReaders(readers)...))
}

func (h s3Handler) deleteObject(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	bypassGovernance, bypassAudit := h.governanceBypass(c, bucket, req.Key, "DeleteObject")
	result, err := h.deps.Metadata.DeleteObject(c.Request.Context(), meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       req.Key,
		VersionID:                 req.Subresources[routing.SubresourceVersionID],
		BypassGovernanceRetention: bypassGovernance,
		BypassAudit:               bypassAudit,
	})
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if result.DeletedVersionID != "" {
		c.Header("x-amz-version-id", result.DeletedVersionID)
	}
	if result.DeleteMarker {
		c.Header("x-amz-delete-marker", "true")
	}
	if result.Deleted && !result.DeleteMarker {
		h.deleteObjectVersionSegments(c, result.DeletedVersion, storage.DeleteReasonObjectOverwritten)
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) deleteObjects(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	payload, err := parseDeleteObjectsRequest(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	result := deleteObjectsResult{
		Deleted: make([]deletedObjectEntry, 0, len(payload.Objects)),
	}
	for _, object := range payload.Objects {
		entry := h.deleteObjectForMultiDelete(c, bucket, object)
		if entry.Error.Code != "" {
			result.Errors = append(result.Errors, entry.Error)
			continue
		}
		if !payload.Quiet {
			result.Deleted = append(result.Deleted, entry.Deleted)
		}
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, result)
}

func (h s3Handler) deleteObjectForMultiDelete(c *gin.Context, bucket model.Bucket, object deleteObjectIdentifier) deleteObjectResultEntry {
	bypassGovernance, bypassAudit := h.governanceBypass(c, bucket, object.Key, "DeleteObjects")
	result, err := h.deps.Metadata.DeleteObject(c.Request.Context(), meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       object.Key,
		VersionID:                 object.VersionID,
		BypassGovernanceRetention: bypassGovernance,
		BypassAudit:               bypassAudit,
	})
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			return deleteObjectResultEntry{
				Deleted: deletedObjectEntry{
					Key:       object.Key,
					VersionID: object.VersionID,
				},
			}
		}
		s3Err := objectError(err)
		return deleteObjectResultEntry{
			Error: deleteErrorEntry{
				Key:       object.Key,
				VersionID: object.VersionID,
				Code:      s3Err.Code,
				Message:   s3Err.Message,
			},
		}
	}
	if result.Deleted && !result.DeleteMarker {
		h.deleteObjectVersionSegments(c, result.DeletedVersion, storage.DeleteReasonObjectOverwritten)
	}
	deleted := deletedObjectEntry{
		Key:       object.Key,
		VersionID: result.DeletedVersionID,
	}
	if deleted.VersionID == "" {
		deleted.VersionID = object.VersionID
	}
	if result.DeleteMarker {
		deleted.DeleteMarker = true
		deleted.DeleteMarkerVersionID = result.DeletedVersionID
	}
	return deleteObjectResultEntry{Deleted: deleted}
}

func (h s3Handler) getObjectTagging(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	tags, err := h.deps.Metadata.GetObjectTags(c.Request.Context(), meta.ObjectTagsRequest{
		BucketID:  bucket.BucketID,
		Key:       req.Key,
		VersionID: req.Subresources[routing.SubresourceVersionID],
	})
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, objectTaggingResult{
		TagSet: objectTagSet{
			Tags: tagEntries(tags),
		},
	})
}

func (h s3Handler) putObjectTagging(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	tags, err := parseObjectTagging(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	if err := h.deps.Metadata.PutObjectTags(c.Request.Context(), meta.ObjectTagsRequest{
		BucketID:  bucket.BucketID,
		Key:       req.Key,
		VersionID: req.Subresources[routing.SubresourceVersionID],
		Tags:      tags,
	}); err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) deleteObjectTagging(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	if err := h.deps.Metadata.DeleteObjectTags(c.Request.Context(), meta.ObjectTagsRequest{
		BucketID:  bucket.BucketID,
		Key:       req.Key,
		VersionID: req.Subresources[routing.SubresourceVersionID],
	}); err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	c.Status(http.StatusNoContent)
}

func (h s3Handler) getObjectRetention(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	retention, err := h.deps.Metadata.GetObjectRetention(c.Request.Context(), meta.ObjectRetentionRequest{
		BucketID:  bucket.BucketID,
		Key:       req.Key,
		VersionID: req.Subresources[routing.SubresourceVersionID],
	})
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, objectRetentionFromModel(retention))
}

func (h s3Handler) putObjectRetention(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	retention, err := parseObjectRetention(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	bypassGovernance, bypassAudit := h.governanceBypass(c, bucket, req.Key, "PutObjectRetention")
	if err := h.deps.Metadata.PutObjectRetention(c.Request.Context(), meta.ObjectRetentionRequest{
		BucketID:                  bucket.BucketID,
		Key:                       req.Key,
		VersionID:                 req.Subresources[routing.SubresourceVersionID],
		Retention:                 retention,
		BypassGovernanceRetention: bypassGovernance,
		BypassAudit:               bypassAudit,
		Audit:                     requestAuditContext(c, "PutObjectRetention"),
	}); err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) getObjectLegalHold(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	status, err := h.deps.Metadata.GetObjectLegalHold(c.Request.Context(), meta.ObjectLegalHoldRequest{
		BucketID:  bucket.BucketID,
		Key:       req.Key,
		VersionID: req.Subresources[routing.SubresourceVersionID],
	})
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if status == "" {
		status = model.ObjectLockLegalHoldOff
	}
	_ = xmlresp.Write(c.Writer, http.StatusOK, objectLegalHoldResult{Status: string(status)})
}

func (h s3Handler) putObjectLegalHold(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	status, err := parseObjectLegalHold(c.Request.Body)
	if err != nil {
		writeS3Error(c, s3err.InvalidArgument(err.Error()))
		return
	}
	if err := h.deps.Metadata.PutObjectLegalHold(c.Request.Context(), meta.ObjectLegalHoldRequest{
		BucketID:  bucket.BucketID,
		Key:       req.Key,
		VersionID: req.Subresources[routing.SubresourceVersionID],
		LegalHold: status,
		Audit:     requestAuditContext(c, "PutObjectLegalHold"),
	}); err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) getObjectACL(c *gin.Context, req routing.Request) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		writeS3Error(c, bucketError(err))
		return
	}
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	h.writeACL(c, bucket.TenantID)
}

func (h s3Handler) putObjectACL(c *gin.Context, req routing.Request) {
	head, err := h.objectHead(c, req)
	if err != nil {
		writeS3Error(c, objectError(err))
		return
	}
	if head.DeleteMarker {
		h.writeDeleteMarkerError(c, head)
		return
	}
	c.Status(http.StatusOK)
}

func (h s3Handler) writeACL(c *gin.Context, ownerID string) {
	_ = xmlresp.Write(c.Writer, http.StatusOK, accessControlPolicy{
		Owner: ownerEntry{
			ID:          ownerID,
			DisplayName: ownerID,
		},
		AccessControlList: accessControlList{
			Grants: []grantEntry{
				{
					Grantee: granteeEntry{
						XMLNSXSI:    "http://www.w3.org/2001/XMLSchema-instance",
						Type:        "CanonicalUser",
						ID:          ownerID,
						DisplayName: ownerID,
					},
					Permission: "FULL_CONTROL",
				},
			},
		},
	})
}

func writeJSON(c *gin.Context, status int, value any) {
	c.Header("Content-Type", "application/json")
	c.Writer.WriteHeader(status)
	_ = json.NewEncoder(c.Writer).Encode(value)
}

func (h s3Handler) deleteObjectSegments(c *gin.Context, head model.ObjectHead, reason storage.DeleteReason) {
	for _, ref := range objectSegmentRefs(head) {
		if h.segmentDeleteBlockedByProtectedRef(c, ref) {
			continue
		}
		if err := h.deps.Storage.DeleteSegment(c.Request.Context(), ref, reason); err != nil && !errors.Is(err, storage.ErrNotFound) {
			h.markOrphan(c, ref, reason)
		}
	}
}

func (h s3Handler) deleteObjectVersionSegments(c *gin.Context, version model.ObjectVersion, reason storage.DeleteReason) {
	h.deleteObjectSegments(c, objectHeadFromVersion(version), reason)
}

func (h s3Handler) bucketByName(c *gin.Context, name string) (model.Bucket, error) {
	return h.deps.Metadata.GetBucketByName(c.Request.Context(), name)
}

func (h s3Handler) objectHead(c *gin.Context, req routing.Request) (model.ObjectHead, error) {
	bucket, err := h.bucketByName(c, req.Bucket)
	if err != nil {
		return model.ObjectHead{}, err
	}
	if versionID := req.Subresources[routing.SubresourceVersionID]; versionID != "" {
		version, err := h.deps.Metadata.GetObjectVersion(c.Request.Context(), bucket.BucketID, req.Key, versionID)
		if err != nil {
			return model.ObjectHead{}, err
		}
		return objectHeadFromVersion(version), nil
	}
	return h.deps.Metadata.GetObjectHead(c.Request.Context(), bucket.BucketID, req.Key)
}

func (h s3Handler) writeDeleteMarkerError(c *gin.Context, head model.ObjectHead) {
	writeObjectHeaders(c, head)
	writeS3Error(c, s3err.NoSuchKey("object is a delete marker"))
}

func (h s3Handler) markOrphan(c *gin.Context, ref storage.SegmentRef, reason storage.DeleteReason) {
	if h.deps.Orphans == nil || ref.SegmentID == "" {
		return
	}
	_ = h.deps.Orphans.MarkOrphan(c.Request.Context(), ref, reason)
}

func (h s3Handler) abortMultipartUploadAndMarkOrphans(c *gin.Context, req meta.MultipartUploadRequest, reason storage.DeleteReason) {
	parts, err := h.deps.Metadata.AbortMultipartUpload(c.Request.Context(), req)
	if err != nil {
		return
	}
	for _, part := range parts {
		h.markOrphan(c, part.SegmentRef, reason)
	}
	h.cleanupMultipartUploadPartsBestEffort(c, req)
}

func (h s3Handler) cleanupMultipartUploadPartsBestEffort(c *gin.Context, req meta.MultipartUploadRequest) {
	_, _ = h.deps.Metadata.CleanupMultipartUploadParts(c.Request.Context(), meta.CleanupMultipartUploadPartsRequest{
		BucketID: req.BucketID,
		Key:      req.Key,
		UploadID: req.UploadID,
		Limit:    meta.DefaultMultipartCleanupLimit,
	})
}

func bucketError(err error) s3err.Error {
	switch {
	case isMetadataUnavailable(err):
		return s3err.ServiceUnavailable("metadata backend is unavailable")
	case errors.Is(err, meta.ErrNotFound):
		return s3err.NoSuchBucket("bucket does not exist")
	case errors.Is(err, meta.ErrInvalidArgument):
		return s3err.InvalidArgument(err.Error())
	default:
		return s3err.InvalidRequest(err.Error())
	}
}

func objectError(err error) s3err.Error {
	switch {
	case isMetadataUnavailable(err):
		return s3err.ServiceUnavailable("metadata backend is unavailable")
	case errors.Is(err, meta.ErrObjectLocked):
		return s3err.AccessDenied("object is protected by object lock")
	case errors.Is(err, meta.ErrKMSKeyUnavailable):
		return s3err.AccessDenied(err.Error())
	case errors.Is(err, meta.ErrQuotaExceeded):
		return s3err.AccessDenied(err.Error())
	case errors.Is(err, meta.ErrNotFound):
		return s3err.NoSuchKey("object does not exist")
	case errors.Is(err, storage.ErrNotFound), errors.Is(err, storage.ErrInvalidRange), errors.Is(err, storage.ErrInvalidArgument), errors.Is(err, storage.ErrUnavailable):
		return storageError(err)
	case errors.Is(err, meta.ErrInvalidArgument):
		return s3err.InvalidArgument(err.Error())
	default:
		return s3err.InvalidRequest(err.Error())
	}
}

func storageError(err error) s3err.Error {
	switch {
	case errors.Is(err, storage.ErrNotFound):
		return s3err.NoSuchKey("object segment does not exist")
	case errors.Is(err, storage.ErrInvalidRange):
		return s3err.InvalidRange(err.Error())
	case errors.Is(err, storage.ErrInvalidArgument):
		return s3err.InvalidArgument(err.Error())
	case errors.Is(err, storage.ErrUnavailable):
		return s3err.ServiceUnavailable(err.Error())
	default:
		return s3err.InvalidRequest(err.Error())
	}
}

func uploadError(err error) s3err.Error {
	switch {
	case isMetadataUnavailable(err):
		return s3err.ServiceUnavailable("metadata backend is unavailable")
	case errors.Is(err, meta.ErrNotFound):
		return s3err.NoSuchUpload("multipart upload does not exist")
	case errors.Is(err, meta.ErrQuotaExceeded):
		return s3err.AccessDenied(err.Error())
	case errors.Is(err, meta.ErrInvalidArgument):
		return s3err.InvalidArgument(err.Error())
	default:
		return s3err.InvalidRequest(err.Error())
	}
}

func metadataError(err error) s3err.Error {
	if isMetadataUnavailable(err) {
		return s3err.ServiceUnavailable("metadata backend is unavailable")
	}
	if errors.Is(err, meta.ErrInvalidArgument) {
		return s3err.InvalidArgument(err.Error())
	}
	return s3err.InvalidRequest(err.Error())
}

func isMetadataUnavailable(err error) bool {
	return errors.Is(err, meta.ErrUnavailable) || errors.Is(err, context.DeadlineExceeded)
}

type listBucketsResult struct {
	XMLName xml.Name     `xml:"ListAllMyBucketsResult"`
	Owner   ownerEntry   `xml:"Owner"`
	Buckets bucketsEntry `xml:"Buckets"`
}

type ownerEntry struct {
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type bucketsEntry struct {
	Buckets []bucketEntry `xml:"Bucket"`
}

type bucketEntry struct {
	Name         string `xml:"Name"`
	CreationDate string `xml:"CreationDate"`
}

type locationConstraint struct {
	XMLName xml.Name `xml:"LocationConstraint"`
	Value   string   `xml:",chardata"`
}

type corsConfiguration struct {
	XMLName xml.Name   `xml:"CORSConfiguration"`
	Rules   []corsRule `xml:"CORSRule"`
}

type corsRule struct {
	AllowedOrigins []string `xml:"AllowedOrigin"`
	AllowedMethods []string `xml:"AllowedMethod"`
	AllowedHeaders []string `xml:"AllowedHeader,omitempty"`
	ExposeHeaders  []string `xml:"ExposeHeader,omitempty"`
	MaxAgeSeconds  int      `xml:"MaxAgeSeconds,omitempty"`
}

type lifecycleConfiguration struct {
	XMLName xml.Name        `xml:"LifecycleConfiguration"`
	Rules   []lifecycleRule `xml:"Rule"`
}

type lifecycleRule struct {
	ID                             string                             `xml:"ID,omitempty"`
	Status                         string                             `xml:"Status"`
	Filter                         *lifecycleFilter                   `xml:"Filter,omitempty"`
	Prefix                         string                             `xml:"Prefix,omitempty"`
	Expiration                     *lifecycleExpiration               `xml:"Expiration,omitempty"`
	NoncurrentVersionExpiration    *noncurrentVersionExpiration       `xml:"NoncurrentVersionExpiration,omitempty"`
	AbortIncompleteMultipartUpload *abortIncompleteMultipartUploadXML `xml:"AbortIncompleteMultipartUpload,omitempty"`
}

type lifecycleFilter struct {
	Prefix string `xml:"Prefix,omitempty"`
}

type lifecycleExpiration struct {
	Days                      int    `xml:"Days,omitempty"`
	Date                      string `xml:"Date,omitempty"`
	ExpiredObjectDeleteMarker bool   `xml:"ExpiredObjectDeleteMarker,omitempty"`
}

type noncurrentVersionExpiration struct {
	NoncurrentDays int `xml:"NoncurrentDays,omitempty"`
}

type abortIncompleteMultipartUploadXML struct {
	DaysAfterInitiation int `xml:"DaysAfterInitiation,omitempty"`
}

type bucketEncryptionConfiguration struct {
	XMLName xml.Name               `xml:"ServerSideEncryptionConfiguration"`
	Rules   []bucketEncryptionRule `xml:"Rule"`
}

type bucketEncryptionRule struct {
	ApplyServerSideEncryptionByDefault applyServerSideEncryptionByDefault `xml:"ApplyServerSideEncryptionByDefault"`
}

type applyServerSideEncryptionByDefault struct {
	SSEAlgorithm   string `xml:"SSEAlgorithm"`
	KMSMasterKeyID string `xml:"KMSMasterKeyID,omitempty"`
}

type bucketVersioningConfiguration struct {
	XMLName xml.Name `xml:"VersioningConfiguration"`
	Status  string   `xml:"Status,omitempty"`
}

type bucketObjectLockConfiguration struct {
	XMLName           xml.Name        `xml:"ObjectLockConfiguration"`
	ObjectLockEnabled string          `xml:"ObjectLockEnabled,omitempty"`
	Rule              *objectLockRule `xml:"Rule,omitempty"`
}

type objectLockRule struct {
	DefaultRetention *objectLockDefaultRetention `xml:"DefaultRetention,omitempty"`
}

type objectLockDefaultRetention struct {
	Mode  string `xml:"Mode,omitempty"`
	Days  int    `xml:"Days,omitempty"`
	Years int    `xml:"Years,omitempty"`
}

type listVersionsResult struct {
	XMLName             xml.Name             `xml:"ListVersionsResult"`
	Name                string               `xml:"Name"`
	Prefix              string               `xml:"Prefix"`
	KeyMarker           string               `xml:"KeyMarker"`
	VersionIDMarker     string               `xml:"VersionIdMarker,omitempty"`
	NextKeyMarker       string               `xml:"NextKeyMarker,omitempty"`
	NextVersionIDMarker string               `xml:"NextVersionIdMarker,omitempty"`
	Delimiter           string               `xml:"Delimiter,omitempty"`
	MaxKeys             int                  `xml:"MaxKeys"`
	IsTruncated         bool                 `xml:"IsTruncated"`
	Versions            []objectVersionEntry `xml:"Version"`
	DeleteMarkers       []deleteMarkerEntry  `xml:"DeleteMarker"`
	CommonPrefixes      []commonPrefixEntry  `xml:"CommonPrefixes"`
}

type objectVersionEntry struct {
	Key          string `xml:"Key"`
	VersionID    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type deleteMarkerEntry struct {
	Key          string `xml:"Key"`
	VersionID    string `xml:"VersionId"`
	IsLatest     bool   `xml:"IsLatest"`
	LastModified string `xml:"LastModified"`
}

type listBucketResult struct {
	XMLName               xml.Name            `xml:"ListBucketResult"`
	Name                  string              `xml:"Name"`
	Prefix                string              `xml:"Prefix"`
	Delimiter             string              `xml:"Delimiter,omitempty"`
	KeyCount              int                 `xml:"KeyCount"`
	MaxKeys               int                 `xml:"MaxKeys"`
	IsTruncated           bool                `xml:"IsTruncated"`
	NextContinuationToken string              `xml:"NextContinuationToken,omitempty"`
	Contents              []objectEntry       `xml:"Contents"`
	CommonPrefixes        []commonPrefixEntry `xml:"CommonPrefixes"`
}

type listBucketV1Result struct {
	XMLName        xml.Name            `xml:"ListBucketResult"`
	Name           string              `xml:"Name"`
	Prefix         string              `xml:"Prefix"`
	Marker         string              `xml:"Marker"`
	NextMarker     string              `xml:"NextMarker,omitempty"`
	Delimiter      string              `xml:"Delimiter,omitempty"`
	MaxKeys        int                 `xml:"MaxKeys"`
	IsTruncated    bool                `xml:"IsTruncated"`
	Contents       []objectEntry       `xml:"Contents"`
	CommonPrefixes []commonPrefixEntry `xml:"CommonPrefixes"`
}

type objectEntry struct {
	Key          string `xml:"Key"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
	StorageClass string `xml:"StorageClass"`
}

type commonPrefixEntry struct {
	Prefix string `xml:"Prefix"`
}

type accessControlPolicy struct {
	XMLName           xml.Name          `xml:"AccessControlPolicy"`
	Owner             ownerEntry        `xml:"Owner"`
	AccessControlList accessControlList `xml:"AccessControlList"`
}

type accessControlList struct {
	Grants []grantEntry `xml:"Grant"`
}

type grantEntry struct {
	Grantee    granteeEntry `xml:"Grantee"`
	Permission string       `xml:"Permission"`
}

type granteeEntry struct {
	XMLNSXSI    string `xml:"xmlns:xsi,attr,omitempty"`
	Type        string `xml:"xsi:type,attr"`
	ID          string `xml:"ID"`
	DisplayName string `xml:"DisplayName"`
}

type objectTaggingResult struct {
	XMLName xml.Name     `xml:"Tagging"`
	TagSet  objectTagSet `xml:"TagSet"`
}

type objectTagSet struct {
	Tags []objectTagEntry `xml:"Tag"`
}

type objectTagEntry struct {
	Key   string `xml:"Key"`
	Value string `xml:"Value"`
}

type objectRetentionResult struct {
	XMLName         xml.Name `xml:"Retention"`
	Mode            string   `xml:"Mode,omitempty"`
	RetainUntilDate string   `xml:"RetainUntilDate,omitempty"`
}

type objectLegalHoldResult struct {
	XMLName xml.Name `xml:"LegalHold"`
	Status  string   `xml:"Status,omitempty"`
}

type listMultipartUploadsResult struct {
	XMLName            xml.Name               `xml:"ListMultipartUploadsResult"`
	Bucket             string                 `xml:"Bucket"`
	Prefix             string                 `xml:"Prefix"`
	KeyMarker          string                 `xml:"KeyMarker"`
	UploadIDMarker     string                 `xml:"UploadIdMarker"`
	NextKeyMarker      string                 `xml:"NextKeyMarker,omitempty"`
	NextUploadIDMarker string                 `xml:"NextUploadIdMarker,omitempty"`
	Delimiter          string                 `xml:"Delimiter,omitempty"`
	MaxUploads         int                    `xml:"MaxUploads"`
	IsTruncated        bool                   `xml:"IsTruncated"`
	Uploads            []multipartUploadEntry `xml:"Upload"`
	CommonPrefixes     []commonPrefixEntry    `xml:"CommonPrefixes"`
}

type multipartUploadEntry struct {
	Key          string `xml:"Key"`
	UploadID     string `xml:"UploadId"`
	StorageClass string `xml:"StorageClass"`
	Initiated    string `xml:"Initiated"`
}

type initiateMultipartUploadResult struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type listPartsResult struct {
	XMLName  xml.Name    `xml:"ListPartsResult"`
	Bucket   string      `xml:"Bucket"`
	Key      string      `xml:"Key"`
	UploadID string      `xml:"UploadId"`
	Part     []partEntry `xml:"Part"`
}

type partEntry struct {
	PartNumber   int    `xml:"PartNumber"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
	Size         int64  `xml:"Size"`
}

type uploadPartCopyResult struct {
	XMLName      xml.Name `xml:"CopyPartResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

type completeMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

type copyObjectResult struct {
	XMLName      xml.Name `xml:"CopyObjectResult"`
	LastModified string   `xml:"LastModified"`
	ETag         string   `xml:"ETag"`
}

type deleteObjectsRequest struct {
	XMLName xml.Name                 `xml:"Delete"`
	Objects []deleteObjectIdentifier `xml:"Object"`
	Quiet   bool                     `xml:"Quiet"`
}

type deleteObjectIdentifier struct {
	Key       string `xml:"Key"`
	VersionID string `xml:"VersionId,omitempty"`
}

type deleteObjectsResult struct {
	XMLName xml.Name             `xml:"DeleteResult"`
	Deleted []deletedObjectEntry `xml:"Deleted,omitempty"`
	Errors  []deleteErrorEntry   `xml:"Error,omitempty"`
}

type deletedObjectEntry struct {
	Key                   string `xml:"Key"`
	VersionID             string `xml:"VersionId,omitempty"`
	DeleteMarker          bool   `xml:"DeleteMarker,omitempty"`
	DeleteMarkerVersionID string `xml:"DeleteMarkerVersionId,omitempty"`
}

type deleteErrorEntry struct {
	Key       string `xml:"Key"`
	VersionID string `xml:"VersionId,omitempty"`
	Code      string `xml:"Code"`
	Message   string `xml:"Message"`
}

type deleteObjectResultEntry struct {
	Deleted deletedObjectEntry
	Error   deleteErrorEntry
}

type completeMultipartUploadRequest struct {
	Part []completePart `xml:"Part"`
}

type completePart struct {
	PartNumber int    `xml:"PartNumber"`
	ETag       string `xml:"ETag"`
}

type byteRange struct {
	offset        uint64
	readLength    uint64
	contentLength uint64
	contentRange  string
	status        int
}

func parseRange(header string, size uint64) (byteRange, error) {
	if header == "" {
		return byteRange{contentLength: size, status: http.StatusOK}, nil
	}
	value, ok := strings.CutPrefix(header, "bytes=")
	if !ok || strings.Contains(value, ",") {
		return byteRange{}, errors.New("only a single bytes range is supported")
	}
	startText, endText, ok := strings.Cut(value, "-")
	if !ok {
		return byteRange{}, errors.New("invalid bytes range")
	}
	if size == 0 {
		return byteRange{}, errors.New("range is not satisfiable")
	}
	var start, end uint64
	switch {
	case startText == "":
		suffix, err := strconv.ParseUint(endText, 10, 64)
		if err != nil || suffix == 0 {
			return byteRange{}, errors.New("invalid suffix range")
		}
		if suffix > size {
			suffix = size
		}
		start = size - suffix
		end = size - 1
	case endText == "":
		parsedStart, err := strconv.ParseUint(startText, 10, 64)
		if err != nil || parsedStart >= size {
			return byteRange{}, errors.New("range is not satisfiable")
		}
		start = parsedStart
		end = size - 1
	default:
		parsedStart, err := strconv.ParseUint(startText, 10, 64)
		if err != nil {
			return byteRange{}, errors.New("invalid range start")
		}
		parsedEnd, err := strconv.ParseUint(endText, 10, 64)
		if err != nil || parsedStart > parsedEnd || parsedStart >= size {
			return byteRange{}, errors.New("range is not satisfiable")
		}
		start = parsedStart
		end = parsedEnd
		if end >= size {
			end = size - 1
		}
	}
	length := end - start + 1
	return byteRange{
		offset:        start,
		readLength:    length,
		contentLength: length,
		contentRange:  fmt.Sprintf("bytes %d-%d/%d", start, end, size),
		status:        http.StatusPartialContent,
	}, nil
}

func parseMaxKeys(value string) (int, error) {
	if value == "" {
		return 1000, nil
	}
	maxKeys, err := strconv.Atoi(value)
	if err != nil || maxKeys < 0 {
		return 0, errors.New("max-keys must be a non-negative integer")
	}
	if maxKeys > 1000 {
		return 1000, nil
	}
	return maxKeys, nil
}

func parseMaxUploads(value string) (int, error) {
	if value == "" {
		return 1000, nil
	}
	maxUploads, err := strconv.Atoi(value)
	if err != nil || maxUploads < 0 {
		return 0, errors.New("max-uploads must be a non-negative integer")
	}
	if maxUploads > 1000 {
		return 1000, nil
	}
	return maxUploads, nil
}

func parseBucketVersioning(r io.Reader) (model.BucketVersioningState, error) {
	var payload bucketVersioningConfiguration
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return "", err
	}
	switch model.BucketVersioningState(strings.TrimSpace(payload.Status)) {
	case model.BucketVersioningEnabled:
		return model.BucketVersioningEnabled, nil
	case model.BucketVersioningSuspended:
		return model.BucketVersioningSuspended, nil
	default:
		return "", errors.New("versioning status must be Enabled or Suspended")
	}
}

func parseBucketObjectLockConfiguration(r io.Reader) (model.BucketObjectLockConfiguration, error) {
	var payload bucketObjectLockConfiguration
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return model.BucketObjectLockConfiguration{}, err
	}
	enabled := strings.TrimSpace(payload.ObjectLockEnabled)
	if enabled != "Enabled" {
		return model.BucketObjectLockConfiguration{}, errors.New("ObjectLockEnabled must be Enabled")
	}
	configuration := model.BucketObjectLockConfiguration{
		Enabled: true,
	}
	if payload.Rule != nil && payload.Rule.DefaultRetention != nil {
		defaultRetention := payload.Rule.DefaultRetention
		configuration.DefaultRetention = model.BucketObjectLockDefaultRetention{
			Mode:  model.ObjectLockMode(strings.ToUpper(strings.TrimSpace(defaultRetention.Mode))),
			Days:  defaultRetention.Days,
			Years: defaultRetention.Years,
		}
	}
	return configuration, nil
}

func bucketObjectLockConfigurationFromModel(configuration model.BucketObjectLockConfiguration) bucketObjectLockConfiguration {
	out := bucketObjectLockConfiguration{}
	if !configuration.Enabled {
		return out
	}
	out.ObjectLockEnabled = "Enabled"
	retention := configuration.DefaultRetention
	if retention.Mode != "" || retention.Days > 0 || retention.Years > 0 {
		out.Rule = &objectLockRule{
			DefaultRetention: &objectLockDefaultRetention{
				Mode:  string(retention.Mode),
				Days:  retention.Days,
				Years: retention.Years,
			},
		}
	}
	return out
}

func objectLockStateFromRequest(r *http.Request, bucket model.Bucket) (model.ObjectLockRetention, model.ObjectLockLegalHoldStatus, error) {
	modeHeader := strings.TrimSpace(r.Header.Get("x-amz-object-lock-mode"))
	retainUntilHeader := strings.TrimSpace(r.Header.Get("x-amz-object-lock-retain-until-date"))
	legalHoldHeader := strings.TrimSpace(r.Header.Get("x-amz-object-lock-legal-hold"))
	hasObjectLockHeaders := modeHeader != "" || retainUntilHeader != "" || legalHoldHeader != ""
	if hasObjectLockHeaders && !bucket.ObjectLock.Enabled {
		return model.ObjectLockRetention{}, "", errors.New("object lock headers require bucket object lock configuration")
	}
	retention := defaultObjectLockRetention(bucket.ObjectLock.DefaultRetention, time.Now().UTC())
	if modeHeader != "" || retainUntilHeader != "" {
		if modeHeader == "" || retainUntilHeader == "" {
			return model.ObjectLockRetention{}, "", errors.New("object lock mode and retain-until date must be specified together")
		}
		mode := model.ObjectLockMode(strings.ToUpper(modeHeader))
		if mode != model.ObjectLockModeGovernance && mode != model.ObjectLockModeCompliance {
			return model.ObjectLockRetention{}, "", errors.New("object lock mode must be GOVERNANCE or COMPLIANCE")
		}
		retainUntil, err := parseObjectLockRetainUntilDate(retainUntilHeader)
		if err != nil {
			return model.ObjectLockRetention{}, "", err
		}
		if !retainUntil.After(time.Now().UTC()) {
			return model.ObjectLockRetention{}, "", errors.New("object lock retain-until date must be in the future")
		}
		retention = model.ObjectLockRetention{
			Mode:            mode,
			RetainUntilDate: retainUntil,
		}
	}
	legalHold := model.ObjectLockLegalHoldStatus("")
	if legalHoldHeader != "" {
		legalHold = model.ObjectLockLegalHoldStatus(strings.ToUpper(legalHoldHeader))
		switch legalHold {
		case model.ObjectLockLegalHoldOn, model.ObjectLockLegalHoldOff:
		default:
			return model.ObjectLockRetention{}, "", errors.New("object lock legal hold must be ON or OFF")
		}
	}
	return retention, legalHold, nil
}

func defaultObjectLockRetention(defaultRetention model.BucketObjectLockDefaultRetention, now time.Time) model.ObjectLockRetention {
	if defaultRetention.Mode == "" {
		return model.ObjectLockRetention{}
	}
	retainUntil := now.UTC()
	if defaultRetention.Days > 0 {
		retainUntil = retainUntil.AddDate(0, 0, defaultRetention.Days)
	} else if defaultRetention.Years > 0 {
		retainUntil = retainUntil.AddDate(defaultRetention.Years, 0, 0)
	} else {
		return model.ObjectLockRetention{}
	}
	return model.ObjectLockRetention{
		Mode:            defaultRetention.Mode,
		RetainUntilDate: retainUntil,
	}
}

func bucketObjectLockEnabledForCreate(r *http.Request) (bool, error) {
	value := strings.TrimSpace(r.Header.Get("x-amz-bucket-object-lock-enabled"))
	if value == "" {
		return false, nil
	}
	if strings.EqualFold(value, "true") {
		return true, nil
	}
	return false, errors.New("x-amz-bucket-object-lock-enabled must be true")
}

func parseObjectLockRetainUntilDate(value string) (time.Time, error) {
	retainUntil, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, fmt.Errorf("object lock retain-until date must be RFC3339: %w", err)
	}
	return retainUntil.UTC(), nil
}

func bypassGovernanceRetention(r *http.Request) bool {
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("x-amz-bypass-governance-retention")), "true") {
		return false
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	return ok && auth.AllowsAction(principal, auth.ActionBypassGovernanceRetention)
}

func (h s3Handler) checkDecryptAdmission(ctx context.Context, encryption model.ServerSideEncryption) error {
	if encryption.Algorithm != model.ServerSideEncryptionAWSKMS {
		return nil
	}
	if err := edition.Require(h.cfg.Edition, edition.FeatureSSEKMS); err != nil {
		return err
	}
	if strings.TrimSpace(encryption.KeyID) == "" || h.deps.Metadata == nil {
		return nil
	}
	key, err := h.deps.Metadata.GetKMSKey(ctx, encryption.KeyID)
	if errors.Is(err, meta.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !model.KMSKeyAllowsDecrypt(key.State) {
		return fmt.Errorf("kms key %q state %q does not allow decrypt", key.KeyID, key.State)
	}
	return nil
}

func (h s3Handler) governanceBypass(c *gin.Context, bucket model.Bucket, key, operation string) (bool, meta.AuditContext) {
	if !h.bypassGovernanceRetention(c.Request, bucket, key) {
		return false, meta.AuditContext{}
	}
	return true, requestAuditContext(c, operation+":x-amz-bypass-governance-retention")
}

func (h s3Handler) recordAccessAudit(c *gin.Context, action model.AuditAction, bucketID, key, versionID string, details map[string]string) bool {
	if h.deps.Metadata == nil {
		return true
	}
	req := meta.PutAdminAuditEventRequest{
		Action:    action,
		BucketID:  bucketID,
		Key:       key,
		VersionID: versionID,
		Details:   accessAuditDetails(c.Request, details),
		Audit:     requestAuditContext(c, accessAuditOperationName(action)),
	}
	var err error
	if h.deps.AccessAudit != nil {
		err = h.deps.AccessAudit.RecordAccessAudit(c.Request.Context(), req)
	} else {
		_, err = h.deps.Metadata.PutAdminAuditEvent(c.Request.Context(), req)
	}
	if err != nil {
		writeS3Error(c, s3err.ServiceUnavailable("audit unavailable: "+err.Error()))
		return false
	}
	return true
}

func accessAuditDetails(r *http.Request, details map[string]string) map[string]string {
	out := make(map[string]string, len(details)+8)
	for key, value := range details {
		out[key] = value
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		out["policy_decision"] = "allowed"
		out["policy_decision_source"] = "unknown"
		out["policy_engine"] = "namros_builtin"
		return out
	}
	out["policy_decision"] = "allowed"
	out["policy_engine"] = "namros_builtin"
	out["policy_decision_source"] = "authenticated_principal"
	if decision, ok := auth.PolicyDecisionFromContext(r.Context()); ok {
		if decision.Allowed {
			out["policy_decision"] = "allowed"
		} else {
			out["policy_decision"] = "denied"
		}
		out["policy_decision_source"] = decision.Source
		out["policy_decision_reason"] = decision.Reason
		out["policy_version"] = decision.PolicyVersion
	}
	if principal.Root {
		out["policy_decision_source"] = "root_principal"
	}
	out["session_type"] = "access_key"
	out["principal_access_key_id"] = principal.AccessKeyID
	out["principal_tenant_id"] = principal.TenantID
	out["principal_subject"] = principal.Subject
	out["principal_session_id"] = principal.SessionID
	out["principal_external_issuer"] = principal.ExternalIssuer
	out["principal_policy_version"] = principal.PolicyVersion
	out["principal_root"] = strconv.FormatBool(principal.Root)
	return out
}

func accessAuditOperationName(action model.AuditAction) string {
	switch action {
	case model.AuditActionGetObject:
		return "GetObject"
	case model.AuditActionHeadObject:
		return "HeadObject"
	case model.AuditActionListObjects:
		return "ListObjects"
	default:
		return string(action)
	}
}

func requestAuditContext(c *gin.Context, operation string) meta.AuditContext {
	principal, _ := auth.PrincipalFromContext(c.Request.Context())
	reason := strings.TrimSpace(c.GetHeader("x-namros-audit-reason"))
	if reason == "" {
		reason = "s3:" + operation
	}
	requestID := trace.RequestID(c.Request.Context())
	if requestID == "" {
		requestID = c.Writer.Header().Get(requestIDHeader)
	}
	return meta.AuditContext{
		RequestID: requestID,
		Reason:    reason,
		Principal: model.AuditPrincipal{
			TenantID:       principal.TenantID,
			AccessKeyID:    principal.AccessKeyID,
			DisplayName:    principal.DisplayName,
			Subject:        principal.Subject,
			Groups:         principal.Groups,
			Roles:          principal.Roles,
			SessionID:      principal.SessionID,
			ExternalIssuer: principal.ExternalIssuer,
			PolicyVersion:  principal.PolicyVersion,
			SourceIdentity: principal.SourceIdentity,
			Root:           principal.Root,
		},
	}
}

func (h s3Handler) bypassGovernanceRetention(r *http.Request, bucket model.Bucket, key string) bool {
	if bypassGovernanceRetention(r) {
		return true
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("x-amz-bypass-governance-retention")), "true") {
		return false
	}
	principal, ok := auth.PrincipalFromContext(r.Context())
	if !ok {
		return false
	}
	policy, err := h.deps.Metadata.GetBucketPolicy(r.Context(), bucket.BucketID)
	if err != nil {
		return false
	}
	return policy.Allows(principal, auth.ActionBypassGovernanceRetention, objectARN(bucket.Name, key))
}

func objectARN(bucketName, key string) string {
	if key == "" {
		return "arn:aws:s3:::" + bucketName
	}
	return "arn:aws:s3:::" + bucketName + "/" + key
}

func parseCORSConfiguration(r io.Reader) ([]model.CORSRule, error) {
	var payload corsConfiguration
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	if len(payload.Rules) == 0 {
		return nil, errors.New("CORS configuration must include at least one rule")
	}
	if len(payload.Rules) > 100 {
		return nil, errors.New("CORS configuration cannot contain more than 100 rules")
	}
	rules := make([]model.CORSRule, 0, len(payload.Rules))
	for _, rule := range payload.Rules {
		normalized, err := normalizeCORSRule(rule)
		if err != nil {
			return nil, err
		}
		rules = append(rules, normalized)
	}
	return rules, nil
}

func parseDeleteObjectsRequest(r io.Reader) (deleteObjectsRequest, error) {
	var payload deleteObjectsRequest
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return deleteObjectsRequest{}, err
	}
	if len(payload.Objects) == 0 {
		return deleteObjectsRequest{}, errors.New("delete request must include at least one object")
	}
	if len(payload.Objects) > 1000 {
		return deleteObjectsRequest{}, errors.New("delete request cannot contain more than 1000 objects")
	}
	for i := range payload.Objects {
		payload.Objects[i].Key = strings.TrimSpace(payload.Objects[i].Key)
		payload.Objects[i].VersionID = strings.TrimSpace(payload.Objects[i].VersionID)
		if payload.Objects[i].Key == "" {
			return deleteObjectsRequest{}, errors.New("delete request object key is required")
		}
	}
	return payload, nil
}

func normalizeCORSRule(rule corsRule) (model.CORSRule, error) {
	origins := cleanStringList(rule.AllowedOrigins)
	methods := cleanStringList(rule.AllowedMethods)
	if len(origins) == 0 {
		return model.CORSRule{}, errors.New("CORS rule requires at least one AllowedOrigin")
	}
	if len(methods) == 0 {
		return model.CORSRule{}, errors.New("CORS rule requires at least one AllowedMethod")
	}
	for i, method := range methods {
		method = strings.ToUpper(method)
		switch method {
		case http.MethodGet, http.MethodPut, http.MethodPost, http.MethodDelete, http.MethodHead:
			methods[i] = method
		default:
			return model.CORSRule{}, fmt.Errorf("unsupported CORS AllowedMethod %q", method)
		}
	}
	return model.CORSRule{
		AllowedOrigins: origins,
		AllowedMethods: methods,
		AllowedHeaders: cleanStringList(rule.AllowedHeaders),
		ExposeHeaders:  cleanStringList(rule.ExposeHeaders),
		MaxAgeSeconds:  rule.MaxAgeSeconds,
	}, nil
}

func corsConfigurationFromModel(rules []model.CORSRule) corsConfiguration {
	out := corsConfiguration{
		Rules: make([]corsRule, 0, len(rules)),
	}
	for _, rule := range rules {
		out.Rules = append(out.Rules, corsRule{
			AllowedOrigins: append([]string(nil), rule.AllowedOrigins...),
			AllowedMethods: append([]string(nil), rule.AllowedMethods...),
			AllowedHeaders: append([]string(nil), rule.AllowedHeaders...),
			ExposeHeaders:  append([]string(nil), rule.ExposeHeaders...),
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		})
	}
	return out
}

func parseLifecycleConfiguration(r io.Reader) (model.BucketLifecycleConfiguration, error) {
	var payload lifecycleConfiguration
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return model.BucketLifecycleConfiguration{}, err
	}
	if len(payload.Rules) == 0 {
		return model.BucketLifecycleConfiguration{}, errors.New("lifecycle configuration must include at least one rule")
	}
	if len(payload.Rules) > 1000 {
		return model.BucketLifecycleConfiguration{}, errors.New("lifecycle configuration cannot contain more than 1000 rules")
	}
	out := model.BucketLifecycleConfiguration{
		Rules: make([]model.LifecycleRule, 0, len(payload.Rules)),
	}
	for _, rule := range payload.Rules {
		normalized, err := normalizeLifecycleRule(rule)
		if err != nil {
			return model.BucketLifecycleConfiguration{}, err
		}
		out.Rules = append(out.Rules, normalized)
	}
	return out, nil
}

func normalizeLifecycleRule(rule lifecycleRule) (model.LifecycleRule, error) {
	status := model.LifecycleRuleStatus(strings.TrimSpace(rule.Status))
	switch status {
	case model.LifecycleRuleEnabled, model.LifecycleRuleDisabled:
	default:
		return model.LifecycleRule{}, errors.New("lifecycle rule status must be Enabled or Disabled")
	}
	prefix := rule.Prefix
	if rule.Filter != nil {
		prefix = rule.Filter.Prefix
	}
	out := model.LifecycleRule{
		ID:     strings.TrimSpace(rule.ID),
		Status: status,
		Prefix: prefix,
	}
	if rule.Expiration != nil {
		out.Expiration = model.LifecycleExpiration{
			Days:                      rule.Expiration.Days,
			ExpiredObjectDeleteMarker: rule.Expiration.ExpiredObjectDeleteMarker,
		}
		if strings.TrimSpace(rule.Expiration.Date) != "" {
			date, err := parseLifecycleDate(rule.Expiration.Date)
			if err != nil {
				return model.LifecycleRule{}, err
			}
			out.Expiration.Date = date
		}
	}
	if rule.NoncurrentVersionExpiration != nil {
		out.NoncurrentVersionExpiration = model.LifecycleNoncurrentVersionExpiration{
			NoncurrentDays: rule.NoncurrentVersionExpiration.NoncurrentDays,
		}
	}
	if rule.AbortIncompleteMultipartUpload != nil {
		out.AbortIncompleteMultipartUpload = model.LifecycleAbortIncompleteMultipartUpload{
			DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation,
		}
	}
	if out.Expiration.Days < 0 || out.NoncurrentVersionExpiration.NoncurrentDays < 0 || out.AbortIncompleteMultipartUpload.DaysAfterInitiation < 0 {
		return model.LifecycleRule{}, errors.New("lifecycle day values must be positive")
	}
	if out.Expiration.Days == 0 && out.Expiration.Date.IsZero() && !out.Expiration.ExpiredObjectDeleteMarker &&
		out.NoncurrentVersionExpiration.NoncurrentDays == 0 &&
		out.AbortIncompleteMultipartUpload.DaysAfterInitiation == 0 {
		return model.LifecycleRule{}, errors.New("lifecycle rule must include at least one action")
	}
	return out, nil
}

func parseLifecycleDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC(), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("lifecycle expiration Date must be RFC3339 or YYYY-MM-DD: %w", err)
	}
	return parsed.UTC(), nil
}

func lifecycleConfigurationFromModel(configuration model.BucketLifecycleConfiguration) lifecycleConfiguration {
	out := lifecycleConfiguration{
		Rules: make([]lifecycleRule, 0, len(configuration.Rules)),
	}
	for _, rule := range configuration.Rules {
		encoded := lifecycleRule{
			ID:     rule.ID,
			Status: string(rule.Status),
		}
		if rule.Prefix != "" {
			encoded.Filter = &lifecycleFilter{Prefix: rule.Prefix}
		}
		if rule.Expiration.Days > 0 || !rule.Expiration.Date.IsZero() || rule.Expiration.ExpiredObjectDeleteMarker {
			encoded.Expiration = &lifecycleExpiration{
				Days:                      rule.Expiration.Days,
				ExpiredObjectDeleteMarker: rule.Expiration.ExpiredObjectDeleteMarker,
			}
			if !rule.Expiration.Date.IsZero() {
				encoded.Expiration.Date = rule.Expiration.Date.UTC().Format(time.RFC3339)
			}
		}
		if rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
			encoded.NoncurrentVersionExpiration = &noncurrentVersionExpiration{
				NoncurrentDays: rule.NoncurrentVersionExpiration.NoncurrentDays,
			}
		}
		if rule.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 {
			encoded.AbortIncompleteMultipartUpload = &abortIncompleteMultipartUploadXML{
				DaysAfterInitiation: rule.AbortIncompleteMultipartUpload.DaysAfterInitiation,
			}
		}
		out.Rules = append(out.Rules, encoded)
	}
	return out
}

func parseBucketEncryptionConfiguration(r io.Reader) (model.ServerSideEncryption, error) {
	var payload bucketEncryptionConfiguration
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return model.ServerSideEncryption{}, err
	}
	if len(payload.Rules) != 1 {
		return model.ServerSideEncryption{}, errors.New("encryption configuration must include exactly one rule")
	}
	apply := payload.Rules[0].ApplyServerSideEncryptionByDefault
	algorithm := model.ServerSideEncryptionAlgorithm(strings.TrimSpace(apply.SSEAlgorithm))
	keyID := strings.TrimSpace(apply.KMSMasterKeyID)
	switch algorithm {
	case model.ServerSideEncryptionAES256:
		if keyID != "" {
			return model.ServerSideEncryption{}, errors.New("KMSMasterKeyID requires aws:kms encryption")
		}
		return model.ServerSideEncryption{Algorithm: model.ServerSideEncryptionAES256}, nil
	case model.ServerSideEncryptionAWSKMS:
		return model.ServerSideEncryption{Algorithm: model.ServerSideEncryptionAWSKMS, KeyID: keyID}, nil
	default:
		return model.ServerSideEncryption{}, errors.New("SSEAlgorithm must be AES256 or aws:kms")
	}
}

func bucketEncryptionConfigurationFromModel(encryption model.ServerSideEncryption) bucketEncryptionConfiguration {
	return bucketEncryptionConfiguration{
		Rules: []bucketEncryptionRule{
			{
				ApplyServerSideEncryptionByDefault: applyServerSideEncryptionByDefault{
					SSEAlgorithm:   string(encryption.Algorithm),
					KMSMasterKeyID: encryption.KeyID,
				},
			},
		},
	}
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func parseCopySource(value string) (bucket, key string, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", errors.New("x-amz-copy-source is required")
	}
	value = strings.TrimPrefix(value, "/")
	if before, _, ok := strings.Cut(value, "?"); ok {
		value = before
	}
	decoded, err := url.PathUnescape(value)
	if err != nil {
		return "", "", err
	}
	bucket, key, ok := strings.Cut(decoded, "/")
	if !ok || bucket == "" || key == "" {
		return "", "", errors.New("x-amz-copy-source must be /bucket/key")
	}
	return bucket, key, nil
}

func parseCopySourceRange(header string, size uint64) (byteRange, error) {
	if header == "" {
		return byteRange{
			offset:        0,
			readLength:    size,
			contentLength: size,
			status:        http.StatusOK,
		}, nil
	}
	parsed, err := parseRange(header, size)
	if err != nil {
		return byteRange{}, err
	}
	parsed.readLength = parsed.contentLength
	return parsed, nil
}

func parseCompleteMultipartUpload(r io.Reader) ([]completePart, error) {
	var req completeMultipartUploadRequest
	if err := xml.NewDecoder(r).Decode(&req); err != nil {
		return nil, err
	}
	if len(req.Part) == 0 {
		return nil, errors.New("complete request must include at least one part")
	}
	for i, part := range req.Part {
		if part.PartNumber < 1 || part.PartNumber > meta.MaxMultipartParts {
			return nil, fmt.Errorf("part number must be between 1 and %d", meta.MaxMultipartParts)
		}
		if i > 0 && part.PartNumber <= req.Part[i-1].PartNumber {
			return nil, errors.New("parts must be ordered by increasing part number")
		}
	}
	return req.Part, nil
}

func completePartNumbers(parts []completePart) []int {
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		out = append(out, part.PartNumber)
	}
	return out
}

func selectCompleteParts(requested []completePart, available []model.MultipartPart) ([]model.MultipartPart, error) {
	byNumber := make(map[int]model.MultipartPart, len(available))
	for _, part := range available {
		byNumber[part.PartNumber] = part
	}
	selected := make([]model.MultipartPart, 0, len(requested))
	for _, req := range requested {
		part, ok := byNumber[req.PartNumber]
		if !ok {
			return nil, fmt.Errorf("part %d is missing", req.PartNumber)
		}
		if req.ETag != "" && normalizeETag(req.ETag) != normalizeETag(part.ETag) {
			return nil, fmt.Errorf("part %d ETag does not match", req.PartNumber)
		}
		selected = append(selected, part)
	}
	return selected, nil
}

func readClosersAsReaders(readers []io.ReadCloser) []io.Reader {
	out := make([]io.Reader, 0, len(readers))
	for _, reader := range readers {
		out = append(out, reader)
	}
	return out
}

func closeAll(readers []io.ReadCloser) {
	for _, reader := range readers {
		_ = reader.Close()
	}
}

type preparedSegment struct {
	Reader             io.Reader
	SizeBytes          uint64
	PlaintextSizeBytes uint64
	Envelope           storage.EncryptionEnvelope
	Encryption         model.ServerSideEncryption
}

func (h s3Handler) prepareSegment(ctx context.Context, plaintext io.Reader, plaintextSize uint64, sse model.ServerSideEncryption, context map[string]string) (preparedSegment, error) {
	if sse.Algorithm != model.ServerSideEncryptionAWSKMS {
		return preparedSegment{
			Reader:             plaintext,
			SizeBytes:          plaintextSize,
			PlaintextSizeBytes: plaintextSize,
			Encryption:         sse,
		}, nil
	}
	if h.deps.Encryption == nil {
		return preparedSegment{}, encryption.ErrProviderUnavailable
	}
	result, err := h.deps.Encryption.EncryptSegment(ctx, encryption.EncryptSegmentRequest{
		Plaintext:     plaintext,
		PlaintextSize: plaintextSize,
		Encryption:    sse,
		Context:       context,
	})
	if err != nil {
		return preparedSegment{}, err
	}
	return preparedSegment{
		Reader:             result.Ciphertext,
		SizeBytes:          result.SizeBytes,
		PlaintextSizeBytes: result.Envelope.PlaintextSizeBytes,
		Envelope:           result.Envelope,
		Encryption:         result.Encryption,
	}, nil
}

func segmentEncryptionContext(bucketID, key string) map[string]string {
	return map[string]string{
		"bucket_id": bucketID,
		"key":       key,
	}
}

func (h s3Handler) openObjectReaders(c *gin.Context, head model.ObjectHead, offset, length uint64) ([]io.ReadCloser, error) {
	if length == 0 {
		return nil, nil
	}
	refs := objectSegmentRefs(head)
	readers := make([]io.ReadCloser, 0, len(refs))
	remainingOffset := offset
	remainingLength := length
	for _, ref := range refs {
		if remainingLength == 0 {
			break
		}
		logicalSize := segmentLogicalSize(ref)
		if remainingOffset >= logicalSize {
			remainingOffset -= logicalSize
			continue
		}
		readOffset := remainingOffset
		readLength := logicalSize - readOffset
		if readLength > remainingLength {
			readLength = remainingLength
		}
		reader, err := h.openSegmentReader(c.Request.Context(), ref, readOffset, readLength)
		if err != nil {
			closeAll(readers)
			return nil, err
		}
		readers = append(readers, reader)
		remainingLength -= readLength
		remainingOffset = 0
	}
	if remainingLength != 0 {
		closeAll(readers)
		return nil, storage.ErrInvalidRange
	}
	return readers, nil
}

func (h s3Handler) openSegmentReader(ctx context.Context, ref storage.SegmentRef, offset, length uint64) (io.ReadCloser, error) {
	if ref.Encryption.Algorithm == "" {
		return h.deps.Storage.GetSegment(ctx, ref, offset, length)
	}
	if h.deps.Encryption == nil {
		return nil, fmt.Errorf("%w: encrypted segment requires encryption provider", storage.ErrUnavailable)
	}
	logicalSize := segmentLogicalSize(ref)
	if offset > logicalSize || offset+length < offset || offset+length > logicalSize {
		return nil, storage.ErrInvalidRange
	}
	ciphertext, err := h.deps.Storage.GetSegment(ctx, ref, 0, ref.SizeBytes)
	if err != nil {
		return nil, err
	}
	plaintext, err := h.deps.Encryption.DecryptSegment(ctx, encryption.DecryptSegmentRequest{
		Ciphertext: ciphertext,
		Envelope:   ref.Encryption,
	})
	if err != nil {
		_ = ciphertext.Close()
		return nil, err
	}
	return plaintextRangeReader(sectionReadCloser{
		Reader: plaintext,
		closer: closeFunc(func() error {
			return errors.Join(plaintext.Close(), ciphertext.Close())
		}),
	}, offset, length)
}

type sectionReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r sectionReadCloser) Close() error {
	return r.closer.Close()
}

type closeFunc func() error

func (fn closeFunc) Close() error {
	return fn()
}

func plaintextRangeReader(plaintext io.ReadCloser, offset, length uint64) (io.ReadCloser, error) {
	const maxInt64Uint = uint64(1<<63 - 1)
	if offset > maxInt64Uint || length > maxInt64Uint {
		_ = plaintext.Close()
		return nil, storage.ErrInvalidRange
	}
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, plaintext, int64(offset)); err != nil {
			_ = plaintext.Close()
			if errors.Is(err, io.EOF) {
				return nil, storage.ErrInvalidRange
			}
			return nil, err
		}
	}
	return sectionReadCloser{
		Reader: io.LimitReader(plaintext, int64(length)),
		closer: plaintext,
	}, nil
}

func segmentLogicalSize(ref storage.SegmentRef) uint64 {
	if ref.Encryption.Algorithm != "" && ref.Encryption.PlaintextSizeBytes > 0 {
		return ref.Encryption.PlaintextSizeBytes
	}
	return ref.SizeBytes
}

func objectSegmentRefs(head model.ObjectHead) []storage.SegmentRef {
	if len(head.SegmentRefs) > 0 {
		return head.SegmentRefs
	}
	if head.SegmentRef.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{head.SegmentRef}
}

func objectHeadFromVersion(version model.ObjectVersion) model.ObjectHead {
	lastModified := version.CommittedAt
	if lastModified.IsZero() {
		lastModified = version.CreatedAt
	}
	return model.ObjectHead{
		BucketID:             version.BucketID,
		Key:                  version.Key,
		VersionID:            version.VersionID,
		SizeBytes:            version.SizeBytes,
		ETag:                 version.ETag,
		ContentType:          version.ContentType,
		StorageClass:         version.StorageClass,
		ServerSideEncryption: version.ServerSideEncryption,
		SegmentRef:           version.SegmentRef,
		SegmentRefs:          version.SegmentRefs,
		UserMetadata:         version.UserMetadata,
		Tags:                 version.Tags,
		ObjectLockRetention:  version.ObjectLockRetention,
		ObjectLockLegalHold:  version.ObjectLockLegalHold,
		LastModified:         lastModified,
		DeleteMarker:         version.DeleteMarker,
	}
}

func partSegmentRefs(parts []model.MultipartPart) ([]storage.SegmentRef, uint64) {
	refs := make([]storage.SegmentRef, 0, len(parts))
	var total uint64
	for _, part := range parts {
		refs = append(refs, part.SegmentRef)
		total += uint64(part.SizeBytes)
	}
	return refs, total
}

func multipartETag(parts []model.MultipartPart) (string, error) {
	buf := bytes.NewBuffer(nil)
	for _, part := range parts {
		raw, err := hex.DecodeString(strings.Trim(normalizeETag(part.ETag), `"`))
		if err != nil {
			return "", err
		}
		buf.Write(raw)
	}
	sum := md5.Sum(buf.Bytes())
	return `"` + hex.EncodeToString(sum[:]) + "-" + strconv.Itoa(len(parts)) + `"`, nil
}

func normalizeETag(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return value
	}
	return `"` + value + `"`
}

func requestContentLength(r *http.Request) uint64 {
	if r.ContentLength <= 0 {
		return 0
	}
	return uint64(r.ContentLength)
}

func contentType(r *http.Request) string {
	value := r.Header.Get("Content-Type")
	if value == "" {
		return "application/octet-stream"
	}
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil || mediaType == "" {
		return value
	}
	return mediaType
}

func userMetadata(r *http.Request) map[string]string {
	metadata := make(map[string]string)
	for name, values := range r.Header {
		if !strings.HasPrefix(strings.ToLower(name), "x-amz-meta-") {
			continue
		}
		key := strings.TrimPrefix(strings.ToLower(name), "x-amz-meta-")
		metadata[key] = strings.Join(values, ",")
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func objectTagsFromHeader(r *http.Request) (map[string]string, error) {
	value := r.Header.Get("x-amz-tagging")
	if value == "" {
		return nil, nil
	}
	parsed, err := url.ParseQuery(value)
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string, len(parsed))
	for key, values := range parsed {
		if len(values) > 1 {
			return nil, fmt.Errorf("duplicate object tag key %q", key)
		}
		value := ""
		if len(values) == 1 {
			value = values[0]
		}
		tags[key] = value
	}
	return validateObjectTags(tags)
}

func parseObjectTagging(r io.Reader) (map[string]string, error) {
	var payload objectTaggingResult
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return nil, err
	}
	tags := make(map[string]string, len(payload.TagSet.Tags))
	for _, tag := range payload.TagSet.Tags {
		if _, ok := tags[tag.Key]; ok {
			return nil, fmt.Errorf("duplicate object tag key %q", tag.Key)
		}
		tags[tag.Key] = tag.Value
	}
	return validateObjectTags(tags)
}

func parseObjectRetention(r io.Reader) (model.ObjectLockRetention, error) {
	var payload objectRetentionResult
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return model.ObjectLockRetention{}, err
	}
	mode := model.ObjectLockMode(strings.ToUpper(strings.TrimSpace(payload.Mode)))
	if mode != model.ObjectLockModeGovernance && mode != model.ObjectLockModeCompliance {
		return model.ObjectLockRetention{}, errors.New("retention mode must be GOVERNANCE or COMPLIANCE")
	}
	retainUntil, err := parseObjectLockRetainUntilDate(payload.RetainUntilDate)
	if err != nil {
		return model.ObjectLockRetention{}, err
	}
	if !retainUntil.After(time.Now().UTC()) {
		return model.ObjectLockRetention{}, errors.New("retention retain-until date must be in the future")
	}
	return model.ObjectLockRetention{
		Mode:            mode,
		RetainUntilDate: retainUntil,
	}, nil
}

func objectRetentionFromModel(retention model.ObjectLockRetention) objectRetentionResult {
	out := objectRetentionResult{
		Mode: string(retention.Mode),
	}
	if !retention.RetainUntilDate.IsZero() {
		out.RetainUntilDate = retention.RetainUntilDate.UTC().Format(time.RFC3339)
	}
	return out
}

func parseObjectLegalHold(r io.Reader) (model.ObjectLockLegalHoldStatus, error) {
	var payload objectLegalHoldResult
	if err := xml.NewDecoder(r).Decode(&payload); err != nil {
		return "", err
	}
	status := model.ObjectLockLegalHoldStatus(strings.ToUpper(strings.TrimSpace(payload.Status)))
	switch status {
	case model.ObjectLockLegalHoldOn, model.ObjectLockLegalHoldOff:
		return status, nil
	default:
		return "", errors.New("legal hold status must be ON or OFF")
	}
}

func validateObjectTags(tags map[string]string) (map[string]string, error) {
	if len(tags) == 0 {
		return nil, nil
	}
	if len(tags) > 10 {
		return nil, errors.New("object tags cannot contain more than 10 entries")
	}
	out := make(map[string]string, len(tags))
	for key, value := range tags {
		if key == "" {
			return nil, errors.New("object tag key is required")
		}
		if len(key) > 128 {
			return nil, fmt.Errorf("object tag key %q exceeds 128 bytes", key)
		}
		if len(value) > 256 {
			return nil, fmt.Errorf("object tag value for %q exceeds 256 bytes", key)
		}
		out[key] = value
	}
	return out, nil
}

func validateCompatibilityHeaders(r *http.Request, operation routing.Operation) (s3err.Error, bool) {
	if payer := strings.TrimSpace(r.Header.Get("x-amz-request-payer")); payer != "" && !strings.EqualFold(payer, "requester") {
		return s3err.InvalidArgument("x-amz-request-payer must be requester"), true
	}
	acl := strings.TrimSpace(r.Header.Get("x-amz-acl"))
	if acl == "" {
		return s3err.Error{}, false
	}
	if isNoopCannedACL(acl) {
		return s3err.Error{}, false
	}
	switch operation {
	case routing.OperationCreateBucket, routing.OperationPutBucketACL, routing.OperationPutObject, routing.OperationCopyObject, routing.OperationPutObjectACL, routing.OperationCreateMultipartUpload:
		return s3err.NotImplemented("non-private canned ACL is not implemented"), true
	default:
		return s3err.InvalidArgument("x-amz-acl is not valid for this operation"), true
	}
}

func (h s3Handler) validateEditionFeature(r *http.Request, operation routing.Operation) (s3err.Error, bool) {
	if operationUsesObjectLockFeature(r, operation) {
		if err := edition.Require(h.cfg.Edition, edition.FeatureWORMObjectLock); err != nil {
			return s3err.NotImplemented(err.Error()), true
		}
	}
	if operationUsesSSEKMSFeature(r) {
		if err := edition.Require(h.cfg.Edition, edition.FeatureSSEKMS); err != nil {
			return s3err.NotImplemented(err.Error()), true
		}
	}
	if operationUsesErasureCodingStorageClassFeature(r, operation) {
		if err := edition.Require(h.cfg.Edition, edition.FeatureErasureCoding); err != nil {
			return s3err.NotImplemented(err.Error()), true
		}
	}
	return s3err.Error{}, false
}

func operationUsesErasureCodingStorageClassFeature(r *http.Request, operation routing.Operation) bool {
	switch operation {
	case routing.OperationPutObject, routing.OperationCopyObject, routing.OperationCreateMultipartUpload:
	default:
		return false
	}
	requestedID := strings.TrimSpace(r.Header.Get("x-amz-storage-class"))
	if requestedID == "" {
		return false
	}
	def, ok := storageclass.DefaultResolver().Definition(requestedID)
	return ok && def.RedundancyBackend == storageclass.RedundancyErasureCode
}

func operationUsesObjectLockFeature(r *http.Request, operation routing.Operation) bool {
	switch operation {
	case routing.OperationGetBucketObjectLock,
		routing.OperationPutBucketObjectLock,
		routing.OperationGetObjectRetention,
		routing.OperationPutObjectRetention,
		routing.OperationGetObjectLegalHold,
		routing.OperationPutObjectLegalHold:
		return true
	case routing.OperationCreateBucket:
		return strings.TrimSpace(r.Header.Get("x-amz-bucket-object-lock-enabled")) != ""
	}
	return objectLockHeadersPresent(r)
}

func objectLockHeadersPresent(r *http.Request) bool {
	return strings.TrimSpace(r.Header.Get("x-amz-object-lock-mode")) != "" ||
		strings.TrimSpace(r.Header.Get("x-amz-object-lock-retain-until-date")) != "" ||
		strings.TrimSpace(r.Header.Get("x-amz-object-lock-legal-hold")) != "" ||
		strings.TrimSpace(r.Header.Get("x-amz-bypass-governance-retention")) != ""
}

func operationUsesSSEKMSFeature(r *http.Request) bool {
	algorithm := strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption"))
	return strings.EqualFold(algorithm, string(model.ServerSideEncryptionAWSKMS)) ||
		strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption-aws-kms-key-id")) != "" ||
		strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption-context")) != "" ||
		strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption-bucket-key-enabled")) != ""
}

func isNoopCannedACL(value string) bool {
	return strings.EqualFold(value, "private") || strings.EqualFold(value, "bucket-owner-full-control")
}

func tagEntries(tags map[string]string) []objectTagEntry {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([]objectTagEntry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, objectTagEntry{
			Key:   key,
			Value: tags[key],
		})
	}
	return entries
}

func matchingCORSRule(r *http.Request, repo meta.Repository, bucketName, method string) (model.CORSRule, bool) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" || repo == nil || bucketName == "" || method == "" {
		return model.CORSRule{}, false
	}
	bucket, err := repo.GetBucketByName(r.Context(), bucketName)
	if err != nil {
		return model.CORSRule{}, false
	}
	rules, err := repo.GetBucketCORS(r.Context(), bucket.BucketID)
	if err != nil || len(rules) == 0 {
		return model.CORSRule{}, false
	}
	method = strings.ToUpper(strings.TrimSpace(method))
	requestedHeaders := requestedCORSHeaders(r.Header.Get("Access-Control-Request-Headers"))
	for _, rule := range rules {
		if !corsOriginAllowed(rule, origin) || !corsMethodAllowed(rule, method) || !corsHeadersAllowed(rule, requestedHeaders) {
			continue
		}
		return rule, true
	}
	return model.CORSRule{}, false
}

func corsOriginAllowed(rule model.CORSRule, origin string) bool {
	for _, allowed := range rule.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func corsMethodAllowed(rule model.CORSRule, method string) bool {
	for _, allowed := range rule.AllowedMethods {
		if strings.EqualFold(allowed, method) {
			return true
		}
	}
	return false
}

func corsHeadersAllowed(rule model.CORSRule, requested []string) bool {
	for _, header := range requested {
		if !corsHeaderAllowed(rule.AllowedHeaders, header) {
			return false
		}
	}
	return true
}

func corsHeaderAllowed(allowedHeaders []string, header string) bool {
	if header == "" {
		return true
	}
	for _, allowed := range allowedHeaders {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		header = strings.ToLower(strings.TrimSpace(header))
		switch {
		case allowed == "*":
			return true
		case strings.HasSuffix(allowed, "*") && strings.HasPrefix(header, strings.TrimSuffix(allowed, "*")):
			return true
		case allowed == header:
			return true
		}
	}
	return false
}

func requestedCORSHeaders(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func writeCORSHeaders(headers http.Header, r *http.Request, rule model.CORSRule, preflight bool) {
	origin := r.Header.Get("Origin")
	headers.Set("Access-Control-Allow-Origin", corsAllowOriginValue(rule, origin))
	appendVary(headers, "Origin")
	if preflight {
		appendVary(headers, "Access-Control-Request-Method")
		appendVary(headers, "Access-Control-Request-Headers")
		headers.Set("Access-Control-Allow-Methods", strings.Join(rule.AllowedMethods, ", "))
		if requested := strings.TrimSpace(r.Header.Get("Access-Control-Request-Headers")); requested != "" {
			headers.Set("Access-Control-Allow-Headers", requested)
		} else if len(rule.AllowedHeaders) > 0 {
			headers.Set("Access-Control-Allow-Headers", strings.Join(rule.AllowedHeaders, ", "))
		}
		if rule.MaxAgeSeconds > 0 {
			headers.Set("Access-Control-Max-Age", strconv.Itoa(rule.MaxAgeSeconds))
		}
	}
	if !preflight && len(rule.ExposeHeaders) > 0 {
		headers.Set("Access-Control-Expose-Headers", strings.Join(rule.ExposeHeaders, ", "))
	}
}

func corsAllowOriginValue(rule model.CORSRule, origin string) string {
	for _, allowed := range rule.AllowedOrigins {
		if allowed == "*" {
			return "*"
		}
		if allowed == origin {
			return origin
		}
	}
	return origin
}

func appendVary(headers http.Header, value string) {
	existing := headers.Values("Vary")
	for _, header := range existing {
		for _, part := range strings.Split(header, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	headers.Add("Vary", value)
}

func writeObjectHeaders(c *gin.Context, head model.ObjectHead) {
	c.Header("ETag", head.ETag)
	c.Header("Last-Modified", head.LastModified.UTC().Format(http.TimeFormat))
	c.Header("Content-Type", head.ContentType)
	c.Header("Content-Length", strconv.FormatInt(head.SizeBytes, 10))
	c.Header("x-amz-storage-class", storageClassID(head.StorageClass))
	writeServerSideEncryptionHeaders(c, head.ServerSideEncryption)
	if head.VersionID != "" {
		c.Header("x-amz-version-id", head.VersionID)
	}
	if head.DeleteMarker {
		c.Header("x-amz-delete-marker", "true")
	}
	if head.ObjectLockRetention.Mode != "" {
		c.Header("x-amz-object-lock-mode", string(head.ObjectLockRetention.Mode))
		c.Header("x-amz-object-lock-retain-until-date", head.ObjectLockRetention.RetainUntilDate.UTC().Format(time.RFC3339))
	}
	if head.ObjectLockLegalHold != "" {
		c.Header("x-amz-object-lock-legal-hold", string(head.ObjectLockLegalHold))
	}
	for key, value := range head.UserMetadata {
		c.Header("x-amz-meta-"+key, value)
	}
}

func writeServerSideEncryptionHeaders(c *gin.Context, encryption model.ServerSideEncryption) {
	if encryption.Algorithm == "" {
		return
	}
	c.Header("x-amz-server-side-encryption", string(encryption.Algorithm))
	if encryption.Algorithm == model.ServerSideEncryptionAWSKMS && encryption.KeyID != "" {
		c.Header("x-amz-server-side-encryption-aws-kms-key-id", encryption.KeyID)
	}
}

func versionTimestamp(version model.ObjectVersion) time.Time {
	if !version.CommittedAt.IsZero() {
		return version.CommittedAt
	}
	return version.CreatedAt
}

func standardStorageClass() storage.StorageClassSnapshot {
	return storageclass.StandardSnapshot()
}

func storageClassFromRequest(r *http.Request, fallback storage.StorageClassSnapshot) (storage.StorageClassSnapshot, error) {
	return storageClassFromRequestWithSize(r, fallback, 0, false)
}

func serverSideEncryptionFromRequest(r *http.Request, fallback model.ServerSideEncryption) (model.ServerSideEncryption, error) {
	algorithm := strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption"))
	keyID := strings.TrimSpace(r.Header.Get("x-amz-server-side-encryption-aws-kms-key-id"))
	if algorithm == "" && keyID == "" {
		return fallback, nil
	}
	switch model.ServerSideEncryptionAlgorithm(algorithm) {
	case model.ServerSideEncryptionAES256:
		if keyID != "" {
			return model.ServerSideEncryption{}, fmt.Errorf("x-amz-server-side-encryption-aws-kms-key-id requires aws:kms encryption")
		}
		return model.ServerSideEncryption{Algorithm: model.ServerSideEncryptionAES256}, nil
	case model.ServerSideEncryptionAWSKMS:
		return model.ServerSideEncryption{Algorithm: model.ServerSideEncryptionAWSKMS, KeyID: keyID}, nil
	default:
		return model.ServerSideEncryption{}, fmt.Errorf("unsupported x-amz-server-side-encryption %q", algorithm)
	}
}

func storageClassFromRequestWithSize(r *http.Request, fallback storage.StorageClassSnapshot, sizeBytes uint64, hasSize bool) (storage.StorageClassSnapshot, error) {
	return storageclass.DefaultResolver().Resolve(storageclass.ResolveRequest{
		RequestedID: r.Header.Get("x-amz-storage-class"),
		Fallback:    fallback,
		SizeBytes:   sizeBytes,
		HasSize:     hasSize,
	})
}

func storageClassObjectSizeAdmission(snapshot storage.StorageClassSnapshot, sizeBytes uint64) error {
	_, err := storageclass.DefaultResolver().Resolve(storageclass.ResolveRequest{
		RequestedID: storageclass.ID(snapshot),
		Fallback:    snapshot,
		SizeBytes:   sizeBytes,
		HasSize:     true,
	})
	return err
}

func (h s3Handler) validateCompleteSegments(ctx context.Context, refs []storage.SegmentRef) error {
	validator, ok := h.deps.Storage.(storage.SegmentValidator)
	if !ok {
		return nil
	}
	for _, ref := range refs {
		if ref.Placement.RedundancyBackend != storageclass.RedundancyErasureCode {
			continue
		}
		if err := validator.ValidateSegment(ctx, ref); err != nil {
			return err
		}
	}
	return nil
}

func storageClassID(snapshot storage.StorageClassSnapshot) string {
	return storageclass.ID(snapshot)
}

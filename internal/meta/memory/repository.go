package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/meta"
	metaid "github.com/nosway/namros/internal/meta/id"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

const idCollisionRetryLimit = 8

type Repository struct {
	mu                sync.Mutex
	now               func() time.Time
	ids               metaid.Generator
	metadataSchema    model.MetadataSchemaRecord
	tenants           map[string]model.Tenant
	tenantQuotas      map[string]model.TenantQuota
	tenantUsages      map[string]model.TenantUsage
	accessKeys        map[string]model.AccessKey
	kmsKeys           map[string]model.KMSKeyRecord
	profiles          map[string]model.ComplianceProfileAttachment
	bucketsByID       map[string]model.Bucket
	bucketNameToID    map[string]string
	bucketQuotas      map[string]model.BucketQuota
	heads             map[objectKey]model.ObjectHead
	versions          map[versionKey]model.ObjectVersion
	uploads           map[uploadKey]model.MultipartUpload
	completions       map[uploadKey]model.MultipartCompletionRecord
	parts             map[partKey]model.MultipartPart
	nextAuditID       int
	auditHeadHash     string
	auditEvents       []model.AuditEvent
	protectedRefs     map[string]model.ProtectedRef
	gcCandidates      map[string]model.GCCandidateRecord
	nextMigrationOpID int
	migrationOps      []model.MetadataMigrationOperationRecord
	nextGCOpID        int
	gcOperations      []model.GCOperationRecord
	nextDedupeOpID    int
	dedupeOps         []model.DedupeOperationRecord
	dedupeLocks       map[string]model.DedupeOperationLock
	nextWorkerOpID    int
	workerOps         []model.WorkerOperationRecord
	workerLeases      map[string]model.WorkerLease
	workerControls    map[string]model.WorkerControlRecord
	sharedReleases    map[sharedObjectReleaseKey]model.SharedObjectRelease
	sharedObjects     map[string]model.SharedObject
	sharedRefs        map[string]model.SharedObjectRef
	volumePools       map[string]model.VolumePool
	nextDrainOpID     int
	drainOps          []model.VolumeDrainOperationRecord
}

type Option func(*Repository)

func WithIDGenerator(generator metaid.Generator) Option {
	return func(r *Repository) {
		if generator != nil {
			r.ids = generator
		}
	}
}

type objectKey struct {
	bucketID string
	key      string
}

type versionKey struct {
	bucketID  string
	key       string
	versionID string
}

type uploadKey struct {
	bucketID string
	key      string
	uploadID string
}

type partKey struct {
	bucketID   string
	key        string
	uploadID   string
	partNumber int
}

type sharedObjectReleaseKey struct {
	sharedObjectID string
	segmentID      string
}

func New(options ...Option) *Repository {
	return NewWithClock(func() time.Time { return time.Now().UTC() }, options...)
}

func NewWithClock(now func() time.Time, options ...Option) *Repository {
	repo := &Repository{
		now:            now,
		metadataSchema: meta.DefaultMetadataSchemaRecord(now()),
		tenants:        make(map[string]model.Tenant),
		tenantQuotas:   make(map[string]model.TenantQuota),
		tenantUsages:   make(map[string]model.TenantUsage),
		accessKeys:     make(map[string]model.AccessKey),
		kmsKeys:        make(map[string]model.KMSKeyRecord),
		profiles:       make(map[string]model.ComplianceProfileAttachment),
		bucketsByID:    make(map[string]model.Bucket),
		bucketNameToID: make(map[string]string),
		bucketQuotas:   make(map[string]model.BucketQuota),
		heads:          make(map[objectKey]model.ObjectHead),
		versions:       make(map[versionKey]model.ObjectVersion),
		uploads:        make(map[uploadKey]model.MultipartUpload),
		completions:    make(map[uploadKey]model.MultipartCompletionRecord),
		parts:          make(map[partKey]model.MultipartPart),
		protectedRefs:  make(map[string]model.ProtectedRef),
		gcCandidates:   make(map[string]model.GCCandidateRecord),
		dedupeLocks:    make(map[string]model.DedupeOperationLock),
		workerLeases:   make(map[string]model.WorkerLease),
		workerControls: make(map[string]model.WorkerControlRecord),
		sharedReleases: make(map[sharedObjectReleaseKey]model.SharedObjectRelease),
		sharedObjects:  make(map[string]model.SharedObject),
		sharedRefs:     make(map[string]model.SharedObjectRef),
		volumePools:    make(map[string]model.VolumePool),
	}
	for _, option := range options {
		if option != nil {
			option(repo)
		}
	}
	if repo.ids == nil {
		repo.ids = metaid.MustNewProcessGenerator(metaid.WithClock(now))
	}
	return repo
}

func (r *Repository) GetMetadataSchema(_ context.Context) (model.MetadataSchemaRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.metadataSchema.SchemaVersion == 0 {
		return model.MetadataSchemaRecord{}, meta.ErrNotFound
	}
	return r.metadataSchema, nil
}

func (r *Repository) PutMetadataSchema(_ context.Context, req meta.PutMetadataSchemaRequest) (model.MetadataSchemaRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := meta.MetadataSchemaRecordFromRequest(req, r.now(), r.metadataSchema)
	if err != nil {
		return model.MetadataSchemaRecord{}, err
	}
	r.metadataSchema = record
	return record, nil
}

func (r *Repository) CreateTenant(_ context.Context, req meta.CreateTenantRequest) (model.Tenant, error) {
	if req.TenantID == "" {
		return model.Tenant{}, fmt.Errorf("%w: tenant id is required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[req.TenantID]; ok {
		return model.Tenant{}, meta.ErrAlreadyExists
	}
	tenant := model.Tenant{
		TenantID:    req.TenantID,
		DisplayName: req.DisplayName,
		CreatedAt:   r.now(),
	}
	r.tenants[tenant.TenantID] = tenant
	return tenant, nil
}

func (r *Repository) GetTenant(_ context.Context, tenantID string) (model.Tenant, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	tenant, ok := r.tenants[tenantID]
	if !ok {
		return model.Tenant{}, meta.ErrNotFound
	}
	return tenant, nil
}

func (r *Repository) PutTenantQuota(_ context.Context, req meta.TenantQuotaRequest) (model.TenantQuota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[req.TenantID]; !ok {
		return model.TenantQuota{}, meta.ErrNotFound
	}
	quota, err := meta.BuildTenantQuota(r.tenantQuotas[req.TenantID], req, r.now())
	if err != nil {
		return model.TenantQuota{}, err
	}
	r.tenantQuotas[quota.TenantID] = quota
	return meta.CloneTenantQuotaRecord(quota), nil
}

func (r *Repository) GetTenantQuota(_ context.Context, tenantID string) (model.TenantQuota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[tenantID]; !ok {
		return model.TenantQuota{}, meta.ErrNotFound
	}
	quota, ok := r.tenantQuotas[tenantID]
	if !ok {
		return model.TenantQuota{}, meta.ErrNotFound
	}
	return meta.CloneTenantQuotaRecord(quota), nil
}

func (r *Repository) DeleteTenantQuota(_ context.Context, tenantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[tenantID]; !ok {
		return meta.ErrNotFound
	}
	if _, ok := r.tenantQuotas[tenantID]; !ok {
		return meta.ErrNotFound
	}
	delete(r.tenantQuotas, tenantID)
	return nil
}

func (r *Repository) PutTenantUsage(_ context.Context, req meta.TenantUsageRequest) (model.TenantUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[req.TenantID]; !ok {
		return model.TenantUsage{}, meta.ErrNotFound
	}
	usage, err := meta.BuildTenantUsage(r.tenantUsages[req.TenantID], req, r.now())
	if err != nil {
		return model.TenantUsage{}, err
	}
	r.tenantUsages[usage.TenantID] = usage
	return meta.CloneTenantUsageRecord(usage), nil
}

func (r *Repository) GetTenantUsage(_ context.Context, tenantID string) (model.TenantUsage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.tenants[tenantID]; !ok {
		return model.TenantUsage{}, meta.ErrNotFound
	}
	usage, ok := r.tenantUsages[tenantID]
	if !ok {
		return model.TenantUsage{}, meta.ErrNotFound
	}
	return meta.CloneTenantUsageRecord(usage), nil
}

func (r *Repository) checkTenantActiveUploadQuotaLocked(tenantID string) error {
	if tenantID == "" {
		return nil
	}
	quota, ok := r.tenantQuotas[tenantID]
	if !ok || quota.MaxActiveUploads <= 0 {
		return nil
	}
	usage := r.tenantUsages[tenantID]
	if usage.ActiveUploads >= quota.MaxActiveUploads {
		return fmt.Errorf("%w: tenant %q active multipart uploads %d reached max %d", meta.ErrQuotaExceeded, tenantID, usage.ActiveUploads, quota.MaxActiveUploads)
	}
	return nil
}

func (r *Repository) applyTenantActiveUploadDeltaForBucketLocked(bucketID string, delta int64, now time.Time) {
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return
	}
	r.applyTenantActiveUploadDeltaLocked(bucket.TenantID, delta, now)
}

func (r *Repository) applyTenantActiveUploadDeltaLocked(tenantID string, delta int64, now time.Time) {
	if tenantID == "" {
		return
	}
	if _, ok := r.tenants[tenantID]; !ok {
		return
	}
	usage, err := meta.ApplyTenantActiveUploadDelta(r.tenantUsages[tenantID], tenantID, delta, now)
	if err != nil {
		return
	}
	r.tenantUsages[tenantID] = usage
}

func (r *Repository) PutAccessKey(_ context.Context, req meta.PutAccessKeyRequest) (model.AccessKey, error) {
	if req.TenantID == "" || req.AccessKeyID == "" || req.SecretHash == "" {
		return model.AccessKey{}, fmt.Errorf("%w: access key fields are required", meta.ErrInvalidArgument)
	}
	status := req.Status
	if status == "" {
		status = model.AccessKeyActive
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	accessKey := model.AccessKey{
		TenantID:    req.TenantID,
		AccessKeyID: req.AccessKeyID,
		SecretHash:  req.SecretHash,
		Status:      status,
		Permissions: cloneStringSlice(req.Permissions),
		CreatedAt:   r.now(),
	}
	r.accessKeys[accessKey.AccessKeyID] = accessKey
	return cloneAccessKey(accessKey), nil
}

func (r *Repository) GetAccessKey(_ context.Context, accessKeyID string) (model.AccessKey, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	accessKey, ok := r.accessKeys[accessKeyID]
	if !ok {
		return model.AccessKey{}, meta.ErrNotFound
	}
	return cloneAccessKey(accessKey), nil
}

func (r *Repository) PutKMSKey(_ context.Context, req meta.PutKMSKeyRequest) (model.KMSKeyRecord, error) {
	keyID := strings.TrimSpace(req.KeyID)
	if keyID == "" {
		return model.KMSKeyRecord{}, fmt.Errorf("%w: kms key id is required", meta.ErrInvalidArgument)
	}
	state := model.NormalizeKMSKeyState(req.State)
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.kmsKeys[keyID]
	if record.KeyID == "" {
		record.KeyID = keyID
		record.CreatedAt = now
	}
	record.KeyVersion = strings.TrimSpace(req.KeyVersion)
	record.State = state
	record.UpdatedAt = now
	r.kmsKeys[keyID] = record
	return record, nil
}

func (r *Repository) GetKMSKey(_ context.Context, keyID string) (model.KMSKeyRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.kmsKeys[strings.TrimSpace(keyID)]
	if !ok {
		return model.KMSKeyRecord{}, meta.ErrNotFound
	}
	return record, nil
}

func (r *Repository) ListKMSKeys(_ context.Context, req meta.ListKMSKeysRequest) ([]model.KMSKeyRecord, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	records := make([]model.KMSKeyRecord, 0, min(limit, len(r.kmsKeys)))
	for _, record := range r.kmsKeys {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].KeyID < records[j].KeyID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (r *Repository) PutComplianceProfileAttachment(_ context.Context, req meta.PutComplianceProfileAttachmentRequest) (model.ComplianceProfileAttachment, error) {
	profileID := strings.TrimSpace(req.ProfileID)
	if profileID == "" {
		return model.ComplianceProfileAttachment{}, fmt.Errorf("%w: compliance profile id is required", meta.ErrInvalidArgument)
	}
	if strings.TrimSpace(req.Regulation) == "" || strings.TrimSpace(req.RecordClass) == "" {
		return model.ComplianceProfileAttachment{}, fmt.Errorf("%w: compliance profile regulation and record class are required", meta.ErrInvalidArgument)
	}
	if req.RetentionMode != model.ObjectLockModeGovernance && req.RetentionMode != model.ObjectLockModeCompliance {
		return model.ComplianceProfileAttachment{}, fmt.Errorf("%w: compliance profile retention mode is invalid", meta.ErrInvalidArgument)
	}
	if req.RetentionDays < 0 || req.RetentionYears < 0 {
		return model.ComplianceProfileAttachment{}, fmt.Errorf("%w: compliance profile retention days and years cannot be negative", meta.ErrInvalidArgument)
	}
	if req.RetentionDays <= 0 && req.RetentionYears <= 0 {
		return model.ComplianceProfileAttachment{}, fmt.Errorf("%w: compliance profile retention duration is required", meta.ErrInvalidArgument)
	}
	if req.RetentionDays > 0 && req.RetentionYears > 0 {
		return model.ComplianceProfileAttachment{}, fmt.Errorf("%w: compliance profile retention days and years are mutually exclusive", meta.ErrInvalidArgument)
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	record := r.profiles[profileID]
	if record.ProfileID == "" {
		record.ProfileID = profileID
		record.CreatedAt = now
	}
	record.Regulation = strings.TrimSpace(req.Regulation)
	record.RecordClass = strings.TrimSpace(req.RecordClass)
	record.BucketID = strings.TrimSpace(req.BucketID)
	record.Prefix = strings.TrimSpace(req.Prefix)
	record.ObjectClass = strings.TrimSpace(req.ObjectClass)
	record.RetentionMode = req.RetentionMode
	record.RetentionDays = req.RetentionDays
	record.RetentionYears = req.RetentionYears
	record.LegalHoldPolicy = strings.TrimSpace(req.LegalHoldPolicy)
	record.GovernanceBypassPolicy = strings.TrimSpace(req.GovernanceBypassPolicy)
	record.EvidenceExportPolicy = strings.TrimSpace(req.EvidenceExportPolicy)
	record.UpdatedAt = now
	r.profiles[profileID] = record
	return record, nil
}

func (r *Repository) ListComplianceProfileAttachments(_ context.Context, req meta.ListComplianceProfileAttachmentsRequest) ([]model.ComplianceProfileAttachment, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	records := make([]model.ComplianceProfileAttachment, 0, min(limit, len(r.profiles)))
	for _, record := range r.profiles {
		if req.BucketID != "" && record.BucketID != req.BucketID {
			continue
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ProfileID < records[j].ProfileID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (r *Repository) CreateBucket(_ context.Context, req meta.CreateBucketRequest) (model.Bucket, error) {
	if req.TenantID == "" || req.Name == "" || req.Region == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket fields are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketNameToID[req.Name]; ok {
		return model.Bucket{}, meta.ErrAlreadyExists
	}
	for attempt := 0; attempt < idCollisionRetryLimit; attempt++ {
		bucketID, err := r.ids.NewID(metaid.KindBucket)
		if err != nil {
			return model.Bucket{}, fmt.Errorf("%w: generate bucket id: %v", meta.ErrUnavailable, err)
		}
		if _, ok := r.bucketsByID[bucketID]; ok {
			continue
		}
		bucket := model.Bucket{
			BucketID:  bucketID,
			TenantID:  req.TenantID,
			Name:      req.Name,
			Region:    req.Region,
			CreatedAt: r.now(),
		}
		if req.ObjectLockEnabled {
			bucket.ObjectLock = model.BucketObjectLockConfiguration{Enabled: true}
			bucket.VersioningState = model.BucketVersioningEnabled
		}
		r.bucketsByID[bucket.BucketID] = bucket
		r.bucketNameToID[bucket.Name] = bucket.BucketID
		return bucket, nil
	}
	return model.Bucket{}, fmt.Errorf("%w: bucket id collision retry budget exhausted", meta.ErrUnavailable)
}

func (r *Repository) PutBucketVersioning(_ context.Context, req meta.PutBucketVersioningRequest) (model.Bucket, error) {
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if req.State != model.BucketVersioningEnabled && req.State != model.BucketVersioningSuspended {
		return model.Bucket{}, fmt.Errorf("%w: versioning state must be Enabled or Suspended", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	if bucket.ObjectLock.Enabled && req.State != model.BucketVersioningEnabled {
		return model.Bucket{}, fmt.Errorf("%w: object lock bucket versioning cannot be suspended", meta.ErrInvalidArgument)
	}
	bucket.VersioningState = req.State
	r.bucketsByID[bucket.BucketID] = bucket
	return bucket, nil
}

func (r *Repository) PutBucketCORS(_ context.Context, req meta.BucketCORSRequest) (model.Bucket, error) {
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.CORSRules = cloneCORSRules(req.Rules)
	r.bucketsByID[bucket.BucketID] = bucket
	return bucket, nil
}

func (r *Repository) GetBucketCORS(_ context.Context, bucketID string) ([]model.CORSRule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return nil, meta.ErrNotFound
	}
	return cloneCORSRules(bucket.CORSRules), nil
}

func (r *Repository) DeleteBucketCORS(_ context.Context, bucketID string) (model.Bucket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.CORSRules = nil
	r.bucketsByID[bucket.BucketID] = bucket
	return bucket, nil
}

func (r *Repository) PutBucketLifecycle(_ context.Context, req meta.BucketLifecycleRequest) (model.Bucket, error) {
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if err := validateBucketLifecycleConfiguration(req.Configuration); err != nil {
		return model.Bucket{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.Lifecycle = cloneLifecycleConfiguration(req.Configuration)
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, model.AuditActionPutBucketLifecycle, bucket.BucketID, "", "", map[string]string{
		"rule_count": strconv.Itoa(len(req.Configuration.Rules)),
	}))
	return cloneBucket(bucket), nil
}

func (r *Repository) GetBucketLifecycle(_ context.Context, bucketID string) (model.BucketLifecycleConfiguration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.BucketLifecycleConfiguration{}, meta.ErrNotFound
	}
	if len(bucket.Lifecycle.Rules) == 0 {
		return model.BucketLifecycleConfiguration{}, meta.ErrNotFound
	}
	return cloneLifecycleConfiguration(bucket.Lifecycle), nil
}

func (r *Repository) DeleteBucketLifecycle(_ context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.Lifecycle = model.BucketLifecycleConfiguration{}
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(audit, model.AuditActionDeleteBucketLifecycle, bucket.BucketID, "", "", nil))
	return cloneBucket(bucket), nil
}

func (r *Repository) PutBucketEncryption(_ context.Context, req meta.BucketEncryptionRequest) (model.Bucket, error) {
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if err := meta.ValidateBucketEncryption(req.Encryption); err != nil {
		return model.Bucket{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.DefaultEncryption = req.Encryption
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, model.AuditActionPutBucketEncryption, bucket.BucketID, "", "", map[string]string{
		"algorithm": string(req.Encryption.Algorithm),
		"key_id":    req.Encryption.KeyID,
	}))
	return cloneBucket(bucket), nil
}

func (r *Repository) GetBucketEncryption(_ context.Context, bucketID string) (model.ServerSideEncryption, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.ServerSideEncryption{}, meta.ErrNotFound
	}
	if bucket.DefaultEncryption.Algorithm == "" {
		return model.ServerSideEncryption{}, meta.ErrNotFound
	}
	return bucket.DefaultEncryption, nil
}

func (r *Repository) DeleteBucketEncryption(_ context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.DefaultEncryption = model.ServerSideEncryption{}
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(audit, model.AuditActionDeleteBucketEncryption, bucket.BucketID, "", "", nil))
	return cloneBucket(bucket), nil
}

func (r *Repository) PutBucketQuota(_ context.Context, req meta.BucketQuotaRequest) (model.BucketQuota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[req.BucketID]; !ok {
		return model.BucketQuota{}, meta.ErrNotFound
	}
	quota, err := meta.BuildBucketQuota(r.bucketQuotas[req.BucketID], req, r.now())
	if err != nil {
		return model.BucketQuota{}, err
	}
	r.bucketQuotas[quota.BucketID] = quota
	return meta.CloneBucketQuotaRecord(quota), nil
}

func (r *Repository) GetBucketQuota(_ context.Context, bucketID string) (model.BucketQuota, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[bucketID]; !ok {
		return model.BucketQuota{}, meta.ErrNotFound
	}
	quota, ok := r.bucketQuotas[bucketID]
	if !ok {
		return model.BucketQuota{}, meta.ErrNotFound
	}
	return meta.CloneBucketQuotaRecord(quota), nil
}

func (r *Repository) DeleteBucketQuota(_ context.Context, bucketID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[bucketID]; !ok {
		return meta.ErrNotFound
	}
	if _, ok := r.bucketQuotas[bucketID]; !ok {
		return meta.ErrNotFound
	}
	delete(r.bucketQuotas, bucketID)
	return nil
}

func (r *Repository) PutBucketPolicy(_ context.Context, req meta.BucketPolicyRequest) (model.Bucket, error) {
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if len(req.Policy.Statements) == 0 {
		return model.Bucket{}, fmt.Errorf("%w: bucket policy statements are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.Policy = clonePolicy(req.Policy)
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, model.AuditActionPutBucketPolicy, bucket.BucketID, "", "", map[string]string{
		"statement_count": strconv.Itoa(len(req.Policy.Statements)),
	}))
	return cloneBucket(bucket), nil
}

func (r *Repository) GetBucketPolicy(_ context.Context, bucketID string) (auth.PolicyDocument, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return auth.PolicyDocument{}, meta.ErrNotFound
	}
	if len(bucket.Policy.Statements) == 0 {
		return auth.PolicyDocument{}, meta.ErrNotFound
	}
	return clonePolicy(bucket.Policy), nil
}

func (r *Repository) DeleteBucketPolicy(_ context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.Policy = auth.PolicyDocument{}
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(audit, model.AuditActionDeleteBucketPolicy, bucket.BucketID, "", "", nil))
	return cloneBucket(bucket), nil
}

func (r *Repository) PutBucketObjectLock(_ context.Context, req meta.BucketObjectLockRequest) (model.Bucket, error) {
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if err := validateBucketObjectLockConfiguration(req.Configuration); err != nil {
		return model.Bucket{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket.ObjectLock = req.Configuration
	bucket.VersioningState = model.BucketVersioningEnabled
	r.bucketsByID[bucket.BucketID] = bucket
	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, model.AuditActionPutBucketObjectLock, bucket.BucketID, "", "", bucketObjectLockAuditDetails(req.Configuration)))
	return bucket, nil
}

func (r *Repository) GetBucketObjectLock(_ context.Context, bucketID string) (model.BucketObjectLockConfiguration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return model.BucketObjectLockConfiguration{}, meta.ErrNotFound
	}
	return bucket.ObjectLock, nil
}

func (r *Repository) GetBucketByName(_ context.Context, name string) (model.Bucket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucketID, ok := r.bucketNameToID[name]
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	return r.bucketsByID[bucketID], nil
}

func (r *Repository) ListBuckets(_ context.Context, tenantID string) ([]model.Bucket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	buckets := make([]model.Bucket, 0, len(r.bucketsByID))
	for _, bucket := range r.bucketsByID {
		if tenantID == "" || bucket.TenantID == tenantID {
			buckets = append(buckets, bucket)
		}
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Name < buckets[j].Name
	})
	return buckets, nil
}

func (r *Repository) DeleteBucket(_ context.Context, bucketID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[bucketID]
	if !ok {
		return meta.ErrNotFound
	}
	for key := range r.heads {
		if key.bucketID == bucketID {
			return meta.ErrBucketNotEmpty
		}
	}
	for key, version := range r.versions {
		if key.bucketID == bucketID && version.State == model.ObjectVersionCommitted {
			return meta.ErrBucketNotEmpty
		}
	}
	delete(r.bucketsByID, bucketID)
	delete(r.bucketNameToID, bucket.Name)
	delete(r.bucketQuotas, bucketID)
	return nil
}

func (r *Repository) BeginPutObject(_ context.Context, req meta.BeginPutObjectRequest) (model.PendingObjectVersion, error) {
	if req.BucketID == "" || req.Key == "" {
		return model.PendingObjectVersion{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.ObjectLockRetention, req.ObjectLockLegalHold); err != nil {
		return model.PendingObjectVersion{}, err
	}
	segmentRefs := normalizeSegmentRefs(req.SegmentRef, req.SegmentRefs)
	if err := meta.ValidateObjectManifestScale(segmentRefs); err != nil {
		return model.PendingObjectVersion{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[req.BucketID]; !ok {
		return model.PendingObjectVersion{}, meta.ErrNotFound
	}
	currentHead := r.heads[objectKey{bucketID: req.BucketID, key: req.Key}]
	now := r.now()
	versionID, err := r.nextObjectVersionIDLocked(req.BucketID, req.Key)
	if err != nil {
		return model.PendingObjectVersion{}, err
	}
	version := model.ObjectVersion{
		BucketID:             req.BucketID,
		Key:                  req.Key,
		VersionID:            versionID,
		VersionSortKey:       versionID,
		SizeBytes:            req.SizeBytes,
		ETag:                 req.ETag,
		ContentType:          req.ContentType,
		StorageClass:         cloneStorageClass(req.StorageClass),
		ServerSideEncryption: req.ServerSideEncryption,
		SegmentRef:           firstSegmentRef(segmentRefs),
		SegmentRefs:          cloneSegmentRefs(segmentRefs),
		UserMetadata:         cloneStringMap(req.UserMetadata),
		Tags:                 cloneStringMap(req.Tags),
		ObjectLockRetention:  req.ObjectLockRetention,
		ObjectLockLegalHold:  req.ObjectLockLegalHold,
		State:                model.ObjectVersionPending,
		CreatedAt:            now,
	}
	if err := meta.ValidateObjectVersionScale(version); err != nil {
		return model.PendingObjectVersion{}, err
	}
	r.versions[versionKey{bucketID: req.BucketID, key: req.Key, versionID: versionID}] = version
	return model.PendingObjectVersion{
		Version:           version,
		BaseHeadVersionID: currentHead.VersionID,
		BaseHead:          cloneHead(currentHead),
		BaseHeadFound:     currentHead.VersionID != "",
	}, nil
}

func (r *Repository) CommitObjectVersion(_ context.Context, req meta.CommitObjectVersionRequest) (model.ObjectHead, error) {
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return model.ObjectHead{}, fmt.Errorf("%w: commit fields are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket := r.bucketsByID[req.BucketID]
	key := objectKey{bucketID: req.BucketID, key: req.Key}
	currentHead := r.heads[key]
	if currentHead.VersionID != req.ExpectedHeadVersionID {
		return model.ObjectHead{}, meta.ErrCASConflict
	}
	vkey := versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}
	version, ok := r.versions[vkey]
	if !ok {
		return model.ObjectHead{}, meta.ErrNotFound
	}
	if version.State == model.ObjectVersionCommitted {
		return r.heads[key], nil
	}
	now := r.now()
	version.State = model.ObjectVersionCommitted
	version.CommittedAt = now
	r.versions[vkey] = version
	if bucket.VersioningState != model.BucketVersioningEnabled && currentHead.VersionID != "" && currentHead.VersionID != version.VersionID {
		old := r.deleteVersionRecordLocked(req.BucketID, req.Key, currentHead.VersionID)
		r.deleteProtectedRefsForVersionLocked(old.BucketID, old.Key, old.VersionID)
	}
	head := model.ObjectHead{
		BucketID:             version.BucketID,
		Key:                  version.Key,
		VersionID:            version.VersionID,
		SizeBytes:            version.SizeBytes,
		ETag:                 version.ETag,
		ContentType:          version.ContentType,
		StorageClass:         cloneStorageClass(version.StorageClass),
		ServerSideEncryption: version.ServerSideEncryption,
		SegmentRef:           version.SegmentRef,
		SegmentRefs:          cloneSegmentRefs(version.SegmentRefs),
		UserMetadata:         cloneStringMap(version.UserMetadata),
		Tags:                 cloneStringMap(version.Tags),
		ObjectLockRetention:  version.ObjectLockRetention,
		ObjectLockLegalHold:  version.ObjectLockLegalHold,
		LastModified:         now,
		DeleteMarker:         version.DeleteMarker,
	}
	head = r.setHeadLocked(key, head)
	r.syncProtectedRefsForVersionLocked(version, now)
	return head, nil
}

func (r *Repository) PutObjectVersion(_ context.Context, req meta.PutObjectVersionRequest) (meta.PutObjectVersionResult, error) {
	if req.BucketID == "" || req.Key == "" {
		return meta.PutObjectVersionResult{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.ObjectLockRetention, req.ObjectLockLegalHold); err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	segmentRefs := normalizeSegmentRefs(req.SegmentRef, req.SegmentRefs)
	if err := meta.ValidateObjectManifestScale(segmentRefs); err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return meta.PutObjectVersionResult{}, meta.ErrNotFound
	}
	key := objectKey{bucketID: req.BucketID, key: req.Key}
	currentHead := r.heads[key]
	result := meta.PutObjectVersionResult{
		ReplacedHead:      cloneHead(currentHead),
		ReplacedHeadFound: currentHead.VersionID != "",
	}
	now := r.now()
	versionID, err := r.nextObjectVersionIDLocked(req.BucketID, req.Key)
	if err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	version := model.ObjectVersion{
		BucketID:             req.BucketID,
		Key:                  req.Key,
		VersionID:            versionID,
		VersionSortKey:       versionID,
		SizeBytes:            req.SizeBytes,
		ETag:                 req.ETag,
		ContentType:          req.ContentType,
		StorageClass:         cloneStorageClass(req.StorageClass),
		ServerSideEncryption: req.ServerSideEncryption,
		SegmentRef:           firstSegmentRef(segmentRefs),
		SegmentRefs:          cloneSegmentRefs(segmentRefs),
		UserMetadata:         cloneStringMap(req.UserMetadata),
		Tags:                 cloneStringMap(req.Tags),
		ObjectLockRetention:  req.ObjectLockRetention,
		ObjectLockLegalHold:  req.ObjectLockLegalHold,
		State:                model.ObjectVersionCommitted,
		CreatedAt:            now,
		CommittedAt:          now,
	}
	if err := meta.ValidateObjectVersionScale(version); err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	if bucket.VersioningState != model.BucketVersioningEnabled && currentHead.VersionID != "" && currentHead.VersionID != version.VersionID {
		old := r.deleteVersionRecordLocked(req.BucketID, req.Key, currentHead.VersionID)
		r.deleteProtectedRefsForVersionLocked(old.BucketID, old.Key, old.VersionID)
	}
	r.versions[versionKey{bucketID: req.BucketID, key: req.Key, versionID: versionID}] = version
	head := model.ObjectHead{
		BucketID:             version.BucketID,
		Key:                  version.Key,
		VersionID:            version.VersionID,
		SizeBytes:            version.SizeBytes,
		ETag:                 version.ETag,
		ContentType:          version.ContentType,
		StorageClass:         cloneStorageClass(version.StorageClass),
		ServerSideEncryption: version.ServerSideEncryption,
		SegmentRef:           version.SegmentRef,
		SegmentRefs:          cloneSegmentRefs(version.SegmentRefs),
		UserMetadata:         cloneStringMap(version.UserMetadata),
		Tags:                 cloneStringMap(version.Tags),
		ObjectLockRetention:  version.ObjectLockRetention,
		ObjectLockLegalHold:  version.ObjectLockLegalHold,
		LastModified:         now,
		DeleteMarker:         version.DeleteMarker,
	}
	head = r.setHeadLocked(key, head)
	r.syncProtectedRefsForVersionLocked(version, now)
	result.Head = cloneHead(head)
	return result, nil
}

func (r *Repository) GetObjectHead(_ context.Context, bucketID, key string) (model.ObjectHead, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	head, ok := r.heads[objectKey{bucketID: bucketID, key: key}]
	if !ok {
		return model.ObjectHead{}, meta.ErrNotFound
	}
	return cloneHead(head), nil
}

func (r *Repository) GetObjectVersion(_ context.Context, bucketID, key, versionID string) (model.ObjectVersion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	version, ok := r.versions[versionKey{bucketID: bucketID, key: key, versionID: versionID}]
	if !ok {
		return model.ObjectVersion{}, meta.ErrNotFound
	}
	return cloneVersion(version), nil
}

func (r *Repository) DeleteObject(_ context.Context, req meta.DeleteObjectRequest) (model.DeleteResult, error) {
	if req.BucketID == "" || req.Key == "" {
		return model.DeleteResult{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.DeleteResult{}, meta.ErrNotFound
	}
	if req.VersionID != "" {
		return r.deleteObjectVersionLocked(req)
	}
	if bucket.VersioningState == model.BucketVersioningEnabled {
		if head, ok := r.heads[objectKey{bucketID: req.BucketID, key: req.Key}]; ok {
			if err := r.checkDeleteKMSAdmissionLocked(head.ServerSideEncryption); err != nil {
				return model.DeleteResult{}, err
			}
		}
		return r.createDeleteMarkerLocked(req)
	}
	key := objectKey{bucketID: req.BucketID, key: req.Key}
	head, ok := r.heads[key]
	if !ok {
		return model.DeleteResult{Deleted: false}, nil
	}
	if err := r.checkDeleteKMSAdmissionLocked(head.ServerSideEncryption); err != nil {
		return model.DeleteResult{}, err
	}
	now := r.now()
	if objectHeadProtectedByObjectLock(head, now, false) {
		if objectHeadProtectedByObjectLock(head, now, true) || !req.BypassGovernanceRetention || !validGovernanceBypassAudit(req.BypassAudit) {
			return model.DeleteResult{}, meta.ErrObjectLocked
		}
		r.appendAuditEventLocked(governanceBypassAuditEvent(req, model.AuditActionGovernanceBypassDeleteObject, head.VersionID, map[string]string{
			"target": "object_head",
		}))
	}
	delete(r.heads, key)
	deleted := r.deleteVersionRecordLocked(req.BucketID, req.Key, head.VersionID)
	r.deleteProtectedRefsForVersionLocked(deleted.BucketID, deleted.Key, deleted.VersionID)
	return model.DeleteResult{Deleted: true, DeletedVersionID: head.VersionID, DeletedVersion: deleted}, nil
}

func (r *Repository) ListObjects(_ context.Context, req meta.ListObjectsRequest) (model.ListObjectsResult, error) {
	if req.BucketID == "" {
		return model.ListObjectsResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	maxKeys := req.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[req.BucketID]; !ok {
		return model.ListObjectsResult{}, meta.ErrNotFound
	}
	entries := r.listEntries(req)
	if req.ContinuationToken != "" {
		entries = entriesAfter(entries, req.ContinuationToken)
	}
	result := model.ListObjectsResult{}
	if len(entries) > maxKeys {
		result.IsTruncated = true
		result.NextContinuationToken = entries[maxKeys-1].name
		entries = entries[:maxKeys]
	}
	for _, entry := range entries {
		if entry.prefix {
			result.CommonPrefixes = append(result.CommonPrefixes, entry.name)
		} else {
			result.Contents = append(result.Contents, cloneHead(entry.head))
		}
	}
	return result, nil
}

func (r *Repository) ListObjectVersions(_ context.Context, req meta.ListObjectVersionsRequest) (model.ListObjectVersionsResult, error) {
	if req.BucketID == "" {
		return model.ListObjectVersionsResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	maxKeys := req.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[req.BucketID]; !ok {
		return model.ListObjectVersionsResult{}, meta.ErrNotFound
	}
	entries := r.versionEntries(req)
	if req.KeyMarker != "" {
		entries = versionEntriesAfter(entries, req.KeyMarker, req.VersionIDMarker)
	}
	result := model.ListObjectVersionsResult{}
	if len(entries) > maxKeys {
		result.IsTruncated = true
		result.NextKeyMarker = entries[maxKeys-1].name
		result.NextVersionIDMarker = entries[maxKeys-1].versionID
		entries = entries[:maxKeys]
	}
	for _, entry := range entries {
		if entry.prefix {
			result.CommonPrefixes = append(result.CommonPrefixes, entry.name)
			continue
		}
		versionEntry := model.ObjectVersionEntry{
			Version:  cloneVersion(entry.version),
			IsLatest: entry.isLatest,
		}
		if entry.version.DeleteMarker {
			result.DeleteMarkers = append(result.DeleteMarkers, versionEntry)
		} else {
			result.Versions = append(result.Versions, versionEntry)
		}
	}
	return result, nil
}

func (r *Repository) GetObjectTags(_ context.Context, req meta.ObjectTagsRequest) (map[string]string, error) {
	if req.BucketID == "" || req.Key == "" {
		return nil, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.VersionID != "" {
		version, ok := r.versions[versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}]
		if !ok || version.State != model.ObjectVersionCommitted {
			return nil, meta.ErrNotFound
		}
		return cloneStringMap(version.Tags), nil
	}
	head, ok := r.heads[objectKey{bucketID: req.BucketID, key: req.Key}]
	if !ok {
		return nil, meta.ErrNotFound
	}
	return cloneStringMap(head.Tags), nil
}

func (r *Repository) PutObjectTags(_ context.Context, req meta.ObjectTagsRequest) error {
	if req.BucketID == "" || req.Key == "" {
		return fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tags := cloneStringMap(req.Tags)
	if req.VersionID != "" {
		vkey := versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}
		version, ok := r.versions[vkey]
		if !ok || version.State != model.ObjectVersionCommitted {
			return meta.ErrNotFound
		}
		version.Tags = tags
		r.versions[vkey] = version
		headKey := objectKey{bucketID: req.BucketID, key: req.Key}
		if head, ok := r.heads[headKey]; ok && head.VersionID == req.VersionID {
			head.Tags = cloneStringMap(tags)
			r.setHeadLocked(headKey, head)
		}
		return nil
	}
	headKey := objectKey{bucketID: req.BucketID, key: req.Key}
	head, ok := r.heads[headKey]
	if !ok {
		return meta.ErrNotFound
	}
	head.Tags = tags
	r.setHeadLocked(headKey, head)
	if head.VersionID != "" {
		vkey := versionKey{bucketID: req.BucketID, key: req.Key, versionID: head.VersionID}
		if version, ok := r.versions[vkey]; ok {
			version.Tags = cloneStringMap(tags)
			r.versions[vkey] = version
		}
	}
	return nil
}

func (r *Repository) DeleteObjectTags(ctx context.Context, req meta.ObjectTagsRequest) error {
	req.Tags = nil
	return r.PutObjectTags(ctx, req)
}

func (r *Repository) GetObjectRetention(_ context.Context, req meta.ObjectRetentionRequest) (model.ObjectLockRetention, error) {
	if req.BucketID == "" || req.Key == "" {
		return model.ObjectLockRetention{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, version, err := r.objectLockTargetLocked(req.BucketID, req.Key, req.VersionID)
	if err != nil {
		return model.ObjectLockRetention{}, err
	}
	return version.ObjectLockRetention, nil
}

func (r *Repository) PutObjectRetention(_ context.Context, req meta.ObjectRetentionRequest) error {
	if req.BucketID == "" || req.Key == "" {
		return fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.Retention, ""); err != nil {
		return err
	}
	if !req.Retention.RetainUntilDate.After(r.now()) {
		return fmt.Errorf("%w: object lock retain-until date must be in the future", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	vkey, version, err := r.objectLockTargetLocked(req.BucketID, req.Key, req.VersionID)
	if err != nil {
		return err
	}
	now := r.now()
	if retentionUpdateBlocked(version.ObjectLockRetention, req.Retention, now, false) {
		if retentionUpdateBlocked(version.ObjectLockRetention, req.Retention, now, true) || !req.BypassGovernanceRetention || !validGovernanceBypassAudit(req.BypassAudit) {
			return meta.ErrObjectLocked
		}
		r.appendAuditEventLocked(governanceBypassAuditEvent(meta.DeleteObjectRequest{
			BucketID:    req.BucketID,
			Key:         req.Key,
			VersionID:   version.VersionID,
			BypassAudit: req.BypassAudit,
		}, model.AuditActionGovernanceBypassPutObjectRetention, version.VersionID, map[string]string{
			"current_mode":         string(version.ObjectLockRetention.Mode),
			"current_retain_until": version.ObjectLockRetention.RetainUntilDate.UTC().Format(time.RFC3339Nano),
			"next_mode":            string(req.Retention.Mode),
			"next_retain_until":    req.Retention.RetainUntilDate.UTC().Format(time.RFC3339Nano),
		}))
	}
	previous := version.ObjectLockRetention
	version.ObjectLockRetention = req.Retention
	r.versions[vkey] = version
	r.updateHeadObjectLockLocked(version)
	r.syncProtectedRefsForVersionLocked(version, now)
	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, model.AuditActionPutObjectRetention, req.BucketID, req.Key, version.VersionID, map[string]string{
		"previous_mode":         string(previous.Mode),
		"previous_retain_until": formatAuditTime(previous.RetainUntilDate),
		"next_mode":             string(req.Retention.Mode),
		"next_retain_until":     formatAuditTime(req.Retention.RetainUntilDate),
	}))
	return nil
}

func (r *Repository) GetObjectLegalHold(_ context.Context, req meta.ObjectLegalHoldRequest) (model.ObjectLockLegalHoldStatus, error) {
	if req.BucketID == "" || req.Key == "" {
		return "", fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_, version, err := r.objectLockTargetLocked(req.BucketID, req.Key, req.VersionID)
	if err != nil {
		return "", err
	}
	return version.ObjectLockLegalHold, nil
}

func (r *Repository) PutObjectLegalHold(_ context.Context, req meta.ObjectLegalHoldRequest) error {
	if req.BucketID == "" || req.Key == "" {
		return fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	switch req.LegalHold {
	case model.ObjectLockLegalHoldOn, model.ObjectLockLegalHoldOff:
	default:
		return fmt.Errorf("%w: object lock legal hold status is invalid", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	vkey, version, err := r.objectLockTargetLocked(req.BucketID, req.Key, req.VersionID)
	if err != nil {
		return err
	}
	previous := version.ObjectLockLegalHold
	version.ObjectLockLegalHold = req.LegalHold
	r.versions[vkey] = version
	r.updateHeadObjectLockLocked(version)
	r.syncProtectedRefsForVersionLocked(version, r.now())
	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, model.AuditActionPutObjectLegalHold, req.BucketID, req.Key, version.VersionID, map[string]string{
		"previous_legal_hold": string(previous),
		"next_legal_hold":     string(req.LegalHold),
	}))
	return nil
}

func (r *Repository) ListAuditEvents(_ context.Context, req meta.ListAuditEventsRequest) ([]model.AuditEvent, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	events := make([]model.AuditEvent, 0, min(limit, len(r.auditEvents)))
	for _, event := range r.auditEvents {
		if req.BucketID != "" && event.BucketID != req.BucketID {
			continue
		}
		if req.Key != "" && event.Key != req.Key {
			continue
		}
		if req.Action != "" && event.Action != req.Action {
			continue
		}
		events = append(events, cloneAuditEvent(event))
		if len(events) >= limit {
			break
		}
	}
	return events, nil
}

func (r *Repository) PutAdminAuditEvent(_ context.Context, req meta.PutAdminAuditEventRequest) (model.AuditEvent, error) {
	if req.Action == "" {
		return model.AuditEvent{}, fmt.Errorf("%w: audit action is required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	r.appendAuditEventLocked(transitionAuditEvent(req.Audit, req.Action, req.BucketID, req.Key, req.VersionID, req.Details))
	return cloneAuditEvent(r.auditEvents[len(r.auditEvents)-1]), nil
}

func (r *Repository) PutAdminAuditEvents(_ context.Context, req meta.PutAdminAuditEventsRequest) ([]model.AuditEvent, error) {
	if len(req.Events) == 0 {
		return nil, nil
	}
	for _, eventReq := range req.Events {
		if eventReq.Action == "" {
			return nil, fmt.Errorf("%w: audit action is required", meta.ErrInvalidArgument)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]model.AuditEvent, 0, len(req.Events))
	for _, eventReq := range req.Events {
		r.appendAuditEventLocked(transitionAuditEvent(eventReq.Audit, eventReq.Action, eventReq.BucketID, eventReq.Key, eventReq.VersionID, eventReq.Details))
		events = append(events, cloneAuditEvent(r.auditEvents[len(r.auditEvents)-1]))
	}
	return events, nil
}

func (r *Repository) ImportOperationalMetadata(_ context.Context, req meta.ImportOperationalMetadataRequest) (meta.ImportOperationalMetadataResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if req.RequireEmptyTarget && !r.operationalMetadataEmptyLocked() {
		return meta.ImportOperationalMetadataResult{}, fmt.Errorf("%w: target operational metadata is not empty", meta.ErrAlreadyExists)
	}
	if err := r.validateOperationalMetadataImportLocked(req); err != nil {
		return meta.ImportOperationalMetadataResult{}, err
	}

	if req.MetadataSchema != nil {
		r.metadataSchema = *req.MetadataSchema
	}
	for _, record := range req.MetadataMigrationOperations {
		record = meta.CloneMetadataMigrationOperationRecord(record)
		r.migrationOps = append(r.migrationOps, record)
		r.nextMigrationOpID = max(r.nextMigrationOpID, numericIDSuffix(record.OperationID, "metadata-migration-"))
	}
	for _, event := range req.AuditEvents {
		event = cloneAuditEvent(event)
		r.auditEvents = append(r.auditEvents, event)
		r.auditHeadHash = event.EventHash
		r.nextAuditID = max(r.nextAuditID, numericIDSuffix(event.EventID, "audit-"))
	}
	for _, record := range req.KMSKeys {
		record.KeyID = strings.TrimSpace(record.KeyID)
		record.State = model.NormalizeKMSKeyState(record.State)
		r.kmsKeys[record.KeyID] = record
	}
	for _, record := range req.GCOperations {
		record = cloneGCOperationRecord(record)
		r.gcOperations = append(r.gcOperations, record)
		r.nextGCOpID = max(r.nextGCOpID, numericIDSuffix(record.OperationID, "gc-"))
	}
	for _, record := range req.DedupeOperations {
		record = cloneDedupeOperationRecord(record)
		r.dedupeOps = append(r.dedupeOps, record)
		r.nextDedupeOpID = max(r.nextDedupeOpID, numericIDSuffix(record.OperationID, "dedupe-"))
	}
	for _, shared := range req.SharedObjects {
		r.sharedObjects[shared.SharedObjectID] = cloneSharedObject(shared)
	}
	for _, release := range req.SharedObjectReleases {
		release = cloneSharedObjectRelease(release)
		key := sharedObjectReleaseKey{sharedObjectID: release.SharedObjectID, segmentID: release.SegmentID}
		r.sharedReleases[key] = release
	}
	for _, pool := range req.VolumePools {
		pool = meta.CloneVolumePool(pool)
		r.volumePools[pool.PoolID] = pool
	}
	for _, record := range req.VolumeDrainOperations {
		record = meta.CloneVolumeDrainOperationRecord(record)
		r.drainOps = append(r.drainOps, record)
		r.nextDrainOpID = max(r.nextDrainOpID, numericIDSuffix(record.OperationID, "drain-"))
	}
	for _, lease := range req.WorkerLeases {
		lease = meta.CloneWorkerLease(lease)
		r.workerLeases[meta.WorkerLeaseID(lease.WorkerKind, lease.ShardID)] = lease
	}
	for _, record := range req.WorkerOperations {
		record = meta.CloneWorkerOperationRecord(record)
		r.workerOps = append(r.workerOps, record)
		r.nextWorkerOpID = max(r.nextWorkerOpID, numericIDSuffix(record.OperationID, "worker-op-"))
	}

	return meta.ImportOperationalMetadataResult{
		MetadataSchema:              boolCount(req.MetadataSchema != nil),
		MetadataMigrationOperations: len(req.MetadataMigrationOperations),
		KMSKeys:                     len(req.KMSKeys),
		AuditEvents:                 len(req.AuditEvents),
		GCOperations:                len(req.GCOperations),
		DedupeOperations:            len(req.DedupeOperations),
		SharedObjects:               len(req.SharedObjects),
		SharedObjectReleases:        len(req.SharedObjectReleases),
		VolumePools:                 len(req.VolumePools),
		VolumeDrainOperations:       len(req.VolumeDrainOperations),
		WorkerLeases:                len(req.WorkerLeases),
		WorkerOperations:            len(req.WorkerOperations),
	}, nil
}

func (r *Repository) operationalMetadataEmptyLocked() bool {
	return len(r.kmsKeys) == 0 &&
		len(r.auditEvents) == 0 &&
		len(r.migrationOps) == 0 &&
		len(r.gcOperations) == 0 &&
		len(r.dedupeOps) == 0 &&
		len(r.sharedObjects) == 0 &&
		len(r.sharedReleases) == 0 &&
		len(r.volumePools) == 0 &&
		len(r.drainOps) == 0 &&
		len(r.workerLeases) == 0 &&
		len(r.workerOps) == 0
}

func (r *Repository) validateOperationalMetadataImportLocked(req meta.ImportOperationalMetadataRequest) error {
	if req.MetadataSchema != nil {
		if err := meta.ValidateMetadataSchemaRecord(*req.MetadataSchema); err != nil {
			return err
		}
	}
	seenMigrations := make(map[string]struct{}, len(req.MetadataMigrationOperations))
	for _, record := range req.MetadataMigrationOperations {
		operationID := strings.TrimSpace(record.OperationID)
		if operationID == "" {
			return fmt.Errorf("%w: metadata migration operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenMigrations[operationID]; ok {
			return fmt.Errorf("%w: duplicate metadata migration operation %q", meta.ErrAlreadyExists, operationID)
		}
		seenMigrations[operationID] = struct{}{}
		for _, existing := range r.migrationOps {
			if existing.OperationID == operationID {
				return fmt.Errorf("%w: metadata migration operation %q", meta.ErrAlreadyExists, operationID)
			}
		}
	}

	seenKMS := make(map[string]struct{}, len(req.KMSKeys))
	for _, record := range req.KMSKeys {
		keyID := strings.TrimSpace(record.KeyID)
		if keyID == "" {
			return fmt.Errorf("%w: kms key id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenKMS[keyID]; ok {
			return fmt.Errorf("%w: duplicate kms key %q", meta.ErrAlreadyExists, keyID)
		}
		seenKMS[keyID] = struct{}{}
		if _, ok := r.kmsKeys[keyID]; ok {
			return fmt.Errorf("%w: kms key %q", meta.ErrAlreadyExists, keyID)
		}
	}

	seenAudit := make(map[string]struct{}, len(req.AuditEvents))
	for _, event := range req.AuditEvents {
		if strings.TrimSpace(event.EventID) == "" {
			return fmt.Errorf("%w: audit event id is required", meta.ErrInvalidArgument)
		}
		if strings.TrimSpace(event.EventHash) == "" {
			return fmt.Errorf("%w: audit event hash is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenAudit[event.EventID]; ok {
			return fmt.Errorf("%w: duplicate audit event %q", meta.ErrAlreadyExists, event.EventID)
		}
		seenAudit[event.EventID] = struct{}{}
		for _, existing := range r.auditEvents {
			if existing.EventID == event.EventID {
				return fmt.Errorf("%w: audit event %q", meta.ErrAlreadyExists, event.EventID)
			}
		}
	}

	seenGC := make(map[string]struct{}, len(req.GCOperations))
	for _, record := range req.GCOperations {
		if strings.TrimSpace(record.OperationID) == "" {
			return fmt.Errorf("%w: gc operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenGC[record.OperationID]; ok {
			return fmt.Errorf("%w: duplicate gc operation %q", meta.ErrAlreadyExists, record.OperationID)
		}
		seenGC[record.OperationID] = struct{}{}
		for _, existing := range r.gcOperations {
			if existing.OperationID == record.OperationID {
				return fmt.Errorf("%w: gc operation %q", meta.ErrAlreadyExists, record.OperationID)
			}
		}
	}

	seenDedupe := make(map[string]struct{}, len(req.DedupeOperations))
	for _, record := range req.DedupeOperations {
		if strings.TrimSpace(record.OperationID) == "" {
			return fmt.Errorf("%w: dedupe operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenDedupe[record.OperationID]; ok {
			return fmt.Errorf("%w: duplicate dedupe operation %q", meta.ErrAlreadyExists, record.OperationID)
		}
		seenDedupe[record.OperationID] = struct{}{}
		for _, existing := range r.dedupeOps {
			if existing.OperationID == record.OperationID {
				return fmt.Errorf("%w: dedupe operation %q", meta.ErrAlreadyExists, record.OperationID)
			}
		}
	}

	seenShared := make(map[string]struct{}, len(req.SharedObjects))
	for _, shared := range req.SharedObjects {
		if strings.TrimSpace(shared.SharedObjectID) == "" {
			return fmt.Errorf("%w: shared object id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenShared[shared.SharedObjectID]; ok {
			return fmt.Errorf("%w: duplicate shared object %q", meta.ErrAlreadyExists, shared.SharedObjectID)
		}
		seenShared[shared.SharedObjectID] = struct{}{}
		if _, ok := r.sharedObjects[shared.SharedObjectID]; ok {
			return fmt.Errorf("%w: shared object %q", meta.ErrAlreadyExists, shared.SharedObjectID)
		}
	}

	seenRelease := make(map[sharedObjectReleaseKey]struct{}, len(req.SharedObjectReleases))
	for _, release := range req.SharedObjectReleases {
		if strings.TrimSpace(release.ReleaseID) == "" {
			return fmt.Errorf("%w: shared object release id is required", meta.ErrInvalidArgument)
		}
		if strings.TrimSpace(release.SharedObjectID) == "" {
			return fmt.Errorf("%w: shared object release shared object id is required", meta.ErrInvalidArgument)
		}
		if strings.TrimSpace(release.SegmentID) == "" {
			return fmt.Errorf("%w: shared object release segment id is required", meta.ErrInvalidArgument)
		}
		key := sharedObjectReleaseKey{sharedObjectID: release.SharedObjectID, segmentID: release.SegmentID}
		if _, ok := seenRelease[key]; ok {
			return fmt.Errorf("%w: duplicate shared object release %q", meta.ErrAlreadyExists, release.ReleaseID)
		}
		seenRelease[key] = struct{}{}
		if _, ok := r.sharedReleases[key]; ok {
			return fmt.Errorf("%w: shared object release %q", meta.ErrAlreadyExists, release.ReleaseID)
		}
	}
	seenPools := make(map[string]struct{}, len(req.VolumePools))
	for _, pool := range req.VolumePools {
		poolID := strings.TrimSpace(pool.PoolID)
		if poolID == "" {
			return fmt.Errorf("%w: volume pool id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenPools[poolID]; ok {
			return fmt.Errorf("%w: duplicate volume pool %q", meta.ErrAlreadyExists, poolID)
		}
		seenPools[poolID] = struct{}{}
		if _, ok := r.volumePools[poolID]; ok {
			return fmt.Errorf("%w: volume pool %q", meta.ErrAlreadyExists, poolID)
		}
	}

	seenDrainOps := make(map[string]struct{}, len(req.VolumeDrainOperations))
	for _, record := range req.VolumeDrainOperations {
		operationID := strings.TrimSpace(record.OperationID)
		if operationID == "" {
			return fmt.Errorf("%w: volume drain operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenDrainOps[operationID]; ok {
			return fmt.Errorf("%w: duplicate volume drain operation %q", meta.ErrAlreadyExists, operationID)
		}
		seenDrainOps[operationID] = struct{}{}
		for _, existing := range r.drainOps {
			if existing.OperationID == operationID {
				return fmt.Errorf("%w: volume drain operation %q", meta.ErrAlreadyExists, operationID)
			}
		}
	}

	seenLeases := make(map[string]struct{}, len(req.WorkerLeases))
	for _, lease := range req.WorkerLeases {
		leaseID := strings.TrimSpace(lease.LeaseID)
		workerKind := strings.TrimSpace(lease.WorkerKind)
		shardID := strings.TrimSpace(lease.ShardID)
		if leaseID == "" || workerKind == "" || shardID == "" {
			return fmt.Errorf("%w: worker lease id, kind, and shard id are required", meta.ErrInvalidArgument)
		}
		if want := meta.WorkerLeaseID(workerKind, shardID); leaseID != want {
			return fmt.Errorf("%w: worker lease id %q does not match worker kind/shard %q", meta.ErrInvalidArgument, leaseID, want)
		}
		if _, ok := seenLeases[leaseID]; ok {
			return fmt.Errorf("%w: duplicate worker lease %q", meta.ErrAlreadyExists, leaseID)
		}
		seenLeases[leaseID] = struct{}{}
		if _, ok := r.workerLeases[leaseID]; ok {
			return fmt.Errorf("%w: worker lease %q", meta.ErrAlreadyExists, leaseID)
		}
	}

	seenWorkerOps := make(map[string]struct{}, len(req.WorkerOperations))
	for _, record := range req.WorkerOperations {
		operationID := strings.TrimSpace(record.OperationID)
		if operationID == "" {
			return fmt.Errorf("%w: worker operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenWorkerOps[operationID]; ok {
			return fmt.Errorf("%w: duplicate worker operation %q", meta.ErrAlreadyExists, operationID)
		}
		seenWorkerOps[operationID] = struct{}{}
		for _, existing := range r.workerOps {
			if existing.OperationID == operationID {
				return fmt.Errorf("%w: worker operation %q", meta.ErrAlreadyExists, operationID)
			}
		}
	}
	return nil
}

func (r *Repository) PutMetadataMigrationOperation(_ context.Context, req meta.PutMetadataMigrationOperationRequest) (model.MetadataMigrationOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextMigrationOpID++
	record, err := meta.BuildMetadataMigrationOperation(fmt.Sprintf("metadata-migration-%020d", r.nextMigrationOpID), req, r.now())
	if err != nil {
		r.nextMigrationOpID--
		return model.MetadataMigrationOperationRecord{}, err
	}
	r.migrationOps = append(r.migrationOps, meta.CloneMetadataMigrationOperationRecord(record))
	return meta.CloneMetadataMigrationOperationRecord(record), nil
}

func (r *Repository) ListMetadataMigrationOperations(_ context.Context, req meta.ListMetadataMigrationOperationsRequest) ([]model.MetadataMigrationOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.migrationOps) {
		limit = len(r.migrationOps)
	}
	out := make([]model.MetadataMigrationOperationRecord, 0, limit)
	for i := len(r.migrationOps) - 1; i >= 0 && len(out) < limit; i-- {
		record := r.migrationOps[i]
		if req.Status != "" && record.Status != req.Status {
			continue
		}
		out = append(out, meta.CloneMetadataMigrationOperationRecord(record))
	}
	return out, nil
}

func (r *Repository) ListProtectedRefs(_ context.Context, req meta.ListProtectedRefsRequest) ([]model.ProtectedRef, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	refs := make([]model.ProtectedRef, 0, min(limit, len(r.protectedRefs)))
	for _, ref := range r.protectedRefs {
		if req.BucketID != "" && ref.BucketID != req.BucketID {
			continue
		}
		if req.Key != "" && ref.Key != req.Key {
			continue
		}
		if req.VersionID != "" && ref.VersionID != req.VersionID {
			continue
		}
		if req.SegmentID != "" && ref.SegmentID != req.SegmentID {
			continue
		}
		if req.ActiveOnly && !protectedRefActive(ref, now) {
			continue
		}
		refs = append(refs, cloneProtectedRef(ref))
		if len(refs) >= limit {
			break
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].RefID < refs[j].RefID
	})
	return refs, nil
}

func (r *Repository) PutGCCandidate(_ context.Context, req meta.PutGCCandidateRequest) (model.GCCandidateRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segmentID := strings.TrimSpace(req.SegmentRef.SegmentID)
	record, err := meta.BuildGCCandidate(r.gcCandidates[segmentID], req, r.now())
	if err != nil {
		return model.GCCandidateRecord{}, err
	}
	r.gcCandidates[record.SegmentID] = meta.CloneGCCandidateRecord(record)
	return meta.CloneGCCandidateRecord(record), nil
}

func (r *Repository) ListGCCandidates(_ context.Context, req meta.ListGCCandidatesRequest) ([]model.GCCandidateRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 {
		limit = len(r.gcCandidates)
	}
	records := make([]model.GCCandidateRecord, 0, min(limit, len(r.gcCandidates)))
	for _, record := range r.gcCandidates {
		records = append(records, meta.CloneGCCandidateRecord(record))
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].SegmentID < records[j].SegmentID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, nil
}

func (r *Repository) DeleteGCCandidate(_ context.Context, segmentID string) error {
	segmentID = strings.TrimSpace(segmentID)
	if segmentID == "" {
		return fmt.Errorf("%w: gc candidate segment id is required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.gcCandidates[segmentID]; !ok {
		return meta.ErrNotFound
	}
	delete(r.gcCandidates, segmentID)
	return nil
}

func (r *Repository) PutGCOperation(_ context.Context, req meta.PutGCOperationRequest) (model.GCOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextGCOpID++
	record := model.GCOperationRecord{
		OperationID:         fmt.Sprintf("gc-%020d", r.nextGCOpID),
		ResumeOfOperationID: req.ResumeOfOperationID,
		Status:              normalizeGCOperationStatus(req.Status, req.Retryable),
		StartedAt:           req.StartedAt.UTC(),
		FinishedAt:          req.FinishedAt.UTC(),
		Scanned:             req.Scanned,
		Deleted:             req.Deleted,
		Skipped:             req.Skipped,
		Retryable:           req.Retryable,
		Attempts:            cloneGCOperationAttempts(req.Attempts),
		CreatedAt:           r.now(),
	}
	r.gcOperations = append(r.gcOperations, cloneGCOperationRecord(record))
	return cloneGCOperationRecord(record), nil
}

func (r *Repository) ListGCOperations(_ context.Context, req meta.ListGCOperationsRequest) ([]model.GCOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.gcOperations) {
		limit = len(r.gcOperations)
	}
	out := make([]model.GCOperationRecord, 0, limit)
	for i := len(r.gcOperations) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, cloneGCOperationRecord(r.gcOperations[i]))
	}
	return out, nil
}

func (r *Repository) PutDedupeOperation(_ context.Context, req meta.PutDedupeOperationRequest) (model.DedupeOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextDedupeOpID++
	record := model.DedupeOperationRecord{
		OperationID:         fmt.Sprintf("dedupe-%020d", r.nextDedupeOpID),
		ResumeOfOperationID: req.ResumeOfOperationID,
		Status:              normalizeDedupeOperationStatus(req.Status, req.Retryable),
		StartedAt:           req.StartedAt.UTC(),
		FinishedAt:          req.FinishedAt.UTC(),
		Scanned:             req.Scanned,
		Acked:               req.Acked,
		Skipped:             req.Skipped,
		Retryable:           req.Retryable,
		Attempts:            cloneDedupeOperationAttempts(req.Attempts),
		CreatedAt:           r.now(),
	}
	r.dedupeOps = append(r.dedupeOps, cloneDedupeOperationRecord(record))
	return cloneDedupeOperationRecord(record), nil
}

func (r *Repository) ListDedupeOperations(_ context.Context, req meta.ListDedupeOperationsRequest) ([]model.DedupeOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.dedupeOps) {
		limit = len(r.dedupeOps)
	}
	out := make([]model.DedupeOperationRecord, 0, limit)
	for i := len(r.dedupeOps) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, cloneDedupeOperationRecord(r.dedupeOps[i]))
	}
	return out, nil
}

func (r *Repository) AcquireDedupeOperationLock(_ context.Context, req meta.AcquireDedupeOperationLockRequest) (model.DedupeOperationLock, error) {
	lockID := strings.TrimSpace(req.LockID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if lockID == "" || ownerID == "" {
		return model.DedupeOperationLock{}, fmt.Errorf("%w: dedupe lock id and owner id are required", meta.ErrInvalidArgument)
	}
	if req.TTL <= 0 {
		return model.DedupeOperationLock{}, fmt.Errorf("%w: dedupe lock ttl must be positive", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now().UTC()
	existing, ok := r.dedupeLocks[lockID]
	if ok && existing.ExpiresAt.After(now) && existing.OwnerID != ownerID {
		return model.DedupeOperationLock{}, fmt.Errorf("%w: dedupe lock %q is held by %q until %s", meta.ErrCASConflict, lockID, existing.OwnerID, existing.ExpiresAt.Format(time.RFC3339Nano))
	}

	acquiredAt := now
	if ok && existing.OwnerID == ownerID && existing.ExpiresAt.After(now) {
		acquiredAt = existing.AcquiredAt.UTC()
	}
	lock := model.DedupeOperationLock{
		LockID:     lockID,
		OwnerID:    ownerID,
		AcquiredAt: acquiredAt,
		UpdatedAt:  now,
		ExpiresAt:  now.Add(req.TTL),
	}
	r.dedupeLocks[lockID] = cloneDedupeOperationLock(lock)
	return cloneDedupeOperationLock(lock), nil
}

func (r *Repository) ReleaseDedupeOperationLock(_ context.Context, req meta.ReleaseDedupeOperationLockRequest) error {
	lockID := strings.TrimSpace(req.LockID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if lockID == "" || ownerID == "" {
		return fmt.Errorf("%w: dedupe lock id and owner id are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.dedupeLocks[lockID]
	if !ok {
		return meta.ErrNotFound
	}
	if existing.OwnerID != ownerID {
		return fmt.Errorf("%w: dedupe lock %q is held by %q", meta.ErrCASConflict, lockID, existing.OwnerID)
	}
	delete(r.dedupeLocks, lockID)
	return nil
}

func (r *Repository) PutSharedObjectRelease(_ context.Context, req meta.PutSharedObjectReleaseRequest) (model.SharedObjectRelease, error) {
	sharedObjectID := strings.TrimSpace(req.SharedObjectID)
	if sharedObjectID == "" {
		return model.SharedObjectRelease{}, fmt.Errorf("%w: shared object id is required", meta.ErrInvalidArgument)
	}
	segmentID := strings.TrimSpace(req.SegmentRef.SegmentID)
	if segmentID == "" {
		return model.SharedObjectRelease{}, fmt.Errorf("%w: segment id is required", meta.ErrInvalidArgument)
	}
	status := normalizeSharedObjectReleaseStatus(req.Status)
	now := r.now()
	key := sharedObjectReleaseKey{sharedObjectID: sharedObjectID, segmentID: segmentID}

	r.mu.Lock()
	defer r.mu.Unlock()
	release := r.sharedReleases[key]
	if release.ReleaseID == "" {
		release.ReleaseID = sharedObjectReleaseID(sharedObjectID, segmentID)
		release.SharedObjectID = sharedObjectID
		release.SegmentID = segmentID
		release.CreatedAt = now
	}
	release.SegmentRef = cloneSegmentRef(req.SegmentRef)
	release.SegmentRef.SharedObjectID = sharedObjectID
	release.Reason = req.Reason
	if release.Reason == "" {
		release.Reason = storage.DeleteReasonManualGC
	}
	release.Status = status
	release.UpdatedAt = now
	r.sharedReleases[key] = cloneSharedObjectRelease(release)
	return cloneSharedObjectRelease(release), nil
}

func (r *Repository) ListSharedObjectReleases(_ context.Context, req meta.ListSharedObjectReleasesRequest) ([]model.SharedObjectRelease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.sharedReleases) {
		limit = len(r.sharedReleases)
	}
	releases := make([]model.SharedObjectRelease, 0, len(r.sharedReleases))
	for _, release := range r.sharedReleases {
		if req.SharedObjectID != "" && release.SharedObjectID != req.SharedObjectID {
			continue
		}
		if req.Status != "" && release.Status != req.Status {
			continue
		}
		releases = append(releases, cloneSharedObjectRelease(release))
	}
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].ReleaseID < releases[j].ReleaseID
	})
	if len(releases) > limit {
		releases = releases[:limit]
	}
	return releases, nil
}

func (r *Repository) PutVolumePool(_ context.Context, req meta.PutVolumePoolRequest) (model.VolumePool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing := r.volumePools[strings.TrimSpace(req.PoolID)]
	pool, err := meta.BuildVolumePool(existing, req, r.now())
	if err != nil {
		return model.VolumePool{}, err
	}
	r.volumePools[pool.PoolID] = meta.CloneVolumePool(pool)
	return meta.CloneVolumePool(pool), nil
}

func (r *Repository) GetVolumePool(_ context.Context, poolID string) (model.VolumePool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	pool, ok := r.volumePools[strings.TrimSpace(poolID)]
	if !ok {
		return model.VolumePool{}, meta.ErrNotFound
	}
	return meta.CloneVolumePool(pool), nil
}

func (r *Repository) ListVolumePools(_ context.Context, req meta.ListVolumePoolsRequest) ([]model.VolumePool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.volumePools) {
		limit = len(r.volumePools)
	}
	pools := make([]model.VolumePool, 0, len(r.volumePools))
	for _, pool := range r.volumePools {
		pools = append(pools, meta.CloneVolumePool(pool))
	}
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].PoolID < pools[j].PoolID
	})
	if len(pools) > limit {
		pools = pools[:limit]
	}
	return pools, nil
}

func (r *Repository) PutVolumeDrainOperation(_ context.Context, req meta.PutVolumeDrainOperationRequest) (model.VolumeDrainOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextDrainOpID++
	record, err := meta.BuildVolumeDrainOperation(fmt.Sprintf("drain-%020d", r.nextDrainOpID), req, r.now())
	if err != nil {
		r.nextDrainOpID--
		return model.VolumeDrainOperationRecord{}, err
	}
	r.drainOps = append(r.drainOps, meta.CloneVolumeDrainOperationRecord(record))
	return meta.CloneVolumeDrainOperationRecord(record), nil
}

func (r *Repository) ListVolumeDrainOperations(_ context.Context, req meta.ListVolumeDrainOperationsRequest) ([]model.VolumeDrainOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.drainOps) {
		limit = len(r.drainOps)
	}
	out := make([]model.VolumeDrainOperationRecord, 0, limit)
	for i := len(r.drainOps) - 1; i >= 0 && len(out) < limit; i-- {
		record := r.drainOps[i]
		if req.SourceVolumeID != "" && record.SourceVolumeID != req.SourceVolumeID {
			continue
		}
		if req.TargetVolumeID != "" && record.TargetVolumeID != req.TargetVolumeID {
			continue
		}
		if req.Status != "" && record.Status != req.Status {
			continue
		}
		out = append(out, meta.CloneVolumeDrainOperationRecord(record))
	}
	return out, nil
}

func (r *Repository) AcquireWorkerLease(_ context.Context, req meta.AcquireWorkerLeaseRequest) (model.WorkerLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	leaseID := meta.WorkerLeaseID(req.WorkerKind, req.ShardID)
	lease, err := meta.BuildWorkerLease(r.workerLeases[leaseID], req, r.now())
	if err != nil {
		return model.WorkerLease{}, err
	}
	r.workerLeases[lease.LeaseID] = meta.CloneWorkerLease(lease)
	return meta.CloneWorkerLease(lease), nil
}

func (r *Repository) ReleaseWorkerLease(_ context.Context, req meta.ReleaseWorkerLeaseRequest) error {
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	leaseID := meta.WorkerLeaseID(workerKind, shardID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if workerKind == "" || shardID == "" || ownerID == "" {
		return fmt.Errorf("%w: worker kind, shard id, and owner id are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.workerLeases[leaseID]
	if !ok {
		return meta.ErrNotFound
	}
	if existing.OwnerID != ownerID {
		return fmt.Errorf("%w: worker lease %q is held by %q", meta.ErrCASConflict, leaseID, existing.OwnerID)
	}
	now := r.now()
	existing.UpdatedAt = now
	existing.ExpiresAt = now
	r.workerLeases[leaseID] = existing
	return nil
}

func (r *Repository) ListWorkerLeases(_ context.Context, req meta.ListWorkerLeasesRequest) ([]model.WorkerLease, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.workerLeases) {
		limit = len(r.workerLeases)
	}
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	leases := make([]model.WorkerLease, 0, len(r.workerLeases))
	for _, lease := range r.workerLeases {
		if workerKind != "" && lease.WorkerKind != workerKind {
			continue
		}
		if shardID != "" && lease.ShardID != shardID {
			continue
		}
		leases = append(leases, meta.CloneWorkerLease(lease))
	}
	sort.Slice(leases, func(i, j int) bool {
		return leases[i].LeaseID < leases[j].LeaseID
	})
	if len(leases) > limit {
		leases = leases[:limit]
	}
	return leases, nil
}

func (r *Repository) PutWorkerOperation(_ context.Context, req meta.PutWorkerOperationRequest) (model.WorkerOperationRecord, error) {
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if workerKind == "" || shardID == "" || ownerID == "" {
		return model.WorkerOperationRecord{}, fmt.Errorf("%w: worker kind, shard id, and owner id are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextWorkerOpID++
	record := model.WorkerOperationRecord{
		OperationID: fmt.Sprintf("worker-op-%020d", r.nextWorkerOpID),
		WorkerKind:  workerKind,
		ShardID:     shardID,
		OwnerID:     ownerID,
		LeaseID:     req.LeaseID,
		Status:      meta.NormalizeWorkerOperationStatus(req.Status, req.Retryable),
		Cursor:      strings.TrimSpace(req.Cursor),
		Scanned:     req.Scanned,
		Processed:   req.Processed,
		Skipped:     req.Skipped,
		Retryable:   req.Retryable,
		LastError:   req.LastError,
		StartedAt:   req.StartedAt.UTC(),
		FinishedAt:  req.FinishedAt.UTC(),
		CreatedAt:   r.now(),
	}
	r.workerOps = append(r.workerOps, meta.CloneWorkerOperationRecord(record))
	return meta.CloneWorkerOperationRecord(record), nil
}

func (r *Repository) ListWorkerOperations(_ context.Context, req meta.ListWorkerOperationsRequest) ([]model.WorkerOperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.workerOps) {
		limit = len(r.workerOps)
	}
	out := make([]model.WorkerOperationRecord, 0, limit)
	for i := len(r.workerOps) - 1; i >= 0 && len(out) < limit; i-- {
		record := r.workerOps[i]
		if req.WorkerKind != "" && record.WorkerKind != req.WorkerKind {
			continue
		}
		if req.ShardID != "" && record.ShardID != req.ShardID {
			continue
		}
		if req.Status != "" && record.Status != req.Status {
			continue
		}
		out = append(out, meta.CloneWorkerOperationRecord(record))
	}
	return out, nil
}

func (r *Repository) PutWorkerControl(_ context.Context, req meta.PutWorkerControlRequest) (model.WorkerControlRecord, error) {
	controlID := meta.WorkerControlID(req.WorkerKind, req.ShardID)
	r.mu.Lock()
	defer r.mu.Unlock()
	record, err := meta.BuildWorkerControl(r.workerControls[controlID], req, r.now())
	if err != nil {
		return model.WorkerControlRecord{}, err
	}
	r.workerControls[meta.WorkerControlID(record.WorkerKind, record.ShardID)] = meta.CloneWorkerControlRecord(record)
	return meta.CloneWorkerControlRecord(record), nil
}

func (r *Repository) GetWorkerControl(_ context.Context, req meta.GetWorkerControlRequest) (model.WorkerControlRecord, error) {
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	if workerKind == "" || shardID == "" {
		return model.WorkerControlRecord{}, fmt.Errorf("%w: worker kind and shard id are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.workerControls[meta.WorkerControlID(workerKind, shardID)]
	if !ok {
		return model.WorkerControlRecord{}, meta.ErrNotFound
	}
	return meta.CloneWorkerControlRecord(record), nil
}

func (r *Repository) PublishSharedObject(_ context.Context, req meta.PublishSharedObjectRequest) (model.SharedObject, error) {
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return model.SharedObject{}, fmt.Errorf("%w: bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.SharedObject{}, meta.ErrNotFound
	}
	version, ok := r.versions[versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}]
	if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
		return model.SharedObject{}, meta.ErrNotFound
	}
	now := r.now()
	shared, err := sharedObjectFromVersion(bucket, version, now)
	if err != nil {
		return model.SharedObject{}, err
	}
	shared.ProtectedRootCount = r.sharedObjectProtectedRootCountLocked(shared, now)
	if existing, ok := r.sharedObjects[shared.SharedObjectID]; ok {
		existing.ProtectedRootCount = r.sharedObjectProtectedRootCountLocked(existing, now)
		existing.UpdatedAt = now
		r.sharedObjects[existing.SharedObjectID] = cloneSharedObject(existing)
		ref := sharedObjectRefFromVersion(existing, version, now)
		if _, ok := r.sharedRefs[ref.RefID]; !ok {
			r.sharedRefs[ref.RefID] = cloneSharedObjectRef(ref)
		}
		return cloneSharedObject(existing), nil
	}
	r.sharedObjects[shared.SharedObjectID] = cloneSharedObject(shared)
	ref := sharedObjectRefFromVersion(shared, version, shared.CreatedAt)
	r.sharedRefs[ref.RefID] = cloneSharedObjectRef(ref)
	return cloneSharedObject(shared), nil
}

func (r *Repository) GetSharedObject(_ context.Context, sharedObjectID string) (model.SharedObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	shared, ok := r.sharedObjects[sharedObjectID]
	if !ok {
		return model.SharedObject{}, meta.ErrNotFound
	}
	return cloneSharedObject(shared), nil
}

func (r *Repository) ListSharedObjects(_ context.Context, req meta.ListSharedObjectsRequest) ([]model.SharedObject, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.sharedObjects) {
		limit = len(r.sharedObjects)
	}
	objects := make([]model.SharedObject, 0, len(r.sharedObjects))
	for _, shared := range r.sharedObjects {
		if req.TenantID != "" && shared.TenantID != req.TenantID {
			continue
		}
		if req.BucketID != "" && shared.BucketID != req.BucketID {
			continue
		}
		if req.Key != "" && shared.Key != req.Key {
			continue
		}
		objects = append(objects, cloneSharedObject(shared))
	}
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].SharedObjectID < objects[j].SharedObjectID
	})
	if len(objects) > limit {
		objects = objects[:limit]
	}
	return objects, nil
}

func (r *Repository) AttachObjectVersionToSharedObject(_ context.Context, req meta.AttachObjectVersionToSharedObjectRequest) (model.AttachSharedObjectResult, error) {
	if req.SharedObjectID == "" || req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return model.AttachSharedObjectResult{}, fmt.Errorf("%w: shared object id, bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	shared, ok := r.sharedObjects[req.SharedObjectID]
	if !ok {
		return model.AttachSharedObjectResult{}, meta.ErrNotFound
	}
	if shared.BucketID != req.BucketID || shared.Key != req.Key {
		return model.AttachSharedObjectResult{}, fmt.Errorf("%w: shared object scope mismatch", meta.ErrInvalidArgument)
	}
	key := versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}
	version, ok := r.versions[key]
	if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
		return model.AttachSharedObjectResult{}, meta.ErrNotFound
	}
	if objectVersionProtectedByObjectLock(version, r.now(), false) {
		return model.AttachSharedObjectResult{}, meta.ErrObjectLocked
	}
	if !objectVersionMatchesSharedObject(version, shared) {
		return model.AttachSharedObjectResult{}, fmt.Errorf("%w: object version does not match shared object digest/size", meta.ErrInvalidArgument)
	}
	now := r.now()
	previousRefs := objectSegmentRefsFromVersion(version)
	sharedRefs := cloneSegmentRefs(shared.SegmentRefs)
	version.SegmentRefs = sharedRefs
	version.SegmentRef = firstSegmentRef(sharedRefs)
	version.StorageClass = cloneStorageClass(shared.StorageClass)
	r.versions[key] = cloneVersion(version)
	ref := sharedObjectRefFromVersion(shared, version, now)
	if _, exists := r.sharedRefs[ref.RefID]; !exists {
		shared.RefCount++
	}
	shared.ProtectedRootCount = r.sharedObjectProtectedRootCountLocked(shared, now)
	shared.UpdatedAt = now
	r.sharedObjects[shared.SharedObjectID] = cloneSharedObject(shared)
	r.sharedRefs[ref.RefID] = cloneSharedObjectRef(ref)
	headKey := objectKey{bucketID: req.BucketID, key: req.Key}
	if head, ok := r.heads[headKey]; ok && head.VersionID == version.VersionID {
		head.SegmentRefs = cloneSegmentRefs(version.SegmentRefs)
		head.SegmentRef = cloneSegmentRef(version.SegmentRef)
		head.StorageClass = cloneStorageClass(version.StorageClass)
		head.LastModified = now
		r.setHeadLocked(headKey, head)
	}
	return model.AttachSharedObjectResult{
		Version:             cloneVersion(version),
		SharedObject:        cloneSharedObject(shared),
		Ref:                 cloneSharedObjectRef(ref),
		PreviousSegmentRefs: cloneSegmentRefs(previousRefs),
	}, nil
}

func (r *Repository) PublishObjectVersionRefs(_ context.Context, req meta.PublishObjectVersionRefsRequest) (meta.PublishObjectVersionRefsResult, error) {
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return meta.PublishObjectVersionRefsResult{}, fmt.Errorf("%w: bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	refs := cloneSegmentRefs(req.SegmentRefs)
	if len(refs) == 0 {
		return meta.PublishObjectVersionRefsResult{}, fmt.Errorf("%w: replacement segment refs are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}
	version, ok := r.versions[key]
	if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
		return meta.PublishObjectVersionRefsResult{}, meta.ErrNotFound
	}
	if objectVersionProtectedByObjectLock(version, r.now(), false) {
		return meta.PublishObjectVersionRefsResult{}, meta.ErrObjectLocked
	}
	previousRefs := objectSegmentRefsFromVersion(version)
	if !meta.SegmentRefsContainVolume(previousRefs, req.ExpectedSourceVolumeID) {
		return meta.PublishObjectVersionRefsResult{}, fmt.Errorf("%w: object version has no refs on source volume %q", meta.ErrInvalidArgument, req.ExpectedSourceVolumeID)
	}
	version.SegmentRefs = refs
	version.SegmentRef = firstSegmentRef(refs)
	r.versions[key] = cloneVersion(version)
	headKey := objectKey{bucketID: req.BucketID, key: req.Key}
	if head, ok := r.heads[headKey]; ok && head.VersionID == version.VersionID {
		head.SegmentRefs = cloneSegmentRefs(version.SegmentRefs)
		head.SegmentRef = cloneSegmentRef(version.SegmentRef)
		r.setHeadLocked(headKey, head)
	}
	return meta.PublishObjectVersionRefsResult{
		Version:             cloneVersion(version),
		PreviousSegmentRefs: cloneSegmentRefs(previousRefs),
	}, nil
}

func (r *Repository) ListSharedObjectRefs(_ context.Context, req meta.ListSharedObjectRefsRequest) ([]model.SharedObjectRef, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.sharedRefs) {
		limit = len(r.sharedRefs)
	}
	refs := make([]model.SharedObjectRef, 0, len(r.sharedRefs))
	for _, ref := range r.sharedRefs {
		if req.SharedObjectID != "" && ref.SharedObjectID != req.SharedObjectID {
			continue
		}
		if req.BucketID != "" && ref.BucketID != req.BucketID {
			continue
		}
		if req.Key != "" && ref.Key != req.Key {
			continue
		}
		if req.VersionID != "" && ref.VersionID != req.VersionID {
			continue
		}
		refs = append(refs, cloneSharedObjectRef(ref))
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].RefID < refs[j].RefID
	})
	if len(refs) > limit {
		refs = refs[:limit]
	}
	return refs, nil
}

func (r *Repository) RepairSharedObjectRefCounts(_ context.Context, req meta.RepairSharedObjectRefCountsRequest) (model.SharedObjectRepairResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	limit := req.Limit
	if limit <= 0 || limit > len(r.sharedObjects) {
		limit = len(r.sharedObjects)
	}
	now := r.now()
	ids := make([]string, 0, len(r.sharedObjects))
	for sharedObjectID := range r.sharedObjects {
		if req.SharedObjectID != "" && sharedObjectID != req.SharedObjectID {
			continue
		}
		ids = append(ids, sharedObjectID)
	}
	sort.Strings(ids)
	result := model.SharedObjectRepairResult{}
	for _, sharedObjectID := range ids {
		if result.Scanned >= limit {
			break
		}
		shared := r.sharedObjects[sharedObjectID]
		result.Scanned++
		refCount := 0
		for _, ref := range r.sharedRefs {
			if ref.SharedObjectID == sharedObjectID {
				refCount++
			}
		}
		protectedRootCount := r.sharedObjectProtectedRootCountLocked(shared, now)
		if shared.RefCount == refCount && shared.ProtectedRootCount == protectedRootCount {
			continue
		}
		shared.RefCount = refCount
		shared.ProtectedRootCount = protectedRootCount
		shared.UpdatedAt = now
		r.sharedObjects[sharedObjectID] = cloneSharedObject(shared)
		result.Updated++
	}
	return result, nil
}

func (r *Repository) RepairListIndexes(ctx context.Context, req meta.RepairListIndexesRequest) (model.ListIndexRepairResult, error) {
	if err := ctx.Err(); err != nil {
		return model.ListIndexRepairResult{}, err
	}
	if req.BucketID == "" {
		return model.ListIndexRepairResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[req.BucketID]; !ok {
		return model.ListIndexRepairResult{}, meta.ErrNotFound
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	result := model.ListIndexRepairResult{}
	headKeys := make([]objectKey, 0)
	for key := range r.heads {
		if key.bucketID == req.BucketID {
			headKeys = append(headKeys, key)
		}
	}
	sort.Slice(headKeys, func(i, j int) bool {
		return headKeys[i].key < headKeys[j].key
	})
	for range headKeys {
		if result.ScannedObjectHeads >= limit {
			break
		}
		result.ScannedObjectHeads++
	}
	uploadKeys := make([]uploadKey, 0)
	for key := range r.uploads {
		if key.bucketID == req.BucketID {
			uploadKeys = append(uploadKeys, key)
		}
	}
	sort.Slice(uploadKeys, func(i, j int) bool {
		if uploadKeys[i].key == uploadKeys[j].key {
			return uploadKeys[i].uploadID < uploadKeys[j].uploadID
		}
		return uploadKeys[i].key < uploadKeys[j].key
	})
	for range uploadKeys {
		if result.ScannedMultipartUploads >= limit {
			break
		}
		result.ScannedMultipartUploads++
	}
	return result, nil
}

func (r *Repository) CreateMultipartUpload(_ context.Context, req meta.CreateMultipartUploadRequest) (model.MultipartUpload, error) {
	if req.BucketID == "" || req.Key == "" {
		return model.MultipartUpload{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.ObjectLockRetention, req.ObjectLockLegalHold); err != nil {
		return model.MultipartUpload{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	bucket, ok := r.bucketsByID[req.BucketID]
	if !ok {
		return model.MultipartUpload{}, meta.ErrNotFound
	}
	if err := r.checkTenantActiveUploadQuotaLocked(bucket.TenantID); err != nil {
		return model.MultipartUpload{}, err
	}
	now := r.now()
	uploadID, err := r.nextMultipartUploadIDLocked(req.BucketID)
	if err != nil {
		return model.MultipartUpload{}, err
	}
	upload := model.MultipartUpload{
		UploadID:             uploadID,
		BucketID:             req.BucketID,
		Key:                  req.Key,
		ContentType:          req.ContentType,
		StorageClass:         cloneStorageClass(req.StorageClass),
		ServerSideEncryption: req.ServerSideEncryption,
		UserMetadata:         cloneStringMap(req.UserMetadata),
		Tags:                 cloneStringMap(req.Tags),
		ObjectLockRetention:  req.ObjectLockRetention,
		ObjectLockLegalHold:  req.ObjectLockLegalHold,
		State:                model.MultipartUploadActive,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	r.uploads[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: upload.UploadID}] = upload
	r.applyTenantActiveUploadDeltaLocked(bucket.TenantID, 1, now)
	return cloneUpload(upload), nil
}

func (r *Repository) GetMultipartUpload(_ context.Context, req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	upload, err := r.getUploadLocked(req)
	if err != nil {
		return model.MultipartUpload{}, err
	}
	return cloneUpload(upload), nil
}

func (r *Repository) ListMultipartUploads(_ context.Context, req meta.ListMultipartUploadsRequest) (model.ListMultipartUploadsResult, error) {
	if req.BucketID == "" {
		return model.ListMultipartUploadsResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	maxUploads := req.MaxUploads
	if maxUploads <= 0 {
		maxUploads = 1000
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.bucketsByID[req.BucketID]; !ok {
		return model.ListMultipartUploadsResult{}, meta.ErrNotFound
	}
	entries := r.multipartUploadEntries(req)
	if req.KeyMarker != "" {
		entries = multipartEntriesAfter(entries, req.KeyMarker, req.UploadIDMarker)
	}
	result := model.ListMultipartUploadsResult{}
	if len(entries) > maxUploads {
		result.IsTruncated = true
		result.NextKeyMarker = entries[maxUploads-1].name
		result.NextUploadIDMarker = entries[maxUploads-1].uploadID
		entries = entries[:maxUploads]
	}
	for _, entry := range entries {
		if entry.prefix {
			result.CommonPrefixes = append(result.CommonPrefixes, entry.name)
			continue
		}
		result.Uploads = append(result.Uploads, cloneUpload(entry.upload))
	}
	return result, nil
}

func (r *Repository) PutMultipartPart(_ context.Context, req meta.PutMultipartPartRequest) (model.MultipartPart, *model.MultipartPart, error) {
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" || req.PartNumber < 1 || req.PartNumber > meta.MaxMultipartParts {
		return model.MultipartPart{}, nil, fmt.Errorf("%w: multipart part fields are invalid", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	uploadReq := meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID}
	upload, err := r.getActiveUploadLocked(uploadReq)
	if err != nil {
		return model.MultipartPart{}, nil, err
	}
	key := partKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID, partNumber: req.PartNumber}
	var previous *model.MultipartPart
	if old, ok := r.parts[key]; ok {
		oldCopy := clonePart(old)
		previous = &oldCopy
	}
	now := r.now()
	part := model.MultipartPart{
		UploadID:   req.UploadID,
		PartNumber: req.PartNumber,
		SizeBytes:  req.SizeBytes,
		ETag:       req.ETag,
		SegmentRef: cloneSegmentRef(req.SegmentRef),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if previous != nil {
		part.CreatedAt = previous.CreatedAt
	}
	r.parts[key] = part
	meta.ApplyMultipartPartSummary(&upload, part, previous, now)
	r.uploads[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}] = upload
	return clonePart(part), previous, nil
}

func (r *Repository) ListMultipartParts(_ context.Context, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getActiveUploadLocked(req); err != nil {
		return nil, err
	}
	parts := r.listPartsLocked(req)
	return parts, nil
}

func (r *Repository) GetMultipartParts(_ context.Context, req meta.GetMultipartPartsRequest) ([]model.MultipartPart, error) {
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" {
		return nil, fmt.Errorf("%w: multipart upload fields are required", meta.ErrInvalidArgument)
	}
	if err := meta.ValidateMultipartPartNumberSelection(req.PartNumbers); err != nil {
		return nil, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.getActiveUploadLocked(meta.MultipartUploadRequest{
		BucketID: req.BucketID,
		Key:      req.Key,
		UploadID: req.UploadID,
	}); err != nil {
		return nil, err
	}
	parts := make([]model.MultipartPart, 0, len(req.PartNumbers))
	for _, partNumber := range req.PartNumbers {
		key := partKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID, partNumber: partNumber}
		part, ok := r.parts[key]
		if !ok {
			continue
		}
		parts = append(parts, clonePart(part))
	}
	return parts, nil
}

func (r *Repository) GetMultipartCompletion(_ context.Context, req meta.MultipartUploadRequest) (model.MultipartCompletionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.completions[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}]
	if !ok {
		return model.MultipartCompletionRecord{}, meta.ErrNotFound
	}
	return cloneMultipartCompletionRecord(record), nil
}

func (r *Repository) PrepareMultipartCompletion(_ context.Context, req meta.PrepareMultipartCompletionRequest) (model.MultipartCompletionRecord, error) {
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" || req.ObjectVersionID == "" || req.ETag == "" || req.PartCount < 1 {
		return model.MultipartCompletionRecord{}, fmt.Errorf("%w: multipart completion fields are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}
	if existing, ok := r.completions[key]; ok {
		if !multipartCompletionMatches(existing, req) {
			return model.MultipartCompletionRecord{}, meta.ErrCASConflict
		}
		return cloneMultipartCompletionRecord(existing), nil
	}
	upload, err := r.getUploadLocked(meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID})
	if err != nil {
		return model.MultipartCompletionRecord{}, err
	}
	if upload.State != model.MultipartUploadActive {
		return model.MultipartCompletionRecord{}, meta.ErrNotFound
	}
	now := r.now()
	record := model.MultipartCompletionRecord{
		BucketID:              req.BucketID,
		Key:                   req.Key,
		UploadID:              req.UploadID,
		ObjectVersionID:       req.ObjectVersionID,
		ExpectedHeadVersionID: req.ExpectedHeadVersionID,
		ETag:                  req.ETag,
		SizeBytes:             req.SizeBytes,
		PartCount:             req.PartCount,
		State:                 model.MultipartCompletionPrepared,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	r.completions[key] = record
	return cloneMultipartCompletionRecord(record), nil
}

func (r *Repository) MarkMultipartCompletionPublished(_ context.Context, req meta.MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error) {
	return r.markMultipartCompletionState(req, model.MultipartCompletionPublished)
}

func (r *Repository) MarkMultipartCompletionCompleted(_ context.Context, req meta.MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error) {
	return r.markMultipartCompletionState(req, model.MultipartCompletionCompleted)
}

func (r *Repository) markMultipartCompletionState(req meta.MultipartCompletionStateRequest, state model.MultipartCompletionState) (model.MultipartCompletionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}
	record, ok := r.completions[key]
	if !ok {
		return model.MultipartCompletionRecord{}, meta.ErrNotFound
	}
	if !multipartCompletionStateAtLeast(record.State, state) {
		record.State = state
		record.UpdatedAt = r.now()
		r.completions[key] = record
	}
	return cloneMultipartCompletionRecord(record), nil
}

func (r *Repository) CompleteMultipartUpload(_ context.Context, req meta.CompleteMultipartUploadRequest) (model.MultipartUpload, error) {
	if req.ObjectVersionID == "" || req.ETag == "" {
		return model.MultipartUpload{}, fmt.Errorf("%w: completed object version and etag are required", meta.ErrInvalidArgument)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	upload, err := r.getUploadLocked(meta.MultipartUploadRequest{
		BucketID: req.BucketID,
		Key:      req.Key,
		UploadID: req.UploadID,
	})
	if err != nil {
		return model.MultipartUpload{}, err
	}
	switch upload.State {
	case model.MultipartUploadCompleted:
		return cloneUpload(upload), nil
	case model.MultipartUploadAborted:
		return model.MultipartUpload{}, meta.ErrNotFound
	case model.MultipartUploadActive:
		if err := meta.ValidateCompletedMultipartPartCount(req.PartCount); err != nil {
			return model.MultipartUpload{}, err
		}
	default:
		return model.MultipartUpload{}, meta.ErrNotFound
	}
	now := r.now()
	upload.State = model.MultipartUploadCompleted
	upload.CompletedVersionID = req.ObjectVersionID
	upload.CompletedETag = req.ETag
	upload.CompletedSizeBytes = req.SizeBytes
	upload.CompletedPartCount = req.PartCount
	upload.CompletedAt = now
	upload.PartsCleanupState = model.MultipartPartsCleanupPending
	upload.PartsCleanupDeleted = 0
	upload.PartsCleanupUpdatedAt = now
	upload.UpdatedAt = now
	r.uploads[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}] = upload
	r.applyTenantActiveUploadDeltaForBucketLocked(req.BucketID, -1, now)
	return cloneUpload(upload), nil
}

func (r *Repository) AbortMultipartUpload(_ context.Context, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	upload, err := r.getUploadLocked(req)
	if err != nil {
		return nil, err
	}
	switch upload.State {
	case model.MultipartUploadAborted:
		return r.listPartsLocked(req), nil
	case model.MultipartUploadCompleted:
		return nil, meta.ErrNotFound
	case model.MultipartUploadActive:
	default:
		return nil, meta.ErrNotFound
	}
	parts := r.listPartsLocked(req)
	upload.State = model.MultipartUploadAborted
	now := r.now()
	upload.PartsCleanupState = model.MultipartPartsCleanupPending
	upload.PartsCleanupDeleted = 0
	upload.PartsCleanupUpdatedAt = now
	upload.UpdatedAt = now
	r.uploads[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}] = upload
	r.applyTenantActiveUploadDeltaForBucketLocked(req.BucketID, -1, now)
	return parts, nil
}

func (r *Repository) CleanupMultipartUploadParts(_ context.Context, req meta.CleanupMultipartUploadPartsRequest) (meta.CleanupMultipartUploadPartsResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	uploadReq := meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID}
	upload, err := r.getUploadLocked(uploadReq)
	if err != nil {
		return meta.CleanupMultipartUploadPartsResult{}, err
	}
	switch upload.State {
	case model.MultipartUploadCompleted, model.MultipartUploadAborted:
	case model.MultipartUploadActive:
		return meta.CleanupMultipartUploadPartsResult{}, fmt.Errorf("%w: active multipart upload parts cannot be cleaned", meta.ErrInvalidArgument)
	default:
		return meta.CleanupMultipartUploadPartsResult{}, meta.ErrNotFound
	}
	limit := meta.NormalizeMultipartCleanupLimit(req.Limit)
	keys := r.partKeysForCleanupLocked(uploadReq, limit+1)
	hasMore := len(keys) > limit
	if hasMore {
		keys = keys[:limit]
	}
	for _, key := range keys {
		delete(r.parts, key)
	}
	now := r.now()
	upload.PartsCleanupDeleted += len(keys)
	upload.PartsCleanupUpdatedAt = now
	upload.UpdatedAt = now
	if hasMore {
		upload.PartsCleanupState = model.MultipartPartsCleanupPending
	} else {
		upload.PartsCleanupState = model.MultipartPartsCleanupDone
	}
	r.uploads[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}] = upload
	return meta.CleanupMultipartUploadPartsResult{
		Upload:       cloneUpload(upload),
		DeletedParts: len(keys),
		HasMore:      hasMore,
	}, nil
}

func (r *Repository) getActiveUploadLocked(req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	upload, err := r.getUploadLocked(req)
	if err != nil {
		return model.MultipartUpload{}, err
	}
	if upload.State != model.MultipartUploadActive {
		return model.MultipartUpload{}, meta.ErrNotFound
	}
	return upload, nil
}

func (r *Repository) getUploadLocked(req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" {
		return model.MultipartUpload{}, fmt.Errorf("%w: multipart upload fields are required", meta.ErrInvalidArgument)
	}
	upload, ok := r.uploads[uploadKey{bucketID: req.BucketID, key: req.Key, uploadID: req.UploadID}]
	if !ok {
		return model.MultipartUpload{}, meta.ErrNotFound
	}
	return upload, nil
}

func (r *Repository) listPartsLocked(req meta.MultipartUploadRequest) []model.MultipartPart {
	parts := make([]model.MultipartPart, 0)
	for key, part := range r.parts {
		if key.bucketID == req.BucketID && key.key == req.Key && key.uploadID == req.UploadID {
			parts = append(parts, clonePart(part))
		}
	}
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})
	return parts
}

func (r *Repository) partKeysForCleanupLocked(req meta.MultipartUploadRequest, limit int) []partKey {
	keys := make([]partKey, 0)
	for key := range r.parts {
		if key.bucketID == req.BucketID && key.key == req.Key && key.uploadID == req.UploadID {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].partNumber < keys[j].partNumber
	})
	if limit > 0 && len(keys) > limit {
		return keys[:limit]
	}
	return keys
}

func (r *Repository) objectLockTargetLocked(bucketID, key, versionID string) (versionKey, model.ObjectVersion, error) {
	if versionID == "" {
		head, ok := r.heads[objectKey{bucketID: bucketID, key: key}]
		if !ok || head.DeleteMarker {
			return versionKey{}, model.ObjectVersion{}, meta.ErrNotFound
		}
		versionID = head.VersionID
	}
	vkey := versionKey{bucketID: bucketID, key: key, versionID: versionID}
	version, ok := r.versions[vkey]
	if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
		return versionKey{}, model.ObjectVersion{}, meta.ErrNotFound
	}
	return vkey, version, nil
}

func (r *Repository) updateHeadObjectLockLocked(version model.ObjectVersion) {
	headKey := objectKey{bucketID: version.BucketID, key: version.Key}
	head, ok := r.heads[headKey]
	if !ok || head.VersionID != version.VersionID {
		return
	}
	head.ObjectLockRetention = version.ObjectLockRetention
	head.ObjectLockLegalHold = version.ObjectLockLegalHold
	r.setHeadLocked(headKey, head)
}

func (r *Repository) deleteObjectVersionLocked(req meta.DeleteObjectRequest) (model.DeleteResult, error) {
	vkey := versionKey{bucketID: req.BucketID, key: req.Key, versionID: req.VersionID}
	deleted, ok := r.versions[vkey]
	if !ok {
		return model.DeleteResult{}, meta.ErrNotFound
	}
	if err := r.checkDeleteKMSAdmissionLocked(deleted.ServerSideEncryption); err != nil {
		return model.DeleteResult{}, err
	}
	now := r.now()
	if objectVersionProtectedByObjectLock(deleted, now, false) {
		if objectVersionProtectedByObjectLock(deleted, now, true) || !req.BypassGovernanceRetention || !validGovernanceBypassAudit(req.BypassAudit) {
			return model.DeleteResult{}, meta.ErrObjectLocked
		}
		r.appendAuditEventLocked(governanceBypassAuditEvent(req, model.AuditActionGovernanceBypassDeleteObject, deleted.VersionID, map[string]string{
			"target": "object_version",
		}))
	}
	delete(r.versions, vkey)
	r.deleteProtectedRefsForVersionLocked(deleted.BucketID, deleted.Key, deleted.VersionID)
	headKey := objectKey{bucketID: req.BucketID, key: req.Key}
	if head, ok := r.heads[headKey]; ok && head.VersionID == req.VersionID {
		if promoted, ok := r.latestCommittedVersionLocked(req.BucketID, req.Key); ok {
			r.setHeadLocked(headKey, headFromVersion(promoted, promoted.CommittedAt))
		} else {
			delete(r.heads, headKey)
		}
	}
	return model.DeleteResult{
		Deleted:          true,
		DeletedVersionID: deleted.VersionID,
		DeleteMarker:     deleted.DeleteMarker,
		DeletedVersion:   cloneVersion(deleted),
	}, nil
}

func (r *Repository) checkDeleteKMSAdmissionLocked(encryption model.ServerSideEncryption) error {
	if encryption.Algorithm != model.ServerSideEncryptionAWSKMS {
		return nil
	}
	keyID := strings.TrimSpace(encryption.KeyID)
	if keyID == "" {
		return nil
	}
	key, ok := r.kmsKeys[keyID]
	if !ok {
		return nil
	}
	if !model.KMSKeyAllowsDelete(key.State) {
		return fmt.Errorf("%w: kms key %q state %q does not allow delete", meta.ErrKMSKeyUnavailable, key.KeyID, key.State)
	}
	return nil
}

func (r *Repository) createDeleteMarkerLocked(req meta.DeleteObjectRequest) (model.DeleteResult, error) {
	now := r.now()
	versionID, err := r.nextObjectVersionIDLocked(req.BucketID, req.Key)
	if err != nil {
		return model.DeleteResult{}, err
	}
	version := model.ObjectVersion{
		BucketID:       req.BucketID,
		Key:            req.Key,
		VersionID:      versionID,
		VersionSortKey: versionID,
		State:          model.ObjectVersionCommitted,
		CreatedAt:      now,
		CommittedAt:    now,
		DeleteMarker:   true,
	}
	r.versions[versionKey{bucketID: req.BucketID, key: req.Key, versionID: versionID}] = version
	r.setHeadLocked(objectKey{bucketID: req.BucketID, key: req.Key}, headFromVersion(version, now))
	return model.DeleteResult{
		Deleted:          true,
		DeletedVersionID: versionID,
		DeleteMarker:     true,
		DeletedVersion:   cloneVersion(version),
	}, nil
}

func (r *Repository) deleteVersionRecordLocked(bucketID, key, versionID string) model.ObjectVersion {
	if versionID == "" {
		return model.ObjectVersion{}
	}
	vkey := versionKey{bucketID: bucketID, key: key, versionID: versionID}
	version, ok := r.versions[vkey]
	if !ok {
		return model.ObjectVersion{}
	}
	delete(r.versions, vkey)
	return version
}

func (r *Repository) setHeadLocked(key objectKey, head model.ObjectHead) model.ObjectHead {
	previous, found := r.heads[key]
	head.Revision = nextObjectHeadRevision(previous, found)
	r.heads[key] = cloneHead(head)
	return head
}

func nextObjectHeadRevision(previous model.ObjectHead, found bool) uint64 {
	if !found || previous.Revision == 0 {
		return 1
	}
	return previous.Revision + 1
}

func (r *Repository) nextObjectVersionIDLocked(bucketID, key string) (string, error) {
	for attempt := 0; attempt < idCollisionRetryLimit; attempt++ {
		versionID, err := r.ids.NewID(metaid.KindVersion)
		if err != nil {
			return "", fmt.Errorf("%w: generate object version id: %v", meta.ErrUnavailable, err)
		}
		if _, ok := r.versions[versionKey{bucketID: bucketID, key: key, versionID: versionID}]; ok {
			continue
		}
		return versionID, nil
	}
	return "", fmt.Errorf("%w: object version id collision retry budget exhausted", meta.ErrUnavailable)
}

func (r *Repository) nextMultipartUploadIDLocked(bucketID string) (string, error) {
	for attempt := 0; attempt < idCollisionRetryLimit; attempt++ {
		uploadID, err := r.ids.NewID(metaid.KindUpload)
		if err != nil {
			return "", fmt.Errorf("%w: generate multipart upload id: %v", meta.ErrUnavailable, err)
		}
		if r.multipartUploadIDExistsLocked(bucketID, uploadID) {
			continue
		}
		return uploadID, nil
	}
	return "", fmt.Errorf("%w: multipart upload id collision retry budget exhausted", meta.ErrUnavailable)
}

func (r *Repository) multipartUploadIDExistsLocked(bucketID, uploadID string) bool {
	for key := range r.uploads {
		if key.bucketID == bucketID && key.uploadID == uploadID {
			return true
		}
	}
	return false
}

func (r *Repository) latestCommittedVersionLocked(bucketID, key string) (model.ObjectVersion, bool) {
	var latest model.ObjectVersion
	for vkey, version := range r.versions {
		if vkey.bucketID != bucketID || vkey.key != key || version.State != model.ObjectVersionCommitted {
			continue
		}
		if latest.VersionID == "" || version.VersionSortKey > latest.VersionSortKey {
			latest = version
		}
	}
	return latest, latest.VersionID != ""
}

type multipartUploadEntry struct {
	name     string
	uploadID string
	prefix   bool
	upload   model.MultipartUpload
}

func (r *Repository) multipartUploadEntries(req meta.ListMultipartUploadsRequest) []multipartUploadEntry {
	var entries []multipartUploadEntry
	commonPrefixes := make(map[string]struct{})
	for key, upload := range r.uploads {
		if key.bucketID != req.BucketID || upload.State != model.MultipartUploadActive || !strings.HasPrefix(key.key, req.Prefix) {
			continue
		}
		if req.Delimiter != "" {
			rest := strings.TrimPrefix(key.key, req.Prefix)
			if index := strings.Index(rest, req.Delimiter); index >= 0 {
				commonPrefix := req.Prefix + rest[:index+len(req.Delimiter)]
				if _, ok := commonPrefixes[commonPrefix]; !ok {
					commonPrefixes[commonPrefix] = struct{}{}
					entries = append(entries, multipartUploadEntry{name: commonPrefix, prefix: true})
				}
				continue
			}
		}
		entries = append(entries, multipartUploadEntry{name: key.key, uploadID: key.uploadID, upload: upload})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name == entries[j].name {
			if entries[i].prefix != entries[j].prefix {
				return entries[i].prefix
			}
			return entries[i].uploadID < entries[j].uploadID
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func multipartEntriesAfter(entries []multipartUploadEntry, keyMarker, uploadIDMarker string) []multipartUploadEntry {
	index := sort.Search(len(entries), func(i int) bool {
		if entries[i].name != keyMarker {
			return entries[i].name > keyMarker
		}
		if uploadIDMarker == "" {
			return entries[i].name > keyMarker
		}
		return entries[i].uploadID > uploadIDMarker
	})
	return entries[index:]
}

type listEntry struct {
	name   string
	prefix bool
	head   model.ObjectHead
}

func (r *Repository) listEntries(req meta.ListObjectsRequest) []listEntry {
	var entries []listEntry
	commonPrefixes := make(map[string]struct{})
	for key, head := range r.heads {
		if key.bucketID != req.BucketID || head.DeleteMarker || !strings.HasPrefix(key.key, req.Prefix) {
			continue
		}
		if req.Delimiter != "" {
			rest := strings.TrimPrefix(key.key, req.Prefix)
			if index := strings.Index(rest, req.Delimiter); index >= 0 {
				commonPrefix := req.Prefix + rest[:index+len(req.Delimiter)]
				if _, ok := commonPrefixes[commonPrefix]; !ok {
					commonPrefixes[commonPrefix] = struct{}{}
					entries = append(entries, listEntry{name: commonPrefix, prefix: true})
				}
				continue
			}
		}
		entries = append(entries, listEntry{name: key.key, head: head})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name == entries[j].name {
			return !entries[i].prefix && entries[j].prefix
		}
		return entries[i].name < entries[j].name
	})
	return entries
}

func entriesAfter(entries []listEntry, token string) []listEntry {
	index := sort.Search(len(entries), func(i int) bool {
		return entries[i].name > token
	})
	return entries[index:]
}

type versionListEntry struct {
	name      string
	versionID string
	sortKey   string
	prefix    bool
	isLatest  bool
	version   model.ObjectVersion
}

func (r *Repository) versionEntries(req meta.ListObjectVersionsRequest) []versionListEntry {
	var entries []versionListEntry
	commonPrefixes := make(map[string]struct{})
	for vkey, version := range r.versions {
		if vkey.bucketID != req.BucketID || version.State != model.ObjectVersionCommitted || !strings.HasPrefix(vkey.key, req.Prefix) {
			continue
		}
		if req.Delimiter != "" {
			rest := strings.TrimPrefix(vkey.key, req.Prefix)
			if index := strings.Index(rest, req.Delimiter); index >= 0 {
				commonPrefix := req.Prefix + rest[:index+len(req.Delimiter)]
				if _, ok := commonPrefixes[commonPrefix]; !ok {
					commonPrefixes[commonPrefix] = struct{}{}
					entries = append(entries, versionListEntry{name: commonPrefix, prefix: true})
				}
				continue
			}
		}
		head := r.heads[objectKey{bucketID: vkey.bucketID, key: vkey.key}]
		entries = append(entries, versionListEntry{
			name:      vkey.key,
			versionID: version.VersionID,
			sortKey:   version.VersionSortKey,
			isLatest:  head.VersionID == version.VersionID,
			version:   version,
		})
	}
	sortVersionEntries(entries)
	return entries
}

func sortVersionEntries(entries []versionListEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name == entries[j].name {
			if entries[i].prefix != entries[j].prefix {
				return entries[i].prefix
			}
			return entries[i].sortKey > entries[j].sortKey
		}
		return entries[i].name < entries[j].name
	})
}

func versionEntriesAfter(entries []versionListEntry, keyMarker, versionIDMarker string) []versionListEntry {
	index := sort.Search(len(entries), func(i int) bool {
		if entries[i].name != keyMarker {
			return entries[i].name > keyMarker
		}
		if versionIDMarker == "" {
			return entries[i].name > keyMarker
		}
		return entries[i].versionID == versionIDMarker
	})
	if index < len(entries) && entries[index].name == keyMarker && entries[index].versionID == versionIDMarker {
		index++
	}
	return entries[index:]
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAuditEvent(in model.AuditEvent) model.AuditEvent {
	out := in
	out.Principal.Groups = cloneStringSlice(in.Principal.Groups)
	out.Principal.Roles = cloneStringSlice(in.Principal.Roles)
	out.Details = cloneStringMap(in.Details)
	return out
}

func cloneAccessKey(in model.AccessKey) model.AccessKey {
	out := in
	out.Permissions = cloneStringSlice(in.Permissions)
	return out
}

func cloneBucket(in model.Bucket) model.Bucket {
	out := in
	out.CORSRules = cloneCORSRules(in.CORSRules)
	out.Lifecycle = cloneLifecycleConfiguration(in.Lifecycle)
	out.Policy = clonePolicy(in.Policy)
	return out
}

func clonePolicy(in auth.PolicyDocument) auth.PolicyDocument {
	out := in
	if len(in.Statements) > 0 {
		out.Statements = make([]auth.PolicyStatement, 0, len(in.Statements))
		for _, statement := range in.Statements {
			out.Statements = append(out.Statements, clonePolicyStatement(statement))
		}
	}
	return out
}

func clonePolicyStatement(in auth.PolicyStatement) auth.PolicyStatement {
	out := in
	out.Principals = cloneStringSlice(in.Principals)
	out.Actions = cloneStringSlice(in.Actions)
	out.Resources = cloneStringSlice(in.Resources)
	return out
}

func cloneProtectedRef(in model.ProtectedRef) model.ProtectedRef {
	out := in
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	return out
}

func cloneGCOperationRecord(in model.GCOperationRecord) model.GCOperationRecord {
	out := in
	out.Attempts = cloneGCOperationAttempts(in.Attempts)
	return out
}

func cloneGCOperationAttempts(in []model.GCOperationAttempt) []model.GCOperationAttempt {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.GCOperationAttempt, len(in))
	copy(out, in)
	return out
}

func cloneDedupeOperationRecord(in model.DedupeOperationRecord) model.DedupeOperationRecord {
	out := in
	out.Attempts = cloneDedupeOperationAttempts(in.Attempts)
	return out
}

func cloneDedupeOperationAttempts(in []model.DedupeOperationAttempt) []model.DedupeOperationAttempt {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.DedupeOperationAttempt, len(in))
	copy(out, in)
	return out
}

func cloneDedupeOperationLock(in model.DedupeOperationLock) model.DedupeOperationLock {
	return in
}

func cloneSharedObjectRelease(in model.SharedObjectRelease) model.SharedObjectRelease {
	out := in
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	return out
}

func cloneSharedObject(in model.SharedObject) model.SharedObject {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	return out
}

func cloneSharedObjectRef(in model.SharedObjectRef) model.SharedObjectRef {
	out := in
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	return out
}

func sharedObjectRefFromVersion(shared model.SharedObject, version model.ObjectVersion, now time.Time) model.SharedObjectRef {
	return model.SharedObjectRef{
		RefID:          sharedObjectRefID(shared.SharedObjectID, version.BucketID, version.Key, version.VersionID),
		SharedObjectID: shared.SharedObjectID,
		BucketID:       version.BucketID,
		Key:            version.Key,
		VersionID:      version.VersionID,
		SegmentRefs:    cloneSegmentRefs(shared.SegmentRefs),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func objectVersionMatchesSharedObject(version model.ObjectVersion, shared model.SharedObject) bool {
	if version.SizeBytes != shared.SizeBytes {
		return false
	}
	digest, ok := objectVersionDigest(version)
	if !ok {
		return false
	}
	return strings.EqualFold(digest.Algorithm, shared.Digest.Algorithm) && strings.EqualFold(digest.Hex, shared.Digest.Hex)
}

func (r *Repository) sharedObjectProtectedRootCountLocked(shared model.SharedObject, now time.Time) int {
	active := make(map[string]struct{})
	for _, segmentRef := range shared.SegmentRefs {
		if segmentRef.SegmentID == "" {
			continue
		}
		for _, ref := range r.protectedRefs {
			if ref.SegmentID != segmentRef.SegmentID || !protectedRefActive(ref, now) {
				continue
			}
			active[ref.RefID] = struct{}{}
		}
	}
	return len(active)
}

func sharedObjectFromVersion(bucket model.Bucket, version model.ObjectVersion, now time.Time) (model.SharedObject, error) {
	refs := objectSegmentRefsFromVersion(version)
	if len(refs) == 0 {
		return model.SharedObject{}, fmt.Errorf("%w: source version has no segment refs", meta.ErrInvalidArgument)
	}
	digest, ok := objectVersionDigest(version)
	if !ok {
		return model.SharedObject{}, fmt.Errorf("%w: source version has no stable object digest", meta.ErrInvalidArgument)
	}
	sharedObjectID := sharedObjectID(bucket.TenantID, version.BucketID, version.Key, digest, version.SizeBytes)
	for i := range refs {
		refs[i].SharedObjectID = sharedObjectID
	}
	return model.SharedObject{
		SharedObjectID:     sharedObjectID,
		TenantID:           bucket.TenantID,
		BucketID:           version.BucketID,
		Key:                version.Key,
		SourceVersionID:    version.VersionID,
		SizeBytes:          version.SizeBytes,
		Digest:             digest,
		StorageClass:       cloneStorageClass(version.StorageClass),
		SegmentRefs:        refs,
		RefCount:           1,
		ProtectedRootCount: 0,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

func objectVersionDigest(version model.ObjectVersion) (storage.Digest, bool) {
	if version.SegmentRef.Digest.Algorithm != "" && version.SegmentRef.Digest.Hex != "" {
		return version.SegmentRef.Digest, true
	}
	if len(version.SegmentRefs) == 1 && version.SegmentRefs[0].Digest.Algorithm != "" && version.SegmentRefs[0].Digest.Hex != "" {
		return version.SegmentRefs[0].Digest, true
	}
	return storage.Digest{}, false
}

func sharedObjectID(tenantID, bucketID, key string, digest storage.Digest, sizeBytes int64) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + bucketID + "\x00" + key + "\x00" + strings.ToLower(digest.Algorithm) + "\x00" + strings.ToLower(digest.Hex) + "\x00" + strconv.FormatInt(sizeBytes, 10)))
	return "so-" + hex.EncodeToString(sum[:16])
}

func sharedObjectRefID(sharedObjectID, bucketID, key, versionID string) string {
	sum := sha256.Sum256([]byte(sharedObjectID + "\x00" + bucketID + "\x00" + key + "\x00" + versionID))
	return "soref-" + hex.EncodeToString(sum[:16])
}

func normalizeGCOperationStatus(status model.GCOperationStatus, retryable int) model.GCOperationStatus {
	if status != "" {
		return status
	}
	if retryable > 0 {
		return model.GCOperationRetryPending
	}
	return model.GCOperationSucceeded
}

func normalizeDedupeOperationStatus(status model.DedupeOperationStatus, retryable int) model.DedupeOperationStatus {
	if status != "" {
		return status
	}
	if retryable > 0 {
		return model.DedupeOperationRetryPending
	}
	return model.DedupeOperationSucceeded
}

func normalizeSharedObjectReleaseStatus(status model.SharedObjectReleaseStatus) model.SharedObjectReleaseStatus {
	if status != "" {
		return status
	}
	return model.SharedObjectReleasePending
}

func sharedObjectReleaseID(sharedObjectID, segmentID string) string {
	return sharedObjectID + "/" + segmentID
}

func numericIDSuffix(id, prefix string) int {
	suffix := strings.TrimPrefix(id, prefix)
	if suffix == id || suffix == "" {
		return 0
	}
	value, err := strconv.Atoi(suffix)
	if err != nil {
		return 0
	}
	return value
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneCORSRules(in []model.CORSRule) []model.CORSRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.CORSRule, 0, len(in))
	for _, rule := range in {
		out = append(out, model.CORSRule{
			AllowedOrigins: cloneStringSlice(rule.AllowedOrigins),
			AllowedMethods: cloneStringSlice(rule.AllowedMethods),
			AllowedHeaders: cloneStringSlice(rule.AllowedHeaders),
			ExposeHeaders:  cloneStringSlice(rule.ExposeHeaders),
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		})
	}
	return out
}

func cloneLifecycleConfiguration(in model.BucketLifecycleConfiguration) model.BucketLifecycleConfiguration {
	if len(in.Rules) == 0 {
		return model.BucketLifecycleConfiguration{}
	}
	out := model.BucketLifecycleConfiguration{
		Rules: make([]model.LifecycleRule, len(in.Rules)),
	}
	copy(out.Rules, in.Rules)
	return out
}

func validateBucketLifecycleConfiguration(configuration model.BucketLifecycleConfiguration) error {
	if len(configuration.Rules) == 0 {
		return fmt.Errorf("%w: lifecycle configuration must include at least one rule", meta.ErrInvalidArgument)
	}
	if len(configuration.Rules) > 1000 {
		return fmt.Errorf("%w: lifecycle configuration cannot contain more than 1000 rules", meta.ErrInvalidArgument)
	}
	for _, rule := range configuration.Rules {
		switch rule.Status {
		case model.LifecycleRuleEnabled, model.LifecycleRuleDisabled:
		default:
			return fmt.Errorf("%w: lifecycle rule status must be Enabled or Disabled", meta.ErrInvalidArgument)
		}
		hasAction := false
		if rule.Expiration.Days > 0 || !rule.Expiration.Date.IsZero() || rule.Expiration.ExpiredObjectDeleteMarker {
			hasAction = true
		}
		if rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
			hasAction = true
		}
		if rule.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 {
			hasAction = true
		}
		if !hasAction {
			return fmt.Errorf("%w: lifecycle rule must include at least one action", meta.ErrInvalidArgument)
		}
		if rule.Expiration.Days < 0 || rule.NoncurrentVersionExpiration.NoncurrentDays < 0 || rule.AbortIncompleteMultipartUpload.DaysAfterInitiation < 0 {
			return fmt.Errorf("%w: lifecycle day values must be positive", meta.ErrInvalidArgument)
		}
	}
	return nil
}

func (r *Repository) appendAuditEventLocked(event model.AuditEvent) {
	r.nextAuditID++
	event.EventID = fmt.Sprintf("audit-%020d", r.nextAuditID)
	event.CreatedAt = r.now()
	event.PreviousHash = r.auditHeadHash
	event.EventHash = auditEventHash(event)
	r.auditHeadHash = event.EventHash
	r.auditEvents = append(r.auditEvents, cloneAuditEvent(event))
}

func validGovernanceBypassAudit(audit meta.AuditContext) bool {
	return strings.TrimSpace(audit.Reason) != "" && strings.TrimSpace(audit.Principal.AccessKeyID) != ""
}

func governanceBypassAuditEvent(req meta.DeleteObjectRequest, action model.AuditAction, versionID string, details map[string]string) model.AuditEvent {
	return model.AuditEvent{
		Action:    action,
		BucketID:  req.BucketID,
		Key:       req.Key,
		VersionID: versionID,
		RequestID: req.BypassAudit.RequestID,
		Reason:    strings.TrimSpace(req.BypassAudit.Reason),
		Principal: req.BypassAudit.Principal,
		Details:   cloneStringMap(details),
	}
}

func transitionAuditEvent(audit meta.AuditContext, action model.AuditAction, bucketID, key, versionID string, details map[string]string) model.AuditEvent {
	reason := strings.TrimSpace(audit.Reason)
	if reason == "" {
		reason = string(action)
	}
	return model.AuditEvent{
		Action:    action,
		BucketID:  bucketID,
		Key:       key,
		VersionID: versionID,
		RequestID: audit.RequestID,
		Reason:    reason,
		Principal: audit.Principal,
		Details:   cloneStringMap(details),
	}
}

func bucketObjectLockAuditDetails(config model.BucketObjectLockConfiguration) map[string]string {
	details := map[string]string{
		"enabled": strconv.FormatBool(config.Enabled),
	}
	if config.DefaultRetention.Mode != "" {
		details["default_retention_mode"] = string(config.DefaultRetention.Mode)
	}
	if config.DefaultRetention.Days > 0 {
		details["default_retention_days"] = strconv.Itoa(config.DefaultRetention.Days)
	}
	if config.DefaultRetention.Years > 0 {
		details["default_retention_years"] = strconv.Itoa(config.DefaultRetention.Years)
	}
	return details
}

func formatAuditTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func auditEventHash(event model.AuditEvent) string {
	payload := struct {
		EventID      string
		Action       model.AuditAction
		BucketID     string
		Key          string
		VersionID    string
		RequestID    string
		Reason       string
		Principal    model.AuditPrincipal
		Details      map[string]string
		PreviousHash string
		CreatedAt    string
	}{
		EventID:      event.EventID,
		Action:       event.Action,
		BucketID:     event.BucketID,
		Key:          event.Key,
		VersionID:    event.VersionID,
		RequestID:    event.RequestID,
		Reason:       event.Reason,
		Principal:    event.Principal,
		Details:      cloneStringMap(event.Details),
		PreviousHash: event.PreviousHash,
		CreatedAt:    event.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func (r *Repository) syncProtectedRefsForVersionLocked(version model.ObjectVersion, now time.Time) {
	r.deleteProtectedRefsForVersionLocked(version.BucketID, version.Key, version.VersionID)
	if !objectVersionProtectedByObjectLock(version, now, false) {
		return
	}
	for _, segmentRef := range objectSegmentRefsFromVersion(version) {
		if segmentRef.SegmentID == "" {
			continue
		}
		ref := protectedRefFromVersion(version, segmentRef, now)
		r.protectedRefs[ref.RefID] = ref
	}
}

func (r *Repository) deleteProtectedRefsForVersionLocked(bucketID, key, versionID string) {
	for refID, ref := range r.protectedRefs {
		if ref.BucketID == bucketID && ref.Key == key && ref.VersionID == versionID {
			delete(r.protectedRefs, refID)
		}
	}
}

func protectedRefFromVersion(version model.ObjectVersion, segmentRef storage.SegmentRef, now time.Time) model.ProtectedRef {
	return model.ProtectedRef{
		RefID:           protectedRefID(version, segmentRef),
		Reason:          model.ProtectedRefReasonObjectLock,
		BucketID:        version.BucketID,
		Key:             version.Key,
		VersionID:       version.VersionID,
		SegmentID:       segmentRef.SegmentID,
		SegmentRef:      cloneSegmentRef(segmentRef),
		RetentionMode:   version.ObjectLockRetention.Mode,
		RetainUntilDate: version.ObjectLockRetention.RetainUntilDate,
		LegalHold:       version.ObjectLockLegalHold,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func protectedRefID(version model.ObjectVersion, segmentRef storage.SegmentRef) string {
	sum := sha256.Sum256([]byte(version.BucketID + "\x00" + version.Key + "\x00" + version.VersionID + "\x00" + segmentRef.SegmentID))
	return hex.EncodeToString(sum[:])
}

func protectedRefActive(ref model.ProtectedRef, now time.Time) bool {
	if ref.LegalHold == model.ObjectLockLegalHoldOn {
		return true
	}
	if ref.RetentionMode == "" {
		return false
	}
	if ref.RetainUntilDate.IsZero() {
		return true
	}
	return now.UTC().Before(ref.RetainUntilDate.UTC())
}

func validateBucketObjectLockConfiguration(configuration model.BucketObjectLockConfiguration) error {
	if !configuration.Enabled {
		return fmt.Errorf("%w: object lock configuration must be enabled", meta.ErrInvalidArgument)
	}
	retention := configuration.DefaultRetention
	if retention.Mode == "" && retention.Days == 0 && retention.Years == 0 {
		return nil
	}
	if !isValidObjectLockMode(retention.Mode) {
		return fmt.Errorf("%w: object lock default retention mode is invalid", meta.ErrInvalidArgument)
	}
	if retention.Days <= 0 && retention.Years <= 0 {
		return fmt.Errorf("%w: object lock default retention period is required", meta.ErrInvalidArgument)
	}
	if retention.Days > 0 && retention.Years > 0 {
		return fmt.Errorf("%w: object lock default retention must use days or years", meta.ErrInvalidArgument)
	}
	return nil
}

func validateObjectLockState(retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus) error {
	if retention.Mode == "" {
		if !retention.RetainUntilDate.IsZero() {
			return fmt.Errorf("%w: object lock retention mode is required", meta.ErrInvalidArgument)
		}
	} else {
		if !isValidObjectLockMode(retention.Mode) {
			return fmt.Errorf("%w: object lock retention mode is invalid", meta.ErrInvalidArgument)
		}
		if retention.RetainUntilDate.IsZero() {
			return fmt.Errorf("%w: object lock retain-until date is required", meta.ErrInvalidArgument)
		}
	}
	switch legalHold {
	case "", model.ObjectLockLegalHoldOn, model.ObjectLockLegalHoldOff:
		return nil
	default:
		return fmt.Errorf("%w: object lock legal hold status is invalid", meta.ErrInvalidArgument)
	}
}

func isValidObjectLockMode(mode model.ObjectLockMode) bool {
	return mode == model.ObjectLockModeGovernance || mode == model.ObjectLockModeCompliance
}

func objectHeadProtectedByObjectLock(head model.ObjectHead, now time.Time, bypassGovernance bool) bool {
	return objectLockProtected(head.DeleteMarker, head.ObjectLockRetention, head.ObjectLockLegalHold, now, bypassGovernance)
}

func objectVersionProtectedByObjectLock(version model.ObjectVersion, now time.Time, bypassGovernance bool) bool {
	return objectLockProtected(version.DeleteMarker, version.ObjectLockRetention, version.ObjectLockLegalHold, now, bypassGovernance)
}

func objectLockProtected(deleteMarker bool, retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus, now time.Time, bypassGovernance bool) bool {
	if deleteMarker {
		return false
	}
	if legalHold == model.ObjectLockLegalHoldOn {
		return true
	}
	if retention.Mode == "" {
		return false
	}
	if retention.RetainUntilDate.IsZero() {
		return true
	}
	if !now.UTC().Before(retention.RetainUntilDate.UTC()) {
		return false
	}
	switch retention.Mode {
	case model.ObjectLockModeCompliance:
		return true
	case model.ObjectLockModeGovernance:
		return !bypassGovernance
	default:
		return true
	}
}

func retentionUpdateBlocked(current, next model.ObjectLockRetention, now time.Time, bypassGovernance bool) bool {
	if current.Mode == "" {
		return false
	}
	if current.RetainUntilDate.IsZero() {
		return true
	}
	if !now.UTC().Before(current.RetainUntilDate.UTC()) {
		return false
	}
	switch current.Mode {
	case model.ObjectLockModeCompliance:
		return next.Mode != model.ObjectLockModeCompliance || next.RetainUntilDate.Before(current.RetainUntilDate)
	case model.ObjectLockModeGovernance:
		return !bypassGovernance
	default:
		return true
	}
}

func cloneStorageClass(in storage.StorageClassSnapshot) storage.StorageClassSnapshot {
	return storage.CloneStorageClassSnapshot(in)
}

func cloneSegmentRef(in storage.SegmentRef) storage.SegmentRef {
	return storage.CloneSegmentRef(in)
}

func cloneSegmentRefs(in []storage.SegmentRef) []storage.SegmentRef {
	return storage.CloneSegmentRefs(in)
}

func normalizeSegmentRefs(single storage.SegmentRef, refs []storage.SegmentRef) []storage.SegmentRef {
	if len(refs) > 0 {
		return cloneSegmentRefs(refs)
	}
	if single.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{cloneSegmentRef(single)}
}

func objectSegmentRefsFromVersion(version model.ObjectVersion) []storage.SegmentRef {
	return normalizeSegmentRefs(version.SegmentRef, version.SegmentRefs)
}

func firstSegmentRef(refs []storage.SegmentRef) storage.SegmentRef {
	if len(refs) == 0 {
		return storage.SegmentRef{}
	}
	return cloneSegmentRef(refs[0])
}

func cloneHead(in model.ObjectHead) model.ObjectHead {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	out.UserMetadata = cloneStringMap(in.UserMetadata)
	out.Tags = cloneStringMap(in.Tags)
	return out
}

func cloneVersion(in model.ObjectVersion) model.ObjectVersion {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	out.UserMetadata = cloneStringMap(in.UserMetadata)
	out.Tags = cloneStringMap(in.Tags)
	return out
}

func cloneUpload(in model.MultipartUpload) model.MultipartUpload {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.UserMetadata = cloneStringMap(in.UserMetadata)
	out.Tags = cloneStringMap(in.Tags)
	return out
}

func clonePart(in model.MultipartPart) model.MultipartPart {
	out := in
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	return out
}

func cloneMultipartCompletionRecord(in model.MultipartCompletionRecord) model.MultipartCompletionRecord {
	return in
}

func multipartCompletionMatches(record model.MultipartCompletionRecord, req meta.PrepareMultipartCompletionRequest) bool {
	return record.BucketID == req.BucketID &&
		record.Key == req.Key &&
		record.UploadID == req.UploadID &&
		record.ObjectVersionID == req.ObjectVersionID &&
		record.ExpectedHeadVersionID == req.ExpectedHeadVersionID &&
		record.ETag == req.ETag &&
		record.SizeBytes == req.SizeBytes &&
		record.PartCount == req.PartCount
}

func multipartCompletionStateAtLeast(state, target model.MultipartCompletionState) bool {
	return multipartCompletionStateRank(state) >= multipartCompletionStateRank(target)
}

func multipartCompletionStateRank(state model.MultipartCompletionState) int {
	switch state {
	case model.MultipartCompletionCompleted:
		return 3
	case model.MultipartCompletionPublished:
		return 2
	case model.MultipartCompletionPrepared:
		return 1
	default:
		return 0
	}
}

func headFromVersion(version model.ObjectVersion, lastModified time.Time) model.ObjectHead {
	return model.ObjectHead{
		BucketID:             version.BucketID,
		Key:                  version.Key,
		VersionID:            version.VersionID,
		SizeBytes:            version.SizeBytes,
		ETag:                 version.ETag,
		ContentType:          version.ContentType,
		StorageClass:         cloneStorageClass(version.StorageClass),
		ServerSideEncryption: version.ServerSideEncryption,
		SegmentRef:           cloneSegmentRef(version.SegmentRef),
		SegmentRefs:          cloneSegmentRefs(version.SegmentRefs),
		UserMetadata:         cloneStringMap(version.UserMetadata),
		Tags:                 cloneStringMap(version.Tags),
		ObjectLockRetention:  version.ObjectLockRetention,
		ObjectLockLegalHold:  version.ObjectLockLegalHold,
		LastModified:         lastModified,
		DeleteMarker:         version.DeleteMarker,
	}
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

var _ meta.Repository = (*Repository)(nil)

func IsNotFound(err error) bool {
	return errors.Is(err, meta.ErrNotFound)
}

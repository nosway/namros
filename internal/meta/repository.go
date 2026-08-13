package meta

import (
	"context"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type Repository interface {
	GetMetadataSchema(ctx context.Context) (model.MetadataSchemaRecord, error)
	PutMetadataSchema(ctx context.Context, req PutMetadataSchemaRequest) (model.MetadataSchemaRecord, error)
	CreateTenant(ctx context.Context, req CreateTenantRequest) (model.Tenant, error)
	GetTenant(ctx context.Context, tenantID string) (model.Tenant, error)
	PutTenantQuota(ctx context.Context, req TenantQuotaRequest) (model.TenantQuota, error)
	GetTenantQuota(ctx context.Context, tenantID string) (model.TenantQuota, error)
	DeleteTenantQuota(ctx context.Context, tenantID string) error
	PutTenantUsage(ctx context.Context, req TenantUsageRequest) (model.TenantUsage, error)
	GetTenantUsage(ctx context.Context, tenantID string) (model.TenantUsage, error)
	PutAccessKey(ctx context.Context, req PutAccessKeyRequest) (model.AccessKey, error)
	GetAccessKey(ctx context.Context, accessKeyID string) (model.AccessKey, error)
	PutKMSKey(ctx context.Context, req PutKMSKeyRequest) (model.KMSKeyRecord, error)
	GetKMSKey(ctx context.Context, keyID string) (model.KMSKeyRecord, error)
	ListKMSKeys(ctx context.Context, req ListKMSKeysRequest) ([]model.KMSKeyRecord, error)
	PutComplianceProfileAttachment(ctx context.Context, req PutComplianceProfileAttachmentRequest) (model.ComplianceProfileAttachment, error)
	ListComplianceProfileAttachments(ctx context.Context, req ListComplianceProfileAttachmentsRequest) ([]model.ComplianceProfileAttachment, error)

	CreateBucket(ctx context.Context, req CreateBucketRequest) (model.Bucket, error)
	PutBucketVersioning(ctx context.Context, req PutBucketVersioningRequest) (model.Bucket, error)
	PutBucketCORS(ctx context.Context, req BucketCORSRequest) (model.Bucket, error)
	GetBucketCORS(ctx context.Context, bucketID string) ([]model.CORSRule, error)
	DeleteBucketCORS(ctx context.Context, bucketID string) (model.Bucket, error)
	PutBucketLifecycle(ctx context.Context, req BucketLifecycleRequest) (model.Bucket, error)
	GetBucketLifecycle(ctx context.Context, bucketID string) (model.BucketLifecycleConfiguration, error)
	DeleteBucketLifecycle(ctx context.Context, bucketID string, audit AuditContext) (model.Bucket, error)
	PutBucketEncryption(ctx context.Context, req BucketEncryptionRequest) (model.Bucket, error)
	GetBucketEncryption(ctx context.Context, bucketID string) (model.ServerSideEncryption, error)
	DeleteBucketEncryption(ctx context.Context, bucketID string, audit AuditContext) (model.Bucket, error)
	PutBucketQuota(ctx context.Context, req BucketQuotaRequest) (model.BucketQuota, error)
	GetBucketQuota(ctx context.Context, bucketID string) (model.BucketQuota, error)
	DeleteBucketQuota(ctx context.Context, bucketID string) error
	PutBucketPolicy(ctx context.Context, req BucketPolicyRequest) (model.Bucket, error)
	GetBucketPolicy(ctx context.Context, bucketID string) (auth.PolicyDocument, error)
	DeleteBucketPolicy(ctx context.Context, bucketID string, audit AuditContext) (model.Bucket, error)
	PutBucketObjectLock(ctx context.Context, req BucketObjectLockRequest) (model.Bucket, error)
	GetBucketObjectLock(ctx context.Context, bucketID string) (model.BucketObjectLockConfiguration, error)
	ListBuckets(ctx context.Context, tenantID string) ([]model.Bucket, error)
	GetBucketByName(ctx context.Context, name string) (model.Bucket, error)
	DeleteBucket(ctx context.Context, bucketID string) error

	BeginPutObject(ctx context.Context, req BeginPutObjectRequest) (model.PendingObjectVersion, error)
	CommitObjectVersion(ctx context.Context, req CommitObjectVersionRequest) (model.ObjectHead, error)
	PutObjectVersion(ctx context.Context, req PutObjectVersionRequest) (PutObjectVersionResult, error)
	GetObjectHead(ctx context.Context, bucketID, key string) (model.ObjectHead, error)
	GetObjectVersion(ctx context.Context, bucketID, key, versionID string) (model.ObjectVersion, error)
	DeleteObject(ctx context.Context, req DeleteObjectRequest) (model.DeleteResult, error)
	ListObjects(ctx context.Context, req ListObjectsRequest) (model.ListObjectsResult, error)
	ListObjectVersions(ctx context.Context, req ListObjectVersionsRequest) (model.ListObjectVersionsResult, error)
	GetObjectTags(ctx context.Context, req ObjectTagsRequest) (map[string]string, error)
	PutObjectTags(ctx context.Context, req ObjectTagsRequest) error
	DeleteObjectTags(ctx context.Context, req ObjectTagsRequest) error
	GetObjectRetention(ctx context.Context, req ObjectRetentionRequest) (model.ObjectLockRetention, error)
	PutObjectRetention(ctx context.Context, req ObjectRetentionRequest) error
	GetObjectLegalHold(ctx context.Context, req ObjectLegalHoldRequest) (model.ObjectLockLegalHoldStatus, error)
	PutObjectLegalHold(ctx context.Context, req ObjectLegalHoldRequest) error
	ListAuditEvents(ctx context.Context, req ListAuditEventsRequest) ([]model.AuditEvent, error)
	PutAdminAuditEvent(ctx context.Context, req PutAdminAuditEventRequest) (model.AuditEvent, error)
	PutAdminAuditEvents(ctx context.Context, req PutAdminAuditEventsRequest) ([]model.AuditEvent, error)
	ImportOperationalMetadata(ctx context.Context, req ImportOperationalMetadataRequest) (ImportOperationalMetadataResult, error)
	PutMetadataMigrationOperation(ctx context.Context, req PutMetadataMigrationOperationRequest) (model.MetadataMigrationOperationRecord, error)
	ListMetadataMigrationOperations(ctx context.Context, req ListMetadataMigrationOperationsRequest) ([]model.MetadataMigrationOperationRecord, error)
	ListProtectedRefs(ctx context.Context, req ListProtectedRefsRequest) ([]model.ProtectedRef, error)
	PutGCCandidate(ctx context.Context, req PutGCCandidateRequest) (model.GCCandidateRecord, error)
	ListGCCandidates(ctx context.Context, req ListGCCandidatesRequest) ([]model.GCCandidateRecord, error)
	DeleteGCCandidate(ctx context.Context, segmentID string) error
	PutGCOperation(ctx context.Context, req PutGCOperationRequest) (model.GCOperationRecord, error)
	ListGCOperations(ctx context.Context, req ListGCOperationsRequest) ([]model.GCOperationRecord, error)
	PutDedupeOperation(ctx context.Context, req PutDedupeOperationRequest) (model.DedupeOperationRecord, error)
	ListDedupeOperations(ctx context.Context, req ListDedupeOperationsRequest) ([]model.DedupeOperationRecord, error)
	AcquireDedupeOperationLock(ctx context.Context, req AcquireDedupeOperationLockRequest) (model.DedupeOperationLock, error)
	ReleaseDedupeOperationLock(ctx context.Context, req ReleaseDedupeOperationLockRequest) error
	PutSharedObjectRelease(ctx context.Context, req PutSharedObjectReleaseRequest) (model.SharedObjectRelease, error)
	ListSharedObjectReleases(ctx context.Context, req ListSharedObjectReleasesRequest) ([]model.SharedObjectRelease, error)
	PutVolumePool(ctx context.Context, req PutVolumePoolRequest) (model.VolumePool, error)
	GetVolumePool(ctx context.Context, poolID string) (model.VolumePool, error)
	ListVolumePools(ctx context.Context, req ListVolumePoolsRequest) ([]model.VolumePool, error)
	PutVolumeDrainOperation(ctx context.Context, req PutVolumeDrainOperationRequest) (model.VolumeDrainOperationRecord, error)
	ListVolumeDrainOperations(ctx context.Context, req ListVolumeDrainOperationsRequest) ([]model.VolumeDrainOperationRecord, error)
	AcquireWorkerLease(ctx context.Context, req AcquireWorkerLeaseRequest) (model.WorkerLease, error)
	ReleaseWorkerLease(ctx context.Context, req ReleaseWorkerLeaseRequest) error
	ListWorkerLeases(ctx context.Context, req ListWorkerLeasesRequest) ([]model.WorkerLease, error)
	PutWorkerOperation(ctx context.Context, req PutWorkerOperationRequest) (model.WorkerOperationRecord, error)
	ListWorkerOperations(ctx context.Context, req ListWorkerOperationsRequest) ([]model.WorkerOperationRecord, error)
	PutWorkerControl(ctx context.Context, req PutWorkerControlRequest) (model.WorkerControlRecord, error)
	GetWorkerControl(ctx context.Context, req GetWorkerControlRequest) (model.WorkerControlRecord, error)
	PublishSharedObject(ctx context.Context, req PublishSharedObjectRequest) (model.SharedObject, error)
	GetSharedObject(ctx context.Context, sharedObjectID string) (model.SharedObject, error)
	ListSharedObjects(ctx context.Context, req ListSharedObjectsRequest) ([]model.SharedObject, error)
	AttachObjectVersionToSharedObject(ctx context.Context, req AttachObjectVersionToSharedObjectRequest) (model.AttachSharedObjectResult, error)
	PublishObjectVersionRefs(ctx context.Context, req PublishObjectVersionRefsRequest) (PublishObjectVersionRefsResult, error)
	ListSharedObjectRefs(ctx context.Context, req ListSharedObjectRefsRequest) ([]model.SharedObjectRef, error)
	RepairSharedObjectRefCounts(ctx context.Context, req RepairSharedObjectRefCountsRequest) (model.SharedObjectRepairResult, error)
	RepairListIndexes(ctx context.Context, req RepairListIndexesRequest) (model.ListIndexRepairResult, error)

	CreateMultipartUpload(ctx context.Context, req CreateMultipartUploadRequest) (model.MultipartUpload, error)
	GetMultipartUpload(ctx context.Context, req MultipartUploadRequest) (model.MultipartUpload, error)
	ListMultipartUploads(ctx context.Context, req ListMultipartUploadsRequest) (model.ListMultipartUploadsResult, error)
	PutMultipartPart(ctx context.Context, req PutMultipartPartRequest) (model.MultipartPart, *model.MultipartPart, error)
	GetMultipartParts(ctx context.Context, req GetMultipartPartsRequest) ([]model.MultipartPart, error)
	ListMultipartParts(ctx context.Context, req MultipartUploadRequest) ([]model.MultipartPart, error)
	GetMultipartCompletion(ctx context.Context, req MultipartUploadRequest) (model.MultipartCompletionRecord, error)
	PrepareMultipartCompletion(ctx context.Context, req PrepareMultipartCompletionRequest) (model.MultipartCompletionRecord, error)
	MarkMultipartCompletionPublished(ctx context.Context, req MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error)
	MarkMultipartCompletionCompleted(ctx context.Context, req MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error)
	CompleteMultipartUpload(ctx context.Context, req CompleteMultipartUploadRequest) (model.MultipartUpload, error)
	AbortMultipartUpload(ctx context.Context, req MultipartUploadRequest) ([]model.MultipartPart, error)
	CleanupMultipartUploadParts(ctx context.Context, req CleanupMultipartUploadPartsRequest) (CleanupMultipartUploadPartsResult, error)
}

type CreateTenantRequest struct {
	TenantID    string
	DisplayName string
}

type PutMetadataSchemaRequest struct {
	SchemaVersion    int
	MinReaderVersion int
	MinWriterVersion int
	UpdatedBy        string
}

type TenantQuotaRequest struct {
	TenantID         string
	MaxBytes         int64
	MaxObjects       int64
	MaxActiveUploads int64
}

type TenantUsageRequest struct {
	TenantID          string
	ObjectBytes       int64
	ObjectCount       int64
	ActiveUploads     int64
	ReconciledAt      time.Time
	ReconciliationID  string
	ReconciliationErr string
}

type PutAccessKeyRequest struct {
	TenantID    string
	AccessKeyID string
	SecretHash  string
	Status      model.AccessKeyStatus
	Permissions []string
}

type PutKMSKeyRequest struct {
	KeyID      string
	KeyVersion string
	State      model.KMSKeyState
}

type ListKMSKeysRequest struct {
	Limit int
}

type PutComplianceProfileAttachmentRequest struct {
	ProfileID              string
	Regulation             string
	RecordClass            string
	BucketID               string
	Prefix                 string
	ObjectClass            string
	RetentionMode          model.ObjectLockMode
	RetentionDays          int
	RetentionYears         int
	LegalHoldPolicy        string
	GovernanceBypassPolicy string
	EvidenceExportPolicy   string
}

type ListComplianceProfileAttachmentsRequest struct {
	BucketID string
	Limit    int
}

type CreateBucketRequest struct {
	TenantID          string
	Name              string
	Region            string
	ObjectLockEnabled bool
}

type PutBucketVersioningRequest struct {
	BucketID string
	State    model.BucketVersioningState
}

type BucketCORSRequest struct {
	BucketID string
	Rules    []model.CORSRule
}

type BucketLifecycleRequest struct {
	BucketID      string
	Configuration model.BucketLifecycleConfiguration
	Audit         AuditContext
}

type BucketEncryptionRequest struct {
	BucketID   string
	Encryption model.ServerSideEncryption
	Audit      AuditContext
}

type BucketQuotaRequest struct {
	BucketID           string
	MaxObjectSizeBytes int64
}

type BucketPolicyRequest struct {
	BucketID string
	Policy   auth.PolicyDocument
	Audit    AuditContext
}

type BucketObjectLockRequest struct {
	BucketID      string
	Configuration model.BucketObjectLockConfiguration
	Audit         AuditContext
}

type BeginPutObjectRequest struct {
	BucketID             string
	Key                  string
	SizeBytes            int64
	ETag                 string
	ContentType          string
	StorageClass         storage.StorageClassSnapshot
	ServerSideEncryption model.ServerSideEncryption
	SegmentRef           storage.SegmentRef
	SegmentRefs          []storage.SegmentRef
	UserMetadata         map[string]string
	Tags                 map[string]string
	ObjectLockRetention  model.ObjectLockRetention
	ObjectLockLegalHold  model.ObjectLockLegalHoldStatus
}

type CommitObjectVersionRequest struct {
	BucketID              string
	Key                   string
	VersionID             string
	ExpectedHeadVersionID string
}

type PutObjectVersionRequest = BeginPutObjectRequest

type PutObjectVersionResult struct {
	Head              model.ObjectHead
	ReplacedHead      model.ObjectHead
	ReplacedHeadFound bool
}

type DeleteObjectRequest struct {
	BucketID                  string
	Key                       string
	VersionID                 string
	BypassGovernanceRetention bool
	BypassAudit               AuditContext
}

type ListObjectsRequest struct {
	BucketID          string
	Prefix            string
	Delimiter         string
	ContinuationToken string
	MaxKeys           int
}

type ListObjectVersionsRequest struct {
	BucketID        string
	Prefix          string
	Delimiter       string
	KeyMarker       string
	VersionIDMarker string
	MaxKeys         int
}

type ObjectTagsRequest struct {
	BucketID  string
	Key       string
	VersionID string
	Tags      map[string]string
}

type ObjectRetentionRequest struct {
	BucketID                  string
	Key                       string
	VersionID                 string
	Retention                 model.ObjectLockRetention
	BypassGovernanceRetention bool
	BypassAudit               AuditContext
	Audit                     AuditContext
}

type ObjectLegalHoldRequest struct {
	BucketID  string
	Key       string
	VersionID string
	LegalHold model.ObjectLockLegalHoldStatus
	Audit     AuditContext
}

type AuditContext struct {
	RequestID string
	Reason    string
	Principal model.AuditPrincipal
}

type ListAuditEventsRequest struct {
	BucketID string
	Key      string
	Action   model.AuditAction
	Limit    int
}

type PutAdminAuditEventRequest struct {
	Action    model.AuditAction
	BucketID  string
	Key       string
	VersionID string
	Details   map[string]string
	Audit     AuditContext
}

type PutAdminAuditEventsRequest struct {
	Events []PutAdminAuditEventRequest
}

type ImportOperationalMetadataRequest struct {
	MetadataSchema              *model.MetadataSchemaRecord
	MetadataMigrationOperations []model.MetadataMigrationOperationRecord
	KMSKeys                     []model.KMSKeyRecord
	AuditEvents                 []model.AuditEvent
	GCOperations                []model.GCOperationRecord
	DedupeOperations            []model.DedupeOperationRecord
	SharedObjects               []model.SharedObject
	SharedObjectReleases        []model.SharedObjectRelease
	VolumePools                 []model.VolumePool
	VolumeDrainOperations       []model.VolumeDrainOperationRecord
	WorkerLeases                []model.WorkerLease
	WorkerOperations            []model.WorkerOperationRecord
	RequireEmptyTarget          bool
}

type ImportOperationalMetadataResult struct {
	MetadataSchema              int
	MetadataMigrationOperations int
	KMSKeys                     int
	AuditEvents                 int
	GCOperations                int
	DedupeOperations            int
	SharedObjects               int
	SharedObjectReleases        int
	VolumePools                 int
	VolumeDrainOperations       int
	WorkerLeases                int
	WorkerOperations            int
}

type PutMetadataMigrationOperationRequest struct {
	ResumeOfOperationID string
	TargetSchemaVersion int
	Status              model.MetadataMigrationOperationStatus
	DryRun              bool
	Apply               bool
	OwnerID             string
	Cursor              string
	Steps               []model.MetadataMigrationStep
	StartedAt           time.Time
	FinishedAt          time.Time
}

type ListMetadataMigrationOperationsRequest struct {
	Status model.MetadataMigrationOperationStatus
	Limit  int
}

type ListProtectedRefsRequest struct {
	BucketID   string
	Key        string
	VersionID  string
	SegmentID  string
	ActiveOnly bool
	Limit      int
}

type PutGCCandidateRequest struct {
	SegmentRef storage.SegmentRef
	Reason     storage.DeleteReason
}

type ListGCCandidatesRequest struct {
	Limit int
}

type PutGCOperationRequest struct {
	ResumeOfOperationID string
	Status              model.GCOperationStatus
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Deleted             int
	Skipped             int
	Retryable           int
	Attempts            []model.GCOperationAttempt
}

type ListGCOperationsRequest struct {
	Limit int
}

type PutDedupeOperationRequest struct {
	ResumeOfOperationID string
	Status              model.DedupeOperationStatus
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Acked               int
	Skipped             int
	Retryable           int
	Attempts            []model.DedupeOperationAttempt
}

type ListDedupeOperationsRequest struct {
	Limit int
}

type AcquireDedupeOperationLockRequest struct {
	LockID  string
	OwnerID string
	TTL     time.Duration
}

type ReleaseDedupeOperationLockRequest struct {
	LockID  string
	OwnerID string
}

type PutSharedObjectReleaseRequest struct {
	SharedObjectID string
	SegmentRef     storage.SegmentRef
	Reason         storage.DeleteReason
	Status         model.SharedObjectReleaseStatus
}

type ListSharedObjectReleasesRequest struct {
	SharedObjectID string
	Status         model.SharedObjectReleaseStatus
	Limit          int
}

type PutVolumePoolRequest struct {
	PoolID          string
	Generation      uint64
	DurabilityClass string
	StorageClassIDs []string
	Members         []model.VolumePoolMember
}

type ListVolumePoolsRequest struct {
	Limit int
}

type PutVolumeDrainOperationRequest struct {
	ResumeOfOperationID string
	PoolID              string
	SourceVolumeID      string
	TargetVolumeID      string
	OwnerID             string
	Status              model.VolumeDrainOperationStatus
	Cursor              string
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Copied              int
	Skipped             int
	Protected           int
	Retryable           int
	Attempts            []model.VolumeDrainAttempt
}

type ListVolumeDrainOperationsRequest struct {
	SourceVolumeID string
	TargetVolumeID string
	Status         model.VolumeDrainOperationStatus
	Limit          int
}

type AcquireWorkerLeaseRequest struct {
	WorkerKind string
	ShardID    string
	OwnerID    string
	TTL        time.Duration
	Cursor     string
}

type ReleaseWorkerLeaseRequest struct {
	WorkerKind string
	ShardID    string
	OwnerID    string
}

type ListWorkerLeasesRequest struct {
	WorkerKind string
	ShardID    string
	Limit      int
}

type PutWorkerOperationRequest struct {
	WorkerKind string
	ShardID    string
	OwnerID    string
	LeaseID    string
	Status     model.WorkerOperationStatus
	Cursor     string
	Scanned    int
	Processed  int
	Skipped    int
	Retryable  int
	LastError  string
	StartedAt  time.Time
	FinishedAt time.Time
}

type ListWorkerOperationsRequest struct {
	WorkerKind string
	ShardID    string
	Status     model.WorkerOperationStatus
	Limit      int
}

type PutWorkerControlRequest struct {
	WorkerKind string
	ShardID    string
	State      model.WorkerControlState
	Reason     string
	UpdatedBy  string
}

type GetWorkerControlRequest struct {
	WorkerKind string
	ShardID    string
}

type PublishSharedObjectRequest struct {
	BucketID  string
	Key       string
	VersionID string
}

type ListSharedObjectsRequest struct {
	TenantID string
	BucketID string
	Key      string
	Limit    int
}

type AttachObjectVersionToSharedObjectRequest struct {
	SharedObjectID string
	BucketID       string
	Key            string
	VersionID      string
}

type PublishObjectVersionRefsRequest struct {
	BucketID               string
	Key                    string
	VersionID              string
	ExpectedSourceVolumeID string
	SegmentRefs            []storage.SegmentRef
}

type PublishObjectVersionRefsResult struct {
	Version             model.ObjectVersion
	PreviousSegmentRefs []storage.SegmentRef
}

type ListSharedObjectRefsRequest struct {
	SharedObjectID string
	BucketID       string
	Key            string
	VersionID      string
	Limit          int
}

type RepairSharedObjectRefCountsRequest struct {
	SharedObjectID string
	Limit          int
}

type RepairListIndexesRequest struct {
	BucketID string
	Limit    int
	Apply    bool
}

type CreateMultipartUploadRequest struct {
	BucketID             string
	Key                  string
	ContentType          string
	StorageClass         storage.StorageClassSnapshot
	ServerSideEncryption model.ServerSideEncryption
	UserMetadata         map[string]string
	Tags                 map[string]string
	ObjectLockRetention  model.ObjectLockRetention
	ObjectLockLegalHold  model.ObjectLockLegalHoldStatus
}

type ListMultipartUploadsRequest struct {
	BucketID       string
	Prefix         string
	Delimiter      string
	KeyMarker      string
	UploadIDMarker string
	MaxUploads     int
}

type MultipartUploadRequest struct {
	BucketID string
	Key      string
	UploadID string
}

type GetMultipartPartsRequest struct {
	BucketID    string
	Key         string
	UploadID    string
	PartNumbers []int
}

type PrepareMultipartCompletionRequest struct {
	BucketID              string
	Key                   string
	UploadID              string
	ObjectVersionID       string
	ExpectedHeadVersionID string
	ETag                  string
	SizeBytes             int64
	PartCount             int
}

type MultipartCompletionStateRequest struct {
	BucketID string
	Key      string
	UploadID string
}

type CompleteMultipartUploadRequest struct {
	BucketID        string
	Key             string
	UploadID        string
	ObjectVersionID string
	ETag            string
	SizeBytes       int64
	PartCount       int
}

type CleanupMultipartUploadPartsRequest struct {
	BucketID string
	Key      string
	UploadID string
	Limit    int
}

type CleanupMultipartUploadPartsResult struct {
	Upload       model.MultipartUpload
	DeletedParts int
	HasMore      bool
}

type PutMultipartPartRequest struct {
	BucketID   string
	Key        string
	UploadID   string
	PartNumber int
	SizeBytes  int64
	ETag       string
	SegmentRef storage.SegmentRef
}

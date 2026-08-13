package model

import (
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/storage"
)

type Tenant struct {
	TenantID    string
	DisplayName string
	CreatedAt   time.Time
}

type MetadataSchemaRecord struct {
	SchemaVersion    int
	MinReaderVersion int
	MinWriterVersion int
	UpdatedBy        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type MetadataMigrationOperationStatus string

const (
	MetadataMigrationOperationPlanned      MetadataMigrationOperationStatus = "planned"
	MetadataMigrationOperationRunning      MetadataMigrationOperationStatus = "running"
	MetadataMigrationOperationSucceeded    MetadataMigrationOperationStatus = "succeeded"
	MetadataMigrationOperationRetryPending MetadataMigrationOperationStatus = "retry_pending"
	MetadataMigrationOperationFailed       MetadataMigrationOperationStatus = "failed"
	MetadataMigrationOperationCanceled     MetadataMigrationOperationStatus = "canceled"
)

type MetadataMigrationStepStatus string

const (
	MetadataMigrationStepPlanned      MetadataMigrationStepStatus = "planned"
	MetadataMigrationStepSucceeded    MetadataMigrationStepStatus = "succeeded"
	MetadataMigrationStepRepairNeeded MetadataMigrationStepStatus = "repair_needed"
	MetadataMigrationStepFailed       MetadataMigrationStepStatus = "failed"
	MetadataMigrationStepSkipped      MetadataMigrationStepStatus = "skipped"
)

type MetadataMigrationStep struct {
	Name            string
	Status          MetadataMigrationStepStatus
	Message         string
	RepairNeeded    bool
	RecordsScanned  int
	RecordsRepaired int
}

type MetadataMigrationOperationRecord struct {
	OperationID         string
	ResumeOfOperationID string
	TargetSchemaVersion int
	Status              MetadataMigrationOperationStatus
	DryRun              bool
	Apply               bool
	OwnerID             string
	Cursor              string
	Steps               []MetadataMigrationStep
	StartedAt           time.Time
	FinishedAt          time.Time
	CreatedAt           time.Time
}

type TenantQuota struct {
	TenantID         string
	MaxBytes         int64
	MaxObjects       int64
	MaxActiveUploads int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type TenantUsage struct {
	TenantID          string
	ObjectBytes       int64
	ObjectCount       int64
	ActiveUploads     int64
	ReconciledAt      time.Time
	UpdatedAt         time.Time
	CreatedAt         time.Time
	ReconciliationID  string
	ReconciliationErr string
}

type AccessKeyStatus string

const (
	AccessKeyActive   AccessKeyStatus = "active"
	AccessKeyDisabled AccessKeyStatus = "disabled"
)

type AccessKey struct {
	TenantID    string
	AccessKeyID string
	SecretHash  string
	Status      AccessKeyStatus
	Permissions []string
	CreatedAt   time.Time
}

type Bucket struct {
	BucketID          string
	TenantID          string
	Name              string
	Region            string
	VersioningState   BucketVersioningState
	CORSRules         []CORSRule
	Lifecycle         BucketLifecycleConfiguration
	DefaultEncryption ServerSideEncryption
	ObjectLock        BucketObjectLockConfiguration
	Policy            auth.PolicyDocument
	CreatedAt         time.Time
}

type BucketQuota struct {
	BucketID           string
	MaxObjectSizeBytes int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type BucketVersioningState string

const (
	BucketVersioningUnversioned BucketVersioningState = ""
	BucketVersioningEnabled     BucketVersioningState = "Enabled"
	BucketVersioningSuspended   BucketVersioningState = "Suspended"
)

type CORSRule struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	ExposeHeaders  []string
	MaxAgeSeconds  int
}

type LifecycleRuleStatus string

const (
	LifecycleRuleEnabled  LifecycleRuleStatus = "Enabled"
	LifecycleRuleDisabled LifecycleRuleStatus = "Disabled"
)

type BucketLifecycleConfiguration struct {
	Rules []LifecycleRule
}

type LifecycleRule struct {
	ID                             string
	Status                         LifecycleRuleStatus
	Prefix                         string
	Expiration                     LifecycleExpiration
	NoncurrentVersionExpiration    LifecycleNoncurrentVersionExpiration
	AbortIncompleteMultipartUpload LifecycleAbortIncompleteMultipartUpload
}

type LifecycleExpiration struct {
	Days                      int
	Date                      time.Time
	ExpiredObjectDeleteMarker bool
}

type LifecycleNoncurrentVersionExpiration struct {
	NoncurrentDays int
}

type LifecycleAbortIncompleteMultipartUpload struct {
	DaysAfterInitiation int
}

type ObjectLockMode string

const (
	ObjectLockModeGovernance ObjectLockMode = "GOVERNANCE"
	ObjectLockModeCompliance ObjectLockMode = "COMPLIANCE"
)

type ObjectLockLegalHoldStatus string

const (
	ObjectLockLegalHoldOn  ObjectLockLegalHoldStatus = "ON"
	ObjectLockLegalHoldOff ObjectLockLegalHoldStatus = "OFF"
)

type BucketObjectLockConfiguration struct {
	Enabled          bool
	DefaultRetention BucketObjectLockDefaultRetention
}

type BucketObjectLockDefaultRetention struct {
	Mode  ObjectLockMode
	Days  int
	Years int
}

type ObjectLockRetention struct {
	Mode            ObjectLockMode
	RetainUntilDate time.Time
}

type ServerSideEncryptionAlgorithm string

const (
	ServerSideEncryptionNone   ServerSideEncryptionAlgorithm = ""
	ServerSideEncryptionAES256 ServerSideEncryptionAlgorithm = "AES256"
	ServerSideEncryptionAWSKMS ServerSideEncryptionAlgorithm = "aws:kms"
)

type ServerSideEncryption struct {
	Algorithm  ServerSideEncryptionAlgorithm
	KeyID      string
	KeyVersion string
}

type KMSKeyState string

const (
	KMSKeyActive          KMSKeyState = "active"
	KMSKeyDisabled        KMSKeyState = "disabled"
	KMSKeyPendingDeletion KMSKeyState = "pending_deletion"
	KMSKeyDeleted         KMSKeyState = "deleted"
)

type KMSKeyRecord struct {
	KeyID      string
	KeyVersion string
	State      KMSKeyState
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ComplianceProfileAttachment struct {
	ProfileID              string
	Regulation             string
	RecordClass            string
	BucketID               string
	Prefix                 string
	ObjectClass            string
	RetentionMode          ObjectLockMode
	RetentionDays          int
	RetentionYears         int
	LegalHoldPolicy        string
	GovernanceBypassPolicy string
	EvidenceExportPolicy   string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func NormalizeKMSKeyState(state KMSKeyState) KMSKeyState {
	switch state {
	case "", KMSKeyActive:
		return KMSKeyActive
	case KMSKeyDisabled, KMSKeyPendingDeletion, KMSKeyDeleted:
		return state
	default:
		return KMSKeyDisabled
	}
}

func KMSKeyAllowsDecrypt(state KMSKeyState) bool {
	return NormalizeKMSKeyState(state) == KMSKeyActive
}

func KMSKeyAllowsDelete(state KMSKeyState) bool {
	return NormalizeKMSKeyState(state) == KMSKeyActive
}

type AuditAction string

const (
	AuditActionGovernanceBypassDeleteObject       AuditAction = "governance_bypass_delete_object"
	AuditActionGovernanceBypassPutObjectRetention AuditAction = "governance_bypass_put_object_retention"
	AuditActionPutBucketPolicy                    AuditAction = "put_bucket_policy"
	AuditActionDeleteBucketPolicy                 AuditAction = "delete_bucket_policy"
	AuditActionPutBucketLifecycle                 AuditAction = "put_bucket_lifecycle"
	AuditActionDeleteBucketLifecycle              AuditAction = "delete_bucket_lifecycle"
	AuditActionPutBucketEncryption                AuditAction = "put_bucket_encryption"
	AuditActionDeleteBucketEncryption             AuditAction = "delete_bucket_encryption"
	AuditActionPutBucketObjectLock                AuditAction = "put_bucket_object_lock"
	AuditActionPutObjectRetention                 AuditAction = "put_object_retention"
	AuditActionPutObjectLegalHold                 AuditAction = "put_object_legal_hold"
	AuditActionGetObject                          AuditAction = "get_object"
	AuditActionHeadObject                         AuditAction = "head_object"
	AuditActionListObjects                        AuditAction = "list_objects"
	AuditActionDedupeAck                          AuditAction = "dedupe_ack"
	AuditActionDedupeRepair                       AuditAction = "dedupe_repair"
	AuditActionDedupeScrub                        AuditAction = "dedupe_scrub"
	AuditActionAdminMetadataExport                AuditAction = "admin_metadata_export"
	AuditActionAdminMetadataImport                AuditAction = "admin_metadata_import"
	AuditActionAdminMetadataListIndexRepair       AuditAction = "admin_metadata_list_index_repair"
	AuditActionAdminMetadataMigration             AuditAction = "admin_metadata_migration"
	AuditActionAdminComplianceEvidence            AuditAction = "admin_compliance_evidence"
	AuditActionAdminKMSKeyPut                     AuditAction = "admin_kms_key_put"
	AuditActionAdminComplianceProfileAttach       AuditAction = "admin_compliance_profile_attach"
)

type AuditPrincipal struct {
	TenantID       string   `json:"tenant_id,omitempty"`
	AccessKeyID    string   `json:"access_key_id,omitempty"`
	DisplayName    string   `json:"display_name,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Groups         []string `json:"groups,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	ExternalIssuer string   `json:"external_issuer,omitempty"`
	PolicyVersion  string   `json:"policy_version,omitempty"`
	SourceIdentity string   `json:"source_identity,omitempty"`
	Root           bool     `json:"root"`
}

type AuditEvent struct {
	EventID      string
	Action       AuditAction
	BucketID     string
	Key          string
	VersionID    string
	RequestID    string
	Reason       string
	Principal    AuditPrincipal
	Details      map[string]string
	PreviousHash string
	EventHash    string
	CreatedAt    time.Time
}

type ProtectedRefReason string

const (
	ProtectedRefReasonObjectLock ProtectedRefReason = "object_lock"
)

type ProtectedRef struct {
	RefID           string
	Reason          ProtectedRefReason
	BucketID        string
	Key             string
	VersionID       string
	SegmentID       string
	SegmentRef      storage.SegmentRef
	RetentionMode   ObjectLockMode
	RetainUntilDate time.Time
	LegalHold       ObjectLockLegalHoldStatus
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type GCOperationAttemptStatus string

const (
	GCOperationAttemptDeleted   GCOperationAttemptStatus = "deleted"
	GCOperationAttemptSkipped   GCOperationAttemptStatus = "skipped"
	GCOperationAttemptRetryable GCOperationAttemptStatus = "retryable"
)

type GCOperationStatus string

const (
	GCOperationSucceeded    GCOperationStatus = "succeeded"
	GCOperationRetryPending GCOperationStatus = "retry_pending"
	GCOperationFailed       GCOperationStatus = "failed"
)

type GCOperationAttempt struct {
	SegmentID      string
	SharedObjectID string
	Reason         storage.DeleteReason
	Status         GCOperationAttemptStatus
	Retryable      bool
	Error          string
}

type GCOperationRecord struct {
	OperationID         string
	ResumeOfOperationID string
	Status              GCOperationStatus
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Deleted             int
	Skipped             int
	Retryable           int
	Attempts            []GCOperationAttempt
	CreatedAt           time.Time
}

type GCCandidateRecord struct {
	SegmentID  string
	SegmentRef storage.SegmentRef
	Reason     storage.DeleteReason
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DedupeOperationAttemptStatus string

const (
	DedupeOperationAttemptAcked     DedupeOperationAttemptStatus = "acked"
	DedupeOperationAttemptSkipped   DedupeOperationAttemptStatus = "skipped"
	DedupeOperationAttemptRetryable DedupeOperationAttemptStatus = "retryable"
)

type DedupeOperationStatus string

const (
	DedupeOperationSucceeded    DedupeOperationStatus = "succeeded"
	DedupeOperationRetryPending DedupeOperationStatus = "retry_pending"
	DedupeOperationFailed       DedupeOperationStatus = "failed"
)

type DedupeOperationAttempt struct {
	BucketID         string
	Key              string
	SourceVersion    string
	CandidateVersion string
	PlanStatus       string
	PlanReason       string
	Status           DedupeOperationAttemptStatus
	SharedObjectID   string
	OrphansMarked    int
	Retryable        bool
	Error            string
}

type DedupeOperationRecord struct {
	OperationID         string
	ResumeOfOperationID string
	Status              DedupeOperationStatus
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Acked               int
	Skipped             int
	Retryable           int
	Attempts            []DedupeOperationAttempt
	CreatedAt           time.Time
}

type DedupeOperationLock struct {
	LockID     string
	OwnerID    string
	AcquiredAt time.Time
	UpdatedAt  time.Time
	ExpiresAt  time.Time
}

type VolumeDrainOperationStatus string

const (
	VolumeDrainOperationRunning      VolumeDrainOperationStatus = "running"
	VolumeDrainOperationSucceeded    VolumeDrainOperationStatus = "succeeded"
	VolumeDrainOperationRetryPending VolumeDrainOperationStatus = "retry_pending"
	VolumeDrainOperationFailed       VolumeDrainOperationStatus = "failed"
	VolumeDrainOperationCanceled     VolumeDrainOperationStatus = "canceled"
)

type VolumeDrainAttemptStatus string

const (
	VolumeDrainAttemptCopied    VolumeDrainAttemptStatus = "copied"
	VolumeDrainAttemptQueuedGC  VolumeDrainAttemptStatus = "queued_gc"
	VolumeDrainAttemptSkipped   VolumeDrainAttemptStatus = "skipped"
	VolumeDrainAttemptProtected VolumeDrainAttemptStatus = "protected"
	VolumeDrainAttemptRetryable VolumeDrainAttemptStatus = "retryable"
	VolumeDrainAttemptFailed    VolumeDrainAttemptStatus = "failed"
)

type VolumeDrainAttempt struct {
	BucketID        string
	Key             string
	VersionID       string
	SourceSegmentID string
	SourceRef       storage.SegmentRef
	TargetSegmentID string
	TargetRef       storage.SegmentRef
	Status          VolumeDrainAttemptStatus
	Protected       bool
	Retryable       bool
	Error           string
}

type VolumeDrainOperationRecord struct {
	OperationID         string
	ResumeOfOperationID string
	PoolID              string
	SourceVolumeID      string
	TargetVolumeID      string
	OwnerID             string
	Status              VolumeDrainOperationStatus
	Cursor              string
	StartedAt           time.Time
	FinishedAt          time.Time
	Scanned             int
	Copied              int
	Skipped             int
	Protected           int
	Retryable           int
	Attempts            []VolumeDrainAttempt
	CreatedAt           time.Time
}

type SharedObjectReleaseStatus string

const (
	SharedObjectReleasePending SharedObjectReleaseStatus = "pending"
)

type SharedObjectRelease struct {
	ReleaseID      string
	SharedObjectID string
	SegmentID      string
	SegmentRef     storage.SegmentRef
	Reason         storage.DeleteReason
	Status         SharedObjectReleaseStatus
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type SharedObject struct {
	SharedObjectID     string
	TenantID           string
	BucketID           string
	Key                string
	SourceVersionID    string
	SizeBytes          int64
	Digest             storage.Digest
	StorageClass       storage.StorageClassSnapshot
	SegmentRefs        []storage.SegmentRef
	RefCount           int
	ProtectedRootCount int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type SharedObjectRef struct {
	RefID          string
	SharedObjectID string
	BucketID       string
	Key            string
	VersionID      string
	SegmentRefs    []storage.SegmentRef
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AttachSharedObjectResult struct {
	Version             ObjectVersion
	SharedObject        SharedObject
	Ref                 SharedObjectRef
	PreviousSegmentRefs []storage.SegmentRef
}

type SharedObjectRepairResult struct {
	Scanned int `json:"scanned"`
	Updated int `json:"updated"`
}

type ListIndexRepairResult struct {
	ScannedObjectHeads             int `json:"scanned_object_heads"`
	ScannedObjectListEntries       int `json:"scanned_object_list_entries"`
	MissingObjectListEntries       int `json:"missing_object_list_entries"`
	StaleObjectListEntries         int `json:"stale_object_list_entries"`
	RepairedObjectListEntries      int `json:"repaired_object_list_entries"`
	RemovedObjectListEntries       int `json:"removed_object_list_entries"`
	ScannedMultipartUploads        int `json:"scanned_multipart_uploads"`
	ScannedMultipartUploadIndexes  int `json:"scanned_multipart_upload_indexes"`
	MissingMultipartUploadIndexes  int `json:"missing_multipart_upload_indexes"`
	StaleMultipartUploadIndexes    int `json:"stale_multipart_upload_indexes"`
	RepairedMultipartUploadIndexes int `json:"repaired_multipart_upload_indexes"`
	RemovedMultipartUploadIndexes  int `json:"removed_multipart_upload_indexes"`
}

type ObjectVersionState string

const (
	ObjectVersionPending   ObjectVersionState = "pending"
	ObjectVersionCommitted ObjectVersionState = "committed"
)

type ObjectManifestEncoding string

const (
	ObjectManifestEncodingChunked ObjectManifestEncoding = "chunked"
)

type ObjectManifestDescriptor struct {
	Encoding   ObjectManifestEncoding
	RefCount   int
	ChunkCount int
}

type ObjectManifestChunk struct {
	BucketID    string
	Key         string
	VersionID   string
	ChunkNumber int
	SegmentRefs []storage.SegmentRef
	CreatedAt   time.Time
}

type ObjectHead struct {
	BucketID             string
	Key                  string
	VersionID            string
	Revision             uint64
	SizeBytes            int64
	ETag                 string
	ContentType          string
	StorageClass         storage.StorageClassSnapshot
	ServerSideEncryption ServerSideEncryption
	SegmentRef           storage.SegmentRef
	SegmentRefs          []storage.SegmentRef
	Manifest             ObjectManifestDescriptor
	UserMetadata         map[string]string
	Tags                 map[string]string
	ObjectLockRetention  ObjectLockRetention
	ObjectLockLegalHold  ObjectLockLegalHoldStatus
	LastModified         time.Time
	DeleteMarker         bool
}

type ObjectHeadEntry struct {
	BucketID             string
	Key                  string
	VersionID            string
	Revision             uint64
	SizeBytes            int64
	ETag                 string
	ContentType          string
	StorageClass         storage.StorageClassSnapshot
	ServerSideEncryption ServerSideEncryption
	UserMetadata         map[string]string
	Tags                 map[string]string
	ObjectLockRetention  ObjectLockRetention
	ObjectLockLegalHold  ObjectLockLegalHoldStatus
	LastModified         time.Time
	DeleteMarker         bool
}

type ObjectListEntry struct {
	BucketID     string
	Key          string
	VersionID    string
	Revision     uint64
	SizeBytes    int64
	ETag         string
	ContentType  string
	StorageClass storage.StorageClassSnapshot
	LastModified time.Time
	DeleteMarker bool
}

type ObjectVersion struct {
	BucketID             string
	Key                  string
	VersionID            string
	VersionSortKey       string
	SizeBytes            int64
	ETag                 string
	ContentType          string
	StorageClass         storage.StorageClassSnapshot
	ServerSideEncryption ServerSideEncryption
	SegmentRef           storage.SegmentRef
	SegmentRefs          []storage.SegmentRef
	Manifest             ObjectManifestDescriptor
	UserMetadata         map[string]string
	Tags                 map[string]string
	ObjectLockRetention  ObjectLockRetention
	ObjectLockLegalHold  ObjectLockLegalHoldStatus
	State                ObjectVersionState
	CreatedAt            time.Time
	CommittedAt          time.Time
	DeleteMarker         bool
}

type PendingObjectVersion struct {
	Version           ObjectVersion
	BaseHeadVersionID string
	BaseHead          ObjectHead
	BaseHeadFound     bool
}

type DeleteResult struct {
	Deleted          bool
	DeletedVersionID string
	DeleteMarker     bool
	DeletedVersion   ObjectVersion
}

type ListObjectsResult struct {
	Contents              []ObjectHead
	CommonPrefixes        []string
	IsTruncated           bool
	NextContinuationToken string
}

type ObjectVersionEntry struct {
	Version  ObjectVersion
	IsLatest bool
}

type ListObjectVersionsResult struct {
	Versions            []ObjectVersionEntry
	DeleteMarkers       []ObjectVersionEntry
	CommonPrefixes      []string
	IsTruncated         bool
	NextKeyMarker       string
	NextVersionIDMarker string
}

type ListMultipartUploadsResult struct {
	Uploads            []MultipartUpload
	CommonPrefixes     []string
	IsTruncated        bool
	NextKeyMarker      string
	NextUploadIDMarker string
}

type MultipartUploadState string

const (
	MultipartUploadActive    MultipartUploadState = "active"
	MultipartUploadCompleted MultipartUploadState = "completed"
	MultipartUploadAborted   MultipartUploadState = "aborted"
)

type MultipartPartsCleanupState string

const (
	MultipartPartsCleanupPending MultipartPartsCleanupState = "pending"
	MultipartPartsCleanupDone    MultipartPartsCleanupState = "done"
)

type MultipartCompletionState string

const (
	MultipartCompletionPrepared  MultipartCompletionState = "prepared"
	MultipartCompletionPublished MultipartCompletionState = "published"
	MultipartCompletionCompleted MultipartCompletionState = "completed"
)

type MultipartUpload struct {
	UploadID              string
	BucketID              string
	Key                   string
	ContentType           string
	StorageClass          storage.StorageClassSnapshot
	ServerSideEncryption  ServerSideEncryption
	UserMetadata          map[string]string
	Tags                  map[string]string
	ObjectLockRetention   ObjectLockRetention
	ObjectLockLegalHold   ObjectLockLegalHoldStatus
	State                 MultipartUploadState
	PartCount             int
	TotalPartSizeBytes    int64
	MaxPartNumber         int
	PartsUpdatedAt        time.Time
	CompletedVersionID    string
	CompletedETag         string
	CompletedSizeBytes    int64
	CompletedPartCount    int
	CompletedAt           time.Time
	PartsCleanupState     MultipartPartsCleanupState
	PartsCleanupDeleted   int
	PartsCleanupUpdatedAt time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type MultipartCompletionRecord struct {
	BucketID              string
	Key                   string
	UploadID              string
	ObjectVersionID       string
	ExpectedHeadVersionID string
	ETag                  string
	SizeBytes             int64
	PartCount             int
	State                 MultipartCompletionState
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type MultipartPart struct {
	UploadID   string
	PartNumber int
	SizeBytes  int64
	ETag       string
	SegmentRef storage.SegmentRef
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type StorageClass struct {
	StorageClassID string
	DisplayName    string
	CreatedAt      time.Time
}

type VolumePoolState string

const (
	VolumePoolStateActive   VolumePoolState = "active"
	VolumePoolStateReadOnly VolumePoolState = "read_only"
	VolumePoolStateDraining VolumePoolState = "draining"
	VolumePoolStateDegraded VolumePoolState = "degraded"
	VolumePoolStateFull     VolumePoolState = "full"
	VolumePoolStateOffline  VolumePoolState = "offline"
)

type VolumePool struct {
	PoolID          string
	Generation      uint64
	DurabilityClass string
	StorageClassIDs []string
	Members         []VolumePoolMember
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type VolumePoolMember struct {
	VolumeID             string
	AdminEndpoint        string
	DataEndpoint         string
	GatewayID            string
	AttachmentID         string
	Generation           uint64
	ChunkSizeBytes       uint64
	State                VolumePoolState
	ReadOnly             bool
	Weight               int
	CapacityBytes        uint64
	AvailableBytes       uint64
	UsedPercent          float64
	HighWatermarkPercent float64
	LastObservedAt       time.Time
}

type WorkerLease struct {
	LeaseID    string
	WorkerKind string
	ShardID    string
	OwnerID    string
	Generation uint64
	Cursor     string
	AcquiredAt time.Time
	UpdatedAt  time.Time
	ExpiresAt  time.Time
}

type WorkerOperationStatus string

const (
	WorkerOperationRunning      WorkerOperationStatus = "running"
	WorkerOperationSucceeded    WorkerOperationStatus = "succeeded"
	WorkerOperationRetryPending WorkerOperationStatus = "retry_pending"
	WorkerOperationFailed       WorkerOperationStatus = "failed"
	WorkerOperationCanceled     WorkerOperationStatus = "canceled"
	WorkerOperationPaused       WorkerOperationStatus = "paused"
)

type WorkerControlState string

const (
	WorkerControlActive   WorkerControlState = "active"
	WorkerControlPaused   WorkerControlState = "paused"
	WorkerControlCanceled WorkerControlState = "canceled"
)

type WorkerOperationRecord struct {
	OperationID string
	WorkerKind  string
	ShardID     string
	OwnerID     string
	LeaseID     string
	Status      WorkerOperationStatus
	Cursor      string
	Scanned     int
	Processed   int
	Skipped     int
	Retryable   int
	LastError   string
	StartedAt   time.Time
	FinishedAt  time.Time
	CreatedAt   time.Time
}

type WorkerControlRecord struct {
	WorkerKind string
	ShardID    string
	State      WorkerControlState
	Reason     string
	UpdatedBy  string
	UpdatedAt  time.Time
	CreatedAt  time.Time
}

type IdempotencyRecord struct {
	Scope          string
	IdempotencyKey string
	ResultRef      string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type GCIntent struct {
	IntentID   string
	ObjectID   string
	Reason     string
	CreatedAt  time.Time
	RetryAfter time.Time
}

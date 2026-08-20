package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nosway/namros/internal/adminstatus"
	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/cliflag"
	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/iam"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	pebblemeta "github.com/nosway/namros/internal/meta/pebble"
	tikvmeta "github.com/nosway/namros/internal/meta/tikv"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storage/local"
	sbsegments "github.com/nosway/namros/internal/storage/sbs"
	"github.com/nosway/namros/internal/storage/volumepool"
)

type adminCommand struct {
	stdout io.Writer
	stderr io.Writer
}

type stringListFlag []string

func (f *stringListFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *stringListFlag) Set(value string) error {
	for _, item := range splitCommaList(value) {
		*f = append(*f, item)
	}
	return nil
}

type repeatedStringFlag []string

func (f *repeatedStringFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ";")
}

func (f *repeatedStringFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value != "" {
		*f = append(*f, value)
	}
	return nil
}

func (c adminCommand) defaultConfig() config.Config {
	return config.Default()
}

type sharedObjectReleaseOperationOutput struct {
	ReleaseID      string `json:"release_id"`
	SharedObjectID string `json:"shared_object_id"`
	SegmentID      string `json:"segment_id"`
	Reason         string `json:"reason"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type metadataExportOutput struct {
	SchemaVersion               int                                  `json:"schema_version"`
	GeneratedAt                 string                               `json:"generated_at"`
	Limit                       int                                  `json:"limit"`
	MetadataSchema              *metadataSchemaExportOutput          `json:"metadata_schema,omitempty"`
	MetadataMigrationOperations []metadataMigrationOperationOutput   `json:"metadata_migration_operations,omitempty"`
	KMSKeys                     []kmsKeyExportOutput                 `json:"kms_keys,omitempty"`
	AuditEvents                 []auditEventExportOutput             `json:"audit_events,omitempty"`
	GCOperations                []gcOperationExportOutput            `json:"gc_operations,omitempty"`
	DedupeOperations            []dedupeOperationOutput              `json:"dedupe_operations,omitempty"`
	SharedObjects               []sharedObjectExportOutput           `json:"shared_objects,omitempty"`
	SharedObjectReleases        []sharedObjectReleaseOperationOutput `json:"shared_object_releases,omitempty"`
	VolumePools                 []volumePoolOutput                   `json:"volume_pools,omitempty"`
	VolumeDrainOperations       []volumeDrainOperationOutput         `json:"volume_drain_operations,omitempty"`
	WorkerLeases                []workerLeaseOutput                  `json:"worker_leases,omitempty"`
	WorkerOperations            []workerOperationOutput              `json:"worker_operations,omitempty"`
}

type metadataImportDryRunOutput struct {
	SchemaVersion  int                        `json:"schema_version"`
	DryRun         bool                       `json:"dry_run"`
	Valid          bool                       `json:"valid"`
	ReadyForApply  bool                       `json:"ready_for_apply"`
	Source         string                     `json:"source"`
	Counts         metadataCollectionCount    `json:"counts"`
	TargetChecked  bool                       `json:"target_checked"`
	TargetEmpty    bool                       `json:"target_empty"`
	TargetCounts   metadataCollectionCount    `json:"target_counts"`
	ConflictPolicy string                     `json:"conflict_policy"`
	ApplyRequested bool                       `json:"apply_requested,omitempty"`
	ApplyPlan      metadataImportApplyPlan    `json:"apply_plan"`
	ApplyResult    *metadataImportApplyResult `json:"apply_result,omitempty"`
	Actions        []metadataImportAction     `json:"actions,omitempty"`
	Conflicts      []metadataImportConflict   `json:"conflicts,omitempty"`
}

type metadataListIndexRepairOutput struct {
	SchemaVersion string                      `json:"schema_version"`
	GeneratedAt   string                      `json:"generated_at"`
	BucketID      string                      `json:"bucket_id"`
	Bucket        string                      `json:"bucket,omitempty"`
	Limit         int                         `json:"limit"`
	DryRun        bool                        `json:"dry_run"`
	Apply         bool                        `json:"apply"`
	Status        string                      `json:"status"`
	RepairNeeded  bool                        `json:"repair_needed"`
	AuditRecorded bool                        `json:"audit_recorded,omitempty"`
	Result        model.ListIndexRepairResult `json:"result"`
}

type metadataSchemaExportOutput struct {
	SchemaVersion    int    `json:"schema_version"`
	MinReaderVersion int    `json:"min_reader_version"`
	MinWriterVersion int    `json:"min_writer_version"`
	UpdatedBy        string `json:"updated_by,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type metadataMigrationOutput struct {
	SchemaVersion       string                             `json:"schema_version"`
	GeneratedAt         string                             `json:"generated_at"`
	Action              string                             `json:"action"`
	TargetSchemaVersion int                                `json:"target_schema_version,omitempty"`
	BucketID            string                             `json:"bucket_id,omitempty"`
	Bucket              string                             `json:"bucket,omitempty"`
	Limit               int                                `json:"limit,omitempty"`
	DryRun              bool                               `json:"dry_run,omitempty"`
	Apply               bool                               `json:"apply,omitempty"`
	ResumeOfOperationID string                             `json:"resume_of_operation_id,omitempty"`
	Status              string                             `json:"status,omitempty"`
	AuditRecorded       bool                               `json:"audit_recorded,omitempty"`
	SchemaPosture       *adminstatus.MetadataSchema        `json:"schema_posture,omitempty"`
	Operation           *metadataMigrationOperationOutput  `json:"operation,omitempty"`
	Operations          []metadataMigrationOperationOutput `json:"operations,omitempty"`
}

type metadataMigrationOperationOutput struct {
	OperationID         string                        `json:"operation_id"`
	ResumeOfOperationID string                        `json:"resume_of_operation_id,omitempty"`
	TargetSchemaVersion int                           `json:"target_schema_version"`
	Status              string                        `json:"status"`
	DryRun              bool                          `json:"dry_run"`
	Apply               bool                          `json:"apply"`
	OwnerID             string                        `json:"owner_id,omitempty"`
	Cursor              string                        `json:"cursor,omitempty"`
	Steps               []metadataMigrationStepOutput `json:"steps,omitempty"`
	StartedAt           string                        `json:"started_at,omitempty"`
	FinishedAt          string                        `json:"finished_at,omitempty"`
	CreatedAt           string                        `json:"created_at,omitempty"`
}

type metadataMigrationStepOutput struct {
	Name            string `json:"name"`
	Status          string `json:"status"`
	Message         string `json:"message,omitempty"`
	RepairNeeded    bool   `json:"repair_needed,omitempty"`
	RecordsScanned  int    `json:"records_scanned,omitempty"`
	RecordsRepaired int    `json:"records_repaired,omitempty"`
}

type metadataRestoreValidateOutput struct {
	SchemaVersion string                                `json:"schema_version"`
	GeneratedAt   string                                `json:"generated_at"`
	BucketID      string                                `json:"bucket_id"`
	Bucket        string                                `json:"bucket,omitempty"`
	Prefix        string                                `json:"prefix,omitempty"`
	Limit         int                                   `json:"limit"`
	Status        string                                `json:"status"`
	Sampled       int                                   `json:"sampled"`
	Verified      int                                   `json:"verified"`
	Failed        int                                   `json:"failed"`
	Samples       []metadataRestoreValidateSampleOutput `json:"samples,omitempty"`
}

type metadataRestoreValidateSampleOutput struct {
	Key                   string   `json:"key"`
	VersionID             string   `json:"version_id"`
	SizeBytes             int64    `json:"size_bytes"`
	ServerSideEncryption  string   `json:"server_side_encryption,omitempty"`
	KMSKeyID              string   `json:"kms_key_id,omitempty"`
	KMSKeyVersion         string   `json:"kms_key_version,omitempty"`
	SegmentCount          int      `json:"segment_count"`
	EncryptedSegmentCount int      `json:"encrypted_segment_count,omitempty"`
	VolumeIDs             []string `json:"volume_ids,omitempty"`
	ListIndexMatch        bool     `json:"list_index_match"`
	DigestMatch           bool     `json:"digest_match"`
	Status                string   `json:"status"`
	Error                 string   `json:"error,omitempty"`
}

type metadataCollectionCount struct {
	MetadataSchema              int `json:"metadata_schema"`
	MetadataMigrationOperations int `json:"metadata_migration_operations"`
	KMSKeys                     int `json:"kms_keys"`
	AuditEvents                 int `json:"audit_events"`
	GCOperations                int `json:"gc_operations"`
	DedupeOperations            int `json:"dedupe_operations"`
	SharedObjects               int `json:"shared_objects"`
	SharedObjectReleases        int `json:"shared_object_releases"`
	VolumePools                 int `json:"volume_pools"`
	VolumeDrainOperations       int `json:"volume_drain_operations"`
	WorkerLeases                int `json:"worker_leases"`
	WorkerOperations            int `json:"worker_operations"`
}

type metadataImportAction struct {
	Collection     string `json:"collection"`
	Operation      string `json:"operation"`
	ImportRecords  int    `json:"import_records"`
	TargetRecords  int    `json:"target_records"`
	Policy         string `json:"policy"`
	PreserveIDs    bool   `json:"preserve_ids"`
	PreserveHashes bool   `json:"preserve_hashes,omitempty"`
	WriteEnabled   bool   `json:"write_enabled"`
}

type metadataImportConflict struct {
	Collection string `json:"collection"`
	ID         string `json:"id,omitempty"`
	Reason     string `json:"reason"`
}

type metadataImportApplyPlan struct {
	Status              string                    `json:"status"`
	WriteEnabled        bool                      `json:"write_enabled"`
	ApplySupported      bool                      `json:"apply_supported"`
	Ready               bool                      `json:"ready"`
	ConflictPolicy      string                    `json:"conflict_policy"`
	RequireEmptyTarget  bool                      `json:"require_empty_target"`
	PreserveSourceIDs   bool                      `json:"preserve_source_ids"`
	PreserveAuditHashes bool                      `json:"preserve_audit_hashes"`
	Gates               []metadataImportApplyGate `json:"gates,omitempty"`
	Limitations         []string                  `json:"limitations,omitempty"`
}

type metadataImportApplyGate struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type metadataImportApplyResult struct {
	Status              string                                `json:"status"`
	WriteEnabled        bool                                  `json:"write_enabled"`
	ApplySupported      bool                                  `json:"apply_supported"`
	Ready               bool                                  `json:"ready"`
	ExperimentalAllowed bool                                  `json:"experimental_allowed"`
	RecordsPlanned      int                                   `json:"records_planned"`
	RecordsWritten      int                                   `json:"records_written"`
	Message             string                                `json:"message"`
	Collections         []metadataImportApplyCollectionResult `json:"collections,omitempty"`
	Limitations         []string                              `json:"limitations,omitempty"`
}

type metadataImportApplyCollectionResult struct {
	Collection     string `json:"collection"`
	Status         string `json:"status"`
	Operation      string `json:"operation"`
	RecordsPlanned int    `json:"records_planned"`
	RecordsWritten int    `json:"records_written"`
	PreserveIDs    bool   `json:"preserve_ids"`
	PreserveHashes bool   `json:"preserve_hashes,omitempty"`
}

type metadataImportDryRunRequest struct {
	InputPath          string
	Target             meta.Repository
	TargetChecked      bool
	TargetScanLimit    int
	RequireEmptyTarget bool
	ConflictPolicy     string
}

type kmsKeyExportOutput struct {
	KeyID      string `json:"key_id"`
	KeyVersion string `json:"key_version,omitempty"`
	State      string `json:"state"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type auditEventExportOutput struct {
	EventID      string               `json:"event_id"`
	Action       string               `json:"action"`
	BucketID     string               `json:"bucket_id,omitempty"`
	Key          string               `json:"key,omitempty"`
	VersionID    string               `json:"version_id,omitempty"`
	RequestID    string               `json:"request_id,omitempty"`
	Reason       string               `json:"reason,omitempty"`
	Principal    model.AuditPrincipal `json:"principal,omitempty"`
	Details      map[string]string    `json:"details,omitempty"`
	PreviousHash string               `json:"previous_hash,omitempty"`
	EventHash    string               `json:"event_hash"`
	CreatedAt    string               `json:"created_at"`
}

type gcOperationExportOutput struct {
	OperationID         string                  `json:"operation_id"`
	ResumeOfOperationID string                  `json:"resume_of_operation_id,omitempty"`
	Status              model.GCOperationStatus `json:"status"`
	StartedAt           string                  `json:"started_at"`
	FinishedAt          string                  `json:"finished_at"`
	Scanned             int                     `json:"scanned"`
	Deleted             int                     `json:"deleted"`
	Skipped             int                     `json:"skipped"`
	Retryable           int                     `json:"retryable"`
	CreatedAt           string                  `json:"created_at"`
}

type sharedObjectExportOutput struct {
	SharedObjectID     string `json:"shared_object_id"`
	TenantID           string `json:"tenant_id"`
	BucketID           string `json:"bucket_id"`
	Key                string `json:"key"`
	SourceVersionID    string `json:"source_version_id"`
	SizeBytes          int64  `json:"size_bytes"`
	DigestAlgorithm    string `json:"digest_algorithm,omitempty"`
	DigestHex          string `json:"digest_hex,omitempty"`
	RefCount           int    `json:"ref_count"`
	ProtectedRootCount int    `json:"protected_root_count"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type dedupeOperationOutput struct {
	OperationID         string                         `json:"operation_id"`
	ResumeOfOperationID string                         `json:"resume_of_operation_id,omitempty"`
	Status              model.DedupeOperationStatus    `json:"status"`
	StartedAt           string                         `json:"started_at"`
	FinishedAt          string                         `json:"finished_at"`
	Scanned             int                            `json:"scanned"`
	Acked               int                            `json:"acked"`
	Skipped             int                            `json:"skipped"`
	Retryable           int                            `json:"retryable"`
	Attempts            []dedupeOperationAttemptOutput `json:"attempts,omitempty"`
	CreatedAt           string                         `json:"created_at"`
}

type dedupeOperationAttemptOutput struct {
	BucketID         string                             `json:"bucket_id"`
	Key              string                             `json:"key"`
	SourceVersion    string                             `json:"source_version"`
	CandidateVersion string                             `json:"candidate_version"`
	PlanStatus       string                             `json:"plan_status"`
	PlanReason       string                             `json:"plan_reason,omitempty"`
	Status           model.DedupeOperationAttemptStatus `json:"status"`
	SharedObjectID   string                             `json:"shared_object_id,omitempty"`
	OrphansMarked    int                                `json:"orphans_marked,omitempty"`
	Retryable        bool                               `json:"retryable,omitempty"`
	Error            string                             `json:"error,omitempty"`
}

type workerOperationsOutput struct {
	SchemaVersion string                  `json:"schema_version"`
	GeneratedAt   string                  `json:"generated_at"`
	Limit         int                     `json:"limit"`
	WorkerKind    string                  `json:"worker_kind,omitempty"`
	ShardID       string                  `json:"shard_id,omitempty"`
	Status        string                  `json:"status,omitempty"`
	Operations    []workerOperationOutput `json:"operations"`
}

type workerControlCommandOutput struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Action        string              `json:"action"`
	Control       workerControlOutput `json:"control"`
}

type workerControlOutput struct {
	WorkerKind string `json:"worker_kind"`
	ShardID    string `json:"shard_id"`
	State      string `json:"state"`
	Reason     string `json:"reason,omitempty"`
	UpdatedBy  string `json:"updated_by,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

type workerLeaseOutput struct {
	LeaseID    string `json:"lease_id"`
	WorkerKind string `json:"worker_kind"`
	ShardID    string `json:"shard_id"`
	OwnerID    string `json:"owner_id"`
	Generation uint64 `json:"generation"`
	Cursor     string `json:"cursor,omitempty"`
	AcquiredAt string `json:"acquired_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
	ExpiresAt  string `json:"expires_at,omitempty"`
}

type workerOperationOutput struct {
	OperationID string `json:"operation_id"`
	WorkerKind  string `json:"worker_kind"`
	ShardID     string `json:"shard_id"`
	OwnerID     string `json:"owner_id"`
	LeaseID     string `json:"lease_id,omitempty"`
	Status      string `json:"status"`
	Cursor      string `json:"cursor,omitempty"`
	Scanned     int    `json:"scanned"`
	Processed   int    `json:"processed"`
	Skipped     int    `json:"skipped"`
	Retryable   int    `json:"retryable"`
	LastError   string `json:"last_error,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	FinishedAt  string `json:"finished_at,omitempty"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type gcCandidatesOutput struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Limit         int                 `json:"limit"`
	Candidates    []gcCandidateOutput `json:"candidates"`
}

type gcCandidateSeedObjectOutput struct {
	SchemaVersion  string              `json:"schema_version"`
	GeneratedAt    string              `json:"generated_at"`
	Bucket         string              `json:"bucket"`
	Key            string              `json:"key"`
	VersionID      string              `json:"version_id"`
	Reason         string              `json:"reason"`
	CandidateCount int                 `json:"candidate_count"`
	Candidates     []gcCandidateOutput `json:"candidates"`
}

type gcCandidateOutput struct {
	SegmentID string `json:"segment_id"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at,omitempty"`
	VolumeID  string `json:"volume_id,omitempty"`
	Backend   string `json:"backend,omitempty"`
	Layout    string `json:"layout,omitempty"`
	SizeBytes uint64 `json:"size_bytes,omitempty"`
}

type bucketQuotaOutput struct {
	SchemaVersion      string `json:"schema_version"`
	GeneratedAt        string `json:"generated_at"`
	BucketID           string `json:"bucket_id"`
	Bucket             string `json:"bucket,omitempty"`
	Configured         bool   `json:"configured"`
	Deleted            bool   `json:"deleted,omitempty"`
	MaxObjectSizeBytes int64  `json:"max_object_size_bytes,omitempty"`
	CreatedAt          string `json:"created_at,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

type tenantQuotaOutput struct {
	SchemaVersion    string `json:"schema_version"`
	GeneratedAt      string `json:"generated_at"`
	TenantID         string `json:"tenant_id"`
	Configured       bool   `json:"configured"`
	Deleted          bool   `json:"deleted,omitempty"`
	MaxBytes         int64  `json:"max_bytes,omitempty"`
	MaxObjects       int64  `json:"max_objects,omitempty"`
	MaxActiveUploads int64  `json:"max_active_uploads,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	UpdatedAt        string `json:"updated_at,omitempty"`
}

type volumePoolOutput struct {
	PoolID          string                   `json:"pool_id"`
	Generation      uint64                   `json:"generation"`
	DurabilityClass string                   `json:"durability_class,omitempty"`
	StorageClassIDs []string                 `json:"storage_class_ids,omitempty"`
	Members         []volumePoolMemberOutput `json:"members"`
	CreatedAt       string                   `json:"created_at,omitempty"`
	UpdatedAt       string                   `json:"updated_at,omitempty"`
}

type volumePoolMemberOutput struct {
	VolumeID             string  `json:"volume_id"`
	AdminEndpoint        string  `json:"admin_endpoint,omitempty"`
	DataEndpoint         string  `json:"data_endpoint,omitempty"`
	GatewayID            string  `json:"gateway_id,omitempty"`
	AttachmentID         string  `json:"attachment_id,omitempty"`
	Generation           uint64  `json:"generation,omitempty"`
	ChunkSizeBytes       uint64  `json:"chunk_size_bytes,omitempty"`
	State                string  `json:"state"`
	ReadOnly             bool    `json:"read_only,omitempty"`
	Weight               int     `json:"weight,omitempty"`
	CapacityBytes        uint64  `json:"capacity_bytes,omitempty"`
	AvailableBytes       uint64  `json:"available_bytes,omitempty"`
	UsedPercent          float64 `json:"used_percent,omitempty"`
	HighWatermarkPercent float64 `json:"high_watermark_percent,omitempty"`
	LastObservedAt       string  `json:"last_observed_at,omitempty"`
}

type volumeDrainOperationsOutput struct {
	SchemaVersion  string                       `json:"schema_version"`
	GeneratedAt    string                       `json:"generated_at"`
	Action         string                       `json:"action"`
	Limit          int                          `json:"limit,omitempty"`
	SourceVolumeID string                       `json:"source_volume_id,omitempty"`
	TargetVolumeID string                       `json:"target_volume_id,omitempty"`
	Status         string                       `json:"status,omitempty"`
	Operation      *volumeDrainOperationOutput  `json:"operation,omitempty"`
	Operations     []volumeDrainOperationOutput `json:"operations,omitempty"`
}

type volumeDrainOperationOutput struct {
	OperationID         string                     `json:"operation_id"`
	ResumeOfOperationID string                     `json:"resume_of_operation_id,omitempty"`
	PoolID              string                     `json:"pool_id,omitempty"`
	SourceVolumeID      string                     `json:"source_volume_id"`
	TargetVolumeID      string                     `json:"target_volume_id,omitempty"`
	OwnerID             string                     `json:"owner_id,omitempty"`
	Status              string                     `json:"status"`
	Cursor              string                     `json:"cursor,omitempty"`
	StartedAt           string                     `json:"started_at,omitempty"`
	FinishedAt          string                     `json:"finished_at,omitempty"`
	Scanned             int                        `json:"scanned"`
	Copied              int                        `json:"copied"`
	Skipped             int                        `json:"skipped"`
	Protected           int                        `json:"protected"`
	Retryable           int                        `json:"retryable"`
	Attempts            []volumeDrainAttemptOutput `json:"attempts,omitempty"`
	CreatedAt           string                     `json:"created_at,omitempty"`
}

type volumeDrainAttemptOutput struct {
	BucketID        string `json:"bucket_id,omitempty"`
	Key             string `json:"key,omitempty"`
	VersionID       string `json:"version_id,omitempty"`
	SourceSegmentID string `json:"source_segment_id,omitempty"`
	SourceVolumeID  string `json:"source_volume_id,omitempty"`
	TargetSegmentID string `json:"target_segment_id,omitempty"`
	TargetVolumeID  string `json:"target_volume_id,omitempty"`
	Status          string `json:"status"`
	Protected       bool   `json:"protected,omitempty"`
	Retryable       bool   `json:"retryable,omitempty"`
	Error           string `json:"error,omitempty"`
}

func main() {
	cmd := adminCommand{stdout: os.Stdout, stderr: os.Stderr}
	if err := cmd.run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "namros-admin: %v\n", err)
		os.Exit(1)
	}
}

func (c adminCommand) run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: namros-admin status|metadata-scale-budget|metadata-list-index-repair|metadata-migration|metadata-restore-validate|volume-pool-put|volume-drain-operations|bucket-quota-put|bucket-quota-get|bucket-quota-delete|tenant-quota-put|tenant-quota-get|tenant-quota-delete|worker-operations|worker-control|gc-candidates|gc-candidate-seed-object|iam-principal-inspect|iam-policy-simulate|iam-mapping-validate|dedupe-plan|dedupe-ack|dedupe-ops|dedupe-repair|dedupe-scrub|metadata-export|metadata-import|kms-key-put|kms-key-list|compliance-evidence|compliance-profile-plan|compliance-policy-simulate [flags]")
	}
	if featureID, ok := enterpriseAdminCommandFeatures[args[0]]; ok {
		return c.runEnterpriseAdminCommand(args[0], featureID)
	}
	switch args[0] {
	case "status":
		return c.runStatus(ctx, args[1:])
	case "metadata-scale-budget":
		return c.runMetadataScaleBudget(args[1:])
	case "metadata-list-index-repair":
		return c.runMetadataListIndexRepair(ctx, args[1:])
	case "metadata-migration":
		return c.runMetadataMigration(ctx, args[1:])
	case "metadata-restore-validate":
		return c.runMetadataRestoreValidate(ctx, args[1:])
	case "volume-pool-put":
		return c.runVolumePoolPut(ctx, args[1:])
	case "volume-drain-operations":
		return c.runVolumeDrainOperations(ctx, args[1:])
	case "bucket-quota-put":
		return c.runBucketQuotaPut(ctx, args[1:])
	case "bucket-quota-get":
		return c.runBucketQuotaGet(ctx, args[1:])
	case "bucket-quota-delete":
		return c.runBucketQuotaDelete(ctx, args[1:])
	case "tenant-quota-put":
		return c.runTenantQuotaPut(ctx, args[1:])
	case "tenant-quota-get":
		return c.runTenantQuotaGet(ctx, args[1:])
	case "tenant-quota-delete":
		return c.runTenantQuotaDelete(ctx, args[1:])
	case "worker-operations":
		return c.runWorkerOperations(ctx, args[1:])
	case "worker-control":
		return c.runWorkerControl(ctx, args[1:])
	case "gc-candidates":
		return c.runGCCandidates(ctx, args[1:])
	case "gc-candidate-seed-object":
		return c.runGCCandidateSeedObject(ctx, args[1:])
	case "metadata-export":
		return c.runMetadataExport(ctx, args[1:])
	case "metadata-import":
		return c.runMetadataImport(ctx, args[1:])
	case "iam-principal-inspect":
		return c.runIAMPrincipalInspect(args[1:])
	case "iam-policy-simulate":
		return c.runIAMPolicySimulate(args[1:])
	case "iam-mapping-validate":
		return c.runIAMMappingValidate(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (c adminCommand) runMetadataScaleBudget(args []string) error {
	fs := flag.NewFlagSet("namros-admin metadata-scale-budget", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var req meta.MetadataScaleBudgetRequest
	fs.IntVar(&req.PartCount, "part-count", meta.MaxMultipartParts, "multipart part count to estimate")
	fs.IntVar(&req.SegmentRefCount, "segment-ref-count", 0, "object manifest segment ref count; defaults to part-count")
	fs.IntVar(&req.ChunksPerSegment, "chunks-per-segment", 1, "placement chunk records per segment ref")
	fs.IntVar(&req.ProtectedRefCount, "protected-ref-count", 0, "protected ref count to estimate; defaults to segment-ref-count")
	fs.IntVar(&req.GCCandidateCount, "gc-candidate-count", 0, "GC candidate count to estimate; defaults to segment-ref-count")
	fs.IntVar(&req.ValueBudgetBytes, "value-budget-bytes", meta.DefaultMetadataValueBudgetBytes, "metadata value budget in bytes")
	fs.IntVar(&req.CompleteTxnBudgetBytes, "complete-txn-budget-bytes", meta.DefaultMetadataTransactionBudgetBytes, "complete multipart transaction budget in bytes")
	fs.BoolVar(&req.IncludeListIndexWriteBytes, "include-list-index-write-bytes", true, "include list index head write in complete transaction estimate")
	fs.BoolVar(&req.IncludeProtectedRefBytes, "include-protected-ref-write-bytes", true, "include protected ref index writes in complete transaction estimate")
	fs.BoolVar(&req.IncludeGCCandidateBytes, "include-gc-candidate-write-bytes", true, "include GC candidate writes in complete transaction estimate")
	releaseGate := false
	failOnWatch := false
	fs.BoolVar(&releaseGate, "release-gate", false, "exit non-zero when metadata scale budget has over-budget gates; watch gates are printed as warnings")
	fs.BoolVar(&failOnWatch, "fail-on-watch", false, "treat metadata scale budget watch gates as release failures")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if failOnWatch {
		releaseGate = true
	}
	output, err := meta.EstimateMetadataScaleBudget(req)
	if err != nil {
		return err
	}
	output.ReleaseGate = meta.EvaluateMetadataScaleBudgetReleaseGate(output, failOnWatch)
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return err
	}
	if !releaseGate {
		return nil
	}
	if len(output.ReleaseGate.WarningGates) > 0 {
		fmt.Fprintf(c.stderr, "metadata-scale-budget: watch gates: %s\n", strings.Join(output.ReleaseGate.WarningGates, ","))
	}
	if output.ReleaseGate.Status == "failed" {
		return fmt.Errorf("metadata scale budget release gate failed: %s", strings.Join(output.ReleaseGate.FailedGates, ","))
	}
	return nil
}

func (c adminCommand) runMetadataListIndexRepair(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin metadata-list-index-repair", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var bucketName string
	var bucketID string
	var limit int
	var dryRun bool
	var apply bool
	var auditAdminOperation bool
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&bucketName, "bucket", "", "bucket name whose metadata list indexes should be checked")
	fs.StringVar(&bucketID, "bucket-id", "", "bucket id; optional alternative to bucket name")
	fs.IntVar(&limit, "limit", 1000, "maximum records to scan per metadata key family")
	fs.BoolVar(&dryRun, "dry-run", true, "detect index drift without writing repairs")
	fs.BoolVar(&apply, "apply", false, "repair missing and stale list indexes")
	fs.BoolVar(&auditAdminOperation, "audit-admin-operation", false, "record this admin command in the metadata audit chain")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	if apply {
		dryRun = false
	}
	if !dryRun && !apply {
		return fmt.Errorf("metadata list index repair requires either -dry-run or -apply")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	bucket, err := resolveAdminBucket(ctx, repo, bucketName, bucketID)
	if err != nil {
		return err
	}
	result, err := repo.RepairListIndexes(ctx, meta.RepairListIndexesRequest{
		BucketID: bucket.BucketID,
		Limit:    limit,
		Apply:    apply,
	})
	if err != nil {
		return err
	}
	output := metadataListIndexRepairOutput{
		SchemaVersion: "namros.admin.metadata_list_index_repair.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		BucketID:      bucket.BucketID,
		Bucket:        bucket.Name,
		Limit:         limit,
		DryRun:        dryRun,
		Apply:         apply,
		Status:        metadataListIndexRepairStatus(result, apply),
		RepairNeeded:  metadataListIndexRepairNeeded(result),
		Result:        result,
	}
	if auditAdminOperation {
		if err := putAdminAuditEvent(ctx, repo, model.AuditActionAdminMetadataListIndexRepair, "namros-admin metadata-list-index-repair", metadataListIndexRepairAuditDetails(output)); err != nil {
			return err
		}
		output.AuditRecorded = true
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runMetadataMigration(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin metadata-migration", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var action string
	var bucketName string
	var bucketID string
	var limit int
	var targetSchemaVersion int
	var resumeOfOperationID string
	var statusRaw string
	var ownerID string
	var auditAdminOperation bool
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&action, "action", "plan", "metadata migration action: plan, apply, resume, or list")
	fs.StringVar(&bucketName, "bucket", "", "bucket name whose manifest/list indexes should be migrated")
	fs.StringVar(&bucketID, "bucket-id", "", "bucket id; optional alternative to bucket name")
	fs.IntVar(&limit, "limit", 1000, "maximum records to scan per migration step")
	fs.IntVar(&targetSchemaVersion, "target-schema-version", meta.CurrentMetadataSchemaVersion, "metadata schema version to converge to")
	fs.StringVar(&resumeOfOperationID, "resume-of-operation-id", "", "prior metadata migration operation id for resume/apply records")
	fs.StringVar(&statusRaw, "status", "", "operation status filter for list")
	fs.StringVar(&ownerID, "owner-id", "namros-admin", "admin owner id recorded on migration operations")
	fs.BoolVar(&auditAdminOperation, "audit-admin-operation", false, "record this admin command in the metadata audit chain")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if targetSchemaVersion <= 0 {
		return fmt.Errorf("target-schema-version must be positive")
	}
	if targetSchemaVersion > meta.CurrentMetadataSchemaVersion {
		return fmt.Errorf("target-schema-version %d is newer than supported metadata schema version %d", targetSchemaVersion, meta.CurrentMetadataSchemaVersion)
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	status, err := parseMetadataMigrationOperationStatus(statusRaw)
	if err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	action = strings.ToLower(strings.TrimSpace(action))
	output := metadataMigrationOutput{
		SchemaVersion:       "namros.admin.metadata_migration.v1",
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Action:              action,
		TargetSchemaVersion: targetSchemaVersion,
		Limit:               limit,
		ResumeOfOperationID: strings.TrimSpace(resumeOfOperationID),
	}
	switch action {
	case "list":
		records, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{
			Status: status,
			Limit:  limit,
		})
		if err != nil {
			return err
		}
		output.Status = string(status)
		output.Operations = metadataMigrationOperationOutputs(records)
	case "plan", "apply", "resume":
		if action == "resume" && strings.TrimSpace(resumeOfOperationID) == "" {
			return fmt.Errorf("resume requires -resume-of-operation-id")
		}
		bucket, err := resolveAdminBucket(ctx, repo, bucketName, bucketID)
		if err != nil {
			return err
		}
		output.BucketID = bucket.BucketID
		output.Bucket = bucket.Name
		apply := action == "apply" || action == "resume"
		dryRun := !apply
		output.DryRun = dryRun
		output.Apply = apply

		posture := meta.CheckMetadataSchema(ctx, repo)
		schemaPosture := adminstatus.MetadataSchemaFromPosture(posture)
		output.SchemaPosture = &schemaPosture

		startedAt := time.Now().UTC()
		repairResult, err := repo.RepairListIndexes(ctx, meta.RepairListIndexesRequest{
			BucketID: bucket.BucketID,
			Limit:    limit,
			Apply:    apply,
		})
		finishedAt := time.Now().UTC()
		steps := []model.MetadataMigrationStep{
			metadataMigrationSchemaStep(posture, targetSchemaVersion, apply),
		}
		if err != nil {
			steps = append(steps, model.MetadataMigrationStep{
				Name:    "list_index_repair",
				Status:  model.MetadataMigrationStepFailed,
				Message: err.Error(),
			})
		} else {
			steps = append(steps, metadataMigrationListIndexStep(repairResult, apply))
		}
		operationStatus := metadataMigrationStatusForSteps(steps, apply, err)
		if err == nil && apply {
			if _, schemaErr := repo.PutMetadataSchema(ctx, meta.PutMetadataSchemaRequest{
				SchemaVersion:    targetSchemaVersion,
				MinReaderVersion: meta.MinimumMetadataReaderVersion,
				MinWriterVersion: meta.MinimumMetadataWriterVersion,
				UpdatedBy:        strings.TrimSpace(ownerID),
			}); schemaErr != nil {
				steps = append(steps, model.MetadataMigrationStep{
					Name:    "schema_marker_update",
					Status:  model.MetadataMigrationStepFailed,
					Message: schemaErr.Error(),
				})
				operationStatus = model.MetadataMigrationOperationFailed
				err = schemaErr
			} else {
				steps = append(steps, model.MetadataMigrationStep{
					Name:    "schema_marker_update",
					Status:  model.MetadataMigrationStepSucceeded,
					Message: "metadata schema marker updated",
				})
			}
		}
		record, recordErr := repo.PutMetadataMigrationOperation(ctx, meta.PutMetadataMigrationOperationRequest{
			ResumeOfOperationID: strings.TrimSpace(resumeOfOperationID),
			TargetSchemaVersion: targetSchemaVersion,
			Status:              operationStatus,
			DryRun:              dryRun,
			Apply:               apply,
			OwnerID:             strings.TrimSpace(ownerID),
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			Steps:               steps,
		})
		if recordErr != nil {
			return recordErr
		}
		operationOutput := metadataMigrationOperationOutputFromRecord(record)
		output.Operation = &operationOutput
		output.Status = string(record.Status)
		if auditAdminOperation {
			if auditErr := putAdminAuditEvent(ctx, repo, model.AuditActionAdminMetadataMigration, "namros-admin metadata-migration", metadataMigrationAuditDetails(output)); auditErr != nil {
				return auditErr
			}
			output.AuditRecorded = true
		}
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported metadata migration action %q", action)
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runMetadataRestoreValidate(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin metadata-restore-validate", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var bucketName string
	var bucketID string
	var prefix string
	var limit int
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")
	sbsShardStoreIDs := strings.Join(cfg.SBSShardStoreIDs, ",")
	var sbsVolumePool string
	sbsChunkIDAllocationCacheSize := uint64(cfg.SBSChunkIDAllocationCacheSize)

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&cfg.StorageBackend, "storage-backend", cfg.StorageBackend, "segment storage backend: local, sbs, sbs-local, sbs-physical, sbs-ec, or sbs-cluster")
	fs.StringVar(&cfg.StoragePath, "storage-path", cfg.StoragePath, "segment storage path for local or sbs-local backends")
	fs.StringVar(&cfg.SBSStatePath, "sbs-state-path", cfg.SBSStatePath, "SBS local adapter state path")
	cliflag.StringVarWithDeprecatedAlias(fs, &cfg.SBSAdminEndpoint, "sbs-service-endpoint", cfg.SBSAdminEndpoint, "SBS service gRPC endpoint for sbs-physical or sbs-cluster storage", "sbs-admin-endpoint")
	fs.StringVar(&cfg.SBSDataEndpoint, "sbs-data-endpoint", cfg.SBSDataEndpoint, "SBS data gRPC endpoint for SBS-backed storage")
	fs.StringVar(&cfg.SBSVolumeID, "sbs-volume-id", cfg.SBSVolumeID, "SBS volume id for SBS-backed storage")
	fs.StringVar(&sbsVolumePool, "sbs-volume-pool", "", "semicolon-separated static SBS volume pool members for restore validation")
	fs.Uint64Var(&cfg.SBSChunkSizeBytes, "sbs-chunk-size-bytes", cfg.SBSChunkSizeBytes, "SBS physical allocation chunk size")
	fs.StringVar(&cfg.SBSGatewayID, "sbs-gateway-id", cfg.SBSGatewayID, "SBS gateway id for SBS-backed storage attachment context")
	fs.StringVar(&cfg.SBSAttachmentID, "sbs-attachment-id", cfg.SBSAttachmentID, "SBS attachment id for SBS-backed storage writer context")
	fs.Uint64Var(&cfg.SBSGeneration, "sbs-generation", cfg.SBSGeneration, "SBS attachment generation for SBS-backed storage writer context")
	fs.StringVar(&cfg.SBSWriterGroupID, "sbs-writer-group-id", cfg.SBSWriterGroupID, "SBS shared writer group id for production multi-gateway session fencing")
	fs.StringVar(&cfg.SBSSessionID, "sbs-session-id", cfg.SBSSessionID, "SBS per-gateway writer session id for production multi-gateway session fencing")
	fs.Uint64Var(&cfg.SBSVolumeEpoch, "sbs-volume-epoch", cfg.SBSVolumeEpoch, "SBS volume epoch for production writer session fencing")
	fs.DurationVar(&cfg.SBSSessionTTL, "sbs-session-ttl", cfg.SBSSessionTTL, "SBS writer session lease TTL")
	fs.DurationVar(&cfg.SBSSessionHeartbeat, "sbs-session-heartbeat", cfg.SBSSessionHeartbeat, "SBS writer session heartbeat interval")
	fs.StringVar(&sbsShardStoreIDs, "sbs-shard-store-ids", sbsShardStoreIDs, "comma-separated SBS store ids for sbs-ec shard placement")
	fs.IntVar(&cfg.SBSECShardConcurrency, "sbs-ec-shard-concurrency", cfg.SBSECShardConcurrency, "maximum concurrent sbs-ec shard RPCs per segment read")
	fs.BoolVar(&cfg.SBSVerifyReadback, "sbs-verify-readback", cfg.SBSVerifyReadback, "verify sbs-physical writes with immediate readback")
	fs.IntVar(&cfg.SBSPhysicalWriteConcurrency, "sbs-physical-write-concurrency", cfg.SBSPhysicalWriteConcurrency, "maximum concurrent sbs-physical chunk writes")
	fs.Uint64Var(&cfg.SBSPhysicalFullChunkWriteMinBytes, "sbs-physical-full-chunk-write-min-bytes", cfg.SBSPhysicalFullChunkWriteMinBytes, "minimum SBS allocation chunk size eligible for full-chunk tail writes")
	fs.Uint64Var(&cfg.SBSPhysicalFullChunkWriteMaxBytes, "sbs-physical-full-chunk-write-max-bytes", cfg.SBSPhysicalFullChunkWriteMaxBytes, "maximum SBS allocation chunk size eligible for full-chunk tail writes")
	fs.Uint64Var(&cfg.SBSPhysicalChunkCacheBytes, "sbs-physical-chunk-cache-bytes", cfg.SBSPhysicalChunkCacheBytes, "gateway-local SBS physical chunk cache size in bytes")
	fs.Uint64Var(&sbsChunkIDAllocationCacheSize, "sbs-chunk-id-allocation-cache-size", sbsChunkIDAllocationCacheSize, "SBS physical chunk IDs to preallocate per volume on each cache refill")
	fs.StringVar(&cfg.GatewayInstanceID, "gateway-instance-id", cfg.GatewayInstanceID, "stable gateway instance id for writer session fencing")
	fs.StringVar(&bucketName, "bucket", "", "bucket name to sample")
	fs.StringVar(&bucketID, "bucket-id", "", "bucket id; optional alternative to bucket name")
	fs.StringVar(&prefix, "prefix", "", "object prefix to sample")
	fs.IntVar(&limit, "limit", 100, "maximum current object heads to sample")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if limit <= 0 {
		return fmt.Errorf("limit must be positive")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	cfg.SBSShardStoreIDs = splitCommaList(sbsShardStoreIDs)
	if sbsChunkIDAllocationCacheSize > uint64(^uint32(0)) {
		return fmt.Errorf("sbs chunk id allocation cache size exceeds uint32 max")
	}
	cfg.SBSChunkIDAllocationCacheSize = uint32(sbsChunkIDAllocationCacheSize)
	volumePool, err := config.ParseSBSVolumePoolSpec(sbsVolumePool)
	if err != nil {
		return err
	}
	cfg.SBSVolumePool = volumePool
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	cfg.StorageBackend = config.NormalizeStorageBackend(cfg.StorageBackend)
	store, closeStore, err := openRestoreValidationStorage(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStore()
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	bucket, err := resolveAdminBucket(ctx, repo, bucketName, bucketID)
	if err != nil {
		return err
	}
	listed, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
		BucketID: bucket.BucketID,
		Prefix:   strings.TrimSpace(prefix),
		MaxKeys:  limit,
	})
	if err != nil {
		return err
	}
	output := metadataRestoreValidateOutput{
		SchemaVersion: "namros.admin.metadata_restore_validate.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		BucketID:      bucket.BucketID,
		Bucket:        bucket.Name,
		Prefix:        strings.TrimSpace(prefix),
		Limit:         limit,
		Status:        "passed",
	}
	for _, head := range listed.Contents {
		sample := validateRestoreSample(ctx, repo, store, head)
		output.Samples = append(output.Samples, sample)
		output.Sampled++
		if sample.Status == "verified" {
			output.Verified++
		} else {
			output.Failed++
			output.Status = "failed"
		}
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runVolumePoolPut(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin volume-pool-put", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var poolID string
	var generation uint64
	var durabilityClass string
	var storageClassIDs stringListFlag
	var memberSpecs repeatedStringFlag
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&poolID, "pool-id", "", "volume pool id")
	fs.Uint64Var(&generation, "generation", 0, "volume pool generation; 0 auto-increments")
	fs.StringVar(&durabilityClass, "durability-class", "", "durability class label")
	fs.Var(&storageClassIDs, "storage-class", "storage class id bound to this pool; repeatable or comma-separated")
	fs.Var(&memberSpecs, "member", "volume pool member spec; repeatable, e.g. volume_id=18a00001,admin_endpoint=sbs-admin-a:9443,data_endpoint=sbs-data-a:9460,state=active")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if strings.TrimSpace(poolID) == "" {
		return fmt.Errorf("pool id is required")
	}
	members, err := parseVolumePoolMemberSpecs(memberSpecs)
	if err != nil {
		return err
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	pool, err := repo.PutVolumePool(ctx, meta.PutVolumePoolRequest{
		PoolID:          poolID,
		Generation:      generation,
		DurabilityClass: durabilityClass,
		StorageClassIDs: storageClassIDs,
		Members:         members,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(volumePoolOutputFromModel(pool))
}

func (c adminCommand) runVolumeDrainOperations(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin volume-drain-operations", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var action string
	var limit int
	var resumeOfOperationID string
	var poolID string
	var sourceVolumeID string
	var targetVolumeID string
	var ownerID string
	var statusRaw string
	var cursor string
	var scanned int
	var copied int
	var skipped int
	var protected int
	var retryable int
	var attemptSpecs repeatedStringFlag
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&action, "action", "list", "volume drain action: list or put")
	fs.IntVar(&limit, "limit", 20, "maximum volume drain operation records to include")
	fs.StringVar(&resumeOfOperationID, "resume-of-operation-id", "", "operation id this record resumes")
	fs.StringVar(&poolID, "pool-id", "", "logical volume pool id")
	fs.StringVar(&sourceVolumeID, "source-volume-id", "", "source SBS volume id")
	fs.StringVar(&targetVolumeID, "target-volume-id", "", "target SBS volume id")
	fs.StringVar(&ownerID, "owner-id", "", "worker or operator owner id")
	fs.StringVar(&statusRaw, "status", "", "operation status: running, succeeded, retry_pending, failed, or canceled")
	fs.StringVar(&cursor, "cursor", "", "durable drain cursor")
	fs.IntVar(&scanned, "scanned", 0, "object/ref records scanned")
	fs.IntVar(&copied, "copied", 0, "refs copied to the target volume")
	fs.IntVar(&skipped, "skipped", 0, "refs skipped")
	fs.IntVar(&protected, "protected", 0, "refs skipped because they are protected")
	fs.IntVar(&retryable, "retryable", 0, "refs left retryable")
	fs.Var(&attemptSpecs, "attempt", "attempt spec; repeatable, e.g. bucket_id=b,key=k,version_id=v1,source_segment_id=s1,source_volume_id=18a1,target_segment_id=s2,target_volume_id=18a2,status=copied")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	status, err := parseVolumeDrainOperationStatus(statusRaw)
	if err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	action = strings.ToLower(strings.TrimSpace(action))
	output := volumeDrainOperationsOutput{
		SchemaVersion:  "namros.admin.volume_drain_operations.v1",
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Action:         action,
		Limit:          limit,
		SourceVolumeID: strings.TrimSpace(sourceVolumeID),
		TargetVolumeID: strings.TrimSpace(targetVolumeID),
		Status:         string(status),
	}
	switch action {
	case "list":
		records, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{
			SourceVolumeID: strings.TrimSpace(sourceVolumeID),
			TargetVolumeID: strings.TrimSpace(targetVolumeID),
			Status:         status,
			Limit:          limit,
		})
		if err != nil {
			return err
		}
		output.Operations = volumeDrainOperationOutputs(records)
	case "put":
		attempts, err := parseVolumeDrainAttemptSpecs(attemptSpecs)
		if err != nil {
			return err
		}
		scanned, copied, skipped, protected, retryable = fillVolumeDrainCounters(attempts, scanned, copied, skipped, protected, retryable)
		now := time.Now().UTC()
		startedAt := now
		finishedAt := now
		if status == model.VolumeDrainOperationRunning {
			finishedAt = time.Time{}
		}
		record, err := repo.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{
			ResumeOfOperationID: strings.TrimSpace(resumeOfOperationID),
			PoolID:              strings.TrimSpace(poolID),
			SourceVolumeID:      strings.TrimSpace(sourceVolumeID),
			TargetVolumeID:      strings.TrimSpace(targetVolumeID),
			OwnerID:             strings.TrimSpace(ownerID),
			Status:              status,
			Cursor:              strings.TrimSpace(cursor),
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			Scanned:             scanned,
			Copied:              copied,
			Skipped:             skipped,
			Protected:           protected,
			Retryable:           retryable,
			Attempts:            attempts,
		})
		if err != nil {
			return err
		}
		operation := volumeDrainOperationOutputFromRecord(record)
		output.Operation = &operation
	default:
		return fmt.Errorf("unsupported volume drain action %q", action)
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runBucketQuotaPut(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin bucket-quota-put", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var bucketName string
	var bucketID string
	var maxObjectSizeBytes int64
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&bucketName, "bucket", "", "bucket name")
	fs.StringVar(&bucketID, "bucket-id", "", "bucket id; optional alternative to bucket name")
	fs.Int64Var(&maxObjectSizeBytes, "max-object-size-bytes", 0, "maximum object size in bytes; 0 disables this quota")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if maxObjectSizeBytes < 0 {
		return fmt.Errorf("max-object-size-bytes cannot be negative")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	bucket, err := resolveAdminBucket(ctx, repo, bucketName, bucketID)
	if err != nil {
		return err
	}
	quota, err := repo.PutBucketQuota(ctx, meta.BucketQuotaRequest{
		BucketID:           bucket.BucketID,
		MaxObjectSizeBytes: maxObjectSizeBytes,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(bucketQuotaOutputFromModel(bucket, quota, false))
}

func (c adminCommand) runBucketQuotaGet(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin bucket-quota-get", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var bucketName string
	var bucketID string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&bucketName, "bucket", "", "bucket name")
	fs.StringVar(&bucketID, "bucket-id", "", "bucket id; optional alternative to bucket name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	bucket, err := resolveAdminBucket(ctx, repo, bucketName, bucketID)
	if err != nil {
		return err
	}
	quota, err := repo.GetBucketQuota(ctx, bucket.BucketID)
	if errors.Is(err, meta.ErrNotFound) {
		quota = model.BucketQuota{BucketID: bucket.BucketID}
	} else if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(bucketQuotaOutputFromModel(bucket, quota, false))
}

func (c adminCommand) runBucketQuotaDelete(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin bucket-quota-delete", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var bucketName string
	var bucketID string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&bucketName, "bucket", "", "bucket name")
	fs.StringVar(&bucketID, "bucket-id", "", "bucket id; optional alternative to bucket name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	bucket, err := resolveAdminBucket(ctx, repo, bucketName, bucketID)
	if err != nil {
		return err
	}
	if err := repo.DeleteBucketQuota(ctx, bucket.BucketID); err != nil && !errors.Is(err, meta.ErrNotFound) {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(bucketQuotaOutputFromModel(bucket, model.BucketQuota{BucketID: bucket.BucketID}, true))
}

func (c adminCommand) runTenantQuotaPut(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin tenant-quota-put", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var tenantID string
	var maxBytes int64
	var maxObjects int64
	var maxActiveUploads int64
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&tenantID, "tenant-id", "", "tenant id")
	fs.Int64Var(&maxBytes, "max-bytes", 0, "maximum tenant bytes; 0 disables this quota")
	fs.Int64Var(&maxObjects, "max-objects", 0, "maximum tenant object count; 0 disables this quota")
	fs.Int64Var(&maxActiveUploads, "max-active-uploads", 0, "maximum active multipart uploads; 0 disables this quota")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if tenantID == "" {
		return fmt.Errorf("tenant-id is required")
	}
	if maxBytes < 0 || maxObjects < 0 || maxActiveUploads < 0 {
		return fmt.Errorf("tenant quota values cannot be negative")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	if _, err := repo.GetTenant(ctx, tenantID); err != nil {
		return err
	}
	quota, err := repo.PutTenantQuota(ctx, meta.TenantQuotaRequest{
		TenantID:         tenantID,
		MaxBytes:         maxBytes,
		MaxObjects:       maxObjects,
		MaxActiveUploads: maxActiveUploads,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(tenantQuotaOutputFromModel(quota, false))
}

func (c adminCommand) runTenantQuotaGet(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin tenant-quota-get", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var tenantID string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&tenantID, "tenant-id", "", "tenant id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if tenantID == "" {
		return fmt.Errorf("tenant-id is required")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	if _, err := repo.GetTenant(ctx, tenantID); err != nil {
		return err
	}
	quota, err := repo.GetTenantQuota(ctx, tenantID)
	if errors.Is(err, meta.ErrNotFound) {
		quota = model.TenantQuota{TenantID: tenantID}
	} else if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(tenantQuotaOutputFromModel(quota, false))
}

func (c adminCommand) runTenantQuotaDelete(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin tenant-quota-delete", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var tenantID string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&tenantID, "tenant-id", "", "tenant id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if tenantID == "" {
		return fmt.Errorf("tenant-id is required")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	if _, err := repo.GetTenant(ctx, tenantID); err != nil {
		return err
	}
	if err := repo.DeleteTenantQuota(ctx, tenantID); err != nil && !errors.Is(err, meta.ErrNotFound) {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(tenantQuotaOutputFromModel(model.TenantQuota{TenantID: tenantID}, true))
}

var enterpriseAdminCommandFeatures = map[string]string{
	"dedupe-plan":                edition.FeatureDedupe,
	"dedupe-ack":                 edition.FeatureDedupe,
	"dedupe-ops":                 edition.FeatureDedupe,
	"dedupe-repair":              edition.FeatureDedupe,
	"dedupe-scrub":               edition.FeatureDedupe,
	"kms-key-put":                edition.FeatureSSEKMS,
	"kms-key-list":               edition.FeatureSSEKMS,
	"compliance-evidence":        edition.FeatureComplianceEvidence,
	"compliance-profile-plan":    edition.FeatureComplianceEvidence,
	"compliance-policy-simulate": edition.FeatureComplianceEvidence,
}

func (c adminCommand) runEnterpriseAdminCommand(_ string, featureID string) error {
	return requireEditionFeature(c.defaultConfig(), featureID)
}

func (c adminCommand) runStatus(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin status", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var countLimit int
	var recentDedupeLimit int
	var recentGCLimit int
	var sbsVolumePool string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")
	etcdEndpoints := strings.Join(cfg.EtcdEndpoints, ",")

	fs.StringVar(&cfg.DeploymentProfile, "deployment-profile", cfg.DeploymentProfile, "deployment profile: dev or production")
	fs.BoolVar(&cfg.AllowUnsafeProductionShortcuts, "allow-unsafe-production-shortcuts", cfg.AllowUnsafeProductionShortcuts, "report explicit lab override for production profile shortcuts")
	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&cfg.StorageBackend, "storage-backend", cfg.StorageBackend, "segment storage backend")
	fs.StringVar(&sbsVolumePool, "sbs-volume-pool", "", "semicolon-separated SBS volume pool members")
	fs.StringVar(&cfg.SBSVolumePoolID, "sbs-volume-pool-id", cfg.SBSVolumePoolID, "metadata volume pool id for readiness reporting")
	fs.StringVar(&cfg.SBSWriterGroupID, "sbs-writer-group-id", cfg.SBSWriterGroupID, "SBS writer group id for production readiness reporting")
	fs.StringVar(&cfg.SBSSessionID, "sbs-session-id", cfg.SBSSessionID, "SBS gateway session id for production readiness reporting")
	fs.Uint64Var(&cfg.SBSVolumeEpoch, "sbs-volume-epoch", cfg.SBSVolumeEpoch, "SBS volume epoch for production readiness reporting")
	fs.StringVar(&cfg.CoordinationBackend, "coordination-backend", cfg.CoordinationBackend, "coordination backend: none or etcd")
	fs.StringVar(&etcdEndpoints, "etcd-endpoints", etcdEndpoints, "comma-separated etcd endpoints for gateway coordination")
	fs.DurationVar(&cfg.EtcdDialTimeout, "etcd-dial-timeout", cfg.EtcdDialTimeout, "etcd dial timeout for gateway coordination")
	fs.StringVar(&cfg.GatewayRegistryPrefix, "gateway-registry-prefix", cfg.GatewayRegistryPrefix, "etcd key prefix for gateway registry")
	fs.DurationVar(&cfg.GatewayLeaseTTL, "gateway-lease-ttl", cfg.GatewayLeaseTTL, "gateway registry lease TTL")
	fs.DurationVar(&cfg.GatewayHeartbeat, "gateway-heartbeat", cfg.GatewayHeartbeat, "gateway registry heartbeat interval")
	fs.StringVar(&cfg.GCCandidateQueue, "gc-candidate-queue", cfg.GCCandidateQueue, "orphan GC candidate queue backend: storage or metadata")
	fs.IntVar(&countLimit, "count-limit", 1000, "maximum records per metadata collection to count")
	fs.IntVar(&recentDedupeLimit, "recent-dedupe-limit", 5, "maximum recent dedupe operation records to include")
	fs.IntVar(&recentGCLimit, "recent-gc-limit", 5, "maximum recent GC operation records to include")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	volumePool, err := config.ParseSBSVolumePoolSpec(sbsVolumePool)
	if err != nil {
		return err
	}
	cfg.SBSVolumePool = volumePool
	cfg.DeploymentProfile = config.NormalizeDeploymentProfile(cfg.DeploymentProfile)
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.StorageBackend = config.NormalizeStorageBackend(cfg.StorageBackend)
	cfg.CoordinationBackend = config.NormalizeCoordinationBackend(cfg.CoordinationBackend)
	cfg.GCCandidateQueue = config.NormalizeGCCandidateQueue(cfg.GCCandidateQueue)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	cfg.EtcdEndpoints = splitCommaList(etcdEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}

	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	output, err := adminstatus.Build(ctx, cfg, repo, adminstatus.Request{
		CountLimit:        countLimit,
		RecentDedupeLimit: recentDedupeLimit,
		RecentGCLimit:     recentGCLimit,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runWorkerOperations(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin worker-operations", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var limit int
	var workerKind string
	var shardID string
	var status string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&workerKind, "worker-kind", "", "optional worker kind filter, for example gc")
	fs.StringVar(&shardID, "shard-id", "", "optional worker shard id filter")
	fs.StringVar(&status, "status", "", "optional worker operation status filter")
	fs.IntVar(&limit, "limit", 20, "maximum worker operation records to include")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	operations, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{
		WorkerKind: strings.TrimSpace(workerKind),
		ShardID:    strings.TrimSpace(shardID),
		Status:     model.WorkerOperationStatus(strings.TrimSpace(status)),
		Limit:      limit,
	})
	if err != nil {
		return err
	}
	output := workerOperationsOutput{
		SchemaVersion: "namros.admin.worker_operations.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Limit:         limit,
		WorkerKind:    strings.TrimSpace(workerKind),
		ShardID:       strings.TrimSpace(shardID),
		Status:        strings.TrimSpace(status),
		Operations:    workerOperationOutputs(operations),
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runWorkerControl(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin worker-control", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var action string
	var workerKind string
	var shardID string
	var reason string
	var updatedBy string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&action, "action", "get", "worker control action: get, pause, cancel, or resume")
	fs.StringVar(&workerKind, "worker-kind", "", "worker kind to control, for example gc or lifecycle")
	fs.StringVar(&shardID, "shard-id", "", "worker shard id to control")
	fs.StringVar(&reason, "reason", "", "operator reason stored with the control record")
	fs.StringVar(&updatedBy, "updated-by", "namros-admin", "operator or automation identity stored with the control record")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	workerKind = strings.TrimSpace(workerKind)
	shardID = strings.TrimSpace(shardID)
	if workerKind == "" || shardID == "" {
		return fmt.Errorf("worker-kind and shard-id are required")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	action = strings.ToLower(strings.TrimSpace(action))
	var record model.WorkerControlRecord
	switch action {
	case "get":
		record, err = repo.GetWorkerControl(ctx, meta.GetWorkerControlRequest{
			WorkerKind: workerKind,
			ShardID:    shardID,
		})
		if errors.Is(err, meta.ErrNotFound) {
			record = model.WorkerControlRecord{
				WorkerKind: workerKind,
				ShardID:    shardID,
				State:      model.WorkerControlActive,
			}
			err = nil
		}
	case "pause":
		record, err = repo.PutWorkerControl(ctx, meta.PutWorkerControlRequest{
			WorkerKind: workerKind,
			ShardID:    shardID,
			State:      model.WorkerControlPaused,
			Reason:     reason,
			UpdatedBy:  updatedBy,
		})
	case "cancel":
		record, err = repo.PutWorkerControl(ctx, meta.PutWorkerControlRequest{
			WorkerKind: workerKind,
			ShardID:    shardID,
			State:      model.WorkerControlCanceled,
			Reason:     reason,
			UpdatedBy:  updatedBy,
		})
	case "resume":
		record, err = repo.PutWorkerControl(ctx, meta.PutWorkerControlRequest{
			WorkerKind: workerKind,
			ShardID:    shardID,
			State:      model.WorkerControlActive,
			Reason:     reason,
			UpdatedBy:  updatedBy,
		})
	default:
		return fmt.Errorf("unsupported worker control action %q", action)
	}
	if err != nil {
		return err
	}
	output := workerControlCommandOutput{
		SchemaVersion: "namros.admin.worker_control.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Action:        action,
		Control:       workerControlOutputFromRecord(record),
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runGCCandidates(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin gc-candidates", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var limit int
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.IntVar(&limit, "limit", 20, "maximum GC candidate records to include")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	records, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: limit})
	if err != nil {
		return err
	}
	output := gcCandidatesOutput{
		SchemaVersion: "namros.admin.gc_candidates.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Limit:         limit,
		Candidates:    gcCandidateOutputs(records),
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runGCCandidateSeedObject(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin gc-candidate-seed-object", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var bucketName string
	var key string
	var versionID string
	var reasonRaw string
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&bucketName, "bucket", "", "bucket name containing the object to detach and enqueue")
	fs.StringVar(&key, "key", "", "object key to detach and enqueue")
	fs.StringVar(&versionID, "version-id", "", "optional object version id; defaults to current head")
	fs.StringVar(&reasonRaw, "reason", string(storage.DeleteReasonManualGC), "GC candidate reason")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	bucketName = strings.TrimSpace(bucketName)
	key = strings.TrimSpace(key)
	versionID = strings.TrimSpace(versionID)
	if bucketName == "" {
		return fmt.Errorf("bucket is required")
	}
	if key == "" {
		return fmt.Errorf("key is required")
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}
	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	bucket, err := repo.GetBucketByName(ctx, bucketName)
	if err != nil {
		return err
	}
	if versionID == "" {
		head, err := repo.GetObjectHead(ctx, bucket.BucketID, key)
		if err != nil {
			return err
		}
		versionID = head.VersionID
	}
	version, err := repo.GetObjectVersion(ctx, bucket.BucketID, key, versionID)
	if err != nil {
		return err
	}
	refs := segmentRefsForObjectVersion(version)
	if len(refs) == 0 {
		return fmt.Errorf("object %s/%s version %s has no segment refs", bucketName, key, versionID)
	}
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       key,
		VersionID: version.VersionID,
		BypassAudit: meta.AuditContext{
			Principal: model.AuditPrincipal{DisplayName: "namros-admin"},
			Reason:    "gc-candidate-seed-object",
		},
	}); err != nil {
		return err
	}

	reason := storage.DeleteReason(strings.TrimSpace(reasonRaw))
	if reason == "" {
		reason = storage.DeleteReasonManualGC
	}
	records := make([]model.GCCandidateRecord, 0, len(refs))
	for _, ref := range refs {
		record, err := repo.PutGCCandidate(ctx, meta.PutGCCandidateRequest{
			SegmentRef: ref,
			Reason:     reason,
		})
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	output := gcCandidateSeedObjectOutput{
		SchemaVersion:  "namros.admin.gc_candidate_seed_object.v1",
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Bucket:         bucketName,
		Key:            key,
		VersionID:      version.VersionID,
		Reason:         string(reason),
		CandidateCount: len(records),
		Candidates:     gcCandidateOutputs(records),
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runMetadataExport(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin metadata-export", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var limit int
	var auditAdminOperation bool
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.IntVar(&limit, "limit", 1000, "maximum records to export per collection")
	fs.BoolVar(&auditAdminOperation, "audit-admin-operation", true, "record this admin command in the metadata audit chain")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	if err := validateMetadataConfig(cfg); err != nil {
		return err
	}

	repo, closeRepo, err := openMetadata(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeRepo()

	output, err := buildMetadataExport(ctx, repo, limit)
	if err != nil {
		return err
	}
	if auditAdminOperation {
		if err := putAdminAuditEvent(ctx, repo, model.AuditActionAdminMetadataExport, "namros-admin metadata-export", metadataExportAuditDetails(output)); err != nil {
			return err
		}
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runMetadataImport(ctx context.Context, args []string) error {
	cfg := c.defaultConfig()
	fs := flag.NewFlagSet("namros-admin metadata-import", flag.ContinueOnError)
	fs.SetOutput(c.stderr)

	var inputPath string
	var dryRun bool
	var apply bool
	var allowExperimentalApply bool
	var metadataBackend string
	var requireEmptyTarget bool
	var targetScanLimit int
	var conflictPolicy string
	var auditAdminOperation bool
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")

	fs.StringVar(&inputPath, "input", "", "metadata export JSON file to validate")
	fs.BoolVar(&dryRun, "dry-run", true, "validate input without writing metadata")
	fs.BoolVar(&apply, "apply", false, "apply operational metadata into the target backend")
	fs.BoolVar(&allowExperimentalApply, "allow-experimental-apply", false, "acknowledge the experimental metadata import apply gate")
	fs.StringVar(&metadataBackend, "metadata-backend", "", "optional target metadata backend to validate before future apply: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "target metadata path for pebble backend")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints for target metadata")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for target metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.BoolVar(&requireEmptyTarget, "require-empty-target", true, "mark apply plan not ready when target metadata is not empty")
	fs.IntVar(&targetScanLimit, "target-scan-limit", 1000, "maximum target records per collection to count during dry-run")
	fs.StringVar(&conflictPolicy, "conflict-policy", "fail_if_exists", "future apply conflict policy; currently only fail_if_exists is supported")
	fs.BoolVar(&auditAdminOperation, "audit-admin-operation", false, "record this admin command in the target metadata audit chain; requires -metadata-backend")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if strings.TrimSpace(inputPath) == "" {
		return fmt.Errorf("input is required")
	}
	applyRequested := apply || !dryRun
	if applyRequested && strings.TrimSpace(metadataBackend) == "" {
		return fmt.Errorf("metadata import apply planning requires target -metadata-backend")
	}
	if conflictPolicy != "fail_if_exists" {
		return fmt.Errorf("unsupported metadata import conflict policy %q", conflictPolicy)
	}
	if auditAdminOperation && strings.TrimSpace(metadataBackend) == "" {
		return fmt.Errorf("metadata import audit requires target -metadata-backend")
	}

	var target meta.Repository
	targetChecked := false
	closeTarget := func() error { return nil }
	if strings.TrimSpace(metadataBackend) != "" {
		cfg.MetadataBackend = config.NormalizeMetadataBackend(metadataBackend)
		cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
		if err := validateMetadataConfig(cfg); err != nil {
			return err
		}
		var err error
		target, closeTarget, err = openMetadata(ctx, cfg)
		if err != nil {
			return err
		}
		defer closeTarget()
		targetChecked = true
	}

	output, err := loadMetadataImportDryRun(ctx, metadataImportDryRunRequest{
		InputPath:          inputPath,
		Target:             target,
		TargetChecked:      targetChecked,
		TargetScanLimit:    targetScanLimit,
		RequireEmptyTarget: requireEmptyTarget,
		ConflictPolicy:     conflictPolicy,
	})
	if err != nil {
		return err
	}
	output.DryRun = !applyRequested
	output.ApplyRequested = applyRequested
	if applyRequested {
		importReq, err := loadMetadataImportRequest(inputPath, requireEmptyTarget)
		if err != nil {
			return err
		}
		result := metadataImportApply(ctx, target, output, importReq, allowExperimentalApply)
		output.ApplyResult = &result
	}
	if auditAdminOperation {
		if err := putAdminAuditEvent(ctx, target, model.AuditActionAdminMetadataImport, "namros-admin metadata-import", metadataImportAuditDetails(output)); err != nil {
			return err
		}
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return err
	}
	if output.ApplyResult != nil && output.ApplyResult.Status != "succeeded" {
		return fmt.Errorf("metadata import apply failed: %s", output.ApplyResult.Message)
	}
	return nil
}

type principalFlagValues struct {
	TenantID       string
	AccessKeyID    string
	DisplayName    string
	Subject        string
	SessionID      string
	ExternalIssuer string
	PolicyVersion  string
	SourceIdentity string
	Root           bool
	Groups         stringListFlag
	Roles          stringListFlag
	Permissions    stringListFlag
}

func (c adminCommand) runIAMPrincipalInspect(args []string) error {
	fs := flag.NewFlagSet("namros-admin iam-principal-inspect", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	var principalFlags principalFlagValues
	addPrincipalFlags(fs, &principalFlags)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	output := iam.InspectPrincipal(principalFromFlags(principalFlags))
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func (c adminCommand) runIAMPolicySimulate(args []string) error {
	fs := flag.NewFlagSet("namros-admin iam-policy-simulate", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	var principalFlags principalFlagValues
	var action string
	var resource string
	var policyJSON string
	var policyFile string
	addPrincipalFlags(fs, &principalFlags)
	fs.StringVar(&action, "action", "", "S3/IAM action to simulate, for example s3:GetObject")
	fs.StringVar(&resource, "resource", "", "resource ARN to simulate")
	fs.StringVar(&policyJSON, "policy-json", "", "inline bucket/session policy JSON to evaluate")
	fs.StringVar(&policyFile, "policy-file", "", "policy JSON file to evaluate")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	policy, err := parsePolicyInput(policyJSON, policyFile)
	if err != nil {
		return err
	}
	result, err := iam.SimulatePolicy(iam.PolicySimulationRequest{
		Principal: principalFromFlags(principalFlags),
		Action:    action,
		Resource:  resource,
		Policy:    policy,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

func (c adminCommand) runIAMMappingValidate(args []string) error {
	fs := flag.NewFlagSet("namros-admin iam-mapping-validate", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	var inputPath string
	fs.StringVar(&inputPath, "input", "", "external IAM mapping spec JSON file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if strings.TrimSpace(inputPath) == "" {
		return fmt.Errorf("input is required")
	}
	file, err := os.Open(inputPath)
	if err != nil {
		return err
	}
	defer file.Close()
	spec, err := iam.ParseMappingSpec(file)
	if err != nil {
		return err
	}
	output := map[string]any{
		"schema_version": "namros.admin.iam_mapping_validate.v1",
		"dry_run":        true,
		"edition":        adminstatus.EditionFromConfig(c.defaultConfig()),
		"report":         iam.ValidateMappingSpec(spec),
	}
	encoder := json.NewEncoder(c.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func addPrincipalFlags(fs *flag.FlagSet, values *principalFlagValues) {
	fs.StringVar(&values.TenantID, "tenant-id", "root", "principal tenant id")
	fs.StringVar(&values.AccessKeyID, "access-key-id", "", "principal access key id")
	fs.StringVar(&values.DisplayName, "display-name", "", "principal display name")
	fs.StringVar(&values.Subject, "subject", "", "external IAM subject")
	fs.Var(&values.Groups, "group", "external IAM group; may be repeated or comma-separated")
	fs.Var(&values.Roles, "role", "external IAM role; may be repeated or comma-separated")
	fs.StringVar(&values.SessionID, "session-id", "", "temporary session id")
	fs.StringVar(&values.ExternalIssuer, "external-issuer", "", "external IAM issuer")
	fs.StringVar(&values.PolicyVersion, "policy-version", "", "policy version label")
	fs.StringVar(&values.SourceIdentity, "source-identity", "", "source identity carried into audit evidence")
	fs.BoolVar(&values.Root, "root", false, "mark principal as root")
	fs.Var(&values.Permissions, "permission", "principal permission; may be repeated or comma-separated")
}

func principalFromFlags(values principalFlagValues) auth.Principal {
	return auth.Principal{
		TenantID:       strings.TrimSpace(values.TenantID),
		AccessKeyID:    strings.TrimSpace(values.AccessKeyID),
		DisplayName:    strings.TrimSpace(values.DisplayName),
		Subject:        strings.TrimSpace(values.Subject),
		Groups:         append([]string(nil), values.Groups...),
		Roles:          append([]string(nil), values.Roles...),
		SessionID:      strings.TrimSpace(values.SessionID),
		ExternalIssuer: strings.TrimSpace(values.ExternalIssuer),
		PolicyVersion:  strings.TrimSpace(values.PolicyVersion),
		SourceIdentity: strings.TrimSpace(values.SourceIdentity),
		Root:           values.Root,
		Permissions:    append([]string(nil), values.Permissions...),
	}
}

func parsePolicyInput(policyJSON, policyFile string) (*auth.PolicyDocument, error) {
	if strings.TrimSpace(policyJSON) != "" && strings.TrimSpace(policyFile) != "" {
		return nil, fmt.Errorf("use only one of -policy-json or -policy-file")
	}
	if strings.TrimSpace(policyJSON) == "" && strings.TrimSpace(policyFile) == "" {
		return nil, nil
	}
	var reader io.Reader
	if strings.TrimSpace(policyJSON) != "" {
		reader = strings.NewReader(policyJSON)
	} else {
		file, err := os.Open(policyFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}
	policy, err := auth.ParsePolicyDocument(reader)
	if err != nil {
		return nil, err
	}
	return &policy, nil
}

func validateMetadataConfig(cfg config.Config) error {
	if err := edition.Validate(cfg.Edition); err != nil {
		return err
	}
	switch cfg.MetadataBackend {
	case config.MetadataBackendMemory:
		return nil
	case config.MetadataBackendPebble:
		if strings.TrimSpace(cfg.MetadataPath) == "" {
			return fmt.Errorf("metadata path is required for pebble backend")
		}
	case config.MetadataBackendTiKV:
		if err := edition.Require(cfg.Edition, edition.FeatureTiKVMetadataCluster); err != nil {
			return err
		}
		if len(cfg.TiKVPDEndpoints) == 0 {
			return fmt.Errorf("tikv pd endpoints are required")
		}
		if cfg.TiKVTimeout < 0 || cfg.TiKVRetryAttempts < 0 || cfg.TiKVRetryInitialBackoff < 0 || cfg.TiKVRetryMaxBackoff < 0 {
			return fmt.Errorf("tikv timeout and retry values cannot be negative")
		}
		if cfg.TiKVRetryMaxBackoff > 0 && cfg.TiKVRetryInitialBackoff > cfg.TiKVRetryMaxBackoff {
			return fmt.Errorf("tikv retry initial backoff cannot exceed max backoff")
		}
	default:
		return fmt.Errorf("unsupported metadata backend %q", cfg.MetadataBackend)
	}
	return nil
}

func requireEditionFeature(cfg config.Config, featureID string) error {
	return edition.Require(cfg.Edition, featureID)
}

func openMetadata(ctx context.Context, cfg config.Config) (meta.Repository, func() error, error) {
	switch cfg.MetadataBackend {
	case config.MetadataBackendMemory:
		return memory.New(), func() error { return nil }, nil
	case config.MetadataBackendPebble:
		repo, err := pebblemeta.Open(cfg.MetadataPath)
		if err != nil {
			return nil, nil, err
		}
		return repo, repo.Close, nil
	case config.MetadataBackendTiKV:
		repo, err := tikvmeta.Open(ctx, tikvmeta.Config{
			PDEndpoints: cfg.TiKVPDEndpoints,
			APIVersion:  cfg.TiKVAPIVersion,
			Keyspace:    cfg.TiKVKeyspace,
			Timeout:     cfg.TiKVTimeout,
			TLS: tikvmeta.TLSConfig{
				CAPath:   cfg.TiKVTLSCA,
				CertPath: cfg.TiKVTLSCert,
				KeyPath:  cfg.TiKVTLSKey,
			},
			Retry: tikvmeta.RetryPolicy{
				MaxAttempts:    cfg.TiKVRetryAttempts,
				InitialBackoff: cfg.TiKVRetryInitialBackoff,
				MaxBackoff:     cfg.TiKVRetryMaxBackoff,
			},
		})
		if err != nil {
			return nil, nil, err
		}
		return repo, repo.Close, nil
	default:
		return nil, nil, fmt.Errorf("unsupported metadata backend %q", cfg.MetadataBackend)
	}
}

type restoreValidationSBSLocalOpener func(context.Context, sbsegments.Config) (storage.SegmentStore, error)
type restoreValidationSBSPhysicalOpener func(context.Context, sbsegments.PhysicalOpenConfig) (storage.SegmentStore, func() error, error)
type restoreValidationSBSECOpener func(context.Context, sbsegments.ECOpenConfig) (storage.SegmentStore, func() error, error)
type restoreValidationSBSClusterOpener func(context.Context, sbsegments.ClusterOpenConfig) (storage.SegmentStore, func() error, error)

var openRestoreSBSLocalStorage restoreValidationSBSLocalOpener = func(ctx context.Context, cfg sbsegments.Config) (storage.SegmentStore, error) {
	return sbsegments.Open(ctx, cfg)
}

var openRestoreSBSPhysicalStorage restoreValidationSBSPhysicalOpener = func(ctx context.Context, cfg sbsegments.PhysicalOpenConfig) (storage.SegmentStore, func() error, error) {
	return sbsegments.OpenPhysical(ctx, cfg)
}

var openRestoreSBSECStorage restoreValidationSBSECOpener = func(ctx context.Context, cfg sbsegments.ECOpenConfig) (storage.SegmentStore, func() error, error) {
	return sbsegments.OpenEC(ctx, cfg)
}

var openRestoreSBSClusterStorage restoreValidationSBSClusterOpener = func(ctx context.Context, cfg sbsegments.ClusterOpenConfig) (storage.SegmentStore, func() error, error) {
	return sbsegments.OpenCluster(ctx, cfg)
}

func openRestoreValidationStorage(ctx context.Context, cfg config.Config) (storage.SegmentStore, func() error, error) {
	backend := config.NormalizeStorageBackend(cfg.StorageBackend)
	switch backend {
	case config.StorageBackendLocal:
		if strings.TrimSpace(cfg.StoragePath) == "" {
			return nil, nil, fmt.Errorf("storage path is required for local backend")
		}
		store, err := local.New(cfg.StoragePath)
		if err != nil {
			return nil, nil, err
		}
		return store, func() error { return nil }, nil
	case config.StorageBackendSBS, config.StorageBackendSBSLocal:
		if strings.TrimSpace(cfg.StoragePath) == "" {
			return nil, nil, fmt.Errorf("storage path is required for %s backend", backend)
		}
		volumeIDRaw, err := parseRestoreValidationSBSVolumeIDRaw(cfg.SBSVolumeID)
		if err != nil {
			return nil, nil, err
		}
		store, err := openRestoreSBSLocalStorage(ctx, sbsegments.Config{
			Path:            cfg.StoragePath,
			StatePath:       cfg.SBSStatePath,
			VolumeID:        volumeIDRaw,
			GatewayID:       cfg.SBSGatewayID,
			AttachmentID:    cfg.SBSAttachmentID,
			Generation:      cfg.SBSGeneration,
			SessionIdentity: restoreValidationSBSSessionIdentity(cfg),
		})
		if err != nil {
			return nil, nil, err
		}
		cleanup := func() error { return nil }
		if closer, ok := store.(interface{ Close() error }); ok {
			cleanup = closer.Close
		}
		return store, cleanup, nil
	case config.StorageBackendSBSPhysical, config.StorageBackendSBSEC, config.StorageBackendSBSCluster:
		if len(cfg.SBSVolumePool) > 0 {
			return openRestoreValidationSBSVolumePool(ctx, cfg, backend)
		}
		return openRestoreValidationSingleSBSVolume(ctx, cfg, backend)
	default:
		return nil, nil, fmt.Errorf("metadata restore validation does not support storage backend %q", cfg.StorageBackend)
	}
}

func openRestoreValidationSingleSBSVolume(ctx context.Context, cfg config.Config, backend string) (storage.SegmentStore, func() error, error) {
	sessionCache := sbsegments.NewVolumeSessionCache()
	switch backend {
	case config.StorageBackendSBSPhysical:
		return openRestoreSBSPhysicalStorage(ctx, restoreValidationSBSPhysicalConfig(cfg, cfg.SBSAdminEndpoint, cfg.SBSDataEndpoint, cfg.SBSVolumeID, restoreValidationSBSSessionIdentity(cfg), sessionCache))
	case config.StorageBackendSBSEC:
		if err := edition.Require(cfg.Edition, edition.FeatureErasureCoding); err != nil {
			return nil, nil, err
		}
		return openRestoreSBSECStorage(ctx, sbsegments.ECOpenConfig{
			DataEndpoint:     cfg.SBSDataEndpoint,
			VolumeID:         cfg.SBSVolumeID,
			GatewayID:        cfg.SBSGatewayID,
			AttachmentID:     cfg.SBSAttachmentID,
			Generation:       cfg.SBSGeneration,
			SessionIdentity:  restoreValidationSBSSessionIdentity(cfg),
			SessionCache:     sessionCache,
			ShardStoreIDs:    cfg.SBSShardStoreIDs,
			ShardConcurrency: cfg.SBSECShardConcurrency,
		})
	case config.StorageBackendSBSCluster:
		sessionIdentity := restoreValidationSBSSessionIdentity(cfg)
		clusterCfg := restoreValidationSBSClusterConfig(cfg, cfg.SBSAdminEndpoint, cfg.SBSDataEndpoint, cfg.SBSVolumeID, sessionIdentity, sessionCache, cfg.SBSShardStoreIDs)
		if edition.Allows(cfg.Edition, edition.FeatureErasureCoding) {
			return openRestoreSBSClusterStorage(ctx, clusterCfg)
		}
		return openRestoreSBSPhysicalStorage(ctx, restoreValidationSBSPhysicalConfig(cfg, clusterCfg.AdminEndpoint, clusterCfg.DataEndpoint, clusterCfg.VolumeID, sessionIdentity, sessionCache))
	default:
		return nil, nil, fmt.Errorf("metadata restore validation does not support SBS storage backend %q", backend)
	}
}

func openRestoreValidationSBSVolumePool(ctx context.Context, cfg config.Config, backend string) (storage.SegmentStore, func() error, error) {
	sessionCache := sbsegments.NewVolumeSessionCache()
	members := make([]volumepool.Member, 0, len(cfg.SBSVolumePool))
	cleanups := make([]func() error, 0, len(cfg.SBSVolumePool))
	for _, rawMember := range cfg.SBSVolumePool {
		member := restoreValidationInheritSBSVolumePoolMember(cfg, rawMember, len(cfg.SBSVolumePool))
		segmentStore, cleanupFn, err := openRestoreValidationSBSVolumePoolMember(ctx, cfg, backend, member, sessionCache)
		if err != nil {
			_ = cleanupRestoreValidationStores(cleanups)
			return nil, nil, err
		}
		cleanups = append(cleanups, cleanupFn)
		members = append(members, volumepool.Member{
			VolumeID:             member.VolumeID,
			Store:                segmentStore,
			ReadOnly:             member.ReadOnly,
			State:                member.State,
			Weight:               member.Weight,
			AvailableBytes:       member.AvailableBytes,
			UsedPercent:          member.UsedPercent,
			HighWatermarkPercent: member.HighWatermarkPercent,
		})
	}
	poolStore, err := volumepool.New(members)
	if err != nil {
		_ = cleanupRestoreValidationStores(cleanups)
		return nil, nil, err
	}
	return poolStore, func() error { return cleanupRestoreValidationStores(cleanups) }, nil
}

func openRestoreValidationSBSVolumePoolMember(ctx context.Context, cfg config.Config, backend string, member config.SBSVolumePoolMember, sessionCache *sbsegments.VolumeSessionCache) (storage.SegmentStore, func() error, error) {
	sessionIdentity := restoreValidationSBSSessionIdentityForMember(cfg, member)
	switch backend {
	case config.StorageBackendSBSPhysical:
		physicalCfg := restoreValidationSBSPhysicalConfig(cfg, member.AdminEndpoint, member.DataEndpoint, member.VolumeID, sessionIdentity, sessionCache)
		physicalCfg.GatewayID = member.GatewayID
		physicalCfg.AttachmentID = member.AttachmentID
		physicalCfg.Generation = member.Generation
		physicalCfg.ChunkSizeBytes = member.ChunkSizeBytes
		physicalCfg.VerifyReadback = member.VerifyReadback
		physicalCfg.WriteConcurrency = member.WriteConcurrency
		return openRestoreSBSPhysicalStorage(ctx, physicalCfg)
	case config.StorageBackendSBSEC:
		if err := edition.Require(cfg.Edition, edition.FeatureErasureCoding); err != nil {
			return nil, nil, err
		}
		return openRestoreSBSECStorage(ctx, sbsegments.ECOpenConfig{
			DataEndpoint:     member.DataEndpoint,
			VolumeID:         member.VolumeID,
			GatewayID:        member.GatewayID,
			AttachmentID:     member.AttachmentID,
			Generation:       member.Generation,
			SessionIdentity:  sessionIdentity,
			SessionCache:     sessionCache,
			ShardStoreIDs:    member.ShardStoreIDs,
			ShardConcurrency: cfg.SBSECShardConcurrency,
		})
	case config.StorageBackendSBSCluster:
		clusterCfg := restoreValidationSBSClusterConfig(cfg, member.AdminEndpoint, member.DataEndpoint, member.VolumeID, sessionIdentity, sessionCache, member.ShardStoreIDs)
		clusterCfg.GatewayID = member.GatewayID
		clusterCfg.AttachmentID = member.AttachmentID
		clusterCfg.Generation = member.Generation
		clusterCfg.ChunkSizeBytes = member.ChunkSizeBytes
		clusterCfg.VerifyReadback = member.VerifyReadback
		clusterCfg.WriteConcurrency = member.WriteConcurrency
		if edition.Allows(cfg.Edition, edition.FeatureErasureCoding) {
			return openRestoreSBSClusterStorage(ctx, clusterCfg)
		}
		physicalCfg := restoreValidationSBSPhysicalConfig(cfg, clusterCfg.AdminEndpoint, clusterCfg.DataEndpoint, clusterCfg.VolumeID, sessionIdentity, sessionCache)
		physicalCfg.GatewayID = clusterCfg.GatewayID
		physicalCfg.AttachmentID = clusterCfg.AttachmentID
		physicalCfg.Generation = clusterCfg.Generation
		physicalCfg.ChunkSizeBytes = clusterCfg.ChunkSizeBytes
		physicalCfg.VerifyReadback = clusterCfg.VerifyReadback
		physicalCfg.WriteConcurrency = clusterCfg.WriteConcurrency
		return openRestoreSBSPhysicalStorage(ctx, physicalCfg)
	default:
		return nil, nil, fmt.Errorf("metadata restore validation does not support SBS volume pool backend %q", backend)
	}
}

func restoreValidationSBSPhysicalConfig(cfg config.Config, adminEndpoint, dataEndpoint, volumeID string, sessionIdentity sbsegments.SessionIdentity, sessionCache *sbsegments.VolumeSessionCache) sbsegments.PhysicalOpenConfig {
	return sbsegments.PhysicalOpenConfig{
		AdminEndpoint:              adminEndpoint,
		DataEndpoint:               dataEndpoint,
		VolumeID:                   volumeID,
		ChunkSizeBytes:             cfg.SBSChunkSizeBytes,
		GatewayID:                  cfg.SBSGatewayID,
		AttachmentID:               cfg.SBSAttachmentID,
		Generation:                 cfg.SBSGeneration,
		SessionIdentity:            sessionIdentity,
		SessionCache:               sessionCache,
		VerifyReadback:             cfg.SBSVerifyReadback,
		WriteConcurrency:           cfg.SBSPhysicalWriteConcurrency,
		FullChunkWriteMinBytes:     cfg.SBSPhysicalFullChunkWriteMinBytes,
		FullChunkWriteMaxBytes:     cfg.SBSPhysicalFullChunkWriteMaxBytes,
		ChunkCacheBytes:            cfg.SBSPhysicalChunkCacheBytes,
		ChunkIDAllocationCacheSize: cfg.SBSChunkIDAllocationCacheSize,
	}
}

func restoreValidationSBSClusterConfig(cfg config.Config, adminEndpoint, dataEndpoint, volumeID string, sessionIdentity sbsegments.SessionIdentity, sessionCache *sbsegments.VolumeSessionCache, shardStoreIDs []string) sbsegments.ClusterOpenConfig {
	return sbsegments.ClusterOpenConfig{
		AdminEndpoint:              adminEndpoint,
		DataEndpoint:               dataEndpoint,
		VolumeID:                   volumeID,
		ChunkSizeBytes:             cfg.SBSChunkSizeBytes,
		GatewayID:                  cfg.SBSGatewayID,
		AttachmentID:               cfg.SBSAttachmentID,
		Generation:                 cfg.SBSGeneration,
		SessionIdentity:            sessionIdentity,
		SessionCache:               sessionCache,
		VerifyReadback:             cfg.SBSVerifyReadback,
		WriteConcurrency:           cfg.SBSPhysicalWriteConcurrency,
		FullChunkWriteMinBytes:     cfg.SBSPhysicalFullChunkWriteMinBytes,
		FullChunkWriteMaxBytes:     cfg.SBSPhysicalFullChunkWriteMaxBytes,
		ChunkCacheBytes:            cfg.SBSPhysicalChunkCacheBytes,
		ChunkIDAllocationCacheSize: cfg.SBSChunkIDAllocationCacheSize,
		ShardStoreIDs:              append([]string(nil), shardStoreIDs...),
		ECShardConcurrency:         cfg.SBSECShardConcurrency,
	}
}

func restoreValidationInheritSBSVolumePoolMember(cfg config.Config, member config.SBSVolumePoolMember, poolSize int) config.SBSVolumePoolMember {
	if member.AdminEndpoint == "" {
		member.AdminEndpoint = cfg.SBSAdminEndpoint
	}
	if member.DataEndpoint == "" {
		member.DataEndpoint = cfg.SBSDataEndpoint
	}
	if member.GatewayID == "" {
		member.GatewayID = cfg.SBSGatewayID
	}
	if member.Generation == 0 {
		member.Generation = cfg.SBSGeneration
	}
	if member.WriterGroupID == "" {
		member.WriterGroupID = cfg.SBSWriterGroupID
	}
	if member.VolumeEpoch == 0 {
		member.VolumeEpoch = cfg.SBSVolumeEpoch
	}
	if member.ChunkSizeBytes == 0 {
		member.ChunkSizeBytes = cfg.SBSChunkSizeBytes
	}
	if len(member.ShardStoreIDs) == 0 {
		member.ShardStoreIDs = append([]string(nil), cfg.SBSShardStoreIDs...)
	}
	if !member.VerifyReadback {
		member.VerifyReadback = cfg.SBSVerifyReadback
	}
	if member.WriteConcurrency == 0 {
		member.WriteConcurrency = cfg.SBSPhysicalWriteConcurrency
	}
	if member.AttachmentID == "" {
		member.AttachmentID = cfg.SBSAttachmentID
		if member.AttachmentID != "" && poolSize > 1 && !strings.Contains(member.AttachmentID, "{volume_id}") {
			member.AttachmentID += "-" + member.VolumeID
		}
	}
	member.AttachmentID = strings.ReplaceAll(member.AttachmentID, "{volume_id}", member.VolumeID)
	return member
}

func restoreValidationSBSSessionIdentity(cfg config.Config) sbsegments.SessionIdentity {
	return sbsegments.SessionIdentity{
		PoolID:            cfg.SBSVolumePoolID,
		PoolGeneration:    cfg.SBSVolumePoolGeneration,
		MemberGeneration:  cfg.SBSGeneration,
		VolumeEpoch:       cfg.SBSVolumeEpoch,
		WriterGroupID:     cfg.SBSWriterGroupID,
		GatewayID:         cfg.SBSGatewayID,
		GatewayInstanceID: cfg.GatewayInstanceID,
		SessionID:         cfg.SBSSessionID,
		SessionTTL:        cfg.SBSSessionTTL,
		HeartbeatInterval: cfg.SBSSessionHeartbeat,
	}
}

func restoreValidationSBSSessionIdentityForMember(cfg config.Config, member config.SBSVolumePoolMember) sbsegments.SessionIdentity {
	return sbsegments.SessionIdentity{
		PoolID:            cfg.SBSVolumePoolID,
		PoolGeneration:    cfg.SBSVolumePoolGeneration,
		VolumeID:          member.VolumeID,
		MemberGeneration:  member.Generation,
		VolumeEpoch:       member.VolumeEpoch,
		WriterGroupID:     member.WriterGroupID,
		GatewayID:         member.GatewayID,
		GatewayInstanceID: cfg.GatewayInstanceID,
		SessionID:         cfg.SBSSessionID,
		SessionTTL:        cfg.SBSSessionTTL,
		HeartbeatInterval: cfg.SBSSessionHeartbeat,
	}
}

func parseRestoreValidationSBSVolumeIDRaw(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseUint(value, 0, 64)
	if err == nil {
		return parsed, nil
	}
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		return 0, fmt.Errorf("invalid sbs volume id %q: %w", value, err)
	}
	parsed, hexErr := strconv.ParseUint(value, 16, 64)
	if hexErr == nil {
		return parsed, nil
	}
	return 0, fmt.Errorf("invalid sbs volume id %q: %w", value, err)
}

func cleanupRestoreValidationStores(cleanups []func() error) error {
	var out error
	for i := len(cleanups) - 1; i >= 0; i-- {
		if cleanups[i] == nil {
			continue
		}
		out = errors.Join(out, cleanups[i]())
	}
	return out
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func parseOptionalRFC3339(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func validateRestoreSample(ctx context.Context, repo meta.Repository, store storage.SegmentStore, head model.ObjectHead) metadataRestoreValidateSampleOutput {
	sample := metadataRestoreValidateSampleOutput{
		Key:       head.Key,
		VersionID: head.VersionID,
		SizeBytes: head.SizeBytes,
		Status:    "verified",
	}
	current, err := repo.GetObjectHead(ctx, head.BucketID, head.Key)
	if err != nil {
		sample.Status = "failed"
		sample.Error = err.Error()
		return sample
	}
	sample.ListIndexMatch = current.VersionID == head.VersionID && current.Revision == head.Revision
	if !sample.ListIndexMatch {
		sample.Status = "failed"
		sample.Error = "list index entry does not match object head"
		return sample
	}
	sample.SizeBytes = current.SizeBytes
	sample.ServerSideEncryption = string(current.ServerSideEncryption.Algorithm)
	sample.KMSKeyID = current.ServerSideEncryption.KeyID
	sample.KMSKeyVersion = current.ServerSideEncryption.KeyVersion
	refs := objectHeadSegmentRefs(current)
	sample.SegmentCount = len(refs)
	if len(refs) == 0 {
		sample.Status = "failed"
		sample.Error = "object head has no segment refs"
		return sample
	}
	for _, ref := range refs {
		if ref.Encryption.Algorithm != "" {
			sample.EncryptedSegmentCount++
		}
		if volumeID := segmentRefVolumeID(ref); volumeID != "" && !stringSliceHas(sample.VolumeIDs, volumeID) {
			sample.VolumeIDs = append(sample.VolumeIDs, volumeID)
		}
		if err := validateSegmentDigest(ctx, store, ref); err != nil {
			sample.Status = "failed"
			sample.Error = err.Error()
			return sample
		}
	}
	sample.DigestMatch = true
	return sample
}

func stringSliceHas(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func objectHeadSegmentRefs(head model.ObjectHead) []storage.SegmentRef {
	if len(head.SegmentRefs) > 0 {
		return append([]storage.SegmentRef(nil), head.SegmentRefs...)
	}
	if head.SegmentRef.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{head.SegmentRef}
}

func validateSegmentDigest(ctx context.Context, store storage.SegmentStore, ref storage.SegmentRef) error {
	if strings.TrimSpace(ref.SegmentID) == "" {
		return fmt.Errorf("segment id is required")
	}
	if !strings.EqualFold(ref.Digest.Algorithm, "sha256") || strings.TrimSpace(ref.Digest.Hex) == "" {
		return fmt.Errorf("segment %q has unsupported or missing digest", ref.SegmentID)
	}
	reader, err := store.GetSegment(ctx, ref, 0, ref.SizeBytes)
	if err != nil {
		return fmt.Errorf("segment %q read failed: %w", ref.SegmentID, err)
	}
	defer reader.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, reader); err != nil {
		return fmt.Errorf("segment %q read failed: %w", ref.SegmentID, err)
	}
	got := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(got, ref.Digest.Hex) {
		return fmt.Errorf("segment %q digest mismatch", ref.SegmentID)
	}
	return nil
}

func buildMetadataExport(ctx context.Context, repo meta.Repository, limit int) (metadataExportOutput, error) {
	if limit <= 0 {
		limit = 1000
	}
	schema, err := repo.GetMetadataSchema(ctx)
	if err != nil && !errors.Is(err, meta.ErrNotFound) {
		return metadataExportOutput{}, err
	}
	var schemaOutput *metadataSchemaExportOutput
	if err == nil {
		schemaOutput = metadataSchemaExportOutputFromModel(schema)
	} else if errors.Is(err, meta.ErrNotFound) {
		schemaOutput = metadataSchemaExportOutputFromModel(meta.DefaultMetadataSchemaRecord(time.Now().UTC()))
	}
	migrationOperations, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	auditEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	kmsKeys, err := repo.ListKMSKeys(ctx, meta.ListKMSKeysRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	gcOperations, err := repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	dedupeOperations, err := repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	sharedObjects, err := repo.ListSharedObjects(ctx, meta.ListSharedObjectsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	sharedReleases, err := repo.ListSharedObjectReleases(ctx, meta.ListSharedObjectReleasesRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	volumePools, err := repo.ListVolumePools(ctx, meta.ListVolumePoolsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	volumeDrainOperations, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	workerLeases, err := repo.ListWorkerLeases(ctx, meta.ListWorkerLeasesRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	workerOperations, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{Limit: limit})
	if err != nil {
		return metadataExportOutput{}, err
	}
	return metadataExportOutput{
		SchemaVersion:               1,
		GeneratedAt:                 formatJSONTime(time.Now().UTC()),
		Limit:                       limit,
		MetadataSchema:              schemaOutput,
		MetadataMigrationOperations: metadataMigrationOperationOutputs(migrationOperations),
		KMSKeys:                     kmsKeyExportOutputs(kmsKeys),
		AuditEvents:                 auditEventExportOutputs(auditEvents),
		GCOperations:                gcOperationExportOutputs(gcOperations),
		DedupeOperations:            dedupeOperationOutputs(dedupeOperations),
		SharedObjects:               sharedObjectExportOutputs(sharedObjects),
		SharedObjectReleases:        sharedObjectReleaseOperationOutputs(sharedReleases),
		VolumePools:                 volumePoolOutputs(volumePools),
		VolumeDrainOperations:       volumeDrainOperationOutputs(volumeDrainOperations),
		WorkerLeases:                workerLeaseOutputs(workerLeases),
		WorkerOperations:            workerOperationOutputs(workerOperations),
	}, nil
}

func loadMetadataImportDryRun(ctx context.Context, req metadataImportDryRunRequest) (metadataImportDryRunOutput, error) {
	input, err := readMetadataExportInput(req.InputPath)
	if err != nil {
		return metadataImportDryRunOutput{}, err
	}
	if input.SchemaVersion != 1 {
		return metadataImportDryRunOutput{}, fmt.Errorf("unsupported metadata export schema version %d", input.SchemaVersion)
	}
	sourceCounts := metadataExportCounts(input)
	targetCounts := metadataCollectionCount{}
	if req.TargetChecked {
		var err error
		targetCounts, err = metadataRepositoryCounts(ctx, req.Target, req.TargetScanLimit)
		if err != nil {
			return metadataImportDryRunOutput{}, err
		}
	}
	sourceConflicts := metadataImportConflicts(input)
	conflicts := append([]metadataImportConflict(nil), sourceConflicts...)
	targetEmpty := metadataTargetCountsEmpty(targetCounts)
	if req.TargetChecked && req.RequireEmptyTarget && !targetEmpty {
		conflicts = append(conflicts, metadataTargetConflicts(targetCounts)...)
	}
	readyForApply := len(conflicts) == 0
	applyPlan := metadataImportApplyPlanFor(req, sourceCounts, targetCounts, targetEmpty, len(sourceConflicts), len(conflicts))
	return metadataImportDryRunOutput{
		SchemaVersion:  input.SchemaVersion,
		DryRun:         true,
		Valid:          true,
		ReadyForApply:  readyForApply,
		Source:         req.InputPath,
		Counts:         sourceCounts,
		TargetChecked:  req.TargetChecked,
		TargetEmpty:    targetEmpty,
		TargetCounts:   targetCounts,
		ConflictPolicy: req.ConflictPolicy,
		ApplyPlan:      applyPlan,
		Actions:        metadataImportActions(sourceCounts, targetCounts, req.ConflictPolicy),
		Conflicts:      conflicts,
	}, nil
}

func loadMetadataImportRequest(inputPath string, requireEmptyTarget bool) (meta.ImportOperationalMetadataRequest, error) {
	input, err := readMetadataExportInput(inputPath)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}
	if input.SchemaVersion != 1 {
		return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("unsupported metadata export schema version %d", input.SchemaVersion)
	}
	return metadataImportRequestFromExport(input, requireEmptyTarget)
}

func readMetadataExportInput(inputPath string) (metadataExportOutput, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return metadataExportOutput{}, err
	}
	defer file.Close()

	var input metadataExportOutput
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return metadataExportOutput{}, err
	}
	return input, nil
}

func metadataImportRequestFromExport(input metadataExportOutput, requireEmptyTarget bool) (meta.ImportOperationalMetadataRequest, error) {
	metadataSchema, err := metadataSchemaFromExport(input.MetadataSchema)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}
	metadataMigrationOperations, err := metadataMigrationOperationsFromExport(input.MetadataMigrationOperations)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}
	kmsKeys := make([]model.KMSKeyRecord, 0, len(input.KMSKeys))
	for _, source := range input.KMSKeys {
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("kms key %q created_at: %w", source.KeyID, err)
		}
		updatedAt, err := parseOptionalRFC3339(source.UpdatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("kms key %q updated_at: %w", source.KeyID, err)
		}
		kmsKeys = append(kmsKeys, model.KMSKeyRecord{
			KeyID:      source.KeyID,
			KeyVersion: source.KeyVersion,
			State:      model.NormalizeKMSKeyState(model.KMSKeyState(source.State)),
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		})
	}

	auditEvents := make([]model.AuditEvent, 0, len(input.AuditEvents))
	for _, source := range input.AuditEvents {
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("audit event %q created_at: %w", source.EventID, err)
		}
		auditEvents = append(auditEvents, model.AuditEvent{
			EventID:      source.EventID,
			Action:       model.AuditAction(source.Action),
			BucketID:     source.BucketID,
			Key:          source.Key,
			VersionID:    source.VersionID,
			RequestID:    source.RequestID,
			Reason:       source.Reason,
			Principal:    source.Principal,
			Details:      cloneStringMap(source.Details),
			PreviousHash: source.PreviousHash,
			EventHash:    source.EventHash,
			CreatedAt:    createdAt,
		})
	}

	gcOperations := make([]model.GCOperationRecord, 0, len(input.GCOperations))
	for _, source := range input.GCOperations {
		startedAt, err := parseOptionalRFC3339(source.StartedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("gc operation %q started_at: %w", source.OperationID, err)
		}
		finishedAt, err := parseOptionalRFC3339(source.FinishedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("gc operation %q finished_at: %w", source.OperationID, err)
		}
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("gc operation %q created_at: %w", source.OperationID, err)
		}
		gcOperations = append(gcOperations, model.GCOperationRecord{
			OperationID:         source.OperationID,
			ResumeOfOperationID: source.ResumeOfOperationID,
			Status:              source.Status,
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			Scanned:             source.Scanned,
			Deleted:             source.Deleted,
			Skipped:             source.Skipped,
			Retryable:           source.Retryable,
			CreatedAt:           createdAt,
		})
	}

	dedupeOperations := make([]model.DedupeOperationRecord, 0, len(input.DedupeOperations))
	for _, source := range input.DedupeOperations {
		startedAt, err := parseOptionalRFC3339(source.StartedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("dedupe operation %q started_at: %w", source.OperationID, err)
		}
		finishedAt, err := parseOptionalRFC3339(source.FinishedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("dedupe operation %q finished_at: %w", source.OperationID, err)
		}
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("dedupe operation %q created_at: %w", source.OperationID, err)
		}
		attempts := make([]model.DedupeOperationAttempt, 0, len(source.Attempts))
		for _, attempt := range source.Attempts {
			attempts = append(attempts, model.DedupeOperationAttempt{
				BucketID:         attempt.BucketID,
				Key:              attempt.Key,
				SourceVersion:    attempt.SourceVersion,
				CandidateVersion: attempt.CandidateVersion,
				PlanStatus:       attempt.PlanStatus,
				PlanReason:       attempt.PlanReason,
				Status:           attempt.Status,
				SharedObjectID:   attempt.SharedObjectID,
				OrphansMarked:    attempt.OrphansMarked,
				Retryable:        attempt.Retryable,
				Error:            attempt.Error,
			})
		}
		dedupeOperations = append(dedupeOperations, model.DedupeOperationRecord{
			OperationID:         source.OperationID,
			ResumeOfOperationID: source.ResumeOfOperationID,
			Status:              source.Status,
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			Scanned:             source.Scanned,
			Acked:               source.Acked,
			Skipped:             source.Skipped,
			Retryable:           source.Retryable,
			Attempts:            attempts,
			CreatedAt:           createdAt,
		})
	}

	sharedObjects := make([]model.SharedObject, 0, len(input.SharedObjects))
	for _, source := range input.SharedObjects {
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("shared object %q created_at: %w", source.SharedObjectID, err)
		}
		updatedAt, err := parseOptionalRFC3339(source.UpdatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("shared object %q updated_at: %w", source.SharedObjectID, err)
		}
		sharedObjects = append(sharedObjects, model.SharedObject{
			SharedObjectID:     source.SharedObjectID,
			TenantID:           source.TenantID,
			BucketID:           source.BucketID,
			Key:                source.Key,
			SourceVersionID:    source.SourceVersionID,
			SizeBytes:          source.SizeBytes,
			Digest:             storage.Digest{Algorithm: source.DigestAlgorithm, Hex: source.DigestHex},
			RefCount:           source.RefCount,
			ProtectedRootCount: source.ProtectedRootCount,
			CreatedAt:          createdAt,
			UpdatedAt:          updatedAt,
		})
	}

	sharedObjectReleases := make([]model.SharedObjectRelease, 0, len(input.SharedObjectReleases))
	for _, source := range input.SharedObjectReleases {
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("shared object release %q created_at: %w", source.ReleaseID, err)
		}
		updatedAt, err := parseOptionalRFC3339(source.UpdatedAt)
		if err != nil {
			return meta.ImportOperationalMetadataRequest{}, fmt.Errorf("shared object release %q updated_at: %w", source.ReleaseID, err)
		}
		sharedObjectReleases = append(sharedObjectReleases, model.SharedObjectRelease{
			ReleaseID:      source.ReleaseID,
			SharedObjectID: source.SharedObjectID,
			SegmentID:      source.SegmentID,
			SegmentRef: storage.SegmentRef{
				SegmentID:      source.SegmentID,
				SharedObjectID: source.SharedObjectID,
			},
			Reason:    storage.DeleteReason(source.Reason),
			Status:    model.SharedObjectReleaseStatus(source.Status),
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		})
	}

	volumePools, err := volumePoolsFromExport(input.VolumePools)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}
	volumeDrainOperations, err := volumeDrainOperationsFromExport(input.VolumeDrainOperations)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}
	workerLeases, err := workerLeasesFromExport(input.WorkerLeases)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}
	workerOperations, err := workerOperationsFromExport(input.WorkerOperations)
	if err != nil {
		return meta.ImportOperationalMetadataRequest{}, err
	}

	return meta.ImportOperationalMetadataRequest{
		MetadataSchema:              metadataSchema,
		MetadataMigrationOperations: metadataMigrationOperations,
		KMSKeys:                     kmsKeys,
		AuditEvents:                 auditEvents,
		GCOperations:                gcOperations,
		DedupeOperations:            dedupeOperations,
		SharedObjects:               sharedObjects,
		SharedObjectReleases:        sharedObjectReleases,
		VolumePools:                 volumePools,
		VolumeDrainOperations:       volumeDrainOperations,
		WorkerLeases:                workerLeases,
		WorkerOperations:            workerOperations,
		RequireEmptyTarget:          requireEmptyTarget,
	}, nil
}

func metadataSchemaFromExport(source *metadataSchemaExportOutput) (*model.MetadataSchemaRecord, error) {
	if source == nil {
		return nil, nil
	}
	createdAt, err := parseOptionalRFC3339(source.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("metadata schema created_at: %w", err)
	}
	updatedAt, err := parseOptionalRFC3339(source.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("metadata schema updated_at: %w", err)
	}
	record := model.MetadataSchemaRecord{
		SchemaVersion:    source.SchemaVersion,
		MinReaderVersion: source.MinReaderVersion,
		MinWriterVersion: source.MinWriterVersion,
		UpdatedBy:        source.UpdatedBy,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
	if err := meta.ValidateMetadataSchemaRecord(record); err != nil {
		return nil, err
	}
	return &record, nil
}

func metadataMigrationOperationsFromExport(sources []metadataMigrationOperationOutput) ([]model.MetadataMigrationOperationRecord, error) {
	records := make([]model.MetadataMigrationOperationRecord, 0, len(sources))
	for _, source := range sources {
		startedAt, err := parseOptionalRFC3339(source.StartedAt)
		if err != nil {
			return nil, fmt.Errorf("metadata migration operation %q started_at: %w", source.OperationID, err)
		}
		finishedAt, err := parseOptionalRFC3339(source.FinishedAt)
		if err != nil {
			return nil, fmt.Errorf("metadata migration operation %q finished_at: %w", source.OperationID, err)
		}
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("metadata migration operation %q created_at: %w", source.OperationID, err)
		}
		steps := make([]model.MetadataMigrationStep, 0, len(source.Steps))
		for _, step := range source.Steps {
			steps = append(steps, model.MetadataMigrationStep{
				Name:            step.Name,
				Status:          model.MetadataMigrationStepStatus(step.Status),
				Message:         step.Message,
				RepairNeeded:    step.RepairNeeded,
				RecordsScanned:  step.RecordsScanned,
				RecordsRepaired: step.RecordsRepaired,
			})
		}
		records = append(records, model.MetadataMigrationOperationRecord{
			OperationID:         source.OperationID,
			ResumeOfOperationID: source.ResumeOfOperationID,
			TargetSchemaVersion: source.TargetSchemaVersion,
			Status:              model.MetadataMigrationOperationStatus(source.Status),
			DryRun:              source.DryRun,
			Apply:               source.Apply,
			OwnerID:             source.OwnerID,
			Cursor:              source.Cursor,
			Steps:               steps,
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			CreatedAt:           createdAt,
		})
	}
	return records, nil
}

func volumePoolsFromExport(sources []volumePoolOutput) ([]model.VolumePool, error) {
	pools := make([]model.VolumePool, 0, len(sources))
	for _, source := range sources {
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("volume pool %q created_at: %w", source.PoolID, err)
		}
		updatedAt, err := parseOptionalRFC3339(source.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("volume pool %q updated_at: %w", source.PoolID, err)
		}
		members := make([]model.VolumePoolMember, 0, len(source.Members))
		for _, member := range source.Members {
			lastObservedAt, err := parseOptionalRFC3339(member.LastObservedAt)
			if err != nil {
				return nil, fmt.Errorf("volume pool %q member %q last_observed_at: %w", source.PoolID, member.VolumeID, err)
			}
			members = append(members, model.VolumePoolMember{
				VolumeID:             member.VolumeID,
				AdminEndpoint:        member.AdminEndpoint,
				DataEndpoint:         member.DataEndpoint,
				GatewayID:            member.GatewayID,
				AttachmentID:         member.AttachmentID,
				Generation:           member.Generation,
				ChunkSizeBytes:       member.ChunkSizeBytes,
				State:                model.VolumePoolState(member.State),
				ReadOnly:             member.ReadOnly,
				Weight:               member.Weight,
				CapacityBytes:        member.CapacityBytes,
				AvailableBytes:       member.AvailableBytes,
				UsedPercent:          member.UsedPercent,
				HighWatermarkPercent: member.HighWatermarkPercent,
				LastObservedAt:       lastObservedAt,
			})
		}
		pools = append(pools, model.VolumePool{
			PoolID:          source.PoolID,
			Generation:      source.Generation,
			DurabilityClass: source.DurabilityClass,
			StorageClassIDs: append([]string(nil), source.StorageClassIDs...),
			Members:         members,
			CreatedAt:       createdAt,
			UpdatedAt:       updatedAt,
		})
	}
	return pools, nil
}

func volumeDrainOperationsFromExport(sources []volumeDrainOperationOutput) ([]model.VolumeDrainOperationRecord, error) {
	records := make([]model.VolumeDrainOperationRecord, 0, len(sources))
	for _, source := range sources {
		startedAt, err := parseOptionalRFC3339(source.StartedAt)
		if err != nil {
			return nil, fmt.Errorf("volume drain operation %q started_at: %w", source.OperationID, err)
		}
		finishedAt, err := parseOptionalRFC3339(source.FinishedAt)
		if err != nil {
			return nil, fmt.Errorf("volume drain operation %q finished_at: %w", source.OperationID, err)
		}
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("volume drain operation %q created_at: %w", source.OperationID, err)
		}
		attempts := make([]model.VolumeDrainAttempt, 0, len(source.Attempts))
		for _, attempt := range source.Attempts {
			sourceRef := storage.SegmentRef{SegmentID: attempt.SourceSegmentID}
			targetRef := storage.SegmentRef{SegmentID: attempt.TargetSegmentID}
			setSegmentRefVolumeID(&sourceRef, attempt.SourceVolumeID)
			setSegmentRefVolumeID(&targetRef, attempt.TargetVolumeID)
			attempts = append(attempts, model.VolumeDrainAttempt{
				BucketID:        attempt.BucketID,
				Key:             attempt.Key,
				VersionID:       attempt.VersionID,
				SourceSegmentID: attempt.SourceSegmentID,
				SourceRef:       sourceRef,
				TargetSegmentID: attempt.TargetSegmentID,
				TargetRef:       targetRef,
				Status:          model.VolumeDrainAttemptStatus(attempt.Status),
				Protected:       attempt.Protected,
				Retryable:       attempt.Retryable,
				Error:           attempt.Error,
			})
		}
		records = append(records, model.VolumeDrainOperationRecord{
			OperationID:         source.OperationID,
			ResumeOfOperationID: source.ResumeOfOperationID,
			PoolID:              source.PoolID,
			SourceVolumeID:      source.SourceVolumeID,
			TargetVolumeID:      source.TargetVolumeID,
			OwnerID:             source.OwnerID,
			Status:              model.VolumeDrainOperationStatus(source.Status),
			Cursor:              source.Cursor,
			StartedAt:           startedAt,
			FinishedAt:          finishedAt,
			Scanned:             source.Scanned,
			Copied:              source.Copied,
			Skipped:             source.Skipped,
			Protected:           source.Protected,
			Retryable:           source.Retryable,
			Attempts:            attempts,
			CreatedAt:           createdAt,
		})
	}
	return records, nil
}

func workerLeasesFromExport(sources []workerLeaseOutput) ([]model.WorkerLease, error) {
	records := make([]model.WorkerLease, 0, len(sources))
	for _, source := range sources {
		acquiredAt, err := parseOptionalRFC3339(source.AcquiredAt)
		if err != nil {
			return nil, fmt.Errorf("worker lease %q acquired_at: %w", source.LeaseID, err)
		}
		updatedAt, err := parseOptionalRFC3339(source.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("worker lease %q updated_at: %w", source.LeaseID, err)
		}
		expiresAt, err := parseOptionalRFC3339(source.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("worker lease %q expires_at: %w", source.LeaseID, err)
		}
		records = append(records, model.WorkerLease{
			LeaseID:    source.LeaseID,
			WorkerKind: source.WorkerKind,
			ShardID:    source.ShardID,
			OwnerID:    source.OwnerID,
			Generation: source.Generation,
			Cursor:     source.Cursor,
			AcquiredAt: acquiredAt,
			UpdatedAt:  updatedAt,
			ExpiresAt:  expiresAt,
		})
	}
	return records, nil
}

func workerOperationsFromExport(sources []workerOperationOutput) ([]model.WorkerOperationRecord, error) {
	records := make([]model.WorkerOperationRecord, 0, len(sources))
	for _, source := range sources {
		startedAt, err := parseOptionalRFC3339(source.StartedAt)
		if err != nil {
			return nil, fmt.Errorf("worker operation %q started_at: %w", source.OperationID, err)
		}
		finishedAt, err := parseOptionalRFC3339(source.FinishedAt)
		if err != nil {
			return nil, fmt.Errorf("worker operation %q finished_at: %w", source.OperationID, err)
		}
		createdAt, err := parseOptionalRFC3339(source.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("worker operation %q created_at: %w", source.OperationID, err)
		}
		records = append(records, model.WorkerOperationRecord{
			OperationID: source.OperationID,
			WorkerKind:  source.WorkerKind,
			ShardID:     source.ShardID,
			OwnerID:     source.OwnerID,
			LeaseID:     source.LeaseID,
			Status:      model.WorkerOperationStatus(source.Status),
			Cursor:      source.Cursor,
			Scanned:     source.Scanned,
			Processed:   source.Processed,
			Skipped:     source.Skipped,
			Retryable:   source.Retryable,
			LastError:   source.LastError,
			StartedAt:   startedAt,
			FinishedAt:  finishedAt,
			CreatedAt:   createdAt,
		})
	}
	return records, nil
}

func metadataExportCounts(input metadataExportOutput) metadataCollectionCount {
	return metadataCollectionCount{
		MetadataSchema:              boolCount(input.MetadataSchema != nil),
		MetadataMigrationOperations: len(input.MetadataMigrationOperations),
		KMSKeys:                     len(input.KMSKeys),
		AuditEvents:                 len(input.AuditEvents),
		GCOperations:                len(input.GCOperations),
		DedupeOperations:            len(input.DedupeOperations),
		SharedObjects:               len(input.SharedObjects),
		SharedObjectReleases:        len(input.SharedObjectReleases),
		VolumePools:                 len(input.VolumePools),
		VolumeDrainOperations:       len(input.VolumeDrainOperations),
		WorkerLeases:                len(input.WorkerLeases),
		WorkerOperations:            len(input.WorkerOperations),
	}
}

func metadataRepositoryCounts(ctx context.Context, repo meta.Repository, limit int) (metadataCollectionCount, error) {
	if limit <= 0 {
		limit = 1000
	}
	metadataSchema := 0
	if _, err := repo.GetMetadataSchema(ctx); err == nil {
		metadataSchema = 1
	} else if !errors.Is(err, meta.ErrNotFound) {
		return metadataCollectionCount{}, err
	}
	migrationOperations, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	kmsKeys, err := repo.ListKMSKeys(ctx, meta.ListKMSKeysRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	auditEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	gcOperations, err := repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	dedupeOperations, err := repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	sharedObjects, err := repo.ListSharedObjects(ctx, meta.ListSharedObjectsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	sharedReleases, err := repo.ListSharedObjectReleases(ctx, meta.ListSharedObjectReleasesRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	volumePools, err := repo.ListVolumePools(ctx, meta.ListVolumePoolsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	volumeDrainOperations, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	workerLeases, err := repo.ListWorkerLeases(ctx, meta.ListWorkerLeasesRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	workerOperations, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{Limit: limit})
	if err != nil {
		return metadataCollectionCount{}, err
	}
	return metadataCollectionCount{
		MetadataSchema:              metadataSchema,
		MetadataMigrationOperations: len(migrationOperations),
		KMSKeys:                     len(kmsKeys),
		AuditEvents:                 len(auditEvents),
		GCOperations:                len(gcOperations),
		DedupeOperations:            len(dedupeOperations),
		SharedObjects:               len(sharedObjects),
		SharedObjectReleases:        len(sharedReleases),
		VolumePools:                 len(volumePools),
		VolumeDrainOperations:       len(volumeDrainOperations),
		WorkerLeases:                len(workerLeases),
		WorkerOperations:            len(workerOperations),
	}, nil
}

func metadataCountsEmpty(counts metadataCollectionCount) bool {
	return counts.MetadataSchema == 0 &&
		counts.MetadataMigrationOperations == 0 &&
		counts.KMSKeys == 0 &&
		counts.AuditEvents == 0 &&
		counts.GCOperations == 0 &&
		counts.DedupeOperations == 0 &&
		counts.SharedObjects == 0 &&
		counts.SharedObjectReleases == 0 &&
		counts.VolumePools == 0 &&
		counts.VolumeDrainOperations == 0 &&
		counts.WorkerLeases == 0 &&
		counts.WorkerOperations == 0
}

func metadataTargetCountsEmpty(counts metadataCollectionCount) bool {
	counts.MetadataSchema = 0
	return metadataCountsEmpty(counts)
}

func metadataImportActions(source, target metadataCollectionCount, policy string) []metadataImportAction {
	return []metadataImportAction{
		metadataImportSchemaAction(source.MetadataSchema, target.MetadataSchema),
		metadataImportActionFor("metadata_migration_operations", source.MetadataMigrationOperations, target.MetadataMigrationOperations, policy, false),
		metadataImportActionFor("kms_keys", source.KMSKeys, target.KMSKeys, policy, false),
		metadataImportActionFor("audit_events", source.AuditEvents, target.AuditEvents, policy, true),
		metadataImportActionFor("gc_operations", source.GCOperations, target.GCOperations, policy, false),
		metadataImportActionFor("dedupe_operations", source.DedupeOperations, target.DedupeOperations, policy, false),
		metadataImportActionFor("shared_objects", source.SharedObjects, target.SharedObjects, policy, false),
		metadataImportActionFor("shared_object_releases", source.SharedObjectReleases, target.SharedObjectReleases, policy, false),
		metadataImportActionFor("volume_pools", source.VolumePools, target.VolumePools, policy, false),
		metadataImportActionFor("volume_drain_operations", source.VolumeDrainOperations, target.VolumeDrainOperations, policy, false),
		metadataImportActionFor("worker_leases", source.WorkerLeases, target.WorkerLeases, policy, false),
		metadataImportActionFor("worker_operations", source.WorkerOperations, target.WorkerOperations, policy, false),
	}
}

func metadataImportSchemaAction(importRecords, targetRecords int) metadataImportAction {
	return metadataImportAction{
		Collection:    "metadata_schema",
		Operation:     "upsert_schema_marker",
		ImportRecords: importRecords,
		TargetRecords: targetRecords,
		Policy:        "upsert",
		PreserveIDs:   true,
		WriteEnabled:  false,
	}
}

func metadataImportActionFor(collection string, importRecords, targetRecords int, policy string, preserveHashes bool) metadataImportAction {
	return metadataImportAction{
		Collection:     collection,
		Operation:      "insert_preserve_source_id",
		ImportRecords:  importRecords,
		TargetRecords:  targetRecords,
		Policy:         policy,
		PreserveIDs:    true,
		PreserveHashes: preserveHashes,
		WriteEnabled:   false,
	}
}

func metadataImportApply(ctx context.Context, target meta.Repository, output metadataImportDryRunOutput, req meta.ImportOperationalMetadataRequest, experimentalAllowed bool) metadataImportApplyResult {
	collections := make([]metadataImportApplyCollectionResult, 0, len(output.Actions))
	recordsPlanned := 0
	for _, action := range output.Actions {
		status := "planned"
		if action.ImportRecords == 0 {
			status = "skipped_empty_source"
		}
		recordsPlanned += action.ImportRecords
		collections = append(collections, metadataImportApplyCollectionResult{
			Collection:     action.Collection,
			Status:         status,
			Operation:      action.Operation,
			RecordsPlanned: action.ImportRecords,
			RecordsWritten: 0,
			PreserveIDs:    action.PreserveIDs,
			PreserveHashes: action.PreserveHashes,
		})
	}

	result := metadataImportApplyResult{
		Status:              "blocked",
		WriteEnabled:        false,
		ApplySupported:      true,
		Ready:               output.ApplyPlan.Ready,
		ExperimentalAllowed: experimentalAllowed,
		RecordsPlanned:      recordsPlanned,
		RecordsWritten:      0,
		Collections:         collections,
	}
	if !experimentalAllowed {
		result.Message = "metadata import apply requires -allow-experimental-apply"
		result.Limitations = []string{"apply remains gated because restore writes must be explicitly acknowledged"}
		return result
	}
	if target == nil {
		result.Message = "metadata import apply requires target metadata backend"
		return result
	}
	if !output.ApplyPlan.Ready {
		result.Message = "metadata import preflight is not ready"
		return result
	}

	writeResult, err := target.ImportOperationalMetadata(ctx, req)
	result.WriteEnabled = true
	result.Ready = true
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
		return result
	}
	writtenByCollection := map[string]int{
		"metadata_schema":               writeResult.MetadataSchema,
		"metadata_migration_operations": writeResult.MetadataMigrationOperations,
		"kms_keys":                      writeResult.KMSKeys,
		"audit_events":                  writeResult.AuditEvents,
		"gc_operations":                 writeResult.GCOperations,
		"dedupe_operations":             writeResult.DedupeOperations,
		"shared_objects":                writeResult.SharedObjects,
		"shared_object_releases":        writeResult.SharedObjectReleases,
		"volume_pools":                  writeResult.VolumePools,
		"volume_drain_operations":       writeResult.VolumeDrainOperations,
		"worker_leases":                 writeResult.WorkerLeases,
		"worker_operations":             writeResult.WorkerOperations,
	}
	recordsWritten := 0
	for i := range result.Collections {
		written := writtenByCollection[result.Collections[i].Collection]
		result.Collections[i].RecordsWritten = written
		recordsWritten += written
		if result.Collections[i].RecordsPlanned == 0 {
			result.Collections[i].Status = "skipped_empty_source"
		} else if written == result.Collections[i].RecordsPlanned {
			result.Collections[i].Status = "written"
		} else {
			result.Collections[i].Status = "partial"
		}
	}
	result.Status = "succeeded"
	result.RecordsWritten = recordsWritten
	result.Message = "metadata import apply completed"
	return result
}

func metadataImportApplyPlanFor(req metadataImportDryRunRequest, sourceCounts, targetCounts metadataCollectionCount, targetEmpty bool, sourceConflictCount, totalConflictCount int) metadataImportApplyPlan {
	ready := req.TargetChecked &&
		req.ConflictPolicy == "fail_if_exists" &&
		totalConflictCount == 0 &&
		(!req.RequireEmptyTarget || targetEmpty)
	gates := []metadataImportApplyGate{
		{Name: "source_schema", Status: "passed", Message: "metadata export schema version is supported"},
		{Name: "source_ids_unique", Status: gateStatus(sourceConflictCount == 0), Message: "source collection ids must be present and unique"},
		{Name: "conflict_policy", Status: gateStatus(req.ConflictPolicy == "fail_if_exists"), Message: "only fail_if_exists is accepted before apply writes are enabled"},
		{Name: "target_checked", Status: gateStatus(req.TargetChecked), Message: "target metadata backend must be checked before apply"},
	}
	if req.TargetChecked && req.RequireEmptyTarget {
		gates = append(gates, metadataImportApplyGate{
			Name:    "target_empty",
			Status:  gateStatus(targetEmpty),
			Message: "target metadata must be empty for fail_if_exists restore preflight",
		})
	}
	if metadataCountsEmpty(sourceCounts) {
		gates = append(gates, metadataImportApplyGate{
			Name:    "source_non_empty",
			Status:  "warning",
			Message: "source export has no operational metadata records to import",
		})
	}
	if req.TargetChecked && !metadataTargetCountsEmpty(targetCounts) && !req.RequireEmptyTarget {
		gates = append(gates, metadataImportApplyGate{
			Name:    "target_not_empty",
			Status:  "warning",
			Message: "target contains existing metadata; future apply must resolve per-record conflicts",
		})
	}
	gates = append(gates, metadataImportApplyGate{
		Name:    "write_path",
		Status:  "passed",
		Message: "repository operational metadata import primitive is available",
	})
	status := "blocked"
	if ready {
		status = "ready"
	}
	limitations := []string(nil)
	if !ready {
		limitations = []string{"apply writes require target metadata preflight and fail_if_exists conflict checks"}
	}
	return metadataImportApplyPlan{
		Status:              status,
		WriteEnabled:        ready,
		ApplySupported:      true,
		Ready:               ready,
		ConflictPolicy:      req.ConflictPolicy,
		RequireEmptyTarget:  req.RequireEmptyTarget,
		PreserveSourceIDs:   true,
		PreserveAuditHashes: true,
		Gates:               gates,
		Limitations:         limitations,
	}
}

func gateStatus(pass bool) string {
	if pass {
		return "passed"
	}
	return "blocked"
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func metadataImportConflicts(input metadataExportOutput) []metadataImportConflict {
	var conflicts []metadataImportConflict
	conflicts = append(conflicts, duplicateMetadataImportConflicts("metadata_migration_operations", metadataMigrationOperationExportIDs(input.MetadataMigrationOperations))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("kms_keys", kmsKeyExportIDs(input.KMSKeys))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("audit_events", auditEventExportIDs(input.AuditEvents))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("gc_operations", gcOperationExportIDs(input.GCOperations))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("dedupe_operations", dedupeOperationExportIDs(input.DedupeOperations))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("shared_objects", sharedObjectExportIDs(input.SharedObjects))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("shared_object_releases", sharedObjectReleaseExportIDs(input.SharedObjectReleases))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("volume_pools", volumePoolExportIDs(input.VolumePools))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("volume_drain_operations", volumeDrainOperationExportIDs(input.VolumeDrainOperations))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("worker_leases", workerLeaseExportIDs(input.WorkerLeases))...)
	conflicts = append(conflicts, duplicateMetadataImportConflicts("worker_operations", workerOperationExportIDs(input.WorkerOperations))...)
	return conflicts
}

func duplicateMetadataImportConflicts(collection string, ids []string) []metadataImportConflict {
	seen := make(map[string]struct{}, len(ids))
	var conflicts []metadataImportConflict
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			conflicts = append(conflicts, metadataImportConflict{Collection: collection, Reason: "missing_id"})
			continue
		}
		if _, ok := seen[id]; ok {
			conflicts = append(conflicts, metadataImportConflict{Collection: collection, ID: id, Reason: "duplicate_source_id"})
			continue
		}
		seen[id] = struct{}{}
	}
	return conflicts
}

func metadataTargetConflicts(counts metadataCollectionCount) []metadataImportConflict {
	targets := []metadataImportAction{
		{Collection: "metadata_migration_operations", TargetRecords: counts.MetadataMigrationOperations},
		{Collection: "kms_keys", TargetRecords: counts.KMSKeys},
		{Collection: "audit_events", TargetRecords: counts.AuditEvents},
		{Collection: "gc_operations", TargetRecords: counts.GCOperations},
		{Collection: "dedupe_operations", TargetRecords: counts.DedupeOperations},
		{Collection: "shared_objects", TargetRecords: counts.SharedObjects},
		{Collection: "shared_object_releases", TargetRecords: counts.SharedObjectReleases},
		{Collection: "volume_pools", TargetRecords: counts.VolumePools},
		{Collection: "volume_drain_operations", TargetRecords: counts.VolumeDrainOperations},
		{Collection: "worker_leases", TargetRecords: counts.WorkerLeases},
		{Collection: "worker_operations", TargetRecords: counts.WorkerOperations},
	}
	conflicts := make([]metadataImportConflict, 0, len(targets))
	for _, target := range targets {
		if target.TargetRecords > 0 {
			conflicts = append(conflicts, metadataImportConflict{
				Collection: target.Collection,
				Reason:     "target_not_empty",
			})
		}
	}
	return conflicts
}

func kmsKeyExportIDs(records []kmsKeyExportOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.KeyID)
	}
	return ids
}

func auditEventExportIDs(records []auditEventExportOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.EventID)
	}
	return ids
}

func gcOperationExportIDs(records []gcOperationExportOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.OperationID)
	}
	return ids
}

func metadataMigrationOperationExportIDs(records []metadataMigrationOperationOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.OperationID)
	}
	return ids
}

func dedupeOperationExportIDs(records []dedupeOperationOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.OperationID)
	}
	return ids
}

func sharedObjectExportIDs(records []sharedObjectExportOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.SharedObjectID)
	}
	return ids
}

func sharedObjectReleaseExportIDs(records []sharedObjectReleaseOperationOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.ReleaseID)
	}
	return ids
}

func volumePoolExportIDs(records []volumePoolOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.PoolID)
	}
	return ids
}

func volumeDrainOperationExportIDs(records []volumeDrainOperationOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.OperationID)
	}
	return ids
}

func workerLeaseExportIDs(records []workerLeaseOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.LeaseID)
	}
	return ids
}

func workerOperationExportIDs(records []workerOperationOutput) []string {
	ids := make([]string, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.OperationID)
	}
	return ids
}

func auditEventExportOutputs(events []model.AuditEvent) []auditEventExportOutput {
	if len(events) == 0 {
		return nil
	}
	out := make([]auditEventExportOutput, 0, len(events))
	for _, event := range events {
		out = append(out, auditEventExportOutput{
			EventID:      event.EventID,
			Action:       string(event.Action),
			BucketID:     event.BucketID,
			Key:          event.Key,
			VersionID:    event.VersionID,
			RequestID:    event.RequestID,
			Reason:       event.Reason,
			Principal:    event.Principal,
			Details:      cloneStringMap(event.Details),
			PreviousHash: event.PreviousHash,
			EventHash:    event.EventHash,
			CreatedAt:    formatJSONTime(event.CreatedAt),
		})
	}
	return out
}

func kmsKeyExportOutputs(records []model.KMSKeyRecord) []kmsKeyExportOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]kmsKeyExportOutput, 0, len(records))
	for _, record := range records {
		out = append(out, kmsKeyExportOutput{
			KeyID:      record.KeyID,
			KeyVersion: record.KeyVersion,
			State:      string(model.NormalizeKMSKeyState(record.State)),
			CreatedAt:  formatJSONTime(record.CreatedAt),
			UpdatedAt:  formatJSONTime(record.UpdatedAt),
		})
	}
	return out
}

func gcOperationExportOutputs(records []model.GCOperationRecord) []gcOperationExportOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]gcOperationExportOutput, 0, len(records))
	for _, record := range records {
		out = append(out, gcOperationExportOutput{
			OperationID:         record.OperationID,
			ResumeOfOperationID: record.ResumeOfOperationID,
			Status:              record.Status,
			StartedAt:           formatJSONTime(record.StartedAt),
			FinishedAt:          formatJSONTime(record.FinishedAt),
			Scanned:             record.Scanned,
			Deleted:             record.Deleted,
			Skipped:             record.Skipped,
			Retryable:           record.Retryable,
			CreatedAt:           formatJSONTime(record.CreatedAt),
		})
	}
	return out
}

func workerOperationOutputs(records []model.WorkerOperationRecord) []workerOperationOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]workerOperationOutput, 0, len(records))
	for _, record := range records {
		out = append(out, workerOperationOutput{
			OperationID: record.OperationID,
			WorkerKind:  record.WorkerKind,
			ShardID:     record.ShardID,
			OwnerID:     record.OwnerID,
			LeaseID:     record.LeaseID,
			Status:      string(record.Status),
			Cursor:      record.Cursor,
			Scanned:     record.Scanned,
			Processed:   record.Processed,
			Skipped:     record.Skipped,
			Retryable:   record.Retryable,
			LastError:   record.LastError,
			StartedAt:   formatJSONTime(record.StartedAt),
			FinishedAt:  formatJSONTime(record.FinishedAt),
			CreatedAt:   formatJSONTime(record.CreatedAt),
		})
	}
	return out
}

func workerLeaseOutputs(records []model.WorkerLease) []workerLeaseOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]workerLeaseOutput, 0, len(records))
	for _, record := range records {
		out = append(out, workerLeaseOutput{
			LeaseID:    record.LeaseID,
			WorkerKind: record.WorkerKind,
			ShardID:    record.ShardID,
			OwnerID:    record.OwnerID,
			Generation: record.Generation,
			Cursor:     record.Cursor,
			AcquiredAt: formatJSONTime(record.AcquiredAt),
			UpdatedAt:  formatJSONTime(record.UpdatedAt),
			ExpiresAt:  formatJSONTime(record.ExpiresAt),
		})
	}
	return out
}

func metadataMigrationOperationOutputs(records []model.MetadataMigrationOperationRecord) []metadataMigrationOperationOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]metadataMigrationOperationOutput, 0, len(records))
	for _, record := range records {
		out = append(out, metadataMigrationOperationOutputFromRecord(record))
	}
	return out
}

func metadataMigrationOperationOutputFromRecord(record model.MetadataMigrationOperationRecord) metadataMigrationOperationOutput {
	return metadataMigrationOperationOutput{
		OperationID:         record.OperationID,
		ResumeOfOperationID: record.ResumeOfOperationID,
		TargetSchemaVersion: record.TargetSchemaVersion,
		Status:              string(record.Status),
		DryRun:              record.DryRun,
		Apply:               record.Apply,
		OwnerID:             record.OwnerID,
		Cursor:              record.Cursor,
		Steps:               metadataMigrationStepOutputs(record.Steps),
		StartedAt:           formatJSONTime(record.StartedAt),
		FinishedAt:          formatJSONTime(record.FinishedAt),
		CreatedAt:           formatJSONTime(record.CreatedAt),
	}
}

func metadataMigrationStepOutputs(steps []model.MetadataMigrationStep) []metadataMigrationStepOutput {
	if len(steps) == 0 {
		return nil
	}
	out := make([]metadataMigrationStepOutput, 0, len(steps))
	for _, step := range steps {
		out = append(out, metadataMigrationStepOutput{
			Name:            step.Name,
			Status:          string(step.Status),
			Message:         step.Message,
			RepairNeeded:    step.RepairNeeded,
			RecordsScanned:  step.RecordsScanned,
			RecordsRepaired: step.RecordsRepaired,
		})
	}
	return out
}

func metadataSchemaExportOutputFromModel(record model.MetadataSchemaRecord) *metadataSchemaExportOutput {
	return &metadataSchemaExportOutput{
		SchemaVersion:    record.SchemaVersion,
		MinReaderVersion: record.MinReaderVersion,
		MinWriterVersion: record.MinWriterVersion,
		UpdatedBy:        record.UpdatedBy,
		CreatedAt:        formatJSONTime(record.CreatedAt),
		UpdatedAt:        formatJSONTime(record.UpdatedAt),
	}
}

func volumeDrainOperationOutputs(records []model.VolumeDrainOperationRecord) []volumeDrainOperationOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]volumeDrainOperationOutput, 0, len(records))
	for _, record := range records {
		out = append(out, volumeDrainOperationOutputFromRecord(record))
	}
	return out
}

func volumeDrainOperationOutputFromRecord(record model.VolumeDrainOperationRecord) volumeDrainOperationOutput {
	return volumeDrainOperationOutput{
		OperationID:         record.OperationID,
		ResumeOfOperationID: record.ResumeOfOperationID,
		PoolID:              record.PoolID,
		SourceVolumeID:      record.SourceVolumeID,
		TargetVolumeID:      record.TargetVolumeID,
		OwnerID:             record.OwnerID,
		Status:              string(record.Status),
		Cursor:              record.Cursor,
		StartedAt:           formatJSONTime(record.StartedAt),
		FinishedAt:          formatJSONTime(record.FinishedAt),
		Scanned:             record.Scanned,
		Copied:              record.Copied,
		Skipped:             record.Skipped,
		Protected:           record.Protected,
		Retryable:           record.Retryable,
		Attempts:            volumeDrainAttemptOutputs(record.Attempts),
		CreatedAt:           formatJSONTime(record.CreatedAt),
	}
}

func volumeDrainAttemptOutputs(attempts []model.VolumeDrainAttempt) []volumeDrainAttemptOutput {
	if len(attempts) == 0 {
		return nil
	}
	out := make([]volumeDrainAttemptOutput, 0, len(attempts))
	for _, attempt := range attempts {
		out = append(out, volumeDrainAttemptOutput{
			BucketID:        attempt.BucketID,
			Key:             attempt.Key,
			VersionID:       attempt.VersionID,
			SourceSegmentID: attempt.SourceSegmentID,
			SourceVolumeID:  segmentRefVolumeID(attempt.SourceRef),
			TargetSegmentID: attempt.TargetSegmentID,
			TargetVolumeID:  segmentRefVolumeID(attempt.TargetRef),
			Status:          string(attempt.Status),
			Protected:       attempt.Protected,
			Retryable:       attempt.Retryable,
			Error:           attempt.Error,
		})
	}
	return out
}

func workerControlOutputFromRecord(record model.WorkerControlRecord) workerControlOutput {
	return workerControlOutput{
		WorkerKind: record.WorkerKind,
		ShardID:    record.ShardID,
		State:      string(meta.NormalizeWorkerControlState(record.State)),
		Reason:     record.Reason,
		UpdatedBy:  record.UpdatedBy,
		UpdatedAt:  formatJSONTime(record.UpdatedAt),
		CreatedAt:  formatJSONTime(record.CreatedAt),
	}
}

func gcCandidateOutputs(records []model.GCCandidateRecord) []gcCandidateOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]gcCandidateOutput, 0, len(records))
	for _, record := range records {
		ref := record.SegmentRef
		out = append(out, gcCandidateOutput{
			SegmentID: ref.SegmentID,
			Reason:    string(record.Reason),
			CreatedAt: formatJSONTime(record.CreatedAt),
			UpdatedAt: formatJSONTime(record.UpdatedAt),
			VolumeID:  segmentRefVolumeID(ref),
			Backend:   ref.Placement.Backend,
			Layout:    ref.Placement.Layout,
			SizeBytes: ref.SizeBytes,
		})
	}
	return out
}

func segmentRefsForObjectVersion(version model.ObjectVersion) []storage.SegmentRef {
	refs := make([]storage.SegmentRef, 0, len(version.SegmentRefs)+1)
	for _, ref := range version.SegmentRefs {
		if ref.SegmentID == "" {
			continue
		}
		refs = append(refs, storage.CloneSegmentRef(ref))
	}
	if len(refs) == 0 && version.SegmentRef.SegmentID != "" {
		refs = append(refs, storage.CloneSegmentRef(version.SegmentRef))
	}
	return refs
}

func segmentRefVolumeID(ref storage.SegmentRef) string {
	if ref.Placement.Parameters != nil {
		if volumeID := strings.TrimSpace(ref.Placement.Parameters["volume_id"]); volumeID != "" {
			return volumeID
		}
	}
	for _, chunk := range ref.Placement.Chunks {
		if strings.TrimSpace(chunk.VolumeID) != "" {
			return strings.TrimSpace(chunk.VolumeID)
		}
	}
	return ""
}

func sharedObjectExportOutputs(records []model.SharedObject) []sharedObjectExportOutput {
	if len(records) == 0 {
		return nil
	}
	out := make([]sharedObjectExportOutput, 0, len(records))
	for _, record := range records {
		out = append(out, sharedObjectExportOutput{
			SharedObjectID:     record.SharedObjectID,
			TenantID:           record.TenantID,
			BucketID:           record.BucketID,
			Key:                record.Key,
			SourceVersionID:    record.SourceVersionID,
			SizeBytes:          record.SizeBytes,
			DigestAlgorithm:    record.Digest.Algorithm,
			DigestHex:          record.Digest.Hex,
			RefCount:           record.RefCount,
			ProtectedRootCount: record.ProtectedRootCount,
			CreatedAt:          formatJSONTime(record.CreatedAt),
			UpdatedAt:          formatJSONTime(record.UpdatedAt),
		})
	}
	return out
}

func putAdminAuditEvent(ctx context.Context, repo meta.Repository, action model.AuditAction, reason string, details map[string]string) error {
	if repo == nil {
		return fmt.Errorf("metadata repository is required for admin audit")
	}
	_, err := repo.PutAdminAuditEvent(ctx, meta.PutAdminAuditEventRequest{
		Action:  action,
		Details: details,
		Audit: meta.AuditContext{
			Reason: reason,
		},
	})
	return err
}

func metadataExportAuditDetails(output metadataExportOutput) map[string]string {
	return map[string]string{
		"schema_version":                strconv.Itoa(output.SchemaVersion),
		"generated_at":                  output.GeneratedAt,
		"limit":                         strconv.Itoa(output.Limit),
		"metadata_schema":               strconv.Itoa(boolCount(output.MetadataSchema != nil)),
		"metadata_migration_operations": strconv.Itoa(len(output.MetadataMigrationOperations)),
		"kms_keys":                      strconv.Itoa(len(output.KMSKeys)),
		"audit_events":                  strconv.Itoa(len(output.AuditEvents)),
		"gc_operations":                 strconv.Itoa(len(output.GCOperations)),
		"dedupe_operations":             strconv.Itoa(len(output.DedupeOperations)),
		"shared_objects":                strconv.Itoa(len(output.SharedObjects)),
		"shared_object_releases":        strconv.Itoa(len(output.SharedObjectReleases)),
		"volume_pools":                  strconv.Itoa(len(output.VolumePools)),
		"volume_drain_operations":       strconv.Itoa(len(output.VolumeDrainOperations)),
		"worker_leases":                 strconv.Itoa(len(output.WorkerLeases)),
		"worker_operations":             strconv.Itoa(len(output.WorkerOperations)),
	}
}

func metadataImportAuditDetails(output metadataImportDryRunOutput) map[string]string {
	details := map[string]string{
		"schema_version":                strconv.Itoa(output.SchemaVersion),
		"source":                        output.Source,
		"dry_run":                       strconv.FormatBool(output.DryRun),
		"valid":                         strconv.FormatBool(output.Valid),
		"ready_for_apply":               strconv.FormatBool(output.ReadyForApply),
		"target_checked":                strconv.FormatBool(output.TargetChecked),
		"target_empty":                  strconv.FormatBool(output.TargetEmpty),
		"conflict_policy":               output.ConflictPolicy,
		"apply_plan_status":             output.ApplyPlan.Status,
		"apply_write_enabled":           strconv.FormatBool(output.ApplyPlan.WriteEnabled),
		"apply_supported":               strconv.FormatBool(output.ApplyPlan.ApplySupported),
		"conflicts":                     strconv.Itoa(len(output.Conflicts)),
		"metadata_schema":               strconv.Itoa(output.Counts.MetadataSchema),
		"metadata_migration_operations": strconv.Itoa(output.Counts.MetadataMigrationOperations),
		"kms_keys":                      strconv.Itoa(output.Counts.KMSKeys),
		"audit_events":                  strconv.Itoa(output.Counts.AuditEvents),
		"gc_operations":                 strconv.Itoa(output.Counts.GCOperations),
		"dedupe_operations":             strconv.Itoa(output.Counts.DedupeOperations),
		"shared_objects":                strconv.Itoa(output.Counts.SharedObjects),
		"shared_object_releases":        strconv.Itoa(output.Counts.SharedObjectReleases),
		"volume_pools":                  strconv.Itoa(output.Counts.VolumePools),
		"volume_drain_operations":       strconv.Itoa(output.Counts.VolumeDrainOperations),
		"worker_leases":                 strconv.Itoa(output.Counts.WorkerLeases),
		"worker_operations":             strconv.Itoa(output.Counts.WorkerOperations),
	}
	if output.ApplyRequested {
		details["apply_requested"] = "true"
	}
	if output.ApplyResult != nil {
		details["apply_result_status"] = output.ApplyResult.Status
		details["apply_records_planned"] = strconv.Itoa(output.ApplyResult.RecordsPlanned)
		details["apply_records_written"] = strconv.Itoa(output.ApplyResult.RecordsWritten)
	}
	return details
}

func metadataListIndexRepairNeeded(result model.ListIndexRepairResult) bool {
	return result.MissingObjectListEntries > 0 ||
		result.StaleObjectListEntries > 0 ||
		result.MissingMultipartUploadIndexes > 0 ||
		result.StaleMultipartUploadIndexes > 0
}

func metadataListIndexRepairStatus(result model.ListIndexRepairResult, apply bool) string {
	if !metadataListIndexRepairNeeded(result) {
		return "clean"
	}
	if apply {
		return "repaired"
	}
	return "repair_available"
}

func metadataListIndexRepairAuditDetails(output metadataListIndexRepairOutput) map[string]string {
	result := output.Result
	return map[string]string{
		"schema_version":                    output.SchemaVersion,
		"generated_at":                      output.GeneratedAt,
		"bucket_id":                         output.BucketID,
		"bucket":                            output.Bucket,
		"limit":                             strconv.Itoa(output.Limit),
		"dry_run":                           strconv.FormatBool(output.DryRun),
		"apply":                             strconv.FormatBool(output.Apply),
		"status":                            output.Status,
		"repair_needed":                     strconv.FormatBool(output.RepairNeeded),
		"scanned_object_heads":              strconv.Itoa(result.ScannedObjectHeads),
		"scanned_object_list_entries":       strconv.Itoa(result.ScannedObjectListEntries),
		"missing_object_list_entries":       strconv.Itoa(result.MissingObjectListEntries),
		"stale_object_list_entries":         strconv.Itoa(result.StaleObjectListEntries),
		"repaired_object_list_entries":      strconv.Itoa(result.RepairedObjectListEntries),
		"removed_object_list_entries":       strconv.Itoa(result.RemovedObjectListEntries),
		"scanned_multipart_uploads":         strconv.Itoa(result.ScannedMultipartUploads),
		"scanned_multipart_upload_indexes":  strconv.Itoa(result.ScannedMultipartUploadIndexes),
		"missing_multipart_upload_indexes":  strconv.Itoa(result.MissingMultipartUploadIndexes),
		"stale_multipart_upload_indexes":    strconv.Itoa(result.StaleMultipartUploadIndexes),
		"repaired_multipart_upload_indexes": strconv.Itoa(result.RepairedMultipartUploadIndexes),
		"removed_multipart_upload_indexes":  strconv.Itoa(result.RemovedMultipartUploadIndexes),
	}
}

func metadataMigrationSchemaStep(posture meta.MetadataSchemaPosture, targetSchemaVersion int, apply bool) model.MetadataMigrationStep {
	status := model.MetadataMigrationStepSucceeded
	repairNeeded := false
	message := posture.Reason
	if posture.UnsupportedFuture {
		status = model.MetadataMigrationStepFailed
	} else if posture.MigrationRequired {
		if apply {
			status = model.MetadataMigrationStepSucceeded
			message = "schema marker will be updated after repair steps"
		} else {
			status = model.MetadataMigrationStepRepairNeeded
			repairNeeded = true
		}
	}
	if message == "" {
		message = fmt.Sprintf("target schema version %d", targetSchemaVersion)
	}
	return model.MetadataMigrationStep{
		Name:           "schema_posture",
		Status:         status,
		Message:        message,
		RepairNeeded:   repairNeeded,
		RecordsScanned: 1,
	}
}

func metadataMigrationListIndexStep(result model.ListIndexRepairResult, apply bool) model.MetadataMigrationStep {
	repairNeeded := metadataListIndexRepairNeeded(result)
	status := model.MetadataMigrationStepSucceeded
	message := "list indexes are consistent"
	if repairNeeded && !apply {
		status = model.MetadataMigrationStepRepairNeeded
		message = "list index repair is available"
	} else if repairNeeded {
		message = "list index repairs applied"
	}
	return model.MetadataMigrationStep{
		Name:            "list_index_repair",
		Status:          status,
		Message:         message,
		RepairNeeded:    repairNeeded && !apply,
		RecordsScanned:  metadataMigrationListIndexRecordsScanned(result),
		RecordsRepaired: metadataMigrationListIndexRecordsRepaired(result),
	}
}

func metadataMigrationListIndexRecordsScanned(result model.ListIndexRepairResult) int {
	return result.ScannedObjectHeads +
		result.ScannedObjectListEntries +
		result.ScannedMultipartUploads +
		result.ScannedMultipartUploadIndexes
}

func metadataMigrationListIndexRecordsRepaired(result model.ListIndexRepairResult) int {
	return result.RepairedObjectListEntries +
		result.RemovedObjectListEntries +
		result.RepairedMultipartUploadIndexes +
		result.RemovedMultipartUploadIndexes
}

func metadataMigrationStatusForSteps(steps []model.MetadataMigrationStep, apply bool, runErr error) model.MetadataMigrationOperationStatus {
	if runErr != nil {
		return model.MetadataMigrationOperationFailed
	}
	for _, step := range steps {
		if step.Status == model.MetadataMigrationStepFailed {
			return model.MetadataMigrationOperationFailed
		}
	}
	if !apply {
		return model.MetadataMigrationOperationPlanned
	}
	for _, step := range steps {
		if step.Status == model.MetadataMigrationStepRepairNeeded {
			return model.MetadataMigrationOperationRetryPending
		}
	}
	return model.MetadataMigrationOperationSucceeded
}

func metadataMigrationAuditDetails(output metadataMigrationOutput) map[string]string {
	details := map[string]string{
		"schema_version":        output.SchemaVersion,
		"generated_at":          output.GeneratedAt,
		"action":                output.Action,
		"target_schema_version": strconv.Itoa(output.TargetSchemaVersion),
		"bucket_id":             output.BucketID,
		"bucket":                output.Bucket,
		"limit":                 strconv.Itoa(output.Limit),
		"dry_run":               strconv.FormatBool(output.DryRun),
		"apply":                 strconv.FormatBool(output.Apply),
		"status":                output.Status,
	}
	if output.ResumeOfOperationID != "" {
		details["resume_of_operation_id"] = output.ResumeOfOperationID
	}
	if output.Operation != nil {
		details["operation_id"] = output.Operation.OperationID
	}
	return details
}

func parseMetadataMigrationOperationStatus(raw string) (model.MetadataMigrationOperationStatus, error) {
	status := model.MetadataMigrationOperationStatus(strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, "-", "_"))))
	switch status {
	case "",
		model.MetadataMigrationOperationPlanned,
		model.MetadataMigrationOperationRunning,
		model.MetadataMigrationOperationSucceeded,
		model.MetadataMigrationOperationRetryPending,
		model.MetadataMigrationOperationFailed,
		model.MetadataMigrationOperationCanceled:
		return status, nil
	default:
		return "", fmt.Errorf("unsupported metadata migration operation status %q", raw)
	}
}

func sharedObjectReleaseOperationOutputs(releases []model.SharedObjectRelease) []sharedObjectReleaseOperationOutput {
	if len(releases) == 0 {
		return nil
	}
	out := make([]sharedObjectReleaseOperationOutput, 0, len(releases))
	for _, release := range releases {
		out = append(out, sharedObjectReleaseOperationOutput{
			ReleaseID:      release.ReleaseID,
			SharedObjectID: release.SharedObjectID,
			SegmentID:      release.SegmentID,
			Reason:         string(release.Reason),
			Status:         string(release.Status),
			CreatedAt:      formatJSONTime(release.CreatedAt),
			UpdatedAt:      formatJSONTime(release.UpdatedAt),
		})
	}
	return out
}

func dedupeOperationOutputs(records []model.DedupeOperationRecord) []dedupeOperationOutput {
	out := make([]dedupeOperationOutput, 0, len(records))
	for _, record := range records {
		attempts := make([]dedupeOperationAttemptOutput, 0, len(record.Attempts))
		for _, attempt := range record.Attempts {
			attempts = append(attempts, dedupeOperationAttemptOutput{
				BucketID:         attempt.BucketID,
				Key:              attempt.Key,
				SourceVersion:    attempt.SourceVersion,
				CandidateVersion: attempt.CandidateVersion,
				PlanStatus:       attempt.PlanStatus,
				PlanReason:       attempt.PlanReason,
				Status:           attempt.Status,
				SharedObjectID:   attempt.SharedObjectID,
				OrphansMarked:    attempt.OrphansMarked,
				Retryable:        attempt.Retryable,
				Error:            attempt.Error,
			})
		}
		out = append(out, dedupeOperationOutput{
			OperationID:         record.OperationID,
			ResumeOfOperationID: record.ResumeOfOperationID,
			Status:              record.Status,
			StartedAt:           formatJSONTime(record.StartedAt),
			FinishedAt:          formatJSONTime(record.FinishedAt),
			Scanned:             record.Scanned,
			Acked:               record.Acked,
			Skipped:             record.Skipped,
			Retryable:           record.Retryable,
			Attempts:            attempts,
			CreatedAt:           formatJSONTime(record.CreatedAt),
		})
	}
	return out
}

func parseVolumePoolMemberSpecs(specs []string) ([]model.VolumePoolMember, error) {
	if len(specs) == 0 {
		return nil, fmt.Errorf("at least one volume pool member is required")
	}
	out := make([]model.VolumePoolMember, 0, len(specs))
	for _, spec := range specs {
		member, err := parseVolumePoolMemberSpec(spec)
		if err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, nil
}

func parseVolumeDrainOperationStatus(value string) (model.VolumeDrainOperationStatus, error) {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_")))
	switch model.VolumeDrainOperationStatus(normalized) {
	case "":
		return "", nil
	case model.VolumeDrainOperationRunning,
		model.VolumeDrainOperationSucceeded,
		model.VolumeDrainOperationRetryPending,
		model.VolumeDrainOperationFailed,
		model.VolumeDrainOperationCanceled:
		return model.VolumeDrainOperationStatus(normalized), nil
	default:
		return "", fmt.Errorf("unsupported volume drain operation status %q", value)
	}
}

func parseVolumeDrainAttemptStatus(value string) (model.VolumeDrainAttemptStatus, error) {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_")))
	switch model.VolumeDrainAttemptStatus(normalized) {
	case "":
		return "", nil
	case model.VolumeDrainAttemptCopied,
		model.VolumeDrainAttemptQueuedGC,
		model.VolumeDrainAttemptSkipped,
		model.VolumeDrainAttemptProtected,
		model.VolumeDrainAttemptRetryable,
		model.VolumeDrainAttemptFailed:
		return model.VolumeDrainAttemptStatus(normalized), nil
	default:
		return "", fmt.Errorf("unsupported volume drain attempt status %q", value)
	}
}

func parseVolumeDrainAttemptSpecs(specs []string) ([]model.VolumeDrainAttempt, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	out := make([]model.VolumeDrainAttempt, 0, len(specs))
	for _, spec := range specs {
		attempt, err := parseVolumeDrainAttemptSpec(spec)
		if err != nil {
			return nil, err
		}
		out = append(out, attempt)
	}
	return out, nil
}

func parseVolumeDrainAttemptSpec(spec string) (model.VolumeDrainAttempt, error) {
	var attempt model.VolumeDrainAttempt
	var sourceVolumeID string
	var targetVolumeID string
	for _, field := range strings.Split(spec, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return model.VolumeDrainAttempt{}, fmt.Errorf("invalid volume drain attempt field %q", field)
		}
		key = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
		value = strings.TrimSpace(value)
		switch key {
		case "bucket_id", "bucket":
			attempt.BucketID = value
		case "key", "object_key":
			attempt.Key = value
		case "version_id", "version":
			attempt.VersionID = value
		case "source_segment_id", "source_segment", "source":
			attempt.SourceSegmentID = value
			attempt.SourceRef.SegmentID = value
		case "target_segment_id", "target_segment", "target":
			attempt.TargetSegmentID = value
			attempt.TargetRef.SegmentID = value
		case "source_volume_id", "source_volume":
			sourceVolumeID = value
			attempt.SourceRef.Placement.Backend = "sbs"
			setSegmentRefVolumeID(&attempt.SourceRef, value)
		case "target_volume_id", "target_volume":
			targetVolumeID = value
			attempt.TargetRef.Placement.Backend = "sbs"
			setSegmentRefVolumeID(&attempt.TargetRef, value)
		case "status":
			status, err := parseVolumeDrainAttemptStatus(value)
			if err != nil {
				return model.VolumeDrainAttempt{}, err
			}
			attempt.Status = status
		case "protected":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return model.VolumeDrainAttempt{}, fmt.Errorf("invalid volume drain attempt protected %q: %w", value, err)
			}
			attempt.Protected = parsed
		case "retryable":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return model.VolumeDrainAttempt{}, fmt.Errorf("invalid volume drain attempt retryable %q: %w", value, err)
			}
			attempt.Retryable = parsed
		case "error":
			attempt.Error = value
		default:
			return model.VolumeDrainAttempt{}, fmt.Errorf("unsupported volume drain attempt field %q", key)
		}
	}
	if attempt.SourceSegmentID == "" {
		attempt.SourceSegmentID = attempt.SourceRef.SegmentID
	}
	if attempt.TargetSegmentID == "" {
		attempt.TargetSegmentID = attempt.TargetRef.SegmentID
	}
	if segmentRefVolumeID(attempt.SourceRef) == "" {
		setSegmentRefVolumeID(&attempt.SourceRef, sourceVolumeID)
	}
	if segmentRefVolumeID(attempt.TargetRef) == "" {
		setSegmentRefVolumeID(&attempt.TargetRef, targetVolumeID)
	}
	if attempt.Status == "" {
		switch {
		case attempt.Protected:
			attempt.Status = model.VolumeDrainAttemptProtected
		case attempt.Retryable:
			attempt.Status = model.VolumeDrainAttemptRetryable
		case attempt.TargetSegmentID != "":
			attempt.Status = model.VolumeDrainAttemptCopied
		default:
			attempt.Status = model.VolumeDrainAttemptSkipped
		}
	}
	return attempt, nil
}

func setSegmentRefVolumeID(ref *storage.SegmentRef, volumeID string) {
	volumeID = strings.TrimSpace(volumeID)
	if ref == nil || volumeID == "" {
		return
	}
	if ref.Placement.Parameters == nil {
		ref.Placement.Parameters = make(map[string]string)
	}
	ref.Placement.Parameters["volume_id"] = volumeID
}

func fillVolumeDrainCounters(attempts []model.VolumeDrainAttempt, scanned, copied, skipped, protected, retryable int) (int, int, int, int, int) {
	if len(attempts) == 0 {
		return scanned, copied, skipped, protected, retryable
	}
	if scanned == 0 {
		scanned = len(attempts)
	}
	if copied == 0 {
		for _, attempt := range attempts {
			if attempt.Status == model.VolumeDrainAttemptCopied {
				copied++
			}
		}
	}
	if skipped == 0 {
		for _, attempt := range attempts {
			if attempt.Status == model.VolumeDrainAttemptSkipped || attempt.Status == model.VolumeDrainAttemptProtected {
				skipped++
			}
		}
	}
	if protected == 0 {
		for _, attempt := range attempts {
			if attempt.Protected || attempt.Status == model.VolumeDrainAttemptProtected {
				protected++
			}
		}
	}
	if retryable == 0 {
		for _, attempt := range attempts {
			if attempt.Retryable || attempt.Status == model.VolumeDrainAttemptRetryable {
				retryable++
			}
		}
	}
	return scanned, copied, skipped, protected, retryable
}

func parseVolumePoolMemberSpec(spec string) (model.VolumePoolMember, error) {
	var member model.VolumePoolMember
	for _, field := range strings.Split(spec, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			if member.VolumeID == "" {
				member.VolumeID = field
				continue
			}
			return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member field %q", field)
		}
		key = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
		value = strings.TrimSpace(value)
		switch key {
		case "volume_id", "volume":
			member.VolumeID = value
		case "admin_endpoint", "admin":
			member.AdminEndpoint = value
		case "data_endpoint", "data":
			member.DataEndpoint = value
		case "gateway_id", "gateway":
			member.GatewayID = value
		case "attachment_id", "attachment":
			member.AttachmentID = value
		case "generation":
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member generation %q: %w", value, err)
			}
			member.Generation = parsed
		case "chunk_size_bytes", "chunk_size":
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member chunk_size_bytes %q: %w", value, err)
			}
			member.ChunkSizeBytes = parsed
		case "state":
			member.State = model.VolumePoolState(strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "-", "_"))))
		case "readonly", "read_only":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member readonly %q: %w", value, err)
			}
			member.ReadOnly = parsed
		case "weight":
			parsed, err := strconv.Atoi(value)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member weight %q: %w", value, err)
			}
			member.Weight = parsed
		case "capacity_bytes", "capacity":
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member capacity_bytes %q: %w", value, err)
			}
			member.CapacityBytes = parsed
		case "available_bytes", "available":
			parsed, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member available_bytes %q: %w", value, err)
			}
			member.AvailableBytes = parsed
		case "used_percent", "used":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member used_percent %q: %w", value, err)
			}
			member.UsedPercent = parsed
		case "high_watermark_percent", "high_watermark", "watermark":
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member high_watermark_percent %q: %w", value, err)
			}
			member.HighWatermarkPercent = parsed
		case "last_observed_at", "observed_at":
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return model.VolumePoolMember{}, fmt.Errorf("invalid volume pool member last_observed_at %q: %w", value, err)
			}
			member.LastObservedAt = parsed
		default:
			return model.VolumePoolMember{}, fmt.Errorf("unsupported volume pool member field %q", key)
		}
	}
	return member, nil
}

func volumePoolOutputFromModel(pool model.VolumePool) volumePoolOutput {
	members := make([]volumePoolMemberOutput, 0, len(pool.Members))
	for _, member := range pool.Members {
		members = append(members, volumePoolMemberOutput{
			VolumeID:             member.VolumeID,
			AdminEndpoint:        member.AdminEndpoint,
			DataEndpoint:         member.DataEndpoint,
			GatewayID:            member.GatewayID,
			AttachmentID:         member.AttachmentID,
			Generation:           member.Generation,
			ChunkSizeBytes:       member.ChunkSizeBytes,
			State:                string(member.State),
			ReadOnly:             member.ReadOnly,
			Weight:               member.Weight,
			CapacityBytes:        member.CapacityBytes,
			AvailableBytes:       member.AvailableBytes,
			UsedPercent:          member.UsedPercent,
			HighWatermarkPercent: member.HighWatermarkPercent,
			LastObservedAt:       formatJSONTime(member.LastObservedAt),
		})
	}
	return volumePoolOutput{
		PoolID:          pool.PoolID,
		Generation:      pool.Generation,
		DurabilityClass: pool.DurabilityClass,
		StorageClassIDs: append([]string(nil), pool.StorageClassIDs...),
		Members:         members,
		CreatedAt:       formatJSONTime(pool.CreatedAt),
		UpdatedAt:       formatJSONTime(pool.UpdatedAt),
	}
}

func bucketQuotaOutputFromModel(bucket model.Bucket, quota model.BucketQuota, deleted bool) bucketQuotaOutput {
	configured := !quota.CreatedAt.IsZero() && !deleted
	return bucketQuotaOutput{
		SchemaVersion:      "namros.admin.bucket_quota.v1",
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339Nano),
		BucketID:           bucket.BucketID,
		Bucket:             bucket.Name,
		Configured:         configured,
		Deleted:            deleted,
		MaxObjectSizeBytes: quota.MaxObjectSizeBytes,
		CreatedAt:          formatJSONTime(quota.CreatedAt),
		UpdatedAt:          formatJSONTime(quota.UpdatedAt),
	}
}

func volumePoolOutputs(pools []model.VolumePool) []volumePoolOutput {
	if len(pools) == 0 {
		return nil
	}
	out := make([]volumePoolOutput, 0, len(pools))
	for _, pool := range pools {
		out = append(out, volumePoolOutputFromModel(pool))
	}
	return out
}

func tenantQuotaOutputFromModel(quota model.TenantQuota, deleted bool) tenantQuotaOutput {
	configured := !quota.CreatedAt.IsZero() && !deleted
	return tenantQuotaOutput{
		SchemaVersion:    "namros.admin.tenant_quota.v1",
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339Nano),
		TenantID:         quota.TenantID,
		Configured:       configured,
		Deleted:          deleted,
		MaxBytes:         quota.MaxBytes,
		MaxObjects:       quota.MaxObjects,
		MaxActiveUploads: quota.MaxActiveUploads,
		CreatedAt:        formatJSONTime(quota.CreatedAt),
		UpdatedAt:        formatJSONTime(quota.UpdatedAt),
	}
}

func resolveAdminBucket(ctx context.Context, repo meta.Repository, bucketName, bucketID string) (model.Bucket, error) {
	bucketName = strings.TrimSpace(bucketName)
	bucketID = strings.TrimSpace(bucketID)
	if bucketName == "" && bucketID == "" {
		return model.Bucket{}, fmt.Errorf("bucket or bucket-id is required")
	}
	var byName *model.Bucket
	if bucketName != "" {
		bucket, err := repo.GetBucketByName(ctx, bucketName)
		if err != nil {
			return model.Bucket{}, err
		}
		byName = &bucket
	}
	if bucketID == "" {
		return *byName, nil
	}
	buckets, err := repo.ListBuckets(ctx, "")
	if err != nil {
		return model.Bucket{}, err
	}
	for _, bucket := range buckets {
		if bucket.BucketID != bucketID {
			continue
		}
		if byName != nil && byName.BucketID != bucket.BucketID {
			return model.Bucket{}, fmt.Errorf("bucket %q does not match bucket-id %q", bucketName, bucketID)
		}
		return bucket, nil
	}
	return model.Bucket{}, meta.ErrNotFound
}

func formatJSONTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
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

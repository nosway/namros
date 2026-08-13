package testsuite

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

type RepositoryUnderTest interface {
	meta.Repository
}

func RunRepositoryTests(t *testing.T, newRepository func(t *testing.T) RepositoryUnderTest) {
	t.Helper()
	t.Run("metadata schema record", func(t *testing.T) {
		testMetadataSchemaRecord(t, newRepository(t))
	})
	t.Run("metadata migration operation records", func(t *testing.T) {
		testMetadataMigrationOperationRecords(t, newRepository(t))
	})
	t.Run("bucket lifecycle", func(t *testing.T) {
		testBucketLifecycle(t, newRepository(t))
	})
	t.Run("bucket create object lock enabled", func(t *testing.T) {
		testCreateBucketObjectLockEnabled(t, newRepository(t))
	})
	t.Run("bucket quota records", func(t *testing.T) {
		testBucketQuotaRecords(t, newRepository(t))
	})
	t.Run("tenant quota records", func(t *testing.T) {
		testTenantQuotaRecords(t, newRepository(t))
	})
	t.Run("tenant usage records", func(t *testing.T) {
		testTenantUsageRecords(t, newRepository(t))
	})
	t.Run("tenant active upload quota admission", func(t *testing.T) {
		testTenantActiveUploadQuotaAdmission(t, newRepository(t))
	})
	t.Run("object commit cas conflict", func(t *testing.T) {
		testObjectCommitCASConflict(t, newRepository(t))
	})
	t.Run("object manifest commit", func(t *testing.T) {
		testObjectManifestCommit(t, newRepository(t))
	})
	t.Run("begin put object returns base head manifest", func(t *testing.T) {
		testBeginPutObjectReturnsBaseHeadManifest(t, newRepository(t))
	})
	t.Run("put object version direct publish", func(t *testing.T) {
		testPutObjectVersionDirectPublish(t, newRepository(t))
	})
	t.Run("object manifest scale limits", func(t *testing.T) {
		testObjectManifestScaleLimits(t, newRepository(t))
	})
	t.Run("object tags", func(t *testing.T) {
		testObjectTags(t, newRepository(t))
	})
	t.Run("object head revision", func(t *testing.T) {
		testObjectHeadRevision(t, newRepository(t))
	})
	t.Run("concurrent unrelated object writes", func(t *testing.T) {
		testConcurrentUnrelatedObjectWrites(t, newRepository(t))
	})
	t.Run("object server side encryption metadata", func(t *testing.T) {
		testObjectServerSideEncryptionMetadata(t, newRepository(t))
	})
	t.Run("list prefix delimiter", func(t *testing.T) {
		testListPrefixDelimiter(t, newRepository(t))
	})
	t.Run("tenant and access key", func(t *testing.T) {
		testTenantAndAccessKey(t, newRepository(t))
	})
	t.Run("kms key records", func(t *testing.T) {
		testKMSKeyRecords(t, newRepository(t))
	})
	t.Run("kms key delete admission", func(t *testing.T) {
		testKMSKeyDeleteAdmission(t, newRepository(t))
	})
	t.Run("compliance profile attachments", func(t *testing.T) {
		testComplianceProfileAttachments(t, newRepository(t))
	})
	t.Run("multipart lifecycle", func(t *testing.T) {
		testMultipartLifecycle(t, newRepository(t))
	})
	t.Run("list multipart uploads", func(t *testing.T) {
		testListMultipartUploads(t, newRepository(t))
	})
	t.Run("bucket versioning and delete markers", func(t *testing.T) {
		testBucketVersioningAndDeleteMarkers(t, newRepository(t))
	})
	t.Run("bucket cors lifecycle", func(t *testing.T) {
		testBucketCORSLifecycle(t, newRepository(t))
	})
	t.Run("bucket lifecycle configuration", func(t *testing.T) {
		testBucketLifecycleConfiguration(t, newRepository(t))
	})
	t.Run("bucket policy lifecycle", func(t *testing.T) {
		testBucketPolicyLifecycle(t, newRepository(t))
	})
	t.Run("object lock metadata lifecycle", func(t *testing.T) {
		testObjectLockMetadataLifecycle(t, newRepository(t))
	})
	t.Run("object lock delete enforcement", func(t *testing.T) {
		testObjectLockDeleteEnforcement(t, newRepository(t))
	})
	t.Run("object lock retention legal hold api", func(t *testing.T) {
		testObjectLockRetentionLegalHoldAPI(t, newRepository(t))
	})
	t.Run("object lock audit transition chain", func(t *testing.T) {
		testObjectLockAuditTransitionChain(t, newRepository(t))
	})
	t.Run("admin audit events", func(t *testing.T) {
		testAdminAuditEvents(t, newRepository(t))
	})
	t.Run("operational metadata import", func(t *testing.T) {
		testOperationalMetadataImport(t, newRepository(t))
	})
	t.Run("gc operation records", func(t *testing.T) {
		testGCOperationRecords(t, newRepository(t))
	})
	t.Run("gc candidate records", func(t *testing.T) {
		testGCCandidateRecords(t, newRepository(t))
	})
	t.Run("dedupe operation records", func(t *testing.T) {
		testDedupeOperationRecords(t, newRepository(t))
	})
	t.Run("dedupe operation locks", func(t *testing.T) {
		testDedupeOperationLocks(t, newRepository(t))
	})
	t.Run("volume pool registry", func(t *testing.T) {
		testVolumePoolRegistry(t, newRepository(t))
	})
	t.Run("volume drain operation records", func(t *testing.T) {
		testVolumeDrainOperationRecords(t, newRepository(t))
	})
	t.Run("publish object version refs", func(t *testing.T) {
		testPublishObjectVersionRefs(t, newRepository(t))
	})
	t.Run("worker lease and operation records", func(t *testing.T) {
		testWorkerLeaseAndOperationRecords(t, newRepository(t))
	})
	t.Run("shared object release records", func(t *testing.T) {
		testSharedObjectReleaseRecords(t, newRepository(t))
	})
	t.Run("shared object publish", func(t *testing.T) {
		testSharedObjectPublish(t, newRepository(t))
	})
	t.Run("shared object attach transaction", func(t *testing.T) {
		testSharedObjectAttachTransaction(t, newRepository(t))
	})
	t.Run("shared object protected root accounting", func(t *testing.T) {
		testSharedObjectProtectedRootAccounting(t, newRepository(t))
	})
	t.Run("shared object refcount repair", func(t *testing.T) {
		testSharedObjectRefCountRepair(t, newRepository(t))
	})
}

func testMetadataSchemaRecord(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	record, err := repo.PutMetadataSchema(ctx, meta.PutMetadataSchemaRequest{
		SchemaVersion:    meta.CurrentMetadataSchemaVersion,
		MinReaderVersion: meta.MinimumMetadataReaderVersion,
		MinWriterVersion: meta.MinimumMetadataWriterVersion,
		UpdatedBy:        "repository-suite",
	})
	if err != nil {
		t.Fatalf("PutMetadataSchema() error = %v", err)
	}
	if record.SchemaVersion != meta.CurrentMetadataSchemaVersion || record.UpdatedBy != "repository-suite" || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		t.Fatalf("PutMetadataSchema() record = %+v", record)
	}
	got, err := repo.GetMetadataSchema(ctx)
	if err != nil {
		t.Fatalf("GetMetadataSchema() error = %v", err)
	}
	if got.SchemaVersion != record.SchemaVersion || got.MinReaderVersion != record.MinReaderVersion || got.MinWriterVersion != record.MinWriterVersion {
		t.Fatalf("GetMetadataSchema() = %+v, want %+v", got, record)
	}

	future, err := repo.PutMetadataSchema(ctx, meta.PutMetadataSchemaRequest{
		SchemaVersion:    meta.CurrentMetadataSchemaVersion + 1,
		MinReaderVersion: meta.CurrentMetadataSchemaVersion + 1,
		MinWriterVersion: meta.CurrentMetadataSchemaVersion + 1,
		UpdatedBy:        "future-release",
	})
	if err != nil {
		t.Fatalf("PutMetadataSchema(future) error = %v", err)
	}
	posture := meta.MetadataSchemaPostureForRecord(future)
	if posture.Status != meta.MetadataSchemaStatusUnsupported || !posture.UnsupportedFuture {
		t.Fatalf("future metadata schema posture = %+v", posture)
	}

	if _, err := repo.PutMetadataSchema(ctx, meta.PutMetadataSchemaRequest{SchemaVersion: 0}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutMetadataSchema(invalid) error = %v, want ErrInvalidArgument", err)
	}
}

func testMetadataMigrationOperationRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 11, 2, 15, 0, 0, time.UTC)
	plan, err := repo.PutMetadataMigrationOperation(ctx, meta.PutMetadataMigrationOperationRequest{
		TargetSchemaVersion: meta.CurrentMetadataSchemaVersion,
		DryRun:              true,
		OwnerID:             "namros-admin",
		StartedAt:           startedAt,
		FinishedAt:          startedAt.Add(time.Second),
		Steps: []model.MetadataMigrationStep{
			{
				Name:           "list_index_repair",
				Status:         model.MetadataMigrationStepRepairNeeded,
				Message:        "dry-run detected missing list index entries",
				RepairNeeded:   true,
				RecordsScanned: 5,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutMetadataMigrationOperation(plan) error = %v", err)
	}
	if plan.OperationID == "" || plan.Status != model.MetadataMigrationOperationPlanned || !plan.DryRun || plan.Apply || len(plan.Steps) != 1 {
		t.Fatalf("plan operation = %+v", plan)
	}
	plan.Steps[0].Message = "mutated"

	apply, err := repo.PutMetadataMigrationOperation(ctx, meta.PutMetadataMigrationOperationRequest{
		ResumeOfOperationID: plan.OperationID,
		TargetSchemaVersion: meta.CurrentMetadataSchemaVersion,
		Apply:               true,
		OwnerID:             "namros-admin",
		Cursor:              "bucket-a/logs/a.txt",
		StartedAt:           startedAt.Add(time.Minute),
		FinishedAt:          startedAt.Add(time.Minute + time.Second),
		Steps: []model.MetadataMigrationStep{
			{
				Name:            "list_index_repair",
				Status:          model.MetadataMigrationStepSucceeded,
				Message:         "applied list index repairs",
				RecordsScanned:  5,
				RecordsRepaired: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutMetadataMigrationOperation(apply) error = %v", err)
	}
	if apply.Status != model.MetadataMigrationOperationSucceeded || apply.ResumeOfOperationID != plan.OperationID || !apply.Apply || apply.Cursor == "" {
		t.Fatalf("apply operation = %+v", apply)
	}

	records, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListMetadataMigrationOperations() error = %v", err)
	}
	if len(records) != 2 || records[0].OperationID != apply.OperationID || records[1].OperationID != plan.OperationID {
		t.Fatalf("migration operation order = %+v", records)
	}
	if records[1].Steps[0].Message != "dry-run detected missing list index entries" {
		t.Fatalf("migration steps mutated through returned value: %+v", records[1].Steps)
	}

	filtered, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{
		Status: model.MetadataMigrationOperationPlanned,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListMetadataMigrationOperations(status) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].OperationID != plan.OperationID {
		t.Fatalf("filtered migration operations = %+v", filtered)
	}

	limited, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListMetadataMigrationOperations(limit) error = %v", err)
	}
	if len(limited) != 1 || limited[0].OperationID != apply.OperationID {
		t.Fatalf("limited migration operations = %+v", limited)
	}
}

func testBucketLifecycle(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "photos",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	got, err := repo.GetBucketByName(ctx, "photos")
	if err != nil {
		t.Fatalf("GetBucketByName() error = %v", err)
	}
	if got.BucketID != bucket.BucketID {
		t.Fatalf("bucket id = %q, want %q", got.BucketID, bucket.BucketID)
	}
	if _, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{TenantID: "tenant-1", Name: "photos", Region: "us-east-1"}); !errors.Is(err, meta.ErrAlreadyExists) {
		t.Fatalf("CreateBucket(duplicate) error = %v, want ErrAlreadyExists", err)
	}
	if err := repo.DeleteBucket(ctx, bucket.BucketID); err != nil {
		t.Fatalf("DeleteBucket() error = %v", err)
	}
	if _, err := repo.GetBucketByName(ctx, "photos"); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketByName(deleted) error = %v, want ErrNotFound", err)
	}
}

func testCreateBucketObjectLockEnabled(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID:          "tenant-1",
		Name:              "bucket-object-lock-" + t.Name(),
		Region:            "us-east-1",
		ObjectLockEnabled: true,
	})
	if err != nil {
		t.Fatalf("CreateBucket(ObjectLockEnabled) error = %v", err)
	}
	if !bucket.ObjectLock.Enabled || bucket.VersioningState != model.BucketVersioningEnabled {
		t.Fatalf("created bucket object lock/versioning = %+v/%q", bucket.ObjectLock, bucket.VersioningState)
	}
	got, err := repo.GetBucketObjectLock(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketObjectLock() error = %v", err)
	}
	if !got.Enabled {
		t.Fatalf("GetBucketObjectLock().Enabled = false, want true")
	}
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningSuspended,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutBucketVersioning(Suspended object lock bucket) error = %v, want ErrInvalidArgument", err)
	}
}

func testBucketQuotaRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.GetBucketQuota(ctx, bucket.BucketID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketQuota(missing) error = %v, want ErrNotFound", err)
	}
	quota, err := repo.PutBucketQuota(ctx, meta.BucketQuotaRequest{
		BucketID:           bucket.BucketID,
		MaxObjectSizeBytes: 1024,
	})
	if err != nil {
		t.Fatalf("PutBucketQuota() error = %v", err)
	}
	if quota.BucketID != bucket.BucketID || quota.MaxObjectSizeBytes != 1024 || quota.CreatedAt.IsZero() || quota.UpdatedAt.IsZero() {
		t.Fatalf("quota = %+v", quota)
	}
	updated, err := repo.PutBucketQuota(ctx, meta.BucketQuotaRequest{
		BucketID:           bucket.BucketID,
		MaxObjectSizeBytes: 2048,
	})
	if err != nil {
		t.Fatalf("PutBucketQuota(update) error = %v", err)
	}
	if !updated.CreatedAt.Equal(quota.CreatedAt) || updated.MaxObjectSizeBytes != 2048 || updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("updated quota = %+v original = %+v", updated, quota)
	}
	got, err := repo.GetBucketQuota(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketQuota() error = %v", err)
	}
	if got.MaxObjectSizeBytes != 2048 {
		t.Fatalf("GetBucketQuota() = %+v", got)
	}
	if _, err := repo.PutBucketQuota(ctx, meta.BucketQuotaRequest{
		BucketID:           bucket.BucketID,
		MaxObjectSizeBytes: -1,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutBucketQuota(negative) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := repo.PutBucketQuota(ctx, meta.BucketQuotaRequest{
		BucketID:           "missing-bucket",
		MaxObjectSizeBytes: 1,
	}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("PutBucketQuota(missing bucket) error = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteBucketQuota(ctx, bucket.BucketID); err != nil {
		t.Fatalf("DeleteBucketQuota() error = %v", err)
	}
	if _, err := repo.GetBucketQuota(ctx, bucket.BucketID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketQuota(after delete) error = %v, want ErrNotFound", err)
	}
}

func testTenantQuotaRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	tenant, err := repo.CreateTenant(ctx, meta.CreateTenantRequest{
		TenantID:    "tenant-quota-a",
		DisplayName: "Tenant Quota A",
	})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if _, err := repo.GetTenantQuota(ctx, tenant.TenantID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetTenantQuota(missing) error = %v, want ErrNotFound", err)
	}
	quota, err := repo.PutTenantQuota(ctx, meta.TenantQuotaRequest{
		TenantID:         tenant.TenantID,
		MaxBytes:         1 << 40,
		MaxObjects:       10_000_000,
		MaxActiveUploads: 1024,
	})
	if err != nil {
		t.Fatalf("PutTenantQuota() error = %v", err)
	}
	if quota.TenantID != tenant.TenantID || quota.MaxBytes != 1<<40 || quota.MaxObjects != 10_000_000 || quota.MaxActiveUploads != 1024 || quota.CreatedAt.IsZero() || quota.UpdatedAt.IsZero() {
		t.Fatalf("quota = %+v", quota)
	}
	updated, err := repo.PutTenantQuota(ctx, meta.TenantQuotaRequest{
		TenantID:         tenant.TenantID,
		MaxBytes:         2 << 40,
		MaxObjects:       20_000_000,
		MaxActiveUploads: 2048,
	})
	if err != nil {
		t.Fatalf("PutTenantQuota(update) error = %v", err)
	}
	if !updated.CreatedAt.Equal(quota.CreatedAt) || updated.MaxBytes != 2<<40 || updated.MaxObjects != 20_000_000 || updated.MaxActiveUploads != 2048 || updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("updated quota = %+v original = %+v", updated, quota)
	}
	got, err := repo.GetTenantQuota(ctx, tenant.TenantID)
	if err != nil {
		t.Fatalf("GetTenantQuota() error = %v", err)
	}
	if got.MaxBytes != 2<<40 || got.MaxObjects != 20_000_000 || got.MaxActiveUploads != 2048 {
		t.Fatalf("GetTenantQuota() = %+v", got)
	}
	for name, req := range map[string]meta.TenantQuotaRequest{
		"negative bytes":          {TenantID: tenant.TenantID, MaxBytes: -1},
		"negative objects":        {TenantID: tenant.TenantID, MaxObjects: -1},
		"negative active uploads": {TenantID: tenant.TenantID, MaxActiveUploads: -1},
	} {
		if _, err := repo.PutTenantQuota(ctx, req); !errors.Is(err, meta.ErrInvalidArgument) {
			t.Fatalf("PutTenantQuota(%s) error = %v, want ErrInvalidArgument", name, err)
		}
	}
	if _, err := repo.PutTenantQuota(ctx, meta.TenantQuotaRequest{
		TenantID:         "missing-tenant",
		MaxBytes:         1,
		MaxObjects:       1,
		MaxActiveUploads: 1,
	}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("PutTenantQuota(missing tenant) error = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteTenantQuota(ctx, tenant.TenantID); err != nil {
		t.Fatalf("DeleteTenantQuota() error = %v", err)
	}
	if _, err := repo.GetTenantQuota(ctx, tenant.TenantID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetTenantQuota(after delete) error = %v, want ErrNotFound", err)
	}
}

func testTenantUsageRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	tenant, err := repo.CreateTenant(ctx, meta.CreateTenantRequest{
		TenantID:    "tenant-usage-a",
		DisplayName: "Tenant Usage A",
	})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if _, err := repo.GetTenantUsage(ctx, tenant.TenantID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetTenantUsage(missing) error = %v, want ErrNotFound", err)
	}
	reconciledAt := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	usage, err := repo.PutTenantUsage(ctx, meta.TenantUsageRequest{
		TenantID:         tenant.TenantID,
		ObjectBytes:      4096,
		ObjectCount:      3,
		ActiveUploads:    2,
		ReconciledAt:     reconciledAt,
		ReconciliationID: "reconcile-1",
	})
	if err != nil {
		t.Fatalf("PutTenantUsage() error = %v", err)
	}
	if usage.TenantID != tenant.TenantID || usage.ObjectBytes != 4096 || usage.ObjectCount != 3 || usage.ActiveUploads != 2 || !usage.ReconciledAt.Equal(reconciledAt) || usage.CreatedAt.IsZero() || usage.UpdatedAt.IsZero() {
		t.Fatalf("usage = %+v", usage)
	}
	updated, err := repo.PutTenantUsage(ctx, meta.TenantUsageRequest{
		TenantID:         tenant.TenantID,
		ObjectBytes:      8192,
		ObjectCount:      4,
		ActiveUploads:    1,
		ReconciliationID: "reconcile-2",
	})
	if err != nil {
		t.Fatalf("PutTenantUsage(update) error = %v", err)
	}
	if !updated.CreatedAt.Equal(usage.CreatedAt) || updated.ObjectBytes != 8192 || updated.ObjectCount != 4 || updated.ActiveUploads != 1 || updated.ReconciliationID != "reconcile-2" || updated.UpdatedAt.Before(updated.CreatedAt) {
		t.Fatalf("updated usage = %+v original = %+v", updated, usage)
	}
	got, err := repo.GetTenantUsage(ctx, tenant.TenantID)
	if err != nil {
		t.Fatalf("GetTenantUsage() error = %v", err)
	}
	if got.ObjectBytes != 8192 || got.ObjectCount != 4 || got.ActiveUploads != 1 {
		t.Fatalf("GetTenantUsage() = %+v", got)
	}
	for name, req := range map[string]meta.TenantUsageRequest{
		"negative bytes":          {TenantID: tenant.TenantID, ObjectBytes: -1},
		"negative object count":   {TenantID: tenant.TenantID, ObjectCount: -1},
		"negative active uploads": {TenantID: tenant.TenantID, ActiveUploads: -1},
	} {
		if _, err := repo.PutTenantUsage(ctx, req); !errors.Is(err, meta.ErrInvalidArgument) {
			t.Fatalf("PutTenantUsage(%s) error = %v, want ErrInvalidArgument", name, err)
		}
	}
	if _, err := repo.PutTenantUsage(ctx, meta.TenantUsageRequest{
		TenantID:      "missing-tenant",
		ObjectBytes:   1,
		ObjectCount:   1,
		ActiveUploads: 1,
	}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("PutTenantUsage(missing tenant) error = %v, want ErrNotFound", err)
	}
}

func testTenantActiveUploadQuotaAdmission(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	tenant, err := repo.CreateTenant(ctx, meta.CreateTenantRequest{
		TenantID:    "tenant-active-upload-quota-a",
		DisplayName: "Tenant Active Upload Quota A",
	})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if _, err := repo.PutTenantQuota(ctx, meta.TenantQuotaRequest{
		TenantID:         tenant.TenantID,
		MaxActiveUploads: 2,
	}); err != nil {
		t.Fatalf("PutTenantQuota() error = %v", err)
	}
	firstBucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: tenant.TenantID,
		Name:     "active-upload-quota-a",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(first) error = %v", err)
	}
	secondBucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: tenant.TenantID,
		Name:     "active-upload-quota-b",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(second) error = %v", err)
	}
	first, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: firstBucket.BucketID,
		Key:      "first.bin",
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(first) error = %v", err)
	}
	second, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: secondBucket.BucketID,
		Key:      "second.bin",
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(second) error = %v", err)
	}
	usage, err := repo.GetTenantUsage(ctx, tenant.TenantID)
	if err != nil {
		t.Fatalf("GetTenantUsage(after create) error = %v", err)
	}
	if usage.ActiveUploads != 2 {
		t.Fatalf("ActiveUploads after create = %d, want 2", usage.ActiveUploads)
	}
	if _, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: firstBucket.BucketID,
		Key:      "third-denied.bin",
	}); !errors.Is(err, meta.ErrQuotaExceeded) {
		t.Fatalf("CreateMultipartUpload(over quota) error = %v, want ErrQuotaExceeded", err)
	}
	usage, err = repo.GetTenantUsage(ctx, tenant.TenantID)
	if err != nil {
		t.Fatalf("GetTenantUsage(after deny) error = %v", err)
	}
	if usage.ActiveUploads != 2 {
		t.Fatalf("ActiveUploads after deny = %d, want 2", usage.ActiveUploads)
	}
	if _, err := repo.AbortMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: first.BucketID,
		Key:      first.Key,
		UploadID: first.UploadID,
	}); err != nil {
		t.Fatalf("AbortMultipartUpload() error = %v", err)
	}
	usage, err = repo.GetTenantUsage(ctx, tenant.TenantID)
	if err != nil {
		t.Fatalf("GetTenantUsage(after abort) error = %v", err)
	}
	if usage.ActiveUploads != 1 {
		t.Fatalf("ActiveUploads after abort = %d, want 1", usage.ActiveUploads)
	}
	if _, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: firstBucket.BucketID,
		Key:      "third-allowed.bin",
	}); err != nil {
		t.Fatalf("CreateMultipartUpload(after abort) error = %v", err)
	}
	if _, err := repo.CompleteMultipartUpload(ctx, meta.CompleteMultipartUploadRequest{
		BucketID:        second.BucketID,
		Key:             second.Key,
		UploadID:        second.UploadID,
		ObjectVersionID: "version-completed",
		ETag:            `"completed"`,
		PartCount:       1,
	}); err != nil {
		t.Fatalf("CompleteMultipartUpload() error = %v", err)
	}
	usage, err = repo.GetTenantUsage(ctx, tenant.TenantID)
	if err != nil {
		t.Fatalf("GetTenantUsage(after complete) error = %v", err)
	}
	if usage.ActiveUploads != 1 {
		t.Fatalf("ActiveUploads after complete = %d, want 1", usage.ActiveUploads)
	}
}

func testObjectCommitCASConflict(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	first, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "dir/object",
		SizeBytes: 11,
		ETag:      `"etag-1"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(first) error = %v", err)
	}
	second, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "dir/object",
		SizeBytes: 12,
		ETag:      `"etag-2"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(second) error = %v", err)
	}
	head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   first.Version.Key,
		VersionID:             first.Version.VersionID,
		ExpectedHeadVersionID: first.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion(first) error = %v", err)
	}
	if head.VersionID != first.Version.VersionID {
		t.Fatalf("head version = %q, want %q", head.VersionID, first.Version.VersionID)
	}
	_, err = repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   second.Version.Key,
		VersionID:             second.Version.VersionID,
		ExpectedHeadVersionID: second.BaseHeadVersionID,
	})
	if !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("CommitObjectVersion(second) error = %v, want ErrCASConflict", err)
	}
	if err := repo.DeleteBucket(ctx, bucket.BucketID); !errors.Is(err, meta.ErrBucketNotEmpty) {
		t.Fatalf("DeleteBucket(non-empty) error = %v, want ErrBucketNotEmpty", err)
	}
}

func testBeginPutObjectReturnsBaseHeadManifest(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	first, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "dir/object",
		SizeBytes: 12,
		ETag:      `"etag-1"`,
		SegmentRefs: []storage.SegmentRef{
			testSegmentRef("segment-base-a", 6),
			testSegmentRef("segment-base-b", 6),
		},
	})
	if err != nil {
		t.Fatalf("BeginPutObject(first) error = %v", err)
	}
	if first.BaseHeadFound || first.BaseHeadVersionID != "" {
		t.Fatalf("first base head = found %v version %q, want empty", first.BaseHeadFound, first.BaseHeadVersionID)
	}
	if _, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "dir/object",
		VersionID:             first.Version.VersionID,
		ExpectedHeadVersionID: first.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion(first) error = %v", err)
	}
	second, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:   bucket.BucketID,
		Key:        "dir/object",
		SizeBytes:  7,
		ETag:       `"etag-2"`,
		SegmentRef: testSegmentRef("segment-next", 7),
	})
	if err != nil {
		t.Fatalf("BeginPutObject(second) error = %v", err)
	}
	if !second.BaseHeadFound || second.BaseHeadVersionID != first.Version.VersionID {
		t.Fatalf("second base head = found %v version %q, want %q", second.BaseHeadFound, second.BaseHeadVersionID, first.Version.VersionID)
	}
	if len(second.BaseHead.SegmentRefs) != 2 || second.BaseHead.SegmentRefs[0].SegmentID != "segment-base-a" {
		t.Fatalf("second base head refs = %+v", second.BaseHead.SegmentRefs)
	}
	second.BaseHead.SegmentRefs[0].SegmentID = "mutated"
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "dir/object")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if len(head.SegmentRefs) != 2 || head.SegmentRefs[0].SegmentID != "segment-base-a" {
		t.Fatalf("stored head refs mutated through pending base head: %+v", head.SegmentRefs)
	}
}

func testPutObjectVersionDirectPublish(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	first, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:  bucket.BucketID,
		Key:       "dir/direct.txt",
		SizeBytes: 12,
		ETag:      `"etag-1"`,
		SegmentRefs: []storage.SegmentRef{
			testSegmentRef("segment-direct-a", 6),
			testSegmentRef("segment-direct-b", 6),
		},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(first) error = %v", err)
	}
	if first.ReplacedHeadFound {
		t.Fatalf("first replaced head found = true, want false")
	}
	if first.Head.VersionID == "" || first.Head.SizeBytes != 12 || len(first.Head.SegmentRefs) != 2 {
		t.Fatalf("first head = %+v", first.Head)
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "dir/direct.txt")
	if err != nil {
		t.Fatalf("GetObjectHead(first) error = %v", err)
	}
	if head.VersionID != first.Head.VersionID || head.SegmentRefs[1].SegmentID != "segment-direct-b" {
		t.Fatalf("stored head = %+v, want direct first head %+v", head, first.Head)
	}
	second, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:    bucket.BucketID,
		Key:         "dir/direct.txt",
		SizeBytes:   7,
		ETag:        `"etag-2"`,
		SegmentRefs: []storage.SegmentRef{testSegmentRef("segment-direct-c", 7)},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(second) error = %v", err)
	}
	if !second.ReplacedHeadFound || second.ReplacedHead.VersionID != first.Head.VersionID {
		t.Fatalf("second replaced head = found %v %+v, want first %q", second.ReplacedHeadFound, second.ReplacedHead, first.Head.VersionID)
	}
	if _, err := repo.GetObjectVersion(ctx, bucket.BucketID, "dir/direct.txt", first.Head.VersionID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetObjectVersion(first after non-versioned overwrite) error = %v, want ErrNotFound", err)
	}
	gotSecond, err := repo.GetObjectVersion(ctx, bucket.BucketID, "dir/direct.txt", second.Head.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(second) error = %v", err)
	}
	if gotSecond.State != model.ObjectVersionCommitted || gotSecond.CommittedAt.IsZero() {
		t.Fatalf("second version state/timestamp = %+v", gotSecond)
	}

	versionedBucket := mustCreateBucketNamed(t, repo, "direct-versioned")
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: versionedBucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	versionedFirst, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:    versionedBucket.BucketID,
		Key:         "keep-history.txt",
		SizeBytes:   5,
		ETag:        `"etag-v1"`,
		SegmentRefs: []storage.SegmentRef{testSegmentRef("segment-history-a", 5)},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(versioned first) error = %v", err)
	}
	versionedSecond, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:    versionedBucket.BucketID,
		Key:         "keep-history.txt",
		SizeBytes:   6,
		ETag:        `"etag-v2"`,
		SegmentRefs: []storage.SegmentRef{testSegmentRef("segment-history-b", 6)},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(versioned second) error = %v", err)
	}
	if !versionedSecond.ReplacedHeadFound || versionedSecond.ReplacedHead.VersionID != versionedFirst.Head.VersionID {
		t.Fatalf("versioned replaced head = found %v %+v", versionedSecond.ReplacedHeadFound, versionedSecond.ReplacedHead)
	}
	if _, err := repo.GetObjectVersion(ctx, versionedBucket.BucketID, "keep-history.txt", versionedFirst.Head.VersionID); err != nil {
		t.Fatalf("GetObjectVersion(versioned first) error = %v", err)
	}
}

func testObjectServerSideEncryptionMetadata(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	encryption := model.ServerSideEncryption{
		Algorithm:  model.ServerSideEncryptionAWSKMS,
		KeyID:      "arn:aws:kms:us-east-1:111122223333:key/namros-test",
		KeyVersion: "kv-1",
	}
	pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:             bucket.BucketID,
		Key:                  "encrypted/object.txt",
		SizeBytes:            11,
		ETag:                 `"etag-encrypted"`,
		ServerSideEncryption: encryption,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "encrypted/object.txt",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if head.ServerSideEncryption != encryption {
		t.Fatalf("head server-side encryption = %+v, want %+v", head.ServerSideEncryption, encryption)
	}
	version, err := repo.GetObjectVersion(ctx, bucket.BucketID, "encrypted/object.txt", head.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion() error = %v", err)
	}
	if version.ServerSideEncryption != encryption {
		t.Fatalf("version server-side encryption = %+v, want %+v", version.ServerSideEncryption, encryption)
	}

	upload, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "encrypted/mpu.bin",
		ServerSideEncryption: model.ServerSideEncryption{
			Algorithm: model.ServerSideEncryptionAES256,
		},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	if upload.ServerSideEncryption.Algorithm != model.ServerSideEncryptionAES256 {
		t.Fatalf("multipart server-side encryption = %+v", upload.ServerSideEncryption)
	}
}

func testObjectManifestCommit(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	refs := []storage.SegmentRef{
		testSegmentRef("segment-1", 6),
		testSegmentRef("segment-2", 9),
	}
	pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:    bucket.BucketID,
		Key:         "multipart-object",
		SizeBytes:   15,
		ETag:        `"multipart-etag-2"`,
		SegmentRefs: refs,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	refs[0].Placement.Parameters["profile_id"] = "mutated"
	refs[0].Placement.Chunks[0].ChunkID = 999
	head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "multipart-object",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if len(head.SegmentRefs) != 2 || head.SegmentRefs[0].SegmentID != "segment-1" || head.SegmentRefs[1].SegmentID != "segment-2" {
		t.Fatalf("head segment refs = %+v", head.SegmentRefs)
	}
	if head.SegmentRef.SegmentID != "segment-1" {
		t.Fatalf("head legacy segment ref = %+v, want first segment", head.SegmentRef)
	}
	if got := head.SegmentRefs[0].Placement.Parameters["profile_id"]; got != "STANDARD" {
		t.Fatalf("head placement profile_id = %q, want STANDARD", got)
	}
	if got := head.SegmentRefs[0].Placement.Chunks[0].ChunkID; got != 6 {
		t.Fatalf("head placement chunk id = %d, want 6", got)
	}
	version, err := repo.GetObjectVersion(ctx, bucket.BucketID, "multipart-object", pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion() error = %v", err)
	}
	if len(version.SegmentRefs) != 2 || version.SegmentRefs[1].SegmentID != "segment-2" {
		t.Fatalf("version segment refs = %+v", version.SegmentRefs)
	}
	if got := version.SegmentRefs[0].Placement.Chunks[0].ChunkID; got != 6 {
		t.Fatalf("version placement chunk id = %d, want 6", got)
	}
}

func testObjectManifestScaleLimits(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	refs := make([]storage.SegmentRef, meta.MaxObjectManifestSegmentRefs+1)
	for i := range refs {
		refs[i] = testSegmentRef(fmt.Sprintf("segment-%05d", i), 1)
	}
	if _, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:    bucket.BucketID,
		Key:         "too-many-segments",
		SizeBytes:   int64(len(refs)),
		ETag:        `"too-many-segments"`,
		SegmentRefs: refs,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("BeginPutObject(too many segment refs) error = %v, want ErrInvalidArgument", err)
	}
	oversizedValueRef := testSegmentRef("segment-oversized-value", 1)
	oversizedValueRef.Placement.Parameters["padding"] = strings.Repeat("x", meta.MaxObjectManifestValueBytes)
	if _, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:   bucket.BucketID,
		Key:        "oversized-manifest-value",
		SizeBytes:  int64(oversizedValueRef.SizeBytes),
		ETag:       `"oversized-manifest-value"`,
		SegmentRef: oversizedValueRef,
	}); !errors.Is(err, meta.ErrObjectManifestTooLarge) || !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("BeginPutObject(oversized manifest value) error = %v, want ErrObjectManifestTooLarge and ErrInvalidArgument", err)
	}
	if _, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:   bucket.BucketID,
		Key:        "oversized-direct-value",
		SizeBytes:  int64(oversizedValueRef.SizeBytes),
		ETag:       `"oversized-direct-value"`,
		SegmentRef: oversizedValueRef,
	}); !errors.Is(err, meta.ErrObjectManifestTooLarge) || !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutObjectVersion(oversized manifest value) error = %v, want ErrObjectManifestTooLarge and ErrInvalidArgument", err)
	}
}

func testObjectTags(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "tagged-object",
		SizeBytes: 6,
		ETag:      `"tagged"`,
		Tags: map[string]string{
			"color": "blue",
			"shape": "square",
		},
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	if _, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "tagged-object",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	tags, err := repo.GetObjectTags(ctx, meta.ObjectTagsRequest{
		BucketID: bucket.BucketID,
		Key:      "tagged-object",
	})
	if err != nil {
		t.Fatalf("GetObjectTags() error = %v", err)
	}
	if !reflect.DeepEqual(tags, map[string]string{"color": "blue", "shape": "square"}) {
		t.Fatalf("initial tags = %#v", tags)
	}
	if err := repo.PutObjectTags(ctx, meta.ObjectTagsRequest{
		BucketID: bucket.BucketID,
		Key:      "tagged-object",
		Tags: map[string]string{
			"color": "green",
		},
	}); err != nil {
		t.Fatalf("PutObjectTags() error = %v", err)
	}
	tags, err = repo.GetObjectTags(ctx, meta.ObjectTagsRequest{
		BucketID: bucket.BucketID,
		Key:      "tagged-object",
	})
	if err != nil {
		t.Fatalf("GetObjectTags(after put) error = %v", err)
	}
	if !reflect.DeepEqual(tags, map[string]string{"color": "green"}) {
		t.Fatalf("updated tags = %#v", tags)
	}
	version, err := repo.GetObjectVersion(ctx, bucket.BucketID, "tagged-object", pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion() error = %v", err)
	}
	if !reflect.DeepEqual(version.Tags, map[string]string{"color": "green"}) {
		t.Fatalf("version tags = %#v, want updated tags", version.Tags)
	}
	if err := repo.DeleteObjectTags(ctx, meta.ObjectTagsRequest{
		BucketID: bucket.BucketID,
		Key:      "tagged-object",
	}); err != nil {
		t.Fatalf("DeleteObjectTags() error = %v", err)
	}
	tags, err = repo.GetObjectTags(ctx, meta.ObjectTagsRequest{
		BucketID: bucket.BucketID,
		Key:      "tagged-object",
	})
	if err != nil {
		t.Fatalf("GetObjectTags(after delete) error = %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("deleted tags = %#v, want none", tags)
	}
}

func testObjectHeadRevision(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "revision.txt",
		ETag:     `"rev-1"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "revision.txt",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if head.Revision != 1 {
		t.Fatalf("committed head revision = %d, want 1", head.Revision)
	}
	if err := repo.PutObjectTags(ctx, meta.ObjectTagsRequest{
		BucketID: bucket.BucketID,
		Key:      "revision.txt",
		Tags:     map[string]string{"step": "tagged"},
	}); err != nil {
		t.Fatalf("PutObjectTags() error = %v", err)
	}
	tagged, err := repo.GetObjectHead(ctx, bucket.BucketID, "revision.txt")
	if err != nil {
		t.Fatalf("GetObjectHead(tagged) error = %v", err)
	}
	if tagged.Revision != 2 {
		t.Fatalf("tagged head revision = %d, want 2", tagged.Revision)
	}
	overwritten, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID: bucket.BucketID,
		Key:      "revision.txt",
		ETag:     `"rev-2"`,
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}
	if overwritten.ReplacedHead.Revision != 2 || overwritten.Head.Revision != 3 {
		t.Fatalf("overwrite revisions = replaced %d head %d, want 2/3", overwritten.ReplacedHead.Revision, overwritten.Head.Revision)
	}
	nextPending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "revision.txt",
		ETag:     `"rev-3"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(next) error = %v", err)
	}
	if !nextPending.BaseHeadFound || nextPending.BaseHead.Revision != 3 {
		t.Fatalf("base head = found %v revision %d, want found revision 3", nextPending.BaseHeadFound, nextPending.BaseHead.Revision)
	}
}

func testConcurrentUnrelatedObjectWrites(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	const writers = 12

	errs := make(chan error, writers)
	versions := make(chan string, writers)
	var wg sync.WaitGroup
	for i := range writers {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			key := fmt.Sprintf("concurrent/%02d.txt", i)
			pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
				BucketID: bucket.BucketID,
				Key:      key,
				ETag:     fmt.Sprintf(`"etag-%02d"`, i),
			})
			if err != nil {
				errs <- fmt.Errorf("BeginPutObject(%s): %w", key, err)
				return
			}
			head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
				BucketID:              bucket.BucketID,
				Key:                   key,
				VersionID:             pending.Version.VersionID,
				ExpectedHeadVersionID: pending.BaseHeadVersionID,
			})
			if err != nil {
				errs <- fmt.Errorf("CommitObjectVersion(%s): %w", key, err)
				return
			}
			if head.VersionID == "" || head.Revision != 1 {
				errs <- fmt.Errorf("head(%s) = %+v, want version id and revision 1", key, head)
				return
			}
			versions <- head.VersionID
		}()
	}
	wg.Wait()
	close(errs)
	close(versions)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[string]struct{}, writers)
	for versionID := range versions {
		if _, ok := seen[versionID]; ok {
			t.Fatalf("duplicate version id %q from concurrent unrelated writes", versionID)
		}
		seen[versionID] = struct{}{}
	}
	if len(seen) != writers {
		t.Fatalf("version count = %d, want %d", len(seen), writers)
	}
	list, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "concurrent/",
		MaxKeys:  writers + 1,
	})
	if err != nil {
		t.Fatalf("ListObjects(concurrent) error = %v", err)
	}
	if len(list.Contents) != writers {
		t.Fatalf("listed concurrent objects = %d, want %d: %+v", len(list.Contents), writers, list.Contents)
	}
}

func testListPrefixDelimiter(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	for _, key := range []string{
		"a/1.txt",
		"a/b/2.txt",
		"a/b/3.txt",
		"a/c/4.txt",
		"a/z.txt",
		"outside.txt",
	} {
		putObject(t, repo, bucket.BucketID, key)
	}
	result, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
		BucketID:  bucket.BucketID,
		Prefix:    "a/",
		Delimiter: "/",
		MaxKeys:   10,
	})
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	gotKeys := objectKeys(result.Contents)
	wantKeys := []string{"a/1.txt", "a/z.txt"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("contents = %#v, want %#v", gotKeys, wantKeys)
	}
	wantPrefixes := []string{"a/b/", "a/c/"}
	if !reflect.DeepEqual(result.CommonPrefixes, wantPrefixes) {
		t.Fatalf("common prefixes = %#v, want %#v", result.CommonPrefixes, wantPrefixes)
	}

	page, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "a/",
		MaxKeys:  2,
	})
	if err != nil {
		t.Fatalf("ListObjects(page1) error = %v", err)
	}
	if !page.IsTruncated || page.NextContinuationToken == "" {
		t.Fatalf("page1 truncation = %v token %q", page.IsTruncated, page.NextContinuationToken)
	}
	next, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
		BucketID:          bucket.BucketID,
		Prefix:            "a/",
		ContinuationToken: page.NextContinuationToken,
		MaxKeys:           10,
	})
	if err != nil {
		t.Fatalf("ListObjects(page2) error = %v", err)
	}
	if len(next.Contents) != 3 {
		t.Fatalf("page2 content len = %d, want 3", len(next.Contents))
	}
}

func testTenantAndAccessKey(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	tenant, err := repo.CreateTenant(ctx, meta.CreateTenantRequest{
		TenantID:    "tenant-1",
		DisplayName: "Tenant One",
	})
	if err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if tenant.TenantID != "tenant-1" {
		t.Fatalf("tenant id = %q", tenant.TenantID)
	}
	accessKey, err := repo.PutAccessKey(ctx, meta.PutAccessKeyRequest{
		TenantID:    "tenant-1",
		AccessKeyID: "ak-1",
		SecretHash:  "hash",
		Status:      model.AccessKeyActive,
		Permissions: []string{"s3:GetObject", "s3:BypassGovernanceRetention"},
	})
	if err != nil {
		t.Fatalf("PutAccessKey() error = %v", err)
	}
	got, err := repo.GetAccessKey(ctx, "ak-1")
	if err != nil {
		t.Fatalf("GetAccessKey() error = %v", err)
	}
	if got.AccessKeyID != accessKey.AccessKeyID || got.Status != model.AccessKeyActive {
		t.Fatalf("access key = %+v", got)
	}
	if !reflect.DeepEqual(got.Permissions, []string{"s3:GetObject", "s3:BypassGovernanceRetention"}) {
		t.Fatalf("access key permissions = %+v", got.Permissions)
	}
}

func testKMSKeyRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	if _, err := repo.GetKMSKey(ctx, "kms-key-1"); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetKMSKey(missing) error = %v, want ErrNotFound", err)
	}
	created, err := repo.PutKMSKey(ctx, meta.PutKMSKeyRequest{
		KeyID:      "kms-key-1",
		KeyVersion: "v1",
	})
	if err != nil {
		t.Fatalf("PutKMSKey(create) error = %v", err)
	}
	if created.KeyID != "kms-key-1" || created.KeyVersion != "v1" || created.State != model.KMSKeyActive || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("created KMS key = %+v", created)
	}
	updated, err := repo.PutKMSKey(ctx, meta.PutKMSKeyRequest{
		KeyID:      "kms-key-1",
		KeyVersion: "v2",
		State:      model.KMSKeyDisabled,
	})
	if err != nil {
		t.Fatalf("PutKMSKey(update) error = %v", err)
	}
	if updated.KeyVersion != "v2" || updated.State != model.KMSKeyDisabled || !updated.CreatedAt.Equal(created.CreatedAt) || updated.UpdatedAt.Before(created.UpdatedAt) {
		t.Fatalf("updated KMS key = %+v created=%+v", updated, created)
	}
	gotKey, err := repo.GetKMSKey(ctx, "kms-key-1")
	if err != nil {
		t.Fatalf("GetKMSKey() error = %v", err)
	}
	if gotKey != updated {
		t.Fatalf("GetKMSKey() = %+v, want %+v", gotKey, updated)
	}
	keys, err := repo.ListKMSKeys(ctx, meta.ListKMSKeysRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListKMSKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0] != updated {
		t.Fatalf("ListKMSKeys() = %+v, want %+v", keys, updated)
	}
	if _, err := repo.PutKMSKey(ctx, meta.PutKMSKeyRequest{KeyVersion: "v1"}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutKMSKey(empty key) error = %v, want ErrInvalidArgument", err)
	}
}

func testKMSKeyDeleteAdmission(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "kms-delete-admission",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if _, err := repo.PutKMSKey(ctx, meta.PutKMSKeyRequest{
		KeyID:      "kms-delete-key",
		KeyVersion: "v1",
		State:      model.KMSKeyActive,
	}); err != nil {
		t.Fatalf("PutKMSKey(active) error = %v", err)
	}
	pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "encrypted.txt",
		SizeBytes: 9,
		ETag:      `"encrypted"`,
		ServerSideEncryption: model.ServerSideEncryption{
			Algorithm: model.ServerSideEncryptionAWSKMS,
			KeyID:     "kms-delete-key",
		},
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "encrypted.txt",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	for _, state := range []model.KMSKeyState{
		model.KMSKeyDisabled,
		model.KMSKeyPendingDeletion,
		model.KMSKeyDeleted,
	} {
		if _, err := repo.PutKMSKey(ctx, meta.PutKMSKeyRequest{
			KeyID:      "kms-delete-key",
			KeyVersion: "v1",
			State:      state,
		}); err != nil {
			t.Fatalf("PutKMSKey(%s) error = %v", state, err)
		}
		if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
			BucketID: bucket.BucketID,
			Key:      "encrypted.txt",
		}); !errors.Is(err, meta.ErrKMSKeyUnavailable) {
			t.Fatalf("DeleteObject(%s) error = %v, want ErrKMSKeyUnavailable", state, err)
		}
		got, err := repo.GetObjectHead(ctx, bucket.BucketID, "encrypted.txt")
		if err != nil {
			t.Fatalf("GetObjectHead(after %s denied delete) error = %v", state, err)
		}
		if got.VersionID != head.VersionID {
			t.Fatalf("head after %s denied delete = %+v, want version %q", state, got, head.VersionID)
		}
	}
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "encrypted.txt",
		VersionID: head.VersionID,
	}); !errors.Is(err, meta.ErrKMSKeyUnavailable) {
		t.Fatalf("DeleteObject(version pending key) error = %v, want ErrKMSKeyUnavailable", err)
	}
	if _, err := repo.PutKMSKey(ctx, meta.PutKMSKeyRequest{
		KeyID:      "kms-delete-key",
		KeyVersion: "v1",
		State:      model.KMSKeyActive,
	}); err != nil {
		t.Fatalf("PutKMSKey(reactive) error = %v", err)
	}
	deleted, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "encrypted.txt",
	})
	if err != nil {
		t.Fatalf("DeleteObject(active key) error = %v", err)
	}
	if !deleted.Deleted || deleted.DeletedVersionID != head.VersionID {
		t.Fatalf("active key delete result = %+v, want deleted version %q", deleted, head.VersionID)
	}
}

func testComplianceProfileAttachments(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	created, err := repo.PutComplianceProfileAttachment(ctx, meta.PutComplianceProfileAttachmentRequest{
		ProfileID:              "finra-books",
		Regulation:             "FINRA",
		RecordClass:            "broker_dealer_books",
		BucketID:               "bucket-1",
		Prefix:                 "books/",
		ObjectClass:            "books",
		RetentionMode:          model.ObjectLockModeCompliance,
		RetentionYears:         7,
		LegalHoldPolicy:        "manual_or_policy",
		GovernanceBypassPolicy: "privileged_approval_required",
		EvidenceExportPolicy:   "retention_audit_chain",
	})
	if err != nil {
		t.Fatalf("PutComplianceProfileAttachment(create) error = %v", err)
	}
	if created.ProfileID != "finra-books" || created.Regulation != "FINRA" || created.RetentionMode != model.ObjectLockModeCompliance ||
		created.RetentionYears != 7 || created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
		t.Fatalf("created compliance profile attachment = %+v", created)
	}
	updated, err := repo.PutComplianceProfileAttachment(ctx, meta.PutComplianceProfileAttachmentRequest{
		ProfileID:              "finra-books",
		Regulation:             "FINRA",
		RecordClass:            "broker_dealer_books",
		BucketID:               "bucket-2",
		Prefix:                 "books/",
		RetentionMode:          model.ObjectLockModeCompliance,
		RetentionYears:         8,
		LegalHoldPolicy:        "manual_or_policy",
		GovernanceBypassPolicy: "privileged_approval_required",
		EvidenceExportPolicy:   "retention_audit_chain",
	})
	if err != nil {
		t.Fatalf("PutComplianceProfileAttachment(update) error = %v", err)
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) || updated.UpdatedAt.Before(created.UpdatedAt) || updated.BucketID != "bucket-2" || updated.RetentionYears != 8 {
		t.Fatalf("updated compliance profile attachment = %+v created=%+v", updated, created)
	}
	second, err := repo.PutComplianceProfileAttachment(ctx, meta.PutComplianceProfileAttachmentRequest{
		ProfileID:     "hipaa-ephi",
		Regulation:    "HIPAA",
		RecordClass:   "ephi",
		BucketID:      "bucket-1",
		RetentionMode: model.ObjectLockModeGovernance,
		RetentionDays: 365,
	})
	if err != nil {
		t.Fatalf("PutComplianceProfileAttachment(second) error = %v", err)
	}
	records, err := repo.ListComplianceProfileAttachments(ctx, meta.ListComplianceProfileAttachmentsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListComplianceProfileAttachments() error = %v", err)
	}
	if len(records) != 2 || records[0].ProfileID != "finra-books" || records[1].ProfileID != second.ProfileID {
		t.Fatalf("ListComplianceProfileAttachments() = %+v", records)
	}
	filtered, err := repo.ListComplianceProfileAttachments(ctx, meta.ListComplianceProfileAttachmentsRequest{BucketID: "bucket-1", Limit: 10})
	if err != nil {
		t.Fatalf("ListComplianceProfileAttachments(bucket) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].ProfileID != second.ProfileID {
		t.Fatalf("ListComplianceProfileAttachments(bucket) = %+v", filtered)
	}
	if _, err := repo.PutComplianceProfileAttachment(ctx, meta.PutComplianceProfileAttachmentRequest{
		Regulation:     "SEC",
		RecordClass:    "records",
		RetentionMode:  model.ObjectLockModeCompliance,
		RetentionYears: 7,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutComplianceProfileAttachment(empty profile) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := repo.PutComplianceProfileAttachment(ctx, meta.PutComplianceProfileAttachmentRequest{
		ProfileID:     "invalid-duration",
		Regulation:    "SEC",
		RecordClass:   "records",
		RetentionMode: model.ObjectLockModeCompliance,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutComplianceProfileAttachment(no duration) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := repo.PutComplianceProfileAttachment(ctx, meta.PutComplianceProfileAttachmentRequest{
		ProfileID:      "negative-duration",
		Regulation:     "SEC",
		RecordClass:    "records",
		RetentionMode:  model.ObjectLockModeCompliance,
		RetentionDays:  -1,
		RetentionYears: 7,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutComplianceProfileAttachment(negative duration) error = %v, want ErrInvalidArgument", err)
	}
}

func testMultipartLifecycle(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	upload, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID:    bucket.BucketID,
		Key:         "large.bin",
		ContentType: "application/octet-stream",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
		UserMetadata: map[string]string{"color": "blue"},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	if upload.State != model.MultipartUploadActive || upload.UploadID == "" {
		t.Fatalf("upload = %+v", upload)
	}
	firstPart := multipartPartRequest(bucket.BucketID, "large.bin", upload.UploadID, 1, "segment-old", `"old"`)
	if _, previous, err := repo.PutMultipartPart(ctx, firstPart); err != nil || previous != nil {
		t.Fatalf("PutMultipartPart(first) previous = %+v error = %v", previous, err)
	}
	if _, _, err := repo.PutMultipartPart(ctx, multipartPartRequest(bucket.BucketID, "large.bin", upload.UploadID, meta.MaxMultipartParts+1, "segment-overflow", `"overflow"`)); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutMultipartPart(overflow) error = %v, want ErrInvalidArgument", err)
	}
	replacementPart := multipartPartRequest(bucket.BucketID, "large.bin", upload.UploadID, 1, "segment-new", `"new"`)
	if _, previous, err := repo.PutMultipartPart(ctx, replacementPart); err != nil {
		t.Fatalf("PutMultipartPart(replace) error = %v", err)
	} else if previous == nil || previous.SegmentRef.SegmentID != "segment-old" {
		t.Fatalf("replacement previous = %+v, want old segment", previous)
	}
	uploadSummary, err := repo.GetMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("GetMultipartUpload(summary) error = %v", err)
	}
	if uploadSummary.PartCount != 1 || uploadSummary.TotalPartSizeBytes != replacementPart.SizeBytes || uploadSummary.MaxPartNumber != 1 || uploadSummary.PartsUpdatedAt.IsZero() {
		t.Fatalf("multipart summary = %+v, want one replacement part", uploadSummary)
	}
	parts, err := repo.ListMultipartParts(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("ListMultipartParts() error = %v", err)
	}
	if len(parts) != 1 || parts[0].ETag != `"new"` || parts[0].SegmentRef.SegmentID != "segment-new" {
		t.Fatalf("parts = %+v", parts)
	}
	if _, _, err := repo.PutMultipartPart(ctx, multipartPartRequest(bucket.BucketID, "large.bin", upload.UploadID, 2, "segment-second", `"second"`)); err != nil {
		t.Fatalf("PutMultipartPart(second) error = %v", err)
	}
	if _, _, err := repo.PutMultipartPart(ctx, multipartPartRequest(bucket.BucketID, "large.bin", upload.UploadID, 3, "segment-third", `"third"`)); err != nil {
		t.Fatalf("PutMultipartPart(third) error = %v", err)
	}
	selectedParts, err := repo.GetMultipartParts(ctx, meta.GetMultipartPartsRequest{
		BucketID:    bucket.BucketID,
		Key:         "large.bin",
		UploadID:    upload.UploadID,
		PartNumbers: []int{3, 1, 4},
	})
	if err != nil {
		t.Fatalf("GetMultipartParts() error = %v", err)
	}
	if len(selectedParts) != 2 || selectedParts[0].PartNumber != 3 || selectedParts[1].PartNumber != 1 {
		t.Fatalf("selected parts = %+v, want requested order with missing part skipped", selectedParts)
	}
	if _, err := repo.GetMultipartParts(ctx, meta.GetMultipartPartsRequest{
		BucketID:    bucket.BucketID,
		Key:         "large.bin",
		UploadID:    upload.UploadID,
		PartNumbers: []int{1, 1},
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("GetMultipartParts(duplicate) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := repo.GetMultipartParts(ctx, meta.GetMultipartPartsRequest{
		BucketID:    bucket.BucketID,
		Key:         "large.bin",
		UploadID:    upload.UploadID,
		PartNumbers: []int{meta.MaxMultipartParts + 1},
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("GetMultipartParts(overflow) error = %v, want ErrInvalidArgument", err)
	}
	prepared, err := repo.PrepareMultipartCompletion(ctx, meta.PrepareMultipartCompletionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "large.bin",
		UploadID:              upload.UploadID,
		ObjectVersionID:       "version-1",
		ExpectedHeadVersionID: "",
		ETag:                  `"complete-1"`,
		SizeBytes:             10,
		PartCount:             3,
	})
	if err != nil {
		t.Fatalf("PrepareMultipartCompletion() error = %v", err)
	}
	if prepared.State != model.MultipartCompletionPrepared || prepared.ObjectVersionID != "version-1" {
		t.Fatalf("prepared completion = %+v", prepared)
	}
	preparedAgain, err := repo.PrepareMultipartCompletion(ctx, meta.PrepareMultipartCompletionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "large.bin",
		UploadID:              upload.UploadID,
		ObjectVersionID:       "version-1",
		ExpectedHeadVersionID: "",
		ETag:                  `"complete-1"`,
		SizeBytes:             10,
		PartCount:             3,
	})
	if err != nil {
		t.Fatalf("PrepareMultipartCompletion(retry) error = %v", err)
	}
	if preparedAgain.CreatedAt != prepared.CreatedAt || preparedAgain.State != model.MultipartCompletionPrepared {
		t.Fatalf("prepared retry = %+v, want original record", preparedAgain)
	}
	if _, err := repo.PrepareMultipartCompletion(ctx, meta.PrepareMultipartCompletionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "large.bin",
		UploadID:              upload.UploadID,
		ObjectVersionID:       "version-conflict",
		ExpectedHeadVersionID: "",
		ETag:                  `"complete-1"`,
		SizeBytes:             10,
		PartCount:             3,
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("PrepareMultipartCompletion(conflict) error = %v, want ErrCASConflict", err)
	}
	published, err := repo.MarkMultipartCompletionPublished(ctx, meta.MultipartCompletionStateRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("MarkMultipartCompletionPublished() error = %v", err)
	}
	if published.State != model.MultipartCompletionPublished {
		t.Fatalf("published completion = %+v", published)
	}
	if _, err := repo.CompleteMultipartUpload(ctx, meta.CompleteMultipartUploadRequest{
		BucketID:        bucket.BucketID,
		Key:             "large.bin",
		UploadID:        upload.UploadID,
		ObjectVersionID: "version-missing-count",
		ETag:            `"missing-count"`,
		SizeBytes:       10,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("CompleteMultipartUpload(missing part count) error = %v, want ErrInvalidArgument", err)
	}
	completed, err := repo.CompleteMultipartUpload(ctx, meta.CompleteMultipartUploadRequest{
		BucketID:        bucket.BucketID,
		Key:             "large.bin",
		UploadID:        upload.UploadID,
		ObjectVersionID: "version-1",
		ETag:            `"complete-1"`,
		SizeBytes:       10,
		PartCount:       3,
	})
	if err != nil {
		t.Fatalf("CompleteMultipartUpload() error = %v", err)
	}
	if completed.State != model.MultipartUploadCompleted || completed.CompletedVersionID != "version-1" || completed.CompletedETag != `"complete-1"` || completed.CompletedPartCount != 3 || completed.PartsCleanupState != model.MultipartPartsCleanupPending {
		t.Fatalf("completed upload = %+v", completed)
	}
	again, err := repo.CompleteMultipartUpload(ctx, meta.CompleteMultipartUploadRequest{
		BucketID:        bucket.BucketID,
		Key:             "large.bin",
		UploadID:        upload.UploadID,
		ObjectVersionID: "version-ignored",
		ETag:            `"ignored"`,
		SizeBytes:       99,
	})
	if err != nil {
		t.Fatalf("CompleteMultipartUpload(retry) error = %v", err)
	}
	if again.CompletedVersionID != "version-1" || again.CompletedETag != `"complete-1"` {
		t.Fatalf("retry complete = %+v, want original result", again)
	}
	cleanup, err := repo.CleanupMultipartUploadParts(ctx, meta.CleanupMultipartUploadPartsRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("CleanupMultipartUploadParts(first) error = %v", err)
	}
	if cleanup.DeletedParts != 2 || !cleanup.HasMore || cleanup.Upload.PartsCleanupState != model.MultipartPartsCleanupPending || cleanup.Upload.PartsCleanupDeleted != 2 {
		t.Fatalf("cleanup first = %+v, want two deleted with more", cleanup)
	}
	cleanup, err = repo.CleanupMultipartUploadParts(ctx, meta.CleanupMultipartUploadPartsRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
		Limit:    2,
	})
	if err != nil {
		t.Fatalf("CleanupMultipartUploadParts(second) error = %v", err)
	}
	if cleanup.DeletedParts != 1 || cleanup.HasMore || cleanup.Upload.PartsCleanupState != model.MultipartPartsCleanupDone || cleanup.Upload.PartsCleanupDeleted != 3 {
		t.Fatalf("cleanup second = %+v, want final cleanup", cleanup)
	}
	cleanup, err = repo.CleanupMultipartUploadParts(ctx, meta.CleanupMultipartUploadPartsRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("CleanupMultipartUploadParts(done retry) error = %v", err)
	}
	if cleanup.DeletedParts != 0 || cleanup.HasMore || cleanup.Upload.PartsCleanupState != model.MultipartPartsCleanupDone {
		t.Fatalf("cleanup retry = %+v, want no-op done", cleanup)
	}
	completedRecord, err := repo.MarkMultipartCompletionCompleted(ctx, meta.MultipartCompletionStateRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("MarkMultipartCompletionCompleted() error = %v", err)
	}
	if completedRecord.State != model.MultipartCompletionCompleted {
		t.Fatalf("completed record = %+v", completedRecord)
	}
	gotCompletion, err := repo.GetMultipartCompletion(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "large.bin",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("GetMultipartCompletion() error = %v", err)
	}
	if gotCompletion.State != model.MultipartCompletionCompleted || gotCompletion.ObjectVersionID != "version-1" {
		t.Fatalf("completion record = %+v", gotCompletion)
	}
	if _, err := repo.ListMultipartParts(ctx, meta.MultipartUploadRequest{BucketID: bucket.BucketID, Key: "large.bin", UploadID: upload.UploadID}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("ListMultipartParts(completed) error = %v, want ErrNotFound", err)
	}
	if _, err := repo.GetMultipartParts(ctx, meta.GetMultipartPartsRequest{BucketID: bucket.BucketID, Key: "large.bin", UploadID: upload.UploadID, PartNumbers: []int{1}}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetMultipartParts(completed) error = %v, want ErrNotFound", err)
	}

	abortUpload, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "abort.bin",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(abort) error = %v", err)
	}
	if _, _, err := repo.PutMultipartPart(ctx, multipartPartRequest(bucket.BucketID, "abort.bin", abortUpload.UploadID, 1, "segment-abort", `"abort"`)); err != nil {
		t.Fatalf("PutMultipartPart(abort) error = %v", err)
	}
	abortedParts, err := repo.AbortMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "abort.bin",
		UploadID: abortUpload.UploadID,
	})
	if err != nil {
		t.Fatalf("AbortMultipartUpload() error = %v", err)
	}
	if len(abortedParts) != 1 || abortedParts[0].SegmentRef.SegmentID != "segment-abort" {
		t.Fatalf("aborted parts = %+v", abortedParts)
	}
	abortedParts, err = repo.AbortMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "abort.bin",
		UploadID: abortUpload.UploadID,
	})
	if err != nil {
		t.Fatalf("AbortMultipartUpload(retry) error = %v", err)
	}
	if len(abortedParts) != 1 || abortedParts[0].SegmentRef.SegmentID != "segment-abort" {
		t.Fatalf("AbortMultipartUpload(retry) parts = %+v, want remaining aborted part", abortedParts)
	}
	abortCleanup, err := repo.CleanupMultipartUploadParts(ctx, meta.CleanupMultipartUploadPartsRequest{
		BucketID: bucket.BucketID,
		Key:      "abort.bin",
		UploadID: abortUpload.UploadID,
	})
	if err != nil {
		t.Fatalf("CleanupMultipartUploadParts(abort) error = %v", err)
	}
	if abortCleanup.DeletedParts != 1 || abortCleanup.HasMore || abortCleanup.Upload.PartsCleanupState != model.MultipartPartsCleanupDone {
		t.Fatalf("abort cleanup = %+v, want one deleted", abortCleanup)
	}
	gotAbort, err := repo.GetMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "abort.bin",
		UploadID: abortUpload.UploadID,
	})
	if err != nil {
		t.Fatalf("GetMultipartUpload(aborted) error = %v", err)
	}
	if gotAbort.State != model.MultipartUploadAborted {
		t.Fatalf("aborted state = %q", gotAbort.State)
	}
}

func testListMultipartUploads(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	first, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "logs/2026/a.bin",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(first) error = %v", err)
	}
	second, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "logs/2026/b.bin",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(second) error = %v", err)
	}
	third, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "logs/2027/c.bin",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(third) error = %v", err)
	}
	result, err := repo.ListMultipartUploads(ctx, meta.ListMultipartUploadsRequest{
		BucketID:   bucket.BucketID,
		Prefix:     "logs/",
		Delimiter:  "/",
		MaxUploads: 10,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(delimiter) error = %v", err)
	}
	if len(result.Uploads) != 0 || !reflect.DeepEqual(result.CommonPrefixes, []string{"logs/2026/", "logs/2027/"}) {
		t.Fatalf("delimiter result = %+v", result)
	}
	page, err := repo.ListMultipartUploads(ctx, meta.ListMultipartUploadsRequest{
		BucketID:   bucket.BucketID,
		Prefix:     "logs/2026/",
		MaxUploads: 1,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(page1) error = %v", err)
	}
	if !page.IsTruncated || page.NextKeyMarker != "logs/2026/a.bin" || page.NextUploadIDMarker != first.UploadID {
		t.Fatalf("page1 = %+v", page)
	}
	next, err := repo.ListMultipartUploads(ctx, meta.ListMultipartUploadsRequest{
		BucketID:       bucket.BucketID,
		Prefix:         "logs/2026/",
		KeyMarker:      page.NextKeyMarker,
		UploadIDMarker: page.NextUploadIDMarker,
		MaxUploads:     10,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(page2) error = %v", err)
	}
	if len(next.Uploads) != 1 || next.Uploads[0].UploadID != second.UploadID {
		t.Fatalf("page2 = %+v, want second upload", next)
	}
	if _, err := repo.CompleteMultipartUpload(ctx, meta.CompleteMultipartUploadRequest{
		BucketID:        bucket.BucketID,
		Key:             third.Key,
		UploadID:        third.UploadID,
		ObjectVersionID: "version-third",
		ETag:            `"third"`,
		SizeBytes:       0,
		PartCount:       1,
	}); err != nil {
		t.Fatalf("CompleteMultipartUpload(third) error = %v", err)
	}
	afterComplete, err := repo.ListMultipartUploads(ctx, meta.ListMultipartUploadsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "logs/2027/",
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(after complete) error = %v", err)
	}
	if len(afterComplete.Uploads) != 0 {
		t.Fatalf("completed upload should be hidden: %+v", afterComplete)
	}
}

func testBucketVersioningAndDeleteMarkers(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	updated, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	})
	if err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	if updated.VersioningState != model.BucketVersioningEnabled {
		t.Fatalf("VersioningState = %q, want Enabled", updated.VersioningState)
	}

	first := putObjectVersion(t, repo, bucket.BucketID, "docs/readme.txt", "segment-first")
	second := putObjectVersion(t, repo, bucket.BucketID, "docs/readme.txt", "segment-second")
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "docs/readme.txt")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if head.VersionID != second.VersionID {
		t.Fatalf("head version = %q, want %q", head.VersionID, second.VersionID)
	}

	list, err := repo.ListObjectVersions(ctx, meta.ListObjectVersionsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "docs/",
		MaxKeys:  10,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions() error = %v", err)
	}
	if len(list.Versions) != 2 || list.Versions[0].Version.VersionID != second.VersionID || !list.Versions[0].IsLatest {
		t.Fatalf("initial versions = %+v", list.Versions)
	}
	if list.Versions[1].Version.VersionID != first.VersionID || list.Versions[1].IsLatest {
		t.Fatalf("older version = %+v", list.Versions[1])
	}

	deleted, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "docs/readme.txt",
	})
	if err != nil {
		t.Fatalf("DeleteObject(delete marker) error = %v", err)
	}
	if !deleted.DeleteMarker || deleted.DeletedVersionID == "" {
		t.Fatalf("delete marker result = %+v", deleted)
	}
	markerHead, err := repo.GetObjectHead(ctx, bucket.BucketID, "docs/readme.txt")
	if err != nil {
		t.Fatalf("GetObjectHead(delete marker) error = %v", err)
	}
	if !markerHead.DeleteMarker || markerHead.VersionID != deleted.DeletedVersionID {
		t.Fatalf("delete marker head = %+v, result = %+v", markerHead, deleted)
	}
	visible, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "docs/",
		MaxKeys:  10,
	})
	if err != nil {
		t.Fatalf("ListObjects(after delete marker) error = %v", err)
	}
	if len(visible.Contents) != 0 {
		t.Fatalf("visible objects after delete marker = %+v", visible.Contents)
	}

	withMarker, err := repo.ListObjectVersions(ctx, meta.ListObjectVersionsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "docs/",
		MaxKeys:  10,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(after delete marker) error = %v", err)
	}
	if len(withMarker.DeleteMarkers) != 1 || withMarker.DeleteMarkers[0].Version.VersionID != deleted.DeletedVersionID || !withMarker.DeleteMarkers[0].IsLatest {
		t.Fatalf("delete markers = %+v", withMarker.DeleteMarkers)
	}
	if len(withMarker.Versions) != 2 {
		t.Fatalf("versions with marker = %+v", withMarker.Versions)
	}

	removeMarker, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "docs/readme.txt",
		VersionID: deleted.DeletedVersionID,
	})
	if err != nil {
		t.Fatalf("DeleteObject(marker version) error = %v", err)
	}
	if !removeMarker.DeleteMarker {
		t.Fatalf("remove marker result = %+v", removeMarker)
	}
	restored, err := repo.GetObjectHead(ctx, bucket.BucketID, "docs/readme.txt")
	if err != nil {
		t.Fatalf("GetObjectHead(after marker delete) error = %v", err)
	}
	if restored.VersionID != second.VersionID || restored.DeleteMarker {
		t.Fatalf("restored head = %+v, want second object version", restored)
	}

	removeFirst, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "docs/readme.txt",
		VersionID: first.VersionID,
	})
	if err != nil {
		t.Fatalf("DeleteObject(first version) error = %v", err)
	}
	if removeFirst.DeletedVersion.SegmentRef.SegmentID != "segment-first" {
		t.Fatalf("removed first version = %+v", removeFirst.DeletedVersion)
	}
	if _, err := repo.GetObjectVersion(ctx, bucket.BucketID, "docs/readme.txt", first.VersionID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetObjectVersion(deleted first) error = %v, want ErrNotFound", err)
	}

	page, err := repo.ListObjectVersions(ctx, meta.ListObjectVersionsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "docs/",
		MaxKeys:  1,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(page1) error = %v", err)
	}
	if page.IsTruncated || len(page.Versions) != 1 || page.Versions[0].Version.VersionID != second.VersionID {
		t.Fatalf("final versions page = %+v", page)
	}
}

func testBucketCORSLifecycle(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	rules := []model.CORSRule{
		{
			AllowedOrigins: []string{"https://console.example"},
			AllowedMethods: []string{"GET", "PUT"},
			AllowedHeaders: []string{"x-amz-meta-*", "content-type"},
			ExposeHeaders:  []string{"ETag", "x-amz-version-id"},
			MaxAgeSeconds:  300,
		},
	}
	updated, err := repo.PutBucketCORS(ctx, meta.BucketCORSRequest{
		BucketID: bucket.BucketID,
		Rules:    rules,
	})
	if err != nil {
		t.Fatalf("PutBucketCORS() error = %v", err)
	}
	if !reflect.DeepEqual(updated.CORSRules, rules) {
		t.Fatalf("updated CORS rules = %+v, want %+v", updated.CORSRules, rules)
	}
	got, err := repo.GetBucketCORS(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketCORS() error = %v", err)
	}
	if !reflect.DeepEqual(got, rules) {
		t.Fatalf("CORS rules = %+v, want %+v", got, rules)
	}
	got[0].AllowedOrigins[0] = "mutated"
	again, err := repo.GetBucketCORS(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketCORS(again) error = %v", err)
	}
	if again[0].AllowedOrigins[0] != "https://console.example" {
		t.Fatalf("CORS rules were mutated through returned slice: %+v", again)
	}
	deleted, err := repo.DeleteBucketCORS(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("DeleteBucketCORS() error = %v", err)
	}
	if len(deleted.CORSRules) != 0 {
		t.Fatalf("deleted bucket CORS rules = %+v, want none", deleted.CORSRules)
	}
	empty, err := repo.GetBucketCORS(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketCORS(after delete) error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("CORS rules after delete = %+v, want none", empty)
	}
}

func testBucketLifecycleConfiguration(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.GetBucketLifecycle(ctx, bucket.BucketID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketLifecycle(missing) error = %v, want ErrNotFound", err)
	}
	configuration := model.BucketLifecycleConfiguration{
		Rules: []model.LifecycleRule{
			{
				ID:     "expire-current",
				Status: model.LifecycleRuleEnabled,
				Prefix: "logs/",
				Expiration: model.LifecycleExpiration{
					Days: 30,
				},
				NoncurrentVersionExpiration: model.LifecycleNoncurrentVersionExpiration{
					NoncurrentDays: 7,
				},
			},
			{
				ID:     "abort-mpu",
				Status: model.LifecycleRuleEnabled,
				AbortIncompleteMultipartUpload: model.LifecycleAbortIncompleteMultipartUpload{
					DaysAfterInitiation: 3,
				},
			},
		},
	}
	updated, err := repo.PutBucketLifecycle(ctx, meta.BucketLifecycleRequest{
		BucketID:      bucket.BucketID,
		Configuration: configuration,
	})
	if err != nil {
		t.Fatalf("PutBucketLifecycle() error = %v", err)
	}
	if !reflect.DeepEqual(updated.Lifecycle, configuration) {
		t.Fatalf("updated lifecycle = %+v, want %+v", updated.Lifecycle, configuration)
	}
	got, err := repo.GetBucketLifecycle(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketLifecycle() error = %v", err)
	}
	if !reflect.DeepEqual(got, configuration) {
		t.Fatalf("lifecycle = %+v, want %+v", got, configuration)
	}
	got.Rules[0].Prefix = "mutated/"
	again, err := repo.GetBucketLifecycle(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketLifecycle(again) error = %v", err)
	}
	if again.Rules[0].Prefix != "logs/" {
		t.Fatalf("lifecycle was mutated through returned slice: %+v", again)
	}
	putEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Action:   model.AuditActionPutBucketLifecycle,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(put lifecycle) error = %v", err)
	}
	if len(putEvents) != 1 || putEvents[0].Details["rule_count"] != "2" || putEvents[0].EventHash == "" {
		t.Fatalf("put lifecycle audit events = %+v, want one event with rule_count", putEvents)
	}
	deleted, err := repo.DeleteBucketLifecycle(ctx, bucket.BucketID, meta.AuditContext{Reason: "test-delete-lifecycle"})
	if err != nil {
		t.Fatalf("DeleteBucketLifecycle() error = %v", err)
	}
	if len(deleted.Lifecycle.Rules) != 0 {
		t.Fatalf("deleted lifecycle = %+v, want none", deleted.Lifecycle)
	}
	if _, err := repo.GetBucketLifecycle(ctx, bucket.BucketID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketLifecycle(after delete) error = %v, want ErrNotFound", err)
	}
	deleteEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Action:   model.AuditActionDeleteBucketLifecycle,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(delete lifecycle) error = %v", err)
	}
	if len(deleteEvents) != 1 || deleteEvents[0].PreviousHash != putEvents[0].EventHash {
		t.Fatalf("delete lifecycle audit events = %+v, want chained event", deleteEvents)
	}
}

func testBucketPolicyLifecycle(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.GetBucketPolicy(ctx, bucket.BucketID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketPolicy(missing) error = %v, want ErrNotFound", err)
	}
	policy := auth.PolicyDocument{
		Version: "2012-10-17",
		Statements: []auth.PolicyStatement{{
			Effect:     "Allow",
			Principals: []string{"ak-bypass"},
			Actions:    []string{auth.ActionBypassGovernanceRetention},
			Resources:  []string{"arn:aws:s3:::" + bucket.Name + "/*"},
		}},
	}
	if _, err := repo.PutBucketPolicy(ctx, meta.BucketPolicyRequest{
		BucketID: bucket.BucketID,
		Policy:   policy,
	}); err != nil {
		t.Fatalf("PutBucketPolicy() error = %v", err)
	}
	putEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Action:   model.AuditActionPutBucketPolicy,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(put bucket policy) error = %v", err)
	}
	if len(putEvents) != 1 || putEvents[0].Details["statement_count"] != "1" || putEvents[0].EventHash == "" {
		t.Fatalf("put bucket policy audit events = %+v, want one event with statement_count", putEvents)
	}
	got, err := repo.GetBucketPolicy(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketPolicy() error = %v", err)
	}
	if !got.Allows(auth.Principal{AccessKeyID: "ak-bypass"}, auth.ActionBypassGovernanceRetention, "arn:aws:s3:::"+bucket.Name+"/object.txt") {
		t.Fatalf("bucket policy does not allow expected governance bypass: %+v", got)
	}
	if _, err := repo.DeleteBucketPolicy(ctx, bucket.BucketID, meta.AuditContext{}); err != nil {
		t.Fatalf("DeleteBucketPolicy() error = %v", err)
	}
	deleteEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Action:   model.AuditActionDeleteBucketPolicy,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(delete bucket policy) error = %v", err)
	}
	if len(deleteEvents) != 1 || deleteEvents[0].PreviousHash != putEvents[0].EventHash {
		t.Fatalf("delete bucket policy audit events = %+v, want chained event after %+v", deleteEvents, putEvents[0])
	}
	if _, err := repo.GetBucketPolicy(ctx, bucket.BucketID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetBucketPolicy(deleted) error = %v, want ErrNotFound", err)
	}
}

func testObjectLockMetadataLifecycle(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	configuration := model.BucketObjectLockConfiguration{
		Enabled: true,
		DefaultRetention: model.BucketObjectLockDefaultRetention{
			Mode: model.ObjectLockModeGovernance,
			Days: 30,
		},
	}
	updated, err := repo.PutBucketObjectLock(ctx, meta.BucketObjectLockRequest{
		BucketID:      bucket.BucketID,
		Configuration: configuration,
	})
	if err != nil {
		t.Fatalf("PutBucketObjectLock() error = %v", err)
	}
	if updated.VersioningState != model.BucketVersioningEnabled {
		t.Fatalf("object lock bucket versioning = %q, want Enabled", updated.VersioningState)
	}
	if !reflect.DeepEqual(updated.ObjectLock, configuration) {
		t.Fatalf("updated object lock config = %+v, want %+v", updated.ObjectLock, configuration)
	}
	objectLockEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Action:   model.AuditActionPutBucketObjectLock,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(put bucket object lock) error = %v", err)
	}
	if len(objectLockEvents) != 1 || objectLockEvents[0].Details["default_retention_days"] != "30" {
		t.Fatalf("object lock audit events = %+v, want default retention detail", objectLockEvents)
	}
	got, err := repo.GetBucketObjectLock(ctx, bucket.BucketID)
	if err != nil {
		t.Fatalf("GetBucketObjectLock() error = %v", err)
	}
	if !reflect.DeepEqual(got, configuration) {
		t.Fatalf("object lock config = %+v, want %+v", got, configuration)
	}
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningSuspended,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutBucketVersioning(Suspended object lock bucket) error = %v, want ErrInvalidArgument", err)
	}

	retainUntil := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	pending, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "locked-object",
		ETag:     `"locked"`,
		SegmentRef: storage.SegmentRef{
			SegmentID: "segment-locked-object",
			SizeBytes: 13,
		},
		ObjectLockRetention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeCompliance,
			RetainUntilDate: retainUntil,
		},
		ObjectLockLegalHold: model.ObjectLockLegalHoldOn,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(locked) error = %v", err)
	}
	if pending.Version.ObjectLockRetention.Mode != model.ObjectLockModeCompliance || !pending.Version.ObjectLockRetention.RetainUntilDate.Equal(retainUntil) {
		t.Fatalf("pending retention = %+v", pending.Version.ObjectLockRetention)
	}
	if pending.Version.ObjectLockLegalHold != model.ObjectLockLegalHoldOn {
		t.Fatalf("pending legal hold = %q, want ON", pending.Version.ObjectLockLegalHold)
	}
	head, err := repo.CommitObjectVersion(ctx, meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "locked-object",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion(locked) error = %v", err)
	}
	if head.ObjectLockRetention.Mode != model.ObjectLockModeCompliance || !head.ObjectLockRetention.RetainUntilDate.Equal(retainUntil) {
		t.Fatalf("head retention = %+v", head.ObjectLockRetention)
	}
	if head.ObjectLockLegalHold != model.ObjectLockLegalHoldOn {
		t.Fatalf("head legal hold = %q, want ON", head.ObjectLockLegalHold)
	}
	version, err := repo.GetObjectVersion(ctx, bucket.BucketID, "locked-object", pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(locked) error = %v", err)
	}
	if version.ObjectLockRetention.Mode != model.ObjectLockModeCompliance || !version.ObjectLockRetention.RetainUntilDate.Equal(retainUntil) {
		t.Fatalf("version retention = %+v", version.ObjectLockRetention)
	}
	if version.ObjectLockLegalHold != model.ObjectLockLegalHoldOn {
		t.Fatalf("version legal hold = %q, want ON", version.ObjectLockLegalHold)
	}
	protectedRefs, err := repo.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		BucketID:   bucket.BucketID,
		Key:        "locked-object",
		VersionID:  version.VersionID,
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListProtectedRefs(locked version) error = %v", err)
	}
	if len(protectedRefs) != 1 {
		t.Fatalf("protected refs = %d, want 1: %+v", len(protectedRefs), protectedRefs)
	}
	if protectedRefs[0].SegmentID != "segment-locked-object" || protectedRefs[0].Reason != model.ProtectedRefReasonObjectLock || protectedRefs[0].LegalHold != model.ObjectLockLegalHoldOn {
		t.Fatalf("protected ref = %+v", protectedRefs[0])
	}
	bySegment, err := repo.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		SegmentID:  "segment-locked-object",
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListProtectedRefs(segment) error = %v", err)
	}
	if len(bySegment) != 1 || bySegment[0].VersionID != version.VersionID {
		t.Fatalf("segment protected refs = %+v", bySegment)
	}

	upload, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "locked-mpu",
		ObjectLockRetention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeGovernance,
			RetainUntilDate: retainUntil,
		},
		ObjectLockLegalHold: model.ObjectLockLegalHoldOff,
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(locked) error = %v", err)
	}
	gotUpload, err := repo.GetMultipartUpload(ctx, meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "locked-mpu",
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("GetMultipartUpload(locked) error = %v", err)
	}
	if gotUpload.ObjectLockRetention.Mode != model.ObjectLockModeGovernance || !gotUpload.ObjectLockRetention.RetainUntilDate.Equal(retainUntil) {
		t.Fatalf("upload retention = %+v", gotUpload.ObjectLockRetention)
	}
	if gotUpload.ObjectLockLegalHold != model.ObjectLockLegalHoldOff {
		t.Fatalf("upload legal hold = %q, want OFF", gotUpload.ObjectLockLegalHold)
	}
	if _, err := repo.BeginPutObject(ctx, meta.BeginPutObjectRequest{
		BucketID:            bucket.BucketID,
		Key:                 "bad-lock",
		ObjectLockLegalHold: model.ObjectLockLegalHoldStatus("MAYBE"),
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("BeginPutObject(invalid legal hold) error = %v, want ErrInvalidArgument", err)
	}
}

func testObjectLockDeleteEnforcement(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.PutBucketObjectLock(ctx, meta.BucketObjectLockRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketObjectLockConfiguration{
			Enabled: true,
		},
	}); err != nil {
		t.Fatalf("PutBucketObjectLock() error = %v", err)
	}
	retainUntil := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	compliance := putObjectVersionWithLock(t, repo, bucket.BucketID, "compliance.txt", model.ObjectLockRetention{
		Mode:            model.ObjectLockModeCompliance,
		RetainUntilDate: retainUntil,
	}, "")
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "compliance.txt",
		VersionID: compliance.VersionID,
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("DeleteObject(compliance) error = %v, want ErrObjectLocked", err)
	}
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "compliance.txt",
		VersionID:                 compliance.VersionID,
		BypassGovernanceRetention: true,
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("DeleteObject(compliance bypass) error = %v, want ErrObjectLocked", err)
	}
	if _, err := repo.GetObjectVersion(ctx, bucket.BucketID, "compliance.txt", compliance.VersionID); err != nil {
		t.Fatalf("GetObjectVersion(compliance after denied delete) error = %v", err)
	}

	legalHold := putObjectVersionWithLock(t, repo, bucket.BucketID, "legal-hold.txt", model.ObjectLockRetention{}, model.ObjectLockLegalHoldOn)
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "legal-hold.txt",
		VersionID:                 legalHold.VersionID,
		BypassGovernanceRetention: true,
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("DeleteObject(legal hold bypass) error = %v, want ErrObjectLocked", err)
	}

	governance := putObjectVersionWithLock(t, repo, bucket.BucketID, "governance.txt", model.ObjectLockRetention{
		Mode:            model.ObjectLockModeGovernance,
		RetainUntilDate: retainUntil,
	}, "")
	protectedRefs, err := repo.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		BucketID:   bucket.BucketID,
		Key:        "governance.txt",
		VersionID:  governance.VersionID,
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListProtectedRefs(governance) error = %v", err)
	}
	if len(protectedRefs) != 1 || protectedRefs[0].SegmentID != "governance.txt" {
		t.Fatalf("governance protected refs = %+v, want one active ref", protectedRefs)
	}
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "governance.txt",
		VersionID: governance.VersionID,
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("DeleteObject(governance) error = %v, want ErrObjectLocked", err)
	}
	deleted, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "governance.txt",
		VersionID:                 governance.VersionID,
		BypassGovernanceRetention: true,
	})
	if !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("DeleteObject(governance bypass without audit) error = %v, want ErrObjectLocked", err)
	}
	deleted, err = repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "governance.txt",
		VersionID:                 governance.VersionID,
		BypassGovernanceRetention: true,
		BypassAudit:               governanceBypassAudit("delete retained object for test"),
	})
	if err != nil {
		t.Fatalf("DeleteObject(governance bypass) error = %v", err)
	}
	if deleted.DeletedVersionID != governance.VersionID {
		t.Fatalf("governance bypass deleted version = %q, want %q", deleted.DeletedVersionID, governance.VersionID)
	}
	if _, err := repo.GetObjectVersion(ctx, bucket.BucketID, "governance.txt", governance.VersionID); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetObjectVersion(governance deleted) error = %v, want ErrNotFound", err)
	}
	protectedRefs, err = repo.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		BucketID:  bucket.BucketID,
		Key:       "governance.txt",
		VersionID: governance.VersionID,
	})
	if err != nil {
		t.Fatalf("ListProtectedRefs(governance deleted) error = %v", err)
	}
	if len(protectedRefs) != 0 {
		t.Fatalf("governance protected refs after delete = %+v, want none", protectedRefs)
	}
	events, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Key:      "governance.txt",
		Action:   model.AuditActionGovernanceBypassDeleteObject,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(governance delete) error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("governance delete audit events = %d, want 1: %+v", len(events), events)
	}
	if events[0].Reason != "delete retained object for test" || events[0].Principal.AccessKeyID != "root-access-key" || !events[0].Principal.Root {
		t.Fatalf("governance delete audit event = %+v", events[0])
	}
	if events[0].EventHash == "" || events[0].VersionID != governance.VersionID {
		t.Fatalf("governance delete audit hash/version = %+v", events[0])
	}

	expired := putObjectVersionWithLock(t, repo, bucket.BucketID, "expired.txt", model.ObjectLockRetention{
		Mode:            model.ObjectLockModeCompliance,
		RetainUntilDate: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC),
	}, "")
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "expired.txt",
		VersionID: expired.VersionID,
	}); err != nil {
		t.Fatalf("DeleteObject(expired compliance) error = %v", err)
	}
}

func testObjectLockRetentionLegalHoldAPI(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.PutBucketObjectLock(ctx, meta.BucketObjectLockRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketObjectLockConfiguration{
			Enabled: true,
		},
	}); err != nil {
		t.Fatalf("PutBucketObjectLock() error = %v", err)
	}
	initial := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	version := putObjectVersionWithLock(t, repo, bucket.BucketID, "api-lock.txt", model.ObjectLockRetention{
		Mode:            model.ObjectLockModeGovernance,
		RetainUntilDate: initial,
	}, model.ObjectLockLegalHoldOff)
	retention, err := repo.GetObjectRetention(ctx, meta.ObjectRetentionRequest{
		BucketID:  bucket.BucketID,
		Key:       "api-lock.txt",
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("GetObjectRetention() error = %v", err)
	}
	if retention.Mode != model.ObjectLockModeGovernance || !retention.RetainUntilDate.Equal(initial) {
		t.Fatalf("initial retention = %+v", retention)
	}
	extended := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if err := repo.PutObjectRetention(ctx, meta.ObjectRetentionRequest{
		BucketID:  bucket.BucketID,
		Key:       "api-lock.txt",
		VersionID: version.VersionID,
		Retention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeCompliance,
			RetainUntilDate: extended,
		},
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("PutObjectRetention(governance without bypass) error = %v, want ErrObjectLocked", err)
	}
	if err := repo.PutObjectRetention(ctx, meta.ObjectRetentionRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "api-lock.txt",
		VersionID:                 version.VersionID,
		BypassGovernanceRetention: true,
		Retention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeCompliance,
			RetainUntilDate: extended,
		},
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("PutObjectRetention(governance bypass without audit) error = %v, want ErrObjectLocked", err)
	}
	if err := repo.PutObjectRetention(ctx, meta.ObjectRetentionRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "api-lock.txt",
		VersionID:                 version.VersionID,
		BypassGovernanceRetention: true,
		BypassAudit:               governanceBypassAudit("extend governance retention for test"),
		Retention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeCompliance,
			RetainUntilDate: extended,
		},
	}); err != nil {
		t.Fatalf("PutObjectRetention(governance bypass) error = %v", err)
	}
	events, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Key:      "api-lock.txt",
		Action:   model.AuditActionGovernanceBypassPutObjectRetention,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(retention bypass) error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("retention bypass audit events = %d, want 1: %+v", len(events), events)
	}
	if events[0].Reason != "extend governance retention for test" || events[0].EventHash == "" {
		t.Fatalf("retention bypass audit event = %+v", events[0])
	}
	retentionEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Key:      "api-lock.txt",
		Action:   model.AuditActionPutObjectRetention,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(retention transition) error = %v", err)
	}
	if len(retentionEvents) != 1 || retentionEvents[0].Details["next_mode"] != string(model.ObjectLockModeCompliance) {
		t.Fatalf("retention transition audit events = %+v, want next compliance mode", retentionEvents)
	}
	retention, err = repo.GetObjectRetention(ctx, meta.ObjectRetentionRequest{
		BucketID: bucket.BucketID,
		Key:      "api-lock.txt",
	})
	if err != nil {
		t.Fatalf("GetObjectRetention(current) error = %v", err)
	}
	if retention.Mode != model.ObjectLockModeCompliance || !retention.RetainUntilDate.Equal(extended) {
		t.Fatalf("updated retention = %+v", retention)
	}
	status, err := repo.GetObjectLegalHold(ctx, meta.ObjectLegalHoldRequest{
		BucketID:  bucket.BucketID,
		Key:       "api-lock.txt",
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("GetObjectLegalHold() error = %v", err)
	}
	if status != model.ObjectLockLegalHoldOff {
		t.Fatalf("initial legal hold = %q, want OFF", status)
	}
	if err := repo.PutObjectLegalHold(ctx, meta.ObjectLegalHoldRequest{
		BucketID:  bucket.BucketID,
		Key:       "api-lock.txt",
		VersionID: version.VersionID,
		LegalHold: model.ObjectLockLegalHoldOn,
	}); err != nil {
		t.Fatalf("PutObjectLegalHold(ON) error = %v", err)
	}
	status, err = repo.GetObjectLegalHold(ctx, meta.ObjectLegalHoldRequest{
		BucketID: bucket.BucketID,
		Key:      "api-lock.txt",
	})
	if err != nil {
		t.Fatalf("GetObjectLegalHold(current) error = %v", err)
	}
	if status != model.ObjectLockLegalHoldOn {
		t.Fatalf("updated legal hold = %q, want ON", status)
	}
	legalHoldEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		BucketID: bucket.BucketID,
		Key:      "api-lock.txt",
		Action:   model.AuditActionPutObjectLegalHold,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(legal hold transition) error = %v", err)
	}
	if len(legalHoldEvents) != 1 || legalHoldEvents[0].Details["next_legal_hold"] != string(model.ObjectLockLegalHoldOn) {
		t.Fatalf("legal hold transition audit events = %+v, want next ON", legalHoldEvents)
	}
	if _, err := repo.DeleteObject(ctx, meta.DeleteObjectRequest{
		BucketID:                  bucket.BucketID,
		Key:                       "api-lock.txt",
		VersionID:                 version.VersionID,
		BypassGovernanceRetention: true,
	}); !errors.Is(err, meta.ErrObjectLocked) {
		t.Fatalf("DeleteObject(legal hold) error = %v, want ErrObjectLocked", err)
	}
}

func testObjectLockAuditTransitionChain(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	policy := auth.PolicyDocument{
		Version: "2012-10-17",
		Statements: []auth.PolicyStatement{{
			Effect:     "Allow",
			Principals: []string{"ak-bypass"},
			Actions:    []string{auth.ActionBypassGovernanceRetention},
			Resources:  []string{"arn:aws:s3:::" + bucket.Name + "/*"},
		}},
	}
	if _, err := repo.PutBucketPolicy(ctx, meta.BucketPolicyRequest{
		BucketID: bucket.BucketID,
		Policy:   policy,
		Audit:    governanceBypassAudit("attach bucket policy"),
	}); err != nil {
		t.Fatalf("PutBucketPolicy() error = %v", err)
	}
	if _, err := repo.PutBucketObjectLock(ctx, meta.BucketObjectLockRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketObjectLockConfiguration{
			Enabled: true,
		},
		Audit: governanceBypassAudit("enable object lock"),
	}); err != nil {
		t.Fatalf("PutBucketObjectLock() error = %v", err)
	}
	version := putObjectVersionWithLock(t, repo, bucket.BucketID, "audit-lock.txt", model.ObjectLockRetention{}, model.ObjectLockLegalHoldOff)
	extended := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if err := repo.PutObjectRetention(ctx, meta.ObjectRetentionRequest{
		BucketID:  bucket.BucketID,
		Key:       "audit-lock.txt",
		VersionID: version.VersionID,
		Retention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeGovernance,
			RetainUntilDate: extended,
		},
		Audit: governanceBypassAudit("extend retention"),
	}); err != nil {
		t.Fatalf("PutObjectRetention() error = %v", err)
	}
	if err := repo.PutObjectLegalHold(ctx, meta.ObjectLegalHoldRequest{
		BucketID:  bucket.BucketID,
		Key:       "audit-lock.txt",
		VersionID: version.VersionID,
		LegalHold: model.ObjectLockLegalHoldOn,
		Audit:     governanceBypassAudit("enable legal hold"),
	}); err != nil {
		t.Fatalf("PutObjectLegalHold() error = %v", err)
	}

	events, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("ListAuditEvents(bucket) error = %v", err)
	}
	wantActions := []model.AuditAction{
		model.AuditActionPutBucketPolicy,
		model.AuditActionPutBucketObjectLock,
		model.AuditActionPutObjectRetention,
		model.AuditActionPutObjectLegalHold,
	}
	if len(events) != len(wantActions) {
		t.Fatalf("audit events = %d, want %d: %+v", len(events), len(wantActions), events)
	}
	for i, action := range wantActions {
		if events[i].Action != action {
			t.Fatalf("audit event[%d].Action = %q, want %q: %+v", i, events[i].Action, action, events)
		}
		if events[i].EventHash == "" {
			t.Fatalf("audit event[%d] missing hash: %+v", i, events[i])
		}
		if i > 0 && events[i].PreviousHash != events[i-1].EventHash {
			t.Fatalf("audit event[%d] previous hash = %q, want %q", i, events[i].PreviousHash, events[i-1].EventHash)
		}
	}
	if events[2].Details["next_retain_until"] == "" || events[3].Details["next_legal_hold"] != string(model.ObjectLockLegalHoldOn) {
		t.Fatalf("audit event details = retention %+v legal-hold %+v", events[2], events[3])
	}
}

func testAdminAuditEvents(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	first, err := repo.PutAdminAuditEvent(ctx, meta.PutAdminAuditEventRequest{
		Action: model.AuditActionDedupeAck,
		Details: map[string]string{
			"operation_id": "dedupe-00000000000000000001",
			"status":       "succeeded",
			"tenant_id":    "tenant-1",
		},
		Audit: meta.AuditContext{
			RequestID: "request-1",
			Reason:    "operator accepted dedupe",
			Principal: model.AuditPrincipal{
				AccessKeyID: "ak-admin",
				DisplayName: "admin",
				Root:        true,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutAdminAuditEvent(ack) error = %v", err)
	}
	if first.EventID == "" || first.EventHash == "" || first.Action != model.AuditActionDedupeAck {
		t.Fatalf("first admin audit event = %+v", first)
	}
	if first.Reason != "operator accepted dedupe" || first.Details["operation_id"] == "" {
		t.Fatalf("first admin audit context/details = %+v", first)
	}
	first.Details["operation_id"] = "mutated"

	second, err := repo.PutAdminAuditEvent(ctx, meta.PutAdminAuditEventRequest{
		Action: model.AuditActionDedupeRepair,
		Details: map[string]string{
			"shared_object_id": "shared-1",
			"scanned":          "3",
			"updated":          "1",
		},
	})
	if err != nil {
		t.Fatalf("PutAdminAuditEvent(repair) error = %v", err)
	}
	if second.PreviousHash == "" || second.PreviousHash != first.EventHash {
		t.Fatalf("second previous hash = %q, want first hash %q", second.PreviousHash, first.EventHash)
	}

	batch, err := repo.PutAdminAuditEvents(ctx, meta.PutAdminAuditEventsRequest{
		Events: []meta.PutAdminAuditEventRequest{
			{Action: model.AuditActionGetObject, BucketID: "bucket-1", Key: "object-a", VersionID: "version-1"},
			{Action: model.AuditActionHeadObject, BucketID: "bucket-1", Key: "object-a", VersionID: "version-1"},
		},
	})
	if err != nil {
		t.Fatalf("PutAdminAuditEvents() error = %v", err)
	}
	if len(batch) != 2 || batch[0].PreviousHash != second.EventHash || batch[1].PreviousHash != batch[0].EventHash {
		t.Fatalf("batched audit chain = %+v second=%+v", batch, second)
	}
	if batch[0].EventID >= batch[1].EventID {
		t.Fatalf("batched audit ids not increasing: %+v", batch)
	}

	events, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{
		Action: model.AuditActionDedupeAck,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(dedupe ack) error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("dedupe ack audit events = %d, want 1: %+v", len(events), events)
	}
	if events[0].Details["operation_id"] != "dedupe-00000000000000000001" {
		t.Fatalf("dedupe ack audit details were mutated: %+v", events[0].Details)
	}
}

func testOperationalMetadataImport(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	base := time.Date(2026, 7, 6, 8, 0, 0, 0, time.UTC)
	segmentRef := storage.SegmentRef{
		SegmentID:      "segment-restored",
		SharedObjectID: "shared-restored",
		SizeBytes:      4096,
		Digest: storage.Digest{
			Algorithm: "sha256",
			Hex:       "restored-digest",
		},
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
			Parameters: map[string]string{
				"redundancy": "replicated",
			},
		},
		CreatedAt: base,
	}
	req := meta.ImportOperationalMetadataRequest{
		MetadataSchema: &model.MetadataSchemaRecord{
			SchemaVersion:    meta.CurrentMetadataSchemaVersion,
			MinReaderVersion: meta.MinimumMetadataReaderVersion,
			MinWriterVersion: meta.MinimumMetadataWriterVersion,
			UpdatedBy:        "restore-suite",
			CreatedAt:        base.Add(-2 * time.Hour),
			UpdatedAt:        base.Add(-time.Hour),
		},
		MetadataMigrationOperations: []model.MetadataMigrationOperationRecord{{
			OperationID:         "metadata-migration-00000000000000000007",
			TargetSchemaVersion: meta.CurrentMetadataSchemaVersion,
			Status:              model.MetadataMigrationOperationSucceeded,
			Apply:               true,
			OwnerID:             "restore-suite",
			Steps: []model.MetadataMigrationStep{{
				Name:            "list_index_repair",
				Status:          model.MetadataMigrationStepSucceeded,
				RecordsScanned:  2,
				RecordsRepaired: 1,
			}},
			StartedAt:  base.Add(-30 * time.Minute),
			FinishedAt: base.Add(-29 * time.Minute),
			CreatedAt:  base.Add(-28 * time.Minute),
		}},
		KMSKeys: []model.KMSKeyRecord{{
			KeyID:      "kms-restored",
			KeyVersion: "v7",
			State:      model.KMSKeyPendingDeletion,
			CreatedAt:  base.Add(-time.Hour),
			UpdatedAt:  base.Add(-time.Minute),
		}},
		AuditEvents: []model.AuditEvent{{
			EventID:      "audit-00000000000000000007",
			Action:       model.AuditActionAdminMetadataExport,
			RequestID:    "request-restored",
			Reason:       "restore smoke",
			Details:      map[string]string{"source": "backup"},
			PreviousHash: "hash-6",
			EventHash:    "hash-7",
			CreatedAt:    base,
		}},
		GCOperations: []model.GCOperationRecord{{
			OperationID: "gc-00000000000000000007",
			Status:      model.GCOperationRetryPending,
			StartedAt:   base.Add(time.Minute),
			FinishedAt:  base.Add(2 * time.Minute),
			Scanned:     2,
			Deleted:     1,
			Retryable:   1,
			Attempts: []model.GCOperationAttempt{{
				SegmentID:      "segment-retry",
				SharedObjectID: "shared-restored",
				Reason:         storage.DeleteReasonManualGC,
				Status:         model.GCOperationAttemptRetryable,
				Retryable:      true,
				Error:          "storage unavailable",
			}},
			CreatedAt: base.Add(3 * time.Minute),
		}},
		DedupeOperations: []model.DedupeOperationRecord{{
			OperationID: "dedupe-00000000000000000007",
			Status:      model.DedupeOperationSucceeded,
			StartedAt:   base.Add(4 * time.Minute),
			FinishedAt:  base.Add(5 * time.Minute),
			Scanned:     1,
			Acked:       1,
			Attempts: []model.DedupeOperationAttempt{{
				BucketID:         "bucket-restored",
				Key:              "backup.tar",
				SourceVersion:    "version-source",
				CandidateVersion: "version-candidate",
				PlanStatus:       "admitted",
				Status:           model.DedupeOperationAttemptAcked,
				SharedObjectID:   "shared-restored",
				OrphansMarked:    1,
			}},
			CreatedAt: base.Add(6 * time.Minute),
		}},
		SharedObjects: []model.SharedObject{{
			SharedObjectID:     "shared-restored",
			TenantID:           "tenant-restored",
			BucketID:           "bucket-restored",
			Key:                "backup.tar",
			SourceVersionID:    "version-source",
			SizeBytes:          4096,
			Digest:             segmentRef.Digest,
			StorageClass:       segmentRef.StorageClass,
			SegmentRefs:        []storage.SegmentRef{segmentRef},
			RefCount:           2,
			ProtectedRootCount: 1,
			CreatedAt:          base.Add(7 * time.Minute),
			UpdatedAt:          base.Add(8 * time.Minute),
		}},
		SharedObjectReleases: []model.SharedObjectRelease{{
			ReleaseID:      "shared-restored/segment-restored",
			SharedObjectID: "shared-restored",
			SegmentID:      "segment-restored",
			SegmentRef:     segmentRef,
			Reason:         storage.DeleteReasonManualGC,
			Status:         model.SharedObjectReleasePending,
			CreatedAt:      base.Add(9 * time.Minute),
			UpdatedAt:      base.Add(10 * time.Minute),
		}},
		VolumePools: []model.VolumePool{{
			PoolID:          "restore-pool",
			Generation:      3,
			DurabilityClass: "replicated",
			StorageClassIDs: []string{"STANDARD"},
			Members: []model.VolumePoolMember{{
				VolumeID:       "18a00001",
				DataEndpoint:   "sbs-data-a:9444",
				State:          model.VolumePoolStateActive,
				Weight:         1,
				AvailableBytes: 1024,
			}},
			CreatedAt: base.Add(11 * time.Minute),
			UpdatedAt: base.Add(12 * time.Minute),
		}},
		VolumeDrainOperations: []model.VolumeDrainOperationRecord{{
			OperationID:    "drain-00000000000000000007",
			PoolID:         "restore-pool",
			SourceVolumeID: "18a00001",
			TargetVolumeID: "18a00002",
			OwnerID:        "restore-suite",
			Status:         model.VolumeDrainOperationSucceeded,
			StartedAt:      base.Add(13 * time.Minute),
			FinishedAt:     base.Add(14 * time.Minute),
			Scanned:        1,
			Copied:         1,
			CreatedAt:      base.Add(15 * time.Minute),
		}},
		WorkerLeases: []model.WorkerLease{{
			LeaseID:    "gc/shard-restored",
			WorkerKind: "gc",
			ShardID:    "shard-restored",
			OwnerID:    "restore-worker",
			Generation: 4,
			Cursor:     "cursor-restored",
			AcquiredAt: base.Add(16 * time.Minute),
			UpdatedAt:  base.Add(17 * time.Minute),
			ExpiresAt:  base.Add(18 * time.Minute),
		}},
		WorkerOperations: []model.WorkerOperationRecord{{
			OperationID: "worker-op-00000000000000000007",
			WorkerKind:  "gc",
			ShardID:     "shard-restored",
			OwnerID:     "restore-worker",
			LeaseID:     "gc/shard-restored",
			Status:      model.WorkerOperationSucceeded,
			Cursor:      "cursor-restored",
			Scanned:     2,
			Processed:   2,
			StartedAt:   base.Add(19 * time.Minute),
			FinishedAt:  base.Add(20 * time.Minute),
			CreatedAt:   base.Add(21 * time.Minute),
		}},
		RequireEmptyTarget: true,
	}
	result, err := repo.ImportOperationalMetadata(ctx, req)
	if err != nil {
		t.Fatalf("ImportOperationalMetadata() error = %v", err)
	}
	if result.MetadataSchema != 1 || result.MetadataMigrationOperations != 1 || result.KMSKeys != 1 || result.AuditEvents != 1 || result.GCOperations != 1 || result.DedupeOperations != 1 || result.SharedObjects != 1 || result.SharedObjectReleases != 1 || result.VolumePools != 1 || result.VolumeDrainOperations != 1 || result.WorkerLeases != 1 || result.WorkerOperations != 1 {
		t.Fatalf("ImportOperationalMetadata() result = %+v", result)
	}
	req.AuditEvents[0].Details["source"] = "mutated"
	req.SharedObjects[0].SegmentRefs[0].StorageClass.Parameters["redundancy"] = "mutated"
	req.SharedObjectReleases[0].SegmentRef.StorageClass.Parameters["redundancy"] = "mutated-release"

	events, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].EventID != "audit-00000000000000000007" || events[0].EventHash != "hash-7" || events[0].Details["source"] != "backup" {
		t.Fatalf("imported audit events = %+v", events)
	}
	nextAudit, err := repo.PutAdminAuditEvent(ctx, meta.PutAdminAuditEventRequest{Action: model.AuditActionAdminMetadataImport})
	if err != nil {
		t.Fatalf("PutAdminAuditEvent(after import) error = %v", err)
	}
	if nextAudit.EventID != "audit-00000000000000000008" || nextAudit.PreviousHash != "hash-7" {
		t.Fatalf("next audit event = %+v, want sequence 8 and previous hash-7", nextAudit)
	}

	keys, err := repo.ListKMSKeys(ctx, meta.ListKMSKeysRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListKMSKeys() error = %v", err)
	}
	if len(keys) != 1 || keys[0].KeyID != "kms-restored" || keys[0].KeyVersion != "v7" || keys[0].State != model.KMSKeyPendingDeletion || !keys[0].CreatedAt.Equal(base.Add(-time.Hour)) {
		t.Fatalf("imported kms keys = %+v", keys)
	}

	gcRecords, err := repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCOperations() error = %v", err)
	}
	if len(gcRecords) != 1 || gcRecords[0].OperationID != "gc-00000000000000000007" || gcRecords[0].Attempts[0].SegmentID != "segment-retry" {
		t.Fatalf("imported gc records = %+v", gcRecords)
	}
	nextGC, err := repo.PutGCOperation(ctx, meta.PutGCOperationRequest{Status: model.GCOperationSucceeded})
	if err != nil {
		t.Fatalf("PutGCOperation(after import) error = %v", err)
	}
	if nextGC.OperationID != "gc-00000000000000000008" {
		t.Fatalf("next gc operation id = %q, want sequence 8", nextGC.OperationID)
	}

	dedupeRecords, err := repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListDedupeOperations() error = %v", err)
	}
	if len(dedupeRecords) != 1 || dedupeRecords[0].OperationID != "dedupe-00000000000000000007" || dedupeRecords[0].Attempts[0].CandidateVersion != "version-candidate" {
		t.Fatalf("imported dedupe records = %+v", dedupeRecords)
	}
	nextDedupe, err := repo.PutDedupeOperation(ctx, meta.PutDedupeOperationRequest{Status: model.DedupeOperationSucceeded})
	if err != nil {
		t.Fatalf("PutDedupeOperation(after import) error = %v", err)
	}
	if nextDedupe.OperationID != "dedupe-00000000000000000008" {
		t.Fatalf("next dedupe operation id = %q, want sequence 8", nextDedupe.OperationID)
	}

	shared, err := repo.GetSharedObject(ctx, "shared-restored")
	if err != nil {
		t.Fatalf("GetSharedObject() error = %v", err)
	}
	if shared.RefCount != 2 || shared.ProtectedRootCount != 1 || shared.SegmentRefs[0].StorageClass.Parameters["redundancy"] != "replicated" {
		t.Fatalf("imported shared object = %+v", shared)
	}
	releases, err := repo.ListSharedObjectReleases(ctx, meta.ListSharedObjectReleasesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListSharedObjectReleases() error = %v", err)
	}
	if len(releases) != 1 || releases[0].ReleaseID != "shared-restored/segment-restored" || releases[0].SegmentRef.StorageClass.Parameters["redundancy"] != "replicated" {
		t.Fatalf("imported shared object releases = %+v", releases)
	}

	schema, err := repo.GetMetadataSchema(ctx)
	if err != nil {
		t.Fatalf("GetMetadataSchema() error = %v", err)
	}
	if schema.UpdatedBy != "restore-suite" || schema.SchemaVersion != meta.CurrentMetadataSchemaVersion {
		t.Fatalf("imported metadata schema = %+v", schema)
	}
	migrations, err := repo.ListMetadataMigrationOperations(ctx, meta.ListMetadataMigrationOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListMetadataMigrationOperations() error = %v", err)
	}
	if len(migrations) != 1 || migrations[0].OperationID != "metadata-migration-00000000000000000007" || migrations[0].Steps[0].RecordsRepaired != 1 {
		t.Fatalf("imported metadata migrations = %+v", migrations)
	}
	nextMigration, err := repo.PutMetadataMigrationOperation(ctx, meta.PutMetadataMigrationOperationRequest{Apply: true})
	if err != nil {
		t.Fatalf("PutMetadataMigrationOperation(after import) error = %v", err)
	}
	if nextMigration.OperationID != "metadata-migration-00000000000000000008" {
		t.Fatalf("next metadata migration operation id = %q, want sequence 8", nextMigration.OperationID)
	}

	pools, err := repo.ListVolumePools(ctx, meta.ListVolumePoolsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListVolumePools() error = %v", err)
	}
	if len(pools) != 1 || pools[0].PoolID != "restore-pool" || pools[0].Members[0].DataEndpoint != "sbs-data-a:9444" {
		t.Fatalf("imported volume pools = %+v", pools)
	}
	drainOps, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListVolumeDrainOperations() error = %v", err)
	}
	if len(drainOps) != 1 || drainOps[0].OperationID != "drain-00000000000000000007" || drainOps[0].SourceVolumeID != "18a00001" {
		t.Fatalf("imported volume drain operations = %+v", drainOps)
	}
	nextDrain, err := repo.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{
		SourceVolumeID: "18a00003",
	})
	if err != nil {
		t.Fatalf("PutVolumeDrainOperation(after import) error = %v", err)
	}
	if nextDrain.OperationID != "drain-00000000000000000008" {
		t.Fatalf("next drain operation id = %q, want sequence 8", nextDrain.OperationID)
	}
	leases, err := repo.ListWorkerLeases(ctx, meta.ListWorkerLeasesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerLeases() error = %v", err)
	}
	if len(leases) != 1 || leases[0].LeaseID != "gc/shard-restored" || leases[0].OwnerID != "restore-worker" {
		t.Fatalf("imported worker leases = %+v", leases)
	}
	workerOps, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations() error = %v", err)
	}
	if len(workerOps) != 1 || workerOps[0].OperationID != "worker-op-00000000000000000007" || workerOps[0].Processed != 2 {
		t.Fatalf("imported worker operations = %+v", workerOps)
	}
	nextWorkerOp, err := repo.PutWorkerOperation(ctx, meta.PutWorkerOperationRequest{
		WorkerKind: "gc",
		ShardID:    "shard-restored",
		OwnerID:    "restore-worker",
	})
	if err != nil {
		t.Fatalf("PutWorkerOperation(after import) error = %v", err)
	}
	if nextWorkerOp.OperationID != "worker-op-00000000000000000008" {
		t.Fatalf("next worker operation id = %q, want sequence 8", nextWorkerOp.OperationID)
	}

	if _, err := repo.ImportOperationalMetadata(ctx, meta.ImportOperationalMetadataRequest{RequireEmptyTarget: true}); !errors.Is(err, meta.ErrAlreadyExists) {
		t.Fatalf("ImportOperationalMetadata(require empty on non-empty target) error = %v, want ErrAlreadyExists", err)
	}
	if _, err := repo.ImportOperationalMetadata(ctx, meta.ImportOperationalMetadataRequest{
		KMSKeys: []model.KMSKeyRecord{{
			KeyID: "kms-restored",
		}},
	}); !errors.Is(err, meta.ErrAlreadyExists) {
		t.Fatalf("ImportOperationalMetadata(duplicate kms key) error = %v, want ErrAlreadyExists", err)
	}
	if _, err := repo.ImportOperationalMetadata(ctx, meta.ImportOperationalMetadataRequest{
		AuditEvents: []model.AuditEvent{{
			EventID:   "audit-00000000000000000007",
			EventHash: "hash-7",
		}},
	}); !errors.Is(err, meta.ErrAlreadyExists) {
		t.Fatalf("ImportOperationalMetadata(duplicate id) error = %v, want ErrAlreadyExists", err)
	}
	if _, err := repo.ImportOperationalMetadata(ctx, meta.ImportOperationalMetadataRequest{
		WorkerLeases: []model.WorkerLease{{
			LeaseID:    "gc/wrong-shard",
			WorkerKind: "gc",
			ShardID:    "shard-restored",
			OwnerID:    "restore-worker",
		}},
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("ImportOperationalMetadata(mismatched worker lease id) error = %v, want ErrInvalidArgument", err)
	}
}

func testGCOperationRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	startedAt := time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC)
	first, err := repo.PutGCOperation(ctx, meta.PutGCOperationRequest{
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(2 * time.Second),
		Scanned:    2,
		Deleted:    1,
		Skipped:    0,
		Retryable:  1,
		Attempts: []model.GCOperationAttempt{
			{
				SegmentID: "segment-deleted",
				Reason:    storage.DeleteReasonPublishFailed,
				Status:    model.GCOperationAttemptDeleted,
			},
			{
				SegmentID: "segment-retry",
				Reason:    storage.DeleteReasonMultipartAborted,
				Status:    model.GCOperationAttemptRetryable,
				Retryable: true,
				Error:     "storage unavailable",
			},
		},
	})
	if err != nil {
		t.Fatalf("PutGCOperation(first) error = %v", err)
	}
	if first.OperationID == "" || first.CreatedAt.IsZero() {
		t.Fatalf("first GC operation missing ids/timestamps: %+v", first)
	}
	first.Attempts[0].SegmentID = "mutated"
	second, err := repo.PutGCOperation(ctx, meta.PutGCOperationRequest{
		ResumeOfOperationID: first.OperationID,
		Status:              model.GCOperationSucceeded,
		StartedAt:           startedAt.Add(time.Minute),
		FinishedAt:          startedAt.Add(time.Minute + time.Second),
		Scanned:             1,
		Skipped:             1,
		Attempts: []model.GCOperationAttempt{{
			SegmentID: "segment-protected",
			Reason:    storage.DeleteReasonManualGC,
			Status:    model.GCOperationAttemptSkipped,
			Error:     "segment protected",
		}},
	})
	if err != nil {
		t.Fatalf("PutGCOperation(second) error = %v", err)
	}
	records, err := repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCOperations() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("GC operation record len = %d, want 2: %+v", len(records), records)
	}
	if records[0].OperationID != second.OperationID || records[1].OperationID != first.OperationID {
		t.Fatalf("GC operation order = %+v, want newest first", records)
	}
	if records[1].Status != model.GCOperationRetryPending {
		t.Fatalf("first GC operation status = %q, want retry_pending", records[1].Status)
	}
	if records[0].Status != model.GCOperationSucceeded || records[0].ResumeOfOperationID != first.OperationID {
		t.Fatalf("second GC operation resume/status = %+v, want resume of %q", records[0], first.OperationID)
	}
	if records[1].Attempts[0].SegmentID != "segment-deleted" {
		t.Fatalf("GC operation attempts were mutated: %+v", records[1].Attempts)
	}
	limited, err := repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListGCOperations(limit) error = %v", err)
	}
	if len(limited) != 1 || limited[0].OperationID != second.OperationID {
		t.Fatalf("limited GC operations = %+v, want newest only", limited)
	}
}

func testGCCandidateRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	refA := storage.SegmentRef{
		SegmentID: "segment-a",
		SizeBytes: 4096,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Parameters: map[string]string{
				"redundancy": "replicated",
			},
		},
	}
	first, err := repo.PutGCCandidate(ctx, meta.PutGCCandidateRequest{
		SegmentRef: refA,
		Reason:     storage.DeleteReasonPublishFailed,
	})
	if err != nil {
		t.Fatalf("PutGCCandidate(first) error = %v", err)
	}
	if first.SegmentID != "segment-a" || first.Reason != storage.DeleteReasonPublishFailed || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("first candidate = %+v", first)
	}
	first.SegmentRef.StorageClass.Parameters["redundancy"] = "mutated"
	refA.SizeBytes = 8192
	second, err := repo.PutGCCandidate(ctx, meta.PutGCCandidateRequest{
		SegmentRef: refA,
		Reason:     storage.DeleteReasonMultipartAborted,
	})
	if err != nil {
		t.Fatalf("PutGCCandidate(update) error = %v", err)
	}
	if second.CreatedAt.IsZero() || second.UpdatedAt.Before(second.CreatedAt) || second.Reason != storage.DeleteReasonMultipartAborted || second.SegmentRef.SizeBytes != 8192 {
		t.Fatalf("updated candidate = %+v", second)
	}
	if _, err := repo.PutGCCandidate(ctx, meta.PutGCCandidateRequest{
		SegmentRef: storage.SegmentRef{SegmentID: "segment-b", SizeBytes: 1},
		Reason:     storage.DeleteReasonManualGC,
	}); err != nil {
		t.Fatalf("PutGCCandidate(second segment) error = %v", err)
	}
	candidates, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 2 || candidates[0].SegmentID != "segment-a" || candidates[1].SegmentID != "segment-b" {
		t.Fatalf("candidates = %+v, want segment-a then segment-b", candidates)
	}
	if candidates[0].SegmentRef.StorageClass.Parameters["redundancy"] != "replicated" {
		t.Fatalf("candidate segment ref was mutated: %+v", candidates[0].SegmentRef.StorageClass.Parameters)
	}
	limited, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListGCCandidates(limit) error = %v", err)
	}
	if len(limited) != 1 || limited[0].SegmentID != "segment-a" {
		t.Fatalf("limited candidates = %+v, want segment-a", limited)
	}
	if err := repo.DeleteGCCandidate(ctx, "segment-a"); err != nil {
		t.Fatalf("DeleteGCCandidate(segment-a) error = %v", err)
	}
	candidates, err = repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates(after delete) error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].SegmentID != "segment-b" {
		t.Fatalf("candidates after delete = %+v, want only segment-b", candidates)
	}
	if err := repo.DeleteGCCandidate(ctx, "segment-a"); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("DeleteGCCandidate(missing) error = %v, want not found", err)
	}
}

func testDedupeOperationRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	startedAt := time.Date(2026, 7, 5, 11, 0, 0, 0, time.UTC)
	first, err := repo.PutDedupeOperation(ctx, meta.PutDedupeOperationRequest{
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(3 * time.Second),
		Scanned:    2,
		Acked:      1,
		Skipped:    0,
		Retryable:  1,
		Attempts: []model.DedupeOperationAttempt{
			{
				BucketID:         "bucket-1",
				Key:              "backup.tar",
				SourceVersion:    "version-source",
				CandidateVersion: "version-acked",
				PlanStatus:       "admitted",
				Status:           model.DedupeOperationAttemptAcked,
				SharedObjectID:   "shared-1",
				OrphansMarked:    1,
			},
			{
				BucketID:         "bucket-1",
				Key:              "backup.tar",
				SourceVersion:    "version-source",
				CandidateVersion: "version-retry",
				PlanStatus:       "admitted",
				Status:           model.DedupeOperationAttemptRetryable,
				Retryable:        true,
				Error:            "storage unavailable",
			},
		},
	})
	if err != nil {
		t.Fatalf("PutDedupeOperation(first) error = %v", err)
	}
	if first.OperationID == "" || first.CreatedAt.IsZero() {
		t.Fatalf("first dedupe operation missing ids/timestamps: %+v", first)
	}
	first.Attempts[0].CandidateVersion = "mutated"
	second, err := repo.PutDedupeOperation(ctx, meta.PutDedupeOperationRequest{
		ResumeOfOperationID: first.OperationID,
		Status:              model.DedupeOperationSucceeded,
		StartedAt:           startedAt.Add(time.Minute),
		FinishedAt:          startedAt.Add(time.Minute + time.Second),
		Scanned:             1,
		Skipped:             1,
		Attempts: []model.DedupeOperationAttempt{{
			BucketID:         "bucket-1",
			Key:              "backup.tar",
			SourceVersion:    "version-source",
			CandidateVersion: "version-skipped",
			PlanStatus:       "rejected",
			PlanReason:       "byte_verification_required",
			Status:           model.DedupeOperationAttemptSkipped,
			Error:            "byte_verification_required",
		}},
	})
	if err != nil {
		t.Fatalf("PutDedupeOperation(second) error = %v", err)
	}
	records, err := repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListDedupeOperations() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("dedupe operation record len = %d, want 2: %+v", len(records), records)
	}
	if records[0].OperationID != second.OperationID || records[1].OperationID != first.OperationID {
		t.Fatalf("dedupe operation order = %+v, want newest first", records)
	}
	if records[1].Status != model.DedupeOperationRetryPending {
		t.Fatalf("first dedupe operation status = %q, want retry_pending", records[1].Status)
	}
	if records[0].Status != model.DedupeOperationSucceeded || records[0].ResumeOfOperationID != first.OperationID {
		t.Fatalf("second dedupe operation resume/status = %+v, want resume of %q", records[0], first.OperationID)
	}
	if records[1].Attempts[0].CandidateVersion != "version-acked" {
		t.Fatalf("dedupe operation attempts were mutated: %+v", records[1].Attempts)
	}
	limited, err := repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListDedupeOperations(limit) error = %v", err)
	}
	if len(limited) != 1 || limited[0].OperationID != second.OperationID {
		t.Fatalf("limited dedupe operations = %+v, want newest only", limited)
	}
}

func testDedupeOperationLocks(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	first, err := repo.AcquireDedupeOperationLock(ctx, meta.AcquireDedupeOperationLockRequest{
		LockID:  "dedupe-background",
		OwnerID: "gateway-a",
		TTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireDedupeOperationLock(first) error = %v", err)
	}
	if first.LockID != "dedupe-background" || first.OwnerID != "gateway-a" {
		t.Fatalf("first lock identity = %+v", first)
	}
	if first.AcquiredAt.IsZero() || first.UpdatedAt.IsZero() || first.ExpiresAt.IsZero() {
		t.Fatalf("first lock missing timestamps: %+v", first)
	}
	if !first.ExpiresAt.After(first.UpdatedAt) {
		t.Fatalf("first lock expiry = %s, want after update %s", first.ExpiresAt, first.UpdatedAt)
	}

	renewed, err := repo.AcquireDedupeOperationLock(ctx, meta.AcquireDedupeOperationLockRequest{
		LockID:  "dedupe-background",
		OwnerID: "gateway-a",
		TTL:     2 * time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireDedupeOperationLock(renew) error = %v", err)
	}
	if renewed.OwnerID != "gateway-a" || renewed.AcquiredAt.IsZero() || renewed.UpdatedAt.Before(first.UpdatedAt) {
		t.Fatalf("renewed lock = %+v, first = %+v", renewed, first)
	}

	_, err = repo.AcquireDedupeOperationLock(ctx, meta.AcquireDedupeOperationLockRequest{
		LockID:  "dedupe-background",
		OwnerID: "gateway-b",
		TTL:     time.Minute,
	})
	if !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("AcquireDedupeOperationLock(contender) error = %v, want CAS conflict", err)
	}

	if err := repo.ReleaseDedupeOperationLock(ctx, meta.ReleaseDedupeOperationLockRequest{
		LockID:  "dedupe-background",
		OwnerID: "gateway-b",
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("ReleaseDedupeOperationLock(wrong owner) error = %v, want CAS conflict", err)
	}

	if err := repo.ReleaseDedupeOperationLock(ctx, meta.ReleaseDedupeOperationLockRequest{
		LockID:  "dedupe-background",
		OwnerID: "gateway-a",
	}); err != nil {
		t.Fatalf("ReleaseDedupeOperationLock(owner) error = %v", err)
	}

	secondOwner, err := repo.AcquireDedupeOperationLock(ctx, meta.AcquireDedupeOperationLockRequest{
		LockID:  "dedupe-background",
		OwnerID: "gateway-b",
		TTL:     time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireDedupeOperationLock(after release) error = %v", err)
	}
	if secondOwner.OwnerID != "gateway-b" {
		t.Fatalf("second owner lock = %+v", secondOwner)
	}
}

func testVolumePoolRegistry(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	if _, err := repo.GetVolumePool(ctx, "object-pool"); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetVolumePool(missing) error = %v, want ErrNotFound", err)
	}
	observedAt := time.Date(2026, 8, 10, 3, 30, 0, 0, time.UTC)

	first, err := repo.PutVolumePool(ctx, meta.PutVolumePoolRequest{
		PoolID:          "object-pool",
		Generation:      7,
		DurabilityClass: "replicated",
		StorageClassIDs: []string{"STANDARD", "STANDARD", "ARCHIVE"},
		Members: []model.VolumePoolMember{
			{
				VolumeID:             "18a00001",
				DataEndpoint:         "sbs-data-a:9460",
				State:                model.VolumePoolStateActive,
				Weight:               2,
				AvailableBytes:       1024,
				UsedPercent:          40,
				HighWatermarkPercent: 90,
				LastObservedAt:       observedAt,
			},
			{
				VolumeID:     "18a00002",
				DataEndpoint: "sbs-data-a:9460",
				ReadOnly:     true,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutVolumePool(first) error = %v", err)
	}
	if first.Generation != 7 || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("first pool version/timestamps = %+v", first)
	}
	if !reflect.DeepEqual(first.StorageClassIDs, []string{"STANDARD", "ARCHIVE"}) {
		t.Fatalf("storage class ids = %v, want deduplicated STANDARD/ARCHIVE", first.StorageClassIDs)
	}
	if len(first.Members) != 2 || first.Members[0].State != model.VolumePoolStateActive || first.Members[0].Weight != 2 || first.Members[1].State != model.VolumePoolStateReadOnly || !first.Members[1].ReadOnly {
		t.Fatalf("first members = %+v", first.Members)
	}
	if !first.Members[0].LastObservedAt.Equal(observedAt) {
		t.Fatalf("first member last observed = %s, want %s", first.Members[0].LastObservedAt, observedAt)
	}

	if _, err := repo.PutVolumePool(ctx, meta.PutVolumePoolRequest{
		PoolID:     "object-pool",
		Generation: 7,
		Members:    first.Members,
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("PutVolumePool(stale generation) error = %v, want CAS conflict", err)
	}

	updated, err := repo.PutVolumePool(ctx, meta.PutVolumePoolRequest{
		PoolID:          "object-pool",
		DurabilityClass: "replicated",
		StorageClassIDs: []string{"STANDARD"},
		Members: []model.VolumePoolMember{
			{
				VolumeID:             "18a00001",
				DataEndpoint:         "sbs-data-a:9460",
				State:                model.VolumePoolStateDraining,
				Weight:               1,
				AvailableBytes:       512,
				UsedPercent:          92,
				HighWatermarkPercent: 90,
			},
		},
	})
	if err != nil {
		t.Fatalf("PutVolumePool(auto generation) error = %v", err)
	}
	if updated.Generation != 8 || !updated.CreatedAt.Equal(first.CreatedAt) || updated.UpdatedAt.Before(first.UpdatedAt) {
		t.Fatalf("updated pool generation/timestamps = %+v, first = %+v", updated, first)
	}

	got, err := repo.GetVolumePool(ctx, "object-pool")
	if err != nil {
		t.Fatalf("GetVolumePool() error = %v", err)
	}
	got.Members[0].VolumeID = "mutated"
	again, err := repo.GetVolumePool(ctx, "object-pool")
	if err != nil {
		t.Fatalf("GetVolumePool(again) error = %v", err)
	}
	if again.Members[0].VolumeID != "18a00001" {
		t.Fatalf("volume pool members are mutable through returned value: %+v", again.Members)
	}

	if _, err := repo.PutVolumePool(ctx, meta.PutVolumePoolRequest{
		PoolID: "archive-pool",
		Members: []model.VolumePoolMember{{
			VolumeID:     "18a00003",
			DataEndpoint: "sbs-data-b:9460",
			State:        model.VolumePoolStateActive,
		}},
	}); err != nil {
		t.Fatalf("PutVolumePool(second pool) error = %v", err)
	}
	listed, err := repo.ListVolumePools(ctx, meta.ListVolumePoolsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListVolumePools(limit) error = %v", err)
	}
	if len(listed) != 1 || listed[0].PoolID != "archive-pool" {
		t.Fatalf("listed volume pools = %+v, want archive-pool first", listed)
	}
}

func testVolumeDrainOperationRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	startedAt := time.Date(2026, 8, 10, 6, 10, 0, 0, time.UTC)
	first, err := repo.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{
		PoolID:         "object-pool",
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		OwnerID:        "gateway-a",
		Status:         model.VolumeDrainOperationRetryPending,
		Cursor:         "bucket-1/logs/a.txt#v1",
		StartedAt:      startedAt,
		FinishedAt:     startedAt.Add(time.Second),
		Scanned:        3,
		Copied:         1,
		Skipped:        1,
		Protected:      1,
		Retryable:      1,
		Attempts: []model.VolumeDrainAttempt{
			{
				BucketID:        "bucket-1",
				Key:             "logs/a.txt",
				VersionID:       "v1",
				SourceSegmentID: "seg-source-a",
				SourceRef: storage.SegmentRef{
					SegmentID: "seg-source-a",
					Placement: storage.PlacementSnapshot{Backend: "sbs", Parameters: map[string]string{"volume_id": "18a00001"}},
				},
				TargetSegmentID: "seg-target-a",
				TargetRef: storage.SegmentRef{
					SegmentID: "seg-target-a",
					Placement: storage.PlacementSnapshot{Backend: "sbs", Parameters: map[string]string{"volume_id": "18a00002"}},
				},
				Status: model.VolumeDrainAttemptCopied,
			},
			{
				BucketID:        "bucket-1",
				Key:             "logs/b.txt",
				VersionID:       "v2",
				SourceSegmentID: "seg-source-b",
				Status:          model.VolumeDrainAttemptProtected,
				Protected:       true,
				Error:           "object lock active",
			},
			{
				BucketID:        "bucket-1",
				Key:             "logs/c.txt",
				VersionID:       "v3",
				SourceSegmentID: "seg-source-c",
				Status:          model.VolumeDrainAttemptRetryable,
				Retryable:       true,
				Error:           "target unavailable",
			},
		},
	})
	if err != nil {
		t.Fatalf("PutVolumeDrainOperation(first) error = %v", err)
	}
	if first.OperationID == "" || first.Status != model.VolumeDrainOperationRetryPending || first.Cursor == "" || len(first.Attempts) != 3 {
		t.Fatalf("first drain operation = %+v", first)
	}
	first.Attempts[0].TargetRef.Placement.Parameters["volume_id"] = "mutated"

	second, err := repo.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{
		ResumeOfOperationID: first.OperationID,
		PoolID:              "object-pool",
		SourceVolumeID:      "18a00003",
		TargetVolumeID:      "18a00004",
		OwnerID:             "gateway-b",
		StartedAt:           startedAt.Add(time.Minute),
		FinishedAt:          startedAt.Add(time.Minute + time.Second),
		Scanned:             1,
		Copied:              1,
	})
	if err != nil {
		t.Fatalf("PutVolumeDrainOperation(second) error = %v", err)
	}
	if second.Status != model.VolumeDrainOperationSucceeded || second.ResumeOfOperationID != first.OperationID {
		t.Fatalf("second drain operation = %+v", second)
	}

	records, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListVolumeDrainOperations() error = %v", err)
	}
	if len(records) != 2 || records[0].OperationID != second.OperationID || records[1].OperationID != first.OperationID {
		t.Fatalf("drain records order = %+v", records)
	}
	if records[1].Attempts[0].TargetRef.Placement.Parameters["volume_id"] != "18a00002" {
		t.Fatalf("drain attempt mutated through returned value: %+v", records[1].Attempts[0])
	}

	filtered, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{
		SourceVolumeID: "18a00001",
		Status:         model.VolumeDrainOperationRetryPending,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListVolumeDrainOperations(filter) error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].OperationID != first.OperationID || filtered[0].Protected != 1 || filtered[0].Retryable != 1 {
		t.Fatalf("filtered drain records = %+v", filtered)
	}

	limited, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListVolumeDrainOperations(limit) error = %v", err)
	}
	if len(limited) != 1 || limited[0].OperationID != second.OperationID {
		t.Fatalf("limited drain records = %+v", limited)
	}

	if _, err := repo.PutVolumeDrainOperation(ctx, meta.PutVolumeDrainOperationRequest{}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PutVolumeDrainOperation(missing source) error = %v, want ErrInvalidArgument", err)
	}
}

func testPublishObjectVersionRefs(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-a",
		Name:     "drain-publish",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	sourceRef := testSegmentRefWithVolume("segment-source", "18a00001", 12)
	published, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:    bucket.BucketID,
		Key:         "logs/a.txt",
		SizeBytes:   12,
		ETag:        "etag-a",
		SegmentRefs: []storage.SegmentRef{sourceRef},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}
	targetRef := testSegmentRefWithVolume("segment-target", "18a00002", 12)
	result, err := repo.PublishObjectVersionRefs(ctx, meta.PublishObjectVersionRefsRequest{
		BucketID:               bucket.BucketID,
		Key:                    "logs/a.txt",
		VersionID:              published.Head.VersionID,
		ExpectedSourceVolumeID: "18a00001",
		SegmentRefs:            []storage.SegmentRef{targetRef},
	})
	if err != nil {
		t.Fatalf("PublishObjectVersionRefs() error = %v", err)
	}
	if len(result.PreviousSegmentRefs) != 1 || result.PreviousSegmentRefs[0].SegmentID != sourceRef.SegmentID {
		t.Fatalf("previous refs = %+v, want source", result.PreviousSegmentRefs)
	}
	if len(result.Version.SegmentRefs) != 1 || result.Version.SegmentRefs[0].SegmentID != targetRef.SegmentID {
		t.Fatalf("result version refs = %+v, want target", result.Version.SegmentRefs)
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "logs/a.txt")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if head.VersionID != published.Head.VersionID || len(head.SegmentRefs) != 1 || head.SegmentRefs[0].SegmentID != targetRef.SegmentID || head.SizeBytes != 12 || head.ETag != "etag-a" {
		t.Fatalf("head after publish refs = %+v", head)
	}
	listed, err := repo.ListObjects(ctx, meta.ListObjectsRequest{BucketID: bucket.BucketID, Prefix: "logs/", MaxKeys: 10})
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	if len(listed.Contents) != 1 || listed.Contents[0].VersionID != published.Head.VersionID || listed.Contents[0].SizeBytes != 12 {
		t.Fatalf("list after publish refs = %+v", listed.Contents)
	}
	if _, err := repo.PublishObjectVersionRefs(ctx, meta.PublishObjectVersionRefsRequest{
		BucketID:               bucket.BucketID,
		Key:                    "logs/a.txt",
		VersionID:              published.Head.VersionID,
		ExpectedSourceVolumeID: "18a99999",
		SegmentRefs:            []storage.SegmentRef{targetRef},
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PublishObjectVersionRefs(source mismatch) error = %v, want ErrInvalidArgument", err)
	}
}

func testWorkerLeaseAndOperationRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	first, err := repo.AcquireWorkerLease(ctx, meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-a",
		TTL:        time.Minute,
		Cursor:     "cursor-a",
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(first) error = %v", err)
	}
	if first.LeaseID != "gc/shard-0001" || first.Generation != 1 || first.Cursor != "cursor-a" {
		t.Fatalf("first lease = %+v", first)
	}
	if first.AcquiredAt.IsZero() || first.UpdatedAt.IsZero() || !first.ExpiresAt.After(first.UpdatedAt) {
		t.Fatalf("first lease timestamps = %+v", first)
	}

	renewed, err := repo.AcquireWorkerLease(ctx, meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-a",
		TTL:        2 * time.Minute,
		Cursor:     "cursor-b",
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(renew) error = %v", err)
	}
	if renewed.Generation != first.Generation || renewed.Cursor != "cursor-b" || !renewed.AcquiredAt.Equal(first.AcquiredAt) {
		t.Fatalf("renewed lease = %+v, first = %+v", renewed, first)
	}

	_, err = repo.AcquireWorkerLease(ctx, meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	})
	if !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("AcquireWorkerLease(contender) error = %v, want CAS conflict", err)
	}
	if err := repo.ReleaseWorkerLease(ctx, meta.ReleaseWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-b",
	}); !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("ReleaseWorkerLease(wrong owner) error = %v, want CAS conflict", err)
	}
	if err := repo.ReleaseWorkerLease(ctx, meta.ReleaseWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-a",
	}); err != nil {
		t.Fatalf("ReleaseWorkerLease(owner) error = %v", err)
	}
	secondOwner, err := repo.AcquireWorkerLease(ctx, meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-b",
		TTL:        time.Minute,
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(after release) error = %v", err)
	}
	if secondOwner.OwnerID != "gateway-b" || secondOwner.Generation != 2 {
		t.Fatalf("second owner lease = %+v", secondOwner)
	}
	leases, err := repo.ListWorkerLeases(ctx, meta.ListWorkerLeasesRequest{WorkerKind: "gc", Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerLeases(gc) error = %v", err)
	}
	if len(leases) != 1 || leases[0].LeaseID != secondOwner.LeaseID || leases[0].OwnerID != "gateway-b" {
		t.Fatalf("gc worker leases = %+v, want second owner %s", leases, secondOwner.LeaseID)
	}
	limitedLeases, err := repo.ListWorkerLeases(ctx, meta.ListWorkerLeasesRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListWorkerLeases(limit) error = %v", err)
	}
	if len(limitedLeases) != 1 || limitedLeases[0].LeaseID != secondOwner.LeaseID {
		t.Fatalf("limited worker leases = %+v, want %s", limitedLeases, secondOwner.LeaseID)
	}

	firstOp, err := repo.PutWorkerOperation(ctx, meta.PutWorkerOperationRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		OwnerID:    "gateway-b",
		LeaseID:    secondOwner.LeaseID,
		Status:     model.WorkerOperationRetryPending,
		Cursor:     "cursor-c",
		Scanned:    10,
		Processed:  7,
		Skipped:    1,
		Retryable:  2,
		LastError:  "temporary storage error",
		StartedAt:  time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 7, 17, 8, 0, 1, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PutWorkerOperation(first) error = %v", err)
	}
	secondOp, err := repo.PutWorkerOperation(ctx, meta.PutWorkerOperationRequest{
		WorkerKind: "lifecycle",
		ShardID:    "bucket-a",
		OwnerID:    "gateway-c",
		Status:     model.WorkerOperationSucceeded,
		Processed:  3,
	})
	if err != nil {
		t.Fatalf("PutWorkerOperation(second) error = %v", err)
	}
	records, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{WorkerKind: "gc", Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations(gc) error = %v", err)
	}
	if len(records) != 1 || records[0].OperationID != firstOp.OperationID || records[0].Status != model.WorkerOperationRetryPending || records[0].Cursor != "cursor-c" {
		t.Fatalf("gc worker operations = %+v", records)
	}
	latest, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListWorkerOperations(limit) error = %v", err)
	}
	if len(latest) != 1 || latest[0].OperationID != secondOp.OperationID {
		t.Fatalf("latest worker operation = %+v, want %s", latest, secondOp.OperationID)
	}

	if _, err := repo.GetWorkerControl(ctx, meta.GetWorkerControlRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
	}); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetWorkerControl(missing) error = %v, want ErrNotFound", err)
	}
	paused, err := repo.PutWorkerControl(ctx, meta.PutWorkerControlRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		State:      model.WorkerControlPaused,
		Reason:     "maintenance",
		UpdatedBy:  "operator-a",
	})
	if err != nil {
		t.Fatalf("PutWorkerControl(paused) error = %v", err)
	}
	if paused.State != model.WorkerControlPaused || paused.Reason != "maintenance" || paused.UpdatedBy != "operator-a" || paused.CreatedAt.IsZero() || paused.UpdatedAt.IsZero() {
		t.Fatalf("paused worker control = %+v", paused)
	}
	gotControl, err := repo.GetWorkerControl(ctx, meta.GetWorkerControlRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
	})
	if err != nil {
		t.Fatalf("GetWorkerControl(paused) error = %v", err)
	}
	if gotControl.State != model.WorkerControlPaused || gotControl.Reason != "maintenance" {
		t.Fatalf("got worker control = %+v", gotControl)
	}
	resumed, err := repo.PutWorkerControl(ctx, meta.PutWorkerControlRequest{
		WorkerKind: "gc",
		ShardID:    "shard-0001",
		State:      model.WorkerControlActive,
		Reason:     "resume after maintenance",
		UpdatedBy:  "operator-b",
	})
	if err != nil {
		t.Fatalf("PutWorkerControl(active) error = %v", err)
	}
	if resumed.State != model.WorkerControlActive || resumed.CreatedAt != paused.CreatedAt || resumed.UpdatedBy != "operator-b" {
		t.Fatalf("resumed worker control = %+v, paused created=%s", resumed, paused.CreatedAt)
	}
}

func testSharedObjectReleaseRecords(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	ref := storage.SegmentRef{
		SegmentID:      "segment-shared",
		SharedObjectID: "shared-1",
		SizeBytes:      4096,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Parameters: map[string]string{
				"redundancy": "replicated",
			},
		},
	}
	first, err := repo.PutSharedObjectRelease(ctx, meta.PutSharedObjectReleaseRequest{
		SharedObjectID: "shared-1",
		SegmentRef:     ref,
		Reason:         storage.DeleteReasonManualGC,
	})
	if err != nil {
		t.Fatalf("PutSharedObjectRelease(first) error = %v", err)
	}
	if first.ReleaseID == "" || first.Status != model.SharedObjectReleasePending || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
		t.Fatalf("first release = %+v", first)
	}
	first.SegmentRef.StorageClass.Parameters["redundancy"] = "mutated"
	ref.SizeBytes = 8192
	second, err := repo.PutSharedObjectRelease(ctx, meta.PutSharedObjectReleaseRequest{
		SharedObjectID: "shared-1",
		SegmentRef:     ref,
		Reason:         storage.DeleteReasonMultipartAborted,
	})
	if err != nil {
		t.Fatalf("PutSharedObjectRelease(second) error = %v", err)
	}
	if second.ReleaseID != first.ReleaseID || second.CreatedAt.IsZero() || second.CreatedAt.After(second.UpdatedAt) {
		t.Fatalf("second release identity/timestamps = %+v first %+v", second, first)
	}
	if second.Reason != storage.DeleteReasonMultipartAborted || second.SegmentRef.SizeBytes != 8192 {
		t.Fatalf("second release content = %+v", second)
	}
	releases, err := repo.ListSharedObjectReleases(ctx, meta.ListSharedObjectReleasesRequest{
		SharedObjectID: "shared-1",
		Status:         model.SharedObjectReleasePending,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListSharedObjectReleases() error = %v", err)
	}
	if len(releases) != 1 || releases[0].ReleaseID != first.ReleaseID {
		t.Fatalf("shared releases = %+v, want one release %q", releases, first.ReleaseID)
	}
	if releases[0].SegmentRef.StorageClass.Parameters["redundancy"] != "replicated" {
		t.Fatalf("shared release segment ref was mutated: %+v", releases[0].SegmentRef.StorageClass.Parameters)
	}
	limited, err := repo.ListSharedObjectReleases(ctx, meta.ListSharedObjectReleasesRequest{Limit: 1})
	if err != nil {
		t.Fatalf("ListSharedObjectReleases(limit) error = %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("limited shared releases = %+v, want one", limited)
	}
}

func testSharedObjectPublish(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	version := putObjectVersionWithDigest(t, repo, bucket.BucketID, "backup.tar", "segment-shared-source", storage.Digest{
		Algorithm: "sha256",
		Hex:       "abc123",
	})
	shared, err := repo.PublishSharedObject(ctx, meta.PublishSharedObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       version.Key,
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("PublishSharedObject() error = %v", err)
	}
	if shared.SharedObjectID == "" || shared.TenantID != bucket.TenantID || shared.BucketID != bucket.BucketID || shared.Key != version.Key {
		t.Fatalf("shared object identity = %+v", shared)
	}
	if shared.SourceVersionID != version.VersionID || shared.RefCount != 1 || shared.SizeBytes != version.SizeBytes {
		t.Fatalf("shared object source/refcount/size = %+v version %+v", shared, version)
	}
	if shared.Digest.Algorithm != "sha256" || shared.Digest.Hex != "abc123" {
		t.Fatalf("shared object digest = %+v", shared.Digest)
	}
	if len(shared.SegmentRefs) != 1 || shared.SegmentRefs[0].SegmentID != "segment-shared-source" || shared.SegmentRefs[0].SharedObjectID != shared.SharedObjectID {
		t.Fatalf("shared object segment refs = %+v", shared.SegmentRefs)
	}
	if shared.CreatedAt.IsZero() || shared.UpdatedAt.IsZero() {
		t.Fatalf("shared object timestamps = %+v", shared)
	}
	shared.SegmentRefs[0].SegmentID = "mutated"

	again, err := repo.PublishSharedObject(ctx, meta.PublishSharedObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       version.Key,
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("PublishSharedObject(again) error = %v", err)
	}
	if again.SharedObjectID != shared.SharedObjectID || again.SegmentRefs[0].SegmentID != "segment-shared-source" {
		t.Fatalf("idempotent shared object = %+v, want original segment", again)
	}
	got, err := repo.GetSharedObject(ctx, shared.SharedObjectID)
	if err != nil {
		t.Fatalf("GetSharedObject() error = %v", err)
	}
	if got.SharedObjectID != shared.SharedObjectID || got.SegmentRefs[0].SegmentID != "segment-shared-source" {
		t.Fatalf("GetSharedObject() = %+v", got)
	}
	list, err := repo.ListSharedObjects(ctx, meta.ListSharedObjectsRequest{
		TenantID: bucket.TenantID,
		BucketID: bucket.BucketID,
		Key:      version.Key,
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("ListSharedObjects() error = %v", err)
	}
	if len(list) != 1 || list[0].SharedObjectID != shared.SharedObjectID {
		t.Fatalf("ListSharedObjects() = %+v, want one shared object", list)
	}
	refs, err := repo.ListSharedObjectRefs(ctx, meta.ListSharedObjectRefsRequest{
		SharedObjectID: shared.SharedObjectID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListSharedObjectRefs(source) error = %v", err)
	}
	if len(refs) != 1 || refs[0].VersionID != version.VersionID || refs[0].SegmentRefs[0].SharedObjectID != shared.SharedObjectID {
		t.Fatalf("source shared object refs = %+v", refs)
	}
	if _, err := repo.PublishSharedObject(ctx, meta.PublishSharedObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "missing-digest.txt",
		VersionID: putObjectVersion(t, repo, bucket.BucketID, "missing-digest.txt", "segment-no-digest").VersionID,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("PublishSharedObject(no digest) error = %v, want ErrInvalidArgument", err)
	}
}

func testSharedObjectAttachTransaction(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	digest := storage.Digest{Algorithm: "sha256", Hex: "abc123"}
	source := putObjectVersionWithDigest(t, repo, bucket.BucketID, "backup.tar", "segment-source", digest)
	candidate := putObjectVersionWithDigest(t, repo, bucket.BucketID, "backup.tar", "segment-target", digest)
	shared, err := repo.PublishSharedObject(ctx, meta.PublishSharedObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       source.Key,
		VersionID: source.VersionID,
	})
	if err != nil {
		t.Fatalf("PublishSharedObject() error = %v", err)
	}
	result, err := repo.AttachObjectVersionToSharedObject(ctx, meta.AttachObjectVersionToSharedObjectRequest{
		SharedObjectID: shared.SharedObjectID,
		BucketID:       bucket.BucketID,
		Key:            candidate.Key,
		VersionID:      candidate.VersionID,
	})
	if err != nil {
		t.Fatalf("AttachObjectVersionToSharedObject() error = %v", err)
	}
	if result.SharedObject.RefCount != 2 {
		t.Fatalf("shared refcount = %d, want 2", result.SharedObject.RefCount)
	}
	if len(result.PreviousSegmentRefs) != 1 || result.PreviousSegmentRefs[0].SegmentID != "segment-target" {
		t.Fatalf("previous refs = %+v, want candidate private segment", result.PreviousSegmentRefs)
	}
	if len(result.Version.SegmentRefs) != 1 || result.Version.SegmentRefs[0].SegmentID != "segment-source" || result.Version.SegmentRefs[0].SharedObjectID != shared.SharedObjectID {
		t.Fatalf("attached version segment refs = %+v", result.Version.SegmentRefs)
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "backup.tar")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if head.VersionID != candidate.VersionID || len(head.SegmentRefs) != 1 || head.SegmentRefs[0].SegmentID != "segment-source" || head.SegmentRefs[0].SharedObjectID != shared.SharedObjectID {
		t.Fatalf("head after attach = %+v", head)
	}
	refs, err := repo.ListSharedObjectRefs(ctx, meta.ListSharedObjectRefsRequest{
		SharedObjectID: shared.SharedObjectID,
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListSharedObjectRefs() error = %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("shared refs = %+v, want source and candidate refs", refs)
	}
	again, err := repo.AttachObjectVersionToSharedObject(ctx, meta.AttachObjectVersionToSharedObjectRequest{
		SharedObjectID: shared.SharedObjectID,
		BucketID:       bucket.BucketID,
		Key:            candidate.Key,
		VersionID:      candidate.VersionID,
	})
	if err != nil {
		t.Fatalf("AttachObjectVersionToSharedObject(again) error = %v", err)
	}
	if again.SharedObject.RefCount != 2 {
		t.Fatalf("idempotent refcount = %d, want 2", again.SharedObject.RefCount)
	}
	mismatch := putObjectVersionWithDigest(t, repo, bucket.BucketID, "backup.tar", "segment-mismatch", storage.Digest{Algorithm: "sha256", Hex: "different"})
	if _, err := repo.AttachObjectVersionToSharedObject(ctx, meta.AttachObjectVersionToSharedObjectRequest{
		SharedObjectID: shared.SharedObjectID,
		BucketID:       bucket.BucketID,
		Key:            mismatch.Key,
		VersionID:      mismatch.VersionID,
	}); !errors.Is(err, meta.ErrInvalidArgument) {
		t.Fatalf("AttachObjectVersionToSharedObject(mismatch) error = %v, want ErrInvalidArgument", err)
	}
}

func testSharedObjectProtectedRootAccounting(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.PutBucketObjectLock(ctx, meta.BucketObjectLockRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketObjectLockConfiguration{
			Enabled: true,
		},
	}); err != nil {
		t.Fatalf("PutBucketObjectLock() error = %v", err)
	}
	version := putObjectVersionWithDigestAndLock(t, repo, bucket.BucketID, "locked-backup.tar", "segment-protected-shared", storage.Digest{
		Algorithm: "sha256",
		Hex:       "protectedabc",
	}, model.ObjectLockRetention{
		Mode:            model.ObjectLockModeCompliance,
		RetainUntilDate: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
	}, model.ObjectLockLegalHoldOff)
	protectedRefs, err := repo.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		BucketID:   bucket.BucketID,
		Key:        version.Key,
		VersionID:  version.VersionID,
		ActiveOnly: true,
	})
	if err != nil {
		t.Fatalf("ListProtectedRefs() error = %v", err)
	}
	if len(protectedRefs) != 1 {
		t.Fatalf("protected refs = %+v, want one", protectedRefs)
	}
	shared, err := repo.PublishSharedObject(ctx, meta.PublishSharedObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       version.Key,
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("PublishSharedObject() error = %v", err)
	}
	if shared.ProtectedRootCount != 1 {
		t.Fatalf("ProtectedRootCount = %d, want 1: %+v", shared.ProtectedRootCount, shared)
	}
	got, err := repo.GetSharedObject(ctx, shared.SharedObjectID)
	if err != nil {
		t.Fatalf("GetSharedObject() error = %v", err)
	}
	if got.ProtectedRootCount != 1 {
		t.Fatalf("GetSharedObject ProtectedRootCount = %d, want 1", got.ProtectedRootCount)
	}
}

func testSharedObjectRefCountRepair(t *testing.T, repo RepositoryUnderTest) {
	ctx := t.Context()
	bucket := mustCreateBucket(t, repo)
	if _, err := repo.PutBucketObjectLock(ctx, meta.BucketObjectLockRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketObjectLockConfiguration{
			Enabled: true,
		},
	}); err != nil {
		t.Fatalf("PutBucketObjectLock() error = %v", err)
	}
	version := putObjectVersionWithDigest(t, repo, bucket.BucketID, "repair-backup.tar", "segment-repair-shared", storage.Digest{
		Algorithm: "sha256",
		Hex:       "repairabc",
	})
	shared, err := repo.PublishSharedObject(ctx, meta.PublishSharedObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       version.Key,
		VersionID: version.VersionID,
	})
	if err != nil {
		t.Fatalf("PublishSharedObject() error = %v", err)
	}
	if shared.RefCount != 1 || shared.ProtectedRootCount != 0 {
		t.Fatalf("initial shared object = %+v, want refcount 1 protected roots 0", shared)
	}
	if err := repo.PutObjectLegalHold(ctx, meta.ObjectLegalHoldRequest{
		BucketID:  bucket.BucketID,
		Key:       version.Key,
		VersionID: version.VersionID,
		LegalHold: model.ObjectLockLegalHoldOn,
	}); err != nil {
		t.Fatalf("PutObjectLegalHold() error = %v", err)
	}
	repair, err := repo.RepairSharedObjectRefCounts(ctx, meta.RepairSharedObjectRefCountsRequest{
		SharedObjectID: shared.SharedObjectID,
	})
	if err != nil {
		t.Fatalf("RepairSharedObjectRefCounts() error = %v", err)
	}
	if repair.Scanned != 1 || repair.Updated != 1 {
		t.Fatalf("repair result = %+v, want scanned 1 updated 1", repair)
	}
	repaired, err := repo.GetSharedObject(ctx, shared.SharedObjectID)
	if err != nil {
		t.Fatalf("GetSharedObject(repaired) error = %v", err)
	}
	if repaired.RefCount != 1 || repaired.ProtectedRootCount != 1 {
		t.Fatalf("repaired shared object = %+v, want refcount 1 protected roots 1", repaired)
	}
	again, err := repo.RepairSharedObjectRefCounts(ctx, meta.RepairSharedObjectRefCountsRequest{
		SharedObjectID: shared.SharedObjectID,
	})
	if err != nil {
		t.Fatalf("RepairSharedObjectRefCounts(again) error = %v", err)
	}
	if again.Scanned != 1 || again.Updated != 0 {
		t.Fatalf("second repair = %+v, want scanned 1 updated 0", again)
	}
}

func mustCreateBucket(t *testing.T, repo RepositoryUnderTest) model.Bucket {
	t.Helper()
	return mustCreateBucketNamed(t, repo, "bucket-"+t.Name())
}

func mustCreateBucketNamed(t *testing.T, repo RepositoryUnderTest, name string) model.Bucket {
	t.Helper()
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     name,
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	return bucket
}

func multipartPartRequest(bucketID, key, uploadID string, partNumber int, segmentID, etag string) meta.PutMultipartPartRequest {
	return meta.PutMultipartPartRequest{
		BucketID:   bucketID,
		Key:        key,
		UploadID:   uploadID,
		PartNumber: partNumber,
		SizeBytes:  int64(len(segmentID)),
		ETag:       etag,
		SegmentRef: testSegmentRef(segmentID, uint64(len(segmentID))),
	}
}

func testSegmentRef(segmentID string, size uint64) storage.SegmentRef {
	return storage.SegmentRef{
		SegmentID: segmentID,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
		Placement: storage.PlacementSnapshot{
			Backend:           "local",
			Layout:            "local-file",
			RedundancyBackend: "replicated",
			ProfileID:         "STANDARD",
			Parameters: map[string]string{
				"profile_id": "STANDARD",
			},
			Chunks: []storage.PlacementChunk{{
				LogicalOffsetBytes: 0,
				SizeBytes:          size,
				LengthBytes:        size,
				ChunkID:            size,
				Role:               "primary",
			}},
		},
		SizeBytes: size,
	}
}

func testSegmentRefWithVolume(segmentID, volumeID string, size uint64) storage.SegmentRef {
	ref := testSegmentRef(segmentID, size)
	ref.StorageClass.Backend = "sbs"
	ref.Placement.Backend = "sbs"
	if ref.Placement.Parameters == nil {
		ref.Placement.Parameters = make(map[string]string)
	}
	ref.Placement.Parameters["volume_id"] = volumeID
	if len(ref.Placement.Chunks) == 0 {
		ref.Placement.Chunks = []storage.PlacementChunk{{}}
	}
	for i := range ref.Placement.Chunks {
		ref.Placement.Chunks[i].VolumeID = volumeID
		ref.Placement.Chunks[i].SizeBytes = size
		ref.Placement.Chunks[i].LengthBytes = size
	}
	return ref
}

func putObject(t *testing.T, repo RepositoryUnderTest, bucketID, key string) {
	t.Helper()
	putObjectVersion(t, repo, bucketID, key, key)
}

func putObjectVersion(t *testing.T, repo RepositoryUnderTest, bucketID, key, segmentID string) model.ObjectVersion {
	t.Helper()
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:   bucketID,
		Key:        key,
		SizeBytes:  int64(len(segmentID)),
		ETag:       `"` + segmentID + `"`,
		SegmentRef: testSegmentRef(segmentID, uint64(len(segmentID))),
	})
	if err != nil {
		t.Fatalf("BeginPutObject(%q) error = %v", key, err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucketID,
		Key:                   key,
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion(%q) error = %v", key, err)
	}
	version, err := repo.GetObjectVersion(t.Context(), bucketID, key, pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(%q) error = %v", key, err)
	}
	return version
}

func putObjectVersionWithDigest(t *testing.T, repo RepositoryUnderTest, bucketID, key, segmentID string, digest storage.Digest) model.ObjectVersion {
	t.Helper()
	ref := testSegmentRef(segmentID, uint64(len(segmentID)))
	ref.Digest = digest
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:   bucketID,
		Key:        key,
		SizeBytes:  int64(ref.SizeBytes),
		ETag:       `"` + segmentID + `"`,
		SegmentRef: ref,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(%q) error = %v", key, err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucketID,
		Key:                   key,
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion(%q) error = %v", key, err)
	}
	version, err := repo.GetObjectVersion(t.Context(), bucketID, key, pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(%q) error = %v", key, err)
	}
	return version
}

func putObjectVersionWithDigestAndLock(t *testing.T, repo RepositoryUnderTest, bucketID, key, segmentID string, digest storage.Digest, retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus) model.ObjectVersion {
	t.Helper()
	ref := testSegmentRef(segmentID, uint64(len(segmentID)))
	ref.Digest = digest
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:            bucketID,
		Key:                 key,
		SizeBytes:           int64(ref.SizeBytes),
		ETag:                `"` + segmentID + `"`,
		SegmentRef:          ref,
		ObjectLockRetention: retention,
		ObjectLockLegalHold: legalHold,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(%q) error = %v", key, err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucketID,
		Key:                   key,
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion(%q) error = %v", key, err)
	}
	version, err := repo.GetObjectVersion(t.Context(), bucketID, key, pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(%q) error = %v", key, err)
	}
	return version
}

func putObjectVersionWithLock(t *testing.T, repo RepositoryUnderTest, bucketID, key string, retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus) model.ObjectVersion {
	t.Helper()
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:            bucketID,
		Key:                 key,
		SizeBytes:           int64(len(key)),
		ETag:                `"` + key + `"`,
		SegmentRef:          testSegmentRef(key, uint64(len(key))),
		ObjectLockRetention: retention,
		ObjectLockLegalHold: legalHold,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(%q) error = %v", key, err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucketID,
		Key:                   key,
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion(%q) error = %v", key, err)
	}
	version, err := repo.GetObjectVersion(t.Context(), bucketID, key, pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(%q) error = %v", key, err)
	}
	return version
}

func governanceBypassAudit(reason string) meta.AuditContext {
	return meta.AuditContext{
		RequestID: "test-request",
		Reason:    reason,
		Principal: model.AuditPrincipal{
			TenantID:    "root",
			AccessKeyID: "root-access-key",
			DisplayName: "root",
			Root:        true,
		},
	}
}

func objectKeys(heads []model.ObjectHead) []string {
	keys := make([]string, 0, len(heads))
	for _, head := range heads {
		keys = append(keys, head.Key)
	}
	return keys
}

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/encryption"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storage/local"
	sbsegments "github.com/nosway/namros/internal/storage/sbs"
)

func testAdminCommand(stdout, stderr io.Writer) adminCommand {
	return adminCommand{stdout: stdout, stderr: stderr}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func skipPrivateEnterpriseOverlay(t *testing.T) {
	t.Helper()
	t.Skip("Enterprise admin command implementation is provided by the private Enterprise source overlay")
}

func enterpriseOverlayTest() bool {
	return os.Getenv("NAMROS_ENTERPRISE_OVERLAY_TEST") == "1" && edition.Current() == edition.Enterprise
}

func skipEnterpriseOverlayCommunityAssertion(t *testing.T) {
	t.Helper()
	if enterpriseOverlayTest() {
		t.Skip("community edition assertion is not applicable to Enterprise overlay test runs")
	}
}

func TestMetadataScaleBudgetCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-scale-budget",
		"-part-count", "128",
		"-chunks-per-segment", "2",
	})
	if err != nil {
		t.Fatalf("run() error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion     string `json:"schema_version"`
		PartCount         int    `json:"part_count"`
		SegmentRefCount   int    `json:"segment_ref_count"`
		ProtectedRefCount int    `json:"protected_ref_count"`
		GCCandidateCount  int    `json:"gc_candidate_count"`
		Records           []struct {
			Name  string `json:"name"`
			Count int    `json:"count"`
		} `json:"records"`
		CompleteTransaction struct {
			ApproxTotalBytes   int `json:"approx_total_bytes"`
			RecordCountTouched int `json:"record_count_touched"`
		} `json:"complete_transaction"`
		Gates []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"gates"`
		ReleaseGate struct {
			Status string `json:"status"`
		} `json:"release_gate"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != meta.MetadataScaleBudgetSchemaVersion || body.PartCount != 128 || body.SegmentRefCount != 128 {
		t.Fatalf("body = %+v", body)
	}
	if body.ProtectedRefCount != 128 || body.GCCandidateCount != 128 {
		t.Fatalf("protection/GC counts = %+v", body)
	}
	if !metadataScaleBudgetHasRecord(body.Records, "protected_ref_by_version", 128) ||
		!metadataScaleBudgetHasRecord(body.Records, "protected_ref_by_segment", 128) ||
		!metadataScaleBudgetHasRecord(body.Records, "gc_candidate", 128) {
		t.Fatalf("records = %+v", body.Records)
	}
	if body.CompleteTransaction.ApproxTotalBytes <= 0 || body.CompleteTransaction.RecordCountTouched <= 128*2 || len(body.Gates) == 0 {
		t.Fatalf("budget output = %+v", body)
	}
	if body.ReleaseGate.Status == "" {
		t.Fatalf("release gate missing from output: %+v", body)
	}
}

func TestMetadataScaleBudgetReleaseGateCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-scale-budget",
		"-release-gate",
	})
	if err != nil {
		t.Fatalf("run(release-gate) error = %v stderr=%s", err, stderr.String())
	}
	if !strings.Contains(stderr.String(), "watch gates") {
		t.Fatalf("stderr = %q, want watch warning", stderr.String())
	}
	var warningBody struct {
		ReleaseGate struct {
			Status       string   `json:"status"`
			WarningGates []string `json:"warning_gates"`
		} `json:"release_gate"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &warningBody); err != nil {
		t.Fatalf("json decode(warning): %v body=%s", err, stdout.String())
	}
	if warningBody.ReleaseGate.Status != "warning" || len(warningBody.ReleaseGate.WarningGates) == 0 {
		t.Fatalf("release gate = %+v, want warning gates", warningBody.ReleaseGate)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-scale-budget",
		"-fail-on-watch",
	})
	if err == nil {
		t.Fatalf("run(fail-on-watch) error = nil, want failure")
	}
	var failedBody struct {
		ReleaseGate struct {
			Status      string   `json:"status"`
			FailedGates []string `json:"failed_gates"`
		} `json:"release_gate"`
	}
	if decodeErr := json.Unmarshal(stdout.Bytes(), &failedBody); decodeErr != nil {
		t.Fatalf("json decode(failed): %v body=%s", decodeErr, stdout.String())
	}
	if failedBody.ReleaseGate.Status != "failed" || len(failedBody.ReleaseGate.FailedGates) == 0 {
		t.Fatalf("failed release gate = %+v, want failed gates", failedBody.ReleaseGate)
	}
}

func metadataScaleBudgetHasRecord(records []struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}, name string, count int) bool {
	for _, record := range records {
		if record.Name == name && record.Count == count {
			return true
		}
	}
	return false
}

func TestMetadataListIndexRepairCommand(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "repair-cli",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(seed) error = %v", err)
	}
	if _, err := repo.PutObjectVersion(context.Background(), meta.PutObjectVersionRequest{
		BucketID: bucket.BucketID,
		Key:      "ok.txt",
		ETag:     `"ok"`,
	}); err != nil {
		t.Fatalf("PutObjectVersion(seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close metadata(seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-list-index-repair",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket", "repair-cli",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("run() error = %v stderr=%s", err, stderr.String())
	}
	var body metadataListIndexRepairOutput
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != "namros.admin.metadata_list_index_repair.v1" || body.Status != "clean" || body.RepairNeeded || !body.DryRun || body.Apply {
		t.Fatalf("body = %+v", body)
	}
	if body.BucketID != bucket.BucketID || body.Result.ScannedObjectHeads != 1 {
		t.Fatalf("repair result = %+v", body)
	}
}

func TestMetadataMigrationCommandPlansAppliesResumesAndLists(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "migration-cli",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(seed) error = %v", err)
	}
	if _, err := repo.PutObjectVersion(context.Background(), meta.PutObjectVersionRequest{
		BucketID: bucket.BucketID,
		Key:      "ok.txt",
		ETag:     `"ok"`,
	}); err != nil {
		t.Fatalf("PutObjectVersion(seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close metadata(seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-migration",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-action", "plan",
		"-bucket", "migration-cli",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("metadata-migration plan error = %v stderr=%s", err, stderr.String())
	}
	var plan metadataMigrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &plan); err != nil {
		t.Fatalf("plan json decode: %v body=%s", err, stdout.String())
	}
	if plan.SchemaVersion != "namros.admin.metadata_migration.v1" || plan.Action != "plan" || plan.Status != string(model.MetadataMigrationOperationPlanned) || plan.BucketID != bucket.BucketID {
		t.Fatalf("plan output = %+v", plan)
	}
	if plan.Operation == nil || !plan.Operation.DryRun || plan.Operation.Apply || len(plan.Operation.Steps) != 2 || plan.Operation.Steps[1].Name != "list_index_repair" || plan.Operation.Steps[1].RecordsScanned != 2 {
		t.Fatalf("plan operation = %+v", plan.Operation)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-migration",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-action", "apply",
		"-bucket-id", bucket.BucketID,
		"-resume-of-operation-id", plan.Operation.OperationID,
		"-audit-admin-operation",
	})
	if err != nil {
		t.Fatalf("metadata-migration apply error = %v stderr=%s", err, stderr.String())
	}
	var applyBody metadataMigrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &applyBody); err != nil {
		t.Fatalf("apply json decode: %v body=%s", err, stdout.String())
	}
	if applyBody.Operation == nil || applyBody.Status != string(model.MetadataMigrationOperationSucceeded) || !applyBody.Operation.Apply || applyBody.Operation.ResumeOfOperationID != plan.Operation.OperationID || !applyBody.AuditRecorded {
		t.Fatalf("apply output = %+v", applyBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-migration",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-action", "resume",
		"-bucket", "migration-cli",
		"-resume-of-operation-id", applyBody.Operation.OperationID,
	})
	if err != nil {
		t.Fatalf("metadata-migration resume error = %v stderr=%s", err, stderr.String())
	}
	var resumeBody metadataMigrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &resumeBody); err != nil {
		t.Fatalf("resume json decode: %v body=%s", err, stdout.String())
	}
	if resumeBody.Operation == nil || resumeBody.Operation.ResumeOfOperationID != applyBody.Operation.OperationID || resumeBody.Operation.Status != string(model.MetadataMigrationOperationSucceeded) {
		t.Fatalf("resume output = %+v", resumeBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-migration",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-action", "list",
		"-status", "succeeded",
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("metadata-migration list error = %v stderr=%s", err, stderr.String())
	}
	var listBody metadataMigrationOutput
	if err := json.Unmarshal(stdout.Bytes(), &listBody); err != nil {
		t.Fatalf("list json decode: %v body=%s", err, stdout.String())
	}
	if len(listBody.Operations) != 2 || listBody.Operations[0].OperationID != resumeBody.Operation.OperationID || listBody.Operations[1].OperationID != applyBody.Operation.OperationID {
		t.Fatalf("list operations = %+v", listBody.Operations)
	}

	cfg = config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err = openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(audit check) error = %v", err)
	}
	defer closeRepo()
	auditEvents, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionAdminMetadataMigration,
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(metadata migration) error = %v", err)
	}
	if len(auditEvents) != 1 || auditEvents[0].Details["operation_id"] != applyBody.Operation.OperationID || auditEvents[0].Details["action"] != "apply" {
		t.Fatalf("metadata migration admin audit = %+v", auditEvents)
	}
}

func TestMetadataRestoreValidateCommandVerifiesLocalSegments(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	storagePath := filepath.Join(t.TempDir(), "segments")
	store, err := local.New(storagePath)
	if err != nil {
		t.Fatalf("local.New() error = %v", err)
	}
	payload := []byte("restore validation payload")
	segmentRef, err := store.PutSegment(context.Background(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader(payload),
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "restore-cli",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(seed) error = %v", err)
	}
	if _, err := repo.PutObjectVersion(context.Background(), meta.PutObjectVersionRequest{
		BucketID:    bucket.BucketID,
		Key:         "logs/a.txt",
		SizeBytes:   int64(len(payload)),
		ETag:        `"restore"`,
		SegmentRefs: []storage.SegmentRef{segmentRef},
	}); err != nil {
		t.Fatalf("PutObjectVersion(seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close metadata(seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-restore-validate",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-storage-backend", "local",
		"-storage-path", storagePath,
		"-bucket", "restore-cli",
		"-prefix", "logs/",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("metadata-restore-validate error = %v stderr=%s", err, stderr.String())
	}
	var body metadataRestoreValidateOutput
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != "namros.admin.metadata_restore_validate.v1" || body.Status != "passed" || body.Sampled != 1 || body.Verified != 1 || body.Failed != 0 {
		t.Fatalf("restore validate output = %+v", body)
	}
	if len(body.Samples) != 1 || body.Samples[0].Key != "logs/a.txt" || !body.Samples[0].ListIndexMatch || !body.Samples[0].DigestMatch || body.Samples[0].SegmentCount != 1 {
		t.Fatalf("restore validate samples = %+v", body.Samples)
	}
}

func TestMetadataRestoreValidateCommandReportsEncryptedLocalSegments(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	storagePath := filepath.Join(t.TempDir(), "segments")
	store, err := local.New(storagePath)
	if err != nil {
		t.Fatalf("local.New() error = %v", err)
	}
	ciphertext := []byte("ciphertext restore validation payload")
	segmentRef, err := store.PutSegment(context.Background(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader(ciphertext),
		SizeBytes: uint64(len(ciphertext)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	segmentRef.Encryption = storage.EncryptionEnvelope{
		Algorithm:           encryption.EnvelopeAlgorithmAES256GCM,
		KeyID:               "kms-restore",
		KeyVersion:          "v3",
		WrappedDEK:          "wrapped-dek",
		Nonce:               "nonce",
		PlaintextSizeBytes:  27,
		CiphertextSizeBytes: uint64(len(ciphertext)),
		Context:             map[string]string{"bucket": "restore-encrypted-cli", "key": "logs/encrypted.txt"},
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "restore-encrypted-cli",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(seed) error = %v", err)
	}
	if _, err := repo.PutObjectVersion(context.Background(), meta.PutObjectVersionRequest{
		BucketID:  bucket.BucketID,
		Key:       "logs/encrypted.txt",
		SizeBytes: int64(segmentRef.Encryption.PlaintextSizeBytes),
		ETag:      `"restore-encrypted"`,
		ServerSideEncryption: model.ServerSideEncryption{
			Algorithm:  model.ServerSideEncryptionAWSKMS,
			KeyID:      "kms-restore",
			KeyVersion: "v3",
		},
		SegmentRefs: []storage.SegmentRef{segmentRef},
	}); err != nil {
		t.Fatalf("PutObjectVersion(seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close metadata(seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-restore-validate",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-storage-backend", "local",
		"-storage-path", storagePath,
		"-bucket", "restore-encrypted-cli",
		"-prefix", "logs/",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("metadata-restore-validate error = %v stderr=%s", err, stderr.String())
	}
	var body metadataRestoreValidateOutput
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != "namros.admin.metadata_restore_validate.v1" || body.Status != "passed" || body.Sampled != 1 || body.Verified != 1 || body.Failed != 0 {
		t.Fatalf("restore validate output = %+v", body)
	}
	if len(body.Samples) != 1 || body.Samples[0].Key != "logs/encrypted.txt" || !body.Samples[0].DigestMatch ||
		body.Samples[0].SegmentCount != 1 || body.Samples[0].EncryptedSegmentCount != 1 ||
		body.Samples[0].ServerSideEncryption != string(model.ServerSideEncryptionAWSKMS) ||
		body.Samples[0].KMSKeyID != "kms-restore" || body.Samples[0].KMSKeyVersion != "v3" ||
		body.Samples[0].SizeBytes != int64(segmentRef.Encryption.PlaintextSizeBytes) {
		t.Fatalf("restore validate encrypted sample = %+v", body.Samples)
	}
}

func TestMetadataRestoreValidateCommandVerifiesSBSLocalSegments(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	storagePath := filepath.Join(t.TempDir(), "sbs")
	statePath := filepath.Join(t.TempDir(), "sbs-state.json")
	store, err := sbsegments.Open(context.Background(), sbsegments.Config{
		Path:      storagePath,
		StatePath: statePath,
	})
	if err != nil {
		t.Fatalf("sbs.Open() error = %v", err)
	}
	payload := []byte("restore validation sbs payload")
	segmentRef, err := store.PutSegment(context.Background(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader(payload),
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-local",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "restore-sbs-cli",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(seed) error = %v", err)
	}
	if _, err := repo.PutObjectVersion(context.Background(), meta.PutObjectVersionRequest{
		BucketID:    bucket.BucketID,
		Key:         "logs/sbs.txt",
		SizeBytes:   int64(len(payload)),
		ETag:        `"restore-sbs"`,
		SegmentRefs: []storage.SegmentRef{segmentRef},
	}); err != nil {
		t.Fatalf("PutObjectVersion(seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close metadata(seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-restore-validate",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-storage-backend", "sbs",
		"-storage-path", storagePath,
		"-sbs-state-path", statePath,
		"-bucket", "restore-sbs-cli",
		"-prefix", "logs/",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("metadata-restore-validate error = %v stderr=%s", err, stderr.String())
	}
	var body metadataRestoreValidateOutput
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != "namros.admin.metadata_restore_validate.v1" || body.Status != "passed" || body.Sampled != 1 || body.Verified != 1 || body.Failed != 0 {
		t.Fatalf("restore validate output = %+v", body)
	}
	if len(body.Samples) != 1 || body.Samples[0].Key != "logs/sbs.txt" || !body.Samples[0].ListIndexMatch || !body.Samples[0].DigestMatch || body.Samples[0].SegmentCount != 1 {
		t.Fatalf("restore validate samples = %+v", body.Samples)
	}
	if !stringSliceContains(body.Samples[0].VolumeIDs, "0a0b0001") {
		t.Fatalf("restore validate sample volume ids = %+v", body.Samples[0].VolumeIDs)
	}
}

func TestOpenRestoreValidationStorageSupportsSBSPhysicalConfig(t *testing.T) {
	previousOpen := openRestoreSBSPhysicalStorage
	var gotConfig sbsegments.PhysicalOpenConfig
	cleanupCalled := false
	openRestoreSBSPhysicalStorage = func(_ context.Context, cfg sbsegments.PhysicalOpenConfig) (storage.SegmentStore, func() error, error) {
		gotConfig = cfg
		store, err := local.New(t.TempDir())
		if err != nil {
			return nil, nil, err
		}
		return store, func() error {
			cleanupCalled = true
			return nil
		}, nil
	}
	t.Cleanup(func() {
		openRestoreSBSPhysicalStorage = previousOpen
	})

	cfg := config.Default()
	cfg.StorageBackend = config.StorageBackendSBSPhysical
	cfg.SBSAdminEndpoint = "127.0.0.1:19091"
	cfg.SBSDataEndpoint = "127.0.0.1:19092"
	cfg.SBSVolumeID = "0a0b0002"
	cfg.SBSChunkSizeBytes = 1048576
	cfg.SBSGatewayID = "gw-restore"
	cfg.SBSAttachmentID = "att-restore"
	cfg.SBSGeneration = 7
	cfg.SBSWriterGroupID = "object-writers"
	cfg.SBSSessionID = "gw-restore-boot-1"
	cfg.SBSVolumeEpoch = 11
	cfg.SBSSessionTTL = 45 * time.Second
	cfg.SBSSessionHeartbeat = 15 * time.Second
	cfg.GatewayInstanceID = "gateway-instance-restore"
	cfg.SBSVerifyReadback = true
	cfg.SBSPhysicalWriteConcurrency = 4
	cfg.SBSPhysicalFullChunkWriteMinBytes = 65536
	cfg.SBSPhysicalFullChunkWriteMaxBytes = 4194304
	cfg.SBSPhysicalChunkCacheBytes = 8388608
	cfg.SBSChunkIDAllocationCacheSize = 32

	store, cleanup, err := openRestoreValidationStorage(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openRestoreValidationStorage() error = %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
	if gotConfig.AdminEndpoint != cfg.SBSAdminEndpoint || gotConfig.DataEndpoint != cfg.SBSDataEndpoint || gotConfig.VolumeID != cfg.SBSVolumeID {
		t.Fatalf("physical config endpoints/volume = admin:%q data:%q volume:%q", gotConfig.AdminEndpoint, gotConfig.DataEndpoint, gotConfig.VolumeID)
	}
	if gotConfig.GatewayID != cfg.SBSGatewayID || gotConfig.AttachmentID != cfg.SBSAttachmentID || gotConfig.Generation != cfg.SBSGeneration {
		t.Fatalf("physical writer config = gateway:%q attachment:%q generation:%d", gotConfig.GatewayID, gotConfig.AttachmentID, gotConfig.Generation)
	}
	if gotConfig.SessionIdentity.WriterGroupID != cfg.SBSWriterGroupID || gotConfig.SessionIdentity.SessionID != cfg.SBSSessionID || gotConfig.SessionIdentity.VolumeEpoch != cfg.SBSVolumeEpoch || gotConfig.SessionIdentity.GatewayInstanceID != cfg.GatewayInstanceID {
		t.Fatalf("physical session identity = %+v", gotConfig.SessionIdentity)
	}
	if gotConfig.SessionCache == nil {
		t.Fatal("physical session cache is nil")
	}
	if !gotConfig.VerifyReadback || gotConfig.WriteConcurrency != cfg.SBSPhysicalWriteConcurrency || gotConfig.FullChunkWriteMinBytes != cfg.SBSPhysicalFullChunkWriteMinBytes || gotConfig.FullChunkWriteMaxBytes != cfg.SBSPhysicalFullChunkWriteMaxBytes || gotConfig.ChunkCacheBytes != cfg.SBSPhysicalChunkCacheBytes || gotConfig.ChunkIDAllocationCacheSize != cfg.SBSChunkIDAllocationCacheSize {
		t.Fatalf("physical tuning config = %+v", gotConfig)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if !cleanupCalled {
		t.Fatal("cleanup was not called")
	}
}

func TestOpenRestoreValidationStorageSupportsSBSClusterStaticVolumePool(t *testing.T) {
	previousOpen := openRestoreSBSPhysicalStorage
	previousClusterOpen := openRestoreSBSClusterStorage
	var gotConfigs []sbsegments.PhysicalOpenConfig
	var gotClusterConfigs []sbsegments.ClusterOpenConfig
	openRestoreSBSPhysicalStorage = func(_ context.Context, cfg sbsegments.PhysicalOpenConfig) (storage.SegmentStore, func() error, error) {
		gotConfigs = append(gotConfigs, cfg)
		store, err := local.New(t.TempDir())
		if err != nil {
			return nil, nil, err
		}
		return store, func() error { return nil }, nil
	}
	openRestoreSBSClusterStorage = func(_ context.Context, cfg sbsegments.ClusterOpenConfig) (storage.SegmentStore, func() error, error) {
		gotClusterConfigs = append(gotClusterConfigs, cfg)
		store, err := local.New(t.TempDir())
		if err != nil {
			return nil, nil, err
		}
		return store, func() error { return nil }, nil
	}
	t.Cleanup(func() {
		openRestoreSBSPhysicalStorage = previousOpen
		openRestoreSBSClusterStorage = previousClusterOpen
	})

	cfg := config.Default()
	cfg.StorageBackend = config.StorageBackendSBSCluster
	cfg.SBSAdminEndpoint = "sbs-admin.default:9443"
	cfg.SBSDataEndpoint = "sbs-data.default:9460"
	cfg.SBSGatewayID = "gw-restore"
	cfg.SBSAttachmentID = "att-restore-{volume_id}"
	cfg.SBSGeneration = 5
	cfg.SBSWriterGroupID = "object-writers"
	cfg.SBSVolumeEpoch = 17
	cfg.GatewayInstanceID = "gateway-instance-restore"
	cfg.SBSChunkSizeBytes = 1048576
	cfg.SBSPhysicalWriteConcurrency = 3
	cfg.SBSVolumePool = []config.SBSVolumePoolMember{
		{VolumeID: "18a00001", DataEndpoint: "sbs-data-a:9460", Weight: 2},
		{VolumeID: "18a00002", AdminEndpoint: "sbs-admin-b:9443", DataEndpoint: "sbs-data-b:9460", GatewayID: "gw-member-b", AttachmentID: "att-b", Generation: 6, WriterGroupID: "object-writers-b", VolumeEpoch: 18, WriteConcurrency: 4, ReadOnly: true},
	}

	store, cleanup, err := openRestoreValidationStorage(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openRestoreValidationStorage() error = %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
	if edition.Allows(cfg.Edition, edition.FeatureErasureCoding) {
		if len(gotClusterConfigs) != 2 {
			t.Fatalf("cluster open count = %d, want 2", len(gotClusterConfigs))
		}
		if gotClusterConfigs[0].AdminEndpoint != cfg.SBSAdminEndpoint || gotClusterConfigs[0].DataEndpoint != "sbs-data-a:9460" || gotClusterConfigs[0].VolumeID != "18a00001" || gotClusterConfigs[0].AttachmentID != "att-restore-18a00001" || gotClusterConfigs[0].WriteConcurrency != cfg.SBSPhysicalWriteConcurrency {
			t.Fatalf("first cluster member config = %+v", gotClusterConfigs[0])
		}
		if gotClusterConfigs[1].AdminEndpoint != "sbs-admin-b:9443" || gotClusterConfigs[1].DataEndpoint != "sbs-data-b:9460" || gotClusterConfigs[1].GatewayID != "gw-member-b" || gotClusterConfigs[1].AttachmentID != "att-b" || gotClusterConfigs[1].Generation != 6 || gotClusterConfigs[1].WriteConcurrency != 4 {
			t.Fatalf("second cluster member config = %+v", gotClusterConfigs[1])
		}
	} else {
		if len(gotConfigs) != 2 {
			t.Fatalf("physical open count = %d, want 2", len(gotConfigs))
		}
		if gotConfigs[0].AdminEndpoint != cfg.SBSAdminEndpoint || gotConfigs[0].DataEndpoint != "sbs-data-a:9460" || gotConfigs[0].VolumeID != "18a00001" || gotConfigs[0].AttachmentID != "att-restore-18a00001" || gotConfigs[0].WriteConcurrency != cfg.SBSPhysicalWriteConcurrency {
			t.Fatalf("first member config = %+v", gotConfigs[0])
		}
		if gotConfigs[1].AdminEndpoint != "sbs-admin-b:9443" || gotConfigs[1].DataEndpoint != "sbs-data-b:9460" || gotConfigs[1].GatewayID != "gw-member-b" || gotConfigs[1].AttachmentID != "att-b" || gotConfigs[1].Generation != 6 || gotConfigs[1].WriteConcurrency != 4 {
			t.Fatalf("second member config = %+v", gotConfigs[1])
		}
		if gotConfigs[1].SessionIdentity.WriterGroupID != "object-writers-b" || gotConfigs[1].SessionIdentity.VolumeEpoch != 18 || gotConfigs[1].SessionIdentity.VolumeID != "18a00002" {
			t.Fatalf("second member session identity = %+v", gotConfigs[1].SessionIdentity)
		}
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
}

func TestVolumePoolPutCommand(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"volume-pool-put",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-pool-id", "object-pool",
		"-generation", "5",
		"-durability-class", "replicated",
		"-storage-class", "STANDARD,ARCHIVE",
		"-member", "volume_id=18a00001,admin_endpoint=sbs-admin-a:9443,data_endpoint=sbs-data-a:9460,state=active,weight=2,available_bytes=1048576,used_percent=25,high_watermark_percent=90,last_observed_at=2026-08-10T03:30:00Z",
		"-member", "volume_id=18a00002,admin_endpoint=sbs-admin-a:9443,data_endpoint=sbs-data-a:9460,readonly=true",
	})
	if err != nil {
		t.Fatalf("run() error = %v stderr=%s", err, stderr.String())
	}
	var body volumePoolOutput
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.PoolID != "object-pool" || body.Generation != 5 || len(body.Members) != 2 {
		t.Fatalf("body = %+v", body)
	}
	if body.Members[0].State != "active" || body.Members[0].Weight != 2 || body.Members[1].State != "read_only" {
		t.Fatalf("members = %+v", body.Members)
	}
	if body.Members[0].LastObservedAt != "2026-08-10T03:30:00Z" {
		t.Fatalf("member last observed = %q", body.Members[0].LastObservedAt)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata() error = %v", err)
	}
	defer closeRepo()
	pool, err := repo.GetVolumePool(context.Background(), "object-pool")
	if err != nil {
		t.Fatalf("GetVolumePool() error = %v", err)
	}
	if pool.Generation != 5 || len(pool.StorageClassIDs) != 2 || pool.Members[1].State != model.VolumePoolStateReadOnly {
		t.Fatalf("stored pool = %+v", pool)
	}
	if got := pool.Members[0].LastObservedAt.Format(time.RFC3339); got != "2026-08-10T03:30:00Z" {
		t.Fatalf("stored member last observed = %q", got)
	}
}

func TestBucketQuotaCommands(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed bucket) error = %v", err)
	}
	if _, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "quota-bucket",
		Region:   "us-east-1",
	}); err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(seed bucket) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"bucket-quota-put",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket", "quota-bucket",
		"-max-object-size-bytes", "4096",
	})
	if err != nil {
		t.Fatalf("bucket-quota-put error = %v stderr=%s", err, stderr.String())
	}
	var putBody bucketQuotaOutput
	if err := json.Unmarshal(stdout.Bytes(), &putBody); err != nil {
		t.Fatalf("json decode put: %v body=%s", err, stdout.String())
	}
	if putBody.SchemaVersion != "namros.admin.bucket_quota.v1" || putBody.Bucket != "quota-bucket" || !putBody.Configured || putBody.MaxObjectSizeBytes != 4096 {
		t.Fatalf("put body = %+v", putBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"bucket-quota-get",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket", "quota-bucket",
	})
	if err != nil {
		t.Fatalf("bucket-quota-get error = %v stderr=%s", err, stderr.String())
	}
	var getBody bucketQuotaOutput
	if err := json.Unmarshal(stdout.Bytes(), &getBody); err != nil {
		t.Fatalf("json decode get: %v body=%s", err, stdout.String())
	}
	if !getBody.Configured || getBody.MaxObjectSizeBytes != 4096 || getBody.BucketID == "" {
		t.Fatalf("get body = %+v", getBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"bucket-quota-delete",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket", "quota-bucket",
	})
	if err != nil {
		t.Fatalf("bucket-quota-delete error = %v stderr=%s", err, stderr.String())
	}
	var deleteBody bucketQuotaOutput
	if err := json.Unmarshal(stdout.Bytes(), &deleteBody); err != nil {
		t.Fatalf("json decode delete: %v body=%s", err, stdout.String())
	}
	if deleteBody.Configured || !deleteBody.Deleted || deleteBody.MaxObjectSizeBytes != 0 {
		t.Fatalf("delete body = %+v", deleteBody)
	}
}

func seedOperationalMetadataForExport(t *testing.T, metadataPath, kmsKeyID string, kmsState model.KMSKeyState) model.DedupeOperationRecord {
	t.Helper()
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed operational metadata) error = %v", err)
	}
	defer closeRepo()
	dedupeOp, err := repo.PutDedupeOperation(context.Background(), meta.PutDedupeOperationRequest{
		Status:     model.DedupeOperationSucceeded,
		StartedAt:  time.Date(2026, 7, 6, 1, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 7, 6, 1, 0, 1, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PutDedupeOperation(seed operational metadata) error = %v", err)
	}
	if _, err := repo.PutAdminAuditEvent(context.Background(), meta.PutAdminAuditEventRequest{
		Action: model.AuditActionDedupeAck,
		Details: map[string]string{
			"operation_id": dedupeOp.OperationID,
			"tenant_id":    "tenant-1",
		},
		Audit: meta.AuditContext{Reason: "metadata export test seed"},
	}); err != nil {
		t.Fatalf("PutAdminAuditEvent(seed operational metadata) error = %v", err)
	}
	if _, err := repo.PutKMSKey(context.Background(), meta.PutKMSKeyRequest{
		KeyID:      kmsKeyID,
		KeyVersion: "v1",
		State:      kmsState,
	}); err != nil {
		t.Fatalf("PutKMSKey(seed operational metadata) error = %v", err)
	}
	if _, err := repo.PutMetadataMigrationOperation(context.Background(), meta.PutMetadataMigrationOperationRequest{
		TargetSchemaVersion: meta.CurrentMetadataSchemaVersion,
		Status:              model.MetadataMigrationOperationSucceeded,
		Apply:               true,
		OwnerID:             "test-admin",
		StartedAt:           time.Date(2026, 7, 6, 1, 5, 0, 0, time.UTC),
		FinishedAt:          time.Date(2026, 7, 6, 1, 5, 1, 0, time.UTC),
		Steps: []model.MetadataMigrationStep{{
			Name:            "list_index_repair",
			Status:          model.MetadataMigrationStepSucceeded,
			RecordsScanned:  2,
			RecordsRepaired: 1,
		}},
	}); err != nil {
		t.Fatalf("PutMetadataMigrationOperation(seed operational metadata) error = %v", err)
	}
	if _, err := repo.PutVolumePool(context.Background(), meta.PutVolumePoolRequest{
		PoolID:          "object-pool",
		Generation:      7,
		DurabilityClass: "replicated",
		StorageClassIDs: []string{"STANDARD"},
		Members: []model.VolumePoolMember{{
			VolumeID:       "18a00001",
			DataEndpoint:   "sbs-data-a:9444",
			State:          model.VolumePoolStateActive,
			Weight:         1,
			AvailableBytes: 1024,
		}},
	}); err != nil {
		t.Fatalf("PutVolumePool(seed operational metadata) error = %v", err)
	}
	if _, err := repo.PutVolumeDrainOperation(context.Background(), meta.PutVolumeDrainOperationRequest{
		PoolID:         "object-pool",
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		OwnerID:        "test-admin",
		Status:         model.VolumeDrainOperationSucceeded,
		StartedAt:      time.Date(2026, 7, 6, 1, 6, 0, 0, time.UTC),
		FinishedAt:     time.Date(2026, 7, 6, 1, 6, 1, 0, time.UTC),
		Scanned:        1,
		Copied:         1,
	}); err != nil {
		t.Fatalf("PutVolumeDrainOperation(seed operational metadata) error = %v", err)
	}
	lease, err := repo.AcquireWorkerLease(context.Background(), meta.AcquireWorkerLeaseRequest{
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "test-worker",
		TTL:        time.Minute,
		Cursor:     "cursor-a",
	})
	if err != nil {
		t.Fatalf("AcquireWorkerLease(seed operational metadata) error = %v", err)
	}
	if _, err := repo.PutWorkerOperation(context.Background(), meta.PutWorkerOperationRequest{
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "test-worker",
		LeaseID:    lease.LeaseID,
		Status:     model.WorkerOperationSucceeded,
		StartedAt:  time.Date(2026, 7, 6, 1, 7, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 7, 6, 1, 7, 1, 0, time.UTC),
		Scanned:    2,
		Processed:  2,
	}); err != nil {
		t.Fatalf("PutWorkerOperation(seed operational metadata) error = %v", err)
	}
	return dedupeOp
}

func TestDedupePlanCommandOutputsEmptyMemoryScan(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"dedupe-plan",
		"-metadata-backend", "memory",
		"-tenant-id", "tenant-1",
		"-policy-id", "policy-1",
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("run() error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SBSClass *struct {
			DedupeClassID string `json:"dedupe_class_id"`
			TenantID      string `json:"tenant_id"`
		} `json:"sbs_class"`
		Result struct {
			ScannedVersions int `json:"scanned_versions"`
			CandidatePairs  int `json:"candidate_pairs"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SBSClass == nil || body.SBSClass.DedupeClassID != "policy-1" || body.SBSClass.TenantID != "tenant-1" {
		t.Fatalf("sbs class = %+v", body.SBSClass)
	}
	if body.Result.ScannedVersions != 0 || body.Result.CandidatePairs != 0 {
		t.Fatalf("result = %+v, want empty scan", body.Result)
	}
}

func TestEnterpriseAdminCommandsRejectCommunityEdition(t *testing.T) {
	skipEnterpriseOverlayCommunityAssertion(t)
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "dedupe plan",
			args: []string{
				"dedupe-plan",
				"-metadata-backend", "memory",
				"-tenant-id", "tenant-1",
			},
		},
		{
			name: "dedupe invalid flag",
			args: []string{
				"dedupe-plan",
				"-not-a-real-flag",
			},
		},
		{
			name: "dedupe ack",
			args: []string{
				"dedupe-ack",
				"-metadata-backend", "memory",
				"-storage-backend", "memory",
				"-tenant-id", "tenant-1",
			},
		},
		{
			name: "dedupe operations",
			args: []string{
				"dedupe-ops",
				"-metadata-backend", "memory",
			},
		},
		{
			name: "dedupe repair",
			args: []string{
				"dedupe-repair",
				"-metadata-backend", "memory",
			},
		},
		{
			name: "dedupe scrub",
			args: []string{
				"dedupe-scrub",
				"-metadata-backend", "memory",
			},
		},
		{
			name: "kms put",
			args: []string{
				"kms-key-put",
				"-metadata-backend", "memory",
				"-key-id", "kms-1",
			},
		},
		{
			name: "kms list",
			args: []string{
				"kms-key-list",
				"-metadata-backend", "memory",
			},
		},
		{
			name: "compliance evidence",
			args: []string{
				"compliance-evidence",
				"-metadata-backend", "memory",
				"-bucket-id", "bucket-1",
			},
		},
		{
			name: "compliance profile",
			args: []string{
				"compliance-profile-plan",
				"-profile-id", "profile-1",
				"-regulation", "sec",
				"-record-class", "records",
				"-retention-days", "1",
			},
		},
		{
			name: "compliance policy simulation",
			args: []string{
				"compliance-policy-simulate",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			err := (adminCommand{stdout: &stdout, stderr: &stderr}).run(context.Background(), tt.args)
			if err == nil || !strings.Contains(err.Error(), "NAMROS Enterprise Edition") {
				t.Fatalf("run() error = %v, want enterprise edition error", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %s, want empty", stdout.String())
			}
		})
	}
}

func TestDedupePlanCommandRejectsMissingTenant(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"dedupe-plan",
		"-metadata-backend", "memory",
	})
	if err == nil || !strings.Contains(err.Error(), "tenant id is required") {
		t.Fatalf("run() error = %v, want tenant id error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s, want empty", stdout.String())
	}
}

func TestDedupeAckCommandOutputsEmptyMemoryRun(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"dedupe-ack",
		"-metadata-backend", "memory",
		"-storage-backend", "memory",
		"-tenant-id", "tenant-1",
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("run() error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		OperationID string `json:"operation_id"`
		Status      string `json:"status"`
		Scan        struct {
			ScannedVersions int `json:"scanned_versions"`
			CandidatePairs  int `json:"candidate_pairs"`
		} `json:"scan"`
		Acked     int `json:"acked"`
		Skipped   int `json:"skipped"`
		Retryable int `json:"retryable"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.OperationID == "" || body.Status != "succeeded" {
		t.Fatalf("operation id/status = %q/%q, want persisted succeeded operation", body.OperationID, body.Status)
	}
	if body.Scan.ScannedVersions != 0 || body.Scan.CandidatePairs != 0 || body.Acked != 0 || body.Skipped != 0 || body.Retryable != 0 {
		t.Fatalf("body = %+v, want empty ack run", body)
	}
}

func TestDedupeAckCommandRejectsMissingTenant(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"dedupe-ack",
		"-metadata-backend", "memory",
		"-storage-backend", "memory",
	})
	if err == nil || !strings.Contains(err.Error(), "tenant id is required") {
		t.Fatalf("run() error = %v, want tenant id error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %s, want empty", stdout.String())
	}
}

func TestDedupeOperationsCommandListsPersistedAckRuns(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	var ackStdout bytes.Buffer
	var ackStderr bytes.Buffer
	err := (testAdminCommand(&ackStdout, &ackStderr)).run(context.Background(), []string{
		"dedupe-ack",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-storage-backend", "memory",
		"-tenant-id", "tenant-1",
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("dedupe-ack error = %v stderr=%s", err, ackStderr.String())
	}

	var listStdout bytes.Buffer
	var listStderr bytes.Buffer
	err = (testAdminCommand(&listStdout, &listStderr)).run(context.Background(), []string{
		"dedupe-ops",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("dedupe-ops error = %v stderr=%s", err, listStderr.String())
	}
	var body struct {
		Operations []struct {
			OperationID string `json:"operation_id"`
			Status      string `json:"status"`
			Scanned     int    `json:"scanned"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(listStdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, listStdout.String())
	}
	if len(body.Operations) != 1 || body.Operations[0].OperationID == "" || body.Operations[0].Status != "succeeded" || body.Operations[0].Scanned != 0 {
		t.Fatalf("operations = %+v, want one persisted empty succeeded run", body.Operations)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata() error = %v", err)
	}
	defer closeRepo()
	events, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionDedupeAck,
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(dedupe ack) error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("dedupe ack audit events = %d, want 1: %+v", len(events), events)
	}
	if events[0].Details["operation_id"] != body.Operations[0].OperationID || events[0].Details["tenant_id"] != "tenant-1" {
		t.Fatalf("dedupe ack audit details = %+v", events[0].Details)
	}
}

func TestStatusCommandReportsMetadataReadiness(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(status seed) error = %v", err)
	}
	dedupeOp, err := repo.PutDedupeOperation(context.Background(), meta.PutDedupeOperationRequest{
		Status:     model.DedupeOperationSucceeded,
		StartedAt:  time.Date(2026, 7, 6, 1, 0, 0, 0, time.UTC),
		FinishedAt: time.Date(2026, 7, 6, 1, 0, 1, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("PutDedupeOperation(status seed) error = %v", err)
	}
	if _, err := repo.PutAdminAuditEvent(context.Background(), meta.PutAdminAuditEventRequest{
		Action: model.AuditActionDedupeAck,
		Details: map[string]string{
			"operation_id": dedupeOp.OperationID,
			"tenant_id":    "tenant-1",
		},
		Audit: meta.AuditContext{
			Reason: "status test seed",
		},
	}); err != nil {
		t.Fatalf("PutAdminAuditEvent(status seed) error = %v", err)
	}
	if _, err := repo.PutGCOperation(context.Background(), meta.PutGCOperationRequest{
		Status:    model.GCOperationRetryPending,
		Scanned:   2,
		Deleted:   1,
		Retryable: 1,
	}); err != nil {
		t.Fatalf("PutGCOperation(status seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(status seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (adminCommand{stdout: &stdout, stderr: &stderr}).run(context.Background(), []string{
		"status",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-count-limit", "10",
		"-recent-dedupe-limit", "3",
		"-recent-gc-limit", "2",
	})
	if err != nil {
		t.Fatalf("status error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion int    `json:"schema_version"`
		GeneratedAt   string `json:"generated_at"`
		Status        string `json:"status"`
		Metadata      struct {
			Backend string `json:"backend"`
			Path    string `json:"path"`
		} `json:"metadata"`
		Limits struct {
			CountLimit        int `json:"count_limit"`
			RecentDedupeLimit int `json:"recent_dedupe_limit"`
			RecentGCLimit     int `json:"recent_gc_limit"`
		} `json:"limits"`
		Counts struct {
			AuditEvents      int `json:"audit_events"`
			GCOperations     int `json:"gc_operations"`
			DedupeOperations int `json:"dedupe_operations"`
		} `json:"counts"`
		AuditChain struct {
			Sampled       int    `json:"sampled"`
			FirstEventID  string `json:"first_event_id"`
			LastEventID   string `json:"last_event_id"`
			LastHash      string `json:"last_hash"`
			HashesPresent bool   `json:"hashes_present"`
		} `json:"audit_chain"`
		RecentDedupeOperations []struct {
			OperationID string `json:"operation_id"`
			Status      string `json:"status"`
		} `json:"recent_dedupe_operations"`
		RecentGCOperations []struct {
			OperationID string `json:"operation_id"`
			Status      string `json:"status"`
			Scanned     int    `json:"scanned"`
			Deleted     int    `json:"deleted"`
		} `json:"recent_gc_operations"`
		MetadataRestore struct {
			SchemaVersion       int      `json:"schema_version"`
			ConflictPolicy      string   `json:"conflict_policy"`
			PreserveSourceIDs   bool     `json:"preserve_source_ids"`
			PreserveAuditHashes bool     `json:"preserve_audit_hashes"`
			Collections         []string `json:"collections"`
		} `json:"metadata_restore"`
		ProductionReadiness struct {
			SchemaVersion                  string   `json:"schema_version"`
			Status                         string   `json:"status"`
			DeploymentProfile              string   `json:"deployment_profile"`
			AllowUnsafeProductionShortcuts bool     `json:"allow_unsafe_production_shortcuts"`
			MetadataBackend                string   `json:"metadata_backend"`
			CoordinationBackend            string   `json:"coordination_backend"`
			GatewayCountKnown              bool     `json:"gateway_count_known"`
			GatewayCount                   int      `json:"gateway_count"`
			StorageBackend                 string   `json:"storage_backend"`
			SBSVolumePoolSource            string   `json:"sbs_volume_pool_source"`
			SBSVolumePoolMemberCount       int      `json:"sbs_volume_pool_member_count"`
			GCCandidateQueue               string   `json:"gc_candidate_queue"`
			UnsupportedClaims              []string `json:"unsupported_claims"`
		} `json:"production_readiness"`
		Capabilities struct {
			MetadataExport            bool `json:"metadata_export"`
			MetadataImportDryRun      bool `json:"metadata_import_dry_run"`
			MetadataImportApplyPlan   bool `json:"metadata_import_apply_plan"`
			MetadataImportApply       bool `json:"metadata_import_apply"`
			ComplianceEvidencePackage bool `json:"compliance_evidence_package"`
			ComplianceAccessEvidence  bool `json:"compliance_access_evidence"`
			ComplianceTimeSource      bool `json:"compliance_time_source"`
			ComplianceProfilePlan     bool `json:"compliance_profile_plan"`
			CompliancePolicySimulate  bool `json:"compliance_policy_simulate"`
			DedupeOperations          bool `json:"dedupe_operations"`
		} `json:"capabilities"`
		Edition struct {
			Name     string `json:"name"`
			Features []struct {
				ID             string `json:"id"`
				MinimumEdition string `json:"minimum_edition"`
				Enabled        bool   `json:"enabled"`
			} `json:"features"`
		} `json:"edition"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != 1 || body.GeneratedAt == "" || body.Status != "ok" {
		t.Fatalf("status envelope = %+v", body)
	}
	if body.Metadata.Backend != "pebble" || body.Metadata.Path != metadataPath {
		t.Fatalf("status metadata = %+v", body.Metadata)
	}
	if body.Limits.CountLimit != 10 || body.Limits.RecentDedupeLimit != 3 || body.Limits.RecentGCLimit != 2 {
		t.Fatalf("status limits = %+v", body.Limits)
	}
	if body.Counts.AuditEvents != 1 || body.Counts.GCOperations != 1 || body.Counts.DedupeOperations != 1 {
		t.Fatalf("status counts = %+v", body.Counts)
	}
	if body.AuditChain.Sampled != 1 || body.AuditChain.FirstEventID == "" || body.AuditChain.LastEventID != body.AuditChain.FirstEventID || body.AuditChain.LastHash == "" || !body.AuditChain.HashesPresent {
		t.Fatalf("status audit chain = %+v", body.AuditChain)
	}
	if len(body.RecentDedupeOperations) != 1 || body.RecentDedupeOperations[0].OperationID == "" || body.RecentDedupeOperations[0].Status != "succeeded" {
		t.Fatalf("status recent dedupe operations = %+v", body.RecentDedupeOperations)
	}
	if len(body.RecentGCOperations) != 1 || body.RecentGCOperations[0].OperationID == "" || body.RecentGCOperations[0].Status != string(model.GCOperationRetryPending) || body.RecentGCOperations[0].Scanned != 2 || body.RecentGCOperations[0].Deleted != 1 {
		t.Fatalf("status recent gc operations = %+v", body.RecentGCOperations)
	}
	if body.MetadataRestore.SchemaVersion != 1 || body.MetadataRestore.ConflictPolicy != "fail_if_exists" || !body.MetadataRestore.PreserveSourceIDs || !body.MetadataRestore.PreserveAuditHashes || len(body.MetadataRestore.Collections) != 12 {
		t.Fatalf("status metadata restore = %+v", body.MetadataRestore)
	}
	for _, collection := range []string{"metadata_schema", "metadata_migration_operations", "volume_pools", "volume_drain_operations", "worker_leases", "worker_operations"} {
		if !stringSliceContains(body.MetadataRestore.Collections, collection) {
			t.Fatalf("status metadata restore collections = %+v, want %q", body.MetadataRestore.Collections, collection)
		}
	}
	if body.ProductionReadiness.SchemaVersion != "namros.production_readiness.v1" || body.ProductionReadiness.Status != "blocked" {
		t.Fatalf("status production readiness = %+v", body.ProductionReadiness)
	}
	if body.ProductionReadiness.DeploymentProfile != config.DeploymentProfileDev || body.ProductionReadiness.MetadataBackend != config.MetadataBackendPebble || body.ProductionReadiness.StorageBackend != config.StorageBackendMemory || body.ProductionReadiness.CoordinationBackend != config.CoordinationBackendNone {
		t.Fatalf("status production readiness topology = %+v", body.ProductionReadiness)
	}
	if !body.ProductionReadiness.GatewayCountKnown || body.ProductionReadiness.GatewayCount != 0 || body.ProductionReadiness.SBSVolumePoolSource != "not_configured" || body.ProductionReadiness.SBSVolumePoolMemberCount != 0 {
		t.Fatalf("status production readiness counts = %+v", body.ProductionReadiness)
	}
	if !stringSliceContains(body.ProductionReadiness.UnsupportedClaims, "deployment_profile_not_production") || !stringSliceContains(body.ProductionReadiness.UnsupportedClaims, "metadata_backend_not_tikv") {
		t.Fatalf("status production readiness unsupported claims = %+v", body.ProductionReadiness.UnsupportedClaims)
	}
	if !body.Capabilities.MetadataExport || !body.Capabilities.MetadataImportDryRun || !body.Capabilities.MetadataImportApplyPlan || !body.Capabilities.MetadataImportApply || !body.Capabilities.ComplianceEvidencePackage || !body.Capabilities.ComplianceAccessEvidence || !body.Capabilities.ComplianceTimeSource || !body.Capabilities.ComplianceProfilePlan || !body.Capabilities.CompliancePolicySimulate || !body.Capabilities.DedupeOperations {
		t.Fatalf("status capabilities = %+v", body.Capabilities)
	}
	if len(body.Edition.Features) == 0 {
		t.Fatalf("status edition = %+v", body.Edition)
	}
	features := map[string]bool{}
	for _, feature := range body.Edition.Features {
		features[feature.ID] = feature.Enabled
	}
	if enterpriseOverlayTest() {
		if body.Edition.Name != "enterprise" || !features["core_s3_api"] || !features["basic_encryption"] || !features["worm_object_lock"] || !features["dedupe"] || !features["erasure_coding"] || !features["sse_kms"] {
			t.Fatalf("status edition = %+v features = %+v", body.Edition, body.Edition.Features)
		}
	} else {
		if body.Edition.Name != "community" || !features["core_s3_api"] || !features["basic_encryption"] || features["worm_object_lock"] || features["dedupe"] || features["erasure_coding"] || features["sse_kms"] {
			t.Fatalf("status edition = %+v features = %+v", body.Edition, body.Edition.Features)
		}
	}

	cfg = config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err = openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(status audit check) error = %v", err)
	}
	defer closeRepo()
	events, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditEvents(status audit check) error = %v", err)
	}
	if len(events) != 1 || events[0].Action != model.AuditActionDedupeAck {
		t.Fatalf("status should not create audit events, got %+v", events)
	}
}

func TestStatusCommandReportsProductionReadinessVolumePool(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(status pool seed) error = %v", err)
	}
	if _, err := repo.PutVolumePool(context.Background(), meta.PutVolumePoolRequest{
		PoolID: "object-pool",
		Members: []model.VolumePoolMember{
			{VolumeID: "18a00001", State: model.VolumePoolStateActive},
			{VolumeID: "18a00002", State: model.VolumePoolStateActive},
		},
	}); err != nil {
		t.Fatalf("PutVolumePool(status pool seed) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(status pool seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (adminCommand{stdout: &stdout, stderr: &stderr}).run(context.Background(), []string{
		"status",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-deployment-profile", "production",
		"-storage-backend", "sbs-physical",
		"-sbs-volume-pool-id", "object-pool",
		"-sbs-writer-group-id", "object-writers",
		"-sbs-session-id", "gw-prod-boot-1",
		"-sbs-volume-epoch", "1",
		"-coordination-backend", "none",
		"-gc-candidate-queue", "metadata",
		"-count-limit", "1",
		"-recent-dedupe-limit", "0",
		"-recent-gc-limit", "0",
	})
	if err != nil {
		t.Fatalf("status production readiness error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		ProductionReadiness struct {
			DeploymentProfile        string   `json:"deployment_profile"`
			StorageBackend           string   `json:"storage_backend"`
			SBSVolumePoolSource      string   `json:"sbs_volume_pool_source"`
			SBSVolumePoolID          string   `json:"sbs_volume_pool_id"`
			SBSVolumePoolGeneration  uint64   `json:"sbs_volume_pool_generation"`
			SBSVolumePoolMemberCount int      `json:"sbs_volume_pool_member_count"`
			SBSSessionFencing        bool     `json:"sbs_session_fencing_configured"`
			GCCandidateQueue         string   `json:"gc_candidate_queue"`
			UnsupportedClaims        []string `json:"unsupported_claims"`
		} `json:"production_readiness"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	readiness := body.ProductionReadiness
	if readiness.DeploymentProfile != config.DeploymentProfileProduction || readiness.StorageBackend != config.StorageBackendSBSPhysical || readiness.GCCandidateQueue != config.GCCandidateQueueMetadata {
		t.Fatalf("production readiness config = %+v", readiness)
	}
	if readiness.SBSVolumePoolSource != "metadata_registry" || readiness.SBSVolumePoolID != "object-pool" || readiness.SBSVolumePoolGeneration != 1 || readiness.SBSVolumePoolMemberCount != 2 {
		t.Fatalf("production readiness volume pool = %+v", readiness)
	}
	if !readiness.SBSSessionFencing {
		t.Fatalf("production readiness session fencing = false, want true")
	}
	if stringSliceContains(readiness.UnsupportedClaims, "sbs_volume_pool_member_count_below_2") {
		t.Fatalf("production readiness unsupported claims = %+v", readiness.UnsupportedClaims)
	}
}

func TestTenantQuotaCommands(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed tenant) error = %v", err)
	}
	if _, err := repo.CreateTenant(context.Background(), meta.CreateTenantRequest{
		TenantID:    "tenant-quota-a",
		DisplayName: "Tenant Quota A",
	}); err != nil {
		t.Fatalf("CreateTenant() error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(seed tenant) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"tenant-quota-put",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-tenant-id", "tenant-quota-a",
		"-max-bytes", "1099511627776",
		"-max-objects", "1000000",
		"-max-active-uploads", "256",
	})
	if err != nil {
		t.Fatalf("tenant-quota-put error = %v stderr=%s", err, stderr.String())
	}
	var putBody tenantQuotaOutput
	if err := json.Unmarshal(stdout.Bytes(), &putBody); err != nil {
		t.Fatalf("json decode put: %v body=%s", err, stdout.String())
	}
	if putBody.SchemaVersion != "namros.admin.tenant_quota.v1" || putBody.TenantID != "tenant-quota-a" || !putBody.Configured || putBody.MaxBytes != 1099511627776 || putBody.MaxObjects != 1000000 || putBody.MaxActiveUploads != 256 {
		t.Fatalf("put body = %+v", putBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"tenant-quota-get",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-tenant-id", "tenant-quota-a",
	})
	if err != nil {
		t.Fatalf("tenant-quota-get error = %v stderr=%s", err, stderr.String())
	}
	var getBody tenantQuotaOutput
	if err := json.Unmarshal(stdout.Bytes(), &getBody); err != nil {
		t.Fatalf("json decode get: %v body=%s", err, stdout.String())
	}
	if !getBody.Configured || getBody.MaxBytes != 1099511627776 || getBody.MaxObjects != 1000000 || getBody.MaxActiveUploads != 256 {
		t.Fatalf("get body = %+v", getBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"tenant-quota-delete",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-tenant-id", "tenant-quota-a",
	})
	if err != nil {
		t.Fatalf("tenant-quota-delete error = %v stderr=%s", err, stderr.String())
	}
	var deleteBody tenantQuotaOutput
	if err := json.Unmarshal(stdout.Bytes(), &deleteBody); err != nil {
		t.Fatalf("json decode delete: %v body=%s", err, stdout.String())
	}
	if deleteBody.Configured || !deleteBody.Deleted || deleteBody.MaxBytes != 0 || deleteBody.MaxObjects != 0 || deleteBody.MaxActiveUploads != 0 {
		t.Fatalf("delete body = %+v", deleteBody)
	}
}

func TestWorkerOperationsCommandListsFilteredOperations(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(worker seed) error = %v", err)
	}
	startedAt := time.Date(2026, 7, 17, 8, 0, 0, 0, time.UTC)
	first, err := repo.PutWorkerOperation(context.Background(), meta.PutWorkerOperationRequest{
		WorkerKind: "gc",
		ShardID:    "orphans",
		OwnerID:    "gateway-a",
		LeaseID:    "lease-a",
		Status:     model.WorkerOperationRetryPending,
		Scanned:    3,
		Processed:  1,
		Skipped:    1,
		Retryable:  1,
		LastError:  "storage unavailable",
		StartedAt:  startedAt,
		FinishedAt: startedAt.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("PutWorkerOperation(gc) error = %v", err)
	}
	if _, err := repo.PutWorkerOperation(context.Background(), meta.PutWorkerOperationRequest{
		WorkerKind: "lifecycle",
		ShardID:    "bucket-a",
		OwnerID:    "gateway-b",
		Status:     model.WorkerOperationSucceeded,
	}); err != nil {
		t.Fatalf("PutWorkerOperation(lifecycle) error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(worker seed) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (adminCommand{stdout: &stdout, stderr: &stderr}).run(context.Background(), []string{
		"worker-operations",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-worker-kind", "gc",
		"-shard-id", "orphans",
		"-status", "retry_pending",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("worker-operations error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion string `json:"schema_version"`
		GeneratedAt   string `json:"generated_at"`
		Limit         int    `json:"limit"`
		WorkerKind    string `json:"worker_kind"`
		ShardID       string `json:"shard_id"`
		Status        string `json:"status"`
		Operations    []struct {
			OperationID string `json:"operation_id"`
			WorkerKind  string `json:"worker_kind"`
			ShardID     string `json:"shard_id"`
			OwnerID     string `json:"owner_id"`
			LeaseID     string `json:"lease_id"`
			Status      string `json:"status"`
			Scanned     int    `json:"scanned"`
			Processed   int    `json:"processed"`
			Skipped     int    `json:"skipped"`
			Retryable   int    `json:"retryable"`
			LastError   string `json:"last_error"`
		} `json:"operations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != "namros.admin.worker_operations.v1" || body.GeneratedAt == "" || body.Limit != 10 {
		t.Fatalf("envelope = %+v", body)
	}
	if body.WorkerKind != "gc" || body.ShardID != "orphans" || body.Status != "retry_pending" {
		t.Fatalf("filters = kind:%q shard:%q status:%q", body.WorkerKind, body.ShardID, body.Status)
	}
	if len(body.Operations) != 1 || body.Operations[0].OperationID != first.OperationID {
		t.Fatalf("operations = %+v, want %q", body.Operations, first.OperationID)
	}
	if got := body.Operations[0]; got.OwnerID != "gateway-a" || got.LeaseID != "lease-a" || got.Status != "retry_pending" || got.Scanned != 3 || got.Processed != 1 || got.Skipped != 1 || got.Retryable != 1 || got.LastError == "" {
		t.Fatalf("operation = %+v", got)
	}
}

func TestVolumeDrainOperationsCommandPutsAndLists(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")

	var putStdout bytes.Buffer
	var putStderr bytes.Buffer
	err := (adminCommand{stdout: &putStdout, stderr: &putStderr}).run(context.Background(), []string{
		"volume-drain-operations",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-action", "put",
		"-pool-id", "object-pool",
		"-source-volume-id", "18a00001",
		"-target-volume-id", "18a00002",
		"-owner-id", "gateway-a",
		"-status", "retry_pending",
		"-cursor", "bucket-1/logs/a.txt#v1",
		"-attempt", "bucket_id=bucket-1,key=logs/a.txt,version_id=v1,source_segment_id=seg-a,source_volume_id=18a00001,target_segment_id=seg-b,target_volume_id=18a00002,status=copied",
		"-attempt", "bucket_id=bucket-1,key=logs/b.txt,version_id=v2,source_segment_id=seg-c,source_volume_id=18a00001,status=protected,protected=true,error=object_lock",
		"-attempt", "bucket_id=bucket-1,key=logs/c.txt,version_id=v3,source_segment_id=seg-d,source_volume_id=18a00001,status=retryable,retryable=true,error=target_unavailable",
		"-attempt", "bucket_id=bucket-1,key=logs/d.txt,version_id=v4,source_segment_id=seg-e,source_volume_id=18a00001,target_segment_id=seg-f,target_volume_id=18a00002,status=queued_gc",
	})
	if err != nil {
		t.Fatalf("volume-drain-operations put error = %v stderr=%s", err, putStderr.String())
	}
	var putBody volumeDrainOperationsOutput
	if err := json.Unmarshal(putStdout.Bytes(), &putBody); err != nil {
		t.Fatalf("put json decode: %v body=%s", err, putStdout.String())
	}
	if putBody.SchemaVersion != "namros.admin.volume_drain_operations.v1" || putBody.Action != "put" || putBody.Operation == nil {
		t.Fatalf("put envelope = %+v", putBody)
	}
	if got := *putBody.Operation; got.OperationID == "" || got.Status != "retry_pending" || got.Scanned != 4 || got.Copied != 1 || got.Skipped != 1 || got.Protected != 1 || got.Retryable != 1 || len(got.Attempts) != 4 || got.Attempts[3].Status != "queued_gc" {
		t.Fatalf("put operation = %+v", got)
	}

	var listStdout bytes.Buffer
	var listStderr bytes.Buffer
	err = (adminCommand{stdout: &listStdout, stderr: &listStderr}).run(context.Background(), []string{
		"volume-drain-operations",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-action", "list",
		"-source-volume-id", "18a00001",
		"-status", "retry_pending",
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("volume-drain-operations list error = %v stderr=%s", err, listStderr.String())
	}
	var listBody volumeDrainOperationsOutput
	if err := json.Unmarshal(listStdout.Bytes(), &listBody); err != nil {
		t.Fatalf("list json decode: %v body=%s", err, listStdout.String())
	}
	if listBody.Action != "list" || listBody.Limit != 10 || listBody.SourceVolumeID != "18a00001" || listBody.Status != "retry_pending" {
		t.Fatalf("list envelope = %+v", listBody)
	}
	if len(listBody.Operations) != 1 || listBody.Operations[0].OperationID != putBody.Operation.OperationID {
		t.Fatalf("list operations = %+v, want %q", listBody.Operations, putBody.Operation.OperationID)
	}
	if got := listBody.Operations[0].Attempts[0]; got.SourceVolumeID != "18a00001" || got.TargetVolumeID != "18a00002" || got.Status != "copied" {
		t.Fatalf("listed attempt = %+v", got)
	}
}

func TestWorkerControlCommandPausesGetsAndResumes(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")

	var pauseStdout bytes.Buffer
	var pauseStderr bytes.Buffer
	err := (adminCommand{stdout: &pauseStdout, stderr: &pauseStderr}).run(context.Background(), []string{
		"worker-control",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-worker-kind", "lifecycle",
		"-shard-id", "buckets",
		"-action", "pause",
		"-reason", "maintenance",
		"-updated-by", "operator-a",
	})
	if err != nil {
		t.Fatalf("worker-control pause error = %v stderr=%s", err, pauseStderr.String())
	}
	pause := decodeWorkerControlCommandOutput(t, pauseStdout.Bytes())
	if pause.SchemaVersion != "namros.admin.worker_control.v1" || pause.Action != "pause" || pause.Control.State != "paused" || pause.Control.Reason != "maintenance" || pause.Control.UpdatedBy != "operator-a" {
		t.Fatalf("pause output = %+v", pause)
	}

	var getStdout bytes.Buffer
	var getStderr bytes.Buffer
	err = (adminCommand{stdout: &getStdout, stderr: &getStderr}).run(context.Background(), []string{
		"worker-control",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-worker-kind", "lifecycle",
		"-shard-id", "buckets",
		"-action", "get",
	})
	if err != nil {
		t.Fatalf("worker-control get error = %v stderr=%s", err, getStderr.String())
	}
	got := decodeWorkerControlCommandOutput(t, getStdout.Bytes())
	if got.Action != "get" || got.Control.State != "paused" || got.Control.WorkerKind != "lifecycle" || got.Control.ShardID != "buckets" {
		t.Fatalf("get output = %+v", got)
	}

	var resumeStdout bytes.Buffer
	var resumeStderr bytes.Buffer
	err = (adminCommand{stdout: &resumeStdout, stderr: &resumeStderr}).run(context.Background(), []string{
		"worker-control",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-worker-kind", "lifecycle",
		"-shard-id", "buckets",
		"-action", "resume",
		"-reason", "maintenance complete",
		"-updated-by", "operator-b",
	})
	if err != nil {
		t.Fatalf("worker-control resume error = %v stderr=%s", err, resumeStderr.String())
	}
	resume := decodeWorkerControlCommandOutput(t, resumeStdout.Bytes())
	if resume.Action != "resume" || resume.Control.State != "active" || resume.Control.Reason != "maintenance complete" || resume.Control.UpdatedBy != "operator-b" {
		t.Fatalf("resume output = %+v", resume)
	}
}

func decodeWorkerControlCommandOutput(t *testing.T, raw []byte) workerControlCommandOutput {
	t.Helper()
	var body workerControlCommandOutput
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, string(raw))
	}
	return body
}

func TestGCCandidateSeedObjectCommandDetachesObject(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed object) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "gc-seed-bucket",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	segmentRef := storage.SegmentRef{
		SegmentID: "segment-gc-seed",
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-physical",
		},
		Placement: storage.PlacementSnapshot{
			Backend: "sbs-physical",
			Layout:  "sbs-physical-chunks",
			Parameters: map[string]string{
				"volume_id": "18a00001",
			},
			Chunks: []storage.PlacementChunk{{
				VolumeID:    "18a00001",
				SizeBytes:   17,
				LengthBytes: 17,
			}},
		},
	}
	pending, err := repo.BeginPutObject(context.Background(), meta.BeginPutObjectRequest{
		BucketID:    bucket.BucketID,
		Key:         "detached.bin",
		ETag:        `"seed"`,
		ContentType: "application/octet-stream",
		SegmentRef:  segmentRef,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	head, err := repo.CommitObjectVersion(context.Background(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "detached.bin",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(seed object) error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"gc-candidate-seed-object",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket", "gc-seed-bucket",
		"-key", "detached.bin",
		"-version-id", head.VersionID,
		"-reason", "manual_gc",
	})
	if err != nil {
		t.Fatalf("gc-candidate-seed-object error = %v stderr=%s", err, stderr.String())
	}
	var seedBody struct {
		SchemaVersion  string `json:"schema_version"`
		Bucket         string `json:"bucket"`
		Key            string `json:"key"`
		VersionID      string `json:"version_id"`
		CandidateCount int    `json:"candidate_count"`
		Candidates     []struct {
			SegmentID string `json:"segment_id"`
			Reason    string `json:"reason"`
			VolumeID  string `json:"volume_id"`
			Backend   string `json:"backend"`
			Layout    string `json:"layout"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &seedBody); err != nil {
		t.Fatalf("json decode seed: %v body=%s", err, stdout.String())
	}
	if seedBody.SchemaVersion != "namros.admin.gc_candidate_seed_object.v1" || seedBody.Bucket != "gc-seed-bucket" || seedBody.Key != "detached.bin" || seedBody.VersionID != head.VersionID || seedBody.CandidateCount != 1 {
		t.Fatalf("seed body = %+v", seedBody)
	}
	if len(seedBody.Candidates) != 1 || seedBody.Candidates[0].SegmentID != segmentRef.SegmentID || seedBody.Candidates[0].Reason != "manual_gc" || seedBody.Candidates[0].VolumeID != "18a00001" {
		t.Fatalf("seed candidates = %+v", seedBody.Candidates)
	}

	cfg = config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err = openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(verify) error = %v", err)
	}
	if _, err := repo.GetObjectHead(context.Background(), bucket.BucketID, "detached.bin"); !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("GetObjectHead() error = %v, want ErrNotFound", err)
	}
	records, err := repo.ListGCCandidates(context.Background(), meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(records) != 1 || records[0].SegmentID != segmentRef.SegmentID {
		t.Fatalf("stored GC candidates = %+v", records)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("closeRepo(verify) error = %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"gc-candidates",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-limit", "10",
	})
	if err != nil {
		t.Fatalf("gc-candidates error = %v stderr=%s", err, stderr.String())
	}
	var listBody struct {
		SchemaVersion string `json:"schema_version"`
		Limit         int    `json:"limit"`
		Candidates    []struct {
			SegmentID string `json:"segment_id"`
			Reason    string `json:"reason"`
			VolumeID  string `json:"volume_id"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &listBody); err != nil {
		t.Fatalf("json decode list: %v body=%s", err, stdout.String())
	}
	if listBody.SchemaVersion != "namros.admin.gc_candidates.v1" || listBody.Limit != 10 || len(listBody.Candidates) != 1 || listBody.Candidates[0].SegmentID != segmentRef.SegmentID {
		t.Fatalf("list body = %+v", listBody)
	}
}

func TestDedupeRepairCommandOutputsEmptyMemoryRepair(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"dedupe-repair",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("run() error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		Scanned int `json:"scanned"`
		Updated int `json:"updated"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.Scanned != 0 || body.Updated != 0 {
		t.Fatalf("body = %+v, want empty repair", body)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata() error = %v", err)
	}
	defer closeRepo()
	events, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionDedupeRepair,
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(dedupe repair) error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("dedupe repair audit events = %d, want 1: %+v", len(events), events)
	}
	if events[0].Details["limit"] != "5" || events[0].Details["scanned"] != "0" || events[0].Details["updated"] != "0" {
		t.Fatalf("dedupe repair audit details = %+v", events[0].Details)
	}
}

func TestDedupeScrubCommandReportsPendingReleasesAndAudits(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	if _, err := repo.PutSharedObjectRelease(context.Background(), meta.PutSharedObjectReleaseRequest{
		SharedObjectID: "shared-1",
		SegmentRef: storage.SegmentRef{
			SegmentID: "segment-1",
		},
		Reason: storage.DeleteReasonDedupeReplaced,
		Status: model.SharedObjectReleasePending,
	}); err != nil {
		t.Fatalf("PutSharedObjectRelease() error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close seed repo error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"dedupe-scrub",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-shared-object-id", "shared-1",
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("dedupe-scrub error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		AuditEventID string `json:"audit_event_id"`
		Status       string `json:"status"`
		RepairNeeded bool   `json:"repair_needed"`
		DedupeRepair struct {
			Scanned int `json:"scanned"`
			Updated int `json:"updated"`
		} `json:"dedupe_repair"`
		MetadataCounts struct {
			SharedObjectReleases int `json:"shared_object_releases"`
			AuditEvents          int `json:"audit_events"`
		} `json:"metadata_counts"`
		AuditChain struct {
			Sampled       int    `json:"sampled"`
			LastEventID   string `json:"last_event_id"`
			HashesPresent bool   `json:"hashes_present"`
		} `json:"audit_chain"`
		Pending []struct {
			SharedObjectID string `json:"shared_object_id"`
			SegmentID      string `json:"segment_id"`
			Reason         string `json:"reason"`
			Status         string `json:"status"`
		} `json:"pending_shared_object_releases"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.AuditEventID == "" || body.Status != "succeeded" || !body.RepairNeeded {
		t.Fatalf("scrub audit/status/repair = %q/%q/%v", body.AuditEventID, body.Status, body.RepairNeeded)
	}
	if body.DedupeRepair.Scanned != 0 || body.DedupeRepair.Updated != 0 {
		t.Fatalf("scrub repair = %+v, want empty repair", body.DedupeRepair)
	}
	if len(body.Pending) != 1 || body.Pending[0].SharedObjectID != "shared-1" || body.Pending[0].SegmentID != "segment-1" || body.Pending[0].Reason != string(storage.DeleteReasonDedupeReplaced) || body.Pending[0].Status != string(model.SharedObjectReleasePending) {
		t.Fatalf("scrub pending releases = %+v", body.Pending)
	}
	if body.MetadataCounts.SharedObjectReleases != 1 || body.MetadataCounts.AuditEvents != 1 {
		t.Fatalf("scrub metadata counts = %+v", body.MetadataCounts)
	}
	if body.AuditChain.Sampled != 1 || body.AuditChain.LastEventID != body.AuditEventID || !body.AuditChain.HashesPresent {
		t.Fatalf("scrub audit chain = %+v", body.AuditChain)
	}

	repo, closeRepo, err = openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(verify) error = %v", err)
	}
	defer closeRepo()
	events, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionDedupeScrub,
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(dedupe scrub) error = %v", err)
	}
	if len(events) != 1 || events[0].EventID != body.AuditEventID {
		t.Fatalf("dedupe scrub audit events = %+v, want %q", events, body.AuditEventID)
	}
	if events[0].Details["pending_shared_object_releases"] != "1" || events[0].Details["shared_object_id"] != "shared-1" || events[0].Details["repair_needed"] != "true" {
		t.Fatalf("dedupe scrub audit details = %+v", events[0].Details)
	}
}

func TestMetadataExportCommandIncludesOperationalRecords(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	seedOperationalMetadataForExport(t, metadataPath, "kms-backup", model.KMSKeyActive)
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-export",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("metadata-export error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion  int `json:"schema_version"`
		MetadataSchema struct {
			SchemaVersion int `json:"schema_version"`
		} `json:"metadata_schema"`
		GeneratedAt string `json:"generated_at"`
		KMSKeys     []struct {
			KeyID      string `json:"key_id"`
			KeyVersion string `json:"key_version"`
			State      string `json:"state"`
		} `json:"kms_keys"`
		MetadataMigrationOperations []struct {
			OperationID string `json:"operation_id"`
			Status      string `json:"status"`
		} `json:"metadata_migration_operations"`
		DedupeOperations []struct {
			OperationID string `json:"operation_id"`
			Status      string `json:"status"`
		} `json:"dedupe_operations"`
		VolumePools []struct {
			PoolID     string `json:"pool_id"`
			Generation uint64 `json:"generation"`
		} `json:"volume_pools"`
		VolumeDrainOperations []struct {
			OperationID    string `json:"operation_id"`
			SourceVolumeID string `json:"source_volume_id"`
		} `json:"volume_drain_operations"`
		WorkerLeases []struct {
			LeaseID string `json:"lease_id"`
			OwnerID string `json:"owner_id"`
		} `json:"worker_leases"`
		WorkerOperations []struct {
			OperationID string `json:"operation_id"`
			WorkerKind  string `json:"worker_kind"`
		} `json:"worker_operations"`
		AuditEvents []struct {
			EventID string            `json:"event_id"`
			Action  string            `json:"action"`
			Details map[string]string `json:"details"`
		} `json:"audit_events"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != 1 || body.GeneratedAt == "" {
		t.Fatalf("metadata export envelope = %+v", body)
	}
	if body.MetadataSchema.SchemaVersion != meta.CurrentMetadataSchemaVersion {
		t.Fatalf("metadata export schema = %+v", body.MetadataSchema)
	}
	if len(body.MetadataMigrationOperations) != 1 || body.MetadataMigrationOperations[0].Status != string(model.MetadataMigrationOperationSucceeded) {
		t.Fatalf("metadata export migration operations = %+v", body.MetadataMigrationOperations)
	}
	if len(body.DedupeOperations) != 1 || body.DedupeOperations[0].OperationID == "" || body.DedupeOperations[0].Status != "succeeded" {
		t.Fatalf("metadata export dedupe operations = %+v", body.DedupeOperations)
	}
	if len(body.KMSKeys) != 1 || body.KMSKeys[0].KeyID != "kms-backup" || body.KMSKeys[0].KeyVersion != "v1" || body.KMSKeys[0].State != string(model.KMSKeyActive) {
		t.Fatalf("metadata export kms keys = %+v", body.KMSKeys)
	}
	if len(body.AuditEvents) != 1 || body.AuditEvents[0].Action != string(model.AuditActionDedupeAck) || body.AuditEvents[0].Details["tenant_id"] != "tenant-1" {
		t.Fatalf("metadata export audit events = %+v", body.AuditEvents)
	}
	if len(body.VolumePools) != 1 || body.VolumePools[0].PoolID != "object-pool" || body.VolumePools[0].Generation != 7 {
		t.Fatalf("metadata export volume pools = %+v", body.VolumePools)
	}
	if len(body.VolumeDrainOperations) != 1 || body.VolumeDrainOperations[0].SourceVolumeID != "18a00001" {
		t.Fatalf("metadata export volume drain operations = %+v", body.VolumeDrainOperations)
	}
	if len(body.WorkerLeases) != 1 || body.WorkerLeases[0].LeaseID != "gc/orphans" || body.WorkerLeases[0].OwnerID != "test-worker" {
		t.Fatalf("metadata export worker leases = %+v", body.WorkerLeases)
	}
	if len(body.WorkerOperations) != 1 || body.WorkerOperations[0].WorkerKind != "gc" {
		t.Fatalf("metadata export worker operations = %+v", body.WorkerOperations)
	}
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(audit check) error = %v", err)
	}
	defer closeRepo()
	exportAuditEvents, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionAdminMetadataExport,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(metadata export) error = %v", err)
	}
	if len(exportAuditEvents) != 1 || exportAuditEvents[0].Details["limit"] != "5" || exportAuditEvents[0].Details["dedupe_operations"] != "1" || exportAuditEvents[0].Details["kms_keys"] != "1" || exportAuditEvents[0].Details["volume_pools"] != "1" || exportAuditEvents[0].Details["worker_operations"] != "1" {
		t.Fatalf("metadata export admin audit = %+v", exportAuditEvents)
	}
}

func TestMetadataImportCommandDryRunValidatesExport(t *testing.T) {
	metadataPath := filepath.Join(t.TempDir(), "meta")
	seedOperationalMetadataForExport(t, metadataPath, "kms-import", model.KMSKeyDisabled)
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath

	var exportStdout bytes.Buffer
	var exportStderr bytes.Buffer
	err := (testAdminCommand(&exportStdout, &exportStderr)).run(context.Background(), []string{
		"metadata-export",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("metadata-export error = %v stderr=%s", err, exportStderr.String())
	}

	inputPath := filepath.Join(t.TempDir(), "metadata-export.json")
	if err := os.WriteFile(inputPath, exportStdout.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", inputPath, err)
	}

	type metadataImportActionBody struct {
		Collection     string `json:"collection"`
		Operation      string `json:"operation"`
		ImportRecords  int    `json:"import_records"`
		TargetRecords  int    `json:"target_records"`
		Policy         string `json:"policy"`
		PreserveIDs    bool   `json:"preserve_ids"`
		PreserveHashes bool   `json:"preserve_hashes"`
		WriteEnabled   bool   `json:"write_enabled"`
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-import",
		"-input", inputPath,
	})
	if err != nil {
		t.Fatalf("metadata-import error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion  int    `json:"schema_version"`
		DryRun         bool   `json:"dry_run"`
		Valid          bool   `json:"valid"`
		ReadyForApply  bool   `json:"ready_for_apply"`
		Source         string `json:"source"`
		ConflictPolicy string `json:"conflict_policy"`
		Counts         struct {
			MetadataSchema              int `json:"metadata_schema"`
			MetadataMigrationOperations int `json:"metadata_migration_operations"`
			KMSKeys                     int `json:"kms_keys"`
			AuditEvents                 int `json:"audit_events"`
			DedupeOperations            int `json:"dedupe_operations"`
			VolumePools                 int `json:"volume_pools"`
			VolumeDrainOperations       int `json:"volume_drain_operations"`
			WorkerLeases                int `json:"worker_leases"`
			WorkerOperations            int `json:"worker_operations"`
		} `json:"counts"`
		TargetChecked bool                       `json:"target_checked"`
		TargetEmpty   bool                       `json:"target_empty"`
		Actions       []metadataImportActionBody `json:"actions"`
		ApplyPlan     struct {
			Status              string `json:"status"`
			WriteEnabled        bool   `json:"write_enabled"`
			ApplySupported      bool   `json:"apply_supported"`
			Ready               bool   `json:"ready"`
			ConflictPolicy      string `json:"conflict_policy"`
			RequireEmptyTarget  bool   `json:"require_empty_target"`
			PreserveSourceIDs   bool   `json:"preserve_source_ids"`
			PreserveAuditHashes bool   `json:"preserve_audit_hashes"`
			Gates               []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"gates"`
			Limitations []string `json:"limitations"`
		} `json:"apply_plan"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != 1 || !body.DryRun || !body.Valid || !body.ReadyForApply || body.Source != inputPath || body.ConflictPolicy != "fail_if_exists" {
		t.Fatalf("metadata import dry-run envelope = %+v", body)
	}
	if body.Counts.MetadataSchema != 1 || body.Counts.MetadataMigrationOperations != 1 || body.Counts.KMSKeys != 1 || body.Counts.AuditEvents != 1 || body.Counts.DedupeOperations != 1 || body.Counts.VolumePools != 1 || body.Counts.VolumeDrainOperations != 1 || body.Counts.WorkerLeases != 1 || body.Counts.WorkerOperations != 1 {
		t.Fatalf("metadata import dry-run counts = %+v", body.Counts)
	}
	if body.TargetChecked || !body.TargetEmpty {
		t.Fatalf("metadata import target state = checked:%v empty:%v", body.TargetChecked, body.TargetEmpty)
	}
	if len(body.Actions) != 12 {
		t.Fatalf("metadata import actions len = %d, want 12: %+v", len(body.Actions), body.Actions)
	}
	actionByCollection := map[string]metadataImportActionBody{}
	for _, action := range body.Actions {
		actionByCollection[action.Collection] = action
	}
	if actionByCollection["metadata_schema"].Operation != "upsert_schema_marker" || actionByCollection["metadata_schema"].ImportRecords != 1 || !actionByCollection["metadata_schema"].PreserveIDs || actionByCollection["metadata_schema"].WriteEnabled {
		t.Fatalf("metadata import schema action = %+v", actionByCollection["metadata_schema"])
	}
	if actionByCollection["kms_keys"].Operation != "insert_preserve_source_id" || actionByCollection["kms_keys"].ImportRecords != 1 || !actionByCollection["kms_keys"].PreserveIDs || actionByCollection["kms_keys"].PreserveHashes || actionByCollection["kms_keys"].WriteEnabled {
		t.Fatalf("metadata import kms action = %+v", actionByCollection["kms_keys"])
	}
	if actionByCollection["audit_events"].Operation != "insert_preserve_source_id" || actionByCollection["audit_events"].ImportRecords != 1 || !actionByCollection["audit_events"].PreserveIDs || !actionByCollection["audit_events"].PreserveHashes || actionByCollection["audit_events"].WriteEnabled {
		t.Fatalf("metadata import audit action = %+v", actionByCollection["audit_events"])
	}
	if actionByCollection["worker_operations"].ImportRecords != 1 || actionByCollection["volume_pools"].ImportRecords != 1 || actionByCollection["metadata_migration_operations"].ImportRecords != 1 {
		t.Fatalf("metadata import extended actions = %+v", actionByCollection)
	}
	if body.ApplyPlan.Status != "blocked" || body.ApplyPlan.WriteEnabled || !body.ApplyPlan.ApplySupported || body.ApplyPlan.Ready || body.ApplyPlan.ConflictPolicy != "fail_if_exists" || !body.ApplyPlan.PreserveSourceIDs || !body.ApplyPlan.PreserveAuditHashes {
		t.Fatalf("metadata import apply plan = %+v", body.ApplyPlan)
	}
	gates := map[string]string{}
	for _, gate := range body.ApplyPlan.Gates {
		gates[gate.Name] = gate.Status
	}
	if gates["target_checked"] != "blocked" || gates["write_path"] != "passed" || len(body.ApplyPlan.Limitations) == 0 {
		t.Fatalf("metadata import apply gates = %+v limitations=%+v", body.ApplyPlan.Gates, body.ApplyPlan.Limitations)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-import",
		"-input", inputPath,
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
	})
	if err != nil {
		t.Fatalf("metadata-import target check error = %v stderr=%s", err, stderr.String())
	}
	var targetBody struct {
		ReadyForApply bool `json:"ready_for_apply"`
		TargetChecked bool `json:"target_checked"`
		TargetEmpty   bool `json:"target_empty"`
		TargetCounts  struct {
			MetadataSchema              int `json:"metadata_schema"`
			MetadataMigrationOperations int `json:"metadata_migration_operations"`
			KMSKeys                     int `json:"kms_keys"`
			AuditEvents                 int `json:"audit_events"`
			DedupeOperations            int `json:"dedupe_operations"`
			VolumePools                 int `json:"volume_pools"`
			VolumeDrainOperations       int `json:"volume_drain_operations"`
			WorkerLeases                int `json:"worker_leases"`
			WorkerOperations            int `json:"worker_operations"`
		} `json:"target_counts"`
		Conflicts []struct {
			Collection string `json:"collection"`
			Reason     string `json:"reason"`
		} `json:"conflicts"`
		ApplyPlan struct {
			Status       string `json:"status"`
			WriteEnabled bool   `json:"write_enabled"`
			Ready        bool   `json:"ready"`
			Gates        []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"gates"`
		} `json:"apply_plan"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &targetBody); err != nil {
		t.Fatalf("target json decode: %v body=%s", err, stdout.String())
	}
	if targetBody.ReadyForApply || !targetBody.TargetChecked || targetBody.TargetEmpty {
		t.Fatalf("metadata import target readiness = %+v", targetBody)
	}
	if targetBody.TargetCounts.MetadataSchema != 0 || targetBody.TargetCounts.MetadataMigrationOperations != 1 || targetBody.TargetCounts.KMSKeys != 1 || targetBody.TargetCounts.AuditEvents != 2 || targetBody.TargetCounts.DedupeOperations != 1 || targetBody.TargetCounts.VolumePools != 1 || targetBody.TargetCounts.VolumeDrainOperations != 1 || targetBody.TargetCounts.WorkerLeases != 1 || targetBody.TargetCounts.WorkerOperations != 1 {
		t.Fatalf("metadata import target counts = %+v", targetBody.TargetCounts)
	}
	if len(targetBody.Conflicts) == 0 {
		t.Fatalf("metadata import target conflicts empty: %+v", targetBody)
	}
	targetGates := map[string]string{}
	for _, gate := range targetBody.ApplyPlan.Gates {
		targetGates[gate.Name] = gate.Status
	}
	if targetBody.ApplyPlan.Status != "blocked" || targetBody.ApplyPlan.WriteEnabled || targetBody.ApplyPlan.Ready || targetGates["target_checked"] != "passed" || targetGates["target_empty"] != "blocked" || targetGates["write_path"] != "passed" {
		t.Fatalf("metadata import target apply gates = %+v", targetBody.ApplyPlan)
	}

	auditTargetPath := filepath.Join(t.TempDir(), "audit-target")
	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-import",
		"-input", inputPath,
		"-metadata-backend", "pebble",
		"-metadata-path", auditTargetPath,
		"-audit-admin-operation",
	})
	if err != nil {
		t.Fatalf("metadata-import audit target error = %v stderr=%s", err, stderr.String())
	}
	cfg = config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = auditTargetPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(import audit check) error = %v", err)
	}
	defer closeRepo()
	importAuditEvents, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionAdminMetadataImport,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(metadata import) error = %v", err)
	}
	if len(importAuditEvents) != 1 || importAuditEvents[0].Details["ready_for_apply"] != "true" || importAuditEvents[0].Details["dedupe_operations"] != "1" || importAuditEvents[0].Details["kms_keys"] != "1" || importAuditEvents[0].Details["metadata_schema"] != "1" || importAuditEvents[0].Details["worker_operations"] != "1" {
		t.Fatalf("metadata import admin audit = %+v", importAuditEvents)
	}

	applyTargetPath := filepath.Join(t.TempDir(), "apply-target")
	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-import",
		"-input", inputPath,
		"-metadata-backend", "pebble",
		"-metadata-path", applyTargetPath,
		"-dry-run=false",
		"-allow-experimental-apply",
	})
	if err != nil {
		t.Fatalf("metadata-import apply error = %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	type metadataImportApplyCollectionBody struct {
		Collection     string `json:"collection"`
		Status         string `json:"status"`
		RecordsPlanned int    `json:"records_planned"`
		RecordsWritten int    `json:"records_written"`
		PreserveIDs    bool   `json:"preserve_ids"`
		PreserveHashes bool   `json:"preserve_hashes"`
	}
	var applyBody struct {
		DryRun         bool `json:"dry_run"`
		ReadyForApply  bool `json:"ready_for_apply"`
		ApplyRequested bool `json:"apply_requested"`
		ApplyResult    struct {
			Status              string                              `json:"status"`
			WriteEnabled        bool                                `json:"write_enabled"`
			ApplySupported      bool                                `json:"apply_supported"`
			Ready               bool                                `json:"ready"`
			ExperimentalAllowed bool                                `json:"experimental_allowed"`
			RecordsPlanned      int                                 `json:"records_planned"`
			RecordsWritten      int                                 `json:"records_written"`
			Collections         []metadataImportApplyCollectionBody `json:"collections"`
		} `json:"apply_result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &applyBody); err != nil {
		t.Fatalf("apply json decode: %v body=%s", err, stdout.String())
	}
	if applyBody.DryRun || !applyBody.ReadyForApply || !applyBody.ApplyRequested {
		t.Fatalf("metadata import apply envelope = %+v", applyBody)
	}
	if applyBody.ApplyResult.Status != "succeeded" || !applyBody.ApplyResult.WriteEnabled || !applyBody.ApplyResult.ApplySupported || !applyBody.ApplyResult.Ready || !applyBody.ApplyResult.ExperimentalAllowed {
		t.Fatalf("metadata import apply result = %+v", applyBody.ApplyResult)
	}
	if applyBody.ApplyResult.RecordsPlanned != 9 || applyBody.ApplyResult.RecordsWritten != 9 || len(applyBody.ApplyResult.Collections) != 12 {
		t.Fatalf("metadata import apply counts = %+v", applyBody.ApplyResult)
	}
	applyCollectionByName := map[string]metadataImportApplyCollectionBody{}
	for _, collection := range applyBody.ApplyResult.Collections {
		applyCollectionByName[collection.Collection] = collection
	}
	if applyCollectionByName["kms_keys"].Status != "written" || !applyCollectionByName["kms_keys"].PreserveIDs || applyCollectionByName["kms_keys"].PreserveHashes || applyCollectionByName["kms_keys"].RecordsWritten != 1 {
		t.Fatalf("metadata import apply kms collection = %+v", applyCollectionByName["kms_keys"])
	}
	if applyCollectionByName["audit_events"].Status != "written" || !applyCollectionByName["audit_events"].PreserveIDs || !applyCollectionByName["audit_events"].PreserveHashes || applyCollectionByName["audit_events"].RecordsWritten != 1 {
		t.Fatalf("metadata import apply audit collection = %+v", applyCollectionByName["audit_events"])
	}
	if applyCollectionByName["metadata_schema"].Status != "written" || applyCollectionByName["metadata_migration_operations"].RecordsWritten != 1 || applyCollectionByName["volume_pools"].RecordsWritten != 1 || applyCollectionByName["worker_operations"].RecordsWritten != 1 {
		t.Fatalf("metadata import apply extended collections = %+v", applyCollectionByName)
	}
	cfg = config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = applyTargetPath
	applyRepo, closeApplyRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(apply target) error = %v", err)
	}
	defer closeApplyRepo()
	appliedAuditEvents, err := applyRepo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditEvents(apply target) error = %v", err)
	}
	if len(appliedAuditEvents) != 1 || appliedAuditEvents[0].EventHash == "" || appliedAuditEvents[0].Details["tenant_id"] != "tenant-1" {
		t.Fatalf("applied audit events = %+v", appliedAuditEvents)
	}
	appliedDedupeOps, err := applyRepo.ListDedupeOperations(context.Background(), meta.ListDedupeOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListDedupeOperations(apply target) error = %v", err)
	}
	if len(appliedDedupeOps) != 1 || appliedDedupeOps[0].OperationID == "" || appliedDedupeOps[0].Status != model.DedupeOperationSucceeded {
		t.Fatalf("applied dedupe operations = %+v", appliedDedupeOps)
	}
	appliedKMSKeys, err := applyRepo.ListKMSKeys(context.Background(), meta.ListKMSKeysRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListKMSKeys(apply target) error = %v", err)
	}
	if len(appliedKMSKeys) != 1 || appliedKMSKeys[0].KeyID != "kms-import" || appliedKMSKeys[0].KeyVersion != "v1" || appliedKMSKeys[0].State != model.KMSKeyDisabled {
		t.Fatalf("applied kms keys = %+v", appliedKMSKeys)
	}
	appliedSchema, err := applyRepo.GetMetadataSchema(context.Background())
	if err != nil {
		t.Fatalf("GetMetadataSchema(apply target) error = %v", err)
	}
	if appliedSchema.SchemaVersion != meta.CurrentMetadataSchemaVersion {
		t.Fatalf("applied metadata schema = %+v", appliedSchema)
	}
	appliedMigrations, err := applyRepo.ListMetadataMigrationOperations(context.Background(), meta.ListMetadataMigrationOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListMetadataMigrationOperations(apply target) error = %v", err)
	}
	if len(appliedMigrations) != 1 || appliedMigrations[0].Status != model.MetadataMigrationOperationSucceeded || len(appliedMigrations[0].Steps) != 1 {
		t.Fatalf("applied metadata migrations = %+v", appliedMigrations)
	}
	appliedPools, err := applyRepo.ListVolumePools(context.Background(), meta.ListVolumePoolsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListVolumePools(apply target) error = %v", err)
	}
	if len(appliedPools) != 1 || appliedPools[0].PoolID != "object-pool" || len(appliedPools[0].Members) != 1 || appliedPools[0].Members[0].DataEndpoint != "sbs-data-a:9444" {
		t.Fatalf("applied volume pools = %+v", appliedPools)
	}
	appliedDrainOps, err := applyRepo.ListVolumeDrainOperations(context.Background(), meta.ListVolumeDrainOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListVolumeDrainOperations(apply target) error = %v", err)
	}
	if len(appliedDrainOps) != 1 || appliedDrainOps[0].SourceVolumeID != "18a00001" || appliedDrainOps[0].Copied != 1 {
		t.Fatalf("applied drain operations = %+v", appliedDrainOps)
	}
	appliedLeases, err := applyRepo.ListWorkerLeases(context.Background(), meta.ListWorkerLeasesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerLeases(apply target) error = %v", err)
	}
	if len(appliedLeases) != 1 || appliedLeases[0].LeaseID != "gc/orphans" || appliedLeases[0].OwnerID != "test-worker" {
		t.Fatalf("applied worker leases = %+v", appliedLeases)
	}
	appliedWorkerOps, err := applyRepo.ListWorkerOperations(context.Background(), meta.ListWorkerOperationsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListWorkerOperations(apply target) error = %v", err)
	}
	if len(appliedWorkerOps) != 1 || appliedWorkerOps[0].WorkerKind != "gc" || appliedWorkerOps[0].Processed != 2 {
		t.Fatalf("applied worker operations = %+v", appliedWorkerOps)
	}
}

func TestMetadataImportCommandDryRunRejectsDuplicateSourceIDs(t *testing.T) {
	inputPath := filepath.Join(t.TempDir(), "metadata-export.json")
	input := []byte(`{
		"schema_version": 1,
		"generated_at": "2026-07-06T00:00:00Z",
		"limit": 1000,
		"audit_events": [
			{"event_id": "audit-1", "action": "dedupe_ack", "event_hash": "hash-1", "created_at": "2026-07-06T00:00:00Z"},
			{"event_id": "audit-1", "action": "dedupe_repair", "event_hash": "hash-2", "created_at": "2026-07-06T00:00:01Z"}
		]
	}`)
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", inputPath, err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"metadata-import",
		"-input", inputPath,
	})
	if err != nil {
		t.Fatalf("metadata-import error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		ReadyForApply bool `json:"ready_for_apply"`
		Conflicts     []struct {
			Collection string `json:"collection"`
			ID         string `json:"id"`
			Reason     string `json:"reason"`
		} `json:"conflicts"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.ReadyForApply {
		t.Fatalf("metadata import ready_for_apply = true, want false: %+v", body)
	}
	if len(body.Conflicts) != 1 || body.Conflicts[0].Collection != "audit_events" || body.Conflicts[0].ID != "audit-1" || body.Conflicts[0].Reason != "duplicate_source_id" {
		t.Fatalf("metadata import conflicts = %+v", body.Conflicts)
	}
}

func TestKMSKeyCommandsPutListAndAudit(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"kms-key-put",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-key-id", "kms-cli",
		"-key-version", "v2",
		"-state", "pending_deletion",
	})
	if err != nil {
		t.Fatalf("kms-key-put error = %v stderr=%s", err, stderr.String())
	}
	var putBody struct {
		AuditEventID string `json:"audit_event_id"`
		Key          struct {
			KeyID      string `json:"key_id"`
			KeyVersion string `json:"key_version"`
			State      string `json:"state"`
			CreatedAt  string `json:"created_at"`
			UpdatedAt  string `json:"updated_at"`
		} `json:"key"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &putBody); err != nil {
		t.Fatalf("put json decode: %v body=%s", err, stdout.String())
	}
	if putBody.AuditEventID == "" || putBody.Key.KeyID != "kms-cli" || putBody.Key.KeyVersion != "v2" || putBody.Key.State != string(model.KMSKeyPendingDeletion) || putBody.Key.CreatedAt == "" || putBody.Key.UpdatedAt == "" {
		t.Fatalf("kms-key-put body = %+v", putBody)
	}

	stdout.Reset()
	stderr.Reset()
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"kms-key-list",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-limit", "5",
	})
	if err != nil {
		t.Fatalf("kms-key-list error = %v stderr=%s", err, stderr.String())
	}
	var listBody struct {
		Keys []struct {
			KeyID      string `json:"key_id"`
			KeyVersion string `json:"key_version"`
			State      string `json:"state"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &listBody); err != nil {
		t.Fatalf("list json decode: %v body=%s", err, stdout.String())
	}
	if len(listBody.Keys) != 1 || listBody.Keys[0].KeyID != "kms-cli" || listBody.Keys[0].KeyVersion != "v2" || listBody.Keys[0].State != string(model.KMSKeyPendingDeletion) {
		t.Fatalf("kms-key-list body = %+v", listBody)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(kms verify) error = %v", err)
	}
	defer closeRepo()
	record, err := repo.GetKMSKey(context.Background(), "kms-cli")
	if err != nil {
		t.Fatalf("GetKMSKey() error = %v", err)
	}
	if record.State != model.KMSKeyPendingDeletion || record.KeyVersion != "v2" {
		t.Fatalf("stored kms key = %+v", record)
	}
	events, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionAdminKMSKeyPut,
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(kms put) error = %v", err)
	}
	if len(events) != 1 || events[0].EventID != putBody.AuditEventID || events[0].Details["state"] != string(model.KMSKeyPendingDeletion) || events[0].Details["decrypt_admission"] != "denied" {
		t.Fatalf("kms put audit events = %+v", events)
	}
}

func TestComplianceEvidenceCommandReportsObjectLockState(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID:          "tenant-1",
		Name:              "locked-bucket",
		Region:            "us-east-1",
		ObjectLockEnabled: true,
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	retainUntil := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	pending, err := repo.BeginPutObject(context.Background(), meta.BeginPutObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "locked.txt",
		ETag:     `"locked"`,
		SegmentRef: storage.SegmentRef{
			SegmentID: "segment-locked",
			SizeBytes: 6,
		},
		ObjectLockRetention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeGovernance,
			RetainUntilDate: retainUntil,
		},
		ObjectLockLegalHold: model.ObjectLockLegalHoldOn,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	head, err := repo.CommitObjectVersion(context.Background(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "locked.txt",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	auditEvent, err := repo.PutAdminAuditEvent(context.Background(), meta.PutAdminAuditEventRequest{
		Action:    model.AuditActionPutObjectRetention,
		BucketID:  bucket.BucketID,
		Key:       "locked.txt",
		VersionID: head.VersionID,
		Details: map[string]string{
			"mode":              string(model.ObjectLockModeGovernance),
			"retain_until_date": retainUntil.Format(time.RFC3339Nano),
		},
		Audit: meta.AuditContext{
			RequestID: "request-1",
			Reason:    "test compliance evidence",
		},
	})
	if err != nil {
		t.Fatalf("PutAdminAuditEvent() error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close seed repo error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-evidence",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket-id", bucket.BucketID,
		"-key", "locked.txt",
		"-version-id", head.VersionID,
	})
	if err != nil {
		t.Fatalf("compliance-evidence error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion int    `json:"schema_version"`
		BucketID      string `json:"bucket_id"`
		Key           string `json:"key"`
		VersionID     string `json:"version_id"`
		Package       struct {
			PackageID     string `json:"package_id"`
			SchemaVersion int    `json:"schema_version"`
			EvidenceType  string `json:"evidence_type"`
			GeneratedAt   string `json:"generated_at"`
		} `json:"package"`
		Scope struct {
			BucketID  string `json:"bucket_id"`
			Key       string `json:"key"`
			VersionID string `json:"version_id"`
		} `json:"scope"`
		Sections []struct {
			Name    string            `json:"name"`
			Status  string            `json:"status"`
			Records int               `json:"records"`
			Summary map[string]string `json:"summary"`
		} `json:"sections"`
		Limitations []struct {
			Code string `json:"code"`
		} `json:"limitations"`
		AccessKey struct {
			Available                bool     `json:"available"`
			Mode                     string   `json:"mode"`
			RootAccessKeyID          string   `json:"root_access_key_id"`
			RootPrincipalEnabled     bool     `json:"root_principal_enabled"`
			RequestPrincipalCaptured bool     `json:"request_principal_captured"`
			PolicyDecisionCaptured   bool     `json:"policy_decision_captured"`
			Notes                    []string `json:"notes"`
		} `json:"access_key_evidence"`
		EncryptionKey struct {
			Available                     bool     `json:"available"`
			EvidenceLevel                 string   `json:"evidence_level"`
			DefaultEncryption             string   `json:"default_encryption"`
			SSES3Configured               bool     `json:"sse_s3_configured"`
			SSEKMSConfigured              bool     `json:"sse_kms_configured"`
			KeyIDCaptured                 bool     `json:"key_id_captured"`
			KeyVersionCaptured            bool     `json:"key_version_captured"`
			RotationEvidenceCaptured      bool     `json:"rotation_evidence_captured"`
			RevocationEvidenceCaptured    bool     `json:"revocation_evidence_captured"`
			DeleteAdmissionTiedToKeyState bool     `json:"delete_admission_tied_to_key_state"`
			Notes                         []string `json:"notes"`
		} `json:"encryption_key_evidence"`
		TimeSource struct {
			Available            bool     `json:"available"`
			GeneratedAt          string   `json:"generated_at"`
			Clock                string   `json:"clock"`
			Authority            string   `json:"authority"`
			AuthorityConfigured  bool     `json:"authority_configured"`
			Timezone             string   `json:"timezone"`
			DriftStatus          string   `json:"drift_status"`
			DriftMeasured        bool     `json:"drift_measured"`
			FailClosedPolicy     string   `json:"fail_closed_policy"`
			FailClosedConfigured bool     `json:"fail_closed_configured"`
			Notes                []string `json:"notes"`
		} `json:"time_source_evidence"`
		Retention struct {
			Mode            string `json:"mode"`
			RetainUntilDate string `json:"retain_until_date"`
		} `json:"retention"`
		LegalHold   string `json:"legal_hold"`
		AuditEvents []struct {
			EventID string `json:"event_id"`
		} `json:"audit_events"`
		AuditChain struct {
			Events         int    `json:"events"`
			HashesVerified bool   `json:"hashes_verified"`
			Contiguous     bool   `json:"contiguous"`
			FirstEventID   string `json:"first_event_id"`
			LastEventID    string `json:"last_event_id"`
		} `json:"audit_chain"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != 1 || body.BucketID != bucket.BucketID || body.Key != "locked.txt" || body.VersionID != head.VersionID {
		t.Fatalf("compliance evidence identity = %+v", body)
	}
	if body.Package.PackageID == "" || body.Package.SchemaVersion != 1 || body.Package.EvidenceType != "retention_audit_chain" || body.Package.GeneratedAt == "" {
		t.Fatalf("compliance evidence package = %+v", body.Package)
	}
	if body.Scope.BucketID != bucket.BucketID || body.Scope.Key != "locked.txt" || body.Scope.VersionID != head.VersionID {
		t.Fatalf("compliance evidence scope = %+v", body.Scope)
	}
	sections := map[string]struct {
		Status  string
		Records int
		Summary map[string]string
	}{}
	for _, section := range body.Sections {
		sections[section.Name] = struct {
			Status  string
			Records int
			Summary map[string]string
		}{Status: section.Status, Records: section.Records, Summary: section.Summary}
	}
	if sections["access_key"].Status != "partial" || sections["access_key"].Records != 1 || sections["access_key"].Summary["root_access_key_id"] != config.DefaultRootAccessKeyID {
		t.Fatalf("compliance evidence access key section = %+v", body.Sections)
	}
	if sections["encryption_key"].Status != "partial" || sections["encryption_key"].Records != 1 || sections["encryption_key"].Summary["default_encryption"] != "none" || sections["encryption_key"].Summary["sse_kms_configured"] != "false" {
		t.Fatalf("compliance evidence encryption key section = %+v", body.Sections)
	}
	if sections["time_source"].Status != "partial" || sections["time_source"].Records != 1 || sections["time_source"].Summary["clock"] != "local_system_clock" || sections["time_source"].Summary["authority_configured"] != "false" {
		t.Fatalf("compliance evidence time source section = %+v", body.Sections)
	}
	if sections["retention"].Status != "available" || sections["legal_hold"].Status != "available" || sections["audit_chain"].Records != 1 {
		t.Fatalf("compliance evidence sections = %+v", body.Sections)
	}
	if len(body.Limitations) == 0 {
		t.Fatalf("compliance evidence limitations empty")
	}
	if !body.AccessKey.Available || body.AccessKey.Mode != "bootstrap_root" || body.AccessKey.RootAccessKeyID != config.DefaultRootAccessKeyID || !body.AccessKey.RootPrincipalEnabled {
		t.Fatalf("compliance evidence access key = %+v", body.AccessKey)
	}
	if body.AccessKey.RequestPrincipalCaptured || body.AccessKey.PolicyDecisionCaptured || len(body.AccessKey.Notes) == 0 {
		t.Fatalf("compliance evidence access key capture flags = %+v", body.AccessKey)
	}
	if !body.EncryptionKey.Available || body.EncryptionKey.EvidenceLevel != "posture_only" || body.EncryptionKey.DefaultEncryption != "none" || body.EncryptionKey.SSEKMSConfigured || body.EncryptionKey.KeyIDCaptured || body.EncryptionKey.DeleteAdmissionTiedToKeyState || len(body.EncryptionKey.Notes) == 0 {
		t.Fatalf("compliance evidence encryption key = %+v", body.EncryptionKey)
	}
	if !body.TimeSource.Available || body.TimeSource.GeneratedAt != body.Package.GeneratedAt || body.TimeSource.Timezone != "UTC" || body.TimeSource.AuthorityConfigured || body.TimeSource.DriftMeasured || body.TimeSource.FailClosedConfigured || body.TimeSource.DriftStatus != "not_measured" || len(body.TimeSource.Notes) == 0 {
		t.Fatalf("compliance evidence time source = %+v package=%+v", body.TimeSource, body.Package)
	}
	limitationCodes := map[string]bool{}
	for _, limitation := range body.Limitations {
		limitationCodes[limitation.Code] = true
	}
	if !limitationCodes["access_principal_capture_not_included"] || !limitationCodes["access_policy_decision_not_included"] || limitationCodes["access_key_evidence_not_included"] {
		t.Fatalf("compliance evidence limitation codes = %+v", body.Limitations)
	}
	if !limitationCodes["time_authority_not_configured"] || !limitationCodes["clock_drift_measurement_not_included"] || !limitationCodes["retention_clock_fail_closed_not_configured"] || limitationCodes["time_source_evidence_not_included"] {
		t.Fatalf("compliance evidence time limitation codes = %+v", body.Limitations)
	}
	if !limitationCodes["sse_kms_evidence_not_included"] || !limitationCodes["key_rotation_evidence_not_included"] || !limitationCodes["key_revocation_delete_admission_not_included"] || limitationCodes["encryption_key_evidence_not_included"] {
		t.Fatalf("compliance evidence encryption limitation codes = %+v", body.Limitations)
	}
	if body.Retention.Mode != string(model.ObjectLockModeGovernance) || body.Retention.RetainUntilDate != retainUntil.Format(time.RFC3339Nano) {
		t.Fatalf("compliance evidence retention = %+v", body.Retention)
	}
	if body.LegalHold != string(model.ObjectLockLegalHoldOn) {
		t.Fatalf("compliance evidence legal hold = %q", body.LegalHold)
	}
	if len(body.AuditEvents) != 1 || body.AuditEvents[0].EventID != auditEvent.EventID {
		t.Fatalf("compliance evidence audit events = %+v, want %q", body.AuditEvents, auditEvent.EventID)
	}
	if body.AuditChain.Events != 1 || !body.AuditChain.HashesVerified || !body.AuditChain.Contiguous || body.AuditChain.FirstEventID != auditEvent.EventID || body.AuditChain.LastEventID != auditEvent.EventID {
		t.Fatalf("compliance evidence audit chain = %+v", body.AuditChain)
	}
	cfg = config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err = openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(compliance audit check) error = %v", err)
	}
	defer closeRepo()
	evidenceAuditEvents, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{
		Action: model.AuditActionAdminComplianceEvidence,
	})
	if err != nil {
		t.Fatalf("ListAuditEvents(compliance evidence) error = %v", err)
	}
	if len(evidenceAuditEvents) != 1 || evidenceAuditEvents[0].Details["bucket_id"] != bucket.BucketID || evidenceAuditEvents[0].Details["hashes_verified"] != "true" {
		t.Fatalf("compliance evidence admin audit = %+v", evidenceAuditEvents)
	}
}

func TestComplianceEvidenceCommandCapturesConfiguredTimeSource(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-evidence",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket-id", "bucket-time",
		"-time-authority", "ntp:time.example.com",
		"-time-drift-status", "measured_ok",
		"-time-fail-closed-policy", "enabled",
		"-audit-admin-operation=false",
	})
	if err != nil {
		t.Fatalf("compliance-evidence time source error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		TimeSource struct {
			Authority            string `json:"authority"`
			AuthorityConfigured  bool   `json:"authority_configured"`
			DriftStatus          string `json:"drift_status"`
			DriftMeasured        bool   `json:"drift_measured"`
			FailClosedPolicy     string `json:"fail_closed_policy"`
			FailClosedConfigured bool   `json:"fail_closed_configured"`
		} `json:"time_source_evidence"`
		Sections []struct {
			Name    string            `json:"name"`
			Summary map[string]string `json:"summary"`
		} `json:"sections"`
		Limitations []struct {
			Code string `json:"code"`
		} `json:"limitations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.TimeSource.Authority != "ntp:time.example.com" || !body.TimeSource.AuthorityConfigured || body.TimeSource.DriftStatus != "measured_ok" || !body.TimeSource.DriftMeasured || body.TimeSource.FailClosedPolicy != "enabled" || !body.TimeSource.FailClosedConfigured {
		t.Fatalf("configured time source = %+v", body.TimeSource)
	}
	foundSection := false
	for _, section := range body.Sections {
		if section.Name == "time_source" {
			foundSection = true
			if section.Summary["authority_configured"] != "true" || section.Summary["drift_measured"] != "true" || section.Summary["fail_closed_configured"] != "true" {
				t.Fatalf("time source section = %+v", section)
			}
		}
	}
	if !foundSection {
		t.Fatalf("time source section missing: %+v", body.Sections)
	}
	for _, limitation := range body.Limitations {
		switch limitation.Code {
		case "time_authority_not_configured", "clock_drift_measurement_not_included", "retention_clock_fail_closed_not_configured", "time_source_evidence_not_included":
			t.Fatalf("configured time source limitation should be absent: %+v", body.Limitations)
		}
	}
}

func TestComplianceEvidenceCommandReportsAccessHistory(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucketID := "bucket-access-history"
	principal := model.AuditPrincipal{
		TenantID:    "root",
		AccessKeyID: cfg.RootAccessKeyID,
		DisplayName: "root",
		Root:        true,
	}
	for _, seed := range []struct {
		action model.AuditAction
		key    string
		detail map[string]string
	}{
		{action: model.AuditActionHeadObject, key: "object.txt", detail: map[string]string{"size_bytes": "11"}},
		{action: model.AuditActionGetObject, key: "object.txt", detail: map[string]string{"range": "bytes=0-4"}},
		{action: model.AuditActionListObjects, detail: map[string]string{"api": "ListObjectsV2", "key_count": "1"}},
	} {
		seed.detail["policy_decision"] = "allowed"
		seed.detail["policy_decision_source"] = "root_principal"
		seed.detail["session_type"] = "access_key"
		seed.detail["principal_access_key_id"] = cfg.RootAccessKeyID
		if _, err := repo.PutAdminAuditEvent(context.Background(), meta.PutAdminAuditEventRequest{
			Action:   seed.action,
			BucketID: bucketID,
			Key:      seed.key,
			Details:  seed.detail,
			Audit: meta.AuditContext{
				RequestID: "request-" + string(seed.action),
				Reason:    "test access history",
				Principal: principal,
			},
		}); err != nil {
			t.Fatalf("PutAdminAuditEvent(%s) error = %v", seed.action, err)
		}
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close seed repo error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-evidence",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket-id", bucketID,
		"-audit-admin-operation=false",
	})
	if err != nil {
		t.Fatalf("compliance-evidence error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		AccessKey struct {
			RequestPrincipalCaptured bool `json:"request_principal_captured"`
			PolicyDecisionCaptured   bool `json:"policy_decision_captured"`
			SessionContextCaptured   bool `json:"session_context_captured"`
		} `json:"access_key_evidence"`
		AccessHistory struct {
			Available              bool   `json:"available"`
			EvidenceLevel          string `json:"evidence_level"`
			TotalReadEvents        int    `json:"total_read_events"`
			GetObjectEvents        int    `json:"get_object_events"`
			HeadObjectEvents       int    `json:"head_object_events"`
			ListObjectsEvents      int    `json:"list_objects_events"`
			PrincipalCaptured      bool   `json:"principal_captured"`
			RequestIDCaptured      bool   `json:"request_id_captured"`
			PolicyDecisionCaptured bool   `json:"policy_decision_captured"`
			SessionContextCaptured bool   `json:"session_context_captured"`
		} `json:"access_history_evidence"`
		Sections []struct {
			Name    string            `json:"name"`
			Records int               `json:"records"`
			Summary map[string]string `json:"summary"`
		} `json:"sections"`
		Limitations []struct {
			Code string `json:"code"`
		} `json:"limitations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if !body.AccessKey.RequestPrincipalCaptured || !body.AccessKey.PolicyDecisionCaptured || !body.AccessKey.SessionContextCaptured {
		t.Fatalf("access key evidence did not inherit access capture: %+v", body.AccessKey)
	}
	if !body.AccessHistory.Available || body.AccessHistory.EvidenceLevel != "read_audit_chain" || body.AccessHistory.TotalReadEvents != 3 ||
		body.AccessHistory.GetObjectEvents != 1 || body.AccessHistory.HeadObjectEvents != 1 || body.AccessHistory.ListObjectsEvents != 1 ||
		!body.AccessHistory.PrincipalCaptured || !body.AccessHistory.RequestIDCaptured ||
		!body.AccessHistory.PolicyDecisionCaptured || !body.AccessHistory.SessionContextCaptured {
		t.Fatalf("access history evidence = %+v", body.AccessHistory)
	}
	sectionRecords := 0
	for _, section := range body.Sections {
		if section.Name == "access_history" {
			sectionRecords = section.Records
			if section.Summary["principal_captured"] != "true" || section.Summary["request_id_captured"] != "true" ||
				section.Summary["policy_decision_captured"] != "true" || section.Summary["session_context_captured"] != "true" {
				t.Fatalf("access history section = %+v", section)
			}
		}
	}
	if sectionRecords != 3 {
		t.Fatalf("access history section records = %d sections=%+v", sectionRecords, body.Sections)
	}
	for _, limitation := range body.Limitations {
		switch limitation.Code {
		case "access_principal_capture_not_included", "access_policy_decision_not_included", "access_session_context_not_included":
			t.Fatalf("access capture limitation should be absent: %+v", body.Limitations)
		}
	}
}

func TestComplianceEvidenceBuildFiltersAuditEventsByTimeRange(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
}

func TestComplianceEvidenceCommandReportsSSEKMSObjectMetadata(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(seed) error = %v", err)
	}
	bucket, err := repo.CreateBucket(context.Background(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "kms-evidence",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if _, err := repo.PutKMSKey(context.Background(), meta.PutKMSKeyRequest{
		KeyID:      "kms-key-evidence",
		KeyVersion: "v1",
		State:      model.KMSKeyActive,
	}); err != nil {
		t.Fatalf("PutKMSKey() error = %v", err)
	}
	pending, err := repo.BeginPutObject(context.Background(), meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "kms.txt",
		SizeBytes: 3,
		ETag:      `"etag-kms"`,
		ServerSideEncryption: model.ServerSideEncryption{
			Algorithm: model.ServerSideEncryptionAWSKMS,
			KeyID:     "kms-key-evidence",
		},
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	head, err := repo.CommitObjectVersion(context.Background(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "kms.txt",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if err := closeRepo(); err != nil {
		t.Fatalf("close seed repo error = %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err = (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-evidence",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-bucket-id", bucket.BucketID,
		"-key", "kms.txt",
		"-version-id", head.VersionID,
		"-audit-admin-operation=false",
	})
	if err != nil {
		t.Fatalf("compliance-evidence error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		EncryptionKey struct {
			Available                     bool   `json:"available"`
			EvidenceLevel                 string `json:"evidence_level"`
			DefaultEncryption             string `json:"default_encryption"`
			SSEKMSConfigured              bool   `json:"sse_kms_configured"`
			KeyIDCaptured                 bool   `json:"key_id_captured"`
			KeyVersionCaptured            bool   `json:"key_version_captured"`
			KeyStateCaptured              bool   `json:"key_state_captured"`
			KeyState                      string `json:"key_state"`
			DecryptAdmission              string `json:"decrypt_admission"`
			DeleteAdmission               string `json:"delete_admission"`
			DeleteAdmissionTiedToKeyState bool   `json:"delete_admission_tied_to_key_state"`
		} `json:"encryption_key_evidence"`
		Sections []struct {
			Name    string            `json:"name"`
			Summary map[string]string `json:"summary"`
		} `json:"sections"`
		Limitations []struct {
			Code string `json:"code"`
		} `json:"limitations"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if !body.EncryptionKey.Available || body.EncryptionKey.EvidenceLevel != "object_version" || body.EncryptionKey.DefaultEncryption != "aws:kms" ||
		!body.EncryptionKey.SSEKMSConfigured || !body.EncryptionKey.KeyIDCaptured || body.EncryptionKey.KeyVersionCaptured ||
		!body.EncryptionKey.KeyStateCaptured || body.EncryptionKey.KeyState != string(model.KMSKeyActive) ||
		body.EncryptionKey.DecryptAdmission != "allowed" || body.EncryptionKey.DeleteAdmission != "allowed" ||
		!body.EncryptionKey.DeleteAdmissionTiedToKeyState {
		t.Fatalf("encryption key evidence = %+v", body.EncryptionKey)
	}
	foundSection := false
	for _, section := range body.Sections {
		if section.Name == "encryption_key" {
			foundSection = true
			if section.Summary["evidence_level"] != "object_version" || section.Summary["sse_kms_configured"] != "true" ||
				section.Summary["key_id_captured"] != "true" || section.Summary["key_state"] != string(model.KMSKeyActive) ||
				section.Summary["decrypt_admission"] != "allowed" || section.Summary["delete_admission"] != "allowed" {
				t.Fatalf("encryption section = %+v", section)
			}
		}
	}
	if !foundSection {
		t.Fatalf("encryption section missing: %+v", body.Sections)
	}
	for _, limitation := range body.Limitations {
		switch limitation.Code {
		case "sse_kms_evidence_not_included", "key_revocation_delete_admission_not_included":
			t.Fatalf("KMS key-state limitation should be absent: %+v", body.Limitations)
		}
	}
}

func TestComplianceProfilePlanCommandOutputsDryRunProfile(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-profile-plan",
		"-profile-id", "finra-books",
		"-regulation", "finra",
		"-record-class", "broker_dealer_books",
		"-bucket-id", "bucket-1",
		"-prefix", "books/",
		"-retention-mode", "COMPLIANCE",
		"-retention-years", "7",
	})
	if err != nil {
		t.Fatalf("compliance-profile-plan error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion int  `json:"schema_version"`
		DryRun        bool `json:"dry_run"`
		Profile       struct {
			ProfileID   string `json:"profile_id"`
			Regulation  string `json:"regulation"`
			RecordClass string `json:"record_class"`
			Scope       struct {
				BucketID string `json:"bucket_id"`
				Prefix   string `json:"prefix"`
			} `json:"scope"`
			Retention struct {
				Mode  string `json:"mode"`
				Years int    `json:"years"`
			} `json:"retention"`
			GovernanceBypassPolicy string `json:"governance_bypass_policy"`
			EvidenceExportPolicy   string `json:"evidence_export_policy"`
		} `json:"profile"`
		Validation struct {
			Valid bool `json:"valid"`
		} `json:"validation"`
		Attachment struct {
			Operation        string `json:"operation"`
			TargetObjectLock struct {
				Required         bool `json:"required"`
				DefaultRetention struct {
					Mode  string `json:"mode"`
					Years int    `json:"years"`
				} `json:"default_retention"`
			} `json:"target_object_lock"`
			RetentionWeakeningCheck struct {
				Checked bool `json:"checked"`
			} `json:"retention_weakening_check"`
		} `json:"attachment"`
		ApplyPlan struct {
			Status         string `json:"status"`
			WriteEnabled   bool   `json:"write_enabled"`
			ApplySupported bool   `json:"apply_supported"`
			AttachReady    bool   `json:"attach_ready"`
			Gates          []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"gates"`
		} `json:"apply_plan"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != 1 || !body.DryRun || !body.Validation.Valid {
		t.Fatalf("compliance profile envelope = %+v", body)
	}
	if body.Profile.ProfileID != "finra-books" || body.Profile.Regulation != "FINRA" || body.Profile.RecordClass != "broker_dealer_books" {
		t.Fatalf("compliance profile identity = %+v", body.Profile)
	}
	if body.Profile.Scope.BucketID != "bucket-1" || body.Profile.Scope.Prefix != "books/" || body.Profile.Retention.Mode != "COMPLIANCE" || body.Profile.Retention.Years != 7 {
		t.Fatalf("compliance profile scope/retention = %+v", body.Profile)
	}
	if body.Profile.GovernanceBypassPolicy != "privileged_approval_required" || body.Profile.EvidenceExportPolicy != "retention_audit_chain" {
		t.Fatalf("compliance profile policy defaults = %+v", body.Profile)
	}
	if body.Attachment.Operation != "attach_profile" || !body.Attachment.TargetObjectLock.Required || body.Attachment.TargetObjectLock.DefaultRetention.Mode != "COMPLIANCE" || body.Attachment.TargetObjectLock.DefaultRetention.Years != 7 || body.Attachment.RetentionWeakeningCheck.Checked {
		t.Fatalf("compliance profile attachment = %+v", body.Attachment)
	}
	if body.ApplyPlan.Status != "blocked" || body.ApplyPlan.WriteEnabled || !body.ApplyPlan.ApplySupported || body.ApplyPlan.AttachReady {
		t.Fatalf("compliance profile apply plan = %+v", body.ApplyPlan)
	}
}

func TestComplianceProfilePlanCommandAppliesProfileAttachment(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	metadataPath := filepath.Join(t.TempDir(), "meta")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-profile-plan",
		"-metadata-backend", "pebble",
		"-metadata-path", metadataPath,
		"-profile-id", "sec-17a4",
		"-regulation", "sec",
		"-record-class", "broker_dealer_records",
		"-bucket-id", "bucket-sec",
		"-prefix", "sec/",
		"-object-class", "records",
		"-retention-mode", "COMPLIANCE",
		"-retention-years", "6",
		"-current-object-lock-enabled",
		"-current-retention-mode", "COMPLIANCE",
		"-current-retention-years", "6",
		"-apply",
	})
	if err != nil {
		t.Fatalf("compliance-profile-plan -apply error = %v stderr=%s stdout=%s", err, stderr.String(), stdout.String())
	}
	var body struct {
		DryRun    bool `json:"dry_run"`
		ApplyPlan struct {
			Status       string `json:"status"`
			WriteEnabled bool   `json:"write_enabled"`
			AttachReady  bool   `json:"attach_ready"`
		} `json:"apply_plan"`
		ApplyResult struct {
			Status         string `json:"status"`
			RecordsWritten int    `json:"records_written"`
			ProfileID      string `json:"profile_id"`
			AuditEventID   string `json:"audit_event_id"`
		} `json:"apply_result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.DryRun || body.ApplyPlan.Status != "ready" || !body.ApplyPlan.WriteEnabled || !body.ApplyPlan.AttachReady ||
		body.ApplyResult.Status != "succeeded" || body.ApplyResult.RecordsWritten != 1 || body.ApplyResult.ProfileID != "sec-17a4" || body.ApplyResult.AuditEventID == "" {
		t.Fatalf("apply output = %+v", body)
	}

	cfg := config.Default()
	cfg.MetadataBackend = config.MetadataBackendPebble
	cfg.MetadataPath = metadataPath
	repo, closeRepo, err := openMetadata(context.Background(), cfg)
	if err != nil {
		t.Fatalf("openMetadata(verify) error = %v", err)
	}
	defer closeRepo()
	attachments, err := repo.ListComplianceProfileAttachments(context.Background(), meta.ListComplianceProfileAttachmentsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListComplianceProfileAttachments() error = %v", err)
	}
	if len(attachments) != 1 || attachments[0].ProfileID != "sec-17a4" || attachments[0].Regulation != "SEC" ||
		attachments[0].BucketID != "bucket-sec" || attachments[0].RetentionMode != model.ObjectLockModeCompliance || attachments[0].RetentionYears != 6 {
		t.Fatalf("attachments = %+v", attachments)
	}
	events, err := repo.ListAuditEvents(context.Background(), meta.ListAuditEventsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Action != model.AuditActionAdminComplianceProfileAttach || events[0].Details["profile_id"] != "sec-17a4" ||
		events[0].Details["apply_plan_status"] != "ready" {
		t.Fatalf("audit events = %+v", events)
	}
}

func TestComplianceProfilePlanCommandRejectsInvalidProfile(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-profile-plan",
		"-profile-id", "bad-profile",
		"-regulation", "sec",
		"-record-class", "records",
		"-retention-days", "30",
		"-retention-years", "1",
	})
	if err == nil || !strings.Contains(err.Error(), "compliance profile plan is invalid") {
		t.Fatalf("compliance-profile-plan error = %v, want invalid profile error", err)
	}
	if stdout.Len() == 0 {
		t.Fatalf("stdout is empty, want validation JSON")
	}
}

func TestComplianceProfilePlanCommandRejectsRetentionWeakening(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-profile-plan",
		"-profile-id", "weaker-profile",
		"-regulation", "sec",
		"-record-class", "records",
		"-bucket-id", "bucket-1",
		"-retention-mode", "GOVERNANCE",
		"-retention-years", "7",
		"-current-object-lock-enabled",
		"-current-retention-mode", "COMPLIANCE",
		"-current-retention-years", "10",
	})
	if err == nil || !strings.Contains(err.Error(), "compliance profile plan is invalid") {
		t.Fatalf("compliance-profile-plan error = %v, want invalid profile error", err)
	}
	var body struct {
		Validation struct {
			Valid  bool     `json:"valid"`
			Errors []string `json:"errors"`
		} `json:"validation"`
		Attachment struct {
			RetentionWeakeningCheck struct {
				Checked          bool   `json:"checked"`
				WeakensRetention bool   `json:"weakens_retention"`
				Reason           string `json:"reason"`
			} `json:"retention_weakening_check"`
		} `json:"attachment"`
		ApplyPlan struct {
			AttachReady bool `json:"attach_ready"`
			Gates       []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"gates"`
		} `json:"apply_plan"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.Validation.Valid || len(body.Validation.Errors) == 0 || !body.Attachment.RetentionWeakeningCheck.Checked || !body.Attachment.RetentionWeakeningCheck.WeakensRetention || body.ApplyPlan.AttachReady {
		t.Fatalf("weaker profile plan = %+v", body)
	}
	gates := map[string]string{}
	for _, gate := range body.ApplyPlan.Gates {
		gates[gate.Name] = gate.Status
	}
	if gates["retention_not_weakened"] != "blocked" {
		t.Fatalf("weaker profile gates = %+v", body.ApplyPlan.Gates)
	}
}

func TestCompliancePolicySimulateCommandReportsDeniedDelete(t *testing.T) {
	skipPrivateEnterpriseOverlay(t)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	err := (testAdminCommand(&stdout, &stderr)).run(context.Background(), []string{
		"compliance-policy-simulate",
		"-operation", "delete_object",
		"-now", "2026-07-06T01:00:00Z",
		"-retention-mode", "COMPLIANCE",
		"-retain-until-date", "2026-07-07T01:00:00Z",
		"-legal-hold", "OFF",
		"-kms-key-state", "active",
		"-principal-policy-evaluated",
		"-principal-policy-allowed",
		"-compliance-profile-id", "sec-17a4",
		"-compliance-profile-attached",
		"-require-compliance-profile",
	})
	if err != nil {
		t.Fatalf("compliance-policy-simulate error = %v stderr=%s", err, stderr.String())
	}
	var body struct {
		SchemaVersion int    `json:"schema_version"`
		Operation     string `json:"operation"`
		Decision      string `json:"decision"`
		Allowed       bool   `json:"allowed"`
		Retention     struct {
			Mode   string `json:"mode"`
			Active bool   `json:"active"`
		} `json:"object_retention"`
		ComplianceProfileID       string `json:"compliance_profile_id"`
		ComplianceProfileAttached bool   `json:"compliance_profile_attached"`
		Gates                     []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"gates"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatalf("json decode: %v body=%s", err, stdout.String())
	}
	if body.SchemaVersion != 1 || body.Operation != "delete_object" || body.Decision != "denied" || body.Allowed ||
		body.Retention.Mode != "COMPLIANCE" || !body.Retention.Active || body.ComplianceProfileID != "sec-17a4" || !body.ComplianceProfileAttached {
		t.Fatalf("policy simulation body = %+v", body)
	}
	gates := map[string]string{}
	for _, gate := range body.Gates {
		gates[gate.Name] = gate.Status
	}
	if gates["retention_allows_operation"] != "blocked" || gates["legal_hold_clear"] != "passed" ||
		gates["kms_delete_admission"] != "passed" || gates["compliance_profile_attached"] != "passed" {
		t.Fatalf("policy simulation gates = %+v", body.Gates)
	}
}

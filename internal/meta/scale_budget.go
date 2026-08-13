package meta

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/nosway/namros/internal/meta/keyspace"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

const (
	MetadataScaleBudgetSchemaVersion = "namros.metadata.scale_budget.v1"

	DefaultMetadataValueBudgetBytes       = 16 * 1024 * 1024
	DefaultMetadataTransactionBudgetBytes = 64 * 1024 * 1024
)

type MetadataScaleBudgetRequest struct {
	PartCount                  int
	SegmentRefCount            int
	ChunksPerSegment           int
	ProtectedRefCount          int
	GCCandidateCount           int
	ValueBudgetBytes           int
	CompleteTxnBudgetBytes     int
	IncludeListIndexWriteBytes bool
	IncludeProtectedRefBytes   bool
	IncludeGCCandidateBytes    bool
}

type MetadataScaleBudgetReport struct {
	SchemaVersion            string                         `json:"schema_version"`
	PartCount                int                            `json:"part_count"`
	SegmentRefCount          int                            `json:"segment_ref_count"`
	ChunksPerSegment         int                            `json:"chunks_per_segment"`
	ProtectedRefCount        int                            `json:"protected_ref_count"`
	GCCandidateCount         int                            `json:"gc_candidate_count"`
	Limits                   MetadataScaleBudgetLimits      `json:"limits"`
	Records                  []MetadataRecordSizeEstimate   `json:"records"`
	CompleteTransaction      MetadataTransactionEstimate    `json:"complete_transaction"`
	Gates                    []MetadataScaleBudgetGate      `json:"gates"`
	ReleaseGate              MetadataScaleBudgetReleaseGate `json:"release_gate"`
	Recommendations          []string                       `json:"recommendations,omitempty"`
	CurrentSchemaLimitations []string                       `json:"current_schema_limitations,omitempty"`
}

type MetadataScaleBudgetLimits struct {
	MaxMultipartParts              int `json:"max_multipart_parts"`
	MaxObjectManifestSegmentRefs   int `json:"max_object_manifest_segment_refs"`
	MaxProtectedRefsPerObject      int `json:"max_protected_refs_per_object"`
	MaxGCCandidatesPerObject       int `json:"max_gc_candidates_per_object"`
	ValueBudgetBytes               int `json:"value_budget_bytes"`
	CompleteTransactionBudgetBytes int `json:"complete_transaction_budget_bytes"`
}

type MetadataRecordSizeEstimate struct {
	Name            string `json:"name"`
	Count           int    `json:"count"`
	KeyBytes        int    `json:"key_bytes"`
	ValueBytes      int    `json:"value_bytes"`
	TotalKeyBytes   int    `json:"total_key_bytes"`
	TotalValueBytes int    `json:"total_value_bytes"`
}

type MetadataTransactionEstimate struct {
	Name               string `json:"name"`
	ReadKeyBytes       int    `json:"read_key_bytes"`
	ReadValueBytes     int    `json:"read_value_bytes"`
	WriteKeyBytes      int    `json:"write_key_bytes"`
	WriteValueBytes    int    `json:"write_value_bytes"`
	DeleteKeyBytes     int    `json:"delete_key_bytes"`
	ApproxTotalBytes   int    `json:"approx_total_bytes"`
	RecordCountTouched int    `json:"record_count_touched"`
}

type MetadataScaleBudgetGate struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	ValueBytes  int    `json:"value_bytes"`
	BudgetBytes int    `json:"budget_bytes"`
	Message     string `json:"message,omitempty"`
}

type MetadataScaleBudgetReleaseGate struct {
	Status       string   `json:"status"`
	FailOnWatch  bool     `json:"fail_on_watch,omitempty"`
	FailedGates  []string `json:"failed_gates,omitempty"`
	WarningGates []string `json:"warning_gates,omitempty"`
	Message      string   `json:"message,omitempty"`
}

func EstimateMetadataScaleBudget(req MetadataScaleBudgetRequest) (MetadataScaleBudgetReport, error) {
	partCount := req.PartCount
	if partCount <= 0 {
		partCount = MaxMultipartParts
	}
	if partCount > MaxMultipartParts {
		return MetadataScaleBudgetReport{}, fmt.Errorf("%w: part count %d exceeds max %d", ErrInvalidArgument, partCount, MaxMultipartParts)
	}
	segmentRefCount := req.SegmentRefCount
	if segmentRefCount <= 0 {
		segmentRefCount = partCount
	}
	if segmentRefCount > MaxObjectManifestSegmentRefs {
		return MetadataScaleBudgetReport{}, fmt.Errorf("%w: segment ref count %d exceeds max %d", ErrInvalidArgument, segmentRefCount, MaxObjectManifestSegmentRefs)
	}
	chunksPerSegment := req.ChunksPerSegment
	if chunksPerSegment <= 0 {
		chunksPerSegment = 1
	}
	if chunksPerSegment > 1024 {
		return MetadataScaleBudgetReport{}, fmt.Errorf("%w: chunks per segment %d exceeds max 1024", ErrInvalidArgument, chunksPerSegment)
	}
	protectedRefCount := req.ProtectedRefCount
	if protectedRefCount <= 0 {
		protectedRefCount = segmentRefCount
	}
	if protectedRefCount > MaxObjectManifestSegmentRefs {
		return MetadataScaleBudgetReport{}, fmt.Errorf("%w: protected ref count %d exceeds max %d", ErrInvalidArgument, protectedRefCount, MaxObjectManifestSegmentRefs)
	}
	gcCandidateCount := req.GCCandidateCount
	if gcCandidateCount <= 0 {
		gcCandidateCount = segmentRefCount
	}
	if gcCandidateCount > MaxObjectManifestSegmentRefs {
		return MetadataScaleBudgetReport{}, fmt.Errorf("%w: gc candidate count %d exceeds max %d", ErrInvalidArgument, gcCandidateCount, MaxObjectManifestSegmentRefs)
	}
	valueBudget := req.ValueBudgetBytes
	if valueBudget <= 0 {
		valueBudget = DefaultMetadataValueBudgetBytes
	}
	completeTxnBudget := req.CompleteTxnBudgetBytes
	if completeTxnBudget <= 0 {
		completeTxnBudget = DefaultMetadataTransactionBudgetBytes
	}

	bucketID := "bucket-000001"
	objectKey := "scale/probe/object-000001"
	uploadID := "upload-000000000001"
	versionSortKey := "00000000000000000001#version-000000000001"
	versionID := "version-000000000001"
	now := time.Unix(1_700_000_000, 0).UTC()
	segmentRefs := sampleSegmentRefs(segmentRefCount, chunksPerSegment, now)
	objectVersion := model.ObjectVersion{
		BucketID:       bucketID,
		Key:            objectKey,
		VersionID:      versionID,
		VersionSortKey: versionSortKey,
		SizeBytes:      int64(segmentRefCount) * 64 * 1024 * 1024,
		ETag:           `"scale-etag-10000"`,
		ContentType:    "application/octet-stream",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-physical",
			Parameters: map[string]string{
				"profile_id": "STANDARD",
				"pool_id":    "standard-repl",
			},
		},
		SegmentRef:  firstScaleSegmentRef(segmentRefs),
		SegmentRefs: segmentRefs,
		UserMetadata: map[string]string{
			"scale-probe": "true",
		},
		Tags: map[string]string{
			"workload": "multipart",
		},
		ObjectLockRetention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeCompliance,
			RetainUntilDate: now.Add(365 * 24 * time.Hour),
		},
		ObjectLockLegalHold: model.ObjectLockLegalHoldOn,
		State:               model.ObjectVersionCommitted,
		CreatedAt:           now,
		CommittedAt:         now,
	}
	objectHead := model.ObjectHead{
		BucketID:     objectVersion.BucketID,
		Key:          objectVersion.Key,
		VersionID:    objectVersion.VersionID,
		SizeBytes:    objectVersion.SizeBytes,
		ETag:         objectVersion.ETag,
		ContentType:  objectVersion.ContentType,
		StorageClass: objectVersion.StorageClass,
		SegmentRef:   objectVersion.SegmentRef,
		SegmentRefs:  objectVersion.SegmentRefs,
		UserMetadata: objectVersion.UserMetadata,
		Tags:         objectVersion.Tags,
		LastModified: objectVersion.CommittedAt,
	}
	objectHeadRecord := model.ObjectHeadEntry{
		BucketID:             objectHead.BucketID,
		Key:                  objectHead.Key,
		VersionID:            objectHead.VersionID,
		SizeBytes:            objectHead.SizeBytes,
		ETag:                 objectHead.ETag,
		ContentType:          objectHead.ContentType,
		StorageClass:         objectHead.StorageClass,
		ServerSideEncryption: objectHead.ServerSideEncryption,
		UserMetadata:         objectHead.UserMetadata,
		Tags:                 objectHead.Tags,
		ObjectLockRetention:  objectHead.ObjectLockRetention,
		ObjectLockLegalHold:  objectHead.ObjectLockLegalHold,
		LastModified:         objectHead.LastModified,
		DeleteMarker:         objectHead.DeleteMarker,
	}
	listEntry := model.ObjectListEntry{
		BucketID:     objectHead.BucketID,
		Key:          objectHead.Key,
		VersionID:    objectHead.VersionID,
		SizeBytes:    objectHead.SizeBytes,
		ETag:         objectHead.ETag,
		ContentType:  objectHead.ContentType,
		StorageClass: objectHead.StorageClass,
		LastModified: objectHead.LastModified,
	}
	upload := model.MultipartUpload{
		UploadID:           uploadID,
		BucketID:           bucketID,
		Key:                objectKey,
		ContentType:        "application/octet-stream",
		StorageClass:       objectVersion.StorageClass,
		UserMetadata:       objectVersion.UserMetadata,
		Tags:               objectVersion.Tags,
		State:              model.MultipartUploadCompleted,
		CompletedVersionID: versionID,
		CompletedETag:      objectVersion.ETag,
		CompletedSizeBytes: objectVersion.SizeBytes,
		CompletedPartCount: partCount,
		CompletedAt:        now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	part := model.MultipartPart{
		UploadID:   uploadID,
		PartNumber: 1,
		SizeBytes:  int64(64 * 1024 * 1024),
		ETag:       `"0123456789abcdef0123456789abcdef"`,
		SegmentRef: firstScaleSegmentRef(segmentRefs),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	protectedRef := model.ProtectedRef{
		RefID:           scaleProtectedRefID(1),
		Reason:          model.ProtectedRefReasonObjectLock,
		BucketID:        bucketID,
		Key:             objectKey,
		VersionID:       versionID,
		SegmentID:       firstScaleSegmentRef(segmentRefs).SegmentID,
		SegmentRef:      firstScaleSegmentRef(segmentRefs),
		RetentionMode:   objectVersion.ObjectLockRetention.Mode,
		RetainUntilDate: objectVersion.ObjectLockRetention.RetainUntilDate,
		LegalHold:       objectVersion.ObjectLockLegalHold,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	gcCandidate := model.GCCandidateRecord{
		SegmentID:  firstScaleSegmentRef(segmentRefs).SegmentID,
		SegmentRef: firstScaleSegmentRef(segmentRefs),
		Reason:     storage.DeleteReasonObjectOverwritten,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	versionRecord := recordEstimate("object_version", 1, keyspace.ObjectVersion(bucketID, objectKey, versionSortKey), objectVersion)
	headRecord := recordEstimate("object_head", 1, keyspace.ObjectHead(bucketID, objectKey), objectHeadRecord)
	listRecord := recordEstimate("list_object", boolToInt(req.IncludeListIndexWriteBytes), keyspace.ListObject(bucketID, objectKey), listEntry)
	uploadRecord := recordEstimate("multipart_upload_state", 1, keyspace.MultipartUpload(bucketID, uploadID), upload)
	partRecord := recordEstimate("multipart_part", partCount, keyspace.MultipartPart(bucketID, uploadID, 1), part)
	protectedRefByVersionRecord := recordEstimate(
		"protected_ref_by_version",
		protectedRefCount,
		keyspace.ProtectedRefByVersion(bucketID, objectKey, versionID, protectedRef.RefID),
		protectedRef,
	)
	protectedRefBySegmentRecord := recordEstimate(
		"protected_ref_by_segment",
		protectedRefCount,
		keyspace.ProtectedRefBySegment(protectedRef.SegmentID, protectedRef.RefID),
		protectedRef,
	)
	gcCandidateRecord := recordEstimate("gc_candidate", gcCandidateCount, keyspace.GCCandidate(gcCandidate.SegmentID), gcCandidate)
	records := []MetadataRecordSizeEstimate{
		versionRecord,
		headRecord,
		listRecord,
		uploadRecord,
		partRecord,
		protectedRefByVersionRecord,
		protectedRefBySegmentRecord,
		gcCandidateRecord,
	}

	writeKeyBytes := versionRecord.TotalKeyBytes + headRecord.TotalKeyBytes + uploadRecord.TotalKeyBytes
	writeValueBytes := versionRecord.TotalValueBytes + headRecord.TotalValueBytes + uploadRecord.TotalValueBytes
	recordCountTouched := 1 + 1 + 1 + partCount
	if req.IncludeListIndexWriteBytes {
		writeKeyBytes += listRecord.TotalKeyBytes
		writeValueBytes += listRecord.TotalValueBytes
		recordCountTouched++
	}
	if req.IncludeProtectedRefBytes {
		writeKeyBytes += protectedRefByVersionRecord.TotalKeyBytes + protectedRefBySegmentRecord.TotalKeyBytes
		writeValueBytes += protectedRefByVersionRecord.TotalValueBytes + protectedRefBySegmentRecord.TotalValueBytes
		recordCountTouched += protectedRefCount * 2
	}
	if req.IncludeGCCandidateBytes {
		writeKeyBytes += gcCandidateRecord.TotalKeyBytes
		writeValueBytes += gcCandidateRecord.TotalValueBytes
		recordCountTouched += gcCandidateCount
	}
	completeTxn := MetadataTransactionEstimate{
		Name:               "complete_multipart_upload_foreground",
		ReadKeyBytes:       partRecord.TotalKeyBytes + uploadRecord.KeyBytes,
		ReadValueBytes:     partRecord.TotalValueBytes + uploadRecord.ValueBytes,
		WriteKeyBytes:      writeKeyBytes,
		WriteValueBytes:    writeValueBytes,
		DeleteKeyBytes:     0,
		ApproxTotalBytes:   partRecord.TotalKeyBytes + partRecord.TotalValueBytes + uploadRecord.KeyBytes + uploadRecord.ValueBytes + writeKeyBytes + writeValueBytes,
		RecordCountTouched: recordCountTouched,
	}
	gates := []MetadataScaleBudgetGate{
		budgetGate("object_version_value", versionRecord.ValueBytes, valueBudget),
		budgetGate("object_head_value", headRecord.ValueBytes, valueBudget),
		budgetGate("protected_ref_value", protectedRefByVersionRecord.ValueBytes, valueBudget),
		budgetGate("gc_candidate_value", gcCandidateRecord.ValueBytes, valueBudget),
		budgetGate("complete_transaction_approx", completeTxn.ApproxTotalBytes, completeTxnBudget),
	}
	recommendations := []string{}
	if versionRecord.ValueBytes > valueBudget/2 || headRecord.ValueBytes > valueBudget/2 {
		recommendations = append(recommendations, "keep large-manifest callers on repository hydration APIs so chunked manifests remain transparent")
	}
	if completeTxn.ApproxTotalBytes > completeTxnBudget/2 {
		recommendations = append(recommendations, "keep multipart completion on requested-part point reads and avoid adding upload-wide foreground scans")
	}
	if req.IncludeProtectedRefBytes && protectedRefByVersionRecord.TotalValueBytes+protectedRefBySegmentRecord.TotalValueBytes > completeTxnBudget/4 {
		recommendations = append(recommendations, "keep protected-ref writes idempotent and resumable before enabling object-lock-heavy large multipart workloads")
	}
	if req.IncludeGCCandidateBytes && gcCandidateRecord.TotalValueBytes > completeTxnBudget/4 {
		recommendations = append(recommendations, "split GC candidate enqueue from foreground publish/delete transactions when replaced manifests are large")
	}
	report := MetadataScaleBudgetReport{
		SchemaVersion:     MetadataScaleBudgetSchemaVersion,
		PartCount:         partCount,
		SegmentRefCount:   segmentRefCount,
		ChunksPerSegment:  chunksPerSegment,
		ProtectedRefCount: protectedRefCount,
		GCCandidateCount:  gcCandidateCount,
		Limits: MetadataScaleBudgetLimits{
			MaxMultipartParts:              MaxMultipartParts,
			MaxObjectManifestSegmentRefs:   MaxObjectManifestSegmentRefs,
			MaxProtectedRefsPerObject:      MaxObjectManifestSegmentRefs,
			MaxGCCandidatesPerObject:       MaxObjectManifestSegmentRefs,
			ValueBudgetBytes:               valueBudget,
			CompleteTransactionBudgetBytes: completeTxnBudget,
		},
		Records:             records,
		CompleteTransaction: completeTxn,
		Gates:               gates,
		Recommendations:     recommendations,
		CurrentSchemaLimitations: []string{
			"CompleteMultipartUpload still reads one metadata record per requested part for S3 part and ETag validation",
			"multipart part cleanup is split into bounded transactions and must be resumed until done",
			"KV protected refs are duplicated into by-version and by-segment indexes",
			"GC candidate enqueue is per segment until cleanup work is chunked or summarized",
		},
	}
	report.ReleaseGate = EvaluateMetadataScaleBudgetReleaseGate(report, false)
	return report, nil
}

func EvaluateMetadataScaleBudgetReleaseGate(report MetadataScaleBudgetReport, failOnWatch bool) MetadataScaleBudgetReleaseGate {
	var failed []string
	var warnings []string
	for _, gate := range report.Gates {
		switch gate.Status {
		case "over_budget":
			failed = append(failed, gate.Name)
		case "watch":
			if failOnWatch {
				failed = append(failed, gate.Name)
			} else {
				warnings = append(warnings, gate.Name)
			}
		}
	}
	status := "passed"
	message := "metadata scale budget is within release gates"
	if len(failed) > 0 {
		status = "failed"
		if failOnWatch {
			message = "metadata scale budget has over-budget or watch gates"
		} else {
			message = "metadata scale budget has over-budget gates"
		}
	} else if len(warnings) > 0 {
		status = "warning"
		message = "metadata scale budget has watch gates"
	}
	return MetadataScaleBudgetReleaseGate{
		Status:       status,
		FailOnWatch:  failOnWatch,
		FailedGates:  failed,
		WarningGates: warnings,
		Message:      message,
	}
}

func sampleSegmentRefs(count, chunksPerSegment int, now time.Time) []storage.SegmentRef {
	if count == 0 {
		return nil
	}
	out := make([]storage.SegmentRef, 0, count)
	for i := 0; i < count; i++ {
		chunks := make([]storage.PlacementChunk, 0, chunksPerSegment)
		for c := 0; c < chunksPerSegment; c++ {
			chunks = append(chunks, storage.PlacementChunk{
				LogicalOffsetBytes: uint64(c) * 64 * 1024 * 1024,
				SizeBytes:          64 * 1024 * 1024,
				VolumeID:           fmt.Sprintf("18a%05d", (i%8)+1),
				OffsetBytes:        uint64(i*chunksPerSegment+c) * 64 * 1024 * 1024,
				LengthBytes:        64 * 1024 * 1024,
				StoreID:            fmt.Sprintf("svc-u%02d", 7+(i+c)%12),
				ChunkID:            uint64(i*chunksPerSegment + c + 1),
				Role:               "replica",
			})
		}
		out = append(out, storage.SegmentRef{
			SegmentID: fmt.Sprintf("segment-%012d", i+1),
			StorageClass: storage.StorageClassSnapshot{
				StorageClassID: "STANDARD",
				Backend:        "sbs-physical",
				Parameters: map[string]string{
					"profile_id": "STANDARD",
					"pool_id":    "standard-repl",
				},
			},
			Placement: storage.PlacementSnapshot{
				Backend:           "sbs",
				Layout:            "physical-chunk",
				RedundancyBackend: "replicated",
				ProfileID:         "STANDARD",
				ProfileGeneration: 1,
				ChunkSizeBytes:    64 * 1024 * 1024,
				Parameters: map[string]string{
					"volume_pool_id": "standard-repl",
					"attachment_id":  "att-scale-probe",
				},
				Chunks: chunks,
			},
			SizeBytes: 64 * 1024 * 1024,
			Digest: storage.Digest{
				Algorithm: "sha256",
				Hex:       "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
			CreatedAt: now,
		})
	}
	return out
}

func firstScaleSegmentRef(refs []storage.SegmentRef) storage.SegmentRef {
	if len(refs) == 0 {
		return storage.SegmentRef{}
	}
	return refs[0]
}

func scaleProtectedRefID(index int) string {
	return fmt.Sprintf("0123456789abcdef0123456789abcdef0123456789abcdef%016x", index)
}

func recordEstimate(name string, count int, key string, value any) MetadataRecordSizeEstimate {
	encoded, _ := json.Marshal(value)
	return MetadataRecordSizeEstimate{
		Name:            name,
		Count:           count,
		KeyBytes:        len(key),
		ValueBytes:      len(encoded),
		TotalKeyBytes:   len(key) * count,
		TotalValueBytes: len(encoded) * count,
	}
}

func budgetGate(name string, valueBytes, budgetBytes int) MetadataScaleBudgetGate {
	status := "within_budget"
	message := ""
	if valueBytes > budgetBytes {
		status = "over_budget"
		message = "split records before using this shape in production-scale workloads"
	} else if valueBytes > budgetBytes/2 {
		status = "watch"
		message = "close to budget; keep this in scale inventory and plan split records"
	}
	return MetadataScaleBudgetGate{
		Name:        name,
		Status:      status,
		ValueBytes:  valueBytes,
		BudgetBytes: budgetBytes,
		Message:     message,
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

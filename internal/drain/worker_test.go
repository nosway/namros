package drain

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func TestWorkerMoveObjectVersionCopiesAndPublishesRefs(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newDrainTestStore("18a00002")

	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-a",
		Name:     "drain-copy",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	payload := []byte("hello drain!")
	sourceRef := drainTestSegmentRef("segment-source", "18a00001", uint64(len(payload)))
	store.seed(sourceRef, payload)
	published, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:     bucket.BucketID,
		Key:          "logs/a.txt",
		SizeBytes:    int64(len(payload)),
		ETag:         "etag-a",
		StorageClass: sourceRef.StorageClass,
		SegmentRefs:  []storage.SegmentRef{sourceRef},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}

	result, err := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(time.Minute) },
	}.MoveObjectVersion(ctx, MoveObjectVersionRequest{
		PoolID:         "object-pool",
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		OwnerID:        "worker-a",
		BucketID:       bucket.BucketID,
		Key:            "logs/a.txt",
		VersionID:      published.Head.VersionID,
	})
	if err != nil {
		t.Fatalf("MoveObjectVersion() error = %v", err)
	}
	if result.Operation.Status != model.VolumeDrainOperationSucceeded || result.Operation.Scanned != 1 || result.Operation.Copied != 1 || result.Operation.Retryable != 0 {
		t.Fatalf("operation = %+v, want succeeded copied operation", result.Operation)
	}
	if len(result.PreviousSegmentRefs) != 1 || result.PreviousSegmentRefs[0].SegmentID != sourceRef.SegmentID {
		t.Fatalf("previous refs = %+v, want source ref", result.PreviousSegmentRefs)
	}
	if len(result.PublishedSegmentRefs) != 1 || meta.SegmentRefVolumeID(result.PublishedSegmentRefs[0]) != "18a00002" {
		t.Fatalf("published refs = %+v, want target volume", result.PublishedSegmentRefs)
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "logs/a.txt")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if head.VersionID != published.Head.VersionID || len(head.SegmentRefs) != 1 || meta.SegmentRefVolumeID(head.SegmentRefs[0]) != "18a00002" || head.SizeBytes != int64(len(payload)) || head.ETag != "etag-a" {
		t.Fatalf("head after drain publish = %+v", head)
	}
	if got := readDrainTestSegment(t, store, head.SegmentRefs[0]); !bytes.Equal(got, payload) {
		t.Fatalf("target payload = %q, want %q", string(got), string(payload))
	}
	if got := readDrainTestSegment(t, store, sourceRef); !bytes.Equal(got, payload) {
		t.Fatalf("source payload = %q, want old ref preserved", string(got))
	}
	ops, err := repo.ListVolumeDrainOperations(ctx, meta.ListVolumeDrainOperationsRequest{
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		Limit:          10,
	})
	if err != nil {
		t.Fatalf("ListVolumeDrainOperations() error = %v", err)
	}
	if len(ops) != 1 || ops[0].OperationID != result.Operation.OperationID || len(ops[0].Attempts) != 1 || ops[0].Attempts[0].TargetSegmentID == "" {
		t.Fatalf("operation history = %+v, want copied attempt", ops)
	}
}

func TestWorkerMoveObjectVersionRejectsUnexpectedTargetVolume(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newDrainTestStore("18a99999")

	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-a",
		Name:     "drain-target-guard",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	payload := []byte("target guard")
	sourceRef := drainTestSegmentRef("segment-source", "18a00001", uint64(len(payload)))
	store.seed(sourceRef, payload)
	published, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:     bucket.BucketID,
		Key:          "logs/a.txt",
		SizeBytes:    int64(len(payload)),
		ETag:         "etag-a",
		StorageClass: sourceRef.StorageClass,
		SegmentRefs:  []storage.SegmentRef{sourceRef},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}

	result, err := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(time.Minute) },
	}.MoveObjectVersion(ctx, MoveObjectVersionRequest{
		PoolID:         "object-pool",
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		OwnerID:        "worker-a",
		BucketID:       bucket.BucketID,
		Key:            "logs/a.txt",
		VersionID:      published.Head.VersionID,
	})
	if err == nil {
		t.Fatalf("MoveObjectVersion() error = nil, want target volume mismatch error")
	}
	if result.Operation.Status != model.VolumeDrainOperationRetryPending || result.Operation.Retryable != 1 {
		t.Fatalf("operation = %+v, want retry_pending", result.Operation)
	}
	if len(result.Operation.Attempts) != 1 || !result.Operation.Attempts[0].Retryable || meta.SegmentRefVolumeID(result.Operation.Attempts[0].TargetRef) != "18a99999" {
		t.Fatalf("attempts = %+v, want retryable copied target ref", result.Operation.Attempts)
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "logs/a.txt")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if len(head.SegmentRefs) != 1 || head.SegmentRefs[0].SegmentID != sourceRef.SegmentID || meta.SegmentRefVolumeID(head.SegmentRefs[0]) != "18a00001" {
		t.Fatalf("head after rejected drain = %+v, want original source ref", head)
	}
}

func TestWorkerReclaimOperationQueuesOldRefsAfterTargetValidation(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newDrainTestStore("18a00002")

	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-a",
		Name:     "drain-reclaim",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	payload := []byte("reclaim me")
	sourceRef := drainTestSegmentRef("segment-source", "18a00001", uint64(len(payload)))
	store.seed(sourceRef, payload)
	published, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:     bucket.BucketID,
		Key:          "logs/a.txt",
		SizeBytes:    int64(len(payload)),
		ETag:         "etag-a",
		StorageClass: sourceRef.StorageClass,
		SegmentRefs:  []storage.SegmentRef{sourceRef},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}
	worker := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(time.Minute) },
	}
	moved, err := worker.MoveObjectVersion(ctx, MoveObjectVersionRequest{
		PoolID:         "object-pool",
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		OwnerID:        "worker-a",
		BucketID:       bucket.BucketID,
		Key:            "logs/a.txt",
		VersionID:      published.Head.VersionID,
	})
	if err != nil {
		t.Fatalf("MoveObjectVersion() error = %v", err)
	}

	reclaimed, err := worker.ReclaimOperation(ctx, ReclaimOperationRequest{
		Operation: moved.Operation,
		OwnerID:   "worker-b",
	})
	if err != nil {
		t.Fatalf("ReclaimOperation() error = %v", err)
	}
	if reclaimed.Queued != 1 || reclaimed.Operation.Status != model.VolumeDrainOperationSucceeded || reclaimed.Operation.ResumeOfOperationID != moved.Operation.OperationID {
		t.Fatalf("reclaimed = %+v, want queued successor operation", reclaimed)
	}
	if len(reclaimed.Operation.Attempts) != 1 || reclaimed.Operation.Attempts[0].Status != model.VolumeDrainAttemptQueuedGC {
		t.Fatalf("reclaim attempts = %+v, want queued_gc", reclaimed.Operation.Attempts)
	}
	candidates, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].SegmentID != sourceRef.SegmentID || candidates[0].Reason != storage.DeleteReasonVolumeDrained {
		t.Fatalf("gc candidates = %+v, want drained source ref", candidates)
	}
	if got := readDrainTestSegment(t, store, sourceRef); !bytes.Equal(got, payload) {
		t.Fatalf("source payload after queue = %q, want physical delete deferred", string(got))
	}
}

func TestWorkerReclaimOperationBlocksWhenTargetValidationFails(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newDrainTestStore("18a00002")
	sourceRef := drainTestSegmentRef("segment-source", "18a00001", 12)
	targetRef := drainTestSegmentRef("segment-target", "18a00002", 12)
	store.seed(sourceRef, []byte("source bytes"))
	store.seed(targetRef, []byte("target bytes"))
	store.failValidation(targetRef.SegmentID, storage.ErrUnavailable)

	result, err := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(time.Minute) },
	}.ReclaimOperation(ctx, ReclaimOperationRequest{
		Operation: drainTestOperation("op-copy", sourceRef, targetRef),
	})
	if err != nil {
		t.Fatalf("ReclaimOperation() error = %v", err)
	}
	if result.Retryable != 1 || result.Operation.Status != model.VolumeDrainOperationRetryPending {
		t.Fatalf("result = %+v, want retry pending", result)
	}
	candidates, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("gc candidates = %+v, want none when target validation fails", candidates)
	}
}

func TestWorkerReclaimOperationBlocksProtectedSourceRef(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newDrainTestStore("18a00002")
	sourceRef := drainTestSegmentRef("segment-source", "18a00001", 12)
	targetRef := drainTestSegmentRef("segment-target", "18a00002", 12)
	store.seed(sourceRef, []byte("source bytes"))
	store.seed(targetRef, []byte("target bytes"))
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-a",
		Name:     "drain-protected",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	locked, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:    bucket.BucketID,
		Key:         "locked.txt",
		SizeBytes:   int64(sourceRef.SizeBytes),
		ETag:        "etag-locked",
		SegmentRefs: []storage.SegmentRef{sourceRef},
		ObjectLockRetention: model.ObjectLockRetention{
			Mode:            model.ObjectLockModeCompliance,
			RetainUntilDate: now.Add(time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(locked) error = %v", err)
	}
	op := drainTestOperation("op-copy", sourceRef, targetRef)
	op.Attempts[0].BucketID = bucket.BucketID
	op.Attempts[0].Key = "locked.txt"
	op.Attempts[0].VersionID = locked.Head.VersionID

	result, err := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(time.Minute) },
	}.ReclaimOperation(ctx, ReclaimOperationRequest{Operation: op})
	if err != nil {
		t.Fatalf("ReclaimOperation() error = %v", err)
	}
	if result.Protected != 1 || result.Operation.Status != model.VolumeDrainOperationRetryPending {
		t.Fatalf("result = %+v, want protected retry pending", result)
	}
	candidates, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("gc candidates = %+v, want none for protected source", candidates)
	}
}

func TestWorkerFailoverDoesNotDuplicatePublishOrGCCandidate(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	repo := memory.NewWithClock(func() time.Time { return now })
	store := newDrainTestStore("18a00002")
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-a",
		Name:     "drain-failover",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	payload := []byte("failover")
	sourceRef := drainTestSegmentRef("segment-source", "18a00001", uint64(len(payload)))
	store.seed(sourceRef, payload)
	published, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
		BucketID:     bucket.BucketID,
		Key:          "logs/a.txt",
		SizeBytes:    int64(len(payload)),
		ETag:         "etag-a",
		StorageClass: sourceRef.StorageClass,
		SegmentRefs:  []storage.SegmentRef{sourceRef},
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}
	moveReq := MoveObjectVersionRequest{
		PoolID:         "object-pool",
		SourceVolumeID: "18a00001",
		TargetVolumeID: "18a00002",
		BucketID:       bucket.BucketID,
		Key:            "logs/a.txt",
		VersionID:      published.Head.VersionID,
	}
	firstWorker := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(time.Minute) },
	}
	firstMove, err := firstWorker.MoveObjectVersion(ctx, withMoveOwner(moveReq, "worker-a"))
	if err != nil {
		t.Fatalf("MoveObjectVersion(first) error = %v", err)
	}
	if store.putCount() != 1 {
		t.Fatalf("put count after first move = %d, want 1", store.putCount())
	}
	head, err := repo.GetObjectHead(ctx, bucket.BucketID, "logs/a.txt")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	targetSegmentID := head.SegmentRefs[0].SegmentID

	secondWorker := Worker{
		Metadata: repo,
		Storage:  store,
		Now:      func() time.Time { return now.Add(2 * time.Minute) },
	}
	secondMove, err := secondWorker.MoveObjectVersion(ctx, withMoveOwner(moveReq, "worker-b"))
	if err != nil {
		t.Fatalf("MoveObjectVersion(second) error = %v", err)
	}
	if secondMove.Operation.Copied != 0 || secondMove.Operation.Skipped != 1 || store.putCount() != 1 {
		t.Fatalf("second move operation = %+v put_count=%d, want skipped replay without copy", secondMove.Operation, store.putCount())
	}
	head, err = repo.GetObjectHead(ctx, bucket.BucketID, "logs/a.txt")
	if err != nil {
		t.Fatalf("GetObjectHead(second) error = %v", err)
	}
	if head.SegmentRefs[0].SegmentID != targetSegmentID {
		t.Fatalf("head target changed on replay: got %q want %q", head.SegmentRefs[0].SegmentID, targetSegmentID)
	}

	if _, err := firstWorker.ReclaimOperation(ctx, ReclaimOperationRequest{
		Operation: firstMove.Operation,
		OwnerID:   "worker-a",
	}); err != nil {
		t.Fatalf("ReclaimOperation(first) error = %v", err)
	}
	if _, err := secondWorker.ReclaimOperation(ctx, ReclaimOperationRequest{
		Operation: firstMove.Operation,
		OwnerID:   "worker-b",
	}); err != nil {
		t.Fatalf("ReclaimOperation(replay) error = %v", err)
	}
	candidates, err := repo.ListGCCandidates(ctx, meta.ListGCCandidatesRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].SegmentID != sourceRef.SegmentID {
		t.Fatalf("gc candidates after replay = %+v, want one source candidate", candidates)
	}
}

type drainTestStore struct {
	mu             sync.Mutex
	targetVolumeID string
	next           int
	data           map[string][]byte
	refs           map[string]storage.SegmentRef
	validateErrors map[string]error
}

func newDrainTestStore(targetVolumeID string) *drainTestStore {
	return &drainTestStore{
		targetVolumeID: targetVolumeID,
		data:           make(map[string][]byte),
		refs:           make(map[string]storage.SegmentRef),
		validateErrors: make(map[string]error),
	}
}

func (s *drainTestStore) seed(ref storage.SegmentRef, payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[ref.SegmentID] = append([]byte(nil), payload...)
	s.refs[ref.SegmentID] = storage.CloneSegmentRef(ref)
}

func (s *drainTestStore) failValidation(segmentID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validateErrors[segmentID] = err
}

func (s *drainTestStore) putCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.next
}

func (s *drainTestStore) PutSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
	if err := ctx.Err(); err != nil {
		return storage.SegmentRef{}, err
	}
	if req.Reader == nil {
		return storage.SegmentRef{}, fmt.Errorf("%w: reader is required", storage.ErrInvalidArgument)
	}
	payload, err := io.ReadAll(req.Reader)
	if err != nil {
		return storage.SegmentRef{}, err
	}
	if uint64(len(payload)) != req.SizeBytes {
		return storage.SegmentRef{}, fmt.Errorf("%w: size mismatch", storage.ErrInvalidArgument)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	ref := drainTestSegmentRef(fmt.Sprintf("segment-target-%d", s.next), s.targetVolumeID, req.SizeBytes)
	ref.StorageClass = storage.CloneStorageClassSnapshot(req.StorageClass)
	if ref.StorageClass.Backend == "" {
		ref.StorageClass.Backend = "sbs"
	}
	if ref.StorageClass.Parameters == nil {
		ref.StorageClass.Parameters = make(map[string]string)
	}
	ref.StorageClass.Parameters["volume_id"] = s.targetVolumeID
	s.data[ref.SegmentID] = append([]byte(nil), payload...)
	s.refs[ref.SegmentID] = storage.CloneSegmentRef(ref)
	return ref, nil
}

func (s *drainTestStore) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	payload, ok := s.data[ref.SegmentID]
	s.mu.Unlock()
	if !ok {
		return nil, storage.ErrNotFound
	}
	if off > uint64(len(payload)) || off+length > uint64(len(payload)) {
		return nil, storage.ErrInvalidRange
	}
	return io.NopCloser(bytes.NewReader(payload[off : off+length])), nil
}

func (s *drainTestStore) ValidateSegment(ctx context.Context, ref storage.SegmentRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateErrors[ref.SegmentID]; err != nil {
		return err
	}
	if _, ok := s.data[ref.SegmentID]; !ok {
		return storage.ErrNotFound
	}
	return nil
}

func (s *drainTestStore) DeleteSegment(_ context.Context, ref storage.SegmentRef, _ storage.DeleteReason) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, ref.SegmentID)
	delete(s.refs, ref.SegmentID)
	return nil
}

func readDrainTestSegment(t *testing.T, store *drainTestStore, ref storage.SegmentRef) []byte {
	t.Helper()
	reader, err := store.GetSegment(t.Context(), ref, 0, ref.SizeBytes)
	if err != nil {
		t.Fatalf("GetSegment(%q) error = %v", ref.SegmentID, err)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(%q) error = %v", ref.SegmentID, err)
	}
	return payload
}

func drainTestSegmentRef(segmentID, volumeID string, size uint64) storage.SegmentRef {
	return storage.SegmentRef{
		SegmentID: segmentID,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
			Parameters: map[string]string{
				"volume_id": volumeID,
			},
		},
		Placement: storage.PlacementSnapshot{
			Backend:           "sbs",
			Layout:            "single-segment",
			RedundancyBackend: "replicated",
			ProfileID:         "STANDARD",
			Parameters: map[string]string{
				"volume_id": volumeID,
			},
			Chunks: []storage.PlacementChunk{{
				LogicalOffsetBytes: 0,
				SizeBytes:          size,
				VolumeID:           volumeID,
				LengthBytes:        size,
				Role:               "primary",
			}},
		},
		SizeBytes: size,
	}
}

func drainTestOperation(operationID string, sourceRef, targetRef storage.SegmentRef) model.VolumeDrainOperationRecord {
	return model.VolumeDrainOperationRecord{
		OperationID:    operationID,
		PoolID:         "object-pool",
		SourceVolumeID: meta.SegmentRefVolumeID(sourceRef),
		TargetVolumeID: meta.SegmentRefVolumeID(targetRef),
		OwnerID:        "worker-a",
		Status:         model.VolumeDrainOperationSucceeded,
		Scanned:        1,
		Copied:         1,
		Attempts: []model.VolumeDrainAttempt{{
			BucketID:        "bucket-1",
			Key:             "logs/a.txt",
			VersionID:       "version-1",
			SourceSegmentID: sourceRef.SegmentID,
			SourceRef:       storage.CloneSegmentRef(sourceRef),
			TargetSegmentID: targetRef.SegmentID,
			TargetRef:       storage.CloneSegmentRef(targetRef),
			Status:          model.VolumeDrainAttemptCopied,
		}},
	}
}

func withMoveOwner(req MoveObjectVersionRequest, ownerID string) MoveObjectVersionRequest {
	req.OwnerID = ownerID
	return req
}

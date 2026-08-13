package testsuite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/nosway/namros/internal/storage"
)

type SegmentStoreUnderTest interface {
	storage.SegmentStore
	storage.OrphanTracker
}

func RunSegmentStoreTests(t *testing.T, newStore func(t *testing.T) SegmentStoreUnderTest) {
	t.Helper()
	t.Run("put get range delete", func(t *testing.T) {
		testPutGetRangeDelete(t, newStore(t))
	})
	t.Run("orphan gc queue", func(t *testing.T) {
		testOrphanGCQueue(t, newStore(t))
	})
}

func testPutGetRangeDelete(t *testing.T, store SegmentStoreUnderTest) {
	ctx := t.Context()
	payload := []byte("hello world")
	ref, err := store.PutSegment(ctx, storage.PutSegmentRequest{
		Reader:       bytes.NewReader(payload),
		SizeBytes:    uint64(len(payload)),
		StorageClass: testStorageClass(),
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if ref.SizeBytes != uint64(len(payload)) {
		t.Fatalf("SizeBytes = %d, want %d", ref.SizeBytes, len(payload))
	}
	if ref.Digest.Algorithm != "sha256" || ref.Digest.Hex != sha256Hex(payload) {
		t.Fatalf("digest = %+v", ref.Digest)
	}
	if ref.Placement.Backend == "" || ref.Placement.Layout == "" || ref.Placement.ProfileID != "STANDARD" {
		t.Fatalf("placement snapshot = %+v, want backend/layout/profile", ref.Placement)
	}
	if len(ref.Placement.Chunks) == 0 {
		t.Fatalf("placement chunks = nil, want at least one chunk")
	}
	full := readAll(t, store, ref, 0, 0)
	if !bytes.Equal(full, payload) {
		t.Fatalf("full read = %q, want %q", full, payload)
	}
	ranged := readAll(t, store, ref, 6, 5)
	if string(ranged) != "world" {
		t.Fatalf("range read = %q, want world", ranged)
	}
	if err := store.DeleteSegment(ctx, ref, storage.DeleteReasonManualGC); err != nil {
		t.Fatalf("DeleteSegment() error = %v", err)
	}
	if _, err := store.GetSegment(ctx, ref, 0, 0); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetSegment(deleted) error = %v, want ErrNotFound", err)
	}
}

func testOrphanGCQueue(t *testing.T, store SegmentStoreUnderTest) {
	ctx := t.Context()
	ref, err := store.PutSegment(ctx, storage.PutSegmentRequest{
		Reader:       bytes.NewReader([]byte("orphan")),
		SizeBytes:    6,
		StorageClass: testStorageClass(),
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if err := store.MarkOrphan(ctx, ref, storage.DeleteReasonPublishFailed); err != nil {
		t.Fatalf("MarkOrphan() error = %v", err)
	}
	candidates, err := store.ListGCCandidates(ctx, 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate len = %d, want 1", len(candidates))
	}
	if candidates[0].Ref.SegmentID != ref.SegmentID || candidates[0].Reason != storage.DeleteReasonPublishFailed {
		t.Fatalf("candidate = %+v, want ref %q reason %q", candidates[0], ref.SegmentID, storage.DeleteReasonPublishFailed)
	}
	if !reflect.DeepEqual(candidates[0].Ref.StorageClass, ref.StorageClass) {
		t.Fatalf("storage class snapshot = %+v, want %+v", candidates[0].Ref.StorageClass, ref.StorageClass)
	}
	if !reflect.DeepEqual(candidates[0].Ref.Placement, ref.Placement) {
		t.Fatalf("placement snapshot = %+v, want %+v", candidates[0].Ref.Placement, ref.Placement)
	}
	if err := store.DeleteSegment(ctx, ref, storage.DeleteReasonManualGC); err != nil {
		t.Fatalf("DeleteSegment(orphan) error = %v", err)
	}
	candidates, err = store.ListGCCandidates(ctx, 10)
	if err != nil {
		t.Fatalf("ListGCCandidates(after delete) error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("candidate len after delete = %d, want 0", len(candidates))
	}
}

func readAll(t *testing.T, store SegmentStoreUnderTest, ref storage.SegmentRef, off, length uint64) []byte {
	t.Helper()
	reader, err := store.GetSegment(t.Context(), ref, off, length)
	if err != nil {
		t.Fatalf("GetSegment(off=%d length=%d) error = %v", off, length, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return data
}

func testStorageClass() storage.StorageClassSnapshot {
	return storage.StorageClassSnapshot{
		StorageClassID: "STANDARD",
		Backend:        "local",
		Parameters: map[string]string{
			"root": "test",
		},
	}
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

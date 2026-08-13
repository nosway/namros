package sbs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"

	sbslocal "github.com/nosway/namrbd/sbs/local"

	"github.com/nosway/namros/internal/storage"
	namrossbs "github.com/nosway/namros/internal/storage/sbs"
	"github.com/nosway/namros/internal/storage/testsuite"
)

func TestSegmentStoreSuite(t *testing.T) {
	testsuite.RunSegmentStoreTests(t, func(t *testing.T) testsuite.SegmentStoreUnderTest {
		t.Helper()
		store, err := namrossbs.Open(t.Context(), testConfig(t))
		if err != nil {
			t.Fatalf("sbs.Open() error = %v", err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Fatalf("sbs.Close() error = %v", err)
			}
		})
		return store
	})
}

func TestSegmentSurvivesReopen(t *testing.T) {
	cfg := testConfig(t)
	store, err := namrossbs.Open(t.Context(), cfg)
	if err != nil {
		t.Fatalf("sbs.Open() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("persistent segment")),
		SizeBytes: uint64(len("persistent segment")),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-local",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if ref.Placement.Backend != "sbs-local" || ref.Placement.Layout != "sbs-volume-offset" || ref.Placement.RedundancyBackend != "replicated" {
		t.Fatalf("placement snapshot = %+v", ref.Placement)
	}
	if len(ref.Placement.Chunks) != 1 {
		t.Fatalf("placement chunks len = %d, want 1", len(ref.Placement.Chunks))
	}
	if chunk := ref.Placement.Chunks[0]; chunk.VolumeID == "" || chunk.LengthBytes == 0 || chunk.SizeBytes != ref.SizeBytes {
		t.Fatalf("placement chunk = %+v, ref size = %d", chunk, ref.SizeBytes)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := namrossbs.Open(t.Context(), cfg)
	if err != nil {
		t.Fatalf("sbs.Open(reopen) error = %v", err)
	}
	defer reopened.Close()
	reader, err := reopened.GetSegment(t.Context(), ref, 11, 7)
	if err != nil {
		t.Fatalf("GetSegment(reopen) error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, []byte("segment")) {
		t.Fatalf("reopened range = %q, want segment", got)
	}
}

func TestSegmentStoreStreamsPutIntoPaddedWriteBuffer(t *testing.T) {
	cfg := testConfig(t)
	store, err := namrossbs.Open(t.Context(), cfg)
	if err != nil {
		t.Fatalf("sbs.Open() error = %v", err)
	}
	defer store.Close()
	payload := bytes.Repeat([]byte("x"), 64*1024*2+17)
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    &maxReadSizeReader{data: payload, max: 64 * 1024},
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-local",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment(streaming) error = %v", err)
	}
	got := readSBSStoreSegment(t, store, ref, 64*1024*2, 17)
	if !bytes.Equal(got, bytes.Repeat([]byte("x"), 17)) {
		t.Fatalf("tail range = %q, want 17 x bytes", got)
	}
}

func TestSegmentStoreMarksStreamFailureForGC(t *testing.T) {
	cfg := testConfig(t)
	store, err := namrossbs.Open(t.Context(), cfg)
	if err != nil {
		t.Fatalf("sbs.Open() error = %v", err)
	}
	defer store.Close()
	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcd")),
		SizeBytes: 3,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-local",
		},
	})
	if !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("PutSegment(extra byte) error = %v, want ErrInvalidArgument", err)
	}
	candidates, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("GC candidates = %d, want 1", len(candidates))
	}
	if candidates[0].Reason != storage.DeleteReasonPublishFailed {
		t.Fatalf("candidate reason = %q, want %q", candidates[0].Reason, storage.DeleteReasonPublishFailed)
	}
	if err := store.DeleteSegment(t.Context(), candidates[0].Ref, storage.DeleteReasonPublishFailed); err != nil {
		t.Fatalf("DeleteSegment(GC candidate) error = %v", err)
	}
}

func TestSegmentStoreDeleteAdmissionBlocksDelete(t *testing.T) {
	cfg := testConfig(t)
	called := false
	cfg.DeleteAdmission = func(_ context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
		called = true
		if ref.SegmentID == "" {
			t.Fatal("DeleteAdmission received empty segment id")
		}
		if reason != storage.DeleteReasonManualGC {
			t.Fatalf("DeleteAdmission reason = %q, want manual_gc", reason)
		}
		return storage.ErrProtected
	}
	store, err := namrossbs.Open(t.Context(), cfg)
	if err != nil {
		t.Fatalf("sbs.Open() error = %v", err)
	}
	defer store.Close()
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("protected segment")),
		SizeBytes: uint64(len("protected segment")),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs-local",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if err := store.DeleteSegment(t.Context(), ref, storage.DeleteReasonManualGC); !errors.Is(err, storage.ErrProtected) {
		t.Fatalf("DeleteSegment() error = %v, want ErrProtected", err)
	}
	if !called {
		t.Fatal("DeleteAdmission was not called")
	}
	reader, err := store.GetSegment(t.Context(), ref, 0, 0)
	if err != nil {
		t.Fatalf("GetSegment(blocked delete) error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "protected segment" {
		t.Fatalf("segment after blocked delete = %q", got)
	}
}

func readSBSStoreSegment(t *testing.T, store *namrossbs.Store, ref storage.SegmentRef, off, length uint64) []byte {
	t.Helper()
	reader, err := store.GetSegment(t.Context(), ref, off, length)
	if err != nil {
		t.Fatalf("GetSegment(off=%d length=%d) error = %v", off, length, err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return got
}

func testConfig(t *testing.T) namrossbs.Config {
	t.Helper()
	root := t.TempDir()
	return namrossbs.Config{
		Path:      filepath.Join(root, "metadata"),
		StatePath: filepath.Join(root, "namros-sbs-state.json"),
		Stores: []sbslocal.StoreSpec{
			{
				ID:     "store-a",
				Path:   filepath.Join(root, "store-a"),
				Shards: 1,
				Weight: 100,
			},
		},
		VolumeID:        0x0a0b0002,
		VolumeName:      "namros-test",
		VolumeSizeBytes: 128 * 1024 * 1024,
	}
}

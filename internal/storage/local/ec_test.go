package local

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storageclass"
)

func TestECStorageClassPutGetAndDegradedRead(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	store, err := NewWithClock(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewWithClock() error = %v", err)
	}
	snapshot, err := storageclass.DefaultResolver().Resolve(storageclass.ResolveRequest{
		RequestedID: "EC_4_2",
	})
	if err != nil {
		t.Fatalf("Resolve(EC_4_2) error = %v", err)
	}
	payload := bytes.Repeat([]byte("namros-ec-data-"), 8192)
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:       bytes.NewReader(payload),
		SizeBytes:    uint64(len(payload)),
		StorageClass: snapshot,
	})
	if err != nil {
		t.Fatalf("PutSegment(EC) error = %v", err)
	}
	if ref.Placement.Layout != ecLayout || ref.Placement.RedundancyBackend != storageclass.RedundancyErasureCode {
		t.Fatalf("EC placement = %+v", ref.Placement)
	}
	if len(ref.Placement.Chunks) != 6 {
		t.Fatalf("placement chunks = %d, want 6", len(ref.Placement.Chunks))
	}
	got := readSegment(t, store, ref, 0, 0)
	if !bytes.Equal(got, payload) {
		t.Fatalf("full EC read mismatch")
	}
	ranged := readSegment(t, store, ref, 17, 31)
	if !bytes.Equal(ranged, payload[17:48]) {
		t.Fatalf("range EC read = %q, want %q", ranged, payload[17:48])
	}

	if err := os.Remove(filepath.Join(store.segmentPath(ref.SegmentID), ecShardFile(0))); err != nil {
		t.Fatalf("remove data shard: %v", err)
	}
	degraded := readSegment(t, store, ref, 0, 0)
	if !bytes.Equal(degraded, payload) {
		t.Fatalf("degraded EC read mismatch")
	}

	if err := store.DeleteSegment(t.Context(), ref, storage.DeleteReasonManualGC); err != nil {
		t.Fatalf("DeleteSegment(EC) error = %v", err)
	}
	if _, err := store.GetSegment(t.Context(), ref, 0, 0); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetSegment(deleted EC) error = %v, want ErrNotFound", err)
	}
}

func TestECStorageClassPutStreamsByShard(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	snapshot, err := storageclass.DefaultResolver().Resolve(storageclass.ResolveRequest{
		RequestedID: "EC_4_2",
	})
	if err != nil {
		t.Fatalf("Resolve(EC_4_2) error = %v", err)
	}
	payload := bytes.Repeat([]byte("namros-local-ec-stream-"), 12)
	maxRead := ceilDivInt(len(payload), 4)
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:       &maxLocalECReadSizeReader{data: payload, max: maxRead},
		SizeBytes:    uint64(len(payload)),
		StorageClass: snapshot,
	})
	if err != nil {
		t.Fatalf("PutSegment(EC streaming) error = %v", err)
	}
	wantShardSize := uint64(maxRead)
	if ref.Placement.ChunkSizeBytes != wantShardSize {
		t.Fatalf("shard size = %d, want %d", ref.Placement.ChunkSizeBytes, wantShardSize)
	}
	got := readSegment(t, store, ref, 0, 0)
	if !bytes.Equal(got, payload) {
		t.Fatalf("full streaming EC read mismatch")
	}
}

func readSegment(t *testing.T, store *Store, ref storage.SegmentRef, off, length uint64) []byte {
	t.Helper()
	reader, err := store.GetSegment(t.Context(), ref, off, length)
	if err != nil {
		t.Fatalf("GetSegment(off=%d length=%d) error = %v", off, length, err)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return payload
}

type maxLocalECReadSizeReader struct {
	data []byte
	max  int
	off  int
}

func (r *maxLocalECReadSizeReader) Read(p []byte) (int, error) {
	if len(p) > r.max {
		return 0, errors.New("read buffer exceeds configured maximum")
	}
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

package sbs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	sbsservice "github.com/nosway/namrbd/gateway/service"

	"github.com/nosway/namros/internal/storage"
	namrossbs "github.com/nosway/namros/internal/storage/sbs"
	"github.com/nosway/namros/internal/storage/testsuite"
)

func TestPhysicalSegmentStoreSuite(t *testing.T) {
	testsuite.RunSegmentStoreTests(t, func(t *testing.T) testsuite.SegmentStoreUnderTest {
		t.Helper()
		return newPhysicalTestStore(t, 8)
	})
}

func TestPhysicalSegmentStoreAllocatesAndRecordsChunks(t *testing.T) {
	store := newPhysicalTestStore(t, 8)
	payload := []byte("abcdefghijklmnopq")
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader(payload),
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if ref.StorageClass.Backend != "sbs-physical" {
		t.Fatalf("storage backend = %q, want sbs-physical", ref.StorageClass.Backend)
	}
	if ref.Placement.Layout != "sbs-physical-chunks" || ref.Placement.RedundancyBackend != "replicated" {
		t.Fatalf("placement snapshot = %+v", ref.Placement)
	}
	if ref.Placement.ChunkSizeBytes != 8 {
		t.Fatalf("chunk size = %d, want 8", ref.Placement.ChunkSizeBytes)
	}
	if len(ref.Placement.Chunks) != 3 {
		t.Fatalf("placement chunk len = %d, want 3", len(ref.Placement.Chunks))
	}
	if got := ref.Placement.Chunks[0].ChunkID; got != 100 {
		t.Fatalf("first chunk id = %d, want 100", got)
	}
	if got := ref.Placement.Chunks[2].SizeBytes; got != 1 {
		t.Fatalf("last chunk size = %d, want 1", got)
	}

	reader, err := store.GetSegment(t.Context(), ref, 5, 9)
	if err != nil {
		t.Fatalf("GetSegment(range) error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "fghijklmn" {
		t.Fatalf("range = %q, want fghijklmn", got)
	}
}

func TestPhysicalSegmentStoreMarksFailedPublishOrphan(t *testing.T) {
	client := newFakePhysicalClient(8)
	client.failWriteChunkID = 101
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdefghijklmnopq")),
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("PutSegment() error = %v, want ErrUnavailable", err)
	}
	candidates, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidate len = %d, want 1", len(candidates))
	}
	if candidates[0].Reason != storage.DeleteReasonPublishFailed {
		t.Fatalf("candidate reason = %q, want publish_failed", candidates[0].Reason)
	}
	if len(candidates[0].Ref.Placement.Chunks) != 3 {
		t.Fatalf("candidate placement chunks = %+v, want 3 chunks", candidates[0].Ref.Placement.Chunks)
	}
	if got := candidates[0].Ref.Placement.Parameters["start_chunk_id"]; got != "100" {
		t.Fatalf("candidate start_chunk_id = %q, want 100", got)
	}
}

func TestPhysicalSegmentStoreRejectsStaleSessionBeforeAllocation(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	allocator := &fakePhysicalAllocator{next: 100}
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		SessionIdentity: namrossbs.SessionIdentity{
			VolumeEpoch:       1,
			WriterGroupID:     "object-writers",
			SessionID:         "gateway-instance-a-boot-1",
			SessionGeneration: 1,
			SessionTTL:        30 * time.Second,
			HeartbeatInterval: 10 * time.Second,
		},
		SessionFence: namrossbs.SessionFenceSnapshot{
			VolumeID:             "0a0b0002",
			WriterGroupID:        "object-writers",
			VolumeEpoch:          1,
			MinSessionGeneration: 2,
			ExpiresAt:            now.Add(time.Minute),
		},
		Allocator: allocator,
		Client:    newFakePhysicalClient(8),
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("stale generation")),
		SizeBytes: uint64(len("stale generation")),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if !errors.Is(err, storage.ErrUnavailable) || !strings.Contains(err.Error(), "stale_session_generation") {
		t.Fatalf("PutSegment(stale session) error = %v, want stale session ErrUnavailable", err)
	}
	if allocator.next != 100 {
		t.Fatalf("allocator next = %d, want unchanged 100", allocator.next)
	}
}

func TestPhysicalSegmentStoreDeleteAdmissionBlocksDelete(t *testing.T) {
	client := newFakePhysicalClient(8)
	called := false
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
		DeleteAdmission: func(_ context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
			called = true
			if ref.SegmentID == "" {
				t.Fatal("DeleteAdmission received empty segment id")
			}
			if reason != storage.DeleteReasonManualGC {
				t.Fatalf("DeleteAdmission reason = %q, want manual_gc", reason)
			}
			return storage.ErrProtected
		},
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("protected physical")),
		SizeBytes: uint64(len("protected physical")),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
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
	if len(client.chunks) == 0 {
		t.Fatal("physical chunks were deleted despite admission denial")
	}
}

func TestPhysicalSegmentStoreUsesChunkAlignedWritesAndPartialReads(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdefghijklmnopq")),
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if len(client.writeRequests) != 3 {
		t.Fatalf("write request len = %d, want 3", len(client.writeRequests))
	}
	for i, req := range client.writeRequests[:2] {
		if req.ChunkOffsetBytes != 0 || req.LengthBytes != 8 || len(req.Data) != 8 {
			t.Fatalf("write[%d] offset=%d length=%d data=%d, want full 8-byte chunk", i, req.ChunkOffsetBytes, req.LengthBytes, len(req.Data))
		}
	}
	if req := client.writeRequests[2]; req.ChunkOffsetBytes != 0 || req.LengthBytes != 1 || string(req.Data) != "q" {
		t.Fatalf("last write offset=%d length=%d data=%q, want one-byte tail", req.ChunkOffsetBytes, req.LengthBytes, req.Data)
	}

	reader, err := store.GetSegment(t.Context(), ref, 5, 9)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "fghijklmn" {
		t.Fatalf("range = %q, want fghijklmn", got)
	}
	if len(client.readRequests) != 2 {
		t.Fatalf("read request len = %d, want 2", len(client.readRequests))
	}
	wantReads := []struct {
		offset uint64
		length uint64
	}{
		{offset: 5, length: 3},
		{offset: 0, length: 6},
	}
	for i, req := range client.readRequests {
		if req.ChunkOffsetBytes != wantReads[i].offset || req.LengthBytes != wantReads[i].length {
			t.Fatalf("read[%d] offset=%d length=%d, want offset=%d length=%d", i, req.ChunkOffsetBytes, req.LengthBytes, wantReads[i].offset, wantReads[i].length)
		}
	}
}

func TestPhysicalSegmentStorePadsTailChunkWhenConfigured(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:               "0a0b0002",
		ChunkSizeBytes:         8,
		GatewayID:              "gw-test",
		AttachmentID:           "att-test",
		Generation:             1,
		FullChunkWriteMinBytes: 1,
		FullChunkWriteMaxBytes: 8,
		Allocator:              &fakePhysicalAllocator{next: 100},
		Client:                 client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abc")),
		SizeBytes: 3,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if len(client.writeRequests) != 1 {
		t.Fatalf("write request len = %d, want 1", len(client.writeRequests))
	}
	req := client.writeRequests[0]
	if req.ChunkOffsetBytes != 0 || req.LengthBytes != 8 || len(req.Data) != 8 {
		t.Fatalf("write offset=%d length=%d data=%d, want padded full chunk", req.ChunkOffsetBytes, req.LengthBytes, len(req.Data))
	}
	if string(req.Data[:3]) != "abc" || !bytes.Equal(req.Data[3:], make([]byte, 5)) {
		t.Fatalf("padded data = %q, want abc followed by zeros", req.Data)
	}
	if ref.SizeBytes != 3 || ref.Placement.Chunks[0].SizeBytes != 3 {
		t.Fatalf("logical ref size = %d chunk size = %d, want 3/3", ref.SizeBytes, ref.Placement.Chunks[0].SizeBytes)
	}
	reader, err := store.GetSegment(t.Context(), ref, 0, 0)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "abc" {
		t.Fatalf("payload = %q, want abc", got)
	}
}

func TestPhysicalSegmentStoreDoesNotPadTailChunkAboveConfiguredMax(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:               "0a0b0002",
		ChunkSizeBytes:         8,
		GatewayID:              "gw-test",
		AttachmentID:           "att-test",
		Generation:             1,
		FullChunkWriteMinBytes: 1,
		FullChunkWriteMaxBytes: 4,
		Allocator:              &fakePhysicalAllocator{next: 100},
		Client:                 client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abc")),
		SizeBytes: 3,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if len(client.writeRequests) != 1 {
		t.Fatalf("write request len = %d, want 1", len(client.writeRequests))
	}
	if req := client.writeRequests[0]; req.LengthBytes != 3 || string(req.Data) != "abc" {
		t.Fatalf("write length=%d data=%q, want unpadded 3-byte write", req.LengthBytes, req.Data)
	}
}

func TestPhysicalSegmentStoreServesCachedFullChunkWrites(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:               "0a0b0002",
		ChunkSizeBytes:         8,
		GatewayID:              "gw-test",
		AttachmentID:           "att-test",
		Generation:             1,
		FullChunkWriteMinBytes: 1,
		FullChunkWriteMaxBytes: 8,
		ChunkCacheBytes:        64,
		Allocator:              &fakePhysicalAllocator{next: 100},
		Client:                 client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdef")),
		SizeBytes: 6,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	reader, err := store.GetSegment(t.Context(), ref, 1, 3)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "bcd" {
		t.Fatalf("cached range = %q, want bcd", got)
	}
	if len(client.readRequests) != 0 {
		t.Fatalf("read request len = %d, want cache hit without SBS read", len(client.readRequests))
	}
}

func TestPhysicalSegmentStoreVerifyReadbackBypassesChunkCache(t *testing.T) {
	client := newFakePhysicalClient(8)
	client.corruptReadChunkID = 100
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:               "0a0b0002",
		ChunkSizeBytes:         8,
		GatewayID:              "gw-test",
		AttachmentID:           "att-test",
		Generation:             1,
		VerifyReadback:         true,
		FullChunkWriteMinBytes: 1,
		FullChunkWriteMaxBytes: 8,
		ChunkCacheBytes:        64,
		Allocator:              &fakePhysicalAllocator{next: 100},
		Client:                 client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abc")),
		SizeBytes: 3,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("PutSegment() error = %v, want ErrUnavailable", err)
	}
	if len(client.readRequests) == 0 {
		t.Fatal("verify readback used cache; want SBS read request")
	}
}

func TestPhysicalSegmentStoreStreamsPutByChunk(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	payload := []byte("abcdefghijklmnopq")
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    &maxReadSizeReader{data: payload, max: 8},
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if len(client.writeRequests) != 3 {
		t.Fatalf("write request len = %d, want 3", len(client.writeRequests))
	}
	reader, err := store.GetSegment(t.Context(), ref, 0, 0)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestPhysicalSegmentStoreStreamsGetByChunk(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	payload := []byte("abcdefghijklmnopq")
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader(payload),
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	reader, err := store.GetSegment(t.Context(), ref, 0, 0)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("Read(first) error = %v", err)
	}
	if n != 5 || string(buf) != "abcde" {
		t.Fatalf("first read = %d/%q, want 5/abcde", n, buf)
	}
	if len(client.readRequests) != 1 {
		t.Fatalf("read request len after first read = %d, want 1", len(client.readRequests))
	}
	if req := client.readRequests[0]; req.PhysicalChunkID != 100 || req.ChunkOffsetBytes != 0 || req.LengthBytes != 8 {
		t.Fatalf("first SBS read = chunk %d offset %d length %d, want chunk 100 offset 0 length 8", req.PhysicalChunkID, req.ChunkOffsetBytes, req.LengthBytes)
	}
	n, err = reader.Read(buf[:3])
	if err != nil {
		t.Fatalf("Read(buffered tail) error = %v", err)
	}
	if n != 3 || string(buf[:3]) != "fgh" {
		t.Fatalf("buffered read = %d/%q, want 3/fgh", n, buf[:3])
	}
	if len(client.readRequests) != 1 {
		t.Fatalf("read request len after buffered tail = %d, want still 1", len(client.readRequests))
	}
	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(rest) error = %v", err)
	}
	if string(rest) != "ijklmnopq" {
		t.Fatalf("rest = %q, want ijklmnopq", rest)
	}
	if len(client.readRequests) != 3 {
		t.Fatalf("read request len after full stream = %d, want 3", len(client.readRequests))
	}
}

func TestPhysicalSegmentStoreWritesChunksWithBoundedConcurrency(t *testing.T) {
	client := newFakePhysicalClient(8)
	client.writeDelay = 20 * time.Millisecond
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:         "0a0b0002",
		ChunkSizeBytes:   8,
		GatewayID:        "gw-test",
		AttachmentID:     "att-test",
		Generation:       1,
		WriteConcurrency: 2,
		Allocator:        &fakePhysicalAllocator{next: 100},
		Client:           client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	payload := []byte("abcdefghijklmnopqrstuvwxyz012345")
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader(payload),
		SizeBytes: uint64(len(payload)),
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if len(client.writeRequests) != 4 {
		t.Fatalf("write request len = %d, want 4", len(client.writeRequests))
	}
	if client.maxActiveWrites != 2 {
		t.Fatalf("max active writes = %d, want 2", client.maxActiveWrites)
	}
	reader, err := store.GetSegment(t.Context(), ref, 0, 0)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("payload = %q, want %q", got, payload)
	}
}

func TestPhysicalSegmentStoreVerifyReadbackPasses(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		VerifyReadback: true,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdefghijklmnopq")),
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if ref.SegmentID == "" {
		t.Fatal("SegmentID is empty")
	}
	candidates, err := store.ListRepairCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListRepairCandidates() error = %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("repair candidates = %+v, want none", candidates)
	}
}

func TestPhysicalSegmentStoreVerifyReadbackRecordsRepairCandidate(t *testing.T) {
	client := newFakePhysicalClient(8)
	client.corruptReadChunkID = 101
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		VerifyReadback: true,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdefghijklmnopq")),
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("PutSegment() error = %v, want ErrUnavailable", err)
	}
	repairCandidates, err := store.ListRepairCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListRepairCandidates() error = %v", err)
	}
	if len(repairCandidates) != 1 {
		t.Fatalf("repair candidate len = %d, want 1", len(repairCandidates))
	}
	if repairCandidates[0].Reason != namrossbs.PhysicalRepairReasonWriteReadbackMismatch {
		t.Fatalf("repair reason = %q, want readback mismatch", repairCandidates[0].Reason)
	}
	if got := repairCandidates[0].Ref.Placement.Parameters["start_chunk_id"]; got != "100" {
		t.Fatalf("repair start_chunk_id = %q, want 100", got)
	}
	gcCandidates, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(gcCandidates) != 1 || gcCandidates[0].Reason != storage.DeleteReasonPublishFailed {
		t.Fatalf("gc candidates = %+v, want publish_failed orphan", gcCandidates)
	}
}

func TestPhysicalSegmentStoreReadFailureRecordsRepairCandidate(t *testing.T) {
	client := newFakePhysicalClient(8)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdefghijklmnopq")),
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}

	client.failReadChunkID = 101
	reader, err := store.GetSegment(t.Context(), ref, 0, 0)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	_, err = io.ReadAll(reader)
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("ReadAll() error = %v, want ErrUnavailable", err)
	}
	repairCandidates, err := store.ListRepairCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListRepairCandidates() error = %v", err)
	}
	if len(repairCandidates) != 1 {
		t.Fatalf("repair candidate len = %d, want 1", len(repairCandidates))
	}
	if repairCandidates[0].Reason != namrossbs.PhysicalRepairReasonReadFailed {
		t.Fatalf("repair reason = %q, want read_failed", repairCandidates[0].Reason)
	}
	if got := repairCandidates[0].Ref.SegmentID; got != ref.SegmentID {
		t.Fatalf("repair ref = %q, want %q", got, ref.SegmentID)
	}
}

func newPhysicalTestStore(t *testing.T, chunkSize uint64) *namrossbs.PhysicalStore {
	t.Helper()
	client := newFakePhysicalClient(chunkSize)
	store, err := namrossbs.NewPhysicalStore(namrossbs.PhysicalConfig{
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: chunkSize,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		Allocator:      &fakePhysicalAllocator{next: 100},
		Client:         client,
	})
	if err != nil {
		t.Fatalf("NewPhysicalStore() error = %v", err)
	}
	return store
}

type fakePhysicalAllocator struct {
	next uint64
}

func (a *fakePhysicalAllocator) AllocateChunkIDs(_ context.Context, _ string, count uint32) (uint64, error) {
	start := a.next
	a.next += uint64(count)
	return start, nil
}

type fakePhysicalClient struct {
	mu                 sync.Mutex
	chunkSize          uint64
	failWriteChunkID   uint64
	failReadChunkID    uint64
	corruptReadChunkID uint64
	writeDelay         time.Duration
	activeWrites       int
	maxActiveWrites    int
	readRequests       []sbsservice.ReadPhysicalChunkRequest
	writeRequests      []sbsservice.WritePhysicalChunkRequest
	chunks             map[uint64][]byte
}

func newFakePhysicalClient(chunkSize uint64) *fakePhysicalClient {
	return &fakePhysicalClient{
		chunkSize: chunkSize,
		chunks:    make(map[uint64][]byte),
	}
}

func (c *fakePhysicalClient) ReadPhysicalChunk(_ context.Context, req *sbsservice.ReadPhysicalChunkRequest) (*sbsservice.ReadPhysicalChunkResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readRequests = append(c.readRequests, *req)
	if c.failReadChunkID != 0 && req.PhysicalChunkID == c.failReadChunkID {
		return nil, &sbsservice.SBSError{
			Code:      sbsservice.SBSErrorCodeUnavailable,
			Message:   "injected physical chunk read failure",
			Retryable: true,
		}
	}
	chunk := c.chunk(req.PhysicalChunkID)
	end := req.ChunkOffsetBytes + req.LengthBytes
	if req.ChunkOffsetBytes > uint64(len(chunk)) || end > uint64(len(chunk)) {
		return nil, sbsservice.ErrOutOfRange
	}
	data := append([]byte(nil), chunk[req.ChunkOffsetBytes:end]...)
	if c.corruptReadChunkID != 0 && req.PhysicalChunkID == c.corruptReadChunkID && len(data) > 0 {
		data[0] ^= 0xff
	}
	return &sbsservice.ReadPhysicalChunkResponse{
		VolumeID:         req.VolumeID,
		PhysicalChunkID:  req.PhysicalChunkID,
		ChunkOffsetBytes: req.ChunkOffsetBytes,
		LengthBytes:      req.LengthBytes,
		Data:             data,
	}, nil
}

func (c *fakePhysicalClient) WritePhysicalChunk(_ context.Context, req *sbsservice.WritePhysicalChunkRequest) (*sbsservice.WritePhysicalChunkResponse, error) {
	c.mu.Lock()
	c.writeRequests = append(c.writeRequests, *req)
	if c.failWriteChunkID != 0 && req.PhysicalChunkID == c.failWriteChunkID {
		c.mu.Unlock()
		return nil, &sbsservice.SBSError{
			Code:      sbsservice.SBSErrorCodeUnavailable,
			Message:   "injected physical chunk write failure",
			Retryable: true,
		}
	}
	c.activeWrites++
	if c.activeWrites > c.maxActiveWrites {
		c.maxActiveWrites = c.activeWrites
	}
	delay := c.writeDelay
	c.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	defer func() {
		c.activeWrites--
	}()
	chunk := c.chunk(req.PhysicalChunkID)
	end := req.ChunkOffsetBytes + req.LengthBytes
	if req.ChunkOffsetBytes > uint64(len(chunk)) || end > uint64(len(chunk)) {
		return nil, sbsservice.ErrOutOfRange
	}
	copy(chunk[req.ChunkOffsetBytes:end], req.Data)
	c.chunks[req.PhysicalChunkID] = chunk
	return &sbsservice.WritePhysicalChunkResponse{
		Status:           "ok",
		VolumeID:         req.VolumeID,
		PhysicalChunkID:  req.PhysicalChunkID,
		ChunkOffsetBytes: req.ChunkOffsetBytes,
		LengthBytes:      req.LengthBytes,
	}, nil
}

func (c *fakePhysicalClient) DeletePhysicalChunk(_ context.Context, _ string, chunkID uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.chunks, chunkID)
	return nil
}

func (c *fakePhysicalClient) chunk(chunkID uint64) []byte {
	chunk, ok := c.chunks[chunkID]
	if ok {
		return append([]byte(nil), chunk...)
	}
	return make([]byte, c.chunkSize)
}

type maxReadSizeReader struct {
	data []byte
	max  int
	off  int
}

func (r *maxReadSizeReader) Read(p []byte) (int, error) {
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

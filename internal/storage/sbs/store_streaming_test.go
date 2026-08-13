package sbs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"

	"github.com/nosway/namros/internal/storage"
)

func TestStreamLegacySegmentWritesChunksAndPads(t *testing.T) {
	payloadSize := legacySegmentReadChunkSize*2 + 17
	payload := bytes.Repeat([]byte("x"), int(payloadSize))
	hasher := sha256.New()
	var writes []legacyWrite

	err := streamLegacySegmentWrites(t.Context(), &boundedReader{
		data: append([]byte(nil), payload...),
		max:  int(legacySegmentReadChunkSize),
	}, payloadSize, 4096, hasher, func(relativeOffset uint64, data []byte) error {
		writes = append(writes, legacyWrite{
			offset: relativeOffset,
			data:   append([]byte(nil), data...),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("streamLegacySegmentWrites() error = %v", err)
	}
	if len(writes) != 3 {
		t.Fatalf("write count = %d, want 3", len(writes))
	}
	wantOffsets := []uint64{0, legacySegmentReadChunkSize, legacySegmentReadChunkSize * 2}
	wantLengths := []int{int(legacySegmentReadChunkSize), int(legacySegmentReadChunkSize), 4096}
	for i := range writes {
		if writes[i].offset != wantOffsets[i] || len(writes[i].data) != wantLengths[i] {
			t.Fatalf("write[%d] offset/len = %d/%d, want %d/%d", i, writes[i].offset, len(writes[i].data), wantOffsets[i], wantLengths[i])
		}
	}
	tail := writes[2].data
	if !bytes.Equal(tail[:17], bytes.Repeat([]byte("x"), 17)) {
		t.Fatalf("tail payload = %q, want 17 x bytes", tail[:17])
	}
	if !bytes.Equal(tail[17:], make([]byte, len(tail)-17)) {
		t.Fatal("tail padding contains non-zero bytes")
	}
	wantDigest := sha256.Sum256(payload)
	if gotDigest := hex.EncodeToString(hasher.Sum(nil)); gotDigest != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("digest = %s, want %s", gotDigest, hex.EncodeToString(wantDigest[:]))
	}
}

func TestStreamLegacySegmentWritesAlignsNonDivisorBlockSize(t *testing.T) {
	blockSize := uint64(10 * 1024)
	chunkSize := legacySegmentWritePayloadChunkSize(blockSize)
	if chunkSize%blockSize != 0 {
		t.Fatalf("chunkSize = %d, not aligned to blockSize %d", chunkSize, blockSize)
	}
	if chunkSize > legacySegmentReadChunkSize {
		t.Fatalf("chunkSize = %d exceeds nominal legacy chunk %d", chunkSize, legacySegmentReadChunkSize)
	}
	payloadSize := chunkSize + 1
	payload := bytes.Repeat([]byte("y"), int(payloadSize))
	hasher := sha256.New()
	var writes []legacyWrite

	err := streamLegacySegmentWrites(t.Context(), bytes.NewReader(payload), payloadSize, blockSize, hasher, func(relativeOffset uint64, data []byte) error {
		writes = append(writes, legacyWrite{
			offset: relativeOffset,
			data:   append([]byte(nil), data...),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("streamLegacySegmentWrites() error = %v", err)
	}
	if len(writes) != 2 {
		t.Fatalf("write count = %d, want 2", len(writes))
	}
	if writes[0].offset != 0 || uint64(len(writes[0].data)) != chunkSize {
		t.Fatalf("first write offset/len = %d/%d, want 0/%d", writes[0].offset, len(writes[0].data), chunkSize)
	}
	if writes[1].offset != chunkSize || uint64(len(writes[1].data)) != blockSize {
		t.Fatalf("second write offset/len = %d/%d, want %d/%d", writes[1].offset, len(writes[1].data), chunkSize, blockSize)
	}
	totalWritten := uint64(len(writes[0].data) + len(writes[1].data))
	if totalWritten != alignUp(payloadSize, blockSize) {
		t.Fatalf("total written = %d, want padded length %d", totalWritten, alignUp(payloadSize, blockSize))
	}
}

func TestStreamLegacySegmentWritesRejectsTrailingBytes(t *testing.T) {
	hasher := sha256.New()
	writes := 0
	err := streamLegacySegmentWrites(t.Context(), bytes.NewReader([]byte("abcd")), 3, 4096, hasher, func(_ uint64, _ []byte) error {
		writes++
		return nil
	})
	if !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("streamLegacySegmentWrites() error = %v, want ErrInvalidArgument", err)
	}
	if writes != 1 {
		t.Fatalf("writes = %d, want 1", writes)
	}
}

func TestLegacySegmentReaderReadsChunksLazily(t *testing.T) {
	payloadSize := legacySegmentReadChunkSize*2 + 17
	payload := bytes.Repeat([]byte("z"), int(payloadSize))
	var reads []legacyRead
	reader := &legacySegmentReader{
		ctx:        t.Context(),
		nextOffset: 0,
		remaining:  payloadSize,
		readChunk: func(_ context.Context, relativeOffset, length uint64) ([]byte, error) {
			reads = append(reads, legacyRead{offset: relativeOffset, length: length})
			end := relativeOffset + length
			return append([]byte(nil), payload[relativeOffset:end]...), nil
		},
	}
	defer reader.Close()

	buf := make([]byte, 5)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("Read(first) error = %v", err)
	}
	if n != 5 || string(buf) != "zzzzz" {
		t.Fatalf("first read = %d/%q, want 5 z bytes", n, buf)
	}
	if len(reads) != 1 {
		t.Fatalf("read chunk count after first read = %d, want 1", len(reads))
	}
	if reads[0].offset != 0 || reads[0].length != legacySegmentReadChunkSize {
		t.Fatalf("first chunk read = offset %d length %d, want 0/%d", reads[0].offset, reads[0].length, legacySegmentReadChunkSize)
	}

	n, err = reader.Read(buf)
	if err != nil {
		t.Fatalf("Read(buffered) error = %v", err)
	}
	if n != 5 || len(reads) != 1 {
		t.Fatalf("buffered read n=%d chunks=%d, want 5 chunks still 1", n, len(reads))
	}

	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(rest) error = %v", err)
	}
	if uint64(len(rest))+10 != payloadSize {
		t.Fatalf("remaining bytes = %d, want %d", len(rest), payloadSize-10)
	}
	if len(reads) != 3 {
		t.Fatalf("read chunk count after full stream = %d, want 3", len(reads))
	}
	if reads[2].offset != legacySegmentReadChunkSize*2 || reads[2].length != 17 {
		t.Fatalf("tail chunk read = offset %d length %d, want %d/17", reads[2].offset, reads[2].length, legacySegmentReadChunkSize*2)
	}
}

type legacyWrite struct {
	offset uint64
	data   []byte
}

type legacyRead struct {
	offset uint64
	length uint64
}

type boundedReader struct {
	data []byte
	max  int
}

func (r *boundedReader) Read(p []byte) (int, error) {
	if len(p) > r.max {
		return 0, errors.New("read buffer exceeds max")
	}
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

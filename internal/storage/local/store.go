package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/klauspost/reedsolomon"

	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storageclass"
)

const ecLayout = "local-ec-shards"

type Store struct {
	root    string
	now     func() time.Time
	mu      sync.Mutex
	gcQueue map[string]storage.GCCandidate
}

func New(root string) (*Store, error) {
	return NewWithClock(root, func() time.Time { return time.Now().UTC() })
}

func NewWithClock(root string, now func() time.Time) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: root is required", storage.ErrInvalidArgument)
	}
	store := &Store{
		root:    root,
		now:     now,
		gcQueue: make(map[string]storage.GCCandidate),
	}
	if err := os.MkdirAll(store.segmentDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(store.tmpDir(), 0o755); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) CheckHealth(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, dir := range []string{s.segmentDir(), s.tmpDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: %s is not a directory", storage.ErrUnavailable, dir)
		}
	}
	return nil
}

func (s *Store) PutSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
	if err := ctx.Err(); err != nil {
		return storage.SegmentRef{}, err
	}
	if req.Reader == nil {
		return storage.SegmentRef{}, fmt.Errorf("%w: reader is required", storage.ErrInvalidArgument)
	}
	if req.StorageClass.StorageClassID == "" {
		return storage.SegmentRef{}, fmt.Errorf("%w: storage class id is required", storage.ErrInvalidArgument)
	}
	if isECStorageClass(req.StorageClass) {
		return s.putECSegment(ctx, req)
	}
	tmp, err := os.CreateTemp(s.tmpDir(), "segment-*")
	if err != nil {
		return storage.SegmentRef{}, err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), req.Reader)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return storage.SegmentRef{}, err
	}
	if req.SizeBytes != 0 && uint64(written) != req.SizeBytes {
		return storage.SegmentRef{}, fmt.Errorf("%w: wrote %d bytes, expected %d", storage.ErrInvalidArgument, written, req.SizeBytes)
	}
	segmentID, err := newSegmentID()
	if err != nil {
		return storage.SegmentRef{}, err
	}
	finalPath := s.segmentPath(segmentID)
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return storage.SegmentRef{}, err
	}
	storageClass := storage.CloneStorageClassSnapshot(req.StorageClass)
	placementBackend := storageClass.Backend
	if placementBackend == "" {
		placementBackend = "local"
	}
	ref := storage.SegmentRef{
		SegmentID:    segmentID,
		StorageClass: storageClass,
		Placement: storage.PlacementSnapshot{
			Backend:   placementBackend,
			Layout:    "local-file",
			ProfileID: storageClass.StorageClassID,
			Chunks: []storage.PlacementChunk{{
				LogicalOffsetBytes: 0,
				SizeBytes:          uint64(written),
				LengthBytes:        uint64(written),
				Role:               "primary",
			}},
		},
		SizeBytes: uint64(written),
		Digest: storage.Digest{
			Algorithm: "sha256",
			Hex:       hex.EncodeToString(hasher.Sum(nil)),
		},
		CreatedAt: s.now(),
	}
	return ref, nil
}

func (s *Store) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ref.SegmentID == "" {
		return nil, fmt.Errorf("%w: segment id is required", storage.ErrInvalidArgument)
	}
	if ref.Placement.Layout == ecLayout {
		return s.getECSegment(ctx, ref, off, length)
	}
	file, err := os.Open(s.segmentPath(ref.SegmentID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if off > uint64(info.Size()) {
		_ = file.Close()
		return nil, storage.ErrInvalidRange
	}
	if _, err := file.Seek(int64(off), io.SeekStart); err != nil {
		_ = file.Close()
		return nil, err
	}
	if length == 0 {
		return file, nil
	}
	return &limitedReadCloser{Reader: io.LimitReader(file, int64(length)), Closer: file}, nil
}

func (s *Store) DeleteSegment(ctx context.Context, ref storage.SegmentRef, _ storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ref.SegmentID == "" {
		return fmt.Errorf("%w: segment id is required", storage.ErrInvalidArgument)
	}
	if ref.Placement.Layout == ecLayout {
		if err := os.RemoveAll(s.segmentPath(ref.SegmentID)); err != nil {
			return err
		}
		s.mu.Lock()
		delete(s.gcQueue, ref.SegmentID)
		s.mu.Unlock()
		return nil
	}
	if err := os.Remove(s.segmentPath(ref.SegmentID)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storage.ErrNotFound
		}
		return err
	}
	s.mu.Lock()
	delete(s.gcQueue, ref.SegmentID)
	s.mu.Unlock()
	return nil
}

func (s *Store) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ref.SegmentID == "" {
		return fmt.Errorf("%w: segment id is required", storage.ErrInvalidArgument)
	}
	if _, err := os.Stat(s.segmentPath(ref.SegmentID)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storage.ErrNotFound
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gcQueue[ref.SegmentID] = storage.GCCandidate{
		Ref:       ref,
		Reason:    reason,
		CreatedAt: s.now(),
	}
	return nil
}

func (s *Store) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidates := make([]storage.GCCandidate, 0, len(s.gcQueue))
	for _, candidate := range s.gcQueue {
		candidates = append(candidates, candidate)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	if limit > 0 && len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return candidates, nil
}

func (s *Store) segmentDir() string {
	return filepath.Join(s.root, "segments")
}

func (s *Store) tmpDir() string {
	return filepath.Join(s.root, "tmp")
}

func (s *Store) segmentPath(segmentID string) string {
	return filepath.Join(s.segmentDir(), segmentID)
}

func (s *Store) putECSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
	if err := ctx.Err(); err != nil {
		return storage.SegmentRef{}, err
	}
	if req.SizeBytes == 0 {
		if err := ensureNoExtraBytes(req.Reader); err != nil {
			return storage.SegmentRef{}, err
		}
		return storage.SegmentRef{}, fmt.Errorf("%w: ec segment payload is empty", storage.ErrInvalidArgument)
	}
	if req.SizeBytes > uint64(maxIntValue()) {
		return storage.SegmentRef{}, fmt.Errorf("%w: ec segment exceeds maximum supported size", storage.ErrInvalidArgument)
	}
	size := int(req.SizeBytes)
	dataShards, parityShards, err := ecShardConfig(req.StorageClass)
	if err != nil {
		return storage.SegmentRef{}, err
	}
	encoder, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return storage.SegmentRef{}, fmt.Errorf("%w: create ec encoder: %v", storage.ErrInvalidArgument, err)
	}
	totalShards := dataShards + parityShards
	shardSize := ceilDivInt(size, dataShards)
	if shardSize < 1 {
		shardSize = 1
	}
	shards := make([][]byte, totalShards)
	hasher := sha256.New()
	readPayloadBytes := uint64(0)
	for shardID := 0; shardID < dataShards; shardID++ {
		shard := make([]byte, shardSize)
		start := shardID * shardSize
		logicalLength := 0
		if start < size {
			logicalLength = minInt(shardSize, size-start)
		}
		if logicalLength > 0 {
			n, err := io.ReadFull(req.Reader, shard[:logicalLength])
			if err != nil {
				return storage.SegmentRef{}, fmt.Errorf("%w: read %d bytes, expected %d", storage.ErrInvalidArgument, readPayloadBytes+uint64(n), req.SizeBytes)
			}
			readPayloadBytes += uint64(n)
			if _, err := hasher.Write(shard[:logicalLength]); err != nil {
				return storage.SegmentRef{}, err
			}
		}
		shards[shardID] = shard
	}
	if err := ensureNoExtraBytes(req.Reader); err != nil {
		return storage.SegmentRef{}, err
	}
	for shardID := dataShards; shardID < totalShards; shardID++ {
		shards[shardID] = make([]byte, shardSize)
	}
	if err := encoder.Encode(shards); err != nil {
		return storage.SegmentRef{}, fmt.Errorf("%w: encode ec payload: %v", storage.ErrUnavailable, err)
	}
	segmentID, err := newSegmentID()
	if err != nil {
		return storage.SegmentRef{}, err
	}
	segmentDir := s.segmentPath(segmentID)
	tmpDir, err := os.MkdirTemp(s.tmpDir(), "ec-segment-*")
	if err != nil {
		return storage.SegmentRef{}, err
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()
	for shardID, shard := range shards {
		if err := os.WriteFile(filepath.Join(tmpDir, ecShardFile(shardID)), shard, 0o644); err != nil {
			return storage.SegmentRef{}, err
		}
	}
	if err := os.Rename(tmpDir, segmentDir); err != nil {
		return storage.SegmentRef{}, err
	}

	storageClass := storage.CloneStorageClassSnapshot(req.StorageClass)
	placementBackend := storageClass.Backend
	if placementBackend == "" {
		placementBackend = "local"
	}
	shardSizeBytes := uint64(shardSize)
	ref := storage.SegmentRef{
		SegmentID:    segmentID,
		StorageClass: storageClass,
		Placement: storage.PlacementSnapshot{
			Backend:           placementBackend,
			Layout:            ecLayout,
			RedundancyBackend: storageclass.RedundancyErasureCode,
			ProfileID:         storageClass.StorageClassID,
			ProfileGeneration: uintParam(storageClass.Parameters, storageclass.ParamProfileGeneration),
			ChunkSizeBytes:    shardSizeBytes,
			Parameters: map[string]string{
				storageclass.ParamDataShards:   strconv.Itoa(dataShards),
				storageclass.ParamParityShards: strconv.Itoa(parityShards),
				"shard_size_bytes":             strconv.FormatUint(shardSizeBytes, 10),
				"original_size_bytes":          strconv.FormatUint(req.SizeBytes, 10),
			},
			Chunks: ecPlacementChunks(dataShards, parityShards, req.SizeBytes, shardSizeBytes),
		},
		SizeBytes: req.SizeBytes,
		Digest: storage.Digest{
			Algorithm: "sha256",
			Hex:       hex.EncodeToString(hasher.Sum(nil)),
		},
		CreatedAt: s.now(),
	}
	return ref, nil
}

func (s *Store) getECSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if off > ref.SizeBytes {
		return nil, storage.ErrInvalidRange
	}
	dataShards, parityShards, err := ecShardConfigFromRef(ref)
	if err != nil {
		return nil, err
	}
	encoder, err := reedsolomon.New(dataShards, parityShards)
	if err != nil {
		return nil, fmt.Errorf("%w: create ec encoder: %v", storage.ErrInvalidArgument, err)
	}
	shards := make([][]byte, dataShards+parityShards)
	segmentDir := s.segmentPath(ref.SegmentID)
	if _, err := os.Stat(segmentDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	for shardID := range shards {
		shard, err := os.ReadFile(filepath.Join(segmentDir, ecShardFile(shardID)))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		shards[shardID] = shard
	}
	if err := encoder.Reconstruct(shards); err != nil {
		return nil, fmt.Errorf("%w: reconstruct ec segment: %v", storage.ErrUnavailable, err)
	}
	var payload bytes.Buffer
	if err := encoder.Join(&payload, shards, int(ref.SizeBytes)); err != nil {
		return nil, fmt.Errorf("%w: join ec segment: %v", storage.ErrUnavailable, err)
	}
	data := payload.Bytes()
	end := uint64(len(data))
	if length > 0 && off+length < end {
		end = off + length
	}
	return io.NopCloser(bytes.NewReader(data[off:end])), nil
}

func isECStorageClass(snapshot storage.StorageClassSnapshot) bool {
	return snapshot.Parameters[storageclass.ParamRedundancyBackend] == storageclass.RedundancyErasureCode
}

func ecShardConfig(snapshot storage.StorageClassSnapshot) (int, int, error) {
	dataShards := intParam(snapshot.Parameters, storageclass.ParamDataShards)
	parityShards := intParam(snapshot.Parameters, storageclass.ParamParityShards)
	if dataShards <= 0 || parityShards <= 0 {
		return 0, 0, fmt.Errorf("%w: ec data/parity shards are required", storage.ErrInvalidArgument)
	}
	return dataShards, parityShards, nil
}

func ecShardConfigFromRef(ref storage.SegmentRef) (int, int, error) {
	dataShards := intParam(ref.Placement.Parameters, storageclass.ParamDataShards)
	parityShards := intParam(ref.Placement.Parameters, storageclass.ParamParityShards)
	if dataShards <= 0 {
		dataShards = intParam(ref.StorageClass.Parameters, storageclass.ParamDataShards)
	}
	if parityShards <= 0 {
		parityShards = intParam(ref.StorageClass.Parameters, storageclass.ParamParityShards)
	}
	if dataShards <= 0 || parityShards <= 0 {
		return 0, 0, fmt.Errorf("%w: ec data/parity shards are required", storage.ErrInvalidArgument)
	}
	return dataShards, parityShards, nil
}

func ecPlacementChunks(dataShards, parityShards int, size, shardSize uint64) []storage.PlacementChunk {
	total := dataShards + parityShards
	chunks := make([]storage.PlacementChunk, 0, total)
	for shardID := 0; shardID < total; shardID++ {
		role := "data"
		if shardID >= dataShards {
			role = "parity"
		}
		logicalOffset := uint64(0)
		logicalSize := uint64(0)
		if role == "data" {
			logicalOffset = uint64(shardID) * shardSize
			if logicalOffset < size {
				logicalSize = minUint64(shardSize, size-logicalOffset)
			}
		}
		chunks = append(chunks, storage.PlacementChunk{
			LogicalOffsetBytes: logicalOffset,
			SizeBytes:          logicalSize,
			LengthBytes:        shardSize,
			ShardID:            uint32(shardID),
			Role:               role,
		})
	}
	return chunks
}

func ecShardFile(shardID int) string {
	return fmt.Sprintf("shard-%05d", shardID)
}

func intParam(params map[string]string, key string) int {
	value, _ := strconv.Atoi(params[key])
	return value
}

func uintParam(params map[string]string, key string) uint64 {
	value, _ := strconv.ParseUint(params[key], 10, 64)
	return value
}

func ensureNoExtraBytes(reader io.Reader) error {
	var extra [1]byte
	n, err := io.ReadFull(reader, extra[:])
	if n > 0 {
		return fmt.Errorf("%w: reader contains more bytes than declared size", storage.ErrInvalidArgument)
	}
	if err == nil {
		return fmt.Errorf("%w: reader contains more bytes than declared size", storage.ErrInvalidArgument)
	}
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func ceilDivInt(value, divisor int) int {
	if divisor <= 0 || value <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxIntValue() int {
	return int(^uint(0) >> 1)
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

type limitedReadCloser struct {
	io.Reader
	io.Closer
}

func newSegmentID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

var _ storage.SegmentStore = (*Store)(nil)
var _ storage.OrphanTracker = (*Store)(nil)

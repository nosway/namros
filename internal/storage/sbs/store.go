package sbs

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sbsservice "github.com/nosway/namrbd/gateway/service"
	sbslocal "github.com/nosway/namrbd/sbs/local"

	"github.com/nosway/namros/internal/storage"
)

const (
	backendName                       = "sbs-local"
	defaultVolumeID            uint64 = 0x0a0b0001
	defaultVolumeSize                 = 1 << 40
	defaultBlockSize                  = uint64(sbsservice.DefaultBlockSize)
	defaultGatewayID                  = "namros-gateway"
	defaultGeneration                 = uint64(1)
	legacySegmentReadChunkSize        = uint64(64 * 1024)
)

type Config struct {
	Path            string
	StatePath       string
	Stores          []sbslocal.StoreSpec
	VolumeID        uint64
	VolumeName      string
	VolumeSizeBytes uint64
	GatewayID       string
	AttachmentID    string
	Generation      uint64
	SessionIdentity SessionIdentity
	SessionFence    SessionFenceSnapshot
	BlockSizeBytes  uint64
	DeleteAdmission storage.DeleteAdmissionFunc
}

type Store struct {
	client          *sbslocal.Client
	statePath       string
	volumeID        string
	volumeIDRaw     uint64
	volumeHandle    string
	gatewayID       string
	attachmentID    string
	generation      uint64
	sessionIdentity SessionIdentity
	sessionFence    SessionFenceSnapshot
	blockSize       uint64
	volumeSize      uint64
	now             func() time.Time
	deleteAdmission storage.DeleteAdmissionFunc

	mu    sync.Mutex
	state adapterState
}

type adapterState struct {
	NextOffset uint64                         `json:"next_offset"`
	Sequence   uint64                         `json:"sequence"`
	Deleted    map[string]struct{}            `json:"deleted,omitempty"`
	GCQueue    map[string]storage.GCCandidate `json:"gc_queue,omitempty"`
}

type segmentAddress struct {
	VolumeID     string
	OffsetBytes  uint64
	PaddedLength uint64
	Sequence     uint64
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	if strings.TrimSpace(cfg.Path) == "" {
		return nil, fmt.Errorf("%w: sbs path is required", storage.ErrInvalidArgument)
	}
	if cfg.VolumeID == 0 {
		cfg.VolumeID = defaultVolumeID
	}
	if cfg.VolumeName == "" {
		cfg.VolumeName = "namros-object-segments"
	}
	if cfg.VolumeSizeBytes == 0 {
		cfg.VolumeSizeBytes = defaultVolumeSize
	}
	if cfg.GatewayID == "" {
		cfg.GatewayID = defaultGatewayID
	}
	if cfg.AttachmentID == "" {
		cfg.AttachmentID = "att-" + sbsservice.CanonicalVolumeID(cfg.VolumeID) + "-0001"
	}
	if cfg.Generation == 0 {
		cfg.Generation = defaultGeneration
	}
	volumeID := sbsservice.CanonicalVolumeID(cfg.VolumeID)
	sessionIdentity, err := NormalizeSessionIdentity(cfg.SessionIdentity, SessionIdentityDefaults{
		VolumeID:          volumeID,
		MemberGeneration:  cfg.Generation,
		GatewayID:         cfg.GatewayID,
		SessionTTL:        DefaultSessionTTL,
		HeartbeatInterval: DefaultSessionHeartbeat,
	})
	if err != nil {
		return nil, err
	}
	if cfg.BlockSizeBytes == 0 {
		cfg.BlockSizeBytes = defaultBlockSize
	}
	if cfg.StatePath == "" {
		cfg.StatePath = cfg.Path + ".namros-segments.json"
	}
	client, err := sbslocal.Open(sbslocal.Config{
		Path:   cfg.Path,
		Stores: cfg.Stores,
	})
	if err != nil {
		return nil, err
	}
	spec := sbsservice.NormalizeVolumeSpec(sbsservice.VolumeSpec{
		ID:              sbsservice.HexVolumeID(cfg.VolumeID),
		Name:            cfg.VolumeName,
		Prefix:          sbsservice.BuildVolumePrefix(cfg.VolumeName, cfg.VolumeID),
		SizeBytes:       cfg.VolumeSizeBytes,
		BlockSize:       uint32(cfg.BlockSizeBytes),
		ChunkSizeBytes:  sbsservice.DefaultAllocationChunkSize,
		ExtentPageBytes: sbsservice.DefaultAllocationPageSize,
	})
	if _, err := client.CreateVolume(ctx, spec); err != nil {
		_ = client.Close()
		return nil, err
	}
	openResp, err := client.OpenVolume(ctx, &sbsservice.OpenVolumeRequest{
		VolumeID:   volumeID,
		AccessMode: sbsservice.SBSAccessModeExclusiveWriter,
		Context: sbsservice.SBSRequestContext{
			RequestID:    newRequestID("open"),
			GatewayID:    cfg.GatewayID,
			AttachmentID: cfg.AttachmentID,
			Generation:   cfg.Generation,
		},
	})
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return &Store{
		client:          client,
		statePath:       cfg.StatePath,
		volumeID:        volumeID,
		volumeIDRaw:     cfg.VolumeID,
		volumeHandle:    openResp.VolumeHandle,
		gatewayID:       cfg.GatewayID,
		attachmentID:    cfg.AttachmentID,
		generation:      cfg.Generation,
		sessionIdentity: sessionIdentity,
		sessionFence:    normalizeSessionFence(cfg.SessionFence),
		blockSize:       cfg.BlockSizeBytes,
		volumeSize:      cfg.VolumeSizeBytes,
		now:             func() time.Time { return time.Now().UTC() },
		deleteAdmission: cfg.DeleteAdmission,
		state:           state,
	}, nil
}

func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	client := s.client
	s.client = nil
	volumeID := s.volumeID
	volumeHandle := s.volumeHandle
	closeContext := s.requestContext("close", 0)
	s.mu.Unlock()
	if client == nil {
		return nil
	}
	_, closeVolumeErr := client.CloseVolume(context.Background(), &sbsservice.CloseVolumeRequest{
		VolumeID:     volumeID,
		VolumeHandle: volumeHandle,
		Context:      closeContext,
	})
	if closeVolumeErr != nil {
		closeVolumeErr = mapSBSError(closeVolumeErr)
	}
	return errors.Join(closeVolumeErr, client.Close())
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
	if err := validateSessionFence(s.sessionIdentity, s.sessionFence, s.now); err != nil {
		return storage.SegmentRef{}, err
	}
	size := req.SizeBytes
	paddedLength := alignUp(size, s.blockSize)
	hasher := sha256.New()
	var offset, sequence uint64
	s.mu.Lock()
	offset = alignUp(s.state.NextOffset, s.blockSize)
	sequence = s.state.Sequence + 1
	if paddedLength > 0 && (offset+paddedLength < offset || offset+paddedLength > s.volumeSize) {
		s.mu.Unlock()
		return storage.SegmentRef{}, fmt.Errorf("%w: sbs segment volume is full", storage.ErrInvalidArgument)
	}
	s.state.Sequence = sequence
	s.state.NextOffset = offset + paddedLength
	if err := saveState(s.statePath, s.state); err != nil {
		s.mu.Unlock()
		return storage.SegmentRef{}, err
	}
	s.mu.Unlock()

	storageClass := storage.CloneStorageClassSnapshot(req.StorageClass)
	storageClass.Backend = backendName
	if storageClass.Parameters == nil {
		storageClass.Parameters = make(map[string]string)
	}
	storageClass.Parameters["volume_id"] = s.volumeID
	storageClass.Parameters["offset_bytes"] = strconv.FormatUint(offset, 10)
	storageClass.Parameters["padded_length_bytes"] = strconv.FormatUint(paddedLength, 10)
	addr := segmentAddress{
		VolumeID:     s.volumeID,
		OffsetBytes:  offset,
		PaddedLength: paddedLength,
		Sequence:     sequence,
	}
	if err := streamLegacySegmentWrites(ctx, req.Reader, size, s.blockSize, hasher, func(relativeOffset uint64, data []byte) error {
		writeContext := s.requestContext("write-"+strconv.FormatUint(relativeOffset, 10), sequence)
		if _, err := s.client.Write(ctx, &sbsservice.WriteRequest{
			VolumeID:     s.volumeID,
			VolumeHandle: s.volumeHandle,
			OffsetBytes:  offset + relativeOffset,
			LengthBytes:  uint64(len(data)),
			Data:         data,
			Context:      writeContext,
		}); err != nil {
			return mapSBSError(err)
		}
		return nil
	}); err != nil {
		digestHex := hex.EncodeToString(hasher.Sum(nil))
		ref := s.segmentRef(storageClass, addr, size, digestHex)
		if recordErr := s.recordLegacyOrphan(ref, storage.DeleteReasonPublishFailed); recordErr != nil {
			return storage.SegmentRef{}, errors.Join(err, recordErr)
		}
		return storage.SegmentRef{}, err
	}
	ref := s.segmentRef(storageClass, addr, size, hex.EncodeToString(hasher.Sum(nil)))
	return ref, nil
}

func (s *Store) segmentRef(storageClass storage.StorageClassSnapshot, addr segmentAddress, size uint64, digestHex string) storage.SegmentRef {
	return storage.SegmentRef{
		SegmentID:    encodeSegmentID(addr),
		StorageClass: storageClass,
		Placement: storage.PlacementSnapshot{
			Backend:           backendName,
			Layout:            "sbs-volume-offset",
			RedundancyBackend: "replicated",
			ProfileID:         storageClass.StorageClassID,
			ChunkSizeBytes:    s.blockSize,
			Parameters: map[string]string{
				"volume_id":           s.volumeID,
				"offset_bytes":        strconv.FormatUint(addr.OffsetBytes, 10),
				"padded_length_bytes": strconv.FormatUint(addr.PaddedLength, 10),
			},
			Chunks: []storage.PlacementChunk{{
				LogicalOffsetBytes: 0,
				SizeBytes:          size,
				VolumeID:           s.volumeID,
				OffsetBytes:        addr.OffsetBytes,
				LengthBytes:        addr.PaddedLength,
				Role:               "primary",
			}},
		},
		SizeBytes: size,
		Digest: storage.Digest{
			Algorithm: "sha256",
			Hex:       digestHex,
		},
		CreatedAt: s.now(),
	}
}

func streamLegacySegmentWrites(ctx context.Context, reader io.Reader, size, blockSize uint64, digest io.Writer, write func(relativeOffset uint64, data []byte) error) error {
	if blockSize == 0 {
		return fmt.Errorf("%w: sbs block size is required", storage.ErrInvalidArgument)
	}
	if digest == nil {
		return fmt.Errorf("%w: digest writer is required", storage.ErrInvalidArgument)
	}
	if write == nil {
		return fmt.Errorf("%w: write callback is required", storage.ErrInvalidArgument)
	}
	chunkSize := legacySegmentWritePayloadChunkSize(blockSize)
	readPayloadBytes := uint64(0)
	writtenPaddedBytes := uint64(0)
	for readPayloadBytes < size {
		if err := ctx.Err(); err != nil {
			return err
		}
		readLength := minUint64(chunkSize, size-readPayloadBytes)
		writeLength := alignUp(readLength, blockSize)
		if writeLength < readLength {
			return fmt.Errorf("%w: sbs segment write length overflow", storage.ErrInvalidArgument)
		}
		data := make([]byte, writeLength)
		n, err := io.ReadFull(reader, data[:readLength])
		if err != nil {
			return fmt.Errorf("%w: read %d bytes, expected %d", storage.ErrInvalidArgument, readPayloadBytes+uint64(n), size)
		}
		if _, err := digest.Write(data[:readLength]); err != nil {
			return err
		}
		if err := write(writtenPaddedBytes, data); err != nil {
			return err
		}
		readPayloadBytes += uint64(n)
		writtenPaddedBytes += writeLength
	}
	return ensureEmptyReader(reader)
}

func legacySegmentWritePayloadChunkSize(blockSize uint64) uint64 {
	if blockSize == 0 {
		return legacySegmentReadChunkSize
	}
	if blockSize >= legacySegmentReadChunkSize {
		return blockSize
	}
	chunkSize := legacySegmentReadChunkSize - legacySegmentReadChunkSize%blockSize
	if chunkSize == 0 {
		return blockSize
	}
	return chunkSize
}

func (s *Store) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addr, err := decodeSegmentID(ref.SegmentID)
	if err != nil {
		return nil, err
	}
	if addr.VolumeID != s.volumeID {
		return nil, storage.ErrNotFound
	}
	s.mu.Lock()
	_, deleted := s.state.Deleted[ref.SegmentID]
	s.mu.Unlock()
	if deleted {
		return nil, storage.ErrNotFound
	}
	if off > ref.SizeBytes {
		return nil, storage.ErrInvalidRange
	}
	readLength := length
	if readLength == 0 {
		readLength = ref.SizeBytes - off
	}
	if off+readLength < off || off+readLength > ref.SizeBytes {
		return nil, storage.ErrInvalidRange
	}
	if readLength == 0 {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return &legacySegmentReader{
		ctx:        ctx,
		nextOffset: off,
		remaining:  readLength,
		readChunk: func(ctx context.Context, relativeOffset, length uint64) ([]byte, error) {
			resp, err := s.client.Read(ctx, &sbsservice.ReadRequest{
				VolumeID:     s.volumeID,
				VolumeHandle: s.volumeHandle,
				OffsetBytes:  addr.OffsetBytes + relativeOffset,
				LengthBytes:  length,
				Context:      s.requestContext("read-"+strconv.FormatUint(relativeOffset, 10), addr.Sequence),
			})
			if err != nil {
				return nil, mapSBSError(err)
			}
			if resp == nil {
				return nil, fmt.Errorf("%w: nil legacy sbs read response", storage.ErrUnavailable)
			}
			if uint64(len(resp.Data)) < length {
				return nil, fmt.Errorf("%w: short legacy sbs read", storage.ErrUnavailable)
			}
			return resp.Data[:length], nil
		},
	}, nil
}

type legacySegmentReader struct {
	ctx        context.Context
	nextOffset uint64
	remaining  uint64
	readChunk  func(context.Context, uint64, uint64) ([]byte, error)
	buf        []byte
	closed     bool
}

func (r *legacySegmentReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(r.buf) == 0 {
		if r.remaining == 0 {
			return 0, io.EOF
		}
		data, err := r.readNextChunk()
		if err != nil {
			return 0, err
		}
		r.buf = data
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	return n, nil
}

func (r *legacySegmentReader) Close() error {
	r.closed = true
	r.buf = nil
	return nil
}

func (r *legacySegmentReader) readNextChunk() ([]byte, error) {
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	readLength := minUint64(legacySegmentReadChunkSize, r.remaining)
	data, err := r.readChunk(r.ctx, r.nextOffset, readLength)
	if err != nil {
		return nil, err
	}
	r.nextOffset += readLength
	r.remaining -= readLength
	return data, nil
}

func (s *Store) DeleteSegment(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr, err := decodeSegmentID(ref.SegmentID)
	if err != nil {
		return err
	}
	if addr.VolumeID != s.volumeID {
		return storage.ErrNotFound
	}
	s.mu.Lock()
	if _, deleted := s.state.Deleted[ref.SegmentID]; deleted {
		s.mu.Unlock()
		return storage.ErrNotFound
	}
	s.mu.Unlock()
	if err := storage.CheckDeleteAdmission(ctx, s.deleteAdmission, ref, reason); err != nil {
		return err
	}
	s.mu.Lock()
	if _, deleted := s.state.Deleted[ref.SegmentID]; deleted {
		s.mu.Unlock()
		return storage.ErrNotFound
	}
	if s.state.Deleted == nil {
		s.state.Deleted = make(map[string]struct{})
	}
	s.state.Deleted[ref.SegmentID] = struct{}{}
	delete(s.state.GCQueue, ref.SegmentID)
	if err := saveState(s.statePath, s.state); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if addr.PaddedLength == 0 {
		return nil
	}
	_, err = s.client.Discard(ctx, &sbsservice.DiscardRequest{
		VolumeID:    s.volumeID,
		OffsetBytes: addr.OffsetBytes,
		LengthBytes: addr.PaddedLength,
		Context:     s.requestContext("discard-"+string(reason), addr.Sequence),
	})
	if err != nil {
		return mapSBSError(err)
	}
	return nil
}

func (s *Store) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr, err := decodeSegmentID(ref.SegmentID)
	if err != nil {
		return err
	}
	if addr.VolumeID != s.volumeID {
		return storage.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, deleted := s.state.Deleted[ref.SegmentID]; deleted {
		return storage.ErrNotFound
	}
	if s.state.GCQueue == nil {
		s.state.GCQueue = make(map[string]storage.GCCandidate)
	}
	s.state.GCQueue[ref.SegmentID] = storage.GCCandidate{
		Ref:       ref,
		Reason:    reason,
		CreatedAt: s.now(),
	}
	return saveState(s.statePath, s.state)
}

func (s *Store) recordLegacyOrphan(ref storage.SegmentRef, reason storage.DeleteReason) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, deleted := s.state.Deleted[ref.SegmentID]; deleted {
		return nil
	}
	if s.state.GCQueue == nil {
		s.state.GCQueue = make(map[string]storage.GCCandidate)
	}
	s.state.GCQueue[ref.SegmentID] = storage.GCCandidate{
		Ref:       storage.CloneSegmentRef(ref),
		Reason:    reason,
		CreatedAt: s.now(),
	}
	return saveState(s.statePath, s.state)
}

func (s *Store) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidates := make([]storage.GCCandidate, 0, len(s.state.GCQueue))
	for _, candidate := range s.state.GCQueue {
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

func (s *Store) requestContext(op string, sequence uint64) sbsservice.SBSRequestContext {
	idempotencyKey := fmt.Sprintf("namros-%s-%s-%s-%d-%d", op, s.volumeID, s.attachmentID, s.generation, sequence)
	return sbsservice.SBSRequestContext{
		RequestID:      newRequestID(op),
		GatewayID:      s.gatewayID,
		AttachmentID:   s.attachmentID,
		Generation:     s.generation,
		IdempotencyKey: s.sessionIdentity.ScopedIdempotencyKey(idempotencyKey),
	}
}

func loadState(path string) (adapterState, error) {
	state := adapterState{
		Deleted: make(map[string]struct{}),
		GCQueue: make(map[string]storage.GCCandidate),
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return adapterState{}, err
	}
	if len(payload) == 0 {
		return state, nil
	}
	if err := json.Unmarshal(payload, &state); err != nil {
		return adapterState{}, err
	}
	if state.Deleted == nil {
		state.Deleted = make(map[string]struct{})
	}
	if state.GCQueue == nil {
		state.GCQueue = make(map[string]storage.GCCandidate)
	}
	return state, nil
}

func saveState(path string, state adapterState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}
	if _, err := tmp.Write(payload); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	return os.Rename(tmpPath, path)
}

func encodeSegmentID(addr segmentAddress) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d", backendName, addr.VolumeID, addr.OffsetBytes, addr.PaddedLength, addr.Sequence)
}

func decodeSegmentID(value string) (segmentAddress, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 || parts[0] != backendName {
		return segmentAddress{}, fmt.Errorf("%w: invalid sbs segment id", storage.ErrInvalidArgument)
	}
	offset, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return segmentAddress{}, fmt.Errorf("%w: invalid sbs segment offset", storage.ErrInvalidArgument)
	}
	padded, err := strconv.ParseUint(parts[3], 10, 64)
	if err != nil {
		return segmentAddress{}, fmt.Errorf("%w: invalid sbs segment length", storage.ErrInvalidArgument)
	}
	sequence, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil {
		return segmentAddress{}, fmt.Errorf("%w: invalid sbs segment sequence", storage.ErrInvalidArgument)
	}
	return segmentAddress{
		VolumeID:     parts[1],
		OffsetBytes:  offset,
		PaddedLength: padded,
		Sequence:     sequence,
	}, nil
}

func alignUp(value, alignment uint64) uint64 {
	if alignment == 0 || value == 0 {
		return value
	}
	remainder := value % alignment
	if remainder == 0 {
		return value
	}
	return value + alignment - remainder
}

func mapSBSError(err error) error {
	var sbsErr *sbsservice.SBSError
	if errors.As(err, &sbsErr) {
		switch sbsErr.Code {
		case sbsservice.SBSErrorCodeNotFound:
			return fmt.Errorf("%w: %s", storage.ErrNotFound, sbsErr.Error())
		case sbsservice.SBSErrorCodeBadRequest:
			return fmt.Errorf("%w: %s", storage.ErrInvalidArgument, sbsErr.Error())
		case sbsservice.SBSErrorCodeUnavailable, sbsservice.SBSErrorCodeTimeout:
			return fmt.Errorf("%w: %s", storage.ErrUnavailable, sbsErr.Error())
		default:
			return err
		}
	}
	if errors.Is(err, sbsservice.ErrOutOfRange) || errors.Is(err, sbsservice.ErrBadDataLength) {
		return fmt.Errorf("%w: %s", storage.ErrInvalidArgument, err.Error())
	}
	return err
}

func newRequestID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}

var _ storage.SegmentStore = (*Store)(nil)
var _ storage.OrphanTracker = (*Store)(nil)

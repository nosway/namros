package sbs

import (
	"bytes"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sbsservice "github.com/nosway/namrbd/gateway/service"

	"github.com/nosway/namros/internal/storage"
)

const physicalBackendName = "sbs-physical"

type PhysicalChunkAllocator interface {
	AllocateChunkIDs(ctx context.Context, volumeID string, count uint32) (uint64, error)
}

type PhysicalChunkClient interface {
	ReadPhysicalChunk(ctx context.Context, req *sbsservice.ReadPhysicalChunkRequest) (*sbsservice.ReadPhysicalChunkResponse, error)
	WritePhysicalChunk(ctx context.Context, req *sbsservice.WritePhysicalChunkRequest) (*sbsservice.WritePhysicalChunkResponse, error)
}

type PhysicalChunkDeleter interface {
	DeletePhysicalChunk(ctx context.Context, volumeID string, chunkID uint64) error
}

type PhysicalMetrics interface {
	ObserveSBSPhysicalAllocation(duration time.Duration, chunkCount uint32, err error)
	ObserveSBSPhysicalChunk(operation string, duration time.Duration, bytes uint64, err error)
	ObserveSBSPhysicalReadback(duration time.Duration, bytes uint64, err error)
}

type PhysicalRepairReason string

const (
	PhysicalRepairReasonWriteReadbackFailed   PhysicalRepairReason = "write_readback_failed"
	PhysicalRepairReasonWriteReadbackMismatch PhysicalRepairReason = "write_readback_mismatch"
	PhysicalRepairReasonReadFailed            PhysicalRepairReason = "read_failed"
)

type PhysicalRepairCandidate struct {
	Ref       storage.SegmentRef
	Reason    PhysicalRepairReason
	Detail    string
	CreatedAt time.Time
}

type PhysicalConfig struct {
	VolumeID               string
	VolumeIDRaw            uint64
	ChunkSizeBytes         uint64
	GatewayID              string
	AttachmentID           string
	Generation             uint64
	SessionIdentity        SessionIdentity
	SessionFence           SessionFenceSnapshot
	VolumeHandle           string
	VerifyReadback         bool
	WriteConcurrency       int
	FullChunkWriteMinBytes uint64
	FullChunkWriteMaxBytes uint64
	ChunkCacheBytes        uint64
	Allocator              PhysicalChunkAllocator
	Client                 PhysicalChunkClient
	Metrics                PhysicalMetrics
	DeleteAdmission        storage.DeleteAdmissionFunc
	Now                    func() time.Time
}

type PhysicalStore struct {
	volumeID               string
	chunkSize              uint64
	gatewayID              string
	attachmentID           string
	generation             uint64
	sessionIdentity        SessionIdentity
	sessionFence           SessionFenceSnapshot
	volumeHandle           string
	verifyReadback         bool
	writeConcurrency       int
	fullChunkWriteMinBytes uint64
	fullChunkWriteMaxBytes uint64
	chunkCacheMaxBytes     uint64
	allocator              PhysicalChunkAllocator
	client                 PhysicalChunkClient
	metrics                PhysicalMetrics
	deleteAdmission        storage.DeleteAdmissionFunc
	now                    func() time.Time

	mu          sync.Mutex
	deleted     map[string]struct{}
	gcQueue     map[string]storage.GCCandidate
	repairQueue map[string]PhysicalRepairCandidate

	cacheMu         sync.Mutex
	chunkCache      map[uint64]*list.Element
	chunkCacheLRU   *list.List
	chunkCacheBytes uint64
}

type physicalSegmentAddress struct {
	VolumeID     string
	StartChunkID uint64
	ChunkCount   uint32
	SizeBytes    uint64
}

type physicalChunkCacheEntry struct {
	chunkID uint64
	data    []byte
	size    uint64
}

func NewPhysicalStore(cfg PhysicalConfig) (*PhysicalStore, error) {
	volumeID := strings.TrimSpace(cfg.VolumeID)
	if volumeID == "" {
		raw := cfg.VolumeIDRaw
		if raw == 0 {
			raw = defaultVolumeID
		}
		volumeID = sbsservice.CanonicalVolumeID(raw)
	}
	if cfg.ChunkSizeBytes == 0 {
		cfg.ChunkSizeBytes = sbsservice.DefaultAllocationChunkSize
	}
	if cfg.GatewayID == "" {
		cfg.GatewayID = defaultGatewayID
	}
	if cfg.AttachmentID == "" {
		cfg.AttachmentID = "att-" + volumeID + "-physical"
	}
	if cfg.Generation == 0 {
		cfg.Generation = defaultGeneration
	}
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
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return time.Now().UTC() }
	}
	if cfg.WriteConcurrency <= 0 {
		cfg.WriteConcurrency = 1
	}
	if cfg.FullChunkWriteMaxBytes > 0 && cfg.FullChunkWriteMinBytes > cfg.FullChunkWriteMaxBytes {
		return nil, fmt.Errorf("%w: full chunk write min bytes cannot exceed max bytes", storage.ErrInvalidArgument)
	}
	if cfg.Allocator == nil {
		return nil, fmt.Errorf("%w: physical chunk allocator is required", storage.ErrInvalidArgument)
	}
	if cfg.Client == nil {
		return nil, fmt.Errorf("%w: physical chunk client is required", storage.ErrInvalidArgument)
	}
	store := &PhysicalStore{
		volumeID:               volumeID,
		chunkSize:              cfg.ChunkSizeBytes,
		gatewayID:              cfg.GatewayID,
		attachmentID:           cfg.AttachmentID,
		generation:             cfg.Generation,
		sessionIdentity:        sessionIdentity,
		sessionFence:           normalizeSessionFence(cfg.SessionFence),
		volumeHandle:           strings.TrimSpace(cfg.VolumeHandle),
		verifyReadback:         cfg.VerifyReadback,
		writeConcurrency:       cfg.WriteConcurrency,
		fullChunkWriteMinBytes: cfg.FullChunkWriteMinBytes,
		fullChunkWriteMaxBytes: cfg.FullChunkWriteMaxBytes,
		chunkCacheMaxBytes:     cfg.ChunkCacheBytes,
		allocator:              cfg.Allocator,
		client:                 cfg.Client,
		metrics:                cfg.Metrics,
		deleteAdmission:        cfg.DeleteAdmission,
		now:                    cfg.Now,
		deleted:                make(map[string]struct{}),
		gcQueue:                make(map[string]storage.GCCandidate),
		repairQueue:            make(map[string]PhysicalRepairCandidate),
	}
	if cfg.ChunkCacheBytes > 0 {
		store.chunkCache = make(map[uint64]*list.Element)
		store.chunkCacheLRU = list.New()
	}
	return store, nil
}

func (s *PhysicalStore) PutSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
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
	if size == 0 {
		if err := ensureEmptyReader(req.Reader); err != nil {
			return storage.SegmentRef{}, err
		}
		storageClass := storage.CloneStorageClassSnapshot(req.StorageClass)
		storageClass.Backend = physicalBackendName
		addr := physicalSegmentAddress{
			VolumeID:  s.volumeID,
			SizeBytes: 0,
		}
		emptyDigest := sha256.Sum256(nil)
		return s.segmentRef(storageClass, addr, hex.EncodeToString(emptyDigest[:])), nil
	}
	chunkCount := physicalChunkCount(size, s.chunkSize)
	if chunkCount == 0 {
		return storage.SegmentRef{}, fmt.Errorf("%w: physical segment exceeds maximum chunk count", storage.ErrInvalidArgument)
	}
	storageClass := storage.CloneStorageClassSnapshot(req.StorageClass)
	storageClass.Backend = physicalBackendName
	addr := physicalSegmentAddress{
		VolumeID:   s.volumeID,
		ChunkCount: chunkCount,
		SizeBytes:  size,
	}
	allocationStart := time.Now()
	allocated, err := s.allocator.AllocateChunkIDs(ctx, s.volumeID, chunkCount)
	if s.metrics != nil {
		s.metrics.ObserveSBSPhysicalAllocation(time.Since(allocationStart), chunkCount, err)
	}
	if err != nil {
		return storage.SegmentRef{}, mapSBSError(err)
	}
	startChunkID := allocated
	addr.StartChunkID = startChunkID
	hasher := sha256.New()
	writtenPayloadBytes := uint64(0)
	digestHex := ""
	var writeWG sync.WaitGroup
	var writeErrMu sync.Mutex
	var writeErr error
	writeConcurrency := s.writeConcurrency
	if writeConcurrency > int(chunkCount) {
		writeConcurrency = int(chunkCount)
	}
	writeSlots := make(chan struct{}, writeConcurrency)
	recordWriteErr := func(err error) {
		if err == nil {
			return
		}
		writeErrMu.Lock()
		defer writeErrMu.Unlock()
		if writeErr == nil {
			writeErr = err
		}
	}
	waitForWrites := func() error {
		writeWG.Wait()
		writeErrMu.Lock()
		defer writeErrMu.Unlock()
		return writeErr
	}
	for i := uint32(0); i < chunkCount; i++ {
		offset := uint64(i) * s.chunkSize
		length := minUint64(s.chunkSize, size-offset)
		if length == 0 {
			continue
		}
		chunkPayload := make([]byte, length)
		n, err := io.ReadFull(req.Reader, chunkPayload)
		if err != nil {
			_ = waitForWrites()
			if writtenPayloadBytes > 0 {
				digestHex = hex.EncodeToString(hasher.Sum(nil))
				s.recordPhysicalOrphan(s.segmentRef(storageClass, addr, digestHex), storage.DeleteReasonPublishFailed)
			}
			return storage.SegmentRef{}, fmt.Errorf("%w: read %d bytes, expected %d", storage.ErrInvalidArgument, writtenPayloadBytes+uint64(n), size)
		}
		writtenPayloadBytes += uint64(n)
		if _, err := hasher.Write(chunkPayload); err != nil {
			_ = waitForWrites()
			return storage.SegmentRef{}, err
		}
		digestHex = hex.EncodeToString(hasher.Sum(nil))
		writePayload := chunkPayload
		writeLength := length
		if s.shouldFullChunkWrite(length) {
			padded := make([]byte, s.chunkSize)
			copy(padded, chunkPayload)
			writePayload = padded
			writeLength = s.chunkSize
		}
		writeSlots <- struct{}{}
		writeWG.Add(1)
		go func(logicalChunk uint32, payload []byte, payloadLength uint64) {
			defer func() {
				<-writeSlots
				writeWG.Done()
			}()
			chunkID := startChunkID + uint64(logicalChunk)
			writeStart := time.Now()
			_, err := s.client.WritePhysicalChunk(ctx, &sbsservice.WritePhysicalChunkRequest{
				VolumeID:         s.volumeID,
				VolumeHandle:     s.volumeHandle,
				PhysicalChunkID:  chunkID,
				ChunkOffsetBytes: 0,
				LengthBytes:      payloadLength,
				Data:             payload,
				Context:          s.requestContext("write-physical", chunkID),
			})
			if s.metrics != nil {
				s.metrics.ObserveSBSPhysicalChunk("write", time.Since(writeStart), payloadLength, err)
			}
			if err != nil {
				recordWriteErr(mapSBSError(err))
				return
			}
			if payloadLength == s.chunkSize && uint64(len(payload)) >= payloadLength {
				s.cacheChunk(chunkID, payload[:payloadLength])
			}
		}(i, writePayload, writeLength)
	}
	if err := waitForWrites(); err != nil {
		s.recordPhysicalOrphan(s.segmentRef(storageClass, addr, digestHex), storage.DeleteReasonPublishFailed)
		return storage.SegmentRef{}, err
	}
	if err := ensureEmptyReader(req.Reader); err != nil {
		digestHex = hex.EncodeToString(hasher.Sum(nil))
		s.recordPhysicalOrphan(s.segmentRef(storageClass, addr, digestHex), storage.DeleteReasonPublishFailed)
		return storage.SegmentRef{}, err
	}
	ref := s.segmentRef(storageClass, addr, digestHex)
	if s.verifyReadback {
		readbackStart := time.Now()
		reason, err := s.verifySegmentReadback(ctx, ref, size, digestHex)
		if s.metrics != nil {
			s.metrics.ObserveSBSPhysicalReadback(time.Since(readbackStart), size, err)
		}
		if err != nil {
			s.recordPhysicalOrphan(ref, storage.DeleteReasonPublishFailed)
			s.recordPhysicalRepair(ref, reason, err.Error())
			return storage.SegmentRef{}, err
		}
	}
	return ref, nil
}

func (s *PhysicalStore) segmentRef(storageClass storage.StorageClassSnapshot, addr physicalSegmentAddress, digestHex string) storage.SegmentRef {
	return storage.SegmentRef{
		SegmentID:    encodePhysicalSegmentID(addr),
		StorageClass: storageClass,
		Placement: storage.PlacementSnapshot{
			Backend:           physicalBackendName,
			Layout:            "sbs-physical-chunks",
			RedundancyBackend: "replicated",
			ProfileID:         storageClass.StorageClassID,
			ChunkSizeBytes:    s.chunkSize,
			Parameters: map[string]string{
				"volume_id":      s.volumeID,
				"start_chunk_id": strconv.FormatUint(addr.StartChunkID, 10),
				"chunk_count":    strconv.FormatUint(uint64(addr.ChunkCount), 10),
			},
			Chunks: physicalPlacementChunks(s.volumeID, addr.StartChunkID, addr.ChunkCount, addr.SizeBytes, s.chunkSize),
		},
		SizeBytes: addr.SizeBytes,
		Digest: storage.Digest{
			Algorithm: "sha256",
			Hex:       digestHex,
		},
		CreatedAt: s.now(),
	}
}

func (s *PhysicalStore) recordPhysicalOrphan(ref storage.SegmentRef, reason storage.DeleteReason) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, deleted := s.deleted[ref.SegmentID]; deleted {
		return
	}
	s.gcQueue[ref.SegmentID] = storage.GCCandidate{
		Ref:       storage.CloneSegmentRef(ref),
		Reason:    reason,
		CreatedAt: s.now(),
	}
}

func (s *PhysicalStore) recordPhysicalRepair(ref storage.SegmentRef, reason PhysicalRepairReason, detail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, deleted := s.deleted[ref.SegmentID]; deleted {
		return
	}
	s.repairQueue[ref.SegmentID] = PhysicalRepairCandidate{
		Ref:       storage.CloneSegmentRef(ref),
		Reason:    reason,
		Detail:    detail,
		CreatedAt: s.now(),
	}
}

func ensureEmptyReader(reader io.Reader) error {
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

func (s *PhysicalStore) shouldFullChunkWrite(length uint64) bool {
	if length == 0 || length >= s.chunkSize {
		return false
	}
	if s.fullChunkWriteMaxBytes == 0 {
		return false
	}
	if s.fullChunkWriteMinBytes > 0 && s.chunkSize < s.fullChunkWriteMinBytes {
		return false
	}
	return s.chunkSize <= s.fullChunkWriteMaxBytes
}

func (s *PhysicalStore) cacheChunk(chunkID uint64, data []byte) {
	if s.chunkCacheMaxBytes == 0 || len(data) == 0 || uint64(len(data)) > s.chunkCacheMaxBytes {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.chunkCache == nil || s.chunkCacheLRU == nil {
		return
	}
	if elem, ok := s.chunkCache[chunkID]; ok {
		s.removeCachedChunkElement(elem)
	}
	copied := append([]byte(nil), data...)
	entry := &physicalChunkCacheEntry{
		chunkID: chunkID,
		data:    copied,
		size:    uint64(len(copied)),
	}
	elem := s.chunkCacheLRU.PushFront(entry)
	s.chunkCache[chunkID] = elem
	s.chunkCacheBytes += entry.size
	for s.chunkCacheBytes > s.chunkCacheMaxBytes {
		tail := s.chunkCacheLRU.Back()
		if tail == nil {
			break
		}
		s.removeCachedChunkElement(tail)
	}
}

func (s *PhysicalStore) readCachedChunk(chunkID, off, length uint64) ([]byte, bool) {
	if s.chunkCacheMaxBytes == 0 {
		return nil, false
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if s.chunkCache == nil || s.chunkCacheLRU == nil {
		return nil, false
	}
	elem, ok := s.chunkCache[chunkID]
	if !ok {
		return nil, false
	}
	entry, ok := elem.Value.(*physicalChunkCacheEntry)
	if !ok {
		s.chunkCacheLRU.Remove(elem)
		delete(s.chunkCache, chunkID)
		return nil, false
	}
	end := off + length
	if end < off || off > uint64(len(entry.data)) || end > uint64(len(entry.data)) {
		return nil, false
	}
	s.chunkCacheLRU.MoveToFront(elem)
	return append([]byte(nil), entry.data[off:end]...), true
}

func (s *PhysicalStore) evictCachedChunk(chunkID uint64) {
	if s.chunkCacheMaxBytes == 0 {
		return
	}
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	if elem, ok := s.chunkCache[chunkID]; ok {
		s.removeCachedChunkElement(elem)
	}
}

func (s *PhysicalStore) removeCachedChunkElement(elem *list.Element) {
	entry, ok := elem.Value.(*physicalChunkCacheEntry)
	if ok {
		delete(s.chunkCache, entry.chunkID)
		if s.chunkCacheBytes >= entry.size {
			s.chunkCacheBytes -= entry.size
		} else {
			s.chunkCacheBytes = 0
		}
	}
	s.chunkCacheLRU.Remove(elem)
}

func (s *PhysicalStore) verifySegmentReadback(ctx context.Context, ref storage.SegmentRef, wantSize uint64, wantDigestHex string) (PhysicalRepairReason, error) {
	reader, err := s.getSegment(ctx, ref, 0, ref.SizeBytes, false)
	if err != nil {
		return PhysicalRepairReasonWriteReadbackFailed, fmt.Errorf("%w: physical write readback failed: %v", storage.ErrUnavailable, err)
	}
	defer reader.Close()
	hasher := sha256.New()
	copied, err := io.Copy(hasher, reader)
	if err != nil {
		return PhysicalRepairReasonWriteReadbackFailed, fmt.Errorf("%w: physical write readback failed: %v", storage.ErrUnavailable, err)
	}
	if uint64(copied) != wantSize {
		return PhysicalRepairReasonWriteReadbackMismatch, fmt.Errorf("%w: physical write readback size mismatch", storage.ErrUnavailable)
	}
	if hex.EncodeToString(hasher.Sum(nil)) != wantDigestHex {
		return PhysicalRepairReasonWriteReadbackMismatch, fmt.Errorf("%w: physical write readback mismatch", storage.ErrUnavailable)
	}
	return "", nil
}

func (s *PhysicalStore) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	return s.getSegment(ctx, ref, off, length, true)
}

func (s *PhysicalStore) getSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64, allowCache bool) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	addr, err := decodePhysicalSegmentID(ref.SegmentID)
	if err != nil {
		return nil, err
	}
	if addr.VolumeID != s.volumeID {
		return nil, storage.ErrNotFound
	}
	s.mu.Lock()
	_, deleted := s.deleted[ref.SegmentID]
	s.mu.Unlock()
	if deleted {
		return nil, storage.ErrNotFound
	}
	if off > addr.SizeBytes {
		return nil, storage.ErrInvalidRange
	}
	readLength := length
	if readLength == 0 {
		readLength = addr.SizeBytes - off
	}
	if off+readLength < off || off+readLength > addr.SizeBytes {
		return nil, storage.ErrInvalidRange
	}
	if readLength == 0 {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return &physicalSegmentReader{
		ctx:        ctx,
		store:      s,
		ref:        storage.CloneSegmentRef(ref),
		addr:       addr,
		nextOffset: off,
		remaining:  readLength,
		allowCache: allowCache,
	}, nil
}

type physicalSegmentReader struct {
	ctx        context.Context
	store      *PhysicalStore
	ref        storage.SegmentRef
	addr       physicalSegmentAddress
	nextOffset uint64
	remaining  uint64
	allowCache bool
	buf        []byte
	closed     bool
}

func (r *physicalSegmentReader) Read(p []byte) (int, error) {
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

func (r *physicalSegmentReader) Close() error {
	r.closed = true
	r.buf = nil
	return nil
}

func (r *physicalSegmentReader) readNextChunk() ([]byte, error) {
	s := r.store
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	logicalChunk := r.nextOffset / s.chunkSize
	chunkStart := logicalChunk * s.chunkSize
	copyStart := maxUint64(r.nextOffset, chunkStart)
	copyEnd := minUint64(r.nextOffset+r.remaining, chunkStart+s.chunkSize)
	chunkOffset := copyStart - chunkStart
	chunkLength := copyEnd - copyStart
	physicalChunkID := r.addr.StartChunkID + logicalChunk
	if r.allowCache {
		cacheStart := time.Now()
		if cached, ok := s.readCachedChunk(physicalChunkID, chunkOffset, chunkLength); ok {
			if s.metrics != nil {
				s.metrics.ObserveSBSPhysicalChunk("read_cache_hit", time.Since(cacheStart), chunkLength, nil)
			}
			r.nextOffset += chunkLength
			r.remaining -= chunkLength
			return cached, nil
		}
	}
	readStart := time.Now()
	resp, err := s.client.ReadPhysicalChunk(r.ctx, &sbsservice.ReadPhysicalChunkRequest{
		VolumeID:         s.volumeID,
		VolumeHandle:     s.volumeHandle,
		PhysicalChunkID:  physicalChunkID,
		ChunkOffsetBytes: chunkOffset,
		LengthBytes:      chunkLength,
		Context:          s.requestContext("read-physical", physicalChunkID),
	})
	if err != nil {
		if s.metrics != nil {
			s.metrics.ObserveSBSPhysicalChunk("read", time.Since(readStart), chunkLength, err)
		}
		mapped := mapSBSError(err)
		s.recordPhysicalRepair(r.ref, PhysicalRepairReasonReadFailed, mapped.Error())
		return nil, mapped
	}
	if resp == nil {
		err := fmt.Errorf("%w: nil physical chunk read response", storage.ErrUnavailable)
		if s.metrics != nil {
			s.metrics.ObserveSBSPhysicalChunk("read", time.Since(readStart), chunkLength, err)
		}
		s.recordPhysicalRepair(r.ref, PhysicalRepairReasonReadFailed, err.Error())
		return nil, err
	}
	if uint64(len(resp.Data)) < chunkLength {
		err := fmt.Errorf("%w: short physical chunk read", storage.ErrUnavailable)
		if s.metrics != nil {
			s.metrics.ObserveSBSPhysicalChunk("read", time.Since(readStart), chunkLength, err)
		}
		s.recordPhysicalRepair(r.ref, PhysicalRepairReasonReadFailed, err.Error())
		return nil, err
	}
	if s.metrics != nil {
		s.metrics.ObserveSBSPhysicalChunk("read", time.Since(readStart), chunkLength, nil)
	}
	if chunkOffset == 0 && chunkLength == s.chunkSize && uint64(len(resp.Data)) >= s.chunkSize {
		s.cacheChunk(physicalChunkID, resp.Data[:s.chunkSize])
	}
	r.nextOffset += chunkLength
	r.remaining -= chunkLength
	return resp.Data[:chunkLength], nil
}

func (s *PhysicalStore) DeleteSegment(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr, err := decodePhysicalSegmentID(ref.SegmentID)
	if err != nil {
		return err
	}
	if addr.VolumeID != s.volumeID {
		return storage.ErrNotFound
	}
	s.mu.Lock()
	if _, deleted := s.deleted[ref.SegmentID]; deleted {
		s.mu.Unlock()
		return storage.ErrNotFound
	}
	s.mu.Unlock()
	if err := storage.CheckDeleteAdmission(ctx, s.deleteAdmission, ref, reason); err != nil {
		return err
	}
	s.mu.Lock()
	if _, deleted := s.deleted[ref.SegmentID]; deleted {
		s.mu.Unlock()
		return storage.ErrNotFound
	}
	s.deleted[ref.SegmentID] = struct{}{}
	delete(s.gcQueue, ref.SegmentID)
	s.mu.Unlock()
	for i := uint32(0); i < addr.ChunkCount; i++ {
		s.evictCachedChunk(addr.StartChunkID + uint64(i))
	}

	deleter, ok := s.client.(PhysicalChunkDeleter)
	if !ok {
		return nil
	}
	for i := uint32(0); i < addr.ChunkCount; i++ {
		if err := deleter.DeletePhysicalChunk(ctx, s.volumeID, addr.StartChunkID+uint64(i)); err != nil {
			return fmt.Errorf("%w: %s", storage.ErrUnavailable, err.Error())
		}
	}
	_ = reason
	return nil
}

func (s *PhysicalStore) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	addr, err := decodePhysicalSegmentID(ref.SegmentID)
	if err != nil {
		return err
	}
	if addr.VolumeID != s.volumeID {
		return storage.ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, deleted := s.deleted[ref.SegmentID]; deleted {
		return storage.ErrNotFound
	}
	s.gcQueue[ref.SegmentID] = storage.GCCandidate{
		Ref:       storage.CloneSegmentRef(ref),
		Reason:    reason,
		CreatedAt: s.now(),
	}
	return nil
}

func (s *PhysicalStore) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidates := make([]storage.GCCandidate, 0, len(s.gcQueue))
	for _, candidate := range s.gcQueue {
		candidate.Ref = storage.CloneSegmentRef(candidate.Ref)
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

func (s *PhysicalStore) ListRepairCandidates(ctx context.Context, limit int) ([]PhysicalRepairCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	candidates := make([]PhysicalRepairCandidate, 0, len(s.repairQueue))
	for _, candidate := range s.repairQueue {
		candidate.Ref = storage.CloneSegmentRef(candidate.Ref)
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

func (s *PhysicalStore) requestContext(op string, chunkID uint64) sbsservice.SBSRequestContext {
	idempotencyKey := fmt.Sprintf("namros-%s-%s-%s-%d-%d", op, s.volumeID, s.attachmentID, s.generation, chunkID)
	return sbsservice.SBSRequestContext{
		RequestID:      newRequestID(op),
		GatewayID:      s.gatewayID,
		AttachmentID:   s.attachmentID,
		Generation:     s.generation,
		IdempotencyKey: s.sessionIdentity.ScopedIdempotencyKey(idempotencyKey),
	}
}

func physicalChunkCount(size, chunkSize uint64) uint32 {
	if size == 0 {
		return 0
	}
	count := (size-1)/chunkSize + 1
	if count > math.MaxUint32 {
		return 0
	}
	return uint32(count)
}

func physicalPlacementChunks(volumeID string, startChunkID uint64, chunkCount uint32, size, chunkSize uint64) []storage.PlacementChunk {
	if chunkCount == 0 {
		return nil
	}
	out := make([]storage.PlacementChunk, 0, chunkCount)
	for i := uint32(0); i < chunkCount; i++ {
		logicalOffset := uint64(i) * chunkSize
		length := minUint64(chunkSize, size-logicalOffset)
		out = append(out, storage.PlacementChunk{
			LogicalOffsetBytes: logicalOffset,
			SizeBytes:          length,
			VolumeID:           volumeID,
			LengthBytes:        length,
			ChunkID:            startChunkID + uint64(i),
			Role:               "data",
		})
	}
	return out
}

func encodePhysicalSegmentID(addr physicalSegmentAddress) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d", physicalBackendName, addr.VolumeID, addr.StartChunkID, addr.ChunkCount, addr.SizeBytes)
}

func decodePhysicalSegmentID(value string) (physicalSegmentAddress, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 || parts[0] != physicalBackendName {
		return physicalSegmentAddress{}, fmt.Errorf("%w: invalid sbs physical segment id", storage.ErrInvalidArgument)
	}
	startChunkID, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return physicalSegmentAddress{}, fmt.Errorf("%w: invalid sbs physical start chunk id", storage.ErrInvalidArgument)
	}
	chunkCount64, err := strconv.ParseUint(parts[3], 10, 32)
	if err != nil {
		return physicalSegmentAddress{}, fmt.Errorf("%w: invalid sbs physical chunk count", storage.ErrInvalidArgument)
	}
	size, err := strconv.ParseUint(parts[4], 10, 64)
	if err != nil {
		return physicalSegmentAddress{}, fmt.Errorf("%w: invalid sbs physical size", storage.ErrInvalidArgument)
	}
	return physicalSegmentAddress{
		VolumeID:     parts[1],
		StartChunkID: startChunkID,
		ChunkCount:   uint32(chunkCount64),
		SizeBytes:    size,
	}, nil
}

func minUint64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

var _ storage.SegmentStore = (*PhysicalStore)(nil)
var _ storage.OrphanTracker = (*PhysicalStore)(nil)

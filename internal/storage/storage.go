package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNotFound        = errors.New("segment not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrInvalidRange    = errors.New("invalid range")
	ErrUnavailable     = errors.New("storage unavailable")
	ErrProtected       = errors.New("segment protected")
)

type SegmentStore interface {
	PutSegment(ctx context.Context, req PutSegmentRequest) (SegmentRef, error)
	GetSegment(ctx context.Context, ref SegmentRef, off, length uint64) (io.ReadCloser, error)
	DeleteSegment(ctx context.Context, ref SegmentRef, reason DeleteReason) error
}

type SegmentValidator interface {
	ValidateSegment(ctx context.Context, ref SegmentRef) error
}

type HealthChecker interface {
	CheckHealth(ctx context.Context) error
}

type DeleteAdmissionFunc func(ctx context.Context, ref SegmentRef, reason DeleteReason) error

func CheckDeleteAdmission(ctx context.Context, admit DeleteAdmissionFunc, ref SegmentRef, reason DeleteReason) error {
	if admit == nil {
		return nil
	}
	return admit(ctx, CloneSegmentRef(ref), reason)
}

type OrphanTracker interface {
	MarkOrphan(ctx context.Context, ref SegmentRef, reason DeleteReason) error
	ListGCCandidates(ctx context.Context, limit int) ([]GCCandidate, error)
}

type StorageClassSnapshot struct {
	StorageClassID string
	Backend        string
	Parameters     map[string]string
}

type PlacementSnapshot struct {
	Backend           string
	Layout            string
	RedundancyBackend string
	ProfileID         string
	ProfileGeneration uint64
	ChunkSizeBytes    uint64
	Parameters        map[string]string
	Chunks            []PlacementChunk
}

type PlacementChunk struct {
	LogicalOffsetBytes uint64
	SizeBytes          uint64
	VolumeID           string
	OffsetBytes        uint64
	LengthBytes        uint64
	StoreID            string
	ShardID            uint32
	ChunkID            uint64
	Role               string
}

type Digest struct {
	Algorithm string
	Hex       string
}

type EncryptionEnvelope struct {
	Algorithm           string
	KeyID               string
	KeyVersion          string
	WrappedDEK          string
	Nonce               string
	PlaintextSizeBytes  uint64
	CiphertextSizeBytes uint64
	Context             map[string]string
}

type SegmentRef struct {
	SegmentID      string
	StorageClass   StorageClassSnapshot
	Placement      PlacementSnapshot
	SizeBytes      uint64
	Digest         Digest
	Encryption     EncryptionEnvelope
	CreatedAt      time.Time
	SharedObjectID string
}

type PutSegmentRequest struct {
	Reader       io.Reader
	SizeBytes    uint64
	StorageClass StorageClassSnapshot
}

type GetSegmentRequest struct {
	Ref    SegmentRef
	Offset uint64
	Length uint64
}

type DeleteSegmentRequest struct {
	Ref    SegmentRef
	Reason DeleteReason
}

type DeleteReason string

const (
	DeleteReasonPublishFailed     DeleteReason = "publish_failed"
	DeleteReasonObjectOverwritten DeleteReason = "object_overwritten"
	DeleteReasonMultipartAborted  DeleteReason = "multipart_aborted"
	DeleteReasonManualGC          DeleteReason = "manual_gc"
	DeleteReasonDedupeReplaced    DeleteReason = "dedupe_replaced"
	DeleteReasonVolumeDrained     DeleteReason = "volume_drained"
)

type GCCandidate struct {
	Ref       SegmentRef
	Reason    DeleteReason
	CreatedAt time.Time
}

func CloneStorageClassSnapshot(in StorageClassSnapshot) StorageClassSnapshot {
	out := in
	out.Parameters = cloneStringMap(in.Parameters)
	return out
}

func ClonePlacementSnapshot(in PlacementSnapshot) PlacementSnapshot {
	out := in
	out.Parameters = cloneStringMap(in.Parameters)
	if len(in.Chunks) > 0 {
		out.Chunks = append([]PlacementChunk(nil), in.Chunks...)
	}
	return out
}

func CloneSegmentRef(in SegmentRef) SegmentRef {
	out := in
	out.StorageClass = CloneStorageClassSnapshot(in.StorageClass)
	out.Placement = ClonePlacementSnapshot(in.Placement)
	out.Encryption.Context = cloneStringMap(in.Encryption.Context)
	return out
}

func CloneSegmentRefs(in []SegmentRef) []SegmentRef {
	if len(in) == 0 {
		return nil
	}
	out := make([]SegmentRef, 0, len(in))
	for _, ref := range in {
		out = append(out, CloneSegmentRef(ref))
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

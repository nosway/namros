package sbs

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/storage"

	"google.golang.org/grpc"
)

const EnterpriseECStub = true

type ECOpenConfig struct {
	DataEndpoint     string
	VolumeID         string
	VolumeIDRaw      uint64
	GatewayID        string
	AttachmentID     string
	Generation       uint64
	SessionIdentity  any
	SessionCache     any
	SessionFence     any
	ShardStoreIDs    []string
	ShardConcurrency int
	DialOptions      []grpc.DialOption
	Metrics          any
	DeleteAdmission  storage.DeleteAdmissionFunc
	Now              func() time.Time
}

type ClusterOpenConfig struct {
	AdminEndpoint              string
	DataEndpoint               string
	VolumeID                   string
	VolumeIDRaw                uint64
	ChunkSizeBytes             uint64
	GatewayID                  string
	AttachmentID               string
	Generation                 uint64
	SessionIdentity            any
	SessionCache               any
	SessionFence               any
	VerifyReadback             bool
	WriteConcurrency           int
	FullChunkWriteMinBytes     uint64
	FullChunkWriteMaxBytes     uint64
	ChunkCacheBytes            uint64
	ChunkIDAllocationCacheSize uint32
	Metrics                    any
	ECMetrics                  any
	ShardStoreIDs              []string
	ECShardConcurrency         int
	DialOptions                []grpc.DialOption
	DeleteAdmission            storage.DeleteAdmissionFunc
	Now                        func() time.Time
}

type ECConfig struct {
	VolumeID         string
	VolumeIDRaw      uint64
	VolumeHandle     string
	GatewayID        string
	AttachmentID     string
	Generation       uint64
	SessionIdentity  any
	SessionFence     any
	ShardStoreIDs    []string
	ShardConcurrency int
	Client           any
	Metrics          any
	DeleteAdmission  storage.DeleteAdmissionFunc
	Now              func() time.Time
}

type ECStore struct{}

func OpenEC(context.Context, ECOpenConfig) (*ECStore, func() error, error) {
	return nil, nil, enterpriseError(edition.FeatureErasureCoding)
}

func OpenCluster(context.Context, ClusterOpenConfig) (storage.SegmentStore, func() error, error) {
	return nil, nil, enterpriseError(edition.FeatureErasureCoding)
}

func NewECStore(ECConfig) (*ECStore, error) {
	return nil, enterpriseError(edition.FeatureErasureCoding)
}

func (s *ECStore) PutSegment(context.Context, storage.PutSegmentRequest) (storage.SegmentRef, error) {
	return storage.SegmentRef{}, enterpriseError(edition.FeatureErasureCoding)
}

func (s *ECStore) GetSegment(context.Context, storage.SegmentRef, uint64, uint64) (io.ReadCloser, error) {
	return nil, enterpriseError(edition.FeatureErasureCoding)
}

func (s *ECStore) DeleteSegment(context.Context, storage.SegmentRef, storage.DeleteReason) error {
	return enterpriseError(edition.FeatureErasureCoding)
}

func (s *ECStore) MarkOrphan(context.Context, storage.SegmentRef, storage.DeleteReason) error {
	return enterpriseError(edition.FeatureErasureCoding)
}

func (s *ECStore) ListGCCandidates(context.Context, int) ([]storage.GCCandidate, error) {
	return nil, enterpriseError(edition.FeatureErasureCoding)
}

func enterpriseError(featureID string) error {
	if err := edition.Require(edition.Current(), featureID); err != nil {
		return err
	}
	return fmt.Errorf("SBS EC implementation is provided by the private Enterprise source overlay")
}

var _ storage.SegmentStore = (*ECStore)(nil)
var _ storage.OrphanTracker = (*ECStore)(nil)

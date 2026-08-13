package sbs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namrbd/gateway/sbsgrpc"
	sbsservice "github.com/nosway/namrbd/gateway/service"
	internalv1 "github.com/nosway/namrbd/sbs/internalapi/v1"
	sbsv1 "github.com/nosway/namrbd/sbs/v1"

	"github.com/nosway/namros/internal/storage"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type PhysicalOpenConfig struct {
	AdminEndpoint              string
	DataEndpoint               string
	VolumeID                   string
	VolumeIDRaw                uint64
	ChunkSizeBytes             uint64
	GatewayID                  string
	AttachmentID               string
	Generation                 uint64
	SessionIdentity            SessionIdentity
	SessionCache               *VolumeSessionCache
	SessionFence               SessionFenceSnapshot
	VerifyReadback             bool
	WriteConcurrency           int
	FullChunkWriteMinBytes     uint64
	FullChunkWriteMaxBytes     uint64
	ChunkCacheBytes            uint64
	ChunkIDAllocationCacheSize uint32
	Metrics                    PhysicalMetrics
	DialOptions                []grpc.DialOption
	DeleteAdmission            storage.DeleteAdmissionFunc
	Now                        func() time.Time
}

func OpenPhysical(ctx context.Context, cfg PhysicalOpenConfig) (*PhysicalStore, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	cfg.AdminEndpoint = strings.TrimSpace(cfg.AdminEndpoint)
	if cfg.AdminEndpoint == "" {
		return nil, nil, fmt.Errorf("%w: sbs admin endpoint is required", storage.ErrInvalidArgument)
	}
	cfg.DataEndpoint = strings.TrimSpace(cfg.DataEndpoint)
	if cfg.DataEndpoint == "" {
		return nil, nil, fmt.Errorf("%w: sbs data endpoint is required", storage.ErrInvalidArgument)
	}

	volumeID := strings.TrimSpace(cfg.VolumeID)
	if volumeID == "" {
		raw := cfg.VolumeIDRaw
		if raw == 0 {
			raw = defaultVolumeID
		}
		volumeID = sbsservice.CanonicalVolumeID(raw)
	}
	gatewayID := strings.TrimSpace(cfg.GatewayID)
	if gatewayID == "" {
		gatewayID = defaultGatewayID
	}
	attachmentID := strings.TrimSpace(cfg.AttachmentID)
	if attachmentID == "" {
		attachmentID = "att-" + volumeID + "-physical"
	}
	generation := cfg.Generation
	if generation == 0 {
		generation = defaultGeneration
	}
	sessionIdentity, err := NormalizeSessionIdentity(cfg.SessionIdentity, SessionIdentityDefaults{
		VolumeID:          volumeID,
		MemberGeneration:  generation,
		GatewayID:         gatewayID,
		SessionTTL:        DefaultSessionTTL,
		HeartbeatInterval: DefaultSessionHeartbeat,
	})
	if err != nil {
		return nil, nil, err
	}
	if err := requireVolumeSessionCacheForIdentity(sessionIdentity, cfg.SessionCache); err != nil {
		return nil, nil, err
	}

	dialOptions := physicalGRPCDialOptions(cfg.DialOptions)
	adminConn, err := grpc.NewClient(cfg.AdminEndpoint, dialOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: dial sbs admin endpoint %q: %v", storage.ErrUnavailable, cfg.AdminEndpoint, err)
	}
	cleanup := func() error {
		return adminConn.Close()
	}

	dataConn, err := grpc.NewClient(cfg.DataEndpoint, dialOptions...)
	if err != nil {
		_ = cleanup()
		return nil, nil, fmt.Errorf("%w: dial sbs data endpoint %q: %v", storage.ErrUnavailable, cfg.DataEndpoint, err)
	}
	cleanup = joinCleanup(cleanup, dataConn.Close)

	dataClient := sbsgrpc.NewClient(sbsv1.NewVolumeServiceClient(dataConn))
	openReq := &sbsservice.OpenVolumeRequest{
		VolumeID:   volumeID,
		AccessMode: sbsservice.SBSAccessModeExclusiveWriter,
		Context: sbsservice.SBSRequestContext{
			RequestID:    newRequestID("open-physical"),
			GatewayID:    gatewayID,
			AttachmentID: attachmentID,
			Generation:   generation,
		},
	}
	sessionKey := sessionIdentity.VolumeSessionKey(cfg.DataEndpoint, string(openReq.AccessMode), volumeID, gatewayID, attachmentID, generation)
	session, closeSession, err := acquireVolumeSession(ctx, dataClient, cfg.SessionCache, sessionKey, openReq, func(session VolumeSession) *sbsservice.CloseVolumeRequest {
		return &sbsservice.CloseVolumeRequest{
			VolumeID:     volumeID,
			VolumeHandle: session.VolumeHandle,
			Context: sbsservice.SBSRequestContext{
				RequestID:    newRequestID("close-physical"),
				GatewayID:    gatewayID,
				AttachmentID: attachmentID,
				Generation:   generation,
			},
		}
	})
	if err != nil {
		_ = cleanup()
		return nil, nil, fmt.Errorf("open sbs volume %q on data endpoint %q: %w", volumeID, cfg.DataEndpoint, mapSBSError(err))
	}
	volumeHandle := session.VolumeHandle
	cleanup = joinCleanup(closeSession, cleanup)

	allocator := PhysicalChunkAllocator(NewGRPCPhysicalChunkAllocator(internalv1.NewChunkIDAllocatorServiceClient(adminConn)))
	allocator = NewCachedPhysicalChunkAllocator(allocator, cfg.ChunkIDAllocationCacheSize)

	store, err := NewPhysicalStore(PhysicalConfig{
		VolumeID:               volumeID,
		ChunkSizeBytes:         cfg.ChunkSizeBytes,
		GatewayID:              gatewayID,
		AttachmentID:           attachmentID,
		Generation:             generation,
		SessionIdentity:        sessionIdentity,
		SessionFence:           cfg.SessionFence,
		VolumeHandle:           volumeHandle,
		VerifyReadback:         cfg.VerifyReadback,
		WriteConcurrency:       cfg.WriteConcurrency,
		FullChunkWriteMinBytes: cfg.FullChunkWriteMinBytes,
		FullChunkWriteMaxBytes: cfg.FullChunkWriteMaxBytes,
		ChunkCacheBytes:        cfg.ChunkCacheBytes,
		Allocator:              allocator,
		Client:                 dataClient,
		Metrics:                cfg.Metrics,
		DeleteAdmission:        cfg.DeleteAdmission,
		Now:                    cfg.Now,
	})
	if err != nil {
		_ = cleanup()
		return nil, nil, err
	}
	return store, cleanup, nil
}

func physicalGRPCDialOptions(extra []grpc.DialOption) []grpc.DialOption {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	return append(opts, extra...)
}

func joinCleanup(first, second func() error) func() error {
	return func() error {
		return errors.Join(first(), second())
	}
}

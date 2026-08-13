package sbs

import (
	"context"
	"fmt"
	"strings"

	internalv1 "github.com/nosway/namrbd/sbs/internalapi/v1"

	"github.com/nosway/namros/internal/storage"
)

type GRPCPhysicalChunkAllocator struct {
	client internalv1.ChunkIDAllocatorServiceClient
}

func NewGRPCPhysicalChunkAllocator(client internalv1.ChunkIDAllocatorServiceClient) *GRPCPhysicalChunkAllocator {
	return &GRPCPhysicalChunkAllocator{client: client}
}

func (a *GRPCPhysicalChunkAllocator) AllocateChunkIDs(ctx context.Context, volumeID string, count uint32) (uint64, error) {
	volumeID = strings.TrimSpace(volumeID)
	if volumeID == "" {
		return 0, fmt.Errorf("%w: volume id is required", storage.ErrInvalidArgument)
	}
	if count == 0 {
		return 0, nil
	}
	if a == nil || a.client == nil {
		return 0, fmt.Errorf("%w: chunk id allocator client is required", storage.ErrInvalidArgument)
	}
	resp, err := a.client.AllocateChunkIDs(ctx, &internalv1.AllocateChunkIDsRequest{
		VolumeId: volumeID,
		Count:    count,
	})
	if err != nil {
		return 0, fmt.Errorf("%w: %v", storage.ErrUnavailable, err)
	}
	if resp.GetVolumeId() != volumeID {
		return 0, fmt.Errorf("%w: allocator volume mismatch got %q want %q", storage.ErrInvalidArgument, resp.GetVolumeId(), volumeID)
	}
	if resp.GetCount() != count {
		return 0, fmt.Errorf("%w: allocator count mismatch got %d want %d", storage.ErrInvalidArgument, resp.GetCount(), count)
	}
	if resp.GetStartChunkId() == 0 {
		return 0, fmt.Errorf("%w: allocator returned zero start chunk id", storage.ErrInvalidArgument)
	}
	return resp.GetStartChunkId(), nil
}

var _ PhysicalChunkAllocator = (*GRPCPhysicalChunkAllocator)(nil)

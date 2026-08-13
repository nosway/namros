package sbs

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/nosway/namros/internal/storage"
)

type cachedPhysicalChunkAllocator struct {
	next      PhysicalChunkAllocator
	cacheSize uint32

	mu      sync.Mutex
	volumes map[string]physicalChunkIDCacheRange
}

type physicalChunkIDCacheRange struct {
	next uint64
	end  uint64
}

func NewCachedPhysicalChunkAllocator(next PhysicalChunkAllocator, cacheSize uint32) PhysicalChunkAllocator {
	if cacheSize == 0 {
		return next
	}
	return &cachedPhysicalChunkAllocator{
		next:      next,
		cacheSize: cacheSize,
		volumes:   make(map[string]physicalChunkIDCacheRange),
	}
}

func (a *cachedPhysicalChunkAllocator) AllocateChunkIDs(ctx context.Context, volumeID string, count uint32) (uint64, error) {
	if count == 0 {
		return 0, nil
	}
	volumeID = strings.TrimSpace(volumeID)
	if volumeID == "" {
		return 0, fmt.Errorf("%w: volume id is required", storage.ErrInvalidArgument)
	}
	if a == nil || a.next == nil {
		return 0, fmt.Errorf("%w: chunk id allocator is required", storage.ErrInvalidArgument)
	}
	if a.cacheSize == 0 || count > a.cacheSize {
		return a.next.AllocateChunkIDs(ctx, volumeID, count)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	cached := a.volumes[volumeID]
	if cached.end-cached.next < uint64(count) {
		refillCount := a.cacheSize
		start, err := a.next.AllocateChunkIDs(ctx, volumeID, refillCount)
		if err != nil {
			return 0, err
		}
		if start > ^uint64(0)-uint64(refillCount) {
			return 0, fmt.Errorf("%w: chunk id allocation range overflow", storage.ErrInvalidArgument)
		}
		cached = physicalChunkIDCacheRange{
			next: start,
			end:  start + uint64(refillCount),
		}
	}

	start := cached.next
	cached.next += uint64(count)
	a.volumes[volumeID] = cached
	return start, nil
}

var _ PhysicalChunkAllocator = (*cachedPhysicalChunkAllocator)(nil)

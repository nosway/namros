package sbs_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/nosway/namros/internal/storage"
	namrossbs "github.com/nosway/namros/internal/storage/sbs"
)

type recordingPhysicalChunkAllocator struct {
	mu    sync.Mutex
	next  uint64
	calls []uint32
	err   error
}

func (a *recordingPhysicalChunkAllocator) AllocateChunkIDs(_ context.Context, _ string, count uint32) (uint64, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return 0, a.err
	}
	if a.next == 0 {
		a.next = 1
	}
	start := a.next
	a.next += uint64(count)
	a.calls = append(a.calls, count)
	return start, nil
}

func TestCachedPhysicalChunkAllocatorServesFromRefilledRange(t *testing.T) {
	base := &recordingPhysicalChunkAllocator{}
	allocator := namrossbs.NewCachedPhysicalChunkAllocator(base, 8)

	got1, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 2)
	if err != nil {
		t.Fatalf("AllocateChunkIDs #1: %v", err)
	}
	got2, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 2)
	if err != nil {
		t.Fatalf("AllocateChunkIDs #2: %v", err)
	}
	got3, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 4)
	if err != nil {
		t.Fatalf("AllocateChunkIDs #3: %v", err)
	}
	if got1 != 1 || got2 != 3 || got3 != 5 {
		t.Fatalf("starts=(%d,%d,%d), want (1,3,5)", got1, got2, got3)
	}
	if len(base.calls) != 1 || base.calls[0] != 8 {
		t.Fatalf("base calls=%v, want [8]", base.calls)
	}

	got4, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 2)
	if err != nil {
		t.Fatalf("AllocateChunkIDs #4: %v", err)
	}
	if got4 != 9 {
		t.Fatalf("got4=%d, want 9", got4)
	}
	if len(base.calls) != 2 || base.calls[1] != 8 {
		t.Fatalf("base calls=%v, want [8 8]", base.calls)
	}
}

func TestCachedPhysicalChunkAllocatorKeepsVolumeRangesSeparate(t *testing.T) {
	base := &recordingPhysicalChunkAllocator{}
	allocator := namrossbs.NewCachedPhysicalChunkAllocator(base, 4)

	gotA, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 2)
	if err != nil {
		t.Fatalf("AllocateChunkIDs volume A: %v", err)
	}
	gotB, err := allocator.AllocateChunkIDs(t.Context(), "18a00002", 2)
	if err != nil {
		t.Fatalf("AllocateChunkIDs volume B: %v", err)
	}
	gotA2, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 2)
	if err != nil {
		t.Fatalf("AllocateChunkIDs volume A #2: %v", err)
	}
	if gotA != 1 || gotB != 5 || gotA2 != 3 {
		t.Fatalf("starts=(%d,%d,%d), want (1,5,3)", gotA, gotB, gotA2)
	}
	if len(base.calls) != 2 {
		t.Fatalf("base calls=%v, want two refills", base.calls)
	}
}

func TestCachedPhysicalChunkAllocatorBypassesOversizedRequests(t *testing.T) {
	base := &recordingPhysicalChunkAllocator{}
	allocator := namrossbs.NewCachedPhysicalChunkAllocator(base, 4)

	start, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 5)
	if err != nil {
		t.Fatalf("AllocateChunkIDs oversized: %v", err)
	}
	if start != 1 {
		t.Fatalf("start=%d, want 1", start)
	}
	if len(base.calls) != 1 || base.calls[0] != 5 {
		t.Fatalf("base calls=%v, want [5]", base.calls)
	}
}

func TestCachedPhysicalChunkAllocatorValidatesAndPropagatesErrors(t *testing.T) {
	if _, err := namrossbs.NewCachedPhysicalChunkAllocator(&recordingPhysicalChunkAllocator{}, 4).AllocateChunkIDs(t.Context(), " ", 1); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("blank volume error=%v, want ErrInvalidArgument", err)
	}

	expected := errors.New("allocator offline")
	allocator := namrossbs.NewCachedPhysicalChunkAllocator(&recordingPhysicalChunkAllocator{err: expected}, 4)
	if _, err := allocator.AllocateChunkIDs(t.Context(), "18a00001", 1); !errors.Is(err, expected) {
		t.Fatalf("AllocateChunkIDs error=%v, want %v", err, expected)
	}
}

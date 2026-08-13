package sbs_test

import (
	"context"
	"errors"
	"testing"

	internalv1 "github.com/nosway/namrbd/sbs/internalapi/v1"

	"github.com/nosway/namros/internal/storage"
	namrossbs "github.com/nosway/namros/internal/storage/sbs"

	"google.golang.org/grpc"
)

type fakeAllocatorServiceClient struct {
	req   *internalv1.AllocateChunkIDsRequest
	resp  *internalv1.AllocateChunkIDsResponse
	err   error
	calls int
}

func (c *fakeAllocatorServiceClient) AllocateChunkIDs(_ context.Context, req *internalv1.AllocateChunkIDsRequest, _ ...grpc.CallOption) (*internalv1.AllocateChunkIDsResponse, error) {
	c.req = req
	c.calls++
	return c.resp, c.err
}

func TestGRPCPhysicalChunkAllocatorAllocatesChunkIDs(t *testing.T) {
	client := &fakeAllocatorServiceClient{
		resp: &internalv1.AllocateChunkIDsResponse{
			VolumeId:     "0a0b0002",
			Count:        3,
			StartChunkId: 42,
		},
	}
	allocator := namrossbs.NewGRPCPhysicalChunkAllocator(client)
	startChunkID, err := allocator.AllocateChunkIDs(t.Context(), "0a0b0002", 3)
	if err != nil {
		t.Fatalf("AllocateChunkIDs() error = %v", err)
	}
	if startChunkID != 42 {
		t.Fatalf("start chunk id = %d, want 42", startChunkID)
	}
	if client.calls != 1 {
		t.Fatalf("calls = %d, want 1", client.calls)
	}
	if client.req.GetVolumeId() != "0a0b0002" || client.req.GetCount() != 3 {
		t.Fatalf("request = %+v", client.req)
	}
}

func TestGRPCPhysicalChunkAllocatorSkipsZeroCount(t *testing.T) {
	client := &fakeAllocatorServiceClient{}
	allocator := namrossbs.NewGRPCPhysicalChunkAllocator(client)
	startChunkID, err := allocator.AllocateChunkIDs(t.Context(), "0a0b0002", 0)
	if err != nil {
		t.Fatalf("AllocateChunkIDs() error = %v", err)
	}
	if startChunkID != 0 || client.calls != 0 {
		t.Fatalf("start=%d calls=%d, want zero/no call", startChunkID, client.calls)
	}
}

func TestGRPCPhysicalChunkAllocatorMapsTransportError(t *testing.T) {
	allocator := namrossbs.NewGRPCPhysicalChunkAllocator(&fakeAllocatorServiceClient{
		err: errors.New("allocator offline"),
	})
	_, err := allocator.AllocateChunkIDs(t.Context(), "0a0b0002", 1)
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("AllocateChunkIDs() error = %v, want ErrUnavailable", err)
	}
}

func TestGRPCPhysicalChunkAllocatorRejectsMismatchedResponse(t *testing.T) {
	allocator := namrossbs.NewGRPCPhysicalChunkAllocator(&fakeAllocatorServiceClient{
		resp: &internalv1.AllocateChunkIDsResponse{
			VolumeId:     "0a0b0002",
			Count:        2,
			StartChunkId: 42,
		},
	})
	_, err := allocator.AllocateChunkIDs(t.Context(), "0a0b0002", 1)
	if !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("AllocateChunkIDs() error = %v, want ErrInvalidArgument", err)
	}
}

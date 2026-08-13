package sbs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nosway/namrbd/gateway/sbsgrpc"
	sbsservice "github.com/nosway/namrbd/gateway/service"
	internalv1 "github.com/nosway/namrbd/sbs/internalapi/v1"
	sbsv1 "github.com/nosway/namrbd/sbs/v1"

	"github.com/nosway/namros/internal/storage"
	namrossbs "github.com/nosway/namros/internal/storage/sbs"

	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
)

func TestOpenPhysicalStoreUsesGRPCEndpoints(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	allocator := &bufconnChunkAllocatorServer{next: 100}
	internalv1.RegisterChunkIDAllocatorServiceServer(grpcServer, allocator)

	spec := sbsservice.NormalizeVolumeSpec(sbsservice.VolumeSpec{
		ID:             sbsservice.HexVolumeID(0x0a0b0002),
		Name:           "namros-physical-test",
		Prefix:         sbsservice.BuildVolumePrefix("namros-physical-test", 0x0a0b0002),
		SizeBytes:      1024 * 1024,
		BlockSize:      1,
		ChunkSizeBytes: 8,
	})
	dataClient := sbsservice.NewInMemorySBSClient([]sbsservice.VolumeSpec{spec})
	sbsv1.RegisterVolumeServiceServer(grpcServer, sbsgrpc.NewServer(dataClient))
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(grpcServer.Stop)

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	store, cleanup, err := namrossbs.OpenPhysical(t.Context(), namrossbs.PhysicalOpenConfig{
		AdminEndpoint:  "passthrough:///namros-physical-admin",
		DataEndpoint:   "passthrough:///namros-physical-data",
		VolumeID:       "0a0b0002",
		ChunkSizeBytes: 8,
		GatewayID:      "gw-test",
		AttachmentID:   "att-test",
		Generation:     1,
		DialOptions:    []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if err != nil {
		t.Fatalf("OpenPhysical() error = %v", err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Fatalf("cleanup() error = %v", err)
		}
	})

	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("abcdefghijklmnopq")),
		SizeBytes: 17,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if ref.Placement.Layout != "sbs-physical-chunks" {
		t.Fatalf("placement layout = %q", ref.Placement.Layout)
	}
	if allocator.calls != 1 || allocator.lastVolumeID != "0a0b0002" || allocator.lastCount != 3 {
		t.Fatalf("allocator calls=%d volume=%q count=%d", allocator.calls, allocator.lastVolumeID, allocator.lastCount)
	}

	reader, err := store.GetSegment(t.Context(), ref, 5, 9)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "fghijklmn" {
		t.Fatalf("range = %q, want fghijklmn", got)
	}
}

func TestOpenPhysicalStoreWrapsMissingVolume(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	internalv1.RegisterChunkIDAllocatorServiceServer(grpcServer, &bufconnChunkAllocatorServer{next: 100})
	sbsv1.RegisterVolumeServiceServer(grpcServer, sbsgrpc.NewServer(sbsservice.NewInMemorySBSClient(nil)))
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(grpcServer.Stop)

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	_, cleanup, err := namrossbs.OpenPhysical(t.Context(), namrossbs.PhysicalOpenConfig{
		AdminEndpoint: "passthrough:///namros-missing-admin",
		DataEndpoint:  "passthrough:///namros-missing-data",
		VolumeID:      "0a0b00ff",
		DialOptions:   []grpc.DialOption{grpc.WithContextDialer(dialer)},
	})
	if cleanup != nil {
		t.Fatalf("cleanup is non-nil, want nil")
	}
	if !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("OpenPhysical() error = %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), `open sbs volume "0a0b00ff" on data endpoint "passthrough:///namros-missing-data"`) {
		t.Fatalf("OpenPhysical() error = %q, want volume and endpoint context", err)
	}
}

func TestOpenPhysicalSessionCacheReferenceCountsRemoteOpen(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	internalv1.RegisterChunkIDAllocatorServiceServer(grpcServer, &bufconnChunkAllocatorServer{next: 500})

	spec := sbsservice.NormalizeVolumeSpec(sbsservice.VolumeSpec{
		ID:             sbsservice.HexVolumeID(0x0a0b0005),
		Name:           "namros-physical-session-test",
		Prefix:         sbsservice.BuildVolumePrefix("namros-physical-session-test", 0x0a0b0005),
		SizeBytes:      1024 * 1024,
		BlockSize:      1,
		ChunkSizeBytes: 8,
	})
	dataClient := &countingVolumeClient{InMemorySBSClient: sbsservice.NewInMemorySBSClient([]sbsservice.VolumeSpec{spec})}
	sbsv1.RegisterVolumeServiceServer(grpcServer, sbsgrpc.NewServer(dataClient))
	go func() {
		_ = grpcServer.Serve(lis)
	}()
	t.Cleanup(grpcServer.Stop)

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}
	cache := namrossbs.NewVolumeSessionCache()
	cfg := namrossbs.PhysicalOpenConfig{
		AdminEndpoint:   "passthrough:///namros-physical-session-admin",
		DataEndpoint:    "passthrough:///namros-physical-session-data",
		VolumeID:        "0a0b0005",
		ChunkSizeBytes:  8,
		GatewayID:       "gw-session",
		AttachmentID:    "att-session",
		Generation:      1,
		SessionIdentity: testOpenSessionIdentity(),
		SessionCache:    cache,
		DialOptions:     []grpc.DialOption{grpc.WithContextDialer(dialer)},
	}
	first, cleanupFirst, err := namrossbs.OpenPhysical(t.Context(), cfg)
	if err != nil {
		t.Fatalf("OpenPhysical(first) error = %v", err)
	}
	cleanupFirstCalled := false
	defer func() {
		if !cleanupFirstCalled {
			_ = cleanupFirst()
		}
	}()
	second, cleanupSecond, err := namrossbs.OpenPhysical(t.Context(), cfg)
	if err != nil {
		t.Fatalf("OpenPhysical(second) error = %v", err)
	}
	cleanupSecondCalled := false
	defer func() {
		if !cleanupSecondCalled {
			_ = cleanupSecond()
		}
	}()

	if opens, closes := dataClient.counts(); opens != 1 || closes != 0 {
		t.Fatalf("after two opens counts = %d/%d, want 1/0", opens, closes)
	}
	if err := cleanupFirst(); err != nil {
		t.Fatalf("cleanupFirst() error = %v", err)
	}
	cleanupFirstCalled = true
	if opens, closes := dataClient.counts(); opens != 1 || closes != 0 {
		t.Fatalf("after first cleanup counts = %d/%d, want 1/0", opens, closes)
	}

	ref, err := second.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte("still-open-after-first-cleanup")),
		SizeBytes: 30,
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "sbs",
		},
	})
	if err != nil {
		t.Fatalf("PutSegment(after first cleanup) error = %v", err)
	}
	reader, err := second.GetSegment(t.Context(), ref, 0, 10)
	if err != nil {
		t.Fatalf("GetSegment(after first cleanup) error = %v", err)
	}
	payload, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("ReadAll(after first cleanup) error = %v", err)
	}
	if string(payload) != "still-open" {
		t.Fatalf("range = %q, want still-open", payload)
	}

	if err := cleanupSecond(); err != nil {
		t.Fatalf("cleanupSecond() error = %v", err)
	}
	cleanupSecondCalled = true
	if opens, closes := dataClient.counts(); opens != 1 || closes != 1 {
		t.Fatalf("after second cleanup counts = %d/%d, want 1/1", opens, closes)
	}
	if first == nil {
		t.Fatal("first store is nil")
	}
}

func TestOpenPhysicalRequiresSessionCacheForSessionIdentity(t *testing.T) {
	_, cleanup, err := namrossbs.OpenPhysical(t.Context(), namrossbs.PhysicalOpenConfig{
		AdminEndpoint:   "passthrough:///namros-physical-session-admin",
		DataEndpoint:    "passthrough:///namros-physical-session-data",
		VolumeID:        "0a0b0005",
		GatewayID:       "gw-session",
		AttachmentID:    "att-session",
		Generation:      1,
		SessionIdentity: testOpenSessionIdentity(),
	})
	if cleanup != nil {
		t.Fatal("cleanup is non-nil, want nil")
	}
	if !errors.Is(err, storage.ErrInvalidArgument) || !strings.Contains(err.Error(), "session cache") {
		t.Fatalf("OpenPhysical() error = %v, want session cache invalid argument", err)
	}
}

type bufconnChunkAllocatorServer struct {
	internalv1.UnimplementedChunkIDAllocatorServiceServer
	next         uint64
	calls        int
	lastVolumeID string
	lastCount    uint32
}

func (s *bufconnChunkAllocatorServer) AllocateChunkIDs(_ context.Context, req *internalv1.AllocateChunkIDsRequest) (*internalv1.AllocateChunkIDsResponse, error) {
	s.calls++
	s.lastVolumeID = req.GetVolumeId()
	s.lastCount = req.GetCount()
	start := s.next
	s.next += uint64(req.GetCount())
	return &internalv1.AllocateChunkIDsResponse{
		VolumeId:     req.GetVolumeId(),
		Count:        req.GetCount(),
		StartChunkId: start,
	}, nil
}

type countingVolumeClient struct {
	*sbsservice.InMemorySBSClient

	mu     sync.Mutex
	opens  int
	closes int
}

func (c *countingVolumeClient) OpenVolume(ctx context.Context, req *sbsservice.OpenVolumeRequest) (*sbsservice.OpenVolumeResponse, error) {
	c.mu.Lock()
	c.opens++
	c.mu.Unlock()
	return c.InMemorySBSClient.OpenVolume(ctx, req)
}

func (c *countingVolumeClient) CloseVolume(ctx context.Context, req *sbsservice.CloseVolumeRequest) (*sbsservice.CloseVolumeResponse, error) {
	c.mu.Lock()
	c.closes++
	c.mu.Unlock()
	return c.InMemorySBSClient.CloseVolume(ctx, req)
}

func (c *countingVolumeClient) counts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens, c.closes
}

func testOpenSessionIdentity() namrossbs.SessionIdentity {
	return namrossbs.SessionIdentity{
		PoolID:            "object-pool",
		PoolGeneration:    1,
		VolumeEpoch:       1,
		WriterGroupID:     "object-writers",
		GatewayInstanceID: "gateway-instance-a",
		SessionID:         "gateway-instance-a-boot-1",
		SessionGeneration: 1,
		SessionTTL:        30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}

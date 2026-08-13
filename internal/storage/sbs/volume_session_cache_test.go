package sbs

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/nosway/namros/internal/storage"
)

func TestVolumeSessionCacheReferenceCountsMatchingKey(t *testing.T) {
	cache := NewVolumeSessionCache()
	key := testVolumeSessionKey()
	var opens int
	var closes int

	open := func(context.Context) (VolumeSession, error) {
		opens++
		return VolumeSession{
			VolumeID:     key.VolumeID,
			VolumeHandle: "handle-1",
		}, nil
	}
	close := func(session VolumeSession) error {
		closes++
		if session.VolumeHandle != "handle-1" {
			t.Fatalf("close handle = %q, want handle-1", session.VolumeHandle)
		}
		return nil
	}

	first, err := cache.Acquire(t.Context(), key, open)
	if err != nil {
		t.Fatalf("Acquire(first) error = %v", err)
	}
	second, err := cache.Acquire(t.Context(), key, open)
	if err != nil {
		t.Fatalf("Acquire(second) error = %v", err)
	}
	if opens != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
	if first.Session().VolumeHandle != second.Session().VolumeHandle {
		t.Fatalf("leases handles = %q/%q, want shared", first.Session().VolumeHandle, second.Session().VolumeHandle)
	}

	if err := first.Release(close); err != nil {
		t.Fatalf("Release(first) error = %v", err)
	}
	if closes != 0 || cache.Len() != 1 {
		t.Fatalf("after first release closes=%d len=%d, want 0/1", closes, cache.Len())
	}
	if err := second.Release(close); err != nil {
		t.Fatalf("Release(second) error = %v", err)
	}
	if closes != 1 || cache.Len() != 0 {
		t.Fatalf("after second release closes=%d len=%d, want 1/0", closes, cache.Len())
	}
	if err := second.Release(close); err != nil {
		t.Fatalf("Release(second again) error = %v", err)
	}
	if closes != 1 {
		t.Fatalf("double release closes = %d, want 1", closes)
	}
}

func TestVolumeSessionCacheDoesNotShareDifferentSession(t *testing.T) {
	cache := NewVolumeSessionCache()
	keyA := testVolumeSessionKey()
	keyB := keyA
	keyB.SessionID = "session-b"
	var opens int

	open := func(context.Context) (VolumeSession, error) {
		opens++
		return VolumeSession{
			VolumeID:     keyA.VolumeID,
			VolumeHandle: "handle-" + strconv.Itoa(opens),
		}, nil
	}

	first, err := cache.Acquire(t.Context(), keyA, open)
	if err != nil {
		t.Fatalf("Acquire(first) error = %v", err)
	}
	second, err := cache.Acquire(t.Context(), keyB, func(ctx context.Context) (VolumeSession, error) {
		session, err := open(ctx)
		session.VolumeID = keyB.VolumeID
		return session, err
	})
	if err != nil {
		t.Fatalf("Acquire(second) error = %v", err)
	}
	if opens != 2 {
		t.Fatalf("opens = %d, want 2", opens)
	}
	if first.Session().VolumeHandle == second.Session().VolumeHandle {
		t.Fatalf("handles are shared for different sessions: %q", first.Session().VolumeHandle)
	}
}

func TestVolumeSessionCacheDoesNotStoreFailedOpen(t *testing.T) {
	cache := NewVolumeSessionCache()
	key := testVolumeSessionKey()
	openErr := errors.New("open failed")
	if _, err := cache.Acquire(t.Context(), key, func(context.Context) (VolumeSession, error) {
		return VolumeSession{}, openErr
	}); !errors.Is(err, openErr) {
		t.Fatalf("Acquire(error) = %v, want %v", err, openErr)
	}
	if cache.Len() != 0 {
		t.Fatalf("cache Len after failed open = %d, want 0", cache.Len())
	}
}

func TestVolumeSessionKeyValidate(t *testing.T) {
	key := testVolumeSessionKey()
	key.VolumeID = " "
	err := key.Validate()
	if !errors.Is(err, storage.ErrInvalidArgument) || !strings.Contains(err.Error(), "volume id") {
		t.Fatalf("Validate() error = %v, want volume id invalid argument", err)
	}
}

func testVolumeSessionKey() VolumeSessionKey {
	return VolumeSessionKey{
		DataEndpoint:      "passthrough:///sbs-data",
		VolumeID:          "18a30001",
		AccessMode:        "exclusive-writer",
		GatewayID:         "gw-a",
		AttachmentID:      "att-a",
		Generation:        3,
		PoolID:            "object-pool",
		PoolGeneration:    4,
		MemberGeneration:  5,
		VolumeEpoch:       6,
		WriterGroupID:     "object-writers",
		GatewayInstanceID: "gateway-instance-a",
		SessionID:         "session-a",
		SessionGeneration: 7,
	}
}

package sbs

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/storage"
)

func TestSessionFenceAllowsCurrentSession(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	identity := testSessionFenceIdentity()
	fence := SessionFenceSnapshot{
		VolumeID:             identity.VolumeID,
		WriterGroupID:        identity.WriterGroupID,
		VolumeEpoch:          identity.VolumeEpoch,
		MinSessionGeneration: identity.SessionGeneration,
		ExpiresAt:            now.Add(time.Minute),
	}
	if err := fence.Validate(identity, now); err != nil {
		t.Fatalf("Validate(current) error = %v", err)
	}
}

func TestSessionFenceRejectsLowerGeneration(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	identity := testSessionFenceIdentity()
	fence := SessionFenceSnapshot{
		VolumeID:             identity.VolumeID,
		WriterGroupID:        identity.WriterGroupID,
		VolumeEpoch:          identity.VolumeEpoch,
		MinSessionGeneration: identity.SessionGeneration + 1,
		ExpiresAt:            now.Add(time.Minute),
	}
	err := fence.Validate(identity, now)
	assertSessionFenceError(t, err, SessionFenceRejectStaleGeneration)
}

func TestSessionFenceRejectsExpiredSession(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	identity := testSessionFenceIdentity()
	fence := SessionFenceSnapshot{
		VolumeID:             identity.VolumeID,
		WriterGroupID:        identity.WriterGroupID,
		VolumeEpoch:          identity.VolumeEpoch,
		MinSessionGeneration: identity.SessionGeneration,
		ExpiresAt:            now,
	}
	err := fence.Validate(identity, now)
	assertSessionFenceError(t, err, SessionFenceRejectExpired)
}

func TestSessionFenceRejectsStaleVolumeEpoch(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC)
	identity := testSessionFenceIdentity()
	fence := SessionFenceSnapshot{
		VolumeID:      identity.VolumeID,
		WriterGroupID: identity.WriterGroupID,
		VolumeEpoch:   identity.VolumeEpoch + 1,
	}
	err := fence.Validate(identity, now)
	assertSessionFenceError(t, err, SessionFenceRejectStaleVolumeEpoch)
}

func TestSessionFenceRejectsMissingIdentity(t *testing.T) {
	fence := SessionFenceSnapshot{VolumeID: "18a30001"}
	err := fence.Validate(SessionIdentity{}, time.Date(2026, 8, 10, 1, 2, 3, 0, time.UTC))
	assertSessionFenceError(t, err, SessionFenceRejectMissingIdentity)
}

func assertSessionFenceError(t *testing.T, err error, reason SessionFenceRejectReason) {
	t.Helper()
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("fence error = %v, want ErrUnavailable", err)
	}
	var fenceErr *SessionFenceError
	if !errors.As(err, &fenceErr) {
		t.Fatalf("fence error type = %T, want *SessionFenceError", err)
	}
	if fenceErr.Decision.Reason != reason {
		t.Fatalf("fence reason = %q, want %q", fenceErr.Decision.Reason, reason)
	}
	if !strings.Contains(err.Error(), string(reason)) {
		t.Fatalf("fence error = %q, want reason text", err)
	}
}

func testSessionFenceIdentity() SessionIdentity {
	return SessionIdentity{
		VolumeID:          "18a30001",
		VolumeEpoch:       4,
		WriterGroupID:     "object-writers",
		GatewayID:         "gw-a",
		SessionID:         "gw-a-boot-1",
		SessionGeneration: 7,
		SessionTTL:        30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}
}

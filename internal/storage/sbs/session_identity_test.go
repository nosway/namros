package sbs

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/storage"
)

func TestNormalizeSessionIdentityBuildsFencingScope(t *testing.T) {
	identity, err := NormalizeSessionIdentity(SessionIdentity{
		PoolID:            "object-pool",
		PoolGeneration:    4,
		VolumeID:          "18a30001",
		MemberGeneration:  5,
		VolumeEpoch:       6,
		WriterGroupID:     "object-writers",
		GatewayID:         "gw-a",
		GatewayInstanceID: "gateway-instance-a",
		SessionID:         "gw-a-boot-1",
		SessionGeneration: 7,
		SessionTTL:        45 * time.Second,
		HeartbeatInterval: 15 * time.Second,
	}, SessionIdentityDefaults{})
	if err != nil {
		t.Fatalf("NormalizeSessionIdentity() error = %v", err)
	}
	scope := identity.IdempotencyScope()
	for _, want := range []string{
		"pool=object-pool",
		"pool_generation=4",
		"volume=18a30001",
		"member_generation=5",
		"volume_epoch=6",
		"writer_group=object-writers",
		"gateway=gw-a",
		"gateway_instance=gateway-instance-a",
		"session=gw-a-boot-1",
		"session_generation=7",
	} {
		if !strings.Contains(scope, want) {
			t.Fatalf("scope %q does not contain %q", scope, want)
		}
	}
	if got := identity.ScopedIdempotencyKey("legacy-key"); !strings.Contains(got, "legacy-key|session:") || !strings.Contains(got, "session=gw-a-boot-1") {
		t.Fatalf("ScopedIdempotencyKey() = %q", got)
	}
}

func TestNormalizeSessionIdentityUsesDefaultsWhenEnabled(t *testing.T) {
	identity, err := NormalizeSessionIdentity(SessionIdentity{WriterGroupID: "object-writers", SessionID: "gw-a-session"}, SessionIdentityDefaults{
		VolumeID:          "18a30001",
		MemberGeneration:  11,
		GatewayID:         "gw-a",
		GatewayInstanceID: "gateway-instance-a",
		SessionTTL:        30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NormalizeSessionIdentity() error = %v", err)
	}
	if identity.VolumeEpoch != defaultVolumeEpoch || identity.SessionGeneration != defaultSessionGeneration {
		t.Fatalf("identity defaults = epoch:%d generation:%d", identity.VolumeEpoch, identity.SessionGeneration)
	}
	if identity.MemberGeneration != 11 || identity.GatewayID != "gw-a" {
		t.Fatalf("identity = %+v", identity)
	}
}

func TestNormalizeSessionIdentityRequiresWriterGroupAndSession(t *testing.T) {
	_, err := NormalizeSessionIdentity(SessionIdentity{WriterGroupID: "object-writers"}, SessionIdentityDefaults{
		VolumeID:          "18a30001",
		GatewayID:         "gw-a",
		SessionTTL:        30 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	})
	if !errors.Is(err, storage.ErrInvalidArgument) || !strings.Contains(err.Error(), "sbs session id is required") {
		t.Fatalf("NormalizeSessionIdentity() error = %v, want session id invalid argument", err)
	}
}

func TestNormalizeSessionIdentityRejectsSlowHeartbeat(t *testing.T) {
	_, err := NormalizeSessionIdentity(SessionIdentity{
		VolumeID:          "18a30001",
		WriterGroupID:     "object-writers",
		GatewayID:         "gw-a",
		SessionID:         "gw-a-session",
		SessionTTL:        10 * time.Second,
		HeartbeatInterval: 10 * time.Second,
	}, SessionIdentityDefaults{})
	if !errors.Is(err, storage.ErrInvalidArgument) || !strings.Contains(err.Error(), "heartbeat must be shorter") {
		t.Fatalf("NormalizeSessionIdentity() error = %v, want heartbeat validation", err)
	}
}

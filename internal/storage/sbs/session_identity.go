package sbs

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/storage"
)

const (
	DefaultSessionTTL        = 30 * time.Second
	DefaultSessionHeartbeat  = 10 * time.Second
	defaultVolumeEpoch       = uint64(1)
	defaultSessionGeneration = uint64(1)
)

type SessionIdentity struct {
	PoolID            string
	PoolGeneration    uint64
	VolumeID          string
	MemberGeneration  uint64
	VolumeEpoch       uint64
	WriterGroupID     string
	GatewayID         string
	GatewayInstanceID string
	SessionID         string
	SessionGeneration uint64
	SessionTTL        time.Duration
	HeartbeatInterval time.Duration
}

type SessionIdentityDefaults struct {
	PoolID            string
	PoolGeneration    uint64
	VolumeID          string
	MemberGeneration  uint64
	VolumeEpoch       uint64
	WriterGroupID     string
	GatewayID         string
	GatewayInstanceID string
	SessionID         string
	SessionGeneration uint64
	SessionTTL        time.Duration
	HeartbeatInterval time.Duration
}

func (id SessionIdentity) Enabled() bool {
	return strings.TrimSpace(id.WriterGroupID) != "" ||
		strings.TrimSpace(id.SessionID) != "" ||
		id.VolumeEpoch != 0 ||
		id.SessionGeneration != 0
}

func NormalizeSessionIdentity(id SessionIdentity, defaults SessionIdentityDefaults) (SessionIdentity, error) {
	id = trimSessionIdentity(id)
	defaults = trimSessionDefaults(defaults)
	if id.PoolID == "" {
		id.PoolID = defaults.PoolID
	}
	if id.PoolGeneration == 0 {
		id.PoolGeneration = defaults.PoolGeneration
	}
	if id.VolumeID == "" {
		id.VolumeID = defaults.VolumeID
	}
	if id.MemberGeneration == 0 {
		id.MemberGeneration = defaults.MemberGeneration
	}
	if id.VolumeEpoch == 0 {
		id.VolumeEpoch = defaults.VolumeEpoch
	}
	if id.WriterGroupID == "" {
		id.WriterGroupID = defaults.WriterGroupID
	}
	if id.GatewayID == "" {
		id.GatewayID = defaults.GatewayID
	}
	if id.GatewayInstanceID == "" {
		id.GatewayInstanceID = defaults.GatewayInstanceID
	}
	if id.SessionID == "" {
		id.SessionID = defaults.SessionID
	}
	if id.SessionGeneration == 0 {
		id.SessionGeneration = defaults.SessionGeneration
	}
	if id.SessionTTL == 0 {
		id.SessionTTL = defaults.SessionTTL
	}
	if id.HeartbeatInterval == 0 {
		id.HeartbeatInterval = defaults.HeartbeatInterval
	}
	if !id.Enabled() {
		return SessionIdentity{}, nil
	}
	if id.VolumeEpoch == 0 {
		id.VolumeEpoch = defaultVolumeEpoch
	}
	if id.SessionGeneration == 0 {
		id.SessionGeneration = defaultSessionGeneration
	}
	if id.SessionTTL == 0 {
		id.SessionTTL = DefaultSessionTTL
	}
	if id.HeartbeatInterval == 0 {
		id.HeartbeatInterval = DefaultSessionHeartbeat
	}
	if err := id.Validate(); err != nil {
		return SessionIdentity{}, err
	}
	return id, nil
}

func (id SessionIdentity) Validate() error {
	id = trimSessionIdentity(id)
	if !id.Enabled() {
		return nil
	}
	if id.VolumeID == "" {
		return fmt.Errorf("%w: sbs session identity volume id is required", storage.ErrInvalidArgument)
	}
	if id.WriterGroupID == "" {
		return fmt.Errorf("%w: sbs writer group id is required when session identity is enabled", storage.ErrInvalidArgument)
	}
	if id.GatewayID == "" {
		return fmt.Errorf("%w: sbs gateway id is required when session identity is enabled", storage.ErrInvalidArgument)
	}
	if id.SessionID == "" {
		return fmt.Errorf("%w: sbs session id is required when session identity is enabled", storage.ErrInvalidArgument)
	}
	if id.VolumeEpoch == 0 {
		return fmt.Errorf("%w: sbs volume epoch is required when session identity is enabled", storage.ErrInvalidArgument)
	}
	if id.SessionGeneration == 0 {
		return fmt.Errorf("%w: sbs session generation is required when session identity is enabled", storage.ErrInvalidArgument)
	}
	if id.SessionTTL <= 0 {
		return fmt.Errorf("%w: sbs session ttl must be positive", storage.ErrInvalidArgument)
	}
	if id.HeartbeatInterval <= 0 {
		return fmt.Errorf("%w: sbs session heartbeat must be positive", storage.ErrInvalidArgument)
	}
	if id.HeartbeatInterval >= id.SessionTTL {
		return fmt.Errorf("%w: sbs session heartbeat must be shorter than session ttl", storage.ErrInvalidArgument)
	}
	return nil
}

func (id SessionIdentity) IdempotencyScope() string {
	id = trimSessionIdentity(id)
	if !id.Enabled() {
		return ""
	}
	parts := []string{
		"scope=v1",
		"pool=" + escapeSessionScopePart(id.PoolID),
		fmt.Sprintf("pool_generation=%d", id.PoolGeneration),
		"volume=" + escapeSessionScopePart(id.VolumeID),
		fmt.Sprintf("member_generation=%d", id.MemberGeneration),
		fmt.Sprintf("volume_epoch=%d", id.VolumeEpoch),
		"writer_group=" + escapeSessionScopePart(id.WriterGroupID),
		"gateway=" + escapeSessionScopePart(id.GatewayID),
		"gateway_instance=" + escapeSessionScopePart(id.GatewayInstanceID),
		"session=" + escapeSessionScopePart(id.SessionID),
		fmt.Sprintf("session_generation=%d", id.SessionGeneration),
	}
	return strings.Join(parts, "|")
}

func (id SessionIdentity) ScopedIdempotencyKey(legacy string) string {
	if scope := id.IdempotencyScope(); scope != "" {
		return legacy + "|session:" + scope
	}
	return legacy
}

type VolumeSessionKey struct {
	DataEndpoint      string
	VolumeID          string
	AccessMode        string
	GatewayID         string
	AttachmentID      string
	Generation        uint64
	PoolID            string
	PoolGeneration    uint64
	MemberGeneration  uint64
	VolumeEpoch       uint64
	WriterGroupID     string
	GatewayInstanceID string
	SessionID         string
	SessionGeneration uint64
}

func (id SessionIdentity) VolumeSessionKey(dataEndpoint, accessMode, volumeID, gatewayID, attachmentID string, generation uint64) VolumeSessionKey {
	id = trimSessionIdentity(id)
	return VolumeSessionKey{
		DataEndpoint:      strings.TrimSpace(dataEndpoint),
		VolumeID:          strings.TrimSpace(volumeID),
		AccessMode:        strings.TrimSpace(accessMode),
		GatewayID:         strings.TrimSpace(gatewayID),
		AttachmentID:      strings.TrimSpace(attachmentID),
		Generation:        generation,
		PoolID:            id.PoolID,
		PoolGeneration:    id.PoolGeneration,
		MemberGeneration:  id.MemberGeneration,
		VolumeEpoch:       id.VolumeEpoch,
		WriterGroupID:     id.WriterGroupID,
		GatewayInstanceID: id.GatewayInstanceID,
		SessionID:         id.SessionID,
		SessionGeneration: id.SessionGeneration,
	}
}

func trimSessionIdentity(id SessionIdentity) SessionIdentity {
	id.PoolID = strings.TrimSpace(id.PoolID)
	id.VolumeID = strings.TrimSpace(id.VolumeID)
	id.WriterGroupID = strings.TrimSpace(id.WriterGroupID)
	id.GatewayID = strings.TrimSpace(id.GatewayID)
	id.GatewayInstanceID = strings.TrimSpace(id.GatewayInstanceID)
	id.SessionID = strings.TrimSpace(id.SessionID)
	return id
}

func trimSessionDefaults(defaults SessionIdentityDefaults) SessionIdentityDefaults {
	defaults.PoolID = strings.TrimSpace(defaults.PoolID)
	defaults.VolumeID = strings.TrimSpace(defaults.VolumeID)
	defaults.WriterGroupID = strings.TrimSpace(defaults.WriterGroupID)
	defaults.GatewayID = strings.TrimSpace(defaults.GatewayID)
	defaults.GatewayInstanceID = strings.TrimSpace(defaults.GatewayInstanceID)
	defaults.SessionID = strings.TrimSpace(defaults.SessionID)
	return defaults
}

func escapeSessionScopePart(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "=", "\\=")
	return value
}

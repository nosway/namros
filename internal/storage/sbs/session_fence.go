package sbs

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/storage"
)

type SessionFenceRejectReason string

const (
	SessionFenceRejectMissingIdentity     SessionFenceRejectReason = "missing_session_identity"
	SessionFenceRejectVolumeMismatch      SessionFenceRejectReason = "volume_mismatch"
	SessionFenceRejectWriterGroupMismatch SessionFenceRejectReason = "writer_group_mismatch"
	SessionFenceRejectStaleVolumeEpoch    SessionFenceRejectReason = "stale_volume_epoch"
	SessionFenceRejectStaleGeneration     SessionFenceRejectReason = "stale_session_generation"
	SessionFenceRejectExpired             SessionFenceRejectReason = "session_expired"
)

type SessionFenceSnapshot struct {
	VolumeID             string
	WriterGroupID        string
	VolumeEpoch          uint64
	MinSessionGeneration uint64
	ExpiresAt            time.Time
}

type SessionFenceDecision struct {
	Admitted bool
	Reason   SessionFenceRejectReason
	Detail   string
}

type SessionFenceError struct {
	Decision SessionFenceDecision
}

func (e *SessionFenceError) Error() string {
	if e == nil {
		return ""
	}
	if e.Decision.Detail != "" {
		return fmt.Sprintf("%s: sbs session fenced: %s: %s", storage.ErrUnavailable, e.Decision.Reason, e.Decision.Detail)
	}
	return fmt.Sprintf("%s: sbs session fenced: %s", storage.ErrUnavailable, e.Decision.Reason)
}

func (e *SessionFenceError) Unwrap() error {
	return storage.ErrUnavailable
}

func (f SessionFenceSnapshot) Enabled() bool {
	f = normalizeSessionFence(f)
	return f.VolumeID != "" ||
		f.WriterGroupID != "" ||
		f.VolumeEpoch != 0 ||
		f.MinSessionGeneration != 0 ||
		!f.ExpiresAt.IsZero()
}

func (f SessionFenceSnapshot) Validate(identity SessionIdentity, now time.Time) error {
	decision := f.Decide(identity, now)
	if decision.Admitted {
		return nil
	}
	return &SessionFenceError{Decision: decision}
}

func (f SessionFenceSnapshot) Decide(identity SessionIdentity, now time.Time) SessionFenceDecision {
	f = normalizeSessionFence(f)
	identity = trimSessionIdentity(identity)
	if !f.Enabled() {
		return SessionFenceDecision{Admitted: true}
	}
	if !identity.Enabled() {
		return rejectSessionFence(SessionFenceRejectMissingIdentity, "session identity is required")
	}
	if f.VolumeID != "" && identity.VolumeID != f.VolumeID {
		return rejectSessionFence(SessionFenceRejectVolumeMismatch, fmt.Sprintf("volume %q is not current volume %q", identity.VolumeID, f.VolumeID))
	}
	if f.WriterGroupID != "" && identity.WriterGroupID != f.WriterGroupID {
		return rejectSessionFence(SessionFenceRejectWriterGroupMismatch, fmt.Sprintf("writer group %q is not current writer group %q", identity.WriterGroupID, f.WriterGroupID))
	}
	if f.VolumeEpoch != 0 && identity.VolumeEpoch != f.VolumeEpoch {
		return rejectSessionFence(SessionFenceRejectStaleVolumeEpoch, fmt.Sprintf("volume epoch %d is not current epoch %d", identity.VolumeEpoch, f.VolumeEpoch))
	}
	if f.MinSessionGeneration != 0 && identity.SessionGeneration < f.MinSessionGeneration {
		return rejectSessionFence(SessionFenceRejectStaleGeneration, fmt.Sprintf("session generation %d is lower than current generation %d", identity.SessionGeneration, f.MinSessionGeneration))
	}
	if !f.ExpiresAt.IsZero() {
		if now.IsZero() {
			now = time.Now().UTC()
		}
		if !now.Before(f.ExpiresAt) {
			return rejectSessionFence(SessionFenceRejectExpired, fmt.Sprintf("session expired at %s", f.ExpiresAt.UTC().Format(time.RFC3339Nano)))
		}
	}
	return SessionFenceDecision{Admitted: true}
}

func validateSessionFence(identity SessionIdentity, fence SessionFenceSnapshot, now func() time.Time) error {
	if !fence.Enabled() {
		return nil
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	return fence.Validate(identity, now())
}

func rejectSessionFence(reason SessionFenceRejectReason, detail string) SessionFenceDecision {
	return SessionFenceDecision{
		Admitted: false,
		Reason:   reason,
		Detail:   detail,
	}
}

func normalizeSessionFence(f SessionFenceSnapshot) SessionFenceSnapshot {
	f.VolumeID = strings.TrimSpace(f.VolumeID)
	f.WriterGroupID = strings.TrimSpace(f.WriterGroupID)
	if !f.ExpiresAt.IsZero() {
		f.ExpiresAt = f.ExpiresAt.UTC()
	}
	return f
}

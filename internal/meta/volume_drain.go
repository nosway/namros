package meta

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func BuildVolumeDrainOperation(operationID string, req PutVolumeDrainOperationRequest, now time.Time) (model.VolumeDrainOperationRecord, error) {
	sourceVolumeID := strings.TrimSpace(req.SourceVolumeID)
	if sourceVolumeID == "" {
		return model.VolumeDrainOperationRecord{}, fmt.Errorf("%w: source volume id is required", ErrInvalidArgument)
	}
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return model.VolumeDrainOperationRecord{}, fmt.Errorf("%w: volume drain operation id is required", ErrInvalidArgument)
	}
	return model.VolumeDrainOperationRecord{
		OperationID:         operationID,
		ResumeOfOperationID: strings.TrimSpace(req.ResumeOfOperationID),
		PoolID:              strings.TrimSpace(req.PoolID),
		SourceVolumeID:      sourceVolumeID,
		TargetVolumeID:      strings.TrimSpace(req.TargetVolumeID),
		OwnerID:             strings.TrimSpace(req.OwnerID),
		Status:              NormalizeVolumeDrainOperationStatus(req.Status, req.Retryable),
		Cursor:              strings.TrimSpace(req.Cursor),
		StartedAt:           req.StartedAt.UTC(),
		FinishedAt:          req.FinishedAt.UTC(),
		Scanned:             req.Scanned,
		Copied:              req.Copied,
		Skipped:             req.Skipped,
		Protected:           req.Protected,
		Retryable:           req.Retryable,
		Attempts:            CloneVolumeDrainAttempts(req.Attempts),
		CreatedAt:           now.UTC(),
	}, nil
}

func NormalizeVolumeDrainOperationStatus(status model.VolumeDrainOperationStatus, retryable int) model.VolumeDrainOperationStatus {
	switch status {
	case model.VolumeDrainOperationRunning,
		model.VolumeDrainOperationSucceeded,
		model.VolumeDrainOperationRetryPending,
		model.VolumeDrainOperationFailed,
		model.VolumeDrainOperationCanceled:
		return status
	case "":
		if retryable > 0 {
			return model.VolumeDrainOperationRetryPending
		}
		return model.VolumeDrainOperationSucceeded
	default:
		return model.VolumeDrainOperationFailed
	}
}

func CloneVolumeDrainOperationRecord(in model.VolumeDrainOperationRecord) model.VolumeDrainOperationRecord {
	out := in
	out.Attempts = CloneVolumeDrainAttempts(in.Attempts)
	return out
}

func CloneVolumeDrainAttempts(in []model.VolumeDrainAttempt) []model.VolumeDrainAttempt {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.VolumeDrainAttempt, len(in))
	for i, attempt := range in {
		out[i] = attempt
		out[i].SourceSegmentID = strings.TrimSpace(out[i].SourceSegmentID)
		out[i].TargetSegmentID = strings.TrimSpace(out[i].TargetSegmentID)
		out[i].SourceRef = storage.CloneSegmentRef(attempt.SourceRef)
		out[i].TargetRef = storage.CloneSegmentRef(attempt.TargetRef)
		if out[i].SourceSegmentID == "" {
			out[i].SourceSegmentID = out[i].SourceRef.SegmentID
		}
		if out[i].TargetSegmentID == "" {
			out[i].TargetSegmentID = out[i].TargetRef.SegmentID
		}
	}
	return out
}

func SegmentRefVolumeID(ref storage.SegmentRef) string {
	if ref.Placement.Parameters != nil {
		if volumeID := strings.TrimSpace(ref.Placement.Parameters["volume_id"]); volumeID != "" {
			return volumeID
		}
	}
	for _, chunk := range ref.Placement.Chunks {
		if volumeID := strings.TrimSpace(chunk.VolumeID); volumeID != "" {
			return volumeID
		}
	}
	return ""
}

func SegmentRefsContainVolume(refs []storage.SegmentRef, volumeID string) bool {
	volumeID = strings.TrimSpace(volumeID)
	if volumeID == "" {
		return true
	}
	for _, ref := range refs {
		if SegmentRefVolumeID(ref) == volumeID {
			return true
		}
	}
	return false
}

package meta

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

func BuildMetadataMigrationOperation(operationID string, req PutMetadataMigrationOperationRequest, now time.Time) (model.MetadataMigrationOperationRecord, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return model.MetadataMigrationOperationRecord{}, fmt.Errorf("%w: metadata migration operation id is required", ErrInvalidArgument)
	}
	targetSchemaVersion := req.TargetSchemaVersion
	if targetSchemaVersion <= 0 {
		targetSchemaVersion = CurrentMetadataSchemaVersion
	}
	return model.MetadataMigrationOperationRecord{
		OperationID:         operationID,
		ResumeOfOperationID: strings.TrimSpace(req.ResumeOfOperationID),
		TargetSchemaVersion: targetSchemaVersion,
		Status:              NormalizeMetadataMigrationOperationStatus(req.Status, req.DryRun, req.Apply),
		DryRun:              req.DryRun,
		Apply:               req.Apply,
		OwnerID:             strings.TrimSpace(req.OwnerID),
		Cursor:              strings.TrimSpace(req.Cursor),
		Steps:               CloneMetadataMigrationSteps(req.Steps),
		StartedAt:           req.StartedAt.UTC(),
		FinishedAt:          req.FinishedAt.UTC(),
		CreatedAt:           now.UTC(),
	}, nil
}

func NormalizeMetadataMigrationOperationStatus(status model.MetadataMigrationOperationStatus, dryRun, apply bool) model.MetadataMigrationOperationStatus {
	switch status {
	case model.MetadataMigrationOperationPlanned,
		model.MetadataMigrationOperationRunning,
		model.MetadataMigrationOperationSucceeded,
		model.MetadataMigrationOperationRetryPending,
		model.MetadataMigrationOperationFailed,
		model.MetadataMigrationOperationCanceled:
		return status
	case "":
		if dryRun && !apply {
			return model.MetadataMigrationOperationPlanned
		}
		return model.MetadataMigrationOperationSucceeded
	default:
		return model.MetadataMigrationOperationFailed
	}
}

func CloneMetadataMigrationOperationRecord(in model.MetadataMigrationOperationRecord) model.MetadataMigrationOperationRecord {
	out := in
	out.Steps = CloneMetadataMigrationSteps(in.Steps)
	return out
}

func CloneMetadataMigrationSteps(in []model.MetadataMigrationStep) []model.MetadataMigrationStep {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.MetadataMigrationStep, len(in))
	copy(out, in)
	for i := range out {
		out[i].Name = strings.TrimSpace(out[i].Name)
		out[i].Message = strings.TrimSpace(out[i].Message)
	}
	return out
}

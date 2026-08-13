package meta

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

func BuildWorkerLease(existing model.WorkerLease, req AcquireWorkerLeaseRequest, now time.Time) (model.WorkerLease, error) {
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if workerKind == "" || shardID == "" || ownerID == "" {
		return model.WorkerLease{}, fmt.Errorf("%w: worker kind, shard id, and owner id are required", ErrInvalidArgument)
	}
	if req.TTL <= 0 {
		return model.WorkerLease{}, fmt.Errorf("%w: worker lease ttl must be positive", ErrInvalidArgument)
	}
	now = now.UTC()
	if existing.LeaseID != "" && existing.OwnerID != ownerID && existing.ExpiresAt.After(now) {
		return model.WorkerLease{}, fmt.Errorf("%w: worker lease %q is held by %q until %s", ErrCASConflict, existing.LeaseID, existing.OwnerID, existing.ExpiresAt.Format(time.RFC3339Nano))
	}
	generation := existing.Generation
	acquiredAt := existing.AcquiredAt
	if existing.LeaseID == "" || existing.OwnerID != ownerID {
		generation++
		if generation == 0 {
			generation = 1
		}
		acquiredAt = now
	}
	leaseID := WorkerLeaseID(workerKind, shardID)
	return model.WorkerLease{
		LeaseID:    leaseID,
		WorkerKind: workerKind,
		ShardID:    shardID,
		OwnerID:    ownerID,
		Generation: generation,
		Cursor:     strings.TrimSpace(req.Cursor),
		AcquiredAt: acquiredAt,
		UpdatedAt:  now,
		ExpiresAt:  now.Add(req.TTL).UTC(),
	}, nil
}

func WorkerLeaseID(workerKind, shardID string) string {
	return strings.TrimSpace(workerKind) + "/" + strings.TrimSpace(shardID)
}

func WorkerControlID(workerKind, shardID string) string {
	return WorkerLeaseID(workerKind, shardID)
}

func NormalizeWorkerOperationStatus(status model.WorkerOperationStatus, retryable int) model.WorkerOperationStatus {
	switch status {
	case model.WorkerOperationRunning, model.WorkerOperationSucceeded, model.WorkerOperationRetryPending, model.WorkerOperationFailed, model.WorkerOperationCanceled, model.WorkerOperationPaused:
		return status
	}
	if retryable > 0 {
		return model.WorkerOperationRetryPending
	}
	return model.WorkerOperationSucceeded
}

func BuildWorkerControl(existing model.WorkerControlRecord, req PutWorkerControlRequest, now time.Time) (model.WorkerControlRecord, error) {
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	if workerKind == "" || shardID == "" {
		return model.WorkerControlRecord{}, fmt.Errorf("%w: worker kind and shard id are required", ErrInvalidArgument)
	}
	state := NormalizeWorkerControlState(req.State)
	if state == "" {
		return model.WorkerControlRecord{}, fmt.Errorf("%w: unsupported worker control state %q", ErrInvalidArgument, req.State)
	}
	now = now.UTC()
	createdAt := existing.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	return model.WorkerControlRecord{
		WorkerKind: workerKind,
		ShardID:    shardID,
		State:      state,
		Reason:     strings.TrimSpace(req.Reason),
		UpdatedBy:  strings.TrimSpace(req.UpdatedBy),
		UpdatedAt:  now,
		CreatedAt:  createdAt.UTC(),
	}, nil
}

func NormalizeWorkerControlState(state model.WorkerControlState) model.WorkerControlState {
	switch model.WorkerControlState(strings.ToLower(strings.TrimSpace(string(state)))) {
	case "", model.WorkerControlActive:
		return model.WorkerControlActive
	case model.WorkerControlPaused:
		return model.WorkerControlPaused
	case model.WorkerControlCanceled:
		return model.WorkerControlCanceled
	default:
		return ""
	}
}

func CloneWorkerLease(in model.WorkerLease) model.WorkerLease {
	return in
}

func CloneWorkerOperationRecord(in model.WorkerOperationRecord) model.WorkerOperationRecord {
	return in
}

func CloneWorkerControlRecord(in model.WorkerControlRecord) model.WorkerControlRecord {
	return in
}

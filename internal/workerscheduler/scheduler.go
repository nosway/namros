package workerscheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
)

type Runner struct {
	Repository  meta.Repository
	WorkerKind  string
	ShardID     string
	OwnerID     string
	TTL         time.Duration
	Clock       func() time.Time
	HoldLease   bool
	Budget      *Budget
	BudgetScope BudgetScope
}

type Result struct {
	Status    model.WorkerOperationStatus
	Cursor    string
	Scanned   int
	Processed int
	Skipped   int
	Retryable int
	LastError string
}

type WorkFunc func(context.Context, model.WorkerLease) (Result, error)

func (r Runner) RunOnce(ctx context.Context, cursor string, work WorkFunc) (model.WorkerOperationRecord, error) {
	if r.Repository == nil {
		return model.WorkerOperationRecord{}, fmt.Errorf("%w: worker scheduler repository is required", meta.ErrInvalidArgument)
	}
	if work == nil {
		return model.WorkerOperationRecord{}, fmt.Errorf("%w: worker scheduler work func is required", meta.ErrInvalidArgument)
	}
	if err := r.checkControl(ctx); err != nil {
		return model.WorkerOperationRecord{}, err
	}
	releaseBudget, reason, ok := r.Budget.TryAcquire(r.budgetScope())
	if !ok {
		return model.WorkerOperationRecord{}, throttleError(reason)
	}
	defer releaseBudget()
	ttl := r.TTL
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	now := r.now()
	lease, err := r.Repository.AcquireWorkerLease(ctx, meta.AcquireWorkerLeaseRequest{
		WorkerKind: r.WorkerKind,
		ShardID:    r.ShardID,
		OwnerID:    r.OwnerID,
		TTL:        ttl,
		Cursor:     cursor,
	})
	if err != nil {
		return model.WorkerOperationRecord{}, err
	}
	release := true
	defer func() {
		if release && !r.HoldLease {
			_ = r.Repository.ReleaseWorkerLease(context.Background(), meta.ReleaseWorkerLeaseRequest{
				WorkerKind: r.WorkerKind,
				ShardID:    r.ShardID,
				OwnerID:    r.OwnerID,
			})
		}
	}()

	result, workErr := work(ctx, lease)
	status := result.Status
	if status == "" {
		status = model.WorkerOperationSucceeded
		if workErr != nil {
			status = model.WorkerOperationFailed
			if result.Retryable > 0 {
				status = model.WorkerOperationRetryPending
			}
		}
	}
	lastError := result.LastError
	if lastError == "" && workErr != nil {
		lastError = workErr.Error()
	}
	record, putErr := r.Repository.PutWorkerOperation(ctx, meta.PutWorkerOperationRequest{
		WorkerKind: r.WorkerKind,
		ShardID:    r.ShardID,
		OwnerID:    r.OwnerID,
		LeaseID:    lease.LeaseID,
		Status:     status,
		Cursor:     result.Cursor,
		Scanned:    result.Scanned,
		Processed:  result.Processed,
		Skipped:    result.Skipped,
		Retryable:  result.Retryable,
		LastError:  lastError,
		StartedAt:  now,
		FinishedAt: r.now(),
	})
	if putErr != nil {
		return model.WorkerOperationRecord{}, putErr
	}
	if workErr != nil {
		return record, workErr
	}
	return record, nil
}

func (r Runner) budgetScope() BudgetScope {
	scope := r.BudgetScope
	if scope.WorkerKind == "" {
		scope.WorkerKind = r.WorkerKind
	}
	if scope.ShardID == "" {
		scope.ShardID = r.ShardID
	}
	if scope.OwnerID == "" {
		scope.OwnerID = r.OwnerID
	}
	return scope
}

func (r Runner) checkControl(ctx context.Context) error {
	control, err := r.Repository.GetWorkerControl(ctx, meta.GetWorkerControlRequest{
		WorkerKind: r.WorkerKind,
		ShardID:    r.ShardID,
	})
	if errors.Is(err, meta.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	switch meta.NormalizeWorkerControlState(control.State) {
	case model.WorkerControlPaused:
		return fmt.Errorf("%w: worker %s/%s is paused", meta.ErrWorkerPaused, r.WorkerKind, r.ShardID)
	case model.WorkerControlCanceled:
		return fmt.Errorf("%w: worker %s/%s is canceled", meta.ErrWorkerCanceled, r.WorkerKind, r.ShardID)
	default:
		return nil
	}
}

func (r Runner) now() time.Time {
	if r.Clock != nil {
		return r.Clock().UTC()
	}
	return time.Now().UTC()
}

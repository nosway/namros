package workerops

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
)

const (
	DefaultOperationLimit = 1000
)

type Config struct {
	OperationLimit int
	LeaseLimit     int
	Now            func() time.Time
}

type BacklogSnapshot struct {
	SchemaVersion    string          `json:"schema_version"`
	GeneratedAt      string          `json:"generated_at"`
	Status           string          `json:"status"`
	OperationLimit   int             `json:"operation_limit"`
	LeaseLimit       int             `json:"lease_limit"`
	OperationCount   int             `json:"operation_count"`
	LeaseCount       int             `json:"lease_count"`
	OperationLimited bool            `json:"operation_limited,omitempty"`
	LeaseLimited     bool            `json:"lease_limited,omitempty"`
	Totals           BacklogTotals   `json:"totals"`
	Workers          []WorkerSummary `json:"workers,omitempty"`
}

type BacklogTotals struct {
	BacklogOperations      int `json:"backlog_operations"`
	RunningOperations      int `json:"running_operations"`
	RetryPendingOperations int `json:"retry_pending_operations"`
	FailedOperations       int `json:"failed_operations"`
	SucceededOperations    int `json:"succeeded_operations"`
	CanceledOperations     int `json:"canceled_operations"`
	PausedOperations       int `json:"paused_operations"`
	Retryable              int `json:"retryable"`
	LeasesFresh            int `json:"leases_fresh"`
	LeasesExpired          int `json:"leases_expired"`
}

type WorkerSummary struct {
	WorkerKind              string       `json:"worker_kind"`
	ShardID                 string       `json:"shard_id"`
	OwnerID                 string       `json:"owner_id,omitempty"`
	LeaseID                 string       `json:"lease_id,omitempty"`
	LeaseGeneration         uint64       `json:"lease_generation,omitempty"`
	LeaseFresh              bool         `json:"lease_fresh,omitempty"`
	LeaseExpired            bool         `json:"lease_expired,omitempty"`
	LeaseAgeSeconds         float64      `json:"lease_age_seconds,omitempty"`
	LeaseExpiresInSeconds   float64      `json:"lease_expires_in_seconds,omitempty"`
	CurrentStatus           string       `json:"current_status"`
	BacklogOperations       int          `json:"backlog_operations"`
	RunningOperations       int          `json:"running_operations"`
	RetryPendingOperations  int          `json:"retry_pending_operations"`
	FailedOperations        int          `json:"failed_operations"`
	SucceededOperations     int          `json:"succeeded_operations"`
	CanceledOperations      int          `json:"canceled_operations"`
	PausedOperations        int          `json:"paused_operations"`
	Retryable               int          `json:"retryable"`
	LastError               string       `json:"last_error,omitempty"`
	LastOperationID         string       `json:"last_operation_id,omitempty"`
	LastOperationStatus     string       `json:"last_operation_status,omitempty"`
	LastOperationAt         string       `json:"last_operation_at,omitempty"`
	LastOperationAgeSeconds float64      `json:"last_operation_age_seconds,omitempty"`
	StatusCounts            StatusCounts `json:"status_counts"`
}

type StatusCounts struct {
	Running      int `json:"running"`
	RetryPending int `json:"retry_pending"`
	Failed       int `json:"failed"`
	Succeeded    int `json:"succeeded"`
	Canceled     int `json:"canceled"`
	Paused       int `json:"paused"`
}

type workerKey struct {
	kind  string
	shard string
}

func Summarize(ctx context.Context, repo meta.Repository, cfg Config) (BacklogSnapshot, error) {
	now := cfg.now()
	opLimit := cfg.OperationLimit
	if opLimit <= 0 {
		opLimit = DefaultOperationLimit
	}
	leaseLimit := cfg.LeaseLimit
	if leaseLimit <= 0 {
		leaseLimit = DefaultOperationLimit
	}
	out := BacklogSnapshot{
		SchemaVersion:  "namros.worker.backlog.v1",
		GeneratedAt:    now.Format(time.RFC3339Nano),
		Status:         "ok",
		OperationLimit: opLimit,
		LeaseLimit:     leaseLimit,
	}
	if repo == nil {
		out.Status = "disabled"
		return out, nil
	}
	leases, err := repo.ListWorkerLeases(ctx, meta.ListWorkerLeasesRequest{Limit: leaseLimit})
	if err != nil {
		return out, err
	}
	operations, err := repo.ListWorkerOperations(ctx, meta.ListWorkerOperationsRequest{Limit: opLimit})
	if err != nil {
		return out, err
	}
	sortWorkerOperations(operations)
	out.LeaseCount = len(leases)
	out.OperationCount = len(operations)
	out.LeaseLimited = len(leases) >= leaseLimit
	out.OperationLimited = len(operations) >= opLimit

	workers := make(map[workerKey]*WorkerSummary)
	for _, lease := range leases {
		key := workerKey{kind: strings.TrimSpace(lease.WorkerKind), shard: strings.TrimSpace(lease.ShardID)}
		worker := ensureWorker(workers, key)
		applyLease(worker, lease, now)
		if worker.LeaseExpired {
			out.Totals.LeasesExpired++
		} else if worker.LeaseID != "" {
			out.Totals.LeasesFresh++
		}
	}
	for _, operation := range operations {
		key := workerKey{kind: strings.TrimSpace(operation.WorkerKind), shard: strings.TrimSpace(operation.ShardID)}
		worker := ensureWorker(workers, key)
		applyOperation(worker, operation, now)
	}

	out.Workers = make([]WorkerSummary, 0, len(workers))
	for _, worker := range workers {
		worker.CurrentStatus = currentWorkerStatus(*worker)
		out.Totals.BacklogOperations += worker.BacklogOperations
		out.Totals.RunningOperations += worker.RunningOperations
		out.Totals.RetryPendingOperations += worker.RetryPendingOperations
		out.Totals.FailedOperations += worker.FailedOperations
		out.Totals.SucceededOperations += worker.SucceededOperations
		out.Totals.CanceledOperations += worker.CanceledOperations
		out.Totals.PausedOperations += worker.PausedOperations
		out.Totals.Retryable += worker.Retryable
		out.Workers = append(out.Workers, *worker)
	}
	sort.Slice(out.Workers, func(i, j int) bool {
		if out.Workers[i].WorkerKind != out.Workers[j].WorkerKind {
			return out.Workers[i].WorkerKind < out.Workers[j].WorkerKind
		}
		return out.Workers[i].ShardID < out.Workers[j].ShardID
	})
	return out, nil
}

func (cfg Config) now() time.Time {
	if cfg.Now != nil {
		return cfg.Now().UTC()
	}
	return time.Now().UTC()
}

func ensureWorker(workers map[workerKey]*WorkerSummary, key workerKey) *WorkerSummary {
	worker := workers[key]
	if worker == nil {
		worker = &WorkerSummary{
			WorkerKind: strings.TrimSpace(key.kind),
			ShardID:    strings.TrimSpace(key.shard),
		}
		workers[key] = worker
	}
	return worker
}

func applyLease(worker *WorkerSummary, lease model.WorkerLease, now time.Time) {
	worker.OwnerID = strings.TrimSpace(lease.OwnerID)
	worker.LeaseID = strings.TrimSpace(lease.LeaseID)
	worker.LeaseGeneration = lease.Generation
	worker.LeaseExpired = !lease.ExpiresAt.IsZero() && !lease.ExpiresAt.After(now)
	worker.LeaseFresh = worker.LeaseID != "" && !worker.LeaseExpired
	if !lease.AcquiredAt.IsZero() {
		worker.LeaseAgeSeconds = nonNegativeSeconds(now.Sub(lease.AcquiredAt.UTC()))
	}
	if !lease.ExpiresAt.IsZero() {
		worker.LeaseExpiresInSeconds = nonNegativeSeconds(lease.ExpiresAt.UTC().Sub(now))
	}
}

func applyOperation(worker *WorkerSummary, operation model.WorkerOperationRecord, now time.Time) {
	status := operation.Status
	switch status {
	case model.WorkerOperationRunning:
		worker.RunningOperations++
		worker.StatusCounts.Running++
	case model.WorkerOperationRetryPending:
		worker.RetryPendingOperations++
		worker.BacklogOperations++
		worker.StatusCounts.RetryPending++
	case model.WorkerOperationFailed:
		worker.FailedOperations++
		worker.BacklogOperations++
		worker.StatusCounts.Failed++
	case model.WorkerOperationCanceled:
		worker.CanceledOperations++
		worker.StatusCounts.Canceled++
	case model.WorkerOperationPaused:
		worker.PausedOperations++
		worker.StatusCounts.Paused++
	default:
		worker.SucceededOperations++
		worker.StatusCounts.Succeeded++
	}
	if operation.Retryable > 0 {
		worker.Retryable += operation.Retryable
	}
	if worker.OwnerID == "" {
		worker.OwnerID = strings.TrimSpace(operation.OwnerID)
	}
	if worker.LeaseID == "" {
		worker.LeaseID = strings.TrimSpace(operation.LeaseID)
	}
	if worker.LastOperationID == "" {
		worker.LastOperationID = strings.TrimSpace(operation.OperationID)
		worker.LastOperationStatus = string(status)
		if at := operationTime(operation); !at.IsZero() {
			worker.LastOperationAt = at.UTC().Format(time.RFC3339Nano)
			worker.LastOperationAgeSeconds = nonNegativeSeconds(now.Sub(at.UTC()))
		}
	}
	if worker.LastError == "" && strings.TrimSpace(operation.LastError) != "" {
		worker.LastError = strings.TrimSpace(operation.LastError)
	}
}

func currentWorkerStatus(worker WorkerSummary) string {
	switch {
	case worker.RunningOperations > 0:
		return string(model.WorkerOperationRunning)
	case worker.RetryPendingOperations > 0:
		return string(model.WorkerOperationRetryPending)
	case worker.FailedOperations > 0:
		return string(model.WorkerOperationFailed)
	case worker.PausedOperations > 0:
		return string(model.WorkerOperationPaused)
	case worker.LeaseID != "" && worker.LeaseExpired:
		return "lease_expired"
	case worker.LeaseID != "":
		return "idle"
	case worker.SucceededOperations > 0:
		return string(model.WorkerOperationSucceeded)
	default:
		return "unknown"
	}
}

func sortWorkerOperations(operations []model.WorkerOperationRecord) {
	sort.SliceStable(operations, func(i, j int) bool {
		left := operationTime(operations[i])
		right := operationTime(operations[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return operations[i].OperationID > operations[j].OperationID
	})
}

func operationTime(operation model.WorkerOperationRecord) time.Time {
	for _, candidate := range []time.Time{operation.FinishedAt, operation.StartedAt, operation.CreatedAt} {
		if !candidate.IsZero() {
			return candidate.UTC()
		}
	}
	return time.Time{}
}

func nonNegativeSeconds(duration time.Duration) float64 {
	if duration < 0 {
		return 0
	}
	return duration.Seconds()
}

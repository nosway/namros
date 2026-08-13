package gc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/workerscheduler"
)

const (
	DefaultWorkerKind    = "gc"
	DefaultWorkerShardID = "orphans"
)

type SchedulerConfig struct {
	WorkerKind string
	ShardID    string
	OwnerID    string
	LeaseTTL   time.Duration
	Interval   time.Duration
	Limit      int
}

type Scheduler struct {
	Worker      Worker
	Repository  meta.Repository
	Config      SchedulerConfig
	Logger      *slog.Logger
	Status      *SchedulerStatus
	Clock       func() time.Time
	Budget      *workerscheduler.Budget
	BudgetScope workerscheduler.BudgetScope
}

type SchedulerResult struct {
	WorkerOperation model.WorkerOperationRecord `json:"worker_operation,omitempty"`
	GCOperation     OperationRecord             `json:"gc_operation,omitempty"`
	Skipped         bool                        `json:"skipped,omitempty"`
	SkipReason      string                      `json:"skip_reason,omitempty"`
}

type SchedulerStatus struct {
	mu       sync.Mutex
	snapshot SchedulerStatusSnapshot
}

type SchedulerStatusSnapshot struct {
	Enabled             bool                        `json:"enabled"`
	WorkerKind          string                      `json:"worker_kind,omitempty"`
	ShardID             string                      `json:"shard_id,omitempty"`
	OwnerID             string                      `json:"owner_id,omitempty"`
	Interval            string                      `json:"interval,omitempty"`
	LeaseTTL            string                      `json:"lease_ttl,omitempty"`
	Limit               int                         `json:"limit,omitempty"`
	Runs                uint64                      `json:"runs"`
	Successes           uint64                      `json:"successes"`
	Skipped             uint64                      `json:"skipped"`
	Errors              uint64                      `json:"errors"`
	LastStartedAt       time.Time                   `json:"last_started_at,omitempty"`
	LastFinishedAt      time.Time                   `json:"last_finished_at,omitempty"`
	LastOperationID     string                      `json:"last_operation_id,omitempty"`
	LastOperationStatus model.WorkerOperationStatus `json:"last_operation_status,omitempty"`
	LastSkipped         bool                        `json:"last_skipped,omitempty"`
	LastSkipReason      string                      `json:"last_skip_reason,omitempty"`
	LastError           string                      `json:"last_error,omitempty"`
	LastScanned         int                         `json:"last_scanned,omitempty"`
	LastDeleted         int                         `json:"last_deleted,omitempty"`
	LastRetryable       int                         `json:"last_retryable,omitempty"`
}

func NewSchedulerStatus() *SchedulerStatus {
	return &SchedulerStatus{}
}

func (s *SchedulerStatus) Configure(enabled bool, cfg SchedulerConfig) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Enabled = enabled
	s.snapshot.WorkerKind = cfg.WorkerKind
	s.snapshot.ShardID = cfg.ShardID
	s.snapshot.OwnerID = cfg.OwnerID
	s.snapshot.Interval = cfg.Interval.String()
	s.snapshot.LeaseTTL = cfg.LeaseTTL.String()
	s.snapshot.Limit = cfg.Limit
}

func (s *SchedulerStatus) Snapshot() SchedulerStatusSnapshot {
	if s == nil {
		return SchedulerStatusSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s *SchedulerStatus) observeStart(startedAt time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.Runs++
	s.snapshot.LastStartedAt = startedAt.UTC()
	s.snapshot.LastFinishedAt = time.Time{}
	s.snapshot.LastSkipped = false
	s.snapshot.LastSkipReason = ""
	s.snapshot.LastError = ""
}

func (s *SchedulerStatus) observeFinish(result SchedulerResult, err error, finishedAt time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshot.LastFinishedAt = finishedAt.UTC()
	s.snapshot.LastOperationID = result.WorkerOperation.OperationID
	s.snapshot.LastOperationStatus = result.WorkerOperation.Status
	s.snapshot.LastSkipped = result.Skipped
	s.snapshot.LastSkipReason = result.SkipReason
	s.snapshot.LastScanned = result.GCOperation.Scanned
	s.snapshot.LastDeleted = result.GCOperation.Deleted
	s.snapshot.LastRetryable = result.GCOperation.Retryable
	s.snapshot.LastError = result.WorkerOperation.LastError
	if err != nil {
		s.snapshot.Errors++
		s.snapshot.LastError = err.Error()
		return
	}
	if result.Skipped {
		s.snapshot.Skipped++
		return
	}
	s.snapshot.Successes++
}

func (s Scheduler) RunOnce(ctx context.Context) (result SchedulerResult, err error) {
	cfg, err := normalizeSchedulerConfig(s.Config)
	if err != nil {
		return SchedulerResult{}, err
	}
	if s.Status != nil {
		s.Status.Configure(true, cfg)
		s.Status.observeStart(s.now())
		defer func() {
			s.Status.observeFinish(result, err, s.now())
		}()
	}

	var gcRecord OperationRecord
	runner := workerscheduler.Runner{
		Repository:  s.Repository,
		WorkerKind:  cfg.WorkerKind,
		ShardID:     cfg.ShardID,
		OwnerID:     cfg.OwnerID,
		TTL:         cfg.LeaseTTL,
		Clock:       s.Clock,
		HoldLease:   true,
		Budget:      s.Budget,
		BudgetScope: s.BudgetScope,
	}
	workerOperation, err := runner.RunOnce(ctx, "", func(ctx context.Context, _ model.WorkerLease) (workerscheduler.Result, error) {
		var workErr error
		gcRecord, workErr = s.Worker.RunOnce(ctx, cfg.Limit)
		return schedulerResultFromGCRecord(gcRecord, workErr), workErr
	})
	if errors.Is(err, meta.ErrCASConflict) {
		return SchedulerResult{
			Skipped:    true,
			SkipReason: "lease_held",
		}, nil
	}
	if errors.Is(err, meta.ErrWorkerPaused) {
		return SchedulerResult{
			Skipped:    true,
			SkipReason: "paused",
		}, nil
	}
	if errors.Is(err, meta.ErrWorkerCanceled) {
		return SchedulerResult{
			Skipped:    true,
			SkipReason: "canceled",
		}, nil
	}
	if errors.Is(err, workerscheduler.ErrThrottled) {
		return SchedulerResult{
			Skipped:    true,
			SkipReason: "throttled",
		}, nil
	}
	if err != nil {
		return SchedulerResult{WorkerOperation: workerOperation, GCOperation: gcRecord}, err
	}
	return SchedulerResult{
		WorkerOperation: workerOperation,
		GCOperation:     gcRecord,
	}, nil
}

func (s Scheduler) Run(ctx context.Context) error {
	cfg, err := normalizeSchedulerConfig(s.Config)
	if err != nil {
		return err
	}
	s.Config = cfg

	s.logRunOnce(ctx)
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.logRunOnce(ctx)
		}
	}
}

func (s Scheduler) logRunOnce(ctx context.Context) {
	result, err := s.RunOnce(ctx)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("gc scheduler run failed", "error", err)
		}
		return
	}
	if result.Skipped {
		if s.Logger != nil {
			s.Logger.Info("gc scheduler run skipped", "reason", result.SkipReason)
		}
		return
	}
	if s.Logger != nil {
		s.Logger.Info("gc scheduler run completed",
			"worker_operation_id", result.WorkerOperation.OperationID,
			"worker_status", result.WorkerOperation.Status,
			"gc_status", result.GCOperation.Status,
			"scanned", result.GCOperation.Scanned,
			"deleted", result.GCOperation.Deleted,
			"skipped", result.GCOperation.Skipped,
			"retryable", result.GCOperation.Retryable,
		)
	}
}

func normalizeSchedulerConfig(cfg SchedulerConfig) (SchedulerConfig, error) {
	if cfg.WorkerKind == "" {
		cfg.WorkerKind = DefaultWorkerKind
	}
	if cfg.ShardID == "" {
		cfg.ShardID = DefaultWorkerShardID
	}
	if cfg.OwnerID == "" {
		return SchedulerConfig{}, fmt.Errorf("gc scheduler owner id is required")
	}
	if cfg.LeaseTTL <= 0 {
		return SchedulerConfig{}, fmt.Errorf("gc scheduler lease ttl must be positive")
	}
	if cfg.Interval <= 0 {
		return SchedulerConfig{}, fmt.Errorf("gc scheduler interval must be positive")
	}
	if cfg.Limit < 0 {
		return SchedulerConfig{}, fmt.Errorf("gc scheduler limit cannot be negative")
	}
	return cfg, nil
}

func schedulerResultFromGCRecord(record OperationRecord, err error) workerscheduler.Result {
	result := workerscheduler.Result{
		Status:    workerStatusFromGCStatus(record.Status),
		Scanned:   record.Scanned,
		Processed: record.Deleted,
		Skipped:   record.Skipped,
		Retryable: record.Retryable,
	}
	if err != nil {
		if result.Status == "" {
			result.Status = model.WorkerOperationFailed
		}
		result.LastError = err.Error()
		return result
	}
	if result.LastError == "" {
		result.LastError = firstRetryableAttemptError(record.Attempts)
	}
	return result
}

func workerStatusFromGCStatus(status model.GCOperationStatus) model.WorkerOperationStatus {
	switch status {
	case model.GCOperationSucceeded:
		return model.WorkerOperationSucceeded
	case model.GCOperationRetryPending:
		return model.WorkerOperationRetryPending
	case model.GCOperationFailed:
		return model.WorkerOperationFailed
	default:
		return ""
	}
}

func firstRetryableAttemptError(attempts []AttemptRecord) string {
	for _, attempt := range attempts {
		if attempt.Retryable || attempt.Status == AttemptRetryable {
			return attempt.Error
		}
	}
	return ""
}

func (s Scheduler) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

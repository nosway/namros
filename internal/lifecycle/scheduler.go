package lifecycle

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
	DefaultWorkerKind    = "lifecycle"
	DefaultWorkerShardID = "buckets"
)

type SchedulerConfig struct {
	WorkerKind string
	ShardID    string
	OwnerID    string
	BucketID   string
	LeaseTTL   time.Duration
	Interval   time.Duration
	MaxKeys    int
	MaxUploads int
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
	LifecycleWorker WorkerResult                `json:"lifecycle_worker,omitempty"`
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
	BucketID            string                      `json:"bucket_id,omitempty"`
	Interval            string                      `json:"interval,omitempty"`
	LeaseTTL            string                      `json:"lease_ttl,omitempty"`
	MaxKeys             int                         `json:"max_keys,omitempty"`
	MaxUploads          int                         `json:"max_uploads,omitempty"`
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
	LastPlanned         int                         `json:"last_planned,omitempty"`
	LastProcessed       int                         `json:"last_processed,omitempty"`
	LastActionSkipped   int                         `json:"last_action_skipped,omitempty"`
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
	s.snapshot.BucketID = cfg.BucketID
	s.snapshot.Interval = cfg.Interval.String()
	s.snapshot.LeaseTTL = cfg.LeaseTTL.String()
	s.snapshot.MaxKeys = cfg.MaxKeys
	s.snapshot.MaxUploads = cfg.MaxUploads
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
	s.snapshot.LastPlanned = result.LifecycleWorker.Planned
	s.snapshot.LastProcessed = result.LifecycleWorker.Processed
	s.snapshot.LastActionSkipped = result.LifecycleWorker.Skipped
	s.snapshot.LastRetryable = lifecycleRetryable(result.LifecycleWorker)
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

	var lifecycleResult WorkerResult
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
	workerOperation, err := runner.RunOnce(ctx, cfg.BucketID, func(ctx context.Context, _ model.WorkerLease) (workerscheduler.Result, error) {
		var workErr error
		lifecycleResult, workErr = s.Worker.RunOnce(ctx, WorkerRequest{
			Plan: PlanRequest{
				BucketID:   cfg.BucketID,
				Now:        s.now(),
				MaxKeys:    cfg.MaxKeys,
				MaxUploads: cfg.MaxUploads,
			},
		})
		return schedulerResultFromLifecycle(lifecycleResult, cfg.BucketID, workErr), workErr
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
		return SchedulerResult{WorkerOperation: workerOperation, LifecycleWorker: lifecycleResult}, err
	}
	return SchedulerResult{
		WorkerOperation: workerOperation,
		LifecycleWorker: lifecycleResult,
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
			s.Logger.Error("lifecycle scheduler run failed", "error", err)
		}
		return
	}
	if result.Skipped {
		if s.Logger != nil {
			s.Logger.Info("lifecycle scheduler run skipped", "reason", result.SkipReason)
		}
		return
	}
	if s.Logger != nil {
		s.Logger.Info("lifecycle scheduler run completed",
			"worker_operation_id", result.WorkerOperation.OperationID,
			"worker_status", result.WorkerOperation.Status,
			"bucket_id", s.Config.BucketID,
			"planned", result.LifecycleWorker.Planned,
			"processed", result.LifecycleWorker.Processed,
			"skipped", result.LifecycleWorker.Skipped,
			"retryable", lifecycleRetryable(result.LifecycleWorker),
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
		return SchedulerConfig{}, fmt.Errorf("lifecycle scheduler owner id is required")
	}
	if cfg.BucketID == "" {
		return SchedulerConfig{}, fmt.Errorf("lifecycle scheduler bucket id is required")
	}
	if cfg.LeaseTTL <= 0 {
		return SchedulerConfig{}, fmt.Errorf("lifecycle scheduler lease ttl must be positive")
	}
	if cfg.Interval <= 0 {
		return SchedulerConfig{}, fmt.Errorf("lifecycle scheduler interval must be positive")
	}
	if cfg.MaxKeys < 0 {
		return SchedulerConfig{}, fmt.Errorf("lifecycle scheduler max keys cannot be negative")
	}
	if cfg.MaxUploads < 0 {
		return SchedulerConfig{}, fmt.Errorf("lifecycle scheduler max uploads cannot be negative")
	}
	return cfg, nil
}

func schedulerResultFromLifecycle(result WorkerResult, cursor string, err error) workerscheduler.Result {
	retryable := lifecycleRetryable(result)
	status := model.WorkerOperationSucceeded
	lastError := ""
	if retryable > 0 {
		status = model.WorkerOperationRetryPending
		lastError = "lifecycle worker completed with retryable failures"
	}
	if err != nil {
		status = model.WorkerOperationFailed
		if retryable > 0 {
			status = model.WorkerOperationRetryPending
		}
		lastError = err.Error()
	}
	return workerscheduler.Result{
		Status:    status,
		Cursor:    cursor,
		Scanned:   result.Planned,
		Processed: result.Processed,
		Skipped:   result.Skipped,
		Retryable: retryable,
		LastError: lastError,
	}
}

func lifecycleRetryable(result WorkerResult) int {
	return result.DeleteFailed + result.Failed
}

func (s Scheduler) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

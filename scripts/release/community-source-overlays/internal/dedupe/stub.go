package dedupe

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/workerscheduler"
)

const DefaultSchedulerLockID = "dedupe-background"

type Mode string

const (
	ModePostProcess    Mode = "post_process"
	ModeIngestAssisted Mode = "ingest_assisted"
	ModeVerifiedInline Mode = "verified_inline"
	ModeHashOnlyInline Mode = "hash_only_inline"
)

type DecisionStatus string

const (
	DecisionAdmitted DecisionStatus = "admitted"
	DecisionRejected DecisionStatus = "rejected"
)

type RejectReason string

const (
	RejectFeatureDisabled RejectReason = "feature_disabled"
)

type Plan struct {
	Status DecisionStatus `json:"status"`
	Reason RejectReason   `json:"reason,omitempty"`
	Mode   Mode           `json:"mode"`
	Scope  string         `json:"scope"`
}

type Object struct {
	TenantID            string                          `json:"tenant_id"`
	BucketID            string                          `json:"bucket_id"`
	Key                 string                          `json:"key"`
	VersionID           string                          `json:"version_id"`
	SizeBytes           int64                           `json:"size_bytes"`
	Digest              storage.Digest                  `json:"digest"`
	StorageClass        storage.StorageClassSnapshot    `json:"storage_class"`
	ObjectLockRetention model.ObjectLockRetention       `json:"object_lock_retention,omitempty"`
	ObjectLockLegalHold model.ObjectLockLegalHoldStatus `json:"object_lock_legal_hold,omitempty"`
}

type ScanRequest struct {
	TenantID            string
	BucketID            string
	Prefix              string
	MaxKeys             int
	Limit               int
	Enabled             bool
	Mode                Mode
	ByteVerified        bool
	VerifyBytes         bool
	AllowVerifiedInline bool
}

type ScanResult struct {
	ScannedVersions int         `json:"scanned_versions"`
	CandidatePairs  int         `json:"candidate_pairs"`
	Candidates      []Candidate `json:"candidates"`
}

type Candidate struct {
	Source    Object `json:"source"`
	Candidate Object `json:"candidate"`
	Plan      Plan   `json:"plan"`
}

type RunRequest struct {
	Scan ScanRequest `json:"scan"`
}

type RunStatus string

const (
	RunCandidateAcked     RunStatus = "acked"
	RunCandidateSkipped   RunStatus = "skipped"
	RunCandidateRetryable RunStatus = "retryable"
)

type RunRecord struct {
	OperationID         string                      `json:"operation_id,omitempty"`
	ResumeOfOperationID string                      `json:"resume_of_operation_id,omitempty"`
	Status              model.DedupeOperationStatus `json:"status"`
	StartedAt           time.Time                   `json:"started_at"`
	FinishedAt          time.Time                   `json:"finished_at"`
	Scan                ScanResult                  `json:"scan"`
	ScannedCandidates   int                         `json:"scanned_candidates"`
	Acked               int                         `json:"acked"`
	Skipped             int                         `json:"skipped"`
	Retryable           int                         `json:"retryable"`
	Attempts            []RunCandidateRecord        `json:"attempts"`
}

type RunCandidateRecord struct {
	BucketID         string    `json:"bucket_id"`
	Key              string    `json:"key"`
	SourceVersion    string    `json:"source_version"`
	CandidateVersion string    `json:"candidate_version"`
	Plan             Plan      `json:"plan"`
	Status           RunStatus `json:"status"`
	SharedObjectID   string    `json:"shared_object_id,omitempty"`
	OrphansMarked    int       `json:"orphans_marked,omitempty"`
	Error            string    `json:"error,omitempty"`
}

type Metrics struct {
	mu       sync.Mutex
	runs     uint64
	errors   uint64
	statuses map[model.DedupeOperationStatus]uint64
}

type MetricsSnapshot struct {
	Runs      uint64                                 `json:"runs"`
	Errors    uint64                                 `json:"errors,omitempty"`
	Resumed   uint64                                 `json:"resumed,omitempty"`
	Scanned   uint64                                 `json:"scanned"`
	Acked     uint64                                 `json:"acked"`
	Skipped   uint64                                 `json:"skipped"`
	Retryable uint64                                 `json:"retryable"`
	Statuses  map[model.DedupeOperationStatus]uint64 `json:"statuses"`
}

func NewMetrics() *Metrics {
	return &Metrics{statuses: map[model.DedupeOperationStatus]uint64{}}
}

func (m *Metrics) ObserveRun(record RunRecord, err error) {
	if m == nil {
		return
	}
	status := record.Status
	if status == "" {
		status = model.DedupeOperationSucceeded
	}
	if err != nil {
		status = model.DedupeOperationFailed
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs++
	if err != nil {
		m.errors++
	}
	if m.statuses == nil {
		m.statuses = map[model.DedupeOperationStatus]uint64{}
	}
	m.statuses[status]++
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	statuses := make(map[model.DedupeOperationStatus]uint64, len(m.statuses))
	for key, value := range m.statuses {
		statuses[key] = value
	}
	return MetricsSnapshot{
		Runs:     m.runs,
		Errors:   m.errors,
		Statuses: statuses,
	}
}

type BackgroundWorker struct {
	Repository     meta.Repository
	Storage        storage.SegmentStore
	Orphans        storage.OrphanTracker
	OperationStore any
	Metrics        *Metrics
	Now            func() time.Time
}

func (w BackgroundWorker) RunOnce(context.Context, RunRequest) (RunRecord, error) {
	err := enterpriseError()
	record := RunRecord{
		Status:     model.DedupeOperationFailed,
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
	}
	if w.Metrics != nil {
		w.Metrics.ObserveRun(record, err)
	}
	return record, err
}

type LockStore interface {
	AcquireDedupeOperationLock(context.Context, meta.AcquireDedupeOperationLockRequest) (model.DedupeOperationLock, error)
	ReleaseDedupeOperationLock(context.Context, meta.ReleaseDedupeOperationLockRequest) error
}

type SchedulerConfig struct {
	LockID   string
	OwnerID  string
	LockTTL  time.Duration
	Interval time.Duration
	Request  RunRequest
}

type Scheduler struct {
	Worker      BackgroundWorker
	LockStore   LockStore
	Config      SchedulerConfig
	Logger      *slog.Logger
	Status      *SchedulerStatus
	Budget      *workerscheduler.Budget
	BudgetScope workerscheduler.BudgetScope
}

type SchedulerResult struct {
	Lock       model.DedupeOperationLock `json:"lock,omitempty"`
	Record     RunRecord                 `json:"record,omitempty"`
	Skipped    bool                      `json:"skipped,omitempty"`
	SkipReason string                    `json:"skip_reason,omitempty"`
}

type SchedulerStatus struct {
	mu       sync.Mutex
	snapshot SchedulerStatusSnapshot
}

type SchedulerStatusSnapshot struct {
	Enabled             bool                        `json:"enabled"`
	LockID              string                      `json:"lock_id,omitempty"`
	OwnerID             string                      `json:"owner_id,omitempty"`
	Interval            string                      `json:"interval,omitempty"`
	LockTTL             string                      `json:"lock_ttl,omitempty"`
	Runs                uint64                      `json:"runs"`
	Successes           uint64                      `json:"successes"`
	Skipped             uint64                      `json:"skipped"`
	Errors              uint64                      `json:"errors"`
	LastStartedAt       time.Time                   `json:"last_started_at,omitempty"`
	LastFinishedAt      time.Time                   `json:"last_finished_at,omitempty"`
	LastOperationID     string                      `json:"last_operation_id,omitempty"`
	LastOperationStatus model.DedupeOperationStatus `json:"last_operation_status,omitempty"`
	LastSkipped         bool                        `json:"last_skipped,omitempty"`
	LastSkipReason      string                      `json:"last_skip_reason,omitempty"`
	LastError           string                      `json:"last_error,omitempty"`
	LastLock            model.DedupeOperationLock   `json:"last_lock,omitempty"`
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
	s.snapshot.LockID = cfg.LockID
	s.snapshot.OwnerID = cfg.OwnerID
	s.snapshot.Interval = cfg.Interval.String()
	s.snapshot.LockTTL = cfg.LockTTL.String()
}

func (s *SchedulerStatus) Snapshot() SchedulerStatusSnapshot {
	if s == nil {
		return SchedulerStatusSnapshot{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot
}

func (s Scheduler) RunOnce(context.Context) (SchedulerResult, error) {
	if s.Status != nil {
		cfg := s.Config
		if cfg.LockID == "" {
			cfg.LockID = DefaultSchedulerLockID
		}
		s.Status.Configure(true, cfg)
	}
	return SchedulerResult{}, enterpriseError()
}

func (s Scheduler) Run(ctx context.Context) error {
	if _, err := s.RunOnce(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

func enterpriseError() error {
	if err := edition.Require(edition.Current(), edition.FeatureDedupe); err != nil {
		return err
	}
	return fmt.Errorf("dedupe implementation is provided by the private Enterprise source overlay")
}

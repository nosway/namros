package gc

import (
	"sync"

	"github.com/nosway/namros/internal/meta/model"
)

type Metrics struct {
	mu sync.Mutex

	runs      uint64
	errors    uint64
	resumed   uint64
	scanned   uint64
	deleted   uint64
	skipped   uint64
	retryable uint64
	statuses  map[model.GCOperationStatus]uint64
}

type MetricsSnapshot struct {
	Runs      uint64                             `json:"runs"`
	Errors    uint64                             `json:"errors,omitempty"`
	Resumed   uint64                             `json:"resumed,omitempty"`
	Scanned   uint64                             `json:"scanned"`
	Deleted   uint64                             `json:"deleted"`
	Skipped   uint64                             `json:"skipped"`
	Retryable uint64                             `json:"retryable"`
	Statuses  map[model.GCOperationStatus]uint64 `json:"statuses"`
}

func NewMetrics() *Metrics {
	return &Metrics{
		statuses: make(map[model.GCOperationStatus]uint64),
	}
}

func (m *Metrics) ObserveRun(record OperationRecord, err error) {
	if m == nil {
		return
	}
	status := record.Status
	if status == "" {
		status = model.GCOperationSucceeded
		if err != nil {
			status = model.GCOperationFailed
		} else if record.Retryable > 0 {
			status = model.GCOperationRetryPending
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()
	m.runs++
	if err != nil {
		m.errors++
	}
	if record.ResumeOfOperationID != "" {
		m.resumed++
	}
	m.scanned += uint64(record.Scanned)
	m.deleted += uint64(record.Deleted)
	m.skipped += uint64(record.Skipped)
	m.retryable += uint64(record.Retryable)
	m.statuses[status]++
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()
	statuses := make(map[model.GCOperationStatus]uint64, len(m.statuses))
	for key, value := range m.statuses {
		statuses[key] = value
	}
	return MetricsSnapshot{
		Runs:      m.runs,
		Errors:    m.errors,
		Resumed:   m.resumed,
		Scanned:   m.scanned,
		Deleted:   m.deleted,
		Skipped:   m.skipped,
		Retryable: m.retryable,
		Statuses:  statuses,
	}
}

func (m *Metrics) ensureLocked() {
	if m.statuses == nil {
		m.statuses = make(map[model.GCOperationStatus]uint64)
	}
}

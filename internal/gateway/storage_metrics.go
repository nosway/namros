package gateway

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/nosway/namros/internal/opsmetrics"
	"github.com/nosway/namros/internal/storage"
)

type metricsSegmentStore struct {
	next           storage.SegmentStore
	orphanTracker  storage.OrphanTracker
	validator      storage.SegmentValidator
	healthChecker  storage.HealthChecker
	metrics        *opsmetrics.GatewayMetrics
	defaultBackend string
}

func newMetricsSegmentStore(next storage.SegmentStore, metrics *opsmetrics.GatewayMetrics, defaultBackend string) *metricsSegmentStore {
	store := &metricsSegmentStore{
		next:           next,
		metrics:        metrics,
		defaultBackend: cleanMetricsBackend(defaultBackend),
	}
	if orphanTracker, ok := next.(storage.OrphanTracker); ok {
		store.orphanTracker = orphanTracker
	}
	if validator, ok := next.(storage.SegmentValidator); ok {
		store.validator = validator
	}
	if healthChecker, ok := next.(storage.HealthChecker); ok {
		store.healthChecker = healthChecker
	}
	return store
}

func (s *metricsSegmentStore) PutSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
	start := time.Now()
	ref, err := s.next.PutSegment(ctx, req)
	backend := s.defaultBackend
	if err == nil {
		backend = metricsBackendForRef(ref, backend)
	}
	if backend == "" {
		backend = cleanMetricsBackend(req.StorageClass.Backend)
	}
	s.metrics.ObserveStorage("put_segment", backend, time.Since(start), req.SizeBytes, err)
	return ref, err
}

func (s *metricsSegmentStore) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	start := time.Now()
	reader, err := s.next.GetSegment(ctx, ref, off, length)
	readBytes := length
	if readBytes == 0 && off <= ref.SizeBytes {
		readBytes = ref.SizeBytes - off
	}
	s.metrics.ObserveStorage("get_segment", metricsBackendForRef(ref, s.defaultBackend), time.Since(start), readBytes, err)
	return reader, err
}

func (s *metricsSegmentStore) DeleteSegment(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	start := time.Now()
	err := s.next.DeleteSegment(ctx, ref, reason)
	s.metrics.ObserveStorage("delete_segment", metricsBackendForRef(ref, s.defaultBackend), time.Since(start), ref.SizeBytes, err)
	return err
}

func (s *metricsSegmentStore) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if s.orphanTracker == nil {
		return storage.ErrInvalidArgument
	}
	start := time.Now()
	err := s.orphanTracker.MarkOrphan(ctx, ref, reason)
	s.metrics.ObserveStorage("mark_orphan", metricsBackendForRef(ref, s.defaultBackend), time.Since(start), ref.SizeBytes, err)
	return err
}

func (s *metricsSegmentStore) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if s.orphanTracker == nil {
		return nil, storage.ErrInvalidArgument
	}
	start := time.Now()
	candidates, err := s.orphanTracker.ListGCCandidates(ctx, limit)
	s.metrics.ObserveStorage("list_gc_candidates", s.defaultBackend, time.Since(start), 0, err)
	return candidates, err
}

func (s *metricsSegmentStore) ValidateSegment(ctx context.Context, ref storage.SegmentRef) error {
	if s.validator == nil {
		return nil
	}
	start := time.Now()
	err := s.validator.ValidateSegment(ctx, ref)
	s.metrics.ObserveStorage("validate_segment", metricsBackendForRef(ref, s.defaultBackend), time.Since(start), ref.SizeBytes, err)
	return err
}

func (s *metricsSegmentStore) CheckHealth(ctx context.Context) error {
	if s.healthChecker == nil {
		return nil
	}
	return s.healthChecker.CheckHealth(ctx)
}

func metricsBackendForRef(ref storage.SegmentRef, fallback string) string {
	if ref.Placement.Backend != "" {
		return cleanMetricsBackend(ref.Placement.Backend)
	}
	if ref.StorageClass.Backend != "" {
		return cleanMetricsBackend(ref.StorageClass.Backend)
	}
	return cleanMetricsBackend(fallback)
}

func cleanMetricsBackend(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	return value
}

var _ storage.SegmentStore = (*metricsSegmentStore)(nil)
var _ storage.OrphanTracker = (*metricsSegmentStore)(nil)
var _ storage.SegmentValidator = (*metricsSegmentStore)(nil)
var _ storage.HealthChecker = (*metricsSegmentStore)(nil)

package opsmetrics

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/version"
	"github.com/nosway/namros/internal/workerops"
)

type BuildInfo struct {
	Edition string
	Version string
	Commit  string
}

func BuildInfoFromConfig(cfg config.Config) BuildInfo {
	info := version.Info()
	return BuildInfo{
		Edition: cfg.Edition,
		Version: info["version"],
		Commit:  info["commit"],
	}
}

type GatewayMetrics struct {
	registry *prometheus.Registry

	buildInfo           *prometheus.GaugeVec
	s3Requests          *prometheus.CounterVec
	s3Latency           *prometheus.HistogramVec
	s3Errors            *prometheus.CounterVec
	s3FirstByte         *prometheus.HistogramVec
	s3RequestBodyRead   *prometheus.HistogramVec
	s3Bytes             *prometheus.CounterVec
	s3Active            *prometheus.GaugeVec
	metadataOps         *prometheus.CounterVec
	metadataDuration    *prometheus.HistogramVec
	admissions          *prometheus.CounterVec
	admissionDuration   *prometheus.HistogramVec
	leaseFreshness      *prometheus.GaugeVec
	storageDuration     *prometheus.HistogramVec
	storageBytes        *prometheus.CounterVec
	sbsPhysicalDuration *prometheus.HistogramVec
	sbsPhysicalBytes    *prometheus.CounterVec
	sbsAllocation       *prometheus.HistogramVec
	sbsAllocationChunks *prometheus.CounterVec
	sbsReadback         *prometheus.HistogramVec
	sbsECDuration       *prometheus.HistogramVec
	sbsECBytes          *prometheus.CounterVec
	sbsPoolConfigured   *prometheus.GaugeVec
	sbsPoolActive       *prometheus.GaugeVec
	sbsPoolErrors       *prometheus.GaugeVec
	sbsPoolStale        *prometheus.GaugeVec
	workerBacklog       *prometheus.GaugeVec
	workerOperations    *prometheus.GaugeVec
	workerRetryable     *prometheus.GaugeVec
	workerLeaseAge      *prometheus.GaugeVec
	workerLeaseFresh    *prometheus.GaugeVec

	mu             sync.Mutex
	layerStats     map[layerStatsKey]*layerStats
	admissionStats map[admissionStatsKey]uint64
	s3ErrorStats   map[s3ErrorStatsKey]uint64
}

type S3Observation struct {
	API           string
	StatusCode    int
	RequestBytes  int64
	ResponseBytes int
	Duration      time.Duration
	ErrorCode     string
}

type MetricsSnapshot struct {
	SchemaVersion string                 `json:"schema_version"`
	GeneratedAt   string                 `json:"generated_at"`
	Layers        []LayerMetricsSnapshot `json:"layers"`
	Admissions    []AdmissionSnapshot    `json:"admissions,omitempty"`
	S3Errors      []S3ErrorSnapshot      `json:"s3_errors,omitempty"`
}

type SBSVolumePoolObservation struct {
	PoolID               string
	Source               string
	ConfiguredGeneration uint64
	ActiveGeneration     uint64
	RefreshErrorCount    uint64
	StaleSeconds         float64
}

type LayerMetricsSnapshot struct {
	Component string  `json:"component"`
	Operation string  `json:"operation"`
	Status    string  `json:"status"`
	Count     uint64  `json:"count"`
	TotalMs   float64 `json:"total_ms"`
	AvgMs     float64 `json:"avg_ms"`
	MinMs     float64 `json:"min_ms"`
	MaxMs     float64 `json:"max_ms"`
	Bytes     uint64  `json:"bytes,omitempty"`
	Units     uint64  `json:"units,omitempty"`
}

type AdmissionSnapshot struct {
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
	Count  uint64 `json:"count"`
}

type S3ErrorSnapshot struct {
	API         string `json:"api"`
	StatusClass string `json:"status_class"`
	ErrorCode   string `json:"error_code"`
	Count       uint64 `json:"count"`
}

type layerStatsKey struct {
	component string
	operation string
	status    string
}

type admissionStatsKey struct {
	kind   string
	reason string
}

type s3ErrorStatsKey struct {
	api         string
	statusClass string
	errorCode   string
}

type layerStats struct {
	count uint64
	total time.Duration
	min   time.Duration
	max   time.Duration
	bytes uint64
	units uint64
}

func NewGatewayMetrics(info BuildInfo) *GatewayMetrics {
	m := &GatewayMetrics{
		registry:       prometheus.NewRegistry(),
		layerStats:     make(map[layerStatsKey]*layerStats),
		admissionStats: make(map[admissionStatsKey]uint64),
		s3ErrorStats:   make(map[s3ErrorStatsKey]uint64),
	}
	m.buildInfo = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_build_info",
		Help: "NAMROS gateway build and edition information.",
	}, []string{"edition", "version", "commit"})
	m.s3Requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_s3_requests_total",
		Help: "Total number of S3 API requests handled by the NAMROS gateway.",
	}, []string{"api", "status_class"})
	m.s3Latency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_s3_request_duration_seconds",
		Help:    "S3 API request latency observed by the NAMROS gateway.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
	}, []string{"api", "status_class"})
	m.s3Errors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_s3_errors_total",
		Help: "Total S3 API errors observed by the NAMROS gateway.",
	}, []string{"api", "status_class", "error_code"})
	m.s3FirstByte = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_s3_first_byte_duration_seconds",
		Help:    "Time from S3 request start until the first response body byte is written.",
		Buckets: latencyBuckets(),
	}, []string{"api", "status_class"})
	m.s3RequestBodyRead = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_s3_request_body_read_duration_seconds",
		Help:    "Total time spent reading S3 request bodies in the gateway.",
		Buckets: latencyBuckets(),
	}, []string{"api", "status_class"})
	m.s3Bytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_s3_bytes_total",
		Help: "Total S3 request or response bytes observed by the NAMROS gateway.",
	}, []string{"direction", "api"})
	m.s3Active = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_s3_active_requests",
		Help: "Current number of in-flight S3 API requests handled by the NAMROS gateway.",
	}, []string{"api"})
	m.metadataOps = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_metadata_operations_total",
		Help: "Total metadata operations observed by the NAMROS gateway.",
	}, []string{"operation", "status"})
	m.metadataDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_metadata_operation_duration_seconds",
		Help:    "Metadata repository operation latency observed by the NAMROS gateway.",
		Buckets: latencyBuckets(),
	}, []string{"operation", "status"})
	m.admissions = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_admission_rejections_total",
		Help: "Total request admissions rejected by the NAMROS gateway.",
	}, []string{"kind", "reason"})
	m.admissionDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_admission_decision_duration_seconds",
		Help:    "Gateway admission decision latency by admission kind and outcome.",
		Buckets: latencyBuckets(),
	}, []string{"kind", "outcome", "reason"})
	m.leaseFreshness = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_coordination_lease_fresh",
		Help: "Gateway coordination lease freshness. 1 means fresh, 0 means stale, absent means not configured.",
	}, []string{"gateway_id"})
	m.storageDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_storage_operation_duration_seconds",
		Help:    "Storage segment operation latency observed by the NAMROS gateway.",
		Buckets: latencyBuckets(),
	}, []string{"backend", "operation", "status"})
	m.storageBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_storage_operation_bytes_total",
		Help: "Storage segment operation payload bytes observed by the NAMROS gateway.",
	}, []string{"backend", "operation", "direction", "status"})
	m.sbsPhysicalDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_sbs_physical_chunk_duration_seconds",
		Help:    "SBS Physical chunk RPC latency observed by the NAMROS gateway.",
		Buckets: latencyBuckets(),
	}, []string{"operation", "status"})
	m.sbsPhysicalBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_sbs_physical_chunk_bytes_total",
		Help: "SBS Physical chunk RPC payload bytes observed by the NAMROS gateway.",
	}, []string{"operation", "direction", "status"})
	m.sbsAllocation = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_sbs_physical_allocation_duration_seconds",
		Help:    "SBS Physical chunk allocation latency observed by the NAMROS gateway.",
		Buckets: latencyBuckets(),
	}, []string{"status"})
	m.sbsAllocationChunks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_sbs_physical_allocated_chunks_total",
		Help: "SBS Physical allocated chunk count observed by the NAMROS gateway.",
	}, []string{"status"})
	m.sbsReadback = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_sbs_physical_readback_duration_seconds",
		Help:    "SBS Physical synchronous write readback verification latency observed by the NAMROS gateway.",
		Buckets: latencyBuckets(),
	}, []string{"status"})
	m.sbsECDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "namros_gateway_sbs_ec_shard_duration_seconds",
		Help:    "SBS EC shard RPC latency observed by the NAMROS gateway.",
		Buckets: latencyBuckets(),
	}, []string{"operation", "store_id", "role", "status"})
	m.sbsECBytes = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "namros_gateway_sbs_ec_shard_bytes_total",
		Help: "SBS EC shard RPC payload bytes observed by the NAMROS gateway.",
	}, []string{"operation", "store_id", "role", "direction", "status"})
	m.sbsPoolConfigured = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_sbs_volume_pool_configured_generation",
		Help: "Latest SBS volume pool generation observed from configuration or metadata registry.",
	}, []string{"pool_id", "source"})
	m.sbsPoolActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_sbs_volume_pool_active_generation",
		Help: "SBS volume pool generation currently active for gateway read/write routing.",
	}, []string{"pool_id", "source"})
	m.sbsPoolErrors = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_sbs_volume_pool_refresh_errors",
		Help: "Cumulative SBS volume pool refresh errors observed by this gateway process.",
	}, []string{"pool_id", "source"})
	m.sbsPoolStale = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_sbs_volume_pool_stale_seconds",
		Help: "Seconds since the SBS volume pool runtime became stale or started failing refreshes.",
	}, []string{"pool_id", "source"})
	m.workerBacklog = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_worker_backlog_operations",
		Help: "Worker operation backlog needing retry or operator attention.",
	}, []string{"worker_kind", "shard_id"})
	m.workerOperations = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_worker_operations",
		Help: "Recent worker operation counts by status from metadata operation records.",
	}, []string{"worker_kind", "shard_id", "status"})
	m.workerRetryable = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_worker_retryable_items",
		Help: "Retryable worker items observed in recent metadata operation records.",
	}, []string{"worker_kind", "shard_id"})
	m.workerLeaseAge = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_worker_lease_age_seconds",
		Help: "Age of the latest worker lease by worker kind, shard, and owner.",
	}, []string{"worker_kind", "shard_id", "owner_id"})
	m.workerLeaseFresh = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "namros_gateway_worker_lease_fresh",
		Help: "Worker lease freshness. 1 means the lease has not expired, 0 means expired.",
	}, []string{"worker_kind", "shard_id", "owner_id"})

	m.registry.MustRegister(
		m.buildInfo,
		m.s3Requests,
		m.s3Latency,
		m.s3Errors,
		m.s3FirstByte,
		m.s3RequestBodyRead,
		m.s3Bytes,
		m.s3Active,
		m.metadataOps,
		m.metadataDuration,
		m.admissions,
		m.admissionDuration,
		m.leaseFreshness,
		m.storageDuration,
		m.storageBytes,
		m.sbsPhysicalDuration,
		m.sbsPhysicalBytes,
		m.sbsAllocation,
		m.sbsAllocationChunks,
		m.sbsReadback,
		m.sbsECDuration,
		m.sbsECBytes,
		m.sbsPoolConfigured,
		m.sbsPoolActive,
		m.sbsPoolErrors,
		m.sbsPoolStale,
		m.workerBacklog,
		m.workerOperations,
		m.workerRetryable,
		m.workerLeaseAge,
		m.workerLeaseFresh,
	)
	m.buildInfo.WithLabelValues(cleanLabel(info.Edition, "unknown"), cleanLabel(info.Version, "unknown"), cleanLabel(info.Commit, "unknown")).Set(1)
	return m
}

func (m *GatewayMetrics) Gatherer() prometheus.Gatherer {
	if m == nil || m.registry == nil {
		return prometheus.NewRegistry()
	}
	return m.registry
}

func (m *GatewayMetrics) ObserveS3(obs S3Observation) {
	if m == nil {
		return
	}
	api := cleanLabel(obs.API, "Unknown")
	statusClass := statusClass(obs.StatusCode)
	m.s3Requests.WithLabelValues(api, statusClass).Inc()
	if obs.Duration >= 0 {
		m.s3Latency.WithLabelValues(api, statusClass).Observe(obs.Duration.Seconds())
	}
	m.recordLayer("s3", api, statusClass, obs.Duration, s3ObservationBytes(obs), 1)
	if obs.ErrorCode != "" {
		errorCode := cleanLabel(obs.ErrorCode, "Unknown")
		m.s3Errors.WithLabelValues(api, statusClass, errorCode).Inc()
		m.mu.Lock()
		m.s3ErrorStats[s3ErrorStatsKey{api: api, statusClass: statusClass, errorCode: errorCode}]++
		m.mu.Unlock()
	}
	if obs.RequestBytes > 0 {
		m.s3Bytes.WithLabelValues("in", api).Add(float64(obs.RequestBytes))
	}
	if obs.ResponseBytes > 0 {
		m.s3Bytes.WithLabelValues("out", api).Add(float64(obs.ResponseBytes))
	}
}

func (m *GatewayMetrics) AddActiveS3(api string, delta float64) {
	if m == nil {
		return
	}
	m.s3Active.WithLabelValues(cleanLabel(api, "Unknown")).Add(delta)
}

func (m *GatewayMetrics) ObserveS3FirstByte(api string, statusCode int, duration time.Duration) {
	if m == nil {
		return
	}
	api = cleanLabel(api, "Unknown")
	statusClass := statusClass(statusCode)
	if duration < 0 {
		duration = 0
	}
	m.s3FirstByte.WithLabelValues(api, statusClass).Observe(duration.Seconds())
	m.recordLayer("s3_first_byte", api, statusClass, duration, 0, 1)
}

func (m *GatewayMetrics) ObserveS3RequestBodyRead(api string, statusCode int, duration time.Duration) {
	if m == nil {
		return
	}
	api = cleanLabel(api, "Unknown")
	statusClass := statusClass(statusCode)
	if duration < 0 {
		duration = 0
	}
	m.s3RequestBodyRead.WithLabelValues(api, statusClass).Observe(duration.Seconds())
	m.recordLayer("s3_request_body_read", api, statusClass, duration, 0, 1)
}

func (m *GatewayMetrics) ObserveMetadata(operation string, err error) {
	if m == nil {
		return
	}
	status := "ok"
	if err != nil {
		status = "error"
	}
	m.metadataOps.WithLabelValues(cleanLabel(operation, "unknown"), status).Inc()
	m.recordLayer("metadata", cleanLabel(operation, "unknown"), status, 0, 0, 1)
}

func (m *GatewayMetrics) ObserveMetadataDuration(operation string, duration time.Duration, err error) {
	if m == nil {
		return
	}
	operation = cleanLabel(operation, "unknown")
	status := errorStatus(err)
	m.metadataOps.WithLabelValues(operation, status).Inc()
	m.metadataDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
	m.recordLayer("metadata", operation, status, duration, 0, 1)
}

func (m *GatewayMetrics) ObserveAdmissionRejection(kind, reason string) {
	if m == nil {
		return
	}
	kind = cleanLabel(kind, "unknown")
	reason = cleanLabel(reason, "unknown")
	m.admissions.WithLabelValues(kind, reason).Inc()
	m.mu.Lock()
	m.admissionStats[admissionStatsKey{kind: kind, reason: reason}]++
	m.mu.Unlock()
}

func (m *GatewayMetrics) ObserveAdmissionDecision(kind, reason string, accepted bool, duration time.Duration) {
	if m == nil {
		return
	}
	kind = cleanLabel(kind, "unknown")
	reason = cleanLabel(reason, "allowed")
	outcome := "accepted"
	if !accepted {
		outcome = "rejected"
		m.ObserveAdmissionRejection(kind, reason)
	}
	if duration < 0 {
		duration = 0
	}
	m.admissionDuration.WithLabelValues(kind, outcome, reason).Observe(duration.Seconds())
	m.recordLayer("admission", kind, outcome, duration, 0, 1)
}

func (m *GatewayMetrics) SetCoordinationLeaseFresh(gatewayID string, fresh bool) {
	if m == nil || strings.TrimSpace(gatewayID) == "" {
		return
	}
	value := 0.0
	if fresh {
		value = 1
	}
	m.leaseFreshness.WithLabelValues(cleanLabel(gatewayID, "unknown")).Set(value)
}

func (m *GatewayMetrics) ObserveStorage(operation, backend string, duration time.Duration, bytes uint64, err error) {
	if m == nil {
		return
	}
	operation = cleanLabel(operation, "unknown")
	backend = cleanLabel(backend, "unknown")
	status := errorStatus(err)
	m.storageDuration.WithLabelValues(backend, operation, status).Observe(duration.Seconds())
	if bytes > 0 {
		direction := "unknown"
		switch operation {
		case "put_segment":
			direction = "in"
		case "get_segment":
			direction = "out"
		}
		m.storageBytes.WithLabelValues(backend, operation, direction, status).Add(float64(bytes))
	}
	m.recordLayer("storage:"+backend, operation, status, duration, bytes, 1)
}

func (m *GatewayMetrics) ObserveSBSPhysicalAllocation(duration time.Duration, chunkCount uint32, err error) {
	if m == nil {
		return
	}
	status := errorStatus(err)
	m.sbsAllocation.WithLabelValues(status).Observe(duration.Seconds())
	if chunkCount > 0 {
		m.sbsAllocationChunks.WithLabelValues(status).Add(float64(chunkCount))
	}
	m.recordLayer("sbs_physical", "allocate_chunk_ids", status, duration, 0, uint64(chunkCount))
}

func (m *GatewayMetrics) ObserveSBSPhysicalChunk(operation string, duration time.Duration, bytes uint64, err error) {
	if m == nil {
		return
	}
	operation = cleanLabel(operation, "unknown")
	status := errorStatus(err)
	m.sbsPhysicalDuration.WithLabelValues(operation, status).Observe(duration.Seconds())
	if bytes > 0 {
		direction := "unknown"
		switch operation {
		case "write":
			direction = "out"
		case "read":
			direction = "in"
		}
		m.sbsPhysicalBytes.WithLabelValues(operation, direction, status).Add(float64(bytes))
	}
	m.recordLayer("sbs_physical", "chunk_"+operation, status, duration, bytes, 1)
}

func (m *GatewayMetrics) ObserveSBSPhysicalReadback(duration time.Duration, bytes uint64, err error) {
	if m == nil {
		return
	}
	status := errorStatus(err)
	m.sbsReadback.WithLabelValues(status).Observe(duration.Seconds())
	m.recordLayer("sbs_physical", "write_readback", status, duration, bytes, 1)
}

func (m *GatewayMetrics) ObserveSBSECShard(operation, storeID, role string, _ uint32, duration time.Duration, bytes uint64, err error) {
	if m == nil {
		return
	}
	operation = cleanLabel(operation, "unknown")
	storeID = cleanLabel(storeID, "unknown")
	role = cleanLabel(role, "unknown")
	status := errorStatus(err)
	m.sbsECDuration.WithLabelValues(operation, storeID, role, status).Observe(duration.Seconds())
	if bytes > 0 {
		direction := "unknown"
		switch operation {
		case "write":
			direction = "out"
		case "read":
			direction = "in"
		}
		m.sbsECBytes.WithLabelValues(operation, storeID, role, direction, status).Add(float64(bytes))
	}
	m.recordLayer("sbs_ec:"+storeID, "shard_"+operation+"_"+role, status, duration, bytes, 1)
}

func (m *GatewayMetrics) SetSBSVolumePoolStatus(obs SBSVolumePoolObservation) {
	if m == nil {
		return
	}
	poolID := cleanLabel(obs.PoolID, "default")
	source := cleanLabel(obs.Source, "unknown")
	staleSeconds := obs.StaleSeconds
	if staleSeconds < 0 {
		staleSeconds = 0
	}
	m.sbsPoolConfigured.WithLabelValues(poolID, source).Set(float64(obs.ConfiguredGeneration))
	m.sbsPoolActive.WithLabelValues(poolID, source).Set(float64(obs.ActiveGeneration))
	m.sbsPoolErrors.WithLabelValues(poolID, source).Set(float64(obs.RefreshErrorCount))
	m.sbsPoolStale.WithLabelValues(poolID, source).Set(staleSeconds)
}

func (m *GatewayMetrics) SetWorkerBacklog(snapshot workerops.BacklogSnapshot) {
	if m == nil {
		return
	}
	m.workerBacklog.Reset()
	m.workerOperations.Reset()
	m.workerRetryable.Reset()
	m.workerLeaseAge.Reset()
	m.workerLeaseFresh.Reset()
	for _, worker := range snapshot.Workers {
		kind := cleanLabel(worker.WorkerKind, "unknown")
		shardID := cleanLabel(worker.ShardID, "default")
		m.workerBacklog.WithLabelValues(kind, shardID).Set(float64(worker.BacklogOperations))
		for status, count := range map[string]int{
			"canceled":      worker.CanceledOperations,
			"failed":        worker.FailedOperations,
			"paused":        worker.PausedOperations,
			"retry_pending": worker.RetryPendingOperations,
			"running":       worker.RunningOperations,
			"succeeded":     worker.SucceededOperations,
		} {
			m.workerOperations.WithLabelValues(kind, shardID, status).Set(float64(count))
		}
		m.workerRetryable.WithLabelValues(kind, shardID).Set(float64(worker.Retryable))
		if worker.LeaseID == "" {
			continue
		}
		ownerID := cleanLabel(worker.OwnerID, "unowned")
		fresh := 0.0
		if worker.LeaseFresh {
			fresh = 1
		}
		m.workerLeaseAge.WithLabelValues(kind, shardID, ownerID).Set(worker.LeaseAgeSeconds)
		m.workerLeaseFresh.WithLabelValues(kind, shardID, ownerID).Set(fresh)
	}
}

func (m *GatewayMetrics) Snapshot() MetricsSnapshot {
	out := MetricsSnapshot{
		SchemaVersion: "namros.gateway.metrics.snapshot.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}
	if m == nil {
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out.Layers = make([]LayerMetricsSnapshot, 0, len(m.layerStats))
	for key, stat := range m.layerStats {
		if stat == nil || stat.count == 0 {
			continue
		}
		avg := stat.total / time.Duration(stat.count)
		out.Layers = append(out.Layers, LayerMetricsSnapshot{
			Component: key.component,
			Operation: key.operation,
			Status:    key.status,
			Count:     stat.count,
			TotalMs:   durationMillis(stat.total),
			AvgMs:     durationMillis(avg),
			MinMs:     durationMillis(stat.min),
			MaxMs:     durationMillis(stat.max),
			Bytes:     stat.bytes,
			Units:     stat.units,
		})
	}
	sort.Slice(out.Layers, func(i, j int) bool {
		if out.Layers[i].Component != out.Layers[j].Component {
			return out.Layers[i].Component < out.Layers[j].Component
		}
		if out.Layers[i].Operation != out.Layers[j].Operation {
			return out.Layers[i].Operation < out.Layers[j].Operation
		}
		return out.Layers[i].Status < out.Layers[j].Status
	})
	out.Admissions = make([]AdmissionSnapshot, 0, len(m.admissionStats))
	for key, count := range m.admissionStats {
		if count == 0 {
			continue
		}
		out.Admissions = append(out.Admissions, AdmissionSnapshot{
			Kind:   key.kind,
			Reason: key.reason,
			Count:  count,
		})
	}
	sort.Slice(out.Admissions, func(i, j int) bool {
		if out.Admissions[i].Kind != out.Admissions[j].Kind {
			return out.Admissions[i].Kind < out.Admissions[j].Kind
		}
		return out.Admissions[i].Reason < out.Admissions[j].Reason
	})
	out.S3Errors = make([]S3ErrorSnapshot, 0, len(m.s3ErrorStats))
	for key, count := range m.s3ErrorStats {
		if count == 0 {
			continue
		}
		out.S3Errors = append(out.S3Errors, S3ErrorSnapshot{
			API:         key.api,
			StatusClass: key.statusClass,
			ErrorCode:   key.errorCode,
			Count:       count,
		})
	}
	sort.Slice(out.S3Errors, func(i, j int) bool {
		if out.S3Errors[i].API != out.S3Errors[j].API {
			return out.S3Errors[i].API < out.S3Errors[j].API
		}
		if out.S3Errors[i].StatusClass != out.S3Errors[j].StatusClass {
			return out.S3Errors[i].StatusClass < out.S3Errors[j].StatusClass
		}
		return out.S3Errors[i].ErrorCode < out.S3Errors[j].ErrorCode
	})
	return out
}

func (m *GatewayMetrics) recordLayer(component, operation, status string, duration time.Duration, bytes, units uint64) {
	if m == nil {
		return
	}
	component = cleanLabel(component, "unknown")
	operation = cleanLabel(operation, "unknown")
	status = cleanLabel(status, "unknown")
	if duration < 0 {
		duration = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := layerStatsKey{component: component, operation: operation, status: status}
	stat := m.layerStats[key]
	if stat == nil {
		stat = &layerStats{min: duration}
		m.layerStats[key] = stat
	}
	stat.count++
	stat.total += duration
	if stat.count == 1 || duration < stat.min {
		stat.min = duration
	}
	if duration > stat.max {
		stat.max = duration
	}
	stat.bytes += bytes
	stat.units += units
}

func statusClass(code int) string {
	if code <= 0 {
		code = 200
	}
	return strconv.Itoa(code/100) + "xx"
}

func errorStatus(err error) string {
	if err != nil {
		return "error"
	}
	return "ok"
}

func s3ObservationBytes(obs S3Observation) uint64 {
	var total uint64
	if obs.RequestBytes > 0 {
		total += uint64(obs.RequestBytes)
	}
	if obs.ResponseBytes > 0 {
		total += uint64(obs.ResponseBytes)
	}
	return total
}

func durationMillis(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000
}

func latencyBuckets() []float64 {
	return []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.15, 0.25, 0.5, 1, 2.5, 5, 10}
}

func cleanLabel(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}

package tikv

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nosway/namros/internal/meta"
)

const (
	OperationGet     = "get"
	OperationSet     = "set"
	OperationDelete  = "delete"
	OperationList    = "list"
	OperationTxn     = "transaction"
	OperationCommit  = "transaction_commit"
	OperationAttempt = "transaction_attempt"

	DefaultHotspotShardCount = 64
	DefaultHotspotMaxRanges  = 2048
	DefaultHotspotTopN       = 16
)

type MetricsOption func(*Metrics)

type Metrics struct {
	mu sync.Mutex

	shardCount uint32
	maxRanges  int
	topN       int
	operations map[string]OperationMetrics
	ranges     map[string]*HotspotRangeMetrics
	retry      RetryMetrics
}

type OperationMetrics struct {
	Calls           uint64  `json:"calls"`
	Errors          uint64  `json:"errors,omitempty"`
	KeysReturned    uint64  `json:"keys_returned,omitempty"`
	DurationSamples uint64  `json:"duration_samples,omitempty"`
	TotalMs         float64 `json:"total_ms,omitempty"`
	AvgMs           float64 `json:"avg_ms,omitempty"`
	MinMs           float64 `json:"min_ms,omitempty"`
	MaxMs           float64 `json:"max_ms,omitempty"`
}

type RetryObservation struct {
	MaxAttempts     int
	Attempts        int
	RetryAttempts   int
	WriteConflicts  int
	TransientErrors int
	Backoff         time.Duration
	Exhausted       bool
	FinalError      error
}

type RetryMetrics struct {
	Transactions        uint64            `json:"transactions"`
	Attempts            uint64            `json:"attempts"`
	RetriedTransactions uint64            `json:"retried_transactions,omitempty"`
	RetryAttempts       uint64            `json:"retry_attempts,omitempty"`
	WriteConflicts      uint64            `json:"write_conflicts,omitempty"`
	TransientErrors     uint64            `json:"transient_errors,omitempty"`
	Exhausted           uint64            `json:"exhausted,omitempty"`
	BackoffMs           float64           `json:"backoff_ms,omitempty"`
	MaxAttemptsObserved uint64            `json:"max_attempts_observed,omitempty"`
	FinalStatuses       map[string]uint64 `json:"final_statuses,omitempty"`
}

type HotspotRangeMetrics struct {
	Range      string `json:"range"`
	HashShard  string `json:"hash_shard"`
	Operations uint64 `json:"operations"`
	Reads      uint64 `json:"reads,omitempty"`
	Writes     uint64 `json:"writes,omitempty"`
	Deletes    uint64 `json:"deletes,omitempty"`
	Lists      uint64 `json:"lists,omitempty"`
	Errors     uint64 `json:"errors,omitempty"`
}

type MetricsSnapshot struct {
	HotspotShardCount int                         `json:"hotspot_shard_count"`
	HotspotMaxRanges  int                         `json:"hotspot_max_ranges"`
	HotspotTopN       int                         `json:"hotspot_top_n"`
	Operations        map[string]OperationMetrics `json:"operations"`
	HotspotRanges     []HotspotRangeMetrics       `json:"hotspot_ranges"`
	Retry             RetryMetrics                `json:"retry"`
}

func NewMetrics(options ...MetricsOption) *Metrics {
	m := &Metrics{
		shardCount: DefaultHotspotShardCount,
		maxRanges:  DefaultHotspotMaxRanges,
		topN:       DefaultHotspotTopN,
		operations: make(map[string]OperationMetrics),
		ranges:     make(map[string]*HotspotRangeMetrics),
	}
	for _, option := range options {
		option(m)
	}
	if m.shardCount == 0 {
		m.shardCount = DefaultHotspotShardCount
	}
	if m.maxRanges <= 0 {
		m.maxRanges = DefaultHotspotMaxRanges
	}
	if m.topN <= 0 {
		m.topN = DefaultHotspotTopN
	}
	return m
}

func WithHotspotShardCount(count int) MetricsOption {
	return func(m *Metrics) {
		if count > 0 {
			m.shardCount = uint32(count)
		}
	}
}

func WithHotspotMaxRanges(maxRanges int) MetricsOption {
	return func(m *Metrics) {
		if maxRanges > 0 {
			m.maxRanges = maxRanges
		}
	}
}

func WithHotspotTopN(topN int) MetricsOption {
	return func(m *Metrics) {
		if topN > 0 {
			m.topN = topN
		}
	}
}

func (m *Metrics) ObserveOperation(operation, key string, err error) {
	if m == nil {
		return
	}
	m.observe(operation, key, 0, -1, err)
}

func (m *Metrics) ObserveOperationDuration(operation, key string, duration time.Duration, err error) {
	if m == nil {
		return
	}
	m.observe(operation, key, 0, duration, err)
}

func (m *Metrics) ObserveList(prefix string, keysReturned int, err error) {
	if m == nil {
		return
	}
	m.observe(OperationList, prefix, keysReturned, -1, err)
}

func (m *Metrics) ObserveListDuration(prefix string, keysReturned int, duration time.Duration, err error) {
	if m == nil {
		return
	}
	m.observe(OperationList, prefix, keysReturned, duration, err)
}

func (m *Metrics) ObserveRetry(obs RetryObservation) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()

	m.retry.Transactions++
	if obs.Attempts > 0 {
		m.retry.Attempts += uint64(obs.Attempts)
	}
	if obs.RetryAttempts > 0 {
		m.retry.RetriedTransactions++
		m.retry.RetryAttempts += uint64(obs.RetryAttempts)
	}
	if obs.WriteConflicts > 0 {
		m.retry.WriteConflicts += uint64(obs.WriteConflicts)
	}
	if obs.TransientErrors > 0 {
		m.retry.TransientErrors += uint64(obs.TransientErrors)
	}
	if obs.Exhausted {
		m.retry.Exhausted++
	}
	if obs.Backoff > 0 {
		m.retry.BackoffMs += obs.Backoff.Seconds() * 1000
	}
	if obs.MaxAttempts > 0 && uint64(obs.MaxAttempts) > m.retry.MaxAttemptsObserved {
		m.retry.MaxAttemptsObserved = uint64(obs.MaxAttempts)
	}
	m.retry.FinalStatuses[retryFinalStatus(obs.FinalError)]++
}

func (m *Metrics) observe(operation, key string, keysReturned int, duration time.Duration, err error) {
	if operation == "" {
		operation = "unknown"
	}
	keyRange, shard := hotspotRange(key, m.shardCount)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()

	op := m.operations[operation]
	op.Calls++
	if err != nil {
		op.Errors++
	}
	if keysReturned > 0 {
		op.KeysReturned += uint64(keysReturned)
	}
	if duration >= 0 {
		ms := duration.Seconds() * 1000
		op.DurationSamples++
		op.TotalMs += ms
		op.AvgMs = op.TotalMs / float64(op.DurationSamples)
		if op.DurationSamples == 1 || ms < op.MinMs {
			op.MinMs = ms
		}
		if ms > op.MaxMs {
			op.MaxMs = ms
		}
	}
	m.operations[operation] = op

	stats := m.rangeStatsLocked(keyRange, shard)
	stats.Operations++
	if err != nil {
		stats.Errors++
	}
	switch operation {
	case OperationGet:
		stats.Reads++
	case OperationSet:
		stats.Writes++
	case OperationDelete:
		stats.Deletes++
	case OperationList:
		stats.Lists++
	}
}

func (m *Metrics) Snapshot() MetricsSnapshot {
	if m == nil {
		return MetricsSnapshot{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLocked()

	operations := make(map[string]OperationMetrics, len(m.operations))
	for key, value := range m.operations {
		operations[key] = value
	}
	retry := m.retry
	if len(m.retry.FinalStatuses) > 0 {
		retry.FinalStatuses = make(map[string]uint64, len(m.retry.FinalStatuses))
		for key, value := range m.retry.FinalStatuses {
			retry.FinalStatuses[key] = value
		}
	}
	ranges := make([]HotspotRangeMetrics, 0, len(m.ranges))
	for _, value := range m.ranges {
		ranges = append(ranges, *value)
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].Operations != ranges[j].Operations {
			return ranges[i].Operations > ranges[j].Operations
		}
		if ranges[i].Range != ranges[j].Range {
			return ranges[i].Range < ranges[j].Range
		}
		return ranges[i].HashShard < ranges[j].HashShard
	})
	if len(ranges) > m.topN {
		ranges = ranges[:m.topN]
	}
	return MetricsSnapshot{
		HotspotShardCount: int(m.shardCount),
		HotspotMaxRanges:  m.maxRanges,
		HotspotTopN:       m.topN,
		Operations:        operations,
		HotspotRanges:     ranges,
		Retry:             retry,
	}
}

func (m *Metrics) ensureLocked() {
	if m.shardCount == 0 {
		m.shardCount = DefaultHotspotShardCount
	}
	if m.maxRanges <= 0 {
		m.maxRanges = DefaultHotspotMaxRanges
	}
	if m.topN <= 0 {
		m.topN = DefaultHotspotTopN
	}
	if m.operations == nil {
		m.operations = make(map[string]OperationMetrics)
	}
	if m.ranges == nil {
		m.ranges = make(map[string]*HotspotRangeMetrics)
	}
	if m.retry.FinalStatuses == nil {
		m.retry.FinalStatuses = make(map[string]uint64)
	}
}

func (m *Metrics) rangeStatsLocked(keyRange, shard string) *HotspotRangeMetrics {
	mapKey := keyRange + "|" + shard
	stats := m.ranges[mapKey]
	if stats != nil {
		return stats
	}
	if len(m.ranges) >= m.maxRanges {
		keyRange = "overflow"
		shard = "overflow"
		mapKey = keyRange + "|" + shard
		if stats = m.ranges[mapKey]; stats != nil {
			return stats
		}
	}
	stats = &HotspotRangeMetrics{
		Range:     keyRange,
		HashShard: shard,
	}
	m.ranges[mapKey] = stats
	return stats
}

func retryFinalStatus(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, meta.ErrCASConflict):
		return "cas_conflict"
	case errors.Is(err, meta.ErrUnavailable):
		return "unavailable"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "error"
	}
}

type instrumentedKV struct {
	base    KV
	metrics *Metrics
}

type instrumentedReadWriter struct {
	base    ReadWriter
	metrics *Metrics
}

func newInstrumentedKV(base KV, metrics *Metrics) KV {
	if metrics == nil {
		return base
	}
	return &instrumentedKV{base: base, metrics: metrics}
}

func (kv *instrumentedKV) Get(ctx context.Context, key string) ([]byte, bool, error) {
	start := time.Now()
	value, found, err := kv.base.Get(ctx, key)
	kv.metrics.ObserveOperationDuration(OperationGet, key, time.Since(start), err)
	return value, found, err
}

func (kv *instrumentedKV) Set(ctx context.Context, key string, value []byte) error {
	start := time.Now()
	err := kv.base.Set(ctx, key, value)
	kv.metrics.ObserveOperationDuration(OperationSet, key, time.Since(start), err)
	return err
}

func (kv *instrumentedKV) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := kv.base.Delete(ctx, key)
	kv.metrics.ObserveOperationDuration(OperationDelete, key, time.Since(start), err)
	return err
}

func (kv *instrumentedKV) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	start := time.Now()
	keys, next, err := kv.base.List(ctx, prefix, cursor, limit)
	kv.metrics.ObserveListDuration(prefix, len(keys), time.Since(start), err)
	return keys, next, err
}

func (kv *instrumentedKV) ListRange(ctx context.Context, rangeStart, end, cursor string, limit int) ([]string, string, error) {
	start := time.Now()
	keys, next, err := kv.base.ListRange(ctx, rangeStart, end, cursor, limit)
	kv.metrics.ObserveListDuration(rangeStart, len(keys), time.Since(start), err)
	return keys, next, err
}

func (kv *instrumentedKV) RunInTransaction(ctx context.Context, fn func(tx ReadWriter) error) error {
	start := time.Now()
	err := RunInTransaction(ctx, kv.base, func(tx ReadWriter) error {
		return fn(&instrumentedReadWriter{base: tx, metrics: kv.metrics})
	})
	kv.metrics.ObserveOperationDuration(OperationTxn, "/namros/v1/tikv/transactions", time.Since(start), transactionMetricError(err))
	return err
}

func (tx *instrumentedReadWriter) Get(ctx context.Context, key string) ([]byte, bool, error) {
	start := time.Now()
	value, found, err := tx.base.Get(ctx, key)
	tx.metrics.ObserveOperationDuration(OperationGet, key, time.Since(start), err)
	return value, found, err
}

func (tx *instrumentedReadWriter) Set(ctx context.Context, key string, value []byte) error {
	start := time.Now()
	err := tx.base.Set(ctx, key, value)
	tx.metrics.ObserveOperationDuration(OperationSet, key, time.Since(start), err)
	return err
}

func (tx *instrumentedReadWriter) Delete(ctx context.Context, key string) error {
	start := time.Now()
	err := tx.base.Delete(ctx, key)
	tx.metrics.ObserveOperationDuration(OperationDelete, key, time.Since(start), err)
	return err
}

func (tx *instrumentedReadWriter) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	start := time.Now()
	keys, next, err := tx.base.List(ctx, prefix, cursor, limit)
	tx.metrics.ObserveListDuration(prefix, len(keys), time.Since(start), err)
	return keys, next, err
}

func (tx *instrumentedReadWriter) ListRange(ctx context.Context, rangeStart, end, cursor string, limit int) ([]string, string, error) {
	start := time.Now()
	keys, next, err := tx.base.ListRange(ctx, rangeStart, end, cursor, limit)
	tx.metrics.ObserveListDuration(rangeStart, len(keys), time.Since(start), err)
	return keys, next, err
}

func hotspotRange(key string, shardCount uint32) (string, string) {
	logicalKey := stripTiKVKeyspacePrefix(key)
	if shardCount == 0 {
		shardCount = DefaultHotspotShardCount
	}
	return hotspotFamily(logicalKey), fmt.Sprintf("hash-%02d", hashKey(logicalKey)%shardCount)
}

func stripTiKVKeyspacePrefix(key string) string {
	const prefix = "keyspaces/"
	if !strings.HasPrefix(key, prefix) {
		return key
	}
	rest := strings.TrimPrefix(key, prefix)
	index := strings.IndexByte(rest, '/')
	if index < 0 {
		return key
	}
	return rest[index+1:]
}

func hotspotFamily(key string) string {
	switch {
	case strings.HasPrefix(key, "/namros/v1/tenants/"):
		return "/namros/v1/tenants"
	case strings.HasPrefix(key, "/namros/v1/access-keys/"):
		return "/namros/v1/access-keys"
	case strings.HasPrefix(key, "/namros/v1/buckets/by-name/"):
		return "/namros/v1/buckets/by-name"
	case strings.HasPrefix(key, "/namros/v1/buckets/by-id/"):
		return "/namros/v1/buckets/by-id"
	case strings.HasPrefix(key, "/namros/v1/sequences/"):
		return "/namros/v1/sequences"
	case strings.HasPrefix(key, "/namros/v1/idempotency/"):
		return "/namros/v1/idempotency"
	case strings.HasPrefix(key, "/namros/v1/tikv/transactions"):
		return "/namros/v1/tikv/transactions"
	case strings.HasPrefix(key, "/namros/v1/gc/orphans/"):
		return "/namros/v1/gc/orphans"
	case strings.HasPrefix(key, "/namros/v1/buckets/"):
		return bucketHotspotFamily(key)
	default:
		return "unknown"
	}
}

func bucketHotspotFamily(key string) string {
	rest := strings.TrimPrefix(key, "/namros/v1/buckets/")
	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 || parts[0] == "" {
		return "/namros/v1/buckets"
	}
	switch parts[1] {
	case "objects", "versions", "list", "multipart":
		return "/namros/v1/buckets/" + parts[0] + "/" + parts[1]
	default:
		return "/namros/v1/buckets/" + parts[0]
	}
}

func hashKey(key string) uint32 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return hash.Sum32()
}

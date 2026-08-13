package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/pebble"
	"github.com/nosway/namros/internal/meta/tikv"
)

const (
	defaultRegion          = "us-east-1"
	defaultAccessKeyID     = "namrosroot"
	defaultSecretAccessKey = "namrosrootsecret"
	unsignedPayload        = "UNSIGNED-PAYLOAD"
)

type benchConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	BucketPrefix    string
	KeyPrefix       string
	StorageClass    string
	SmallCount      int
	SmallSize       int
	LargeCount      int
	LargeSize       int
	RangeSize       int
	ListRepetitions int
	Timeout         time.Duration
	Cleanup         bool
	FailFast        bool
	OutputJSONL     string
	SummaryJSON     string
	MemoryJSONL     string
	MemoryInterval  time.Duration
}

type metadataListBenchConfig struct {
	MetadataBackend         string
	MetadataPath            string
	TiKVPDEndpoints         []string
	TiKVAPIVersion          string
	TiKVKeyspace            string
	TiKVTimeout             time.Duration
	TiKVTLSCA               string
	TiKVTLSCert             string
	TiKVTLSKey              string
	TiKVRetryAttempts       int
	TiKVRetryInitialBackoff time.Duration
	TiKVRetryMaxBackoff     time.Duration
	TenantID                string
	Bucket                  string
	BucketPrefix            string
	KeyPrefix               string
	ObjectCount             int
	PageSize                int
	MaxPages                int
	SummaryJSON             string
	PageJSONL               string
	FailOnGate              bool
}

type noisyTenantProfileConfig struct {
	NoisyTenantID          string
	NeighborTenantID       string
	MaxConcurrentGlobal    int
	MaxConcurrentPerTenant int
	NoisyHoldRequests      int
	NoisyAttempts          int
	NeighborAttempts       int
	SummaryJSON            string
	EventJSONL             string
	FailOnGate             bool
}

type benchResult struct {
	Schema        string  `json:"schema"`
	TimestampUnix int64   `json:"timestamp_unix"`
	Phase         string  `json:"phase"`
	Operation     string  `json:"operation"`
	Iteration     int     `json:"iteration"`
	Bucket        string  `json:"bucket"`
	Key           string  `json:"key,omitempty"`
	Status        string  `json:"status"`
	StatusCode    int     `json:"status_code"`
	DurationMS    float64 `json:"duration_ms"`
	RequestBytes  int64   `json:"request_bytes"`
	ResponseBytes int64   `json:"response_bytes"`
	StorageClass  string  `json:"storage_class,omitempty"`
	Error         string  `json:"error,omitempty"`
}

type operationSummary struct {
	Operation       string  `json:"operation"`
	Count           int     `json:"count"`
	Errors          int     `json:"errors"`
	TotalDurationMS float64 `json:"total_duration_ms"`
	AvgDurationMS   float64 `json:"avg_duration_ms"`
	MinDurationMS   float64 `json:"min_duration_ms"`
	MaxDurationMS   float64 `json:"max_duration_ms"`
	P50DurationMS   float64 `json:"p50_duration_ms"`
	P95DurationMS   float64 `json:"p95_duration_ms"`
	RequestBytes    int64   `json:"request_bytes"`
	ResponseBytes   int64   `json:"response_bytes"`
}

type benchSummary struct {
	Schema        string              `json:"schema"`
	Endpoint      string              `json:"endpoint"`
	Region        string              `json:"region"`
	Bucket        string              `json:"bucket"`
	KeyPrefix     string              `json:"key_prefix"`
	StorageClass  string              `json:"storage_class,omitempty"`
	StartedAt     string              `json:"started_at"`
	FinishedAt    string              `json:"finished_at"`
	ElapsedMS     float64             `json:"elapsed_ms"`
	Operations    []operationSummary  `json:"operations"`
	Errors        []benchResult       `json:"errors,omitempty"`
	ResultJSONL   string              `json:"result_jsonl,omitempty"`
	SmallCount    int                 `json:"small_count"`
	SmallSize     int                 `json:"small_size"`
	LargeCount    int                 `json:"large_count"`
	LargeSize     int                 `json:"large_size"`
	RangeSize     int                 `json:"range_size"`
	HTTPKeepAlive bool                `json:"http_keep_alive"`
	MemoryJSONL   string              `json:"memory_jsonl,omitempty"`
	Memory        *benchMemorySummary `json:"memory,omitempty"`
}

type benchMemorySummary struct {
	Samples            int    `json:"samples"`
	RSSSource          string `json:"rss_source,omitempty"`
	PeakRSSBytes       uint64 `json:"peak_rss_bytes,omitempty"`
	PeakHeapAllocBytes uint64 `json:"peak_heap_alloc_bytes"`
	PeakHeapSysBytes   uint64 `json:"peak_heap_sys_bytes"`
	PeakStackBytes     uint64 `json:"peak_stack_bytes"`
	PeakGoroutines     int    `json:"peak_goroutines"`
}

type metadataListBenchPage struct {
	Page                   int                              `json:"page"`
	RequestedMaxKeys       int                              `json:"requested_max_keys"`
	Objects                int                              `json:"objects"`
	CommonPrefixes         int                              `json:"common_prefixes"`
	DurationMS             float64                          `json:"duration_ms"`
	IsTruncated            bool                             `json:"is_truncated"`
	ContinuationToken      string                           `json:"continuation_token,omitempty"`
	NextContinuationToken  string                           `json:"next_continuation_token,omitempty"`
	RepositoryReadEstimate int                              `json:"repository_read_estimate"`
	TiKVOperations         *metadataListBenchTiKVOperations `json:"tikv_operations,omitempty"`
}

type metadataListBenchTiKVOperations struct {
	ListCalls        uint64  `json:"list_calls"`
	ListKeysReturned uint64  `json:"list_keys_returned"`
	GetCalls         uint64  `json:"get_calls"`
	TotalReadCalls   uint64  `json:"total_read_calls"`
	ListDurationMS   float64 `json:"list_duration_ms"`
	GetDurationMS    float64 `json:"get_duration_ms"`
}

type metadataListBenchGate struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type metadataListBenchSummary struct {
	SchemaVersion       string                  `json:"schema_version"`
	Status              string                  `json:"status"`
	MetadataBackend     string                  `json:"metadata_backend"`
	MetadataPath        string                  `json:"metadata_path,omitempty"`
	Bucket              string                  `json:"bucket"`
	BucketID            string                  `json:"bucket_id"`
	KeyPrefix           string                  `json:"key_prefix"`
	ObjectCount         int                     `json:"object_count"`
	PageSize            int                     `json:"page_size"`
	MaxPages            int                     `json:"max_pages,omitempty"`
	SeedDurationMS      float64                 `json:"seed_duration_ms"`
	ListDurationMS      float64                 `json:"list_duration_ms"`
	Pages               []metadataListBenchPage `json:"pages"`
	PageCount           int                     `json:"page_count"`
	ListedObjects       int                     `json:"listed_objects"`
	AvgPageDurationMS   float64                 `json:"avg_page_duration_ms"`
	P50PageDurationMS   float64                 `json:"p50_page_duration_ms"`
	P95PageDurationMS   float64                 `json:"p95_page_duration_ms"`
	MaxPageDurationMS   float64                 `json:"max_page_duration_ms"`
	MaxPageReadEstimate int                     `json:"max_page_read_estimate"`
	PageJSONL           string                  `json:"page_jsonl,omitempty"`
	TiKVMetrics         *tikv.MetricsSnapshot   `json:"tikv_metrics,omitempty"`
	Gates               []metadataListBenchGate `json:"gates"`
}

type noisyTenantProfileSummary struct {
	SchemaVersion          string                  `json:"schema_version"`
	Status                 string                  `json:"status"`
	NoisyTenantID          string                  `json:"noisy_tenant_id"`
	NeighborTenantID       string                  `json:"neighbor_tenant_id"`
	MaxConcurrentGlobal    int                     `json:"max_concurrent_global"`
	MaxConcurrentPerTenant int                     `json:"max_concurrent_per_tenant"`
	NoisyHoldRequests      int                     `json:"noisy_hold_requests"`
	NoisyAttempts          int                     `json:"noisy_attempts"`
	NeighborAttempts       int                     `json:"neighbor_attempts"`
	PeakGlobalInflight     int                     `json:"peak_global_inflight"`
	Tenants                []noisyTenantStats      `json:"tenants"`
	Gates                  []metadataListBenchGate `json:"gates"`
	EventJSONL             string                  `json:"event_jsonl,omitempty"`
	Events                 []noisyTenantEvent      `json:"events,omitempty"`
}

type noisyTenantStats struct {
	TenantID     string `json:"tenant_id"`
	Role         string `json:"role"`
	Attempted    int    `json:"attempted"`
	Admitted     int    `json:"admitted"`
	Throttled    int    `json:"throttled"`
	Completed    int    `json:"completed"`
	PeakInflight int    `json:"peak_inflight"`
}

type noisyTenantEvent struct {
	Schema         string `json:"schema"`
	Sequence       int    `json:"sequence"`
	TenantID       string `json:"tenant_id"`
	Role           string `json:"role"`
	Operation      string `json:"operation"`
	Iteration      int    `json:"iteration"`
	Action         string `json:"action"`
	Status         string `json:"status"`
	Reason         string `json:"reason,omitempty"`
	GlobalInflight int    `json:"global_inflight"`
	TenantInflight int    `json:"tenant_inflight"`
	MaxGlobal      int    `json:"max_global"`
	MaxTenant      int    `json:"max_tenant"`
	NeighborSafe   bool   `json:"neighbor_safe,omitempty"`
	NoisyThrottled bool   `json:"noisy_throttled,omitempty"`
}

type memorySample struct {
	Schema             string  `json:"schema"`
	TimestampUnixNano  int64   `json:"timestamp_unix_nano"`
	ElapsedMS          float64 `json:"elapsed_ms"`
	Phase              string  `json:"phase"`
	RSSBytes           uint64  `json:"rss_bytes,omitempty"`
	RSSSource          string  `json:"rss_source,omitempty"`
	HeapAllocBytes     uint64  `json:"heap_alloc_bytes"`
	HeapSysBytes       uint64  `json:"heap_sys_bytes"`
	StackInuseBytes    uint64  `json:"stack_inuse_bytes"`
	Goroutines         int     `json:"goroutines"`
	RecordedOperations int     `json:"recorded_operations"`
	ErrorCount         int     `json:"error_count"`
	RequestBytes       int64   `json:"request_bytes"`
	ResponseBytes      int64   `json:"response_bytes"`
}

type benchRunner struct {
	cfg        benchConfig
	endpoint   *url.URL
	httpClient *http.Client
	jsonl      *json.Encoder
	jsonlFile  *os.File
	mu         sync.Mutex
	results    []benchResult
	keys       []string
	memory     benchMemorySummary
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "metadata-list-index" {
		cfg, err := parseMetadataListBenchFlags(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := runMetadataListIndexBench(context.Background(), cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "noisy-tenant-profile" {
		cfg, err := parseNoisyTenantProfileFlags(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := runNoisyTenantProfile(context.Background(), cfg); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	cfg := parseFlags()
	if err := run(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseFlags() benchConfig {
	var cfg benchConfig
	smallSize := sizeFlag(4 << 10)
	largeSize := sizeFlag(256 << 10)
	rangeSize := sizeFlag(4 << 10)
	flag.StringVar(&cfg.Endpoint, "endpoint", "http://127.0.0.1:9000", "S3 endpoint URL")
	flag.StringVar(&cfg.Region, "region", defaultRegion, "SigV4 region")
	flag.StringVar(&cfg.AccessKeyID, "access-key-id", defaultAccessKeyID, "S3 access key ID")
	flag.StringVar(&cfg.SecretAccessKey, "secret-access-key", defaultSecretAccessKey, "S3 secret access key")
	flag.StringVar(&cfg.Bucket, "bucket", "", "bucket name; generated when empty")
	flag.StringVar(&cfg.BucketPrefix, "bucket-prefix", "namros-s3bench", "generated bucket prefix")
	flag.StringVar(&cfg.KeyPrefix, "key-prefix", "bench", "object key prefix")
	flag.StringVar(&cfg.StorageClass, "storage-class", "", "optional x-amz-storage-class used for PutObject requests")
	flag.IntVar(&cfg.SmallCount, "small-count", 8, "number of small object iterations")
	flag.Var(&smallSize, "small-size", "small object size in bytes, or K/M/G suffix")
	flag.IntVar(&cfg.LargeCount, "large-count", 1, "number of large object iterations")
	flag.Var(&largeSize, "large-size", "large object size in bytes, or K/M/G suffix")
	flag.Var(&rangeSize, "range-size", "range GET size in bytes, or K/M/G suffix")
	flag.IntVar(&cfg.ListRepetitions, "list-repetitions", 3, "number of ListObjectsV2 requests")
	flag.DurationVar(&cfg.Timeout, "timeout", 30*time.Second, "per-request HTTP timeout")
	flag.BoolVar(&cfg.Cleanup, "cleanup", true, "delete benchmark objects and bucket")
	flag.BoolVar(&cfg.FailFast, "fail-fast", true, "stop on the first non-cleanup operation error")
	flag.StringVar(&cfg.OutputJSONL, "output-jsonl", "", "write per-operation results to this JSONL file")
	flag.StringVar(&cfg.SummaryJSON, "summary-json", "", "write aggregate summary to this JSON file")
	flag.StringVar(&cfg.MemoryJSONL, "memory-jsonl", "", "write periodic memory samples to this JSONL file")
	flag.DurationVar(&cfg.MemoryInterval, "memory-sample-interval", time.Second, "memory sample interval when -memory-jsonl is set")
	flag.Parse()
	cfg.SmallSize = int(smallSize)
	cfg.LargeSize = int(largeSize)
	cfg.RangeSize = int(rangeSize)
	return cfg
}

func parseMetadataListBenchFlags(args []string) (metadataListBenchConfig, error) {
	defaults := config.Default()
	cfg := metadataListBenchConfig{
		MetadataBackend:         config.MetadataBackendPebble,
		TiKVAPIVersion:          defaults.TiKVAPIVersion,
		TiKVKeyspace:            defaults.TiKVKeyspace,
		TiKVTimeout:             defaults.TiKVTimeout,
		TiKVRetryAttempts:       defaults.TiKVRetryAttempts,
		TiKVRetryInitialBackoff: defaults.TiKVRetryInitialBackoff,
		TiKVRetryMaxBackoff:     defaults.TiKVRetryMaxBackoff,
		TenantID:                "tenant-bench",
		BucketPrefix:            "namros-listbench",
		KeyPrefix:               "bench/list",
		ObjectCount:             1000000,
		PageSize:                1000,
		FailOnGate:              true,
	}
	tikvPDEndpoints := strings.Join(defaults.TiKVPDEndpoints, ",")
	fs := flag.NewFlagSet("namros-s3bench metadata-list-index", flag.ContinueOnError)
	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend: memory, pebble, or tikv")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", "", "metadata path for pebble backend; temporary when empty")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", "", "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", "", "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", "", "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.StringVar(&cfg.TenantID, "tenant-id", cfg.TenantID, "tenant id for the synthetic bucket")
	fs.StringVar(&cfg.Bucket, "bucket", "", "bucket name; generated when empty")
	fs.StringVar(&cfg.BucketPrefix, "bucket-prefix", cfg.BucketPrefix, "generated bucket prefix")
	fs.StringVar(&cfg.KeyPrefix, "key-prefix", cfg.KeyPrefix, "object key prefix to seed and list")
	fs.IntVar(&cfg.ObjectCount, "object-count", cfg.ObjectCount, "number of synthetic objects to seed")
	fs.IntVar(&cfg.PageSize, "page-size", cfg.PageSize, "ListObjectsV2 MaxKeys value")
	fs.IntVar(&cfg.MaxPages, "max-pages", 0, "maximum list pages to read; 0 reads all pages")
	fs.StringVar(&cfg.SummaryJSON, "summary-json", "", "write metadata list benchmark summary to this JSON file")
	fs.StringVar(&cfg.PageJSONL, "page-jsonl", "", "write per-page list results to this JSONL file")
	fs.BoolVar(&cfg.FailOnGate, "fail-on-gate", cfg.FailOnGate, "return a non-zero exit when benchmark gates fail")
	if err := fs.Parse(args); err != nil {
		return metadataListBenchConfig{}, err
	}
	if fs.NArg() != 0 {
		return metadataListBenchConfig{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	cfg.MetadataBackend = config.NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	return cfg, nil
}

func parseNoisyTenantProfileFlags(args []string) (noisyTenantProfileConfig, error) {
	cfg := noisyTenantProfileConfig{
		NoisyTenantID:          "tenant-noisy",
		NeighborTenantID:       "tenant-neighbor",
		MaxConcurrentGlobal:    2,
		MaxConcurrentPerTenant: 1,
		NoisyHoldRequests:      1,
		NoisyAttempts:          4,
		NeighborAttempts:       4,
		FailOnGate:             true,
	}
	fs := flag.NewFlagSet("namros-s3bench noisy-tenant-profile", flag.ContinueOnError)
	fs.StringVar(&cfg.NoisyTenantID, "noisy-tenant-id", cfg.NoisyTenantID, "tenant id used for the saturated/noisy workload")
	fs.StringVar(&cfg.NeighborTenantID, "neighbor-tenant-id", cfg.NeighborTenantID, "tenant id that must continue making progress")
	fs.IntVar(&cfg.MaxConcurrentGlobal, "max-concurrent-global", cfg.MaxConcurrentGlobal, "gateway-local global concurrent request budget")
	fs.IntVar(&cfg.MaxConcurrentPerTenant, "max-concurrent-per-tenant", cfg.MaxConcurrentPerTenant, "gateway-local per-tenant concurrent request budget")
	fs.IntVar(&cfg.NoisyHoldRequests, "noisy-hold-requests", cfg.NoisyHoldRequests, "number of noisy tenant requests held open before burst attempts")
	fs.IntVar(&cfg.NoisyAttempts, "noisy-attempts", cfg.NoisyAttempts, "number of noisy tenant burst attempts while held requests are active")
	fs.IntVar(&cfg.NeighborAttempts, "neighbor-attempts", cfg.NeighborAttempts, "number of neighbor tenant attempts while the noisy tenant is throttled")
	fs.StringVar(&cfg.SummaryJSON, "summary-json", "", "write noisy-tenant profile summary to this JSON file")
	fs.StringVar(&cfg.EventJSONL, "event-jsonl", "", "write per-admission events to this JSONL file")
	fs.BoolVar(&cfg.FailOnGate, "fail-on-gate", cfg.FailOnGate, "return a non-zero exit when noisy-tenant gates fail")
	if err := fs.Parse(args); err != nil {
		return noisyTenantProfileConfig{}, err
	}
	if fs.NArg() != 0 {
		return noisyTenantProfileConfig{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	return cfg, nil
}

func run(ctx context.Context, cfg benchConfig) error {
	r, err := newBenchRunner(cfg)
	if err != nil {
		return err
	}
	started := time.Now().UTC()
	memorySampler, err := r.startMemorySampler(started)
	if err != nil {
		return err
	}
	err = r.runWorkload(ctx)
	if memorySampler != nil {
		if closeErr := memorySampler.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	finished := time.Now().UTC()
	summary := r.summary(started, finished)
	if writeErr := r.writeSummary(summary); writeErr != nil && err == nil {
		err = writeErr
	}
	if err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if writeErr := enc.Encode(summary); writeErr != nil {
			return writeErr
		}
	}
	return err
}

func newBenchRunner(cfg benchConfig) (*benchRunner, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, errors.New("endpoint is required")
	}
	if cfg.Bucket == "" {
		cfg.Bucket = fmt.Sprintf("%s-%d", sanitizeName(cfg.BucketPrefix), time.Now().UnixNano())
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "bench"
	}
	cfg.StorageClass = strings.TrimSpace(cfg.StorageClass)
	if cfg.SmallCount < 0 || cfg.LargeCount < 0 || cfg.ListRepetitions < 0 {
		return nil, errors.New("counts must be non-negative")
	}
	if cfg.SmallSize < 0 || cfg.LargeSize < 0 || cfg.RangeSize < 0 {
		return nil, errors.New("sizes must be non-negative")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MemoryInterval <= 0 {
		cfg.MemoryInterval = time.Second
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse endpoint: %w", err)
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, fmt.Errorf("endpoint must include scheme and host: %q", cfg.Endpoint)
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   256,
		MaxConnsPerHost:       256,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		DisableCompression:    true,
	}
	r := &benchRunner{
		cfg:      cfg,
		endpoint: endpoint,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
	}
	if cfg.OutputJSONL != "" {
		file, err := os.Create(cfg.OutputJSONL)
		if err != nil {
			return nil, fmt.Errorf("create output jsonl: %w", err)
		}
		r.jsonl = json.NewEncoder(file)
		r.jsonlFile = file
	}
	return r, nil
}

func runMetadataListIndexBench(ctx context.Context, cfg metadataListBenchConfig) error {
	summary, err := executeMetadataListIndexBench(ctx, cfg)
	if writeErr := writeMetadataListBenchSummary(summary, cfg.SummaryJSON); writeErr != nil && err == nil {
		err = writeErr
	}
	if err == nil {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if writeErr := enc.Encode(summary); writeErr != nil {
			return writeErr
		}
		if cfg.FailOnGate && summary.Status != "passed" {
			return fmt.Errorf("metadata list index benchmark gates failed")
		}
	}
	return err
}

func executeMetadataListIndexBench(ctx context.Context, cfg metadataListBenchConfig) (metadataListBenchSummary, error) {
	if err := validateMetadataListBenchConfig(cfg); err != nil {
		return metadataListBenchSummary{}, err
	}
	repo, closeRepo, metrics, metadataPath, err := openMetadataListBenchRepository(ctx, cfg)
	if err != nil {
		return metadataListBenchSummary{}, err
	}
	defer closeRepo()

	bucketName := strings.TrimSpace(cfg.Bucket)
	if bucketName == "" {
		bucketName = fmt.Sprintf("%s-%d", sanitizeName(cfg.BucketPrefix), time.Now().UnixNano())
	}
	keyPrefix := strings.Trim(strings.TrimSpace(cfg.KeyPrefix), "/")
	if keyPrefix == "" {
		keyPrefix = "bench/list"
	}

	seedStarted := time.Now()
	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: cfg.TenantID,
		Name:     bucketName,
		Region:   defaultRegion,
	})
	if err != nil {
		return metadataListBenchSummary{}, err
	}
	if err := seedMetadataListObjects(ctx, repo, bucket.BucketID, keyPrefix, cfg.ObjectCount); err != nil {
		return metadataListBenchSummary{}, err
	}
	seedDuration := time.Since(seedStarted)

	var pageFile *os.File
	var pageEncoder *json.Encoder
	if strings.TrimSpace(cfg.PageJSONL) != "" {
		pageFile, err = os.Create(cfg.PageJSONL)
		if err != nil {
			return metadataListBenchSummary{}, fmt.Errorf("create page jsonl: %w", err)
		}
		defer pageFile.Close()
		pageEncoder = json.NewEncoder(pageFile)
	}

	listStarted := time.Now()
	pages, listedObjects, err := runMetadataListPages(ctx, repo, metrics, bucket.BucketID, keyPrefix, cfg.PageSize, cfg.MaxPages, pageEncoder)
	if err != nil {
		return metadataListBenchSummary{}, err
	}
	listDuration := time.Since(listStarted)
	summary := buildMetadataListBenchSummary(cfg, metadataPath, bucketName, bucket.BucketID, keyPrefix, seedDuration, listDuration, pages, listedObjects, metrics)
	return summary, nil
}

func validateMetadataListBenchConfig(cfg metadataListBenchConfig) error {
	switch cfg.MetadataBackend {
	case config.MetadataBackendMemory, config.MetadataBackendPebble, config.MetadataBackendTiKV:
	default:
		return fmt.Errorf("unsupported metadata backend %q", cfg.MetadataBackend)
	}
	if cfg.ObjectCount <= 0 {
		return errors.New("object-count must be positive")
	}
	if cfg.PageSize <= 0 {
		return errors.New("page-size must be positive")
	}
	if cfg.MaxPages < 0 {
		return errors.New("max-pages cannot be negative")
	}
	if strings.TrimSpace(cfg.TenantID) == "" {
		return errors.New("tenant-id is required")
	}
	if cfg.MetadataBackend == config.MetadataBackendTiKV {
		if len(cfg.TiKVPDEndpoints) == 0 {
			return errors.New("tikv pd endpoints are required")
		}
		if cfg.TiKVTimeout < 0 || cfg.TiKVRetryAttempts < 0 || cfg.TiKVRetryInitialBackoff < 0 || cfg.TiKVRetryMaxBackoff < 0 {
			return errors.New("tikv timeout and retry values cannot be negative")
		}
		if cfg.TiKVRetryMaxBackoff > 0 && cfg.TiKVRetryInitialBackoff > cfg.TiKVRetryMaxBackoff {
			return errors.New("tikv retry initial backoff cannot exceed max backoff")
		}
	}
	return nil
}

func openMetadataListBenchRepository(ctx context.Context, cfg metadataListBenchConfig) (meta.Repository, func() error, *tikv.Metrics, string, error) {
	switch cfg.MetadataBackend {
	case config.MetadataBackendMemory:
		return memory.New(), func() error { return nil }, nil, "", nil
	case config.MetadataBackendPebble:
		path := strings.TrimSpace(cfg.MetadataPath)
		tempPath := false
		if path == "" {
			var err error
			path, err = os.MkdirTemp("", "namros-s3bench-meta-*")
			if err != nil {
				return nil, nil, nil, "", fmt.Errorf("create temporary metadata path: %w", err)
			}
			tempPath = true
		}
		repo, err := pebble.Open(path)
		if err != nil {
			if tempPath {
				_ = os.RemoveAll(path)
			}
			return nil, nil, nil, "", err
		}
		closeFn := func() error {
			closeErr := repo.Close()
			if tempPath {
				if removeErr := os.RemoveAll(path); closeErr == nil {
					closeErr = removeErr
				}
			}
			return closeErr
		}
		return repo, closeFn, nil, path, nil
	case config.MetadataBackendTiKV:
		metrics := tikv.NewMetrics()
		repo, err := tikv.Open(ctx, tikv.Config{
			PDEndpoints: cfg.TiKVPDEndpoints,
			APIVersion:  cfg.TiKVAPIVersion,
			Keyspace:    cfg.TiKVKeyspace,
			Timeout:     cfg.TiKVTimeout,
			TLS: tikv.TLSConfig{
				CAPath:   cfg.TiKVTLSCA,
				CertPath: cfg.TiKVTLSCert,
				KeyPath:  cfg.TiKVTLSKey,
			},
			Retry: tikv.RetryPolicy{
				MaxAttempts:    cfg.TiKVRetryAttempts,
				InitialBackoff: cfg.TiKVRetryInitialBackoff,
				MaxBackoff:     cfg.TiKVRetryMaxBackoff,
			},
			Metrics: metrics,
		})
		if err != nil {
			return nil, nil, nil, "", err
		}
		return repo, repo.Close, metrics, "", nil
	default:
		return nil, nil, nil, "", fmt.Errorf("unsupported metadata backend %q", cfg.MetadataBackend)
	}
}

func seedMetadataListObjects(ctx context.Context, repo meta.Repository, bucketID, keyPrefix string, objectCount int) error {
	width := len(strconv.Itoa(objectCount - 1))
	if width < 6 {
		width = 6
	}
	for i := 0; i < objectCount; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		key := fmt.Sprintf("%s/%0*d", keyPrefix, width, i)
		if _, err := repo.PutObjectVersion(ctx, meta.PutObjectVersionRequest{
			BucketID: bucketID,
			Key:      key,
			ETag:     fmt.Sprintf(`"bench-%d"`, i),
		}); err != nil {
			return fmt.Errorf("seed object %d: %w", i, err)
		}
	}
	return nil
}

func runMetadataListPages(ctx context.Context, repo meta.Repository, metrics *tikv.Metrics, bucketID, keyPrefix string, pageSize, maxPages int, pageEncoder *json.Encoder) ([]metadataListBenchPage, int, error) {
	pages := make([]metadataListBenchPage, 0)
	listedObjects := 0
	continuationToken := ""
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		before := tikv.MetricsSnapshot{}
		if metrics != nil {
			before = metrics.Snapshot()
		}
		started := time.Now()
		result, err := repo.ListObjects(ctx, meta.ListObjectsRequest{
			BucketID:          bucketID,
			Prefix:            keyPrefix + "/",
			ContinuationToken: continuationToken,
			MaxKeys:           pageSize,
		})
		duration := time.Since(started)
		after := tikv.MetricsSnapshot{}
		if metrics != nil {
			after = metrics.Snapshot()
		}
		if err != nil {
			return nil, 0, err
		}
		page := metadataListBenchPage{
			Page:                   len(pages) + 1,
			RequestedMaxKeys:       pageSize,
			Objects:                len(result.Contents),
			CommonPrefixes:         len(result.CommonPrefixes),
			DurationMS:             duration.Seconds() * 1000,
			IsTruncated:            result.IsTruncated,
			ContinuationToken:      continuationToken,
			NextContinuationToken:  result.NextContinuationToken,
			RepositoryReadEstimate: metadataListPageReadEstimate(len(result.Contents), result.IsTruncated),
			TiKVOperations:         metadataTiKVPageOperations(before, after),
		}
		pages = append(pages, page)
		listedObjects += len(result.Contents)
		if pageEncoder != nil {
			if err := pageEncoder.Encode(page); err != nil {
				return nil, 0, fmt.Errorf("write page jsonl: %w", err)
			}
		}
		if !result.IsTruncated {
			return pages, listedObjects, nil
		}
		if maxPages > 0 && len(pages) >= maxPages {
			return pages, listedObjects, nil
		}
		continuationToken = result.NextContinuationToken
	}
}

func metadataListPageReadEstimate(objects int, truncated bool) int {
	estimate := 2 + objects
	if truncated {
		estimate++
	}
	return estimate
}

func buildMetadataListBenchSummary(cfg metadataListBenchConfig, metadataPath, bucketName, bucketID, keyPrefix string, seedDuration, listDuration time.Duration, pages []metadataListBenchPage, listedObjects int, metrics *tikv.Metrics) metadataListBenchSummary {
	durations := make([]float64, 0, len(pages))
	maxDuration := 0.0
	maxReadEstimate := 0
	for _, page := range pages {
		durations = append(durations, page.DurationMS)
		if page.DurationMS > maxDuration {
			maxDuration = page.DurationMS
		}
		if page.RepositoryReadEstimate > maxReadEstimate {
			maxReadEstimate = page.RepositoryReadEstimate
		}
	}
	var avgDuration float64
	for _, duration := range durations {
		avgDuration += duration
	}
	if len(durations) > 0 {
		avgDuration /= float64(len(durations))
	}
	var tikvSnapshot *tikv.MetricsSnapshot
	if metrics != nil {
		snapshot := metrics.Snapshot()
		tikvSnapshot = &snapshot
	}
	summary := metadataListBenchSummary{
		SchemaVersion:       "namros.s3bench.metadata_list_index.v1",
		Status:              "passed",
		MetadataBackend:     cfg.MetadataBackend,
		MetadataPath:        metadataPath,
		Bucket:              bucketName,
		BucketID:            bucketID,
		KeyPrefix:           keyPrefix,
		ObjectCount:         cfg.ObjectCount,
		PageSize:            cfg.PageSize,
		MaxPages:            cfg.MaxPages,
		SeedDurationMS:      seedDuration.Seconds() * 1000,
		ListDurationMS:      listDuration.Seconds() * 1000,
		Pages:               pages,
		PageCount:           len(pages),
		ListedObjects:       listedObjects,
		AvgPageDurationMS:   avgDuration,
		P50PageDurationMS:   percentile(durations, 0.50),
		P95PageDurationMS:   percentile(durations, 0.95),
		MaxPageDurationMS:   maxDuration,
		MaxPageReadEstimate: maxReadEstimate,
		PageJSONL:           cfg.PageJSONL,
		TiKVMetrics:         tikvSnapshot,
	}
	summary.Gates = metadataListBenchGates(summary)
	for _, gate := range summary.Gates {
		if gate.Status == "failed" {
			summary.Status = "failed"
			break
		}
	}
	return summary
}

func metadataListBenchGates(summary metadataListBenchSummary) []metadataListBenchGate {
	gates := make([]metadataListBenchGate, 0, 3)
	if summary.MaxPages > 0 {
		gates = append(gates, metadataListBenchGate{
			Name:    "listed_expected_objects",
			Status:  "skipped",
			Message: "max-pages limits the benchmark to a partial listing",
		})
	} else if summary.ListedObjects == summary.ObjectCount {
		gates = append(gates, metadataListBenchGate{Name: "listed_expected_objects", Status: "passed"})
	} else {
		gates = append(gates, metadataListBenchGate{
			Name:    "listed_expected_objects",
			Status:  "failed",
			Message: fmt.Sprintf("listed %d objects, expected %d", summary.ListedObjects, summary.ObjectCount),
		})
	}
	readBudget := summary.PageSize + 3
	if summary.MaxPageReadEstimate <= readBudget {
		gates = append(gates, metadataListBenchGate{Name: "bounded_page_reads", Status: "passed"})
	} else {
		gates = append(gates, metadataListBenchGate{
			Name:    "bounded_page_reads",
			Status:  "failed",
			Message: fmt.Sprintf("max read estimate %d exceeds page budget %d", summary.MaxPageReadEstimate, readBudget),
		})
	}
	expectedPages := (summary.ObjectCount + summary.PageSize - 1) / summary.PageSize
	if summary.MaxPages > 0 && expectedPages > summary.MaxPages {
		expectedPages = summary.MaxPages
	}
	if summary.PageCount == expectedPages {
		gates = append(gates, metadataListBenchGate{Name: "page_count", Status: "passed"})
	} else {
		gates = append(gates, metadataListBenchGate{
			Name:    "page_count",
			Status:  "failed",
			Message: fmt.Sprintf("page count %d, expected %d", summary.PageCount, expectedPages),
		})
	}
	return gates
}

func metadataTiKVPageOperations(before, after tikv.MetricsSnapshot) *metadataListBenchTiKVOperations {
	listBefore := before.Operations[tikv.OperationList]
	listAfter := after.Operations[tikv.OperationList]
	getBefore := before.Operations[tikv.OperationGet]
	getAfter := after.Operations[tikv.OperationGet]
	out := metadataListBenchTiKVOperations{
		ListCalls:        subtractUint64(listAfter.Calls, listBefore.Calls),
		ListKeysReturned: subtractUint64(listAfter.KeysReturned, listBefore.KeysReturned),
		GetCalls:         subtractUint64(getAfter.Calls, getBefore.Calls),
		ListDurationMS:   subtractFloat64(listAfter.TotalMs, listBefore.TotalMs),
		GetDurationMS:    subtractFloat64(getAfter.TotalMs, getBefore.TotalMs),
	}
	out.TotalReadCalls = out.ListCalls + out.GetCalls
	if out.TotalReadCalls == 0 && out.ListKeysReturned == 0 {
		return nil
	}
	return &out
}

func writeMetadataListBenchSummary(summary metadataListBenchSummary, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create metadata list summary json: %w", err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

func runNoisyTenantProfile(_ context.Context, cfg noisyTenantProfileConfig) error {
	summary, err := executeNoisyTenantProfile(cfg)
	if err != nil {
		return err
	}
	if writeErr := writeNoisyTenantProfileSummary(summary, cfg.SummaryJSON); writeErr != nil {
		return writeErr
	}
	if writeErr := writeNoisyTenantEvents(summary.Events, cfg.EventJSONL); writeErr != nil {
		return writeErr
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if writeErr := enc.Encode(summary); writeErr != nil {
		return writeErr
	}
	if cfg.FailOnGate && summary.Status != "passed" {
		return fmt.Errorf("noisy tenant profile gates failed")
	}
	return nil
}

func executeNoisyTenantProfile(cfg noisyTenantProfileConfig) (noisyTenantProfileSummary, error) {
	if err := validateNoisyTenantProfileConfig(cfg); err != nil {
		return noisyTenantProfileSummary{}, err
	}
	sim := newNoisyTenantSimulator(cfg)
	held := 0
	for i := 0; i < cfg.NoisyHoldRequests; i++ {
		if sim.start(cfg.NoisyTenantID, "noisy", "hold_request", i, false) {
			held++
		}
	}
	for i := 0; i < cfg.NoisyAttempts; i++ {
		if sim.start(cfg.NoisyTenantID, "noisy", "burst_request", i, false) {
			sim.complete(cfg.NoisyTenantID, "noisy", "burst_request", i)
		}
	}
	for i := 0; i < cfg.NeighborAttempts; i++ {
		if sim.start(cfg.NeighborTenantID, "neighbor", "neighbor_request", i, true) {
			sim.complete(cfg.NeighborTenantID, "neighbor", "neighbor_request", i)
		}
	}
	for i := 0; i < held; i++ {
		sim.complete(cfg.NoisyTenantID, "noisy", "hold_request", i)
	}
	summary := sim.summary()
	summary.EventJSONL = cfg.EventJSONL
	return summary, nil
}

func validateNoisyTenantProfileConfig(cfg noisyTenantProfileConfig) error {
	if strings.TrimSpace(cfg.NoisyTenantID) == "" {
		return errors.New("noisy-tenant-id is required")
	}
	if strings.TrimSpace(cfg.NeighborTenantID) == "" {
		return errors.New("neighbor-tenant-id is required")
	}
	if cfg.NoisyTenantID == cfg.NeighborTenantID {
		return errors.New("noisy and neighbor tenant ids must be distinct")
	}
	if cfg.MaxConcurrentGlobal <= 0 {
		return errors.New("max-concurrent-global must be positive")
	}
	if cfg.MaxConcurrentPerTenant <= 0 {
		return errors.New("max-concurrent-per-tenant must be positive")
	}
	if cfg.NoisyHoldRequests < 0 || cfg.NoisyAttempts < 0 || cfg.NeighborAttempts < 0 {
		return errors.New("request counts cannot be negative")
	}
	if cfg.NoisyAttempts == 0 {
		return errors.New("noisy-attempts must be positive")
	}
	if cfg.NeighborAttempts == 0 {
		return errors.New("neighbor-attempts must be positive")
	}
	return nil
}

type noisyTenantSimulator struct {
	cfg                   noisyTenantProfileConfig
	sequence              int
	activeGlobal          int
	peakGlobal            int
	activeByTenant        map[string]int
	statsByTenant         map[string]*noisyTenantStats
	events                []noisyTenantEvent
	neighborAdmittedAfter bool
	boundsViolated        bool
}

func newNoisyTenantSimulator(cfg noisyTenantProfileConfig) *noisyTenantSimulator {
	return &noisyTenantSimulator{
		cfg:            cfg,
		activeByTenant: make(map[string]int),
		statsByTenant: map[string]*noisyTenantStats{
			cfg.NoisyTenantID: {
				TenantID: cfg.NoisyTenantID,
				Role:     "noisy",
			},
			cfg.NeighborTenantID: {
				TenantID: cfg.NeighborTenantID,
				Role:     "neighbor",
			},
		},
	}
}

func (s *noisyTenantSimulator) start(tenantID, role, operation string, iteration int, markNeighborSafety bool) bool {
	stats := s.statsByTenant[tenantID]
	stats.Attempted++
	reason := ""
	switch {
	case s.activeGlobal >= s.cfg.MaxConcurrentGlobal:
		reason = "global_limit"
	case s.activeByTenant[tenantID] >= s.cfg.MaxConcurrentPerTenant:
		reason = "tenant_limit"
	}
	if reason != "" {
		stats.Throttled++
		s.appendEvent(tenantID, role, operation, iteration, "start", "throttled", reason, markNeighborSafety)
		return false
	}
	s.activeGlobal++
	s.activeByTenant[tenantID]++
	if s.activeGlobal > s.peakGlobal {
		s.peakGlobal = s.activeGlobal
	}
	if s.activeByTenant[tenantID] > stats.PeakInflight {
		stats.PeakInflight = s.activeByTenant[tenantID]
	}
	if s.activeGlobal > s.cfg.MaxConcurrentGlobal || s.activeByTenant[tenantID] > s.cfg.MaxConcurrentPerTenant {
		s.boundsViolated = true
	}
	stats.Admitted++
	if role == "neighbor" && s.statsByTenant[s.cfg.NoisyTenantID].Throttled > 0 {
		s.neighborAdmittedAfter = true
	}
	s.appendEvent(tenantID, role, operation, iteration, "start", "admitted", "", markNeighborSafety)
	return true
}

func (s *noisyTenantSimulator) complete(tenantID, role, operation string, iteration int) {
	stats := s.statsByTenant[tenantID]
	if s.activeByTenant[tenantID] > 0 {
		s.activeByTenant[tenantID]--
	}
	if s.activeGlobal > 0 {
		s.activeGlobal--
	}
	stats.Completed++
	s.appendEvent(tenantID, role, operation, iteration, "complete", "completed", "", false)
}

func (s *noisyTenantSimulator) appendEvent(tenantID, role, operation string, iteration int, action, status, reason string, markNeighborSafety bool) {
	s.sequence++
	s.events = append(s.events, noisyTenantEvent{
		Schema:         "namros.s3bench.noisy_tenant_event.v1",
		Sequence:       s.sequence,
		TenantID:       tenantID,
		Role:           role,
		Operation:      operation,
		Iteration:      iteration,
		Action:         action,
		Status:         status,
		Reason:         reason,
		GlobalInflight: s.activeGlobal,
		TenantInflight: s.activeByTenant[tenantID],
		MaxGlobal:      s.cfg.MaxConcurrentGlobal,
		MaxTenant:      s.cfg.MaxConcurrentPerTenant,
		NeighborSafe:   markNeighborSafety && status == "admitted",
		NoisyThrottled: s.statsByTenant[s.cfg.NoisyTenantID].Throttled > 0,
	})
}

func (s *noisyTenantSimulator) summary() noisyTenantProfileSummary {
	tenants := []noisyTenantStats{
		*s.statsByTenant[s.cfg.NoisyTenantID],
		*s.statsByTenant[s.cfg.NeighborTenantID],
	}
	summary := noisyTenantProfileSummary{
		SchemaVersion:          "namros.s3bench.noisy_tenant_profile.v1",
		Status:                 "passed",
		NoisyTenantID:          s.cfg.NoisyTenantID,
		NeighborTenantID:       s.cfg.NeighborTenantID,
		MaxConcurrentGlobal:    s.cfg.MaxConcurrentGlobal,
		MaxConcurrentPerTenant: s.cfg.MaxConcurrentPerTenant,
		NoisyHoldRequests:      s.cfg.NoisyHoldRequests,
		NoisyAttempts:          s.cfg.NoisyAttempts,
		NeighborAttempts:       s.cfg.NeighborAttempts,
		PeakGlobalInflight:     s.peakGlobal,
		Tenants:                tenants,
		Events:                 append([]noisyTenantEvent(nil), s.events...),
	}
	noisy := s.statsByTenant[s.cfg.NoisyTenantID]
	neighbor := s.statsByTenant[s.cfg.NeighborTenantID]
	summary.Gates = []metadataListBenchGate{
		noisyTenantGate("noisy_tenant_was_throttled", noisy.Throttled > 0, "no noisy tenant request was throttled"),
		noisyTenantGate("neighbor_admitted_while_noisy_throttled", s.neighborAdmittedAfter, "neighbor did not make progress after noisy throttling was observed"),
		noisyTenantGate("neighbor_completed_without_throttle", neighbor.Completed == s.cfg.NeighborAttempts && neighbor.Throttled == 0, fmt.Sprintf("neighbor completed=%d throttled=%d expected_completed=%d", neighbor.Completed, neighbor.Throttled, s.cfg.NeighborAttempts)),
		noisyTenantGate("admission_bounds_respected", !s.boundsViolated && s.peakGlobal <= s.cfg.MaxConcurrentGlobal && noisy.PeakInflight <= s.cfg.MaxConcurrentPerTenant && neighbor.PeakInflight <= s.cfg.MaxConcurrentPerTenant, "simulated inflight count exceeded configured bounds"),
	}
	for _, gate := range summary.Gates {
		if gate.Status == "failed" {
			summary.Status = "failed"
			break
		}
	}
	return summary
}

func noisyTenantGate(name string, passed bool, failureMessage string) metadataListBenchGate {
	if passed {
		return metadataListBenchGate{Name: name, Status: "passed"}
	}
	return metadataListBenchGate{Name: name, Status: "failed", Message: failureMessage}
}

func writeNoisyTenantProfileSummary(summary noisyTenantProfileSummary, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create noisy tenant summary json: %w", err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

func writeNoisyTenantEvents(events []noisyTenantEvent, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create noisy tenant event jsonl: %w", err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	for _, event := range events {
		if err := enc.Encode(event); err != nil {
			return fmt.Errorf("write noisy tenant event jsonl: %w", err)
		}
	}
	return nil
}

type memorySampler struct {
	runner  *benchRunner
	file    *os.File
	encoder *json.Encoder
	started time.Time
	ticker  *time.Ticker
	done    chan struct{}
	wg      sync.WaitGroup
}

func (r *benchRunner) startMemorySampler(started time.Time) (*memorySampler, error) {
	if strings.TrimSpace(r.cfg.MemoryJSONL) == "" {
		return nil, nil
	}
	file, err := os.Create(r.cfg.MemoryJSONL)
	if err != nil {
		return nil, fmt.Errorf("create memory jsonl: %w", err)
	}
	sampler := &memorySampler{
		runner:  r,
		file:    file,
		encoder: json.NewEncoder(file),
		started: started,
		ticker:  time.NewTicker(r.cfg.MemoryInterval),
		done:    make(chan struct{}),
	}
	if err := sampler.writeSample("start"); err != nil {
		_ = sampler.Close()
		return nil, err
	}
	sampler.wg.Add(1)
	go sampler.loop()
	return sampler, nil
}

func (s *memorySampler) loop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.ticker.C:
			_ = s.writeSample("interval")
		case <-s.done:
			return
		}
	}
}

func (s *memorySampler) Close() error {
	if s == nil {
		return nil
	}
	s.ticker.Stop()
	close(s.done)
	s.wg.Wait()
	err := s.writeSample("stop")
	if closeErr := s.file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func (s *memorySampler) writeSample(phase string) error {
	sample := collectMemorySample(phase, s.started)
	sample.RecordedOperations, sample.ErrorCount, sample.RequestBytes, sample.ResponseBytes = s.runner.resultCounters()
	s.runner.observeMemorySample(sample)
	if err := s.encoder.Encode(sample); err != nil {
		return fmt.Errorf("write memory sample: %w", err)
	}
	return nil
}

func collectMemorySample(phase string, started time.Time) memorySample {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rssBytes, rssSource := currentRSSBytes()
	return memorySample{
		Schema:            "namros.s3bench.memory_sample.v1",
		TimestampUnixNano: time.Now().UnixNano(),
		ElapsedMS:         time.Since(started).Seconds() * 1000,
		Phase:             phase,
		RSSBytes:          rssBytes,
		RSSSource:         rssSource,
		HeapAllocBytes:    mem.Alloc,
		HeapSysBytes:      mem.HeapSys,
		StackInuseBytes:   mem.StackInuse,
		Goroutines:        runtime.NumGoroutine(),
	}
}

func currentRSSBytes() (uint64, string) {
	if payload, err := os.ReadFile("/proc/self/statm"); err == nil {
		fields := strings.Fields(string(payload))
		if len(fields) >= 2 {
			if pages, parseErr := strconv.ParseUint(fields[1], 10, 64); parseErr == nil {
				return pages * uint64(os.Getpagesize()), "proc_statm"
			}
		}
	}
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err == nil {
		maxRSS := uint64(usage.Maxrss)
		if runtime.GOOS == "linux" {
			maxRSS *= 1024
		}
		return maxRSS, "getrusage_maxrss"
	}
	return 0, "unavailable"
}

func (r *benchRunner) runWorkload(ctx context.Context) error {
	defer func() {
		if r.jsonlFile != nil {
			_ = r.jsonlFile.Close()
		}
	}()
	if err := r.createBucket(ctx); err != nil {
		return err
	}
	var workloadErr error
	if err := r.runObjectSet(ctx, "small", r.cfg.SmallCount, r.cfg.SmallSize, "small"); err != nil && workloadErr == nil {
		workloadErr = err
	}
	if err := r.runObjectSet(ctx, "large", r.cfg.LargeCount, r.cfg.LargeSize, "large"); err != nil && workloadErr == nil {
		workloadErr = err
	}
	if workloadErr == nil {
		for i := 0; i < r.cfg.ListRepetitions; i++ {
			if err := r.listObjects(ctx, i); err != nil {
				workloadErr = err
				if r.cfg.FailFast {
					break
				}
			}
		}
	}
	if r.cfg.Cleanup {
		cleanupErr := r.cleanup(ctx)
		if workloadErr == nil {
			workloadErr = cleanupErr
		}
	}
	return workloadErr
}

func (r *benchRunner) runObjectSet(ctx context.Context, phase string, count, size int, keyClass string) error {
	for i := 0; i < count; i++ {
		key := fmt.Sprintf("%s/%s/%06d", strings.Trim(r.cfg.KeyPrefix, "/"), keyClass, i)
		payload := makePayload(size, i)
		if err := r.putObject(ctx, phase, "put_object_"+keyClass, i, key, payload); err != nil && r.cfg.FailFast {
			return err
		}
		if err := r.headObject(ctx, phase, "head_object_"+keyClass, i, key); err != nil && r.cfg.FailFast {
			return err
		}
		if err := r.getObject(ctx, phase, "get_object_"+keyClass, i, key, payload, ""); err != nil && r.cfg.FailFast {
			return err
		}
		if r.cfg.RangeSize > 0 && size > 0 {
			end := minInt(size, r.cfg.RangeSize) - 1
			expected := payload[:end+1]
			if err := r.getObject(ctx, phase, "range_get_"+keyClass, i, key, expected, fmt.Sprintf("bytes=0-%d", end)); err != nil && r.cfg.FailFast {
				return err
			}
		}
		r.keys = append(r.keys, key)
	}
	return nil
}

func (r *benchRunner) createBucket(ctx context.Context) error {
	return r.do(ctx, "setup", "create_bucket", 0, "", 0, 0, http.MethodPut, "/"+r.cfg.Bucket, nil, nil, nil, http.StatusOK)
}

func (r *benchRunner) putObject(ctx context.Context, phase, operation string, iteration int, key string, payload []byte) error {
	return r.do(ctx, phase, operation, iteration, key, int64(len(payload)), 0, http.MethodPut, "/"+r.cfg.Bucket+"/"+key, payload, r.putObjectHeaders(), nil, http.StatusOK)
}

func (r *benchRunner) putObjectHeaders() http.Header {
	headers := make(http.Header)
	if r != nil {
		storageClass := strings.TrimSpace(r.cfg.StorageClass)
		if storageClass != "" {
			headers.Set("X-Amz-Storage-Class", storageClass)
		}
	}
	return headers
}

func (r *benchRunner) headObject(ctx context.Context, phase, operation string, iteration int, key string) error {
	return r.do(ctx, phase, operation, iteration, key, 0, 0, http.MethodHead, "/"+r.cfg.Bucket+"/"+key, nil, nil, nil, http.StatusOK)
}

func (r *benchRunner) getObject(ctx context.Context, phase, operation string, iteration int, key string, expected []byte, rangeHeader string) error {
	headers := make(http.Header)
	expectedStatus := http.StatusOK
	if rangeHeader != "" {
		headers.Set("Range", rangeHeader)
		expectedStatus = http.StatusPartialContent
	}
	return r.do(ctx, phase, operation, iteration, key, 0, int64(len(expected)), http.MethodGet, "/"+r.cfg.Bucket+"/"+key, nil, headers, func(body []byte) error {
		if !bytes.Equal(body, expected) {
			return fmt.Errorf("response body mismatch got=%d want=%d", len(body), len(expected))
		}
		return nil
	}, expectedStatus)
}

func (r *benchRunner) listObjects(ctx context.Context, iteration int) error {
	values := url.Values{}
	values.Set("list-type", "2")
	values.Set("prefix", strings.Trim(r.cfg.KeyPrefix, "/")+"/")
	return r.do(ctx, "list", "list_objects", iteration, "", 0, -1, http.MethodGet, "/"+r.cfg.Bucket+"?"+values.Encode(), nil, nil, nil, http.StatusOK)
}

func (r *benchRunner) cleanup(ctx context.Context) error {
	var firstErr error
	for i := len(r.keys) - 1; i >= 0; i-- {
		key := r.keys[i]
		if err := r.do(ctx, "cleanup", "delete_object", len(r.keys)-1-i, key, 0, 0, http.MethodDelete, "/"+r.cfg.Bucket+"/"+key, nil, nil, nil, http.StatusNoContent, http.StatusOK); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := r.do(ctx, "cleanup", "delete_bucket", 0, "", 0, 0, http.MethodDelete, "/"+r.cfg.Bucket, nil, nil, nil, http.StatusNoContent, http.StatusOK); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (r *benchRunner) do(ctx context.Context, phase, operation string, iteration int, key string, requestBytes, expectedResponseBytes int64, method, target string, body []byte, headers http.Header, validate func([]byte) error, expectedStatus ...int) error {
	reqURL := r.requestURL(target)
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), reader)
	if err != nil {
		return err
	}
	req.Host = req.URL.Host
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	if err := signRequest(req, signer{
		accessKeyID:     r.cfg.AccessKeyID,
		secretAccessKey: r.cfg.SecretAccessKey,
		region:          r.cfg.Region,
		now:             time.Now,
	}); err != nil {
		return err
	}

	start := time.Now()
	resp, err := r.httpClient.Do(req)
	duration := time.Since(start)
	var statusCode int
	var responseBody []byte
	if resp != nil {
		statusCode = resp.StatusCode
		if resp.Body != nil {
			responseBody, _ = io.ReadAll(resp.Body)
			_ = resp.Body.Close()
		}
	}
	if err == nil && !statusAllowed(statusCode, expectedStatus) {
		err = fmt.Errorf("unexpected status code got=%d want=%v body=%s", statusCode, expectedStatus, trimBody(responseBody))
	}
	if err == nil && len(expectedStatus) > 0 && expectedResponseBytes >= 0 && method == http.MethodGet {
		if int64(len(responseBody)) != expectedResponseBytes {
			err = fmt.Errorf("unexpected response size got=%d want=%d", len(responseBody), expectedResponseBytes)
		}
	}
	if err == nil && validate != nil {
		err = validate(responseBody)
	}

	result := benchResult{
		Schema:        "namros.s3bench.result.v1",
		TimestampUnix: time.Now().Unix(),
		Phase:         phase,
		Operation:     operation,
		Iteration:     iteration,
		Bucket:        r.cfg.Bucket,
		Key:           key,
		Status:        "ok",
		StatusCode:    statusCode,
		DurationMS:    duration.Seconds() * 1000,
		RequestBytes:  requestBytes,
		ResponseBytes: int64(len(responseBody)),
		StorageClass:  r.cfg.StorageClass,
	}
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
	}
	r.record(result)
	return err
}

func (r *benchRunner) requestURL(target string) *url.URL {
	u := *r.endpoint
	path, rawQuery, _ := strings.Cut(target, "?")
	u.Path = joinURLPath(r.endpoint.Path, path)
	u.RawQuery = rawQuery
	return &u
}

func (r *benchRunner) record(result benchResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, result)
	if r.jsonl != nil {
		_ = r.jsonl.Encode(result)
	}
}

func (r *benchRunner) summary(started, finished time.Time) benchSummary {
	r.mu.Lock()
	results := append([]benchResult(nil), r.results...)
	memory := r.memory
	r.mu.Unlock()
	byOperation := make(map[string][]benchResult)
	var errors []benchResult
	for _, result := range results {
		byOperation[result.Operation] = append(byOperation[result.Operation], result)
		if result.Status != "ok" {
			errors = append(errors, result)
		}
	}
	operations := make([]operationSummary, 0, len(byOperation))
	for operation, results := range byOperation {
		operations = append(operations, summarizeOperation(operation, results))
	}
	sort.Slice(operations, func(i, j int) bool {
		return operations[i].Operation < operations[j].Operation
	})
	summary := benchSummary{
		Schema:        "namros.s3bench.summary.v1",
		Endpoint:      r.cfg.Endpoint,
		Region:        r.cfg.Region,
		Bucket:        r.cfg.Bucket,
		KeyPrefix:     r.cfg.KeyPrefix,
		StorageClass:  r.cfg.StorageClass,
		StartedAt:     started.Format(time.RFC3339Nano),
		FinishedAt:    finished.Format(time.RFC3339Nano),
		ElapsedMS:     finished.Sub(started).Seconds() * 1000,
		Operations:    operations,
		Errors:        errors,
		ResultJSONL:   r.cfg.OutputJSONL,
		SmallCount:    r.cfg.SmallCount,
		SmallSize:     r.cfg.SmallSize,
		LargeCount:    r.cfg.LargeCount,
		LargeSize:     r.cfg.LargeSize,
		RangeSize:     r.cfg.RangeSize,
		HTTPKeepAlive: true,
		MemoryJSONL:   r.cfg.MemoryJSONL,
	}
	if memory.Samples > 0 {
		summary.Memory = &memory
	}
	return summary
}

func (r *benchRunner) resultCounters() (operations int, errorsCount int, requestBytes int64, responseBytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, result := range r.results {
		operations++
		if result.Status != "ok" {
			errorsCount++
		}
		requestBytes += result.RequestBytes
		responseBytes += result.ResponseBytes
	}
	return operations, errorsCount, requestBytes, responseBytes
}

func (r *benchRunner) observeMemorySample(sample memorySample) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.memory.Samples++
	if sample.RSSSource != "" && r.memory.RSSSource == "" {
		r.memory.RSSSource = sample.RSSSource
	}
	if sample.RSSBytes > r.memory.PeakRSSBytes {
		r.memory.PeakRSSBytes = sample.RSSBytes
		if sample.RSSSource != "" {
			r.memory.RSSSource = sample.RSSSource
		}
	}
	if sample.HeapAllocBytes > r.memory.PeakHeapAllocBytes {
		r.memory.PeakHeapAllocBytes = sample.HeapAllocBytes
	}
	if sample.HeapSysBytes > r.memory.PeakHeapSysBytes {
		r.memory.PeakHeapSysBytes = sample.HeapSysBytes
	}
	if sample.StackInuseBytes > r.memory.PeakStackBytes {
		r.memory.PeakStackBytes = sample.StackInuseBytes
	}
	if sample.Goroutines > r.memory.PeakGoroutines {
		r.memory.PeakGoroutines = sample.Goroutines
	}
}

func (r *benchRunner) writeSummary(summary benchSummary) error {
	if r.cfg.SummaryJSON == "" {
		return nil
	}
	file, err := os.Create(r.cfg.SummaryJSON)
	if err != nil {
		return fmt.Errorf("create summary json: %w", err)
	}
	defer file.Close()
	enc := json.NewEncoder(file)
	enc.SetIndent("", "  ")
	return enc.Encode(summary)
}

func summarizeOperation(operation string, results []benchResult) operationSummary {
	summary := operationSummary{
		Operation:     operation,
		Count:         len(results),
		MinDurationMS: math.Inf(1),
	}
	durations := make([]float64, 0, len(results))
	for _, result := range results {
		if result.Status != "ok" {
			summary.Errors++
		}
		summary.TotalDurationMS += result.DurationMS
		summary.RequestBytes += result.RequestBytes
		summary.ResponseBytes += result.ResponseBytes
		if result.DurationMS < summary.MinDurationMS {
			summary.MinDurationMS = result.DurationMS
		}
		if result.DurationMS > summary.MaxDurationMS {
			summary.MaxDurationMS = result.DurationMS
		}
		durations = append(durations, result.DurationMS)
	}
	if summary.Count > 0 {
		summary.AvgDurationMS = summary.TotalDurationMS / float64(summary.Count)
		summary.P50DurationMS = percentile(durations, 0.50)
		summary.P95DurationMS = percentile(durations, 0.95)
	}
	if math.IsInf(summary.MinDurationMS, 1) {
		summary.MinDurationMS = 0
	}
	return summary
}

func percentile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	if len(values) == 1 {
		return values[0]
	}
	pos := q * float64(len(values)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return values[lower]
	}
	weight := pos - float64(lower)
	return values[lower]*(1-weight) + values[upper]*weight
}

func statusAllowed(code int, allowed []int) bool {
	for _, candidate := range allowed {
		if code == candidate {
			return true
		}
	}
	return len(allowed) == 0
}

func makePayload(size, seed int) []byte {
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte((seed + i*31) % 251)
	}
	return payload
}

func sanitizeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "namros-s3bench"
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func subtractUint64(after, before uint64) uint64 {
	if after < before {
		return 0
	}
	return after - before
}

func subtractFloat64(after, before float64) float64 {
	if after < before {
		return 0
	}
	return after - before
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func joinURLPath(base, path string) string {
	if base == "" || base == "/" {
		if path == "" {
			return "/"
		}
		if strings.HasPrefix(path, "/") {
			return path
		}
		return "/" + path
	}
	return strings.TrimRight(base, "/") + "/" + strings.TrimLeft(path, "/")
}

func trimBody(body []byte) string {
	text := strings.TrimSpace(string(body))
	if len(text) > 256 {
		return text[:256] + "..."
	}
	return text
}

type signer struct {
	accessKeyID     string
	secretAccessKey string
	region          string
	now             func() time.Time
}

func signRequest(req *http.Request, s signer) error {
	if req == nil || req.URL == nil {
		return errors.New("request URL is required")
	}
	if s.accessKeyID == "" || s.secretAccessKey == "" {
		return errors.New("access key and secret key are required")
	}
	if s.region == "" {
		s.region = defaultRegion
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	t := now().UTC()
	amzDate := t.Format("20060102T150405Z")
	date := t.Format("20060102")
	if req.Host == "" {
		req.Host = req.URL.Host
	}
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", unsignedPayload)
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	canonicalRequest, signedHeaderText, err := canonicalRequest(req, signedHeaders, unsignedPayload)
	if err != nil {
		return err
	}
	scope := date + "/" + s.region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		sha256Hex(canonicalRequest),
	}, "\n")
	signature := hex.EncodeToString(hmacSHA256(signingKey(s.secretAccessKey, date, s.region, "s3"), stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.accessKeyID+"/"+scope+", SignedHeaders="+signedHeaderText+", Signature="+signature)
	return nil
}

func canonicalRequest(req *http.Request, signedHeaders []string, payloadHash string) (string, string, error) {
	headers := append([]string(nil), signedHeaders...)
	sort.Strings(headers)
	var canonicalHeaders strings.Builder
	for _, name := range headers {
		value, ok := canonicalHeaderValue(req, name)
		if !ok {
			return "", "", fmt.Errorf("signed header %q is missing", name)
		}
		canonicalHeaders.WriteString(name)
		canonicalHeaders.WriteByte(':')
		canonicalHeaders.WriteString(value)
		canonicalHeaders.WriteByte('\n')
	}
	signedHeaderText := strings.Join(headers, ";")
	return strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQueryString(req.URL),
		canonicalHeaders.String(),
		signedHeaderText,
		payloadHash,
	}, "\n"), signedHeaderText, nil
}

func canonicalHeaderValue(req *http.Request, name string) (string, bool) {
	if name == "host" {
		if req.Host == "" {
			return "", false
		}
		return normalizeHeaderValue(req.Host), true
	}
	values, ok := req.Header[http.CanonicalHeaderKey(name)]
	if !ok || len(values) == 0 {
		return "", false
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		normalized = append(normalized, normalizeHeaderValue(value))
	}
	return strings.Join(normalized, ","), true
}

func normalizeHeaderValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func canonicalURI(u *url.URL) string {
	if path := u.EscapedPath(); path != "" {
		return path
	}
	return "/"
}

func canonicalQueryString(u *url.URL) string {
	values := u.Query()
	type pair struct {
		key   string
		value string
	}
	pairs := make([]pair, 0)
	for key, vals := range values {
		if len(vals) == 0 {
			pairs = append(pairs, pair{key: key})
			continue
		}
		for _, value := range vals {
			pairs = append(pairs, pair{key: key, value: value})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})
	encoded := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		encoded = append(encoded, awsPercentEncode(pair.key)+"="+awsPercentEncode(pair.value))
	}
	return strings.Join(encoded, "&")
}

func awsPercentEncode(value string) string {
	var b strings.Builder
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			b.WriteString(fmt.Sprintf("%%%02X", value[0]))
			value = value[1:]
			continue
		}
		if isUnreserved(r) {
			b.WriteRune(r)
		} else {
			for _, c := range []byte(string(r)) {
				b.WriteString(fmt.Sprintf("%%%02X", c))
			}
		}
		value = value[size:]
	}
	return b.String()
}

func isUnreserved(r rune) bool {
	return r >= 'A' && r <= 'Z' ||
		r >= 'a' && r <= 'z' ||
		r >= '0' && r <= '9' ||
		r == '-' || r == '_' || r == '.' || r == '~'
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func signingKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func parseSize(value string) (int, error) {
	if value == "" {
		return 0, nil
	}
	multiplier := int64(1)
	upper := strings.ToUpper(strings.TrimSpace(value))
	for _, suffix := range []struct {
		Text string
		Mult int64
	}{
		{"KIB", 1 << 10},
		{"KI", 1 << 10},
		{"K", 1 << 10},
		{"MIB", 1 << 20},
		{"MI", 1 << 20},
		{"M", 1 << 20},
		{"GIB", 1 << 30},
		{"GI", 1 << 30},
		{"G", 1 << 30},
	} {
		if strings.HasSuffix(upper, suffix.Text) {
			multiplier = suffix.Mult
			upper = strings.TrimSpace(strings.TrimSuffix(upper, suffix.Text))
			break
		}
	}
	n, err := strconv.ParseInt(upper, 10, 64)
	if err != nil {
		return 0, err
	}
	if n < 0 || n > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("size out of range: %s", value)
	}
	total := n * multiplier
	if total > int64(int(total)) {
		return 0, fmt.Errorf("size exceeds int range: %s", value)
	}
	return int(total), nil
}

type sizeFlag int

func (f *sizeFlag) String() string {
	if f == nil {
		return "0"
	}
	return strconv.Itoa(int(*f))
}

func (f *sizeFlag) Set(value string) error {
	size, err := parseSize(value)
	if err != nil {
		return err
	}
	*f = sizeFlag(size)
	return nil
}

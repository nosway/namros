package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nosway/namros/internal/edition"
)

const (
	DefaultListenAddr                        = "127.0.0.1:9000"
	DefaultDeploymentProfile                 = DeploymentProfileDev
	DefaultRegion                            = "us-east-1"
	DefaultMetadataBackend                   = "pebble"
	DefaultMetadataPath                      = ".namros/meta"
	DefaultStorageBackend                    = "memory"
	DefaultRootAccessKeyID                   = "namrosroot"
	DefaultRootSecretKey                     = "namrosrootsecret"
	DefaultTiKVAPIVersion                    = "v1"
	DefaultTiKVTimeout                       = 3 * time.Second
	DefaultTiKVRetryAttempts                 = 3
	DefaultTiKVRetryInitialBackoff           = 10 * time.Millisecond
	DefaultTiKVRetryMaxBackoff               = 100 * time.Millisecond
	DefaultMetadataCacheTTL                  = 0 * time.Second
	DefaultCoordinationBackend               = "none"
	DefaultEtcdDialTimeout                   = 3 * time.Second
	DefaultGatewayRegistryPrefix             = "/namros/gateways"
	DefaultGatewayLeaseTTL                   = 10 * time.Second
	DefaultGatewayHeartbeat                  = 3 * time.Second
	DefaultGatewayDataBudgetUnknownBytes     = 8 << 20
	DefaultDedupeSchedulerInterval           = 5 * time.Minute
	DefaultDedupeSchedulerLockTTL            = 30 * time.Minute
	DefaultDedupeSchedulerMaxKeys            = 1000
	DefaultDedupeSchedulerLimit              = 1000
	DefaultDedupeSchedulerMode               = "post_process"
	DefaultGCWorkerInterval                  = 5 * time.Minute
	DefaultGCWorkerLeaseTTL                  = 30 * time.Minute
	DefaultGCWorkerLimit                     = 1000
	DefaultGCWorkerShardID                   = "orphans"
	DefaultLifecycleWorkerInterval           = 5 * time.Minute
	DefaultLifecycleWorkerLeaseTTL           = 30 * time.Minute
	DefaultLifecycleWorkerMaxKeys            = 1000
	DefaultLifecycleWorkerMaxUploads         = 1000
	DefaultLifecycleWorkerShardID            = "buckets"
	DefaultGCCandidateQueue                  = GCCandidateQueueStorage
	DefaultConsoleAuthMode                   = "disabled"
	DefaultConsoleAdminUsername              = "admin"
	DefaultConsoleSessionTTL                 = 12 * time.Hour
	DefaultNAMRBDSBSObservabilityTimeout     = 3 * time.Second
	DefaultSBSVolumePoolRefreshInterval      = 30 * time.Second
	DefaultSBSPhysicalFullChunkWriteMinBytes = 64 << 10
	DefaultSBSPhysicalFullChunkWriteMaxBytes = 4 << 20
	DefaultSBSPhysicalChunkCacheBytes        = 64 << 20
	DefaultSBSChunkIDAllocationCacheSize     = 256
	DefaultSBSSessionTTL                     = 30 * time.Second
	DefaultSBSSessionHeartbeat               = 10 * time.Second
	DefaultSBSECShardConcurrency             = 8
	DefaultAccessAuditMode                   = "sync"
	DefaultAccessAuditBatchSize              = 32
	DefaultAccessAuditQueueSize              = 1024
	DefaultAccessAuditFlushInterval          = 10 * time.Millisecond
)

var DefaultEdition = edition.Current()
var DefaultTiKVPDEndpoints = []string{"127.0.0.1:2379"}
var DefaultEtcdEndpoints = []string{"127.0.0.1:2379"}

var currentEdition = edition.Current

const (
	MetadataBackendMemory = "memory"
	MetadataBackendPebble = "pebble"
	MetadataBackendTiKV   = "tikv"

	DeploymentProfileDev        = "dev"
	DeploymentProfileProduction = "production"

	StorageBackendMemory      = "memory"
	StorageBackendLocal       = "local"
	StorageBackendSBS         = "sbs"
	StorageBackendSBSLocal    = "sbs-local"
	StorageBackendSBSPhysical = "sbs-physical"
	StorageBackendSBSEC       = "sbs-ec"
	StorageBackendSBSCluster  = "sbs-cluster"

	CoordinationBackendNone = "none"
	CoordinationBackendEtcd = "etcd"

	GCCandidateQueueStorage  = "storage"
	GCCandidateQueueMetadata = "metadata"

	AccessAuditModeSync  = "sync"
	AccessAuditModeAsync = "async"
)

type Config struct {
	ListenAddr                             string                `json:"listen_addr"`
	DeploymentProfile                      string                `json:"deployment_profile"`
	AllowUnsafeProductionShortcuts         bool                  `json:"allow_unsafe_production_shortcuts,omitempty"`
	Edition                                string                `json:"edition"`
	Region                                 string                `json:"region"`
	MetadataBackend                        string                `json:"metadata_backend"`
	MetadataPath                           string                `json:"metadata_path,omitempty"`
	TiKVPDEndpoints                        []string              `json:"tikv_pd_endpoints,omitempty"`
	TiKVAPIVersion                         string                `json:"tikv_api_version,omitempty"`
	TiKVKeyspace                           string                `json:"tikv_keyspace,omitempty"`
	TiKVTimeout                            time.Duration         `json:"tikv_timeout,omitempty"`
	TiKVTLSCA                              string                `json:"tikv_tls_ca,omitempty"`
	TiKVTLSCert                            string                `json:"tikv_tls_cert,omitempty"`
	TiKVTLSKey                             string                `json:"tikv_tls_key,omitempty"`
	TiKVRetryAttempts                      int                   `json:"tikv_retry_attempts,omitempty"`
	TiKVRetryInitialBackoff                time.Duration         `json:"tikv_retry_initial_backoff,omitempty"`
	TiKVRetryMaxBackoff                    time.Duration         `json:"tikv_retry_max_backoff,omitempty"`
	MetadataCacheTTL                       time.Duration         `json:"metadata_cache_ttl,omitempty"`
	StorageBackend                         string                `json:"storage_backend"`
	StoragePath                            string                `json:"storage_path,omitempty"`
	SBSStatePath                           string                `json:"sbs_state_path,omitempty"`
	SBSAdminEndpoint                       string                `json:"sbs_admin_endpoint,omitempty"`
	SBSDataEndpoint                        string                `json:"sbs_data_endpoint,omitempty"`
	SBSVolumeID                            string                `json:"sbs_volume_id,omitempty"`
	SBSChunkSizeBytes                      uint64                `json:"sbs_chunk_size_bytes,omitempty"`
	SBSGatewayID                           string                `json:"sbs_gateway_id,omitempty"`
	SBSAttachmentID                        string                `json:"sbs_attachment_id,omitempty"`
	SBSGeneration                          uint64                `json:"sbs_generation,omitempty"`
	SBSWriterGroupID                       string                `json:"sbs_writer_group_id,omitempty"`
	SBSSessionID                           string                `json:"sbs_session_id,omitempty"`
	SBSVolumeEpoch                         uint64                `json:"sbs_volume_epoch,omitempty"`
	SBSSessionTTL                          time.Duration         `json:"sbs_session_ttl,omitempty"`
	SBSSessionHeartbeat                    time.Duration         `json:"sbs_session_heartbeat,omitempty"`
	SBSShardStoreIDs                       []string              `json:"sbs_shard_store_ids,omitempty"`
	SBSECShardConcurrency                  int                   `json:"sbs_ec_shard_concurrency,omitempty"`
	SBSVerifyReadback                      bool                  `json:"sbs_verify_readback,omitempty"`
	SBSPhysicalWriteConcurrency            int                   `json:"sbs_physical_write_concurrency,omitempty"`
	SBSPhysicalFullChunkWriteMinBytes      uint64                `json:"sbs_physical_full_chunk_write_min_bytes,omitempty"`
	SBSPhysicalFullChunkWriteMaxBytes      uint64                `json:"sbs_physical_full_chunk_write_max_bytes,omitempty"`
	SBSPhysicalChunkCacheBytes             uint64                `json:"sbs_physical_chunk_cache_bytes,omitempty"`
	SBSChunkIDAllocationCacheSize          uint32                `json:"sbs_chunk_id_allocation_cache_size,omitempty"`
	SBSVolumePool                          []SBSVolumePoolMember `json:"sbs_volume_pool,omitempty"`
	SBSVolumePoolID                        string                `json:"sbs_volume_pool_id,omitempty"`
	SBSVolumePoolGeneration                uint64                `json:"sbs_volume_pool_generation,omitempty"`
	SBSVolumePoolRefreshInterval           time.Duration         `json:"sbs_volume_pool_refresh_interval,omitempty"`
	CoordinationBackend                    string                `json:"coordination_backend"`
	EtcdEndpoints                          []string              `json:"etcd_endpoints,omitempty"`
	EtcdDialTimeout                        time.Duration         `json:"etcd_dial_timeout,omitempty"`
	GatewayInstanceID                      string                `json:"gateway_instance_id,omitempty"`
	GatewayAdvertiseEndpoint               string                `json:"gateway_advertise_endpoint,omitempty"`
	GatewayRegistryPrefix                  string                `json:"gateway_registry_prefix,omitempty"`
	GatewayLeaseTTL                        time.Duration         `json:"gateway_lease_ttl,omitempty"`
	GatewayHeartbeat                       time.Duration         `json:"gateway_heartbeat,omitempty"`
	GatewayDataBudgetBytes                 uint64                `json:"gateway_data_budget_bytes,omitempty"`
	GatewayDataBudgetMaxRequests           int                   `json:"gateway_data_budget_max_requests,omitempty"`
	GatewayDataBudgetUnknownBytes          uint64                `json:"gateway_data_budget_unknown_bytes,omitempty"`
	GatewayRequestMaxConcurrent            int                   `json:"gateway_request_max_concurrent,omitempty"`
	GatewayRequestMaxConcurrentPerTenant   int                   `json:"gateway_request_max_concurrent_per_tenant,omitempty"`
	GatewayRequestMaxConcurrentReads       int                   `json:"gateway_request_max_concurrent_reads,omitempty"`
	GatewayRequestMaxConcurrentWrites      int                   `json:"gateway_request_max_concurrent_writes,omitempty"`
	GatewayUploadBandwidthBytesPerSecond   int64                 `json:"gateway_upload_bandwidth_bytes_per_second,omitempty"`
	GatewayDownloadBandwidthBytesPerSecond int64                 `json:"gateway_download_bandwidth_bytes_per_second,omitempty"`
	BackgroundWorkerMaxConcurrent          int                   `json:"background_worker_max_concurrent,omitempty"`
	BackgroundWorkerMaxConcurrentPerTenant int                   `json:"background_worker_max_concurrent_per_tenant,omitempty"`
	BackgroundWorkerMaxConcurrentPerPool   int                   `json:"background_worker_max_concurrent_per_pool,omitempty"`
	DedupeSchedulerEnabled                 bool                  `json:"dedupe_scheduler_enabled,omitempty"`
	DedupeSchedulerTenantID                string                `json:"dedupe_scheduler_tenant_id,omitempty"`
	DedupeSchedulerBucketID                string                `json:"dedupe_scheduler_bucket_id,omitempty"`
	DedupeSchedulerPrefix                  string                `json:"dedupe_scheduler_prefix,omitempty"`
	DedupeSchedulerMode                    string                `json:"dedupe_scheduler_mode,omitempty"`
	DedupeSchedulerInterval                time.Duration         `json:"dedupe_scheduler_interval,omitempty"`
	DedupeSchedulerLockTTL                 time.Duration         `json:"dedupe_scheduler_lock_ttl,omitempty"`
	DedupeSchedulerMaxKeys                 int                   `json:"dedupe_scheduler_max_keys,omitempty"`
	DedupeSchedulerLimit                   int                   `json:"dedupe_scheduler_limit,omitempty"`
	DedupeSchedulerVerifyBytes             bool                  `json:"dedupe_scheduler_verify_bytes,omitempty"`
	GCWorkerEnabled                        bool                  `json:"gc_worker_enabled,omitempty"`
	GCWorkerShardID                        string                `json:"gc_worker_shard_id,omitempty"`
	GCWorkerInterval                       time.Duration         `json:"gc_worker_interval,omitempty"`
	GCWorkerLeaseTTL                       time.Duration         `json:"gc_worker_lease_ttl,omitempty"`
	GCWorkerLimit                          int                   `json:"gc_worker_limit,omitempty"`
	LifecycleWorkerEnabled                 bool                  `json:"lifecycle_worker_enabled,omitempty"`
	LifecycleWorkerShardID                 string                `json:"lifecycle_worker_shard_id,omitempty"`
	LifecycleWorkerBucketID                string                `json:"lifecycle_worker_bucket_id,omitempty"`
	LifecycleWorkerInterval                time.Duration         `json:"lifecycle_worker_interval,omitempty"`
	LifecycleWorkerLeaseTTL                time.Duration         `json:"lifecycle_worker_lease_ttl,omitempty"`
	LifecycleWorkerMaxKeys                 int                   `json:"lifecycle_worker_max_keys,omitempty"`
	LifecycleWorkerMaxUploads              int                   `json:"lifecycle_worker_max_uploads,omitempty"`
	GCCandidateQueue                       string                `json:"gc_candidate_queue,omitempty"`
	AccessAuditMode                        string                `json:"access_audit_mode,omitempty"`
	AccessAuditBatchSize                   int                   `json:"access_audit_batch_size,omitempty"`
	AccessAuditQueueSize                   int                   `json:"access_audit_queue_size,omitempty"`
	AccessAuditFlushInterval               time.Duration         `json:"access_audit_flush_interval,omitempty"`
	ConsoleAuthMode                        string                `json:"console_auth_mode,omitempty"`
	ConsoleAdminUsername                   string                `json:"console_admin_username,omitempty"`
	ConsoleAdminPassword                   string                `json:"-"`
	ConsoleSessionSecret                   string                `json:"-"`
	ConsoleSessionTTL                      time.Duration         `json:"console_session_ttl,omitempty"`
	ObservabilityPrometheusURL             string                `json:"observability_prometheus_url,omitempty"`
	ObservabilityGrafanaURL                string                `json:"observability_grafana_url,omitempty"`
	ObservabilityVictoriaURL               string                `json:"observability_victoria_url,omitempty"`
	NAMRBDSBSObservabilityEndpoint         string                `json:"namrbd_sbs_observability_endpoint,omitempty"`
	NAMRBDSBSObservabilityTimeout          time.Duration         `json:"namrbd_sbs_observability_timeout,omitempty"`
	RootAccessKeyID                        string                `json:"root_access_key_id"`
	RootSecretAccessKey                    string                `json:"-"`
}

type SBSVolumePoolMember struct {
	VolumeID             string   `json:"volume_id"`
	AdminEndpoint        string   `json:"admin_endpoint,omitempty"`
	DataEndpoint         string   `json:"data_endpoint,omitempty"`
	GatewayID            string   `json:"gateway_id,omitempty"`
	AttachmentID         string   `json:"attachment_id,omitempty"`
	Generation           uint64   `json:"generation,omitempty"`
	WriterGroupID        string   `json:"writer_group_id,omitempty"`
	VolumeEpoch          uint64   `json:"volume_epoch,omitempty"`
	ChunkSizeBytes       uint64   `json:"chunk_size_bytes,omitempty"`
	ShardStoreIDs        []string `json:"shard_store_ids,omitempty"`
	VerifyReadback       bool     `json:"verify_readback,omitempty"`
	WriteConcurrency     int      `json:"write_concurrency,omitempty"`
	ReadOnly             bool     `json:"read_only,omitempty"`
	State                string   `json:"state,omitempty"`
	Weight               int      `json:"weight,omitempty"`
	AvailableBytes       uint64   `json:"available_bytes,omitempty"`
	UsedPercent          float64  `json:"used_percent,omitempty"`
	HighWatermarkPercent float64  `json:"high_watermark_percent,omitempty"`
}

func Default() Config {
	return Config{
		ListenAddr:                        DefaultListenAddr,
		DeploymentProfile:                 DefaultDeploymentProfile,
		Edition:                           currentEdition(),
		Region:                            DefaultRegion,
		MetadataBackend:                   DefaultMetadataBackend,
		MetadataPath:                      DefaultMetadataPath,
		TiKVPDEndpoints:                   append([]string(nil), DefaultTiKVPDEndpoints...),
		TiKVAPIVersion:                    DefaultTiKVAPIVersion,
		TiKVTimeout:                       DefaultTiKVTimeout,
		TiKVRetryAttempts:                 DefaultTiKVRetryAttempts,
		TiKVRetryInitialBackoff:           DefaultTiKVRetryInitialBackoff,
		TiKVRetryMaxBackoff:               DefaultTiKVRetryMaxBackoff,
		MetadataCacheTTL:                  DefaultMetadataCacheTTL,
		StorageBackend:                    DefaultStorageBackend,
		CoordinationBackend:               DefaultCoordinationBackend,
		EtcdEndpoints:                     append([]string(nil), DefaultEtcdEndpoints...),
		EtcdDialTimeout:                   DefaultEtcdDialTimeout,
		GatewayRegistryPrefix:             DefaultGatewayRegistryPrefix,
		GatewayLeaseTTL:                   DefaultGatewayLeaseTTL,
		GatewayHeartbeat:                  DefaultGatewayHeartbeat,
		GatewayDataBudgetUnknownBytes:     DefaultGatewayDataBudgetUnknownBytes,
		DedupeSchedulerMode:               DefaultDedupeSchedulerMode,
		SBSVolumePoolRefreshInterval:      DefaultSBSVolumePoolRefreshInterval,
		SBSECShardConcurrency:             DefaultSBSECShardConcurrency,
		SBSPhysicalWriteConcurrency:       1,
		SBSPhysicalFullChunkWriteMinBytes: DefaultSBSPhysicalFullChunkWriteMinBytes,
		SBSPhysicalFullChunkWriteMaxBytes: DefaultSBSPhysicalFullChunkWriteMaxBytes,
		SBSPhysicalChunkCacheBytes:        DefaultSBSPhysicalChunkCacheBytes,
		SBSChunkIDAllocationCacheSize:     DefaultSBSChunkIDAllocationCacheSize,
		SBSSessionTTL:                     DefaultSBSSessionTTL,
		SBSSessionHeartbeat:               DefaultSBSSessionHeartbeat,
		DedupeSchedulerInterval:           DefaultDedupeSchedulerInterval,
		DedupeSchedulerLockTTL:            DefaultDedupeSchedulerLockTTL,
		DedupeSchedulerMaxKeys:            DefaultDedupeSchedulerMaxKeys,
		DedupeSchedulerLimit:              DefaultDedupeSchedulerLimit,
		DedupeSchedulerVerifyBytes:        true,
		GCWorkerShardID:                   DefaultGCWorkerShardID,
		GCWorkerInterval:                  DefaultGCWorkerInterval,
		GCWorkerLeaseTTL:                  DefaultGCWorkerLeaseTTL,
		GCWorkerLimit:                     DefaultGCWorkerLimit,
		LifecycleWorkerShardID:            DefaultLifecycleWorkerShardID,
		LifecycleWorkerInterval:           DefaultLifecycleWorkerInterval,
		LifecycleWorkerLeaseTTL:           DefaultLifecycleWorkerLeaseTTL,
		LifecycleWorkerMaxKeys:            DefaultLifecycleWorkerMaxKeys,
		LifecycleWorkerMaxUploads:         DefaultLifecycleWorkerMaxUploads,
		GCCandidateQueue:                  DefaultGCCandidateQueue,
		AccessAuditMode:                   DefaultAccessAuditMode,
		AccessAuditBatchSize:              DefaultAccessAuditBatchSize,
		AccessAuditQueueSize:              DefaultAccessAuditQueueSize,
		AccessAuditFlushInterval:          DefaultAccessAuditFlushInterval,
		ConsoleAuthMode:                   DefaultConsoleAuthMode,
		ConsoleAdminUsername:              DefaultConsoleAdminUsername,
		ConsoleSessionTTL:                 DefaultConsoleSessionTTL,
		NAMRBDSBSObservabilityTimeout:     DefaultNAMRBDSBSObservabilityTimeout,
		RootAccessKeyID:                   DefaultRootAccessKeyID,
		RootSecretAccessKey:               DefaultRootSecretKey,
	}
}

func Parse(args []string) (Config, error) {
	cfg := Default()
	if err := applyEnvironment(&cfg, args); err != nil {
		return Config{}, err
	}
	fs := flag.NewFlagSet("namros-gateway", flag.ContinueOnError)
	fs.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr, "HTTP listen address")
	fs.StringVar(&cfg.DeploymentProfile, "deployment-profile", cfg.DeploymentProfile, "deployment profile: dev or production")
	fs.BoolVar(&cfg.AllowUnsafeProductionShortcuts, "allow-unsafe-production-shortcuts", cfg.AllowUnsafeProductionShortcuts, "allow production profile to use development-only backends for lab validation")
	fs.StringVar(&cfg.Region, "region", cfg.Region, "S3-compatible default region")
	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path for pebble backend")
	tikvPDEndpoints := strings.Join(cfg.TiKVPDEndpoints, ",")
	fs.StringVar(&tikvPDEndpoints, "tikv-pd-endpoints", tikvPDEndpoints, "comma-separated TiKV PD endpoints")
	fs.StringVar(&cfg.TiKVAPIVersion, "tikv-api-version", cfg.TiKVAPIVersion, "TiKV API version for metadata backend: v1 or v2")
	fs.StringVar(&cfg.TiKVKeyspace, "tikv-keyspace", cfg.TiKVKeyspace, "TiKV keyspace name or v1 key prefix fallback")
	fs.DurationVar(&cfg.TiKVTimeout, "tikv-timeout", cfg.TiKVTimeout, "TiKV metadata operation timeout")
	fs.StringVar(&cfg.TiKVTLSCA, "tikv-tls-ca", cfg.TiKVTLSCA, "TiKV TLS CA file")
	fs.StringVar(&cfg.TiKVTLSCert, "tikv-tls-cert", cfg.TiKVTLSCert, "TiKV TLS cert file")
	fs.StringVar(&cfg.TiKVTLSKey, "tikv-tls-key", cfg.TiKVTLSKey, "TiKV TLS key file")
	fs.IntVar(&cfg.TiKVRetryAttempts, "tikv-retry-attempts", cfg.TiKVRetryAttempts, "TiKV transaction max attempts; 1 disables retry")
	fs.DurationVar(&cfg.TiKVRetryInitialBackoff, "tikv-retry-initial-backoff", cfg.TiKVRetryInitialBackoff, "TiKV transaction retry initial backoff")
	fs.DurationVar(&cfg.TiKVRetryMaxBackoff, "tikv-retry-max-backoff", cfg.TiKVRetryMaxBackoff, "TiKV transaction retry max backoff")
	fs.DurationVar(&cfg.MetadataCacheTTL, "metadata-cache-ttl", cfg.MetadataCacheTTL, "gateway-local metadata read-through cache TTL; 0 disables cache")
	fs.StringVar(&cfg.StorageBackend, "storage-backend", cfg.StorageBackend, "segment storage backend")
	fs.StringVar(&cfg.StoragePath, "storage-path", cfg.StoragePath, "segment storage path for local or sbs-local backends")
	fs.StringVar(&cfg.SBSStatePath, "sbs-state-path", cfg.SBSStatePath, "optional SBS adapter state path")
	fs.StringVar(&cfg.SBSAdminEndpoint, "sbs-admin-endpoint", cfg.SBSAdminEndpoint, "SBS admin gRPC endpoint for sbs-physical chunk allocation")
	fs.StringVar(&cfg.SBSDataEndpoint, "sbs-data-endpoint", cfg.SBSDataEndpoint, "SBS data gRPC endpoint for sbs-physical chunk IO or sbs-ec shard IO")
	fs.StringVar(&cfg.SBSVolumeID, "sbs-volume-id", cfg.SBSVolumeID, "SBS volume id for sbs-physical or sbs-ec storage")
	sbsVolumePool := ""
	applyStringEnv(&sbsVolumePool, "NAMROS_SBS_VOLUME_POOL", "sbs-volume-pool", args)
	fs.StringVar(&sbsVolumePool, "sbs-volume-pool", sbsVolumePool, "semicolon-separated SBS volume pool members; each member is volume_id=<id>,data_endpoint=<host:port>,attachment_id=<id>,writer_group_id=<id>,volume_epoch=<n>,state=<active|read_only|draining|degraded|full|offline>,weight=<n>,available_bytes=<n>,used_percent=<n>,high_watermark_percent=<n>,readonly=<bool>,shards=<id>|<id>")
	fs.StringVar(&cfg.SBSVolumePoolID, "sbs-volume-pool-id", cfg.SBSVolumePoolID, "metadata volume pool id to load for SBS-backed storage")
	fs.DurationVar(&cfg.SBSVolumePoolRefreshInterval, "sbs-volume-pool-refresh-interval", cfg.SBSVolumePoolRefreshInterval, "metadata registry refresh interval for SBS volume pools; 0 disables runtime refresh")
	fs.Uint64Var(&cfg.SBSChunkSizeBytes, "sbs-chunk-size-bytes", cfg.SBSChunkSizeBytes, "SBS physical allocation chunk size")
	fs.StringVar(&cfg.SBSGatewayID, "sbs-gateway-id", cfg.SBSGatewayID, "SBS gateway id for SBS-backed storage attachment context")
	fs.StringVar(&cfg.SBSAttachmentID, "sbs-attachment-id", cfg.SBSAttachmentID, "SBS attachment id for SBS-backed storage writer context")
	fs.Uint64Var(&cfg.SBSGeneration, "sbs-generation", cfg.SBSGeneration, "SBS attachment generation for SBS-backed storage writer context")
	fs.StringVar(&cfg.SBSWriterGroupID, "sbs-writer-group-id", cfg.SBSWriterGroupID, "SBS shared writer group id for production multi-gateway session fencing")
	fs.StringVar(&cfg.SBSSessionID, "sbs-session-id", cfg.SBSSessionID, "SBS per-gateway writer session id for production multi-gateway session fencing")
	fs.Uint64Var(&cfg.SBSVolumeEpoch, "sbs-volume-epoch", cfg.SBSVolumeEpoch, "SBS volume epoch for production writer session fencing; 0 lets the storage adapter use its default")
	fs.DurationVar(&cfg.SBSSessionTTL, "sbs-session-ttl", cfg.SBSSessionTTL, "SBS writer session lease TTL")
	fs.DurationVar(&cfg.SBSSessionHeartbeat, "sbs-session-heartbeat", cfg.SBSSessionHeartbeat, "SBS writer session heartbeat interval")
	sbsShardStoreIDs := strings.Join(cfg.SBSShardStoreIDs, ",")
	fs.StringVar(&sbsShardStoreIDs, "sbs-shard-store-ids", sbsShardStoreIDs, "comma-separated SBS store ids for sbs-ec shard placement")
	fs.IntVar(&cfg.SBSECShardConcurrency, "sbs-ec-shard-concurrency", cfg.SBSECShardConcurrency, "maximum concurrent sbs-ec shard RPCs per segment read/write or delete operation")
	fs.BoolVar(&cfg.SBSVerifyReadback, "sbs-verify-readback", cfg.SBSVerifyReadback, "verify sbs-physical writes with immediate readback")
	fs.IntVar(&cfg.SBSPhysicalWriteConcurrency, "sbs-physical-write-concurrency", cfg.SBSPhysicalWriteConcurrency, "maximum concurrent sbs-physical chunk writes per object")
	fs.Uint64Var(&cfg.SBSPhysicalFullChunkWriteMinBytes, "sbs-physical-full-chunk-write-min-bytes", cfg.SBSPhysicalFullChunkWriteMinBytes, "minimum SBS allocation chunk size eligible for full-chunk tail writes; 0 disables the lower bound")
	fs.Uint64Var(&cfg.SBSPhysicalFullChunkWriteMaxBytes, "sbs-physical-full-chunk-write-max-bytes", cfg.SBSPhysicalFullChunkWriteMaxBytes, "maximum SBS allocation chunk size eligible for full-chunk tail writes; 0 disables full-chunk tail writes")
	fs.Uint64Var(&cfg.SBSPhysicalChunkCacheBytes, "sbs-physical-chunk-cache-bytes", cfg.SBSPhysicalChunkCacheBytes, "gateway-local SBS physical chunk cache size in bytes; 0 disables cache")
	sbsChunkIDAllocationCacheSize := uint64(cfg.SBSChunkIDAllocationCacheSize)
	fs.Uint64Var(&sbsChunkIDAllocationCacheSize, "sbs-chunk-id-allocation-cache-size", sbsChunkIDAllocationCacheSize, "SBS physical chunk IDs to preallocate per volume on each cache refill; 0 disables cache")
	fs.StringVar(&cfg.CoordinationBackend, "coordination-backend", cfg.CoordinationBackend, "coordination backend: none or etcd")
	etcdEndpoints := strings.Join(cfg.EtcdEndpoints, ",")
	fs.StringVar(&etcdEndpoints, "etcd-endpoints", etcdEndpoints, "comma-separated etcd endpoints for gateway coordination")
	fs.DurationVar(&cfg.EtcdDialTimeout, "etcd-dial-timeout", cfg.EtcdDialTimeout, "etcd dial timeout for gateway coordination")
	fs.StringVar(&cfg.GatewayInstanceID, "gateway-instance-id", cfg.GatewayInstanceID, "stable gateway instance id for coordination registry")
	fs.StringVar(&cfg.GatewayAdvertiseEndpoint, "gateway-advertise-endpoint", cfg.GatewayAdvertiseEndpoint, "gateway endpoint advertised in coordination registry")
	fs.StringVar(&cfg.GatewayRegistryPrefix, "gateway-registry-prefix", cfg.GatewayRegistryPrefix, "etcd key prefix for gateway registry")
	fs.DurationVar(&cfg.GatewayLeaseTTL, "gateway-lease-ttl", cfg.GatewayLeaseTTL, "gateway registry lease TTL")
	fs.DurationVar(&cfg.GatewayHeartbeat, "gateway-heartbeat", cfg.GatewayHeartbeat, "gateway registry heartbeat interval")
	fs.Uint64Var(&cfg.GatewayDataBudgetBytes, "gateway-data-budget-bytes", cfg.GatewayDataBudgetBytes, "maximum in-flight gateway data-path bytes; 0 disables byte budget")
	fs.IntVar(&cfg.GatewayDataBudgetMaxRequests, "gateway-data-budget-max-requests", cfg.GatewayDataBudgetMaxRequests, "maximum concurrent gateway data-path requests; 0 disables request budget")
	fs.Uint64Var(&cfg.GatewayDataBudgetUnknownBytes, "gateway-data-budget-unknown-bytes", cfg.GatewayDataBudgetUnknownBytes, "bytes reserved for a data-path request whose payload size is unknown")
	fs.IntVar(&cfg.GatewayRequestMaxConcurrent, "gateway-request-max-concurrent", cfg.GatewayRequestMaxConcurrent, "maximum concurrent S3 requests per gateway; 0 disables")
	fs.IntVar(&cfg.GatewayRequestMaxConcurrentPerTenant, "gateway-request-max-concurrent-per-tenant", cfg.GatewayRequestMaxConcurrentPerTenant, "maximum concurrent S3 requests per tenant on this gateway; 0 disables")
	fs.IntVar(&cfg.GatewayRequestMaxConcurrentReads, "gateway-request-max-concurrent-reads", cfg.GatewayRequestMaxConcurrentReads, "maximum concurrent read-class S3 requests per gateway; 0 disables")
	fs.IntVar(&cfg.GatewayRequestMaxConcurrentWrites, "gateway-request-max-concurrent-writes", cfg.GatewayRequestMaxConcurrentWrites, "maximum concurrent write-class S3 requests per gateway; 0 disables")
	fs.Int64Var(&cfg.GatewayUploadBandwidthBytesPerSecond, "gateway-upload-bandwidth-bytes-per-second", cfg.GatewayUploadBandwidthBytesPerSecond, "gateway-local upload bandwidth limit in bytes per second; 0 disables")
	fs.Int64Var(&cfg.GatewayDownloadBandwidthBytesPerSecond, "gateway-download-bandwidth-bytes-per-second", cfg.GatewayDownloadBandwidthBytesPerSecond, "gateway-local download bandwidth limit in bytes per second; 0 disables")
	fs.IntVar(&cfg.BackgroundWorkerMaxConcurrent, "background-worker-max-concurrent", cfg.BackgroundWorkerMaxConcurrent, "maximum concurrent background worker runs per gateway; 0 disables")
	fs.IntVar(&cfg.BackgroundWorkerMaxConcurrentPerTenant, "background-worker-max-concurrent-per-tenant", cfg.BackgroundWorkerMaxConcurrentPerTenant, "maximum concurrent background worker runs per tenant on this gateway; 0 disables")
	fs.IntVar(&cfg.BackgroundWorkerMaxConcurrentPerPool, "background-worker-max-concurrent-per-pool", cfg.BackgroundWorkerMaxConcurrentPerPool, "maximum concurrent background worker runs per SBS pool on this gateway; 0 disables")
	fs.BoolVar(&cfg.DedupeSchedulerEnabled, "dedupe-scheduler-enabled", cfg.DedupeSchedulerEnabled, "enable background dedupe scheduler")
	fs.StringVar(&cfg.DedupeSchedulerTenantID, "dedupe-scheduler-tenant-id", cfg.DedupeSchedulerTenantID, "tenant id scanned by background dedupe scheduler")
	fs.StringVar(&cfg.DedupeSchedulerBucketID, "dedupe-scheduler-bucket-id", cfg.DedupeSchedulerBucketID, "optional bucket id scanned by background dedupe scheduler")
	fs.StringVar(&cfg.DedupeSchedulerPrefix, "dedupe-scheduler-prefix", cfg.DedupeSchedulerPrefix, "optional object key prefix scanned by background dedupe scheduler")
	fs.StringVar(&cfg.DedupeSchedulerMode, "dedupe-scheduler-mode", cfg.DedupeSchedulerMode, "dedupe scheduler mode")
	fs.DurationVar(&cfg.DedupeSchedulerInterval, "dedupe-scheduler-interval", cfg.DedupeSchedulerInterval, "background dedupe scheduler interval")
	fs.DurationVar(&cfg.DedupeSchedulerLockTTL, "dedupe-scheduler-lock-ttl", cfg.DedupeSchedulerLockTTL, "background dedupe scheduler metadata lock TTL")
	fs.IntVar(&cfg.DedupeSchedulerMaxKeys, "dedupe-scheduler-max-keys", cfg.DedupeSchedulerMaxKeys, "metadata list page size for background dedupe scheduler")
	fs.IntVar(&cfg.DedupeSchedulerLimit, "dedupe-scheduler-limit", cfg.DedupeSchedulerLimit, "maximum candidate pairs processed by each background dedupe scheduler run")
	fs.BoolVar(&cfg.DedupeSchedulerVerifyBytes, "dedupe-scheduler-verify-bytes", cfg.DedupeSchedulerVerifyBytes, "verify candidate bytes before background dedupe ack")
	fs.BoolVar(&cfg.GCWorkerEnabled, "gc-worker-enabled", cfg.GCWorkerEnabled, "enable distributed orphan GC worker")
	fs.StringVar(&cfg.GCWorkerShardID, "gc-worker-shard-id", cfg.GCWorkerShardID, "distributed orphan GC worker shard id")
	fs.DurationVar(&cfg.GCWorkerInterval, "gc-worker-interval", cfg.GCWorkerInterval, "distributed orphan GC worker interval")
	fs.DurationVar(&cfg.GCWorkerLeaseTTL, "gc-worker-lease-ttl", cfg.GCWorkerLeaseTTL, "distributed orphan GC worker lease ttl")
	fs.IntVar(&cfg.GCWorkerLimit, "gc-worker-limit", cfg.GCWorkerLimit, "maximum orphan GC candidates processed by each worker run")
	fs.BoolVar(&cfg.LifecycleWorkerEnabled, "lifecycle-worker-enabled", cfg.LifecycleWorkerEnabled, "enable distributed bucket lifecycle worker")
	fs.StringVar(&cfg.LifecycleWorkerShardID, "lifecycle-worker-shard-id", cfg.LifecycleWorkerShardID, "distributed bucket lifecycle worker shard id")
	fs.StringVar(&cfg.LifecycleWorkerBucketID, "lifecycle-worker-bucket-id", cfg.LifecycleWorkerBucketID, "bucket id processed by the distributed lifecycle worker")
	fs.DurationVar(&cfg.LifecycleWorkerInterval, "lifecycle-worker-interval", cfg.LifecycleWorkerInterval, "distributed bucket lifecycle worker interval")
	fs.DurationVar(&cfg.LifecycleWorkerLeaseTTL, "lifecycle-worker-lease-ttl", cfg.LifecycleWorkerLeaseTTL, "distributed bucket lifecycle worker lease ttl")
	fs.IntVar(&cfg.LifecycleWorkerMaxKeys, "lifecycle-worker-max-keys", cfg.LifecycleWorkerMaxKeys, "maximum object versions planned by each lifecycle worker run")
	fs.IntVar(&cfg.LifecycleWorkerMaxUploads, "lifecycle-worker-max-uploads", cfg.LifecycleWorkerMaxUploads, "maximum multipart uploads planned by each lifecycle worker run")
	fs.StringVar(&cfg.GCCandidateQueue, "gc-candidate-queue", cfg.GCCandidateQueue, "orphan GC candidate queue backend: storage or metadata")
	fs.StringVar(&cfg.AccessAuditMode, "access-audit-mode", cfg.AccessAuditMode, "S3 read/list access audit mode: sync or async")
	fs.IntVar(&cfg.AccessAuditBatchSize, "access-audit-batch-size", cfg.AccessAuditBatchSize, "maximum access audit events written per async metadata batch")
	fs.IntVar(&cfg.AccessAuditQueueSize, "access-audit-queue-size", cfg.AccessAuditQueueSize, "maximum queued async access audit events")
	fs.DurationVar(&cfg.AccessAuditFlushInterval, "access-audit-flush-interval", cfg.AccessAuditFlushInterval, "maximum async access audit flush delay")
	fs.StringVar(&cfg.ConsoleAuthMode, "console-auth-mode", cfg.ConsoleAuthMode, "console auth mode: disabled or local")
	fs.StringVar(&cfg.ConsoleAdminUsername, "console-admin-username", cfg.ConsoleAdminUsername, "bootstrap local console admin username")
	fs.StringVar(&cfg.ConsoleAdminPassword, "console-admin-password", cfg.ConsoleAdminPassword, "bootstrap local console admin password")
	fs.StringVar(&cfg.ConsoleSessionSecret, "console-session-secret", cfg.ConsoleSessionSecret, "shared console session signing secret")
	fs.DurationVar(&cfg.ConsoleSessionTTL, "console-session-ttl", cfg.ConsoleSessionTTL, "console session lifetime")
	fs.StringVar(&cfg.ObservabilityPrometheusURL, "observability-prometheus-url", cfg.ObservabilityPrometheusURL, "external Prometheus base URL shown in console")
	fs.StringVar(&cfg.ObservabilityGrafanaURL, "observability-grafana-url", cfg.ObservabilityGrafanaURL, "external Grafana base URL shown in console")
	fs.StringVar(&cfg.ObservabilityVictoriaURL, "observability-victoria-url", cfg.ObservabilityVictoriaURL, "external VictoriaMetrics base URL shown in console")
	fs.StringVar(&cfg.NAMRBDSBSObservabilityEndpoint, "namrbd-sbs-observability-endpoint", cfg.NAMRBDSBSObservabilityEndpoint, "NAMRBD SBS read-only observability endpoint or base URL")
	fs.DurationVar(&cfg.NAMRBDSBSObservabilityTimeout, "namrbd-sbs-observability-timeout", cfg.NAMRBDSBSObservabilityTimeout, "NAMRBD SBS observability request timeout")
	fs.StringVar(&cfg.RootAccessKeyID, "root-access-key-id", cfg.RootAccessKeyID, "bootstrap root access key id")
	fs.StringVar(&cfg.RootSecretAccessKey, "root-secret-access-key", cfg.RootSecretAccessKey, "bootstrap root secret access key")

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}
	if fs.NArg() != 0 {
		return Config{}, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	cfg.TiKVPDEndpoints = splitCommaList(tikvPDEndpoints)
	cfg.EtcdEndpoints = splitCommaList(etcdEndpoints)
	cfg.SBSShardStoreIDs = splitCommaList(sbsShardStoreIDs)
	if sbsChunkIDAllocationCacheSize > 1<<32-1 {
		return Config{}, fmt.Errorf("sbs chunk id allocation cache size exceeds uint32: %d", sbsChunkIDAllocationCacheSize)
	}
	cfg.SBSChunkIDAllocationCacheSize = uint32(sbsChunkIDAllocationCacheSize)
	volumePool, err := ParseSBSVolumePoolSpec(sbsVolumePool)
	if err != nil {
		return Config{}, err
	}
	cfg.SBSVolumePool = volumePool
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	cfg.Edition = edition.Normalize(cfg.Edition)
	cfg.DeploymentProfile = NormalizeDeploymentProfile(cfg.DeploymentProfile)
	cfg.MetadataBackend = NormalizeMetadataBackend(cfg.MetadataBackend)
	cfg.StorageBackend = NormalizeStorageBackend(cfg.StorageBackend)
	cfg.CoordinationBackend = NormalizeCoordinationBackend(cfg.CoordinationBackend)
	cfg.AccessAuditMode = NormalizeAccessAuditMode(cfg.AccessAuditMode)
	cfg.ConsoleAuthMode = NormalizeConsoleAuthMode(cfg.ConsoleAuthMode)
	return cfg, nil
}

func applyEnvironment(cfg *Config, args []string) error {
	applyStringEnv(&cfg.ListenAddr, "NAMROS_LISTEN", "listen", args)
	applyStringEnv(&cfg.DeploymentProfile, "NAMROS_DEPLOYMENT_PROFILE", "deployment-profile", args)
	if err := applyBoolEnv(&cfg.AllowUnsafeProductionShortcuts, "NAMROS_ALLOW_UNSAFE_PRODUCTION_SHORTCUTS", "allow-unsafe-production-shortcuts", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.Region, "NAMROS_REGION", "region", args)
	applyStringEnv(&cfg.MetadataBackend, "NAMROS_METADATA_BACKEND", "metadata-backend", args)
	applyStringEnv(&cfg.MetadataPath, "NAMROS_METADATA_PATH", "metadata-path", args)
	applyStringListEnv(&cfg.TiKVPDEndpoints, "NAMROS_TIKV_PD_ENDPOINTS", "tikv-pd-endpoints", args)
	applyStringEnv(&cfg.TiKVAPIVersion, "NAMROS_TIKV_API_VERSION", "tikv-api-version", args)
	applyStringEnv(&cfg.TiKVKeyspace, "NAMROS_TIKV_KEYSPACE", "tikv-keyspace", args)
	if err := applyDurationEnv(&cfg.TiKVTimeout, "NAMROS_TIKV_TIMEOUT", "tikv-timeout", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.TiKVTLSCA, "NAMROS_TIKV_TLS_CA", "tikv-tls-ca", args)
	applyStringEnv(&cfg.TiKVTLSCert, "NAMROS_TIKV_TLS_CERT", "tikv-tls-cert", args)
	applyStringEnv(&cfg.TiKVTLSKey, "NAMROS_TIKV_TLS_KEY", "tikv-tls-key", args)
	if err := applyIntEnv(&cfg.TiKVRetryAttempts, "NAMROS_TIKV_RETRY_ATTEMPTS", "tikv-retry-attempts", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.TiKVRetryInitialBackoff, "NAMROS_TIKV_RETRY_INITIAL_BACKOFF", "tikv-retry-initial-backoff", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.TiKVRetryMaxBackoff, "NAMROS_TIKV_RETRY_MAX_BACKOFF", "tikv-retry-max-backoff", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.MetadataCacheTTL, "NAMROS_METADATA_CACHE_TTL", "metadata-cache-ttl", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.StorageBackend, "NAMROS_STORAGE_BACKEND", "storage-backend", args)
	applyStringEnv(&cfg.StoragePath, "NAMROS_STORAGE_PATH", "storage-path", args)
	applyStringEnv(&cfg.SBSStatePath, "NAMROS_SBS_STATE_PATH", "sbs-state-path", args)
	applyStringEnv(&cfg.SBSAdminEndpoint, "NAMROS_SBS_ADMIN_ENDPOINT", "sbs-admin-endpoint", args)
	applyStringEnv(&cfg.SBSDataEndpoint, "NAMROS_SBS_DATA_ENDPOINT", "sbs-data-endpoint", args)
	applyStringEnv(&cfg.SBSVolumeID, "NAMROS_SBS_VOLUME_ID", "sbs-volume-id", args)
	if err := applyUint64Env(&cfg.SBSChunkSizeBytes, "NAMROS_SBS_CHUNK_SIZE_BYTES", "sbs-chunk-size-bytes", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.SBSGatewayID, "NAMROS_SBS_GATEWAY_ID", "sbs-gateway-id", args)
	applyStringEnv(&cfg.SBSAttachmentID, "NAMROS_SBS_ATTACHMENT_ID", "sbs-attachment-id", args)
	if err := applyUint64Env(&cfg.SBSGeneration, "NAMROS_SBS_GENERATION", "sbs-generation", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.SBSWriterGroupID, "NAMROS_SBS_WRITER_GROUP_ID", "sbs-writer-group-id", args)
	applyStringEnv(&cfg.SBSSessionID, "NAMROS_SBS_SESSION_ID", "sbs-session-id", args)
	if err := applyUint64Env(&cfg.SBSVolumeEpoch, "NAMROS_SBS_VOLUME_EPOCH", "sbs-volume-epoch", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.SBSSessionTTL, "NAMROS_SBS_SESSION_TTL", "sbs-session-ttl", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.SBSSessionHeartbeat, "NAMROS_SBS_SESSION_HEARTBEAT", "sbs-session-heartbeat", args); err != nil {
		return err
	}
	applyStringListEnv(&cfg.SBSShardStoreIDs, "NAMROS_SBS_SHARD_STORE_IDS", "sbs-shard-store-ids", args)
	if err := applyIntEnv(&cfg.SBSECShardConcurrency, "NAMROS_SBS_EC_SHARD_CONCURRENCY", "sbs-ec-shard-concurrency", args); err != nil {
		return err
	}
	if err := applyBoolEnv(&cfg.SBSVerifyReadback, "NAMROS_SBS_VERIFY_READBACK", "sbs-verify-readback", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.SBSPhysicalWriteConcurrency, "NAMROS_SBS_PHYSICAL_WRITE_CONCURRENCY", "sbs-physical-write-concurrency", args); err != nil {
		return err
	}
	if err := applyUint64Env(&cfg.SBSPhysicalFullChunkWriteMinBytes, "NAMROS_SBS_PHYSICAL_FULL_CHUNK_WRITE_MIN_BYTES", "sbs-physical-full-chunk-write-min-bytes", args); err != nil {
		return err
	}
	if err := applyUint64Env(&cfg.SBSPhysicalFullChunkWriteMaxBytes, "NAMROS_SBS_PHYSICAL_FULL_CHUNK_WRITE_MAX_BYTES", "sbs-physical-full-chunk-write-max-bytes", args); err != nil {
		return err
	}
	if err := applyUint64Env(&cfg.SBSPhysicalChunkCacheBytes, "NAMROS_SBS_PHYSICAL_CHUNK_CACHE_BYTES", "sbs-physical-chunk-cache-bytes", args); err != nil {
		return err
	}
	if err := applyUint32Env(&cfg.SBSChunkIDAllocationCacheSize, "NAMROS_SBS_CHUNK_ID_ALLOCATION_CACHE_SIZE", "sbs-chunk-id-allocation-cache-size", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.SBSVolumePoolID, "NAMROS_SBS_VOLUME_POOL_ID", "sbs-volume-pool-id", args)
	if err := applyDurationEnv(&cfg.SBSVolumePoolRefreshInterval, "NAMROS_SBS_VOLUME_POOL_REFRESH_INTERVAL", "sbs-volume-pool-refresh-interval", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.CoordinationBackend, "NAMROS_COORDINATION_BACKEND", "coordination-backend", args)
	applyStringListEnv(&cfg.EtcdEndpoints, "NAMROS_ETCD_ENDPOINTS", "etcd-endpoints", args)
	if err := applyDurationEnv(&cfg.EtcdDialTimeout, "NAMROS_ETCD_DIAL_TIMEOUT", "etcd-dial-timeout", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.GatewayInstanceID, "NAMROS_GATEWAY_INSTANCE_ID", "gateway-instance-id", args)
	applyStringEnv(&cfg.GatewayAdvertiseEndpoint, "NAMROS_GATEWAY_ADVERTISE_ENDPOINT", "gateway-advertise-endpoint", args)
	applyStringEnv(&cfg.GatewayRegistryPrefix, "NAMROS_GATEWAY_REGISTRY_PREFIX", "gateway-registry-prefix", args)
	if err := applyDurationEnv(&cfg.GatewayLeaseTTL, "NAMROS_GATEWAY_LEASE_TTL", "gateway-lease-ttl", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.GatewayHeartbeat, "NAMROS_GATEWAY_HEARTBEAT", "gateway-heartbeat", args); err != nil {
		return err
	}
	if err := applyUint64Env(&cfg.GatewayDataBudgetBytes, "NAMROS_GATEWAY_DATA_BUDGET_BYTES", "gateway-data-budget-bytes", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.GatewayDataBudgetMaxRequests, "NAMROS_GATEWAY_DATA_BUDGET_MAX_REQUESTS", "gateway-data-budget-max-requests", args); err != nil {
		return err
	}
	if err := applyUint64Env(&cfg.GatewayDataBudgetUnknownBytes, "NAMROS_GATEWAY_DATA_BUDGET_UNKNOWN_BYTES", "gateway-data-budget-unknown-bytes", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.GatewayRequestMaxConcurrent, "NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT", "gateway-request-max-concurrent", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.GatewayRequestMaxConcurrentPerTenant, "NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT_PER_TENANT", "gateway-request-max-concurrent-per-tenant", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.GatewayRequestMaxConcurrentReads, "NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT_READS", "gateway-request-max-concurrent-reads", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.GatewayRequestMaxConcurrentWrites, "NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT_WRITES", "gateway-request-max-concurrent-writes", args); err != nil {
		return err
	}
	if err := applyInt64Env(&cfg.GatewayUploadBandwidthBytesPerSecond, "NAMROS_GATEWAY_UPLOAD_BANDWIDTH_BYTES_PER_SECOND", "gateway-upload-bandwidth-bytes-per-second", args); err != nil {
		return err
	}
	if err := applyInt64Env(&cfg.GatewayDownloadBandwidthBytesPerSecond, "NAMROS_GATEWAY_DOWNLOAD_BANDWIDTH_BYTES_PER_SECOND", "gateway-download-bandwidth-bytes-per-second", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.BackgroundWorkerMaxConcurrent, "NAMROS_BACKGROUND_WORKER_MAX_CONCURRENT", "background-worker-max-concurrent", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.BackgroundWorkerMaxConcurrentPerTenant, "NAMROS_BACKGROUND_WORKER_MAX_CONCURRENT_PER_TENANT", "background-worker-max-concurrent-per-tenant", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.BackgroundWorkerMaxConcurrentPerPool, "NAMROS_BACKGROUND_WORKER_MAX_CONCURRENT_PER_POOL", "background-worker-max-concurrent-per-pool", args); err != nil {
		return err
	}
	if err := applyBoolEnv(&cfg.DedupeSchedulerEnabled, "NAMROS_DEDUPE_SCHEDULER_ENABLED", "dedupe-scheduler-enabled", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.DedupeSchedulerTenantID, "NAMROS_DEDUPE_SCHEDULER_TENANT_ID", "dedupe-scheduler-tenant-id", args)
	applyStringEnv(&cfg.DedupeSchedulerBucketID, "NAMROS_DEDUPE_SCHEDULER_BUCKET_ID", "dedupe-scheduler-bucket-id", args)
	applyStringEnv(&cfg.DedupeSchedulerPrefix, "NAMROS_DEDUPE_SCHEDULER_PREFIX", "dedupe-scheduler-prefix", args)
	applyStringEnv(&cfg.DedupeSchedulerMode, "NAMROS_DEDUPE_SCHEDULER_MODE", "dedupe-scheduler-mode", args)
	if err := applyDurationEnv(&cfg.DedupeSchedulerInterval, "NAMROS_DEDUPE_SCHEDULER_INTERVAL", "dedupe-scheduler-interval", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.DedupeSchedulerLockTTL, "NAMROS_DEDUPE_SCHEDULER_LOCK_TTL", "dedupe-scheduler-lock-ttl", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.DedupeSchedulerMaxKeys, "NAMROS_DEDUPE_SCHEDULER_MAX_KEYS", "dedupe-scheduler-max-keys", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.DedupeSchedulerLimit, "NAMROS_DEDUPE_SCHEDULER_LIMIT", "dedupe-scheduler-limit", args); err != nil {
		return err
	}
	if err := applyBoolEnv(&cfg.DedupeSchedulerVerifyBytes, "NAMROS_DEDUPE_SCHEDULER_VERIFY_BYTES", "dedupe-scheduler-verify-bytes", args); err != nil {
		return err
	}
	if err := applyBoolEnv(&cfg.GCWorkerEnabled, "NAMROS_GC_WORKER_ENABLED", "gc-worker-enabled", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.GCWorkerShardID, "NAMROS_GC_WORKER_SHARD_ID", "gc-worker-shard-id", args)
	if err := applyDurationEnv(&cfg.GCWorkerInterval, "NAMROS_GC_WORKER_INTERVAL", "gc-worker-interval", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.GCWorkerLeaseTTL, "NAMROS_GC_WORKER_LEASE_TTL", "gc-worker-lease-ttl", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.GCWorkerLimit, "NAMROS_GC_WORKER_LIMIT", "gc-worker-limit", args); err != nil {
		return err
	}
	if err := applyBoolEnv(&cfg.LifecycleWorkerEnabled, "NAMROS_LIFECYCLE_WORKER_ENABLED", "lifecycle-worker-enabled", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.LifecycleWorkerShardID, "NAMROS_LIFECYCLE_WORKER_SHARD_ID", "lifecycle-worker-shard-id", args)
	applyStringEnv(&cfg.LifecycleWorkerBucketID, "NAMROS_LIFECYCLE_WORKER_BUCKET_ID", "lifecycle-worker-bucket-id", args)
	if err := applyDurationEnv(&cfg.LifecycleWorkerInterval, "NAMROS_LIFECYCLE_WORKER_INTERVAL", "lifecycle-worker-interval", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.LifecycleWorkerLeaseTTL, "NAMROS_LIFECYCLE_WORKER_LEASE_TTL", "lifecycle-worker-lease-ttl", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.LifecycleWorkerMaxKeys, "NAMROS_LIFECYCLE_WORKER_MAX_KEYS", "lifecycle-worker-max-keys", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.LifecycleWorkerMaxUploads, "NAMROS_LIFECYCLE_WORKER_MAX_UPLOADS", "lifecycle-worker-max-uploads", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.GCCandidateQueue, "NAMROS_GC_CANDIDATE_QUEUE", "gc-candidate-queue", args)
	applyStringEnv(&cfg.AccessAuditMode, "NAMROS_ACCESS_AUDIT_MODE", "access-audit-mode", args)
	if err := applyIntEnv(&cfg.AccessAuditBatchSize, "NAMROS_ACCESS_AUDIT_BATCH_SIZE", "access-audit-batch-size", args); err != nil {
		return err
	}
	if err := applyIntEnv(&cfg.AccessAuditQueueSize, "NAMROS_ACCESS_AUDIT_QUEUE_SIZE", "access-audit-queue-size", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.AccessAuditFlushInterval, "NAMROS_ACCESS_AUDIT_FLUSH_INTERVAL", "access-audit-flush-interval", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.ConsoleAuthMode, "NAMROS_CONSOLE_AUTH_MODE", "console-auth-mode", args)
	applyStringEnv(&cfg.ConsoleAdminUsername, "NAMROS_CONSOLE_ADMIN_USERNAME", "console-admin-username", args)
	if err := applySecretEnv(&cfg.ConsoleAdminPassword, "NAMROS_CONSOLE_ADMIN_PASSWORD", "NAMROS_CONSOLE_ADMIN_PASSWORD_FILE", "console-admin-password", args); err != nil {
		return err
	}
	if err := applySecretEnv(&cfg.ConsoleSessionSecret, "NAMROS_CONSOLE_SESSION_SECRET", "NAMROS_CONSOLE_SESSION_SECRET_FILE", "console-session-secret", args); err != nil {
		return err
	}
	if err := applyDurationEnv(&cfg.ConsoleSessionTTL, "NAMROS_CONSOLE_SESSION_TTL", "console-session-ttl", args); err != nil {
		return err
	}
	applyStringEnv(&cfg.ObservabilityPrometheusURL, "NAMROS_OBSERVABILITY_PROMETHEUS_URL", "observability-prometheus-url", args)
	applyStringEnv(&cfg.ObservabilityGrafanaURL, "NAMROS_OBSERVABILITY_GRAFANA_URL", "observability-grafana-url", args)
	applyStringEnv(&cfg.ObservabilityVictoriaURL, "NAMROS_OBSERVABILITY_VICTORIA_URL", "observability-victoria-url", args)
	applyStringEnv(&cfg.NAMRBDSBSObservabilityEndpoint, "NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT", "namrbd-sbs-observability-endpoint", args)
	if err := applyDurationEnv(&cfg.NAMRBDSBSObservabilityTimeout, "NAMROS_NAMRBD_SBS_OBSERVABILITY_TIMEOUT", "namrbd-sbs-observability-timeout", args); err != nil {
		return err
	}
	if err := applySecretEnv(&cfg.RootAccessKeyID, "NAMROS_ROOT_ACCESS_KEY_ID", "NAMROS_ROOT_ACCESS_KEY_ID_FILE", "root-access-key-id", args); err != nil {
		return err
	}
	return applySecretEnv(&cfg.RootSecretAccessKey, "NAMROS_ROOT_SECRET_ACCESS_KEY", "NAMROS_ROOT_SECRET_ACCESS_KEY_FILE", "root-secret-access-key", args)
}

func applyStringEnv(dst *string, envName, flagName string, args []string) {
	if argHasFlag(args, flagName) {
		return
	}
	if value, ok := os.LookupEnv(envName); ok {
		*dst = value
	}
}

func applyStringListEnv(dst *[]string, envName, flagName string, args []string) {
	if argHasFlag(args, flagName) {
		return
	}
	if value, ok := os.LookupEnv(envName); ok {
		*dst = splitCommaList(value)
	}
}

func applyBoolEnv(dst *bool, envName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	value, ok := os.LookupEnv(envName)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", envName, value, err)
	}
	*dst = parsed
	return nil
}

func applyIntEnv(dst *int, envName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	value, ok := os.LookupEnv(envName)
	if !ok {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", envName, value, err)
	}
	*dst = parsed
	return nil
}

func applyInt64Env(dst *int64, envName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	value, ok := os.LookupEnv(envName)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", envName, value, err)
	}
	*dst = parsed
	return nil
}

func applyUint64Env(dst *uint64, envName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	value, ok := os.LookupEnv(envName)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", envName, value, err)
	}
	*dst = parsed
	return nil
}

func applyUint32Env(dst *uint32, envName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	raw, ok := os.LookupEnv(envName)
	if !ok {
		return nil
	}
	parsed, err := strconv.ParseUint(raw, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", envName, raw, err)
	}
	*dst = uint32(parsed)
	return nil
}

func applyDurationEnv(dst *time.Duration, envName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	value, ok := os.LookupEnv(envName)
	if !ok {
		return nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("invalid %s value %q: %w", envName, value, err)
	}
	*dst = parsed
	return nil
}

func applySecretEnv(dst *string, envName, fileEnvName, flagName string, args []string) error {
	if argHasFlag(args, flagName) {
		return nil
	}
	value, valueOK := os.LookupEnv(envName)
	filePath, fileOK := os.LookupEnv(fileEnvName)
	if valueOK && fileOK {
		return fmt.Errorf("%s and %s cannot be set together", envName, fileEnvName)
	}
	if valueOK {
		*dst = value
		return nil
	}
	if !fileOK {
		return nil
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read %s path %q: %w", fileEnvName, filePath, err)
	}
	*dst = strings.TrimRight(string(raw), "\r\n")
	return nil
}

func argHasFlag(args []string, name string) bool {
	short := "-" + name
	long := "--" + name
	for _, arg := range args {
		if arg == short || arg == long || strings.HasPrefix(arg, short+"=") || strings.HasPrefix(arg, long+"=") {
			return true
		}
	}
	return false
}

func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return errors.New("listen address is required")
	}
	switch NormalizeDeploymentProfile(c.DeploymentProfile) {
	case DeploymentProfileDev, DeploymentProfileProduction:
	default:
		return fmt.Errorf("unsupported deployment profile %q", c.DeploymentProfile)
	}
	if err := edition.Validate(c.Edition); err != nil {
		return err
	}
	if c.Region == "" {
		return errors.New("region is required")
	}
	if c.MetadataBackend == "" {
		return errors.New("metadata backend is required")
	}
	switch NormalizeMetadataBackend(c.MetadataBackend) {
	case MetadataBackendMemory:
	case MetadataBackendPebble:
		if strings.TrimSpace(c.MetadataPath) == "" {
			return errors.New("metadata path is required for pebble metadata backend")
		}
	case MetadataBackendTiKV:
		if len(cleanStringList(c.TiKVPDEndpoints)) == 0 {
			return errors.New("tikv pd endpoints are required for tikv metadata backend")
		}
		switch strings.ToLower(strings.TrimSpace(c.TiKVAPIVersion)) {
		case "", "v1", "v2":
		default:
			return fmt.Errorf("unsupported tikv api version %q", c.TiKVAPIVersion)
		}
		if c.TiKVTimeout < 0 {
			return errors.New("tikv timeout cannot be negative")
		}
		if c.TiKVRetryAttempts < 0 {
			return errors.New("tikv retry attempts cannot be negative")
		}
		if c.TiKVRetryInitialBackoff < 0 {
			return errors.New("tikv retry initial backoff cannot be negative")
		}
		if c.TiKVRetryMaxBackoff < 0 {
			return errors.New("tikv retry max backoff cannot be negative")
		}
		if c.TiKVRetryInitialBackoff > 0 && c.TiKVRetryMaxBackoff > 0 && c.TiKVRetryInitialBackoff > c.TiKVRetryMaxBackoff {
			return errors.New("tikv retry initial backoff cannot exceed max backoff")
		}
		tlsFields := 0
		for _, value := range []string{c.TiKVTLSCA, c.TiKVTLSCert, c.TiKVTLSKey} {
			if strings.TrimSpace(value) != "" {
				tlsFields++
			}
		}
		if tlsFields != 0 && tlsFields != 3 {
			return errors.New("tikv tls requires ca, cert, and key paths")
		}
	default:
		return fmt.Errorf("unsupported metadata backend %q", c.MetadataBackend)
	}
	if c.MetadataCacheTTL < 0 {
		return errors.New("metadata cache ttl cannot be negative")
	}
	if c.SBSVolumePoolRefreshInterval < 0 {
		return errors.New("sbs volume pool refresh interval cannot be negative")
	}
	if c.SBSSessionTTL <= 0 {
		return errors.New("sbs session ttl must be positive")
	}
	if c.SBSSessionHeartbeat <= 0 {
		return errors.New("sbs session heartbeat must be positive")
	}
	if c.SBSSessionHeartbeat >= c.SBSSessionTTL {
		return errors.New("sbs session heartbeat must be shorter than sbs session ttl")
	}
	if err := validateSBSSessionConfig(c); err != nil {
		return err
	}
	if c.GatewayDataBudgetMaxRequests < 0 {
		return errors.New("gateway data budget max requests cannot be negative")
	}
	if c.GatewayRequestMaxConcurrent < 0 {
		return errors.New("gateway request max concurrent cannot be negative")
	}
	if c.GatewayRequestMaxConcurrentPerTenant < 0 {
		return errors.New("gateway request max concurrent per tenant cannot be negative")
	}
	if c.GatewayRequestMaxConcurrentReads < 0 {
		return errors.New("gateway request max concurrent reads cannot be negative")
	}
	if c.GatewayRequestMaxConcurrentWrites < 0 {
		return errors.New("gateway request max concurrent writes cannot be negative")
	}
	if c.GatewayUploadBandwidthBytesPerSecond < 0 {
		return errors.New("gateway upload bandwidth bytes per second cannot be negative")
	}
	if c.GatewayDownloadBandwidthBytesPerSecond < 0 {
		return errors.New("gateway download bandwidth bytes per second cannot be negative")
	}
	if c.BackgroundWorkerMaxConcurrent < 0 {
		return errors.New("background worker max concurrent cannot be negative")
	}
	if c.BackgroundWorkerMaxConcurrentPerTenant < 0 {
		return errors.New("background worker max concurrent per tenant cannot be negative")
	}
	if c.BackgroundWorkerMaxConcurrentPerPool < 0 {
		return errors.New("background worker max concurrent per pool cannot be negative")
	}
	if c.SBSPhysicalWriteConcurrency < 1 {
		return errors.New("sbs physical write concurrency must be positive")
	}
	if c.SBSECShardConcurrency < 1 {
		return errors.New("sbs ec shard concurrency must be positive")
	}
	if c.SBSPhysicalFullChunkWriteMaxBytes > 0 && c.SBSPhysicalFullChunkWriteMinBytes > c.SBSPhysicalFullChunkWriteMaxBytes {
		return errors.New("sbs physical full chunk write min bytes cannot exceed max bytes")
	}
	if c.StorageBackend == "" {
		return errors.New("storage backend is required")
	}
	if strings.TrimSpace(c.SBSVolumePoolID) != "" && len(c.SBSVolumePool) > 0 {
		return errors.New("sbs volume pool id cannot be combined with static sbs volume pool members")
	}
	switch NormalizeStorageBackend(c.StorageBackend) {
	case StorageBackendMemory:
	case StorageBackendLocal, StorageBackendSBS, StorageBackendSBSLocal:
		if strings.TrimSpace(c.StoragePath) == "" {
			return errors.New("storage path is required for local and sbs storage backends")
		}
	case StorageBackendSBSPhysical:
		if strings.TrimSpace(c.SBSVolumePoolID) != "" {
			break
		}
		if len(c.SBSVolumePool) > 0 {
			if err := validateSBSVolumePool(c.SBSVolumePool, true, true, c.SBSAdminEndpoint, c.SBSDataEndpoint); err != nil {
				return err
			}
			break
		}
		if strings.TrimSpace(c.SBSAdminEndpoint) == "" {
			return errors.New("sbs admin endpoint is required for sbs-physical storage backend")
		}
		if strings.TrimSpace(c.SBSDataEndpoint) == "" {
			return errors.New("sbs data endpoint is required for sbs-physical storage backend")
		}
	case StorageBackendSBSEC:
		if err := edition.Require(c.Edition, edition.FeatureErasureCoding); err != nil {
			return err
		}
		if strings.TrimSpace(c.SBSVolumePoolID) != "" {
			break
		}
		if len(c.SBSVolumePool) > 0 {
			if err := validateSBSVolumePool(c.SBSVolumePool, false, true, c.SBSAdminEndpoint, c.SBSDataEndpoint); err != nil {
				return err
			}
			break
		}
		if strings.TrimSpace(c.SBSDataEndpoint) == "" {
			return errors.New("sbs data endpoint is required for sbs-ec storage backend")
		}
	case StorageBackendSBSCluster:
		if len(cleanStringList(c.SBSShardStoreIDs)) > 0 {
			if err := edition.Require(c.Edition, edition.FeatureErasureCoding); err != nil {
				return err
			}
		}
		if err := edition.Require(c.Edition, edition.FeatureSBSReplicatedObject); err != nil {
			return err
		}
		if strings.TrimSpace(c.SBSVolumePoolID) != "" {
			break
		}
		if len(c.SBSVolumePool) > 0 {
			if err := validateSBSVolumePool(c.SBSVolumePool, true, true, c.SBSAdminEndpoint, c.SBSDataEndpoint); err != nil {
				return err
			}
			break
		}
		if strings.TrimSpace(c.SBSAdminEndpoint) == "" {
			return errors.New("sbs admin endpoint is required for sbs-cluster storage backend")
		}
		if strings.TrimSpace(c.SBSDataEndpoint) == "" {
			return errors.New("sbs data endpoint is required for sbs-cluster storage backend")
		}
	default:
		return fmt.Errorf("unsupported storage backend %q", c.StorageBackend)
	}
	switch NormalizeCoordinationBackend(c.CoordinationBackend) {
	case CoordinationBackendNone:
	case CoordinationBackendEtcd:
		if len(cleanStringList(c.EtcdEndpoints)) == 0 {
			return errors.New("etcd endpoints are required for etcd coordination backend")
		}
		if strings.TrimSpace(c.GatewayRegistryPrefix) == "" {
			return errors.New("gateway registry prefix is required for etcd coordination backend")
		}
		if c.EtcdDialTimeout < 0 {
			return errors.New("etcd dial timeout cannot be negative")
		}
		if c.GatewayLeaseTTL <= 0 {
			return errors.New("gateway lease ttl must be positive")
		}
		if c.GatewayHeartbeat <= 0 {
			return errors.New("gateway heartbeat must be positive")
		}
		if c.GatewayHeartbeat >= c.GatewayLeaseTTL {
			return errors.New("gateway heartbeat must be shorter than gateway lease ttl")
		}
	default:
		return fmt.Errorf("unsupported coordination backend %q", c.CoordinationBackend)
	}
	if c.RootAccessKeyID == "" {
		return errors.New("root access key id is required")
	}
	switch NormalizeConsoleAuthMode(c.ConsoleAuthMode) {
	case "disabled":
	case "local":
		if strings.TrimSpace(c.ConsoleAdminUsername) == "" {
			return errors.New("console admin username is required for local console auth")
		}
		if c.ConsoleAdminPassword == "" {
			return errors.New("console admin password is required for local console auth")
		}
		if len(c.ConsoleSessionSecret) < 16 {
			return errors.New("console session secret must be at least 16 bytes for local console auth")
		}
		if c.ConsoleSessionTTL <= 0 {
			return errors.New("console session ttl must be positive")
		}
	default:
		return fmt.Errorf("unsupported console auth mode %q", c.ConsoleAuthMode)
	}
	switch NormalizeGCCandidateQueue(c.GCCandidateQueue) {
	case GCCandidateQueueStorage, GCCandidateQueueMetadata:
	default:
		return fmt.Errorf("unsupported gc candidate queue %q", c.GCCandidateQueue)
	}
	switch NormalizeAccessAuditMode(c.AccessAuditMode) {
	case AccessAuditModeSync, AccessAuditModeAsync:
	default:
		return fmt.Errorf("unsupported access audit mode %q", c.AccessAuditMode)
	}
	if c.AccessAuditBatchSize < 1 {
		return errors.New("access audit batch size must be positive")
	}
	if c.AccessAuditQueueSize < 1 {
		return errors.New("access audit queue size must be positive")
	}
	if c.AccessAuditFlushInterval < 0 {
		return errors.New("access audit flush interval cannot be negative")
	}
	if c.NAMRBDSBSObservabilityTimeout < 0 {
		return errors.New("namrbd sbs observability timeout cannot be negative")
	}
	if c.DedupeSchedulerEnabled {
		if err := edition.Require(c.Edition, edition.FeatureDedupe); err != nil {
			return err
		}
		if strings.TrimSpace(c.DedupeSchedulerTenantID) == "" {
			return errors.New("dedupe scheduler tenant id is required when scheduler is enabled")
		}
		if c.DedupeSchedulerInterval <= 0 {
			return errors.New("dedupe scheduler interval must be positive")
		}
		if c.DedupeSchedulerLockTTL <= 0 {
			return errors.New("dedupe scheduler lock ttl must be positive")
		}
		if c.DedupeSchedulerMaxKeys < 0 {
			return errors.New("dedupe scheduler max keys cannot be negative")
		}
		if c.DedupeSchedulerLimit < 0 {
			return errors.New("dedupe scheduler limit cannot be negative")
		}
		if strings.TrimSpace(c.DedupeSchedulerMode) == "" {
			return errors.New("dedupe scheduler mode is required")
		}
	}
	if c.GCWorkerEnabled {
		if strings.TrimSpace(c.GCWorkerShardID) == "" {
			return errors.New("gc worker shard id is required when worker is enabled")
		}
		if c.GCWorkerInterval <= 0 {
			return errors.New("gc worker interval must be positive")
		}
		if c.GCWorkerLeaseTTL <= 0 {
			return errors.New("gc worker lease ttl must be positive")
		}
		if c.GCWorkerLimit < 0 {
			return errors.New("gc worker limit cannot be negative")
		}
	}
	if c.LifecycleWorkerEnabled {
		if strings.TrimSpace(c.LifecycleWorkerShardID) == "" {
			return errors.New("lifecycle worker shard id is required when worker is enabled")
		}
		if strings.TrimSpace(c.LifecycleWorkerBucketID) == "" {
			return errors.New("lifecycle worker bucket id is required when worker is enabled")
		}
		if c.LifecycleWorkerInterval <= 0 {
			return errors.New("lifecycle worker interval must be positive")
		}
		if c.LifecycleWorkerLeaseTTL <= 0 {
			return errors.New("lifecycle worker lease ttl must be positive")
		}
		if c.LifecycleWorkerMaxKeys < 0 {
			return errors.New("lifecycle worker max keys cannot be negative")
		}
		if c.LifecycleWorkerMaxUploads < 0 {
			return errors.New("lifecycle worker max uploads cannot be negative")
		}
	}
	if c.RootSecretAccessKey == "" {
		return errors.New("root secret access key is required")
	}
	if NormalizeDeploymentProfile(c.DeploymentProfile) == DeploymentProfileProduction {
		if c.AllowUnsafeProductionShortcuts {
			return nil
		}
		if err := c.validateProductionProfile(); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) validateProductionProfile() error {
	if NormalizeMetadataBackend(c.MetadataBackend) != MetadataBackendTiKV {
		return fmt.Errorf("production deployment profile requires tikv metadata backend (got %q)", c.MetadataBackend)
	}
	if NormalizeCoordinationBackend(c.CoordinationBackend) != CoordinationBackendEtcd {
		return fmt.Errorf("production deployment profile requires etcd coordination backend (got %q)", c.CoordinationBackend)
	}
	if NormalizeGCCandidateQueue(c.GCCandidateQueue) != GCCandidateQueueMetadata {
		return fmt.Errorf("production deployment profile requires metadata gc candidate queue (got %q)", c.GCCandidateQueue)
	}
	switch NormalizeStorageBackend(c.StorageBackend) {
	case StorageBackendSBSPhysical, StorageBackendSBSEC, StorageBackendSBSCluster:
	default:
		return fmt.Errorf("production deployment profile requires an SBS volume-pool storage backend (got %q)", c.StorageBackend)
	}
	if strings.TrimSpace(c.SBSVolumePoolID) != "" {
		return c.validateProductionSBSWriterSessionFencing()
	}
	if len(c.SBSVolumePool) < 2 {
		return errors.New("production deployment profile requires an sbs volume pool id or at least two static sbs volume pool members")
	}
	return c.validateProductionSBSWriterSessionFencing()
}

func (c Config) validateProductionSBSWriterSessionFencing() error {
	if strings.TrimSpace(c.SBSSessionID) == "" {
		return errors.New("production deployment profile requires sbs writer session fencing; shared attachment lab bridge is non-production")
	}
	if strings.TrimSpace(c.SBSVolumePoolID) != "" {
		if strings.TrimSpace(c.SBSWriterGroupID) == "" {
			return errors.New("production deployment profile requires sbs writer group id when using an sbs volume pool registry")
		}
		if c.SBSVolumeEpoch == 0 {
			return errors.New("production deployment profile requires sbs volume epoch when using an sbs volume pool registry")
		}
		return nil
	}
	if strings.TrimSpace(c.SBSWriterGroupID) == "" && !allSBSVolumePoolMembersHaveWriterGroup(c.SBSVolumePool) {
		return errors.New("production deployment profile requires sbs writer group id or per-member writer_group_id for static sbs volume pools")
	}
	if c.SBSVolumeEpoch != 0 {
		return nil
	}
	for _, member := range c.SBSVolumePool {
		if member.VolumeEpoch == 0 {
			return fmt.Errorf("production deployment profile requires sbs volume epoch or per-member volume_epoch for volume pool member %q", member.VolumeID)
		}
	}
	return nil
}

func SBSSessionFencingConfigured(c Config) bool {
	if strings.TrimSpace(c.SBSSessionID) == "" {
		return false
	}
	if strings.TrimSpace(c.SBSVolumePoolID) != "" {
		return strings.TrimSpace(c.SBSWriterGroupID) != "" && c.SBSVolumeEpoch != 0
	}
	if strings.TrimSpace(c.SBSWriterGroupID) == "" && !allSBSVolumePoolMembersHaveWriterGroup(c.SBSVolumePool) {
		return false
	}
	if c.SBSVolumeEpoch != 0 {
		return len(c.SBSVolumePool) > 0
	}
	if len(c.SBSVolumePool) == 0 {
		return false
	}
	for _, member := range c.SBSVolumePool {
		if member.VolumeEpoch == 0 {
			return false
		}
	}
	return true
}

func NormalizeDeploymentProfile(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultDeploymentProfile
	}
	return value
}

func NormalizeMetadataBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeStorageBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeCoordinationBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func NormalizeGCCandidateQueue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultGCCandidateQueue
	}
	return value
}

func NormalizeAccessAuditMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultAccessAuditMode
	}
	return value
}

func NormalizeConsoleAuthMode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return DefaultConsoleAuthMode
	}
	return value
}

func splitCommaList(raw string) []string {
	return cleanStringList(strings.Split(raw, ","))
}

func ParseSBSVolumePoolSpec(raw string) ([]SBSVolumePoolMember, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	memberSpecs := strings.Split(raw, ";")
	out := make([]SBSVolumePoolMember, 0, len(memberSpecs))
	for _, memberSpec := range memberSpecs {
		memberSpec = strings.TrimSpace(memberSpec)
		if memberSpec == "" {
			continue
		}
		member := SBSVolumePoolMember{}
		fields := strings.Split(memberSpec, ",")
		for _, field := range fields {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			key, value, ok := strings.Cut(field, "=")
			if !ok {
				if member.VolumeID == "" {
					member.VolumeID = field
					continue
				}
				return nil, fmt.Errorf("invalid sbs volume pool member field %q", field)
			}
			key = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
			value = strings.TrimSpace(value)
			switch key {
			case "volume_id", "volume":
				member.VolumeID = value
			case "admin_endpoint", "admin":
				member.AdminEndpoint = value
			case "data_endpoint", "data":
				member.DataEndpoint = value
			case "gateway_id", "gateway":
				member.GatewayID = value
			case "attachment_id", "attachment":
				member.AttachmentID = value
			case "generation":
				parsed, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool generation %q: %w", value, err)
				}
				member.Generation = parsed
			case "writer_group_id", "writer_group":
				member.WriterGroupID = value
			case "volume_epoch", "epoch":
				parsed, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool volume_epoch %q: %w", value, err)
				}
				member.VolumeEpoch = parsed
			case "chunk_size_bytes", "chunk_size":
				parsed, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool chunk_size_bytes %q: %w", value, err)
				}
				member.ChunkSizeBytes = parsed
			case "shard_store_ids", "shards":
				member.ShardStoreIDs = splitSBSVolumePoolList(value)
			case "verify_readback":
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool verify_readback %q: %w", value, err)
				}
				member.VerifyReadback = parsed
			case "write_concurrency", "physical_write_concurrency", "sbs_physical_write_concurrency":
				parsed, err := strconv.Atoi(value)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool write_concurrency %q: %w", value, err)
				}
				member.WriteConcurrency = parsed
			case "readonly", "read_only":
				parsed, err := strconv.ParseBool(value)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool readonly %q: %w", value, err)
				}
				member.ReadOnly = parsed
			case "state":
				member.State = normalizeSBSVolumePoolState(value)
			case "weight":
				parsed, err := strconv.Atoi(value)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool weight %q: %w", value, err)
				}
				member.Weight = parsed
			case "available_bytes", "available":
				parsed, err := strconv.ParseUint(value, 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool available_bytes %q: %w", value, err)
				}
				member.AvailableBytes = parsed
			case "used_percent", "used":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool used_percent %q: %w", value, err)
				}
				member.UsedPercent = parsed
			case "high_watermark_percent", "high_watermark", "watermark":
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid sbs volume pool high_watermark_percent %q: %w", value, err)
				}
				member.HighWatermarkPercent = parsed
			default:
				return nil, fmt.Errorf("unsupported sbs volume pool member field %q", key)
			}
		}
		if strings.TrimSpace(member.VolumeID) == "" {
			return nil, errors.New("sbs volume pool member volume_id is required")
		}
		out = append(out, member)
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func validateSBSVolumePool(members []SBSVolumePoolMember, requireAdmin, requireData bool, defaultAdmin, defaultData string) error {
	seen := make(map[string]struct{}, len(members))
	for _, member := range members {
		volumeID := strings.TrimSpace(member.VolumeID)
		if volumeID == "" {
			return errors.New("sbs volume pool member volume_id is required")
		}
		if _, exists := seen[volumeID]; exists {
			return fmt.Errorf("duplicate sbs volume pool member %q", volumeID)
		}
		seen[volumeID] = struct{}{}
		if requireAdmin && strings.TrimSpace(member.AdminEndpoint) == "" && strings.TrimSpace(defaultAdmin) == "" {
			return fmt.Errorf("sbs admin endpoint is required for volume pool member %q", volumeID)
		}
		if requireData && strings.TrimSpace(member.DataEndpoint) == "" && strings.TrimSpace(defaultData) == "" {
			return fmt.Errorf("sbs data endpoint is required for volume pool member %q", volumeID)
		}
		if err := validateSBSVolumePoolAdmission(member); err != nil {
			return err
		}
	}
	return nil
}

func validateSBSSessionConfig(c Config) error {
	writerGroupID := strings.TrimSpace(c.SBSWriterGroupID)
	sessionID := strings.TrimSpace(c.SBSSessionID)
	topLevelEnabled := writerGroupID != "" || sessionID != "" || c.SBSVolumeEpoch != 0
	memberSessionEnabled := false
	memberMissingWriterGroup := ""
	memberMissingEpoch := ""
	for _, member := range c.SBSVolumePool {
		memberWriterGroupID := strings.TrimSpace(member.WriterGroupID)
		if memberWriterGroupID != "" || member.VolumeEpoch != 0 {
			memberSessionEnabled = true
		}
		if memberWriterGroupID == "" && writerGroupID == "" && member.VolumeEpoch != 0 {
			memberMissingWriterGroup = member.VolumeID
		}
		if memberWriterGroupID != "" && member.VolumeEpoch == 0 && c.SBSVolumeEpoch == 0 {
			memberMissingEpoch = member.VolumeID
		}
	}
	if topLevelEnabled || memberSessionEnabled {
		if sessionID == "" {
			return errors.New("sbs session id is required when sbs writer session fencing is configured")
		}
		if writerGroupID == "" && memberMissingWriterGroup != "" {
			return fmt.Errorf("sbs writer group id is required for volume pool member %q when volume_epoch is set", memberMissingWriterGroup)
		}
		if writerGroupID == "" && !allSBSVolumePoolMembersHaveWriterGroup(c.SBSVolumePool) {
			return errors.New("sbs writer group id is required unless every volume pool member has writer_group_id")
		}
		if memberMissingEpoch != "" {
			return fmt.Errorf("sbs volume epoch is required for volume pool member %q when writer_group_id is set", memberMissingEpoch)
		}
	}
	return nil
}

func allSBSVolumePoolMembersHaveWriterGroup(members []SBSVolumePoolMember) bool {
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if strings.TrimSpace(member.WriterGroupID) == "" {
			return false
		}
	}
	return true
}

func validateSBSVolumePoolAdmission(member SBSVolumePoolMember) error {
	state := normalizeSBSVolumePoolState(member.State)
	switch state {
	case "", "active", "read_only", "draining", "degraded", "full", "offline":
	default:
		return fmt.Errorf("unsupported sbs volume pool state %q for member %q", member.State, member.VolumeID)
	}
	if member.Weight < 0 || member.Weight > 1024 {
		return fmt.Errorf("sbs volume pool weight for member %q must be between 0 and 1024", member.VolumeID)
	}
	if member.WriteConcurrency < 0 {
		return fmt.Errorf("sbs volume pool write_concurrency for member %q cannot be negative", member.VolumeID)
	}
	if member.UsedPercent < 0 || member.UsedPercent > 100 {
		return fmt.Errorf("sbs volume pool used_percent for member %q must be between 0 and 100", member.VolumeID)
	}
	if member.HighWatermarkPercent < 0 || member.HighWatermarkPercent > 100 {
		return fmt.Errorf("sbs volume pool high_watermark_percent for member %q must be between 0 and 100", member.VolumeID)
	}
	return nil
}

func normalizeSBSVolumePoolState(state string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(state, "-", "_")))
}

func splitSBSVolumePoolList(raw string) []string {
	if strings.Contains(raw, "|") {
		return cleanStringList(strings.Split(raw, "|"))
	}
	return splitCommaList(raw)
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

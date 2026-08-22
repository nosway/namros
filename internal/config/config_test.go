package config

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/edition"
)

func withCurrentEdition(t *testing.T, value string) {
	t.Helper()
	old := currentEdition
	currentEdition = func() string { return value }
	t.Cleanup(func() {
		currentEdition = old
	})
}

func clearNAMROSEnv(t *testing.T) {
	t.Helper()
	previous := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(name, "NAMROS_") {
			continue
		}
		previous[name] = value
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("Unsetenv(%s): %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, entry := range os.Environ() {
			name, _, ok := strings.Cut(entry, "=")
			if ok && strings.HasPrefix(name, "NAMROS_") {
				_ = os.Unsetenv(name)
			}
		}
		for name, value := range previous {
			_ = os.Setenv(name, value)
		}
	})
}

func TestParseDefaults(t *testing.T) {
	clearNAMROSEnv(t)
	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil) error = %v", err)
	}
	if cfg.ListenAddr != DefaultListenAddr {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, DefaultListenAddr)
	}
	if cfg.DeploymentProfile != DefaultDeploymentProfile {
		t.Fatalf("DeploymentProfile = %q, want %q", cfg.DeploymentProfile, DefaultDeploymentProfile)
	}
	if cfg.AllowUnsafeProductionShortcuts {
		t.Fatal("AllowUnsafeProductionShortcuts = true, want default false")
	}
	if cfg.Edition != DefaultEdition {
		t.Fatalf("Edition = %q, want %q", cfg.Edition, DefaultEdition)
	}
	if cfg.Region != DefaultRegion {
		t.Fatalf("Region = %q, want %q", cfg.Region, DefaultRegion)
	}
	if cfg.MetadataBackend != DefaultMetadataBackend {
		t.Fatalf("MetadataBackend = %q, want %q", cfg.MetadataBackend, DefaultMetadataBackend)
	}
	if cfg.MetadataPath != DefaultMetadataPath {
		t.Fatalf("MetadataPath = %q, want %q", cfg.MetadataPath, DefaultMetadataPath)
	}
	if len(cfg.TiKVPDEndpoints) != 1 || cfg.TiKVPDEndpoints[0] != DefaultTiKVPDEndpoints[0] {
		t.Fatalf("TiKVPDEndpoints = %v, want %v", cfg.TiKVPDEndpoints, DefaultTiKVPDEndpoints)
	}
	if cfg.TiKVAPIVersion != DefaultTiKVAPIVersion {
		t.Fatalf("TiKVAPIVersion = %q, want %q", cfg.TiKVAPIVersion, DefaultTiKVAPIVersion)
	}
	if cfg.TiKVTimeout != DefaultTiKVTimeout {
		t.Fatalf("TiKVTimeout = %s, want %s", cfg.TiKVTimeout, DefaultTiKVTimeout)
	}
	if cfg.SBSSessionTTL != DefaultSBSSessionTTL || cfg.SBSSessionHeartbeat != DefaultSBSSessionHeartbeat {
		t.Fatalf("SBS session defaults = ttl:%s heartbeat:%s, want %s/%s", cfg.SBSSessionTTL, cfg.SBSSessionHeartbeat, DefaultSBSSessionTTL, DefaultSBSSessionHeartbeat)
	}
	if cfg.TiKVRetryAttempts != DefaultTiKVRetryAttempts {
		t.Fatalf("TiKVRetryAttempts = %d, want %d", cfg.TiKVRetryAttempts, DefaultTiKVRetryAttempts)
	}
	if cfg.TiKVRetryInitialBackoff != DefaultTiKVRetryInitialBackoff {
		t.Fatalf("TiKVRetryInitialBackoff = %s, want %s", cfg.TiKVRetryInitialBackoff, DefaultTiKVRetryInitialBackoff)
	}
	if cfg.TiKVRetryMaxBackoff != DefaultTiKVRetryMaxBackoff {
		t.Fatalf("TiKVRetryMaxBackoff = %s, want %s", cfg.TiKVRetryMaxBackoff, DefaultTiKVRetryMaxBackoff)
	}
	if cfg.MetadataCacheTTL != DefaultMetadataCacheTTL {
		t.Fatalf("MetadataCacheTTL = %s, want %s", cfg.MetadataCacheTTL, DefaultMetadataCacheTTL)
	}
	if cfg.StorageBackend != DefaultStorageBackend {
		t.Fatalf("StorageBackend = %q, want %q", cfg.StorageBackend, DefaultStorageBackend)
	}
	if cfg.SBSPhysicalWriteConcurrency != 1 {
		t.Fatalf("SBSPhysicalWriteConcurrency = %d, want 1", cfg.SBSPhysicalWriteConcurrency)
	}
	if cfg.SBSECShardConcurrency != DefaultSBSECShardConcurrency {
		t.Fatalf("SBSECShardConcurrency = %d, want %d", cfg.SBSECShardConcurrency, DefaultSBSECShardConcurrency)
	}
	if cfg.SBSPhysicalFullChunkWriteMinBytes != DefaultSBSPhysicalFullChunkWriteMinBytes || cfg.SBSPhysicalFullChunkWriteMaxBytes != DefaultSBSPhysicalFullChunkWriteMaxBytes || cfg.SBSPhysicalChunkCacheBytes != DefaultSBSPhysicalChunkCacheBytes {
		t.Fatalf("SBS physical optimization defaults = min:%d max:%d cache:%d", cfg.SBSPhysicalFullChunkWriteMinBytes, cfg.SBSPhysicalFullChunkWriteMaxBytes, cfg.SBSPhysicalChunkCacheBytes)
	}
	if cfg.SBSChunkIDAllocationCacheSize != DefaultSBSChunkIDAllocationCacheSize {
		t.Fatalf("SBSChunkIDAllocationCacheSize = %d, want %d", cfg.SBSChunkIDAllocationCacheSize, DefaultSBSChunkIDAllocationCacheSize)
	}
	if cfg.CoordinationBackend != DefaultCoordinationBackend {
		t.Fatalf("CoordinationBackend = %q, want %q", cfg.CoordinationBackend, DefaultCoordinationBackend)
	}
	if len(cfg.EtcdEndpoints) != 1 || cfg.EtcdEndpoints[0] != DefaultEtcdEndpoints[0] {
		t.Fatalf("EtcdEndpoints = %v, want %v", cfg.EtcdEndpoints, DefaultEtcdEndpoints)
	}
	if cfg.EtcdDialTimeout != DefaultEtcdDialTimeout {
		t.Fatalf("EtcdDialTimeout = %s, want %s", cfg.EtcdDialTimeout, DefaultEtcdDialTimeout)
	}
	if cfg.GatewayRegistryPrefix != DefaultGatewayRegistryPrefix {
		t.Fatalf("GatewayRegistryPrefix = %q, want %q", cfg.GatewayRegistryPrefix, DefaultGatewayRegistryPrefix)
	}
	if cfg.GatewayLeaseTTL != DefaultGatewayLeaseTTL || cfg.GatewayHeartbeat != DefaultGatewayHeartbeat {
		t.Fatalf("Gateway lease/heartbeat = %s/%s", cfg.GatewayLeaseTTL, cfg.GatewayHeartbeat)
	}
	if cfg.GatewayDataBudgetBytes != 0 || cfg.GatewayDataBudgetMaxRequests != 0 || cfg.GatewayDataBudgetUnknownBytes != DefaultGatewayDataBudgetUnknownBytes {
		t.Fatalf("Gateway data budget defaults = bytes:%d requests:%d unknown:%d", cfg.GatewayDataBudgetBytes, cfg.GatewayDataBudgetMaxRequests, cfg.GatewayDataBudgetUnknownBytes)
	}
	if cfg.GatewayRequestMaxConcurrent != 0 || cfg.GatewayRequestMaxConcurrentPerTenant != 0 || cfg.GatewayRequestMaxConcurrentReads != 0 || cfg.GatewayRequestMaxConcurrentWrites != 0 {
		t.Fatalf("Gateway request limiter defaults = global:%d tenant:%d reads:%d writes:%d", cfg.GatewayRequestMaxConcurrent, cfg.GatewayRequestMaxConcurrentPerTenant, cfg.GatewayRequestMaxConcurrentReads, cfg.GatewayRequestMaxConcurrentWrites)
	}
	if cfg.GatewayUploadBandwidthBytesPerSecond != 0 || cfg.GatewayDownloadBandwidthBytesPerSecond != 0 {
		t.Fatalf("Gateway bandwidth defaults = upload:%d download:%d", cfg.GatewayUploadBandwidthBytesPerSecond, cfg.GatewayDownloadBandwidthBytesPerSecond)
	}
	if cfg.BackgroundWorkerMaxConcurrent != 0 || cfg.BackgroundWorkerMaxConcurrentPerTenant != 0 || cfg.BackgroundWorkerMaxConcurrentPerPool != 0 {
		t.Fatalf("Background worker budget defaults = global:%d tenant:%d pool:%d", cfg.BackgroundWorkerMaxConcurrent, cfg.BackgroundWorkerMaxConcurrentPerTenant, cfg.BackgroundWorkerMaxConcurrentPerPool)
	}
	if cfg.DedupeSchedulerEnabled {
		t.Fatal("DedupeSchedulerEnabled = true, want default disabled")
	}
	if cfg.DedupeSchedulerInterval != DefaultDedupeSchedulerInterval || cfg.DedupeSchedulerLockTTL != DefaultDedupeSchedulerLockTTL {
		t.Fatalf("Dedupe scheduler interval/lock ttl = %s/%s", cfg.DedupeSchedulerInterval, cfg.DedupeSchedulerLockTTL)
	}
	if cfg.DedupeSchedulerMaxKeys != DefaultDedupeSchedulerMaxKeys || cfg.DedupeSchedulerLimit != DefaultDedupeSchedulerLimit || !cfg.DedupeSchedulerVerifyBytes {
		t.Fatalf("Dedupe scheduler scan defaults = max:%d limit:%d verify:%v", cfg.DedupeSchedulerMaxKeys, cfg.DedupeSchedulerLimit, cfg.DedupeSchedulerVerifyBytes)
	}
	if cfg.GCWorkerEnabled {
		t.Fatal("GCWorkerEnabled = true, want default disabled")
	}
	if cfg.GCWorkerShardID != DefaultGCWorkerShardID || cfg.GCWorkerInterval != DefaultGCWorkerInterval || cfg.GCWorkerLeaseTTL != DefaultGCWorkerLeaseTTL || cfg.GCWorkerLimit != DefaultGCWorkerLimit {
		t.Fatalf("GC worker defaults = shard:%q interval:%s lease:%s limit:%d", cfg.GCWorkerShardID, cfg.GCWorkerInterval, cfg.GCWorkerLeaseTTL, cfg.GCWorkerLimit)
	}
	if cfg.LifecycleWorkerEnabled {
		t.Fatal("LifecycleWorkerEnabled = true, want default disabled")
	}
	if cfg.LifecycleWorkerShardID != DefaultLifecycleWorkerShardID || cfg.LifecycleWorkerInterval != DefaultLifecycleWorkerInterval || cfg.LifecycleWorkerLeaseTTL != DefaultLifecycleWorkerLeaseTTL {
		t.Fatalf("Lifecycle worker defaults = shard:%q interval:%s lease:%s", cfg.LifecycleWorkerShardID, cfg.LifecycleWorkerInterval, cfg.LifecycleWorkerLeaseTTL)
	}
	if cfg.LifecycleWorkerMaxKeys != DefaultLifecycleWorkerMaxKeys || cfg.LifecycleWorkerMaxUploads != DefaultLifecycleWorkerMaxUploads {
		t.Fatalf("Lifecycle worker limits = max_keys:%d max_uploads:%d", cfg.LifecycleWorkerMaxKeys, cfg.LifecycleWorkerMaxUploads)
	}
	if cfg.GCCandidateQueue != DefaultGCCandidateQueue {
		t.Fatalf("GCCandidateQueue = %q, want %q", cfg.GCCandidateQueue, DefaultGCCandidateQueue)
	}
	if cfg.AccessAuditMode != DefaultAccessAuditMode || cfg.AccessAuditBatchSize != DefaultAccessAuditBatchSize || cfg.AccessAuditQueueSize != DefaultAccessAuditQueueSize || cfg.AccessAuditFlushInterval != DefaultAccessAuditFlushInterval {
		t.Fatalf("Access audit defaults = mode:%q batch:%d queue:%d flush:%s", cfg.AccessAuditMode, cfg.AccessAuditBatchSize, cfg.AccessAuditQueueSize, cfg.AccessAuditFlushInterval)
	}
	if cfg.NAMRBDSBSObservabilityEndpoint != "" || cfg.NAMRBDSBSObservabilityTimeout != DefaultNAMRBDSBSObservabilityTimeout {
		t.Fatalf("NAMRBD SBS observability defaults = endpoint:%q timeout:%s", cfg.NAMRBDSBSObservabilityEndpoint, cfg.NAMRBDSBSObservabilityTimeout)
	}
	if cfg.RootAccessKeyID != DefaultRootAccessKeyID {
		t.Fatalf("RootAccessKeyID = %q, want %q", cfg.RootAccessKeyID, DefaultRootAccessKeyID)
	}
	if cfg.RootSecretAccessKey != DefaultRootSecretKey {
		t.Fatalf("RootSecretAccessKey = %q, want default", cfg.RootSecretAccessKey)
	}
}

func TestParseHelp(t *testing.T) {
	clearNAMROSEnv(t)
	var output bytes.Buffer
	_, err := parse([]string{"--help"}, &output)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parse(--help) error = %v, want flag.ErrHelp", err)
	}
	help := output.String()
	if !strings.Contains(help, "Usage of namros-gateway:") {
		t.Fatalf("help output missing usage header: %q", help)
	}
	if !strings.Contains(help, "-http-listen string") {
		t.Fatalf("help output missing -http-listen: %q", help)
	}
	if strings.Contains(help, "  -listen string") {
		t.Fatalf("help output contains obsolete -listen flag: %q", help)
	}
}

func TestParseRejectsSBSAdminEndpointFlag(t *testing.T) {
	clearNAMROSEnv(t)
	var output bytes.Buffer
	_, err := parse([]string{"--sbs-admin-endpoint", "sbs.example:9443"}, &output)
	if err == nil || !strings.Contains(err.Error(), "sbs-admin-endpoint") {
		t.Fatalf("parse(removed SBS endpoint flag) error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "-sbs-service-endpoint") {
		t.Fatalf("flag output = %q, want canonical SBS service endpoint flag", got)
	}
}

func TestParseSBSServiceEndpointEnvironmentCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name        string
		serviceEnv  string
		adminEnv    string
		args        []string
		want        string
		wantWarning bool
	}{
		{
			name:       "canonical environment",
			serviceEnv: "service.example:9443",
			want:       "service.example:9443",
		},
		{
			name:        "deprecated environment fallback",
			adminEnv:    "admin.example:9443",
			want:        "admin.example:9443",
			wantWarning: true,
		},
		{
			name:        "canonical environment wins",
			serviceEnv:  "service.example:9443",
			adminEnv:    "admin.example:9443",
			want:        "service.example:9443",
			wantWarning: true,
		},
		{
			name:        "canonical flag wins",
			serviceEnv:  "service.example:9443",
			adminEnv:    "admin.example:9443",
			args:        []string{"--sbs-service-endpoint", "flag.example:9443"},
			want:        "flag.example:9443",
			wantWarning: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clearNAMROSEnv(t)
			if tc.serviceEnv != "" {
				t.Setenv("NAMROS_SBS_SERVICE_ENDPOINT", tc.serviceEnv)
			}
			if tc.adminEnv != "" {
				t.Setenv("NAMROS_SBS_ADMIN_ENDPOINT", tc.adminEnv)
			}
			var output bytes.Buffer
			cfg, err := parse(tc.args, &output)
			if err != nil {
				t.Fatalf("parse() error = %v", err)
			}
			if cfg.SBSAdminEndpoint != tc.want {
				t.Fatalf("SBSAdminEndpoint = %q, want %q", cfg.SBSAdminEndpoint, tc.want)
			}
			gotWarning := strings.Contains(output.String(), "NAMROS_SBS_ADMIN_ENDPOINT is deprecated; use NAMROS_SBS_SERVICE_ENDPOINT instead")
			if gotWarning != tc.wantWarning {
				t.Fatalf("output = %q, deprecation warning = %t, want %t", output.String(), gotWarning, tc.wantWarning)
			}
		})
	}
}

func TestParseOverrides(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	cfg, err := Parse([]string{
		"-http-listen", "127.0.0.1:19000",
		"-deployment-profile", "dev",
		"-allow-unsafe-production-shortcuts=false",
		"-region", "ap-northeast-2",
		"-metadata-backend", "tikv",
		"-tikv-pd-endpoints", "127.0.0.1:2379,127.0.0.2:2379",
		"-tikv-api-version", "v2",
		"-tikv-keyspace", "namros-prod",
		"-tikv-timeout", "5s",
		"-tikv-tls-ca", "/certs/ca.crt",
		"-tikv-tls-cert", "/certs/client.crt",
		"-tikv-tls-key", "/certs/client.key",
		"-tikv-retry-attempts", "5",
		"-tikv-retry-initial-backoff", "20ms",
		"-tikv-retry-max-backoff", "250ms",
		"-metadata-cache-ttl", "2s",
		"-storage-backend", "sbs",
		"-storage-path", "/tmp/namros-sbs",
		"-coordination-backend", "etcd",
		"-etcd-endpoints", "127.0.0.1:2379,127.0.0.2:2379",
		"-etcd-dial-timeout", "4s",
		"-gateway-instance-id", "gw-a",
		"-gateway-advertise-endpoint", "10.0.0.10:9000",
		"-gateway-registry-prefix", "/namros/prod/gateways",
		"-gateway-lease-ttl", "15s",
		"-gateway-heartbeat", "5s",
		"-gateway-data-budget-bytes", "1048576",
		"-gateway-data-budget-max-requests", "7",
		"-gateway-data-budget-unknown-bytes", "65536",
		"-gateway-request-max-concurrent", "40",
		"-gateway-request-max-concurrent-per-tenant", "12",
		"-gateway-request-max-concurrent-reads", "30",
		"-gateway-request-max-concurrent-writes", "10",
		"-gateway-upload-bandwidth-bytes-per-second", "10485760",
		"-gateway-download-bandwidth-bytes-per-second", "20971520",
		"-background-worker-max-concurrent", "4",
		"-background-worker-max-concurrent-per-tenant", "2",
		"-background-worker-max-concurrent-per-pool", "3",
		"-dedupe-scheduler-enabled",
		"-dedupe-scheduler-tenant-id", "tenant-1",
		"-dedupe-scheduler-bucket-id", "bucket-1",
		"-dedupe-scheduler-prefix", "backups/",
		"-dedupe-scheduler-mode", "ingest_assisted",
		"-dedupe-scheduler-interval", "30s",
		"-dedupe-scheduler-lock-ttl", "2m",
		"-dedupe-scheduler-max-keys", "200",
		"-dedupe-scheduler-limit", "25",
		"-dedupe-scheduler-verify-bytes=false",
		"-gc-worker-enabled",
		"-gc-worker-shard-id", "orphans-a",
		"-gc-worker-interval", "45s",
		"-gc-worker-lease-ttl", "3m",
		"-gc-worker-limit", "33",
		"-lifecycle-worker-enabled",
		"-lifecycle-worker-shard-id", "bucket-shard-a",
		"-lifecycle-worker-bucket-id", "bucket-1",
		"-lifecycle-worker-interval", "55s",
		"-lifecycle-worker-lease-ttl", "4m",
		"-lifecycle-worker-max-keys", "44",
		"-lifecycle-worker-max-uploads", "45",
		"-gc-candidate-queue", "metadata",
		"-access-audit-mode", "async",
		"-access-audit-batch-size", "64",
		"-access-audit-queue-size", "2048",
		"-access-audit-flush-interval", "25ms",
		"-namrbd-sbs-observability-endpoint", "http://namrbd-observability:19110",
		"-namrbd-sbs-observability-timeout", "750ms",
		"-root-access-key-id", "root",
		"-root-secret-access-key", "secret",
	})
	if err != nil {
		t.Fatalf("Parse(overrides) error = %v", err)
	}
	if cfg.ListenAddr != "127.0.0.1:19000" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
	if cfg.DeploymentProfile != DeploymentProfileDev {
		t.Fatalf("DeploymentProfile = %q", cfg.DeploymentProfile)
	}
	if cfg.AllowUnsafeProductionShortcuts {
		t.Fatal("AllowUnsafeProductionShortcuts = true, want false")
	}
	if cfg.Edition != "enterprise" {
		t.Fatalf("Edition = %q", cfg.Edition)
	}
	if cfg.Region != "ap-northeast-2" {
		t.Fatalf("Region = %q", cfg.Region)
	}
	if cfg.MetadataBackend != "tikv" {
		t.Fatalf("MetadataBackend = %q", cfg.MetadataBackend)
	}
	if got, want := cfg.TiKVPDEndpoints, []string{"127.0.0.1:2379", "127.0.0.2:2379"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("TiKVPDEndpoints = %v, want %v", got, want)
	}
	if cfg.TiKVAPIVersion != "v2" {
		t.Fatalf("TiKVAPIVersion = %q", cfg.TiKVAPIVersion)
	}
	if cfg.TiKVKeyspace != "namros-prod" {
		t.Fatalf("TiKVKeyspace = %q", cfg.TiKVKeyspace)
	}
	if cfg.TiKVTimeout.String() != "5s" {
		t.Fatalf("TiKVTimeout = %s", cfg.TiKVTimeout)
	}
	if cfg.TiKVTLSCA != "/certs/ca.crt" || cfg.TiKVTLSCert != "/certs/client.crt" || cfg.TiKVTLSKey != "/certs/client.key" {
		t.Fatalf("TiKV TLS = ca:%q cert:%q key:%q", cfg.TiKVTLSCA, cfg.TiKVTLSCert, cfg.TiKVTLSKey)
	}
	if cfg.TiKVRetryAttempts != 5 || cfg.TiKVRetryInitialBackoff.String() != "20ms" || cfg.TiKVRetryMaxBackoff.String() != "250ms" {
		t.Fatalf("TiKV retry = attempts:%d initial:%s max:%s", cfg.TiKVRetryAttempts, cfg.TiKVRetryInitialBackoff, cfg.TiKVRetryMaxBackoff)
	}
	if cfg.MetadataCacheTTL.String() != "2s" {
		t.Fatalf("MetadataCacheTTL = %s", cfg.MetadataCacheTTL)
	}
	if cfg.StorageBackend != "sbs" {
		t.Fatalf("StorageBackend = %q", cfg.StorageBackend)
	}
	if cfg.StoragePath != "/tmp/namros-sbs" {
		t.Fatalf("StoragePath = %q", cfg.StoragePath)
	}
	if cfg.CoordinationBackend != CoordinationBackendEtcd {
		t.Fatalf("CoordinationBackend = %q", cfg.CoordinationBackend)
	}
	if got, want := cfg.EtcdEndpoints, []string{"127.0.0.1:2379", "127.0.0.2:2379"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("EtcdEndpoints = %v, want %v", got, want)
	}
	if cfg.EtcdDialTimeout.String() != "4s" {
		t.Fatalf("EtcdDialTimeout = %s", cfg.EtcdDialTimeout)
	}
	if cfg.GatewayInstanceID != "gw-a" || cfg.GatewayAdvertiseEndpoint != "10.0.0.10:9000" || cfg.GatewayRegistryPrefix != "/namros/prod/gateways" {
		t.Fatalf("Gateway coordination = id:%q endpoint:%q prefix:%q", cfg.GatewayInstanceID, cfg.GatewayAdvertiseEndpoint, cfg.GatewayRegistryPrefix)
	}
	if cfg.GatewayLeaseTTL.String() != "15s" || cfg.GatewayHeartbeat.String() != "5s" {
		t.Fatalf("Gateway lease/heartbeat = %s/%s", cfg.GatewayLeaseTTL, cfg.GatewayHeartbeat)
	}
	if cfg.GatewayDataBudgetBytes != 1048576 || cfg.GatewayDataBudgetMaxRequests != 7 || cfg.GatewayDataBudgetUnknownBytes != 65536 {
		t.Fatalf("Gateway data budget = bytes:%d requests:%d unknown:%d", cfg.GatewayDataBudgetBytes, cfg.GatewayDataBudgetMaxRequests, cfg.GatewayDataBudgetUnknownBytes)
	}
	if cfg.GatewayRequestMaxConcurrent != 40 || cfg.GatewayRequestMaxConcurrentPerTenant != 12 || cfg.GatewayRequestMaxConcurrentReads != 30 || cfg.GatewayRequestMaxConcurrentWrites != 10 {
		t.Fatalf("Gateway request limiter = global:%d tenant:%d reads:%d writes:%d", cfg.GatewayRequestMaxConcurrent, cfg.GatewayRequestMaxConcurrentPerTenant, cfg.GatewayRequestMaxConcurrentReads, cfg.GatewayRequestMaxConcurrentWrites)
	}
	if cfg.GatewayUploadBandwidthBytesPerSecond != 10485760 || cfg.GatewayDownloadBandwidthBytesPerSecond != 20971520 {
		t.Fatalf("Gateway bandwidth = upload:%d download:%d", cfg.GatewayUploadBandwidthBytesPerSecond, cfg.GatewayDownloadBandwidthBytesPerSecond)
	}
	if cfg.BackgroundWorkerMaxConcurrent != 4 || cfg.BackgroundWorkerMaxConcurrentPerTenant != 2 || cfg.BackgroundWorkerMaxConcurrentPerPool != 3 {
		t.Fatalf("Background worker budget = global:%d tenant:%d pool:%d", cfg.BackgroundWorkerMaxConcurrent, cfg.BackgroundWorkerMaxConcurrentPerTenant, cfg.BackgroundWorkerMaxConcurrentPerPool)
	}
	if !cfg.DedupeSchedulerEnabled || cfg.DedupeSchedulerTenantID != "tenant-1" || cfg.DedupeSchedulerBucketID != "bucket-1" || cfg.DedupeSchedulerPrefix != "backups/" {
		t.Fatalf("Dedupe scheduler scope = enabled:%v tenant:%q bucket:%q prefix:%q", cfg.DedupeSchedulerEnabled, cfg.DedupeSchedulerTenantID, cfg.DedupeSchedulerBucketID, cfg.DedupeSchedulerPrefix)
	}
	if cfg.DedupeSchedulerMode != "ingest_assisted" || cfg.DedupeSchedulerInterval.String() != "30s" || cfg.DedupeSchedulerLockTTL.String() != "2m0s" {
		t.Fatalf("Dedupe scheduler mode/timing = %q %s/%s", cfg.DedupeSchedulerMode, cfg.DedupeSchedulerInterval, cfg.DedupeSchedulerLockTTL)
	}
	if cfg.DedupeSchedulerMaxKeys != 200 || cfg.DedupeSchedulerLimit != 25 || cfg.DedupeSchedulerVerifyBytes {
		t.Fatalf("Dedupe scheduler scan = max:%d limit:%d verify:%v", cfg.DedupeSchedulerMaxKeys, cfg.DedupeSchedulerLimit, cfg.DedupeSchedulerVerifyBytes)
	}
	if !cfg.GCWorkerEnabled || cfg.GCWorkerShardID != "orphans-a" || cfg.GCWorkerInterval.String() != "45s" || cfg.GCWorkerLeaseTTL.String() != "3m0s" || cfg.GCWorkerLimit != 33 {
		t.Fatalf("GC worker = enabled:%v shard:%q interval:%s lease:%s limit:%d", cfg.GCWorkerEnabled, cfg.GCWorkerShardID, cfg.GCWorkerInterval, cfg.GCWorkerLeaseTTL, cfg.GCWorkerLimit)
	}
	if !cfg.LifecycleWorkerEnabled || cfg.LifecycleWorkerShardID != "bucket-shard-a" || cfg.LifecycleWorkerBucketID != "bucket-1" || cfg.LifecycleWorkerInterval.String() != "55s" || cfg.LifecycleWorkerLeaseTTL.String() != "4m0s" {
		t.Fatalf("Lifecycle worker = enabled:%v shard:%q bucket:%q interval:%s lease:%s", cfg.LifecycleWorkerEnabled, cfg.LifecycleWorkerShardID, cfg.LifecycleWorkerBucketID, cfg.LifecycleWorkerInterval, cfg.LifecycleWorkerLeaseTTL)
	}
	if cfg.LifecycleWorkerMaxKeys != 44 || cfg.LifecycleWorkerMaxUploads != 45 {
		t.Fatalf("Lifecycle worker limits = max_keys:%d max_uploads:%d", cfg.LifecycleWorkerMaxKeys, cfg.LifecycleWorkerMaxUploads)
	}
	if cfg.GCCandidateQueue != GCCandidateQueueMetadata {
		t.Fatalf("GCCandidateQueue = %q, want metadata", cfg.GCCandidateQueue)
	}
	if cfg.AccessAuditMode != AccessAuditModeAsync || cfg.AccessAuditBatchSize != 64 || cfg.AccessAuditQueueSize != 2048 || cfg.AccessAuditFlushInterval != 25*time.Millisecond {
		t.Fatalf("Access audit = mode:%q batch:%d queue:%d flush:%s", cfg.AccessAuditMode, cfg.AccessAuditBatchSize, cfg.AccessAuditQueueSize, cfg.AccessAuditFlushInterval)
	}
	if cfg.NAMRBDSBSObservabilityEndpoint != "http://namrbd-observability:19110" || cfg.NAMRBDSBSObservabilityTimeout != 750*time.Millisecond {
		t.Fatalf("NAMRBD SBS observability = endpoint:%q timeout:%s", cfg.NAMRBDSBSObservabilityEndpoint, cfg.NAMRBDSBSObservabilityTimeout)
	}
	if cfg.RootAccessKeyID != "root" {
		t.Fatalf("RootAccessKeyID = %q", cfg.RootAccessKeyID)
	}
	if cfg.RootSecretAccessKey != "secret" {
		t.Fatalf("RootSecretAccessKey = %q", cfg.RootSecretAccessKey)
	}
}

func TestParseEnvironmentOverrides(t *testing.T) {
	clearNAMROSEnv(t)
	t.Setenv("NAMROS_LISTEN", "0.0.0.0:19000")
	t.Setenv("NAMROS_DEPLOYMENT_PROFILE", "dev")
	t.Setenv("NAMROS_ALLOW_UNSAFE_PRODUCTION_SHORTCUTS", "true")
	t.Setenv("NAMROS_REGION", "ap-northeast-2")
	t.Setenv("NAMROS_METADATA_BACKEND", "pebble")
	t.Setenv("NAMROS_METADATA_PATH", "/var/lib/namros/meta")
	t.Setenv("NAMROS_STORAGE_BACKEND", "local")
	t.Setenv("NAMROS_STORAGE_PATH", "/var/lib/namros/segments")
	t.Setenv("NAMROS_SBS_SERVICE_ENDPOINT", "sbs-service:9443")
	t.Setenv("NAMROS_COORDINATION_BACKEND", "etcd")
	t.Setenv("NAMROS_ETCD_ENDPOINTS", "etcd-a:2379,etcd-b:2379")
	t.Setenv("NAMROS_GATEWAY_INSTANCE_ID", "gateway-1")
	t.Setenv("NAMROS_GATEWAY_ADVERTISE_ENDPOINT", "gateway-1:9000")
	t.Setenv("NAMROS_GATEWAY_DATA_BUDGET_BYTES", "2097152")
	t.Setenv("NAMROS_GATEWAY_DATA_BUDGET_MAX_REQUESTS", "11")
	t.Setenv("NAMROS_GATEWAY_DATA_BUDGET_UNKNOWN_BYTES", "131072")
	t.Setenv("NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT", "41")
	t.Setenv("NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT_PER_TENANT", "13")
	t.Setenv("NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT_READS", "31")
	t.Setenv("NAMROS_GATEWAY_REQUEST_MAX_CONCURRENT_WRITES", "9")
	t.Setenv("NAMROS_GATEWAY_UPLOAD_BANDWIDTH_BYTES_PER_SECOND", "10485761")
	t.Setenv("NAMROS_GATEWAY_DOWNLOAD_BANDWIDTH_BYTES_PER_SECOND", "20971521")
	t.Setenv("NAMROS_BACKGROUND_WORKER_MAX_CONCURRENT", "5")
	t.Setenv("NAMROS_BACKGROUND_WORKER_MAX_CONCURRENT_PER_TENANT", "2")
	t.Setenv("NAMROS_BACKGROUND_WORKER_MAX_CONCURRENT_PER_POOL", "3")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_ENABLED", "true")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_SHARD_ID", "env-bucket-shard")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_BUCKET_ID", "env-bucket")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_INTERVAL", "65s")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_LEASE_TTL", "5m")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_MAX_KEYS", "66")
	t.Setenv("NAMROS_LIFECYCLE_WORKER_MAX_UPLOADS", "67")
	t.Setenv("NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT", "http://namrbd-observability:19110")
	t.Setenv("NAMROS_NAMRBD_SBS_OBSERVABILITY_TIMEOUT", "900ms")
	t.Setenv("NAMROS_ROOT_ACCESS_KEY_ID", "env-root")
	t.Setenv("NAMROS_ROOT_SECRET_ACCESS_KEY", "env-secret")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(env) error = %v", err)
	}
	if cfg.ListenAddr != "0.0.0.0:19000" || cfg.Region != "ap-northeast-2" {
		t.Fatalf("listen/region = %q/%q", cfg.ListenAddr, cfg.Region)
	}
	if cfg.DeploymentProfile != DeploymentProfileDev {
		t.Fatalf("DeploymentProfile = %q", cfg.DeploymentProfile)
	}
	if !cfg.AllowUnsafeProductionShortcuts {
		t.Fatal("AllowUnsafeProductionShortcuts = false, want true")
	}
	if cfg.MetadataBackend != MetadataBackendPebble || cfg.MetadataPath != "/var/lib/namros/meta" {
		t.Fatalf("metadata = %q path=%q", cfg.MetadataBackend, cfg.MetadataPath)
	}
	if cfg.StorageBackend != StorageBackendLocal || cfg.StoragePath != "/var/lib/namros/segments" {
		t.Fatalf("storage = %q path=%q", cfg.StorageBackend, cfg.StoragePath)
	}
	if cfg.SBSAdminEndpoint != "sbs-service:9443" {
		t.Fatalf("SBSAdminEndpoint = %q", cfg.SBSAdminEndpoint)
	}
	if cfg.CoordinationBackend != CoordinationBackendEtcd || len(cfg.EtcdEndpoints) != 2 || cfg.EtcdEndpoints[0] != "etcd-a:2379" || cfg.EtcdEndpoints[1] != "etcd-b:2379" {
		t.Fatalf("coordination = %q endpoints=%v", cfg.CoordinationBackend, cfg.EtcdEndpoints)
	}
	if cfg.GatewayInstanceID != "gateway-1" || cfg.GatewayAdvertiseEndpoint != "gateway-1:9000" {
		t.Fatalf("gateway identity = %q endpoint=%q", cfg.GatewayInstanceID, cfg.GatewayAdvertiseEndpoint)
	}
	if cfg.GatewayDataBudgetBytes != 2097152 || cfg.GatewayDataBudgetMaxRequests != 11 || cfg.GatewayDataBudgetUnknownBytes != 131072 {
		t.Fatalf("gateway data budget = bytes:%d requests:%d unknown:%d", cfg.GatewayDataBudgetBytes, cfg.GatewayDataBudgetMaxRequests, cfg.GatewayDataBudgetUnknownBytes)
	}
	if cfg.GatewayRequestMaxConcurrent != 41 || cfg.GatewayRequestMaxConcurrentPerTenant != 13 || cfg.GatewayRequestMaxConcurrentReads != 31 || cfg.GatewayRequestMaxConcurrentWrites != 9 {
		t.Fatalf("gateway request limiter = global:%d tenant:%d reads:%d writes:%d", cfg.GatewayRequestMaxConcurrent, cfg.GatewayRequestMaxConcurrentPerTenant, cfg.GatewayRequestMaxConcurrentReads, cfg.GatewayRequestMaxConcurrentWrites)
	}
	if cfg.GatewayUploadBandwidthBytesPerSecond != 10485761 || cfg.GatewayDownloadBandwidthBytesPerSecond != 20971521 {
		t.Fatalf("gateway bandwidth = upload:%d download:%d", cfg.GatewayUploadBandwidthBytesPerSecond, cfg.GatewayDownloadBandwidthBytesPerSecond)
	}
	if cfg.BackgroundWorkerMaxConcurrent != 5 || cfg.BackgroundWorkerMaxConcurrentPerTenant != 2 || cfg.BackgroundWorkerMaxConcurrentPerPool != 3 {
		t.Fatalf("background worker budget = global:%d tenant:%d pool:%d", cfg.BackgroundWorkerMaxConcurrent, cfg.BackgroundWorkerMaxConcurrentPerTenant, cfg.BackgroundWorkerMaxConcurrentPerPool)
	}
	if !cfg.LifecycleWorkerEnabled || cfg.LifecycleWorkerShardID != "env-bucket-shard" || cfg.LifecycleWorkerBucketID != "env-bucket" {
		t.Fatalf("lifecycle worker scope = enabled:%v shard:%q bucket:%q", cfg.LifecycleWorkerEnabled, cfg.LifecycleWorkerShardID, cfg.LifecycleWorkerBucketID)
	}
	if cfg.LifecycleWorkerInterval != 65*time.Second || cfg.LifecycleWorkerLeaseTTL != 5*time.Minute || cfg.LifecycleWorkerMaxKeys != 66 || cfg.LifecycleWorkerMaxUploads != 67 {
		t.Fatalf("lifecycle worker timing/limits = interval:%s lease:%s max_keys:%d max_uploads:%d", cfg.LifecycleWorkerInterval, cfg.LifecycleWorkerLeaseTTL, cfg.LifecycleWorkerMaxKeys, cfg.LifecycleWorkerMaxUploads)
	}
	if cfg.NAMRBDSBSObservabilityEndpoint != "http://namrbd-observability:19110" || cfg.NAMRBDSBSObservabilityTimeout != 900*time.Millisecond {
		t.Fatalf("NAMRBD SBS observability = endpoint:%q timeout:%s", cfg.NAMRBDSBSObservabilityEndpoint, cfg.NAMRBDSBSObservabilityTimeout)
	}
	if cfg.RootAccessKeyID != "env-root" || cfg.RootSecretAccessKey != "env-secret" {
		t.Fatalf("root credentials = %q/%q", cfg.RootAccessKeyID, cfg.RootSecretAccessKey)
	}
}

func TestParseDeploymentProfileFlagOverridesEnvironment(t *testing.T) {
	clearNAMROSEnv(t)
	t.Setenv("NAMROS_DEPLOYMENT_PROFILE", "production")
	t.Setenv("NAMROS_ALLOW_UNSAFE_PRODUCTION_SHORTCUTS", "true")

	cfg, err := Parse([]string{"-deployment-profile", "dev", "-allow-unsafe-production-shortcuts=false"})
	if err != nil {
		t.Fatalf("Parse(deployment profile override) error = %v", err)
	}
	if cfg.DeploymentProfile != DeploymentProfileDev {
		t.Fatalf("DeploymentProfile = %q, want dev", cfg.DeploymentProfile)
	}
	if cfg.AllowUnsafeProductionShortcuts {
		t.Fatal("AllowUnsafeProductionShortcuts = true, want flag override false")
	}
}

func TestValidateRejectsNegativeNAMRBDSBSObservabilityTimeout(t *testing.T) {
	cfg := Default()
	cfg.NAMRBDSBSObservabilityTimeout = -time.Second
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want negative NAMRBD SBS observability timeout error")
	}
}

func TestValidateRejectsNegativeBackgroundWorkerBudgets(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name: "global",
			mutate: func(cfg *Config) {
				cfg.BackgroundWorkerMaxConcurrent = -1
			},
			want: "background worker max concurrent cannot be negative",
		},
		{
			name: "tenant",
			mutate: func(cfg *Config) {
				cfg.BackgroundWorkerMaxConcurrentPerTenant = -1
			},
			want: "background worker max concurrent per tenant cannot be negative",
		},
		{
			name: "pool",
			mutate: func(cfg *Config) {
				cfg.BackgroundWorkerMaxConcurrentPerPool = -1
			},
			want: "background worker max concurrent per pool cannot be negative",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestParseSecretFiles(t *testing.T) {
	clearNAMROSEnv(t)
	rootAccessKeyPath := writeConfigSecretFile(t, "file-root\n")
	rootSecretPath := writeConfigSecretFile(t, "file-secret\n")
	consolePasswordPath := writeConfigSecretFile(t, "console-pass\n")
	consoleSessionPath := writeConfigSecretFile(t, "0123456789abcdef\n")
	t.Setenv("NAMROS_ROOT_ACCESS_KEY_ID_FILE", rootAccessKeyPath)
	t.Setenv("NAMROS_ROOT_SECRET_ACCESS_KEY_FILE", rootSecretPath)
	t.Setenv("NAMROS_CONSOLE_AUTH_MODE", "local")
	t.Setenv("NAMROS_CONSOLE_ADMIN_PASSWORD_FILE", consolePasswordPath)
	t.Setenv("NAMROS_CONSOLE_SESSION_SECRET_FILE", consoleSessionPath)

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(secret files) error = %v", err)
	}
	if cfg.RootAccessKeyID != "file-root" || cfg.RootSecretAccessKey != "file-secret" {
		t.Fatalf("root credentials = %q/%q", cfg.RootAccessKeyID, cfg.RootSecretAccessKey)
	}
	if cfg.ConsoleAdminPassword != "console-pass" || cfg.ConsoleSessionSecret != "0123456789abcdef" {
		t.Fatalf("console secrets = %q/%q", cfg.ConsoleAdminPassword, cfg.ConsoleSessionSecret)
	}
}

func TestParseSecretEnvFileConflict(t *testing.T) {
	clearNAMROSEnv(t)
	t.Setenv("NAMROS_ROOT_SECRET_ACCESS_KEY", "env-secret")
	t.Setenv("NAMROS_ROOT_SECRET_ACCESS_KEY_FILE", writeConfigSecretFile(t, "file-secret\n"))

	_, err := Parse(nil)
	if err == nil || !strings.Contains(err.Error(), "NAMROS_ROOT_SECRET_ACCESS_KEY") || !strings.Contains(err.Error(), "NAMROS_ROOT_SECRET_ACCESS_KEY_FILE") {
		t.Fatalf("Parse(secret conflict) error = %v, want conflict", err)
	}
}

func TestParseCLIOverridesSecretFileAndEnvConflict(t *testing.T) {
	clearNAMROSEnv(t)
	t.Setenv("NAMROS_ROOT_SECRET_ACCESS_KEY", "env-secret")
	t.Setenv("NAMROS_ROOT_SECRET_ACCESS_KEY_FILE", writeConfigSecretFile(t, "file-secret\n"))

	cfg, err := Parse([]string{"-root-secret-access-key", "cli-secret"})
	if err != nil {
		t.Fatalf("Parse(cli overrides secret conflict) error = %v", err)
	}
	if cfg.RootSecretAccessKey != "cli-secret" {
		t.Fatalf("RootSecretAccessKey = %q, want cli-secret", cfg.RootSecretAccessKey)
	}
}

func writeConfigSecretFile(t *testing.T, value string) string {
	t.Helper()
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func TestValidateDeploymentProfileOptions(t *testing.T) {
	t.Run("rejects unsupported profile", func(t *testing.T) {
		cfg := Default()
		cfg.DeploymentProfile = "staging"
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported deployment profile") {
			t.Fatalf("Validate() error = %v, want unsupported deployment profile", err)
		}
	})

	t.Run("allows production with static volume pool", func(t *testing.T) {
		cfg := productionProfileConfig()
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("allows production with registry volume pool", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.SBSVolumePool = nil
		cfg.SBSVolumePoolID = "object-pool"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("rejects default dev shortcuts", func(t *testing.T) {
		cfg := Default()
		cfg.DeploymentProfile = DeploymentProfileProduction
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "tikv metadata backend") {
			t.Fatalf("Validate() error = %v, want tikv metadata requirement", err)
		}
	})

	t.Run("unsafe override allows explicit lab shortcut", func(t *testing.T) {
		cfg := Default()
		cfg.DeploymentProfile = DeploymentProfileProduction
		cfg.AllowUnsafeProductionShortcuts = true
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})

	t.Run("rejects pebble metadata", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.MetadataBackend = MetadataBackendPebble
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "tikv metadata backend") {
			t.Fatalf("Validate() error = %v, want tikv metadata requirement", err)
		}
	})

	t.Run("rejects missing etcd coordination", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.CoordinationBackend = CoordinationBackendNone
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "etcd coordination backend") {
			t.Fatalf("Validate() error = %v, want etcd coordination requirement", err)
		}
	})

	t.Run("rejects storage local gc candidate queue", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.GCCandidateQueue = GCCandidateQueueStorage
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "metadata gc candidate queue") {
			t.Fatalf("Validate() error = %v, want metadata gc candidate queue requirement", err)
		}
	})

	t.Run("rejects memory storage", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.StorageBackend = StorageBackendMemory
		cfg.SBSVolumePool = nil
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "SBS volume-pool storage backend") {
			t.Fatalf("Validate() error = %v, want SBS storage requirement", err)
		}
	})

	t.Run("rejects direct single volume", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.SBSVolumePool = nil
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sbs volume pool id") {
			t.Fatalf("Validate() error = %v, want volume pool requirement", err)
		}
	})

	t.Run("rejects one static pool member", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.SBSVolumePool = cfg.SBSVolumePool[:1]
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "at least two static sbs volume pool members") {
			t.Fatalf("Validate() error = %v, want two member pool requirement", err)
		}
	})

	t.Run("rejects production lab bridge without session fencing", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.SBSWriterGroupID = ""
		cfg.SBSSessionID = ""
		cfg.SBSVolumeEpoch = 0
		for idx := range cfg.SBSVolumePool {
			cfg.SBSVolumePool[idx].WriterGroupID = ""
			cfg.SBSVolumePool[idx].VolumeEpoch = 0
		}
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "shared attachment lab bridge is non-production") {
			t.Fatalf("Validate() error = %v, want lab bridge rejection", err)
		}
	})

	t.Run("allows production with per-member fencing", func(t *testing.T) {
		cfg := productionProfileConfig()
		cfg.SBSWriterGroupID = ""
		cfg.SBSVolumeEpoch = 0
		cfg.SBSVolumePool[0].WriterGroupID = "object-writers-a"
		cfg.SBSVolumePool[0].VolumeEpoch = 11
		cfg.SBSVolumePool[1].WriterGroupID = "object-writers-a"
		cfg.SBSVolumePool[1].VolumeEpoch = 12
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestParseProductionProfile(t *testing.T) {
	clearNAMROSEnv(t)
	cfg, err := Parse([]string{
		"-deployment-profile", "production",
		"-metadata-backend", "tikv",
		"-coordination-backend", "etcd",
		"-storage-backend", "sbs-physical",
		"-sbs-service-endpoint", "127.0.0.1:19091",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-writer-group-id", "object-writers",
		"-sbs-session-id", "gw-prod-boot-1",
		"-sbs-volume-epoch", "1",
		"-sbs-volume-pool", "volume_id=18a00001;volume_id=18a00002",
		"-gc-candidate-queue", "metadata",
	})
	if err != nil {
		t.Fatalf("Parse(production profile) error = %v", err)
	}
	if cfg.DeploymentProfile != DeploymentProfileProduction {
		t.Fatalf("DeploymentProfile = %q, want production", cfg.DeploymentProfile)
	}
}

func productionProfileConfig() Config {
	cfg := Default()
	cfg.DeploymentProfile = DeploymentProfileProduction
	cfg.MetadataBackend = MetadataBackendTiKV
	cfg.CoordinationBackend = CoordinationBackendEtcd
	cfg.StorageBackend = StorageBackendSBSPhysical
	cfg.SBSAdminEndpoint = "127.0.0.1:19091"
	cfg.SBSDataEndpoint = "127.0.0.1:19092"
	cfg.SBSWriterGroupID = "object-writers"
	cfg.SBSSessionID = "gw-prod-boot-1"
	cfg.SBSVolumeEpoch = 1
	cfg.GCCandidateQueue = GCCandidateQueueMetadata
	cfg.SBSVolumePool = []SBSVolumePoolMember{
		{VolumeID: "18a00001"},
		{VolumeID: "18a00002"},
	}
	return cfg
}

func TestParseSBSPhysicalOptions(t *testing.T) {
	clearNAMROSEnv(t)
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-physical",
		"-sbs-service-endpoint", "127.0.0.1:19091",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-volume-id", "0a0b0002",
		"-sbs-chunk-size-bytes", "1048576",
		"-sbs-gateway-id", "gw-a",
		"-sbs-attachment-id", "att-0a0b0002-physical",
		"-sbs-generation", "7",
		"-sbs-verify-readback",
		"-sbs-physical-write-concurrency", "4",
		"-sbs-physical-full-chunk-write-min-bytes", "131072",
		"-sbs-physical-full-chunk-write-max-bytes", "1048576",
		"-sbs-physical-chunk-cache-bytes", "33554432",
		"-sbs-chunk-id-allocation-cache-size", "64",
	})
	if err != nil {
		t.Fatalf("Parse(sbs-physical) error = %v", err)
	}
	if cfg.StorageBackend != StorageBackendSBSPhysical {
		t.Fatalf("StorageBackend = %q, want %q", cfg.StorageBackend, StorageBackendSBSPhysical)
	}
	if cfg.SBSAdminEndpoint != "127.0.0.1:19091" || cfg.SBSDataEndpoint != "127.0.0.1:19092" {
		t.Fatalf("SBS endpoints = admin:%q data:%q", cfg.SBSAdminEndpoint, cfg.SBSDataEndpoint)
	}
	if cfg.SBSVolumeID != "0a0b0002" || cfg.SBSChunkSizeBytes != 1048576 {
		t.Fatalf("SBS volume/chunk = %q/%d", cfg.SBSVolumeID, cfg.SBSChunkSizeBytes)
	}
	if cfg.SBSGatewayID != "gw-a" || cfg.SBSAttachmentID != "att-0a0b0002-physical" || cfg.SBSGeneration != 7 {
		t.Fatalf("SBS writer context = gateway:%q attachment:%q generation:%d", cfg.SBSGatewayID, cfg.SBSAttachmentID, cfg.SBSGeneration)
	}
	if !cfg.SBSVerifyReadback {
		t.Fatal("SBSVerifyReadback = false, want true")
	}
	if cfg.SBSPhysicalWriteConcurrency != 4 {
		t.Fatalf("SBSPhysicalWriteConcurrency = %d, want 4", cfg.SBSPhysicalWriteConcurrency)
	}
	if cfg.SBSPhysicalFullChunkWriteMinBytes != 131072 || cfg.SBSPhysicalFullChunkWriteMaxBytes != 1048576 || cfg.SBSPhysicalChunkCacheBytes != 33554432 {
		t.Fatalf("SBS physical optimization = min:%d max:%d cache:%d", cfg.SBSPhysicalFullChunkWriteMinBytes, cfg.SBSPhysicalFullChunkWriteMaxBytes, cfg.SBSPhysicalChunkCacheBytes)
	}
	if cfg.SBSChunkIDAllocationCacheSize != 64 {
		t.Fatalf("SBSChunkIDAllocationCacheSize = %d, want 64", cfg.SBSChunkIDAllocationCacheSize)
	}
}

func TestParseSBSECOptions(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-ec",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-volume-id", "0a0b0003",
		"-sbs-gateway-id", "gw-a",
		"-sbs-attachment-id", "att-0a0b0003-ec",
		"-sbs-generation", "11",
		"-sbs-shard-store-ids", "sbs-a,sbs-b,sbs-c",
		"-sbs-ec-shard-concurrency", "6",
	})
	if err != nil {
		t.Fatalf("Parse(sbs-ec) error = %v", err)
	}
	if cfg.StorageBackend != StorageBackendSBSEC {
		t.Fatalf("StorageBackend = %q, want %q", cfg.StorageBackend, StorageBackendSBSEC)
	}
	if cfg.SBSDataEndpoint != "127.0.0.1:19092" || cfg.SBSVolumeID != "0a0b0003" {
		t.Fatalf("SBS endpoint/volume = %q/%q", cfg.SBSDataEndpoint, cfg.SBSVolumeID)
	}
	if cfg.SBSGatewayID != "gw-a" || cfg.SBSAttachmentID != "att-0a0b0003-ec" || cfg.SBSGeneration != 11 {
		t.Fatalf("SBS writer context = gateway:%q attachment:%q generation:%d", cfg.SBSGatewayID, cfg.SBSAttachmentID, cfg.SBSGeneration)
	}
	if got, want := cfg.SBSShardStoreIDs, []string{"sbs-a", "sbs-b", "sbs-c"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("SBSShardStoreIDs = %v, want %v", got, want)
	}
	if cfg.SBSECShardConcurrency != 6 {
		t.Fatalf("SBSECShardConcurrency = %d, want 6", cfg.SBSECShardConcurrency)
	}
}

func TestParseSBSClusterOptions(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-cluster",
		"-sbs-service-endpoint", "127.0.0.1:19091",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-volume-id", "0a0b0004",
		"-sbs-chunk-size-bytes", "1048576",
		"-sbs-gateway-id", "gw-a",
		"-sbs-attachment-id", "att-0a0b0004-cluster",
		"-sbs-generation", "13",
		"-sbs-shard-store-ids", "sbs-a,sbs-b",
		"-sbs-ec-shard-concurrency", "7",
		"-sbs-verify-readback",
		"-sbs-physical-write-concurrency", "3",
		"-sbs-physical-full-chunk-write-min-bytes", "65536",
		"-sbs-physical-full-chunk-write-max-bytes", "2097152",
		"-sbs-physical-chunk-cache-bytes", "16777216",
		"-sbs-chunk-id-allocation-cache-size", "128",
	})
	if err != nil {
		t.Fatalf("Parse(sbs-cluster) error = %v", err)
	}
	if cfg.StorageBackend != StorageBackendSBSCluster {
		t.Fatalf("StorageBackend = %q, want %q", cfg.StorageBackend, StorageBackendSBSCluster)
	}
	if cfg.SBSAdminEndpoint != "127.0.0.1:19091" || cfg.SBSDataEndpoint != "127.0.0.1:19092" {
		t.Fatalf("SBS endpoints = admin:%q data:%q", cfg.SBSAdminEndpoint, cfg.SBSDataEndpoint)
	}
	if cfg.SBSVolumeID != "0a0b0004" || cfg.SBSChunkSizeBytes != 1048576 {
		t.Fatalf("SBS volume/chunk = %q/%d", cfg.SBSVolumeID, cfg.SBSChunkSizeBytes)
	}
	if cfg.SBSGatewayID != "gw-a" || cfg.SBSAttachmentID != "att-0a0b0004-cluster" || cfg.SBSGeneration != 13 {
		t.Fatalf("SBS writer context = gateway:%q attachment:%q generation:%d", cfg.SBSGatewayID, cfg.SBSAttachmentID, cfg.SBSGeneration)
	}
	if got, want := cfg.SBSShardStoreIDs, []string{"sbs-a", "sbs-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("SBSShardStoreIDs = %v, want %v", got, want)
	}
	if cfg.SBSECShardConcurrency != 7 {
		t.Fatalf("SBSECShardConcurrency = %d, want 7", cfg.SBSECShardConcurrency)
	}
	if !cfg.SBSVerifyReadback {
		t.Fatal("SBSVerifyReadback = false, want true")
	}
	if cfg.SBSPhysicalWriteConcurrency != 3 {
		t.Fatalf("SBSPhysicalWriteConcurrency = %d, want 3", cfg.SBSPhysicalWriteConcurrency)
	}
	if cfg.SBSPhysicalFullChunkWriteMinBytes != 65536 || cfg.SBSPhysicalFullChunkWriteMaxBytes != 2097152 || cfg.SBSPhysicalChunkCacheBytes != 16777216 {
		t.Fatalf("SBS physical optimization = min:%d max:%d cache:%d", cfg.SBSPhysicalFullChunkWriteMinBytes, cfg.SBSPhysicalFullChunkWriteMaxBytes, cfg.SBSPhysicalChunkCacheBytes)
	}
	if cfg.SBSChunkIDAllocationCacheSize != 128 {
		t.Fatalf("SBSChunkIDAllocationCacheSize = %d, want 128", cfg.SBSChunkIDAllocationCacheSize)
	}
}

func TestParseSBSVolumePoolOptions(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-cluster",
		"-sbs-service-endpoint", "127.0.0.1:19091",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-gateway-id", "gw-a",
		"-sbs-generation", "17",
		"-sbs-writer-group-id", "object-writers-default",
		"-sbs-session-id", "gw-a-boot-1",
		"-sbs-volume-pool", "volume_id=18a00001,attachment_id=att-a,writer_group_id=object-writers-a,volume_epoch=8,shards=sbs-a|sbs-b,state=active,weight=2,write_concurrency=5,available_bytes=1048576,used_percent=25,high_watermark_percent=90;volume_id=18a00002,service_endpoint=127.0.0.2:19091,data_endpoint=127.0.0.2:19092,attachment_id=att-b,readonly=true,generation=19,state=draining",
	})
	if err != nil {
		t.Fatalf("Parse(sbs-volume-pool) error = %v", err)
	}
	if len(cfg.SBSVolumePool) != 2 {
		t.Fatalf("SBSVolumePool len = %d, want 2", len(cfg.SBSVolumePool))
	}
	if got := cfg.SBSVolumePool[0]; got.VolumeID != "18a00001" || got.AttachmentID != "att-a" || got.WriterGroupID != "object-writers-a" || got.VolumeEpoch != 8 || got.ReadOnly || got.State != "active" || got.Weight != 2 || got.WriteConcurrency != 5 || got.AvailableBytes != 1048576 || got.UsedPercent != 25 || got.HighWatermarkPercent != 90 {
		t.Fatalf("pool[0] = %+v", got)
	}
	if got := cfg.SBSVolumePool[0].ShardStoreIDs; len(got) != 2 || got[0] != "sbs-a" || got[1] != "sbs-b" {
		t.Fatalf("pool[0].ShardStoreIDs = %v", got)
	}
	if got := cfg.SBSVolumePool[1]; got.VolumeID != "18a00002" || got.AdminEndpoint != "127.0.0.2:19091" || got.DataEndpoint != "127.0.0.2:19092" || got.AttachmentID != "att-b" || !got.ReadOnly || got.Generation != 19 || got.State != "draining" {
		t.Fatalf("pool[1] = %+v", got)
	}
	if cfg.SBSGeneration != 17 {
		t.Fatalf("SBSGeneration = %d, want 17", cfg.SBSGeneration)
	}
}

func TestParseSBSSessionIdentityOptions(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-cluster",
		"-sbs-service-endpoint", "127.0.0.1:19091",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-writer-group-id", "object-writers",
		"-sbs-session-id", "gw-a-boot-1",
		"-sbs-volume-epoch", "7",
		"-sbs-session-ttl", "45s",
		"-sbs-session-heartbeat", "15s",
	})
	if err != nil {
		t.Fatalf("Parse(sbs session identity) error = %v", err)
	}
	if cfg.SBSWriterGroupID != "object-writers" || cfg.SBSSessionID != "gw-a-boot-1" || cfg.SBSVolumeEpoch != 7 {
		t.Fatalf("SBS session identity = writer:%q session:%q epoch:%d", cfg.SBSWriterGroupID, cfg.SBSSessionID, cfg.SBSVolumeEpoch)
	}
	if cfg.SBSSessionTTL != 45*time.Second || cfg.SBSSessionHeartbeat != 15*time.Second {
		t.Fatalf("SBS session timings = ttl:%s heartbeat:%s", cfg.SBSSessionTTL, cfg.SBSSessionHeartbeat)
	}
}

func TestParseSBSSessionIdentityFromEnv(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	t.Setenv("NAMROS_SBS_WRITER_GROUP_ID", "object-writers-env")
	t.Setenv("NAMROS_SBS_SESSION_ID", "gw-env-boot-1")
	t.Setenv("NAMROS_SBS_VOLUME_EPOCH", "9")
	t.Setenv("NAMROS_SBS_SESSION_TTL", "40s")
	t.Setenv("NAMROS_SBS_SESSION_HEARTBEAT", "10s")
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-cluster",
		"-sbs-service-endpoint", "127.0.0.1:19091",
		"-sbs-data-endpoint", "127.0.0.1:19092",
		"-sbs-session-id", "gw-cli-wins",
	})
	if err != nil {
		t.Fatalf("Parse(env sbs session identity) error = %v", err)
	}
	if cfg.SBSWriterGroupID != "object-writers-env" || cfg.SBSSessionID != "gw-cli-wins" || cfg.SBSVolumeEpoch != 9 {
		t.Fatalf("SBS session identity = writer:%q session:%q epoch:%d", cfg.SBSWriterGroupID, cfg.SBSSessionID, cfg.SBSVolumeEpoch)
	}
	if cfg.SBSSessionTTL != 40*time.Second || cfg.SBSSessionHeartbeat != 10*time.Second {
		t.Fatalf("SBS session timings = ttl:%s heartbeat:%s", cfg.SBSSessionTTL, cfg.SBSSessionHeartbeat)
	}
}

func TestValidateSBSSessionIdentityRejectsIncompleteConfig(t *testing.T) {
	cfg := Default()
	cfg.SBSWriterGroupID = "object-writers"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sbs session id is required") {
		t.Fatalf("Validate(incomplete session) error = %v, want missing session id", err)
	}
	cfg = Default()
	cfg.SBSSessionID = "gw-a-boot-1"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "sbs writer group id is required") {
		t.Fatalf("Validate(incomplete session) error = %v, want missing writer group id", err)
	}
	cfg = Default()
	cfg.SBSWriterGroupID = "object-writers"
	cfg.SBSSessionID = "gw-a-boot-1"
	cfg.SBSSessionHeartbeat = cfg.SBSSessionTTL
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "heartbeat must be shorter") {
		t.Fatalf("Validate(slow heartbeat) error = %v, want heartbeat validation", err)
	}
}

func TestParseSBSVolumePoolID(t *testing.T) {
	clearNAMROSEnv(t)
	withCurrentEdition(t, edition.Enterprise)
	cfg, err := Parse([]string{
		"-storage-backend", "sbs-cluster",
		"-sbs-volume-pool-id", "object-pool",
		"-sbs-volume-pool-refresh-interval", "250ms",
	})
	if err != nil {
		t.Fatalf("Parse(sbs-volume-pool-id) error = %v", err)
	}
	if cfg.SBSVolumePoolID != "object-pool" {
		t.Fatalf("SBSVolumePoolID = %q, want object-pool", cfg.SBSVolumePoolID)
	}
	if cfg.SBSVolumePoolRefreshInterval != 250*time.Millisecond {
		t.Fatalf("SBSVolumePoolRefreshInterval = %s, want 250ms", cfg.SBSVolumePoolRefreshInterval)
	}
}

func TestValidateSBSVolumePoolRejectsInvalidAdmission(t *testing.T) {
	base := Default()
	base.StorageBackend = StorageBackendSBSPhysical
	base.SBSAdminEndpoint = "127.0.0.1:19091"
	base.SBSDataEndpoint = "127.0.0.1:19092"
	for _, tc := range []struct {
		name   string
		member SBSVolumePoolMember
	}{
		{name: "state", member: SBSVolumePoolMember{VolumeID: "18a00001", State: "mystery"}},
		{name: "weight", member: SBSVolumePoolMember{VolumeID: "18a00001", Weight: 2048}},
		{name: "write_concurrency", member: SBSVolumePoolMember{VolumeID: "18a00001", WriteConcurrency: -1}},
		{name: "used_percent", member: SBSVolumePoolMember{VolumeID: "18a00001", UsedPercent: 101}},
		{name: "watermark", member: SBSVolumePoolMember{VolumeID: "18a00001", HighWatermarkPercent: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.SBSVolumePool = []SBSVolumePoolMember{tc.member}
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want admission validation error")
			}
		})
	}
}

func TestValidateRejectsStaticAndRegistryVolumePoolTogether(t *testing.T) {
	cfg := Default()
	cfg.StorageBackend = StorageBackendSBSPhysical
	cfg.SBSVolumePoolID = "object-pool"
	cfg.SBSVolumePool = []SBSVolumePoolMember{{VolumeID: "18a00001"}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want static/registry conflict")
	}
}

func TestValidateSBSVolumePoolRejectsDuplicateVolume(t *testing.T) {
	cfg := Default()
	cfg.StorageBackend = StorageBackendSBSPhysical
	cfg.SBSAdminEndpoint = "127.0.0.1:19091"
	cfg.SBSDataEndpoint = "127.0.0.1:19092"
	cfg.SBSVolumePool = []SBSVolumePoolMember{
		{VolumeID: "18a00001"},
		{VolumeID: "18a00001"},
	}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want duplicate volume error")
	}
}

func TestValidateRequiresFields(t *testing.T) {
	cfg := Default()
	cfg.ListenAddr = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateRejectsNegativeGatewayDataBudgetMaxRequests(t *testing.T) {
	cfg := Default()
	cfg.GatewayDataBudgetMaxRequests = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want gateway data budget max requests error")
	}
}

func TestValidateRejectsNegativeGatewayRequestLimits(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Config)
		want   string
	}{
		"global": {
			mutate: func(cfg *Config) { cfg.GatewayRequestMaxConcurrent = -1 },
			want:   "gateway request max concurrent cannot be negative",
		},
		"tenant": {
			mutate: func(cfg *Config) { cfg.GatewayRequestMaxConcurrentPerTenant = -1 },
			want:   "gateway request max concurrent per tenant cannot be negative",
		},
		"reads": {
			mutate: func(cfg *Config) { cfg.GatewayRequestMaxConcurrentReads = -1 },
			want:   "gateway request max concurrent reads cannot be negative",
		},
		"writes": {
			mutate: func(cfg *Config) { cfg.GatewayRequestMaxConcurrentWrites = -1 },
			want:   "gateway request max concurrent writes cannot be negative",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsNegativeGatewayBandwidthLimits(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate func(*Config)
		want   string
	}{
		"upload": {
			mutate: func(cfg *Config) { cfg.GatewayUploadBandwidthBytesPerSecond = -1 },
			want:   "gateway upload bandwidth bytes per second cannot be negative",
		},
		"download": {
			mutate: func(cfg *Config) { cfg.GatewayDownloadBandwidthBytesPerSecond = -1 },
			want:   "gateway download bandwidth bytes per second cannot be negative",
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(&cfg)
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateRejectsInvalidSBSPhysicalWriteConcurrency(t *testing.T) {
	cfg := Default()
	cfg.SBSPhysicalWriteConcurrency = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want write concurrency error")
	}
}

func TestValidateRejectsInvalidSBSECShardConcurrency(t *testing.T) {
	cfg := Default()
	cfg.SBSECShardConcurrency = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want ec shard concurrency error")
	}
}

func TestValidateRejectsInvalidSBSPhysicalFullChunkWriteWindow(t *testing.T) {
	cfg := Default()
	cfg.SBSPhysicalFullChunkWriteMinBytes = 2 << 20
	cfg.SBSPhysicalFullChunkWriteMaxBytes = 1 << 20
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want full chunk write window error")
	}
}

func TestValidateRejectsUnsupportedEdition(t *testing.T) {
	cfg := Default()
	cfg.Edition = "ultimate"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want unsupported edition")
	}
}

func TestValidateCommunityRejectsEnterpriseFeatures(t *testing.T) {
	skipEnterpriseOverlayCommunityAssertion(t)
	for _, tc := range []struct {
		name string
		cfg  func() Config
	}{
		{
			name: "sbs ec",
			cfg: func() Config {
				cfg := Default()
				cfg.StorageBackend = StorageBackendSBSEC
				cfg.SBSDataEndpoint = "127.0.0.1:19092"
				return cfg
			},
		},
		{
			name: "sbs cluster ec shard routing",
			cfg: func() Config {
				cfg := Default()
				cfg.StorageBackend = StorageBackendSBSCluster
				cfg.SBSAdminEndpoint = "127.0.0.1:19091"
				cfg.SBSDataEndpoint = "127.0.0.1:19092"
				cfg.SBSShardStoreIDs = []string{"sbs-a", "sbs-b"}
				return cfg
			},
		},
		{
			name: "dedupe scheduler",
			cfg: func() Config {
				cfg := Default()
				cfg.DedupeSchedulerEnabled = true
				cfg.DedupeSchedulerTenantID = "tenant-1"
				return cfg
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cfg().Validate(); err == nil || !strings.Contains(err.Error(), "NAMROS Enterprise Edition") {
				t.Fatalf("Validate() error = %v, want enterprise edition requirement", err)
			}
		})
	}
}

func TestValidateCommunityAllowsDistributedMetadataAndGatewayHA(t *testing.T) {
	t.Run("tikv metadata", func(t *testing.T) {
		cfg := Default()
		cfg.MetadataBackend = MetadataBackendTiKV
		cfg.TiKVPDEndpoints = []string{"127.0.0.1:2379"}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
	t.Run("etcd coordination", func(t *testing.T) {
		cfg := Default()
		cfg.CoordinationBackend = CoordinationBackendEtcd
		cfg.EtcdEndpoints = []string{"127.0.0.1:12379"}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
	t.Run("sbs cluster replicated object", func(t *testing.T) {
		cfg := Default()
		cfg.StorageBackend = StorageBackendSBSCluster
		cfg.SBSAdminEndpoint = "127.0.0.1:19091"
		cfg.SBSDataEndpoint = "127.0.0.1:19092"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("Validate() error = %v", err)
		}
	})
}

func TestValidateTiKVMetadataOptions(t *testing.T) {
	t.Run("requires pd endpoints", func(t *testing.T) {
		cfg := Default()
		cfg.MetadataBackend = MetadataBackendTiKV
		cfg.TiKVPDEndpoints = nil
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})
	t.Run("rejects unsupported api version", func(t *testing.T) {
		cfg := Default()
		cfg.MetadataBackend = MetadataBackendTiKV
		cfg.TiKVAPIVersion = "v1ttl"
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})
	t.Run("requires complete tls triplet", func(t *testing.T) {
		cfg := Default()
		cfg.MetadataBackend = MetadataBackendTiKV
		cfg.TiKVTLSCA = "/certs/ca.crt"
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})
	t.Run("rejects invalid retry policy", func(t *testing.T) {
		cfg := Default()
		cfg.MetadataBackend = MetadataBackendTiKV
		cfg.TiKVRetryInitialBackoff = 200 * time.Millisecond
		cfg.TiKVRetryMaxBackoff = 100 * time.Millisecond
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want error")
		}
	})
}

func TestValidateRequiresPebbleMetadataPath(t *testing.T) {
	cfg := Default()
	cfg.MetadataBackend = MetadataBackendPebble
	cfg.MetadataPath = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want error")
	}
}

func TestValidateSBSPhysicalRequiresEndpoints(t *testing.T) {
	cfg := Default()
	cfg.StorageBackend = StorageBackendSBSPhysical
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing admin endpoint error")
	}
	cfg.SBSAdminEndpoint = "127.0.0.1:19091"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing data endpoint error")
	}
	cfg.SBSDataEndpoint = "127.0.0.1:19092"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateSBSECRequiresDataEndpoint(t *testing.T) {
	cfg := Default()
	cfg.Edition = "enterprise"
	cfg.StorageBackend = StorageBackendSBSEC
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing data endpoint error")
	}
	cfg.SBSDataEndpoint = "127.0.0.1:19092"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateSBSClusterRequiresEndpoints(t *testing.T) {
	cfg := Default()
	cfg.Edition = "enterprise"
	cfg.StorageBackend = StorageBackendSBSCluster
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing admin endpoint error")
	}
	cfg.SBSAdminEndpoint = "127.0.0.1:19091"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want missing data endpoint error")
	}
	cfg.SBSDataEndpoint = "127.0.0.1:19092"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateEtcdCoordinationOptions(t *testing.T) {
	t.Run("requires endpoints", func(t *testing.T) {
		cfg := Default()
		cfg.CoordinationBackend = CoordinationBackendEtcd
		cfg.EtcdEndpoints = nil
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want missing etcd endpoints")
		}
	})
	t.Run("rejects unsupported backend", func(t *testing.T) {
		cfg := Default()
		cfg.CoordinationBackend = "consul"
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want unsupported coordination backend")
		}
	})
	t.Run("requires heartbeat shorter than lease", func(t *testing.T) {
		cfg := Default()
		cfg.CoordinationBackend = CoordinationBackendEtcd
		cfg.GatewayLeaseTTL = 5 * time.Second
		cfg.GatewayHeartbeat = 5 * time.Second
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid heartbeat")
		}
	})
	t.Run("requires registry prefix", func(t *testing.T) {
		cfg := Default()
		cfg.CoordinationBackend = CoordinationBackendEtcd
		cfg.GatewayRegistryPrefix = ""
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want missing registry prefix")
		}
	})
}

func TestValidateDedupeSchedulerOptions(t *testing.T) {
	t.Run("requires tenant when enabled", func(t *testing.T) {
		cfg := Default()
		cfg.Edition = "enterprise"
		cfg.DedupeSchedulerEnabled = true
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want missing tenant id")
		}
	})
	t.Run("rejects invalid timing", func(t *testing.T) {
		cfg := Default()
		cfg.Edition = "enterprise"
		cfg.DedupeSchedulerEnabled = true
		cfg.DedupeSchedulerTenantID = "tenant-1"
		cfg.DedupeSchedulerInterval = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid interval")
		}
		cfg.DedupeSchedulerInterval = time.Minute
		cfg.DedupeSchedulerLockTTL = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid lock ttl")
		}
	})
	t.Run("rejects negative scan limits", func(t *testing.T) {
		cfg := Default()
		cfg.Edition = "enterprise"
		cfg.DedupeSchedulerEnabled = true
		cfg.DedupeSchedulerTenantID = "tenant-1"
		cfg.DedupeSchedulerMaxKeys = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid max keys")
		}
		cfg.DedupeSchedulerMaxKeys = 100
		cfg.DedupeSchedulerLimit = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid limit")
		}
	})
}

func TestValidateGCWorkerOptions(t *testing.T) {
	t.Run("rejects missing shard", func(t *testing.T) {
		cfg := Default()
		cfg.GCWorkerEnabled = true
		cfg.GCWorkerShardID = ""
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want missing shard id")
		}
	})
	t.Run("rejects invalid timing", func(t *testing.T) {
		cfg := Default()
		cfg.GCWorkerEnabled = true
		cfg.GCWorkerInterval = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid interval")
		}
		cfg.GCWorkerInterval = time.Minute
		cfg.GCWorkerLeaseTTL = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid lease ttl")
		}
	})
	t.Run("rejects negative limit", func(t *testing.T) {
		cfg := Default()
		cfg.GCWorkerEnabled = true
		cfg.GCWorkerLimit = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid limit")
		}
	})
}

func TestValidateLifecycleWorkerOptions(t *testing.T) {
	t.Run("rejects missing shard", func(t *testing.T) {
		cfg := Default()
		cfg.LifecycleWorkerEnabled = true
		cfg.LifecycleWorkerBucketID = "bucket-1"
		cfg.LifecycleWorkerShardID = ""
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want missing shard id")
		}
	})
	t.Run("rejects missing bucket", func(t *testing.T) {
		cfg := Default()
		cfg.LifecycleWorkerEnabled = true
		cfg.LifecycleWorkerBucketID = ""
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want missing bucket id")
		}
	})
	t.Run("rejects invalid timing", func(t *testing.T) {
		cfg := Default()
		cfg.LifecycleWorkerEnabled = true
		cfg.LifecycleWorkerBucketID = "bucket-1"
		cfg.LifecycleWorkerInterval = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid interval")
		}
		cfg.LifecycleWorkerInterval = time.Minute
		cfg.LifecycleWorkerLeaseTTL = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid lease ttl")
		}
	})
	t.Run("rejects negative limits", func(t *testing.T) {
		cfg := Default()
		cfg.LifecycleWorkerEnabled = true
		cfg.LifecycleWorkerBucketID = "bucket-1"
		cfg.LifecycleWorkerMaxKeys = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid max keys")
		}
		cfg.LifecycleWorkerMaxKeys = 100
		cfg.LifecycleWorkerMaxUploads = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("Validate() error = nil, want invalid max uploads")
		}
	})
}

func TestValidateGCCandidateQueue(t *testing.T) {
	cfg := Default()
	cfg.GCCandidateQueue = "bogus"
	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want invalid gc candidate queue")
	}
}

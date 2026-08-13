package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/version"
)

type GatewayConfig struct {
	Backend           string
	EtcdEndpoints     []string
	EtcdDialTimeout   time.Duration
	InstanceID        string
	AdvertiseEndpoint string
	ListenAddr        string
	RegistryPrefix    string
	LeaseTTL          time.Duration
	HeartbeatInterval time.Duration
	MetadataBackend   string
	StorageBackend    string
	StartedAt         time.Time
}

type GatewayRecord struct {
	InstanceID        string            `json:"instance_id"`
	AdvertiseEndpoint string            `json:"advertise_endpoint"`
	ListenAddr        string            `json:"listen_addr"`
	Healthy           bool              `json:"healthy"`
	Ready             bool              `json:"ready"`
	Status            string            `json:"status"`
	MetadataBackend   string            `json:"metadata_backend"`
	StorageBackend    string            `json:"storage_backend"`
	StartedAtUnix     int64             `json:"started_at_unix"`
	LastHeartbeatUnix int64             `json:"last_heartbeat_unix"`
	Version           map[string]string `json:"version,omitempty"`
}

func GatewayConfigFromApp(cfg config.Config) GatewayConfig {
	instanceID := strings.TrimSpace(cfg.GatewayInstanceID)
	if instanceID == "" {
		instanceID = defaultGatewayInstanceID()
	}
	advertiseEndpoint := strings.TrimSpace(cfg.GatewayAdvertiseEndpoint)
	if advertiseEndpoint == "" {
		advertiseEndpoint = cfg.ListenAddr
	}
	return GatewayConfig{
		Backend:           config.NormalizeCoordinationBackend(cfg.CoordinationBackend),
		EtcdEndpoints:     append([]string(nil), cfg.EtcdEndpoints...),
		EtcdDialTimeout:   cfg.EtcdDialTimeout,
		InstanceID:        instanceID,
		AdvertiseEndpoint: advertiseEndpoint,
		ListenAddr:        cfg.ListenAddr,
		RegistryPrefix:    cfg.GatewayRegistryPrefix,
		LeaseTTL:          cfg.GatewayLeaseTTL,
		HeartbeatInterval: cfg.GatewayHeartbeat,
		MetadataBackend:   cfg.MetadataBackend,
		StorageBackend:    cfg.StorageBackend,
		StartedAt:         time.Now().UTC(),
	}
}

func Enabled(cfg config.Config) bool {
	return config.NormalizeCoordinationBackend(cfg.CoordinationBackend) != config.CoordinationBackendNone
}

func RunGatewayRegistry(ctx context.Context, cfg GatewayConfig, logger *slog.Logger) error {
	switch config.NormalizeCoordinationBackend(cfg.Backend) {
	case config.CoordinationBackendNone, "":
		return nil
	case config.CoordinationBackendEtcd:
		return runEtcdGatewayRegistry(ctx, cfg, logger)
	default:
		return fmt.Errorf("unsupported coordination backend %q", cfg.Backend)
	}
}

func GatewayRegistryKey(prefix, instanceID string) string {
	return GatewayRegistryPrefix(prefix) + "/" + sanitizeKeyPart(instanceID)
}

func GatewayRegistryPrefix(prefix string) string {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" {
		prefix = config.DefaultGatewayRegistryPrefix
	}
	return prefix
}

func BuildGatewayRecord(cfg GatewayConfig, now time.Time) GatewayRecord {
	startedAt := cfg.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	return GatewayRecord{
		InstanceID:        cfg.InstanceID,
		AdvertiseEndpoint: cfg.AdvertiseEndpoint,
		ListenAddr:        cfg.ListenAddr,
		Healthy:           true,
		Ready:             true,
		Status:            "ready",
		MetadataBackend:   cfg.MetadataBackend,
		StorageBackend:    cfg.StorageBackend,
		StartedAtUnix:     startedAt.Unix(),
		LastHeartbeatUnix: now.Unix(),
		Version:           version.Info(),
	}
}

func EncodeGatewayRecord(record GatewayRecord) (string, error) {
	payload, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func ListGatewayRecords(ctx context.Context, cfg GatewayConfig) ([]GatewayRecord, error) {
	switch config.NormalizeCoordinationBackend(cfg.Backend) {
	case config.CoordinationBackendNone, "":
		return nil, nil
	case config.CoordinationBackendEtcd:
		return listEtcdGatewayRecords(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported coordination backend %q", cfg.Backend)
	}
}

func HealthyGatewayRecords(records []GatewayRecord, now time.Time, maxHeartbeatAge time.Duration) []GatewayRecord {
	healthy := make([]GatewayRecord, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.InstanceID) == "" || strings.TrimSpace(record.AdvertiseEndpoint) == "" {
			continue
		}
		if !record.Healthy || !record.Ready || record.Status != "ready" {
			continue
		}
		if maxHeartbeatAge > 0 {
			if record.LastHeartbeatUnix <= 0 {
				continue
			}
			age := now.Sub(time.Unix(record.LastHeartbeatUnix, 0))
			if age > maxHeartbeatAge {
				continue
			}
		}
		healthy = append(healthy, record)
	}
	sortGatewayRecords(healthy)
	return healthy
}

func SelectFailoverGateway(records []GatewayRecord, failedInstanceID string, now time.Time, maxHeartbeatAge time.Duration) (GatewayRecord, bool) {
	failedInstanceID = strings.TrimSpace(failedInstanceID)
	for _, record := range HealthyGatewayRecords(records, now, maxHeartbeatAge) {
		if failedInstanceID != "" && record.InstanceID == failedInstanceID {
			continue
		}
		return record, true
	}
	return GatewayRecord{}, false
}

func GatewayMaxHeartbeatAge(cfg GatewayConfig) time.Duration {
	maxAge := cfg.LeaseTTL + cfg.HeartbeatInterval
	if maxAge <= 0 {
		return 0
	}
	return maxAge
}

func runEtcdGatewayRegistry(ctx context.Context, cfg GatewayConfig, logger *slog.Logger) error {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.EtcdEndpoints,
		DialTimeout: cfg.EtcdDialTimeout,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	ttlSeconds := leaseTTLSeconds(cfg.LeaseTTL)
	lease, err := client.Grant(ctx, ttlSeconds)
	if err != nil {
		return fmt.Errorf("grant gateway registry lease: %w", err)
	}
	key := GatewayRegistryKey(cfg.RegistryPrefix, cfg.InstanceID)
	if logger != nil {
		logger.Info("gateway coordination registry started", "backend", config.CoordinationBackendEtcd, "key", key, "ttl_seconds", ttlSeconds)
	}
	defer revokeLease(context.Background(), client, lease.ID)

	if err := putGatewayRecord(ctx, client, key, lease.ID, cfg); err != nil {
		return err
	}
	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if _, err := client.KeepAliveOnce(ctx, lease.ID); err != nil {
				return fmt.Errorf("keepalive gateway registry lease: %w", err)
			}
			if err := putGatewayRecord(ctx, client, key, lease.ID, cfg); err != nil {
				return err
			}
		}
	}
}

func listEtcdGatewayRecords(ctx context.Context, cfg GatewayConfig) ([]GatewayRecord, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.EtcdEndpoints,
		DialTimeout: cfg.EtcdDialTimeout,
	})
	if err != nil {
		return nil, err
	}
	defer client.Close()

	resp, err := client.Get(ctx, GatewayRegistryPrefix(cfg.RegistryPrefix)+"/", clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("list gateway registry records: %w", err)
	}
	records := make([]GatewayRecord, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		var record GatewayRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			return nil, fmt.Errorf("decode gateway registry record %q: %w", string(kv.Key), err)
		}
		if strings.TrimSpace(record.InstanceID) == "" {
			continue
		}
		records = append(records, record)
	}
	sortGatewayRecords(records)
	return records, nil
}

func sortGatewayRecords(records []GatewayRecord) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].InstanceID == records[j].InstanceID {
			return records[i].AdvertiseEndpoint < records[j].AdvertiseEndpoint
		}
		return records[i].InstanceID < records[j].InstanceID
	})
}

func putGatewayRecord(ctx context.Context, client *clientv3.Client, key string, leaseID clientv3.LeaseID, cfg GatewayConfig) error {
	value, err := EncodeGatewayRecord(BuildGatewayRecord(cfg, time.Now().UTC()))
	if err != nil {
		return err
	}
	if _, err := client.Put(ctx, key, value, clientv3.WithLease(leaseID)); err != nil {
		return fmt.Errorf("put gateway registry record: %w", err)
	}
	return nil
}

func revokeLease(ctx context.Context, client *clientv3.Client, leaseID clientv3.LeaseID) {
	revokeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, _ = client.Revoke(revokeCtx, leaseID)
}

func leaseTTLSeconds(ttl time.Duration) int64 {
	seconds := int64(ttl / time.Second)
	if ttl%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}

func defaultGatewayInstanceID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		host = "unknown-host"
	}
	return sanitizeKeyPart(host) + "-" + strconv.Itoa(os.Getpid())
}

func sanitizeKeyPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\x00", "_")
	return replacer.Replace(value)
}

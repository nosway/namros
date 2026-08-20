package coordination

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
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
	SchemaVersion       int               `json:"schema_version"`
	InstanceID          string            `json:"gateway_id"`
	Product             string            `json:"product"`
	Role                string            `json:"role"`
	ConnectionState     string            `json:"connection_state"`
	Readiness           string            `json:"readiness"`
	DrainState          string            `json:"drain_state"`
	AdvertiseEndpoint   string            `json:"advertise_endpoint,omitempty"`
	AdvertisedAddresses []string          `json:"advertised_addresses,omitempty"`
	ControlEndpoints    []GatewayEndpoint `json:"control_endpoints"`
	DataplaneEndpoints  []GatewayEndpoint `json:"dataplane_endpoints"`
	MetadataBackend     string            `json:"metadata_backend,omitempty"`
	StorageBackend      string            `json:"storage_backend,omitempty"`
	StartedAtUnix       int64             `json:"started_at_unix"`
	LastHeartbeatUnix   int64             `json:"last_seen_unix"`
	LeaseID             string            `json:"lease_id,omitempty"`
	LeaseExpiresAtUnix  int64             `json:"lease_expires_at_unix,omitempty"`
	FirstError          string            `json:"first_error,omitempty"`
	LastError           string            `json:"last_error,omitempty"`
	RegistryRevision    int64             `json:"registry_revision,omitempty"`
	Version             map[string]string `json:"version,omitempty"`
	Healthy             bool              `json:"-"`
	Ready               bool              `json:"-"`
	Status              string            `json:"-"`
	ListenAddr          string            `json:"-"`
}

type GatewayEndpoint struct {
	Address  string `json:"address"`
	AuthMode string `json:"auth_mode,omitempty"`
}

const (
	DefaultGatewayFleetPageSize int64 = 128
	MaxGatewayFleetPageSize     int64 = 512
)

type GatewayFleetListOptions struct {
	Limit    int64
	Cursor   string
	Revision int64
}

type GatewayFleetPage struct {
	Records    []GatewayRecord `json:"records"`
	Revision   int64           `json:"revision"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type GatewayFleetEvent struct {
	Type            string         `json:"type"`
	GatewayID       string         `json:"gateway_id,omitempty"`
	Record          *GatewayRecord `json:"record,omitempty"`
	Revision        int64          `json:"revision"`
	CompactRevision int64          `json:"compact_revision,omitempty"`
	Reason          string         `json:"reason,omitempty"`
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
	return GatewayRegistryPrefix(prefix) + "/" + sanitizeKeyPart(instanceID) + "/status"
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
		SchemaVersion:       1,
		InstanceID:          cfg.InstanceID,
		Product:             "namros",
		Role:                "object",
		ConnectionState:     "up",
		Readiness:           "ready",
		DrainState:          "active",
		AdvertiseEndpoint:   cfg.AdvertiseEndpoint,
		AdvertisedAddresses: []string{cfg.AdvertiseEndpoint},
		ControlEndpoints:    []GatewayEndpoint{{Address: cfg.AdvertiseEndpoint, AuthMode: "http"}},
		DataplaneEndpoints:  []GatewayEndpoint{},
		ListenAddr:          cfg.ListenAddr,
		Healthy:             true,
		Ready:               true,
		Status:              "ready",
		MetadataBackend:     cfg.MetadataBackend,
		StorageBackend:      cfg.StorageBackend,
		StartedAtUnix:       startedAt.Unix(),
		LastHeartbeatUnix:   now.Unix(),
		Version:             version.Info(),
	}
}

func (r *GatewayRecord) UnmarshalJSON(data []byte) error {
	type recordAlias GatewayRecord
	var wire struct {
		*recordAlias
		LegacyInstanceID        string `json:"instance_id"`
		LegacyLastHeartbeatUnix int64  `json:"last_heartbeat_unix"`
		LegacyHealthy           *bool  `json:"healthy"`
		LegacyReady             *bool  `json:"ready"`
		LegacyStatus            string `json:"status"`
		LegacyListenAddr        string `json:"listen_addr"`
	}
	wire.recordAlias = (*recordAlias)(r)
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if r.InstanceID == "" {
		r.InstanceID = wire.LegacyInstanceID
	}
	if r.LastHeartbeatUnix == 0 {
		r.LastHeartbeatUnix = wire.LegacyLastHeartbeatUnix
	}
	r.ListenAddr = wire.LegacyListenAddr
	if wire.LegacyHealthy != nil {
		r.Healthy = *wire.LegacyHealthy
	}
	if wire.LegacyReady != nil {
		r.Ready = *wire.LegacyReady
	}
	r.Status = wire.LegacyStatus
	normalizeGatewayRecord(r)
	return nil
}

func normalizeGatewayRecord(record *GatewayRecord) {
	if record.SchemaVersion == 0 {
		record.SchemaVersion = 1
	}
	if record.Product == "" {
		record.Product = "namros"
	}
	if record.Role == "" {
		record.Role = "object"
	}
	if record.ConnectionState == "" {
		if !record.Healthy && (record.Ready || record.Status != "") {
			record.ConnectionState = "down"
		} else {
			record.ConnectionState = "up"
		}
	}
	if record.Readiness == "" {
		if record.Ready {
			record.Readiness = "ready"
		} else {
			record.Readiness = "blocked"
		}
	}
	if record.DrainState == "" {
		if record.Status == "draining" {
			record.DrainState = "draining"
		} else {
			record.DrainState = "active"
		}
	}
	record.Healthy = record.ConnectionState == "up" || record.ConnectionState == "degraded"
	record.Ready = record.Readiness == "ready"
	record.Status = record.Readiness
	if record.DrainState != "active" {
		record.Status = record.DrainState
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

func ListGatewayFleetPage(ctx context.Context, cfg GatewayConfig, opts GatewayFleetListOptions) (GatewayFleetPage, error) {
	switch config.NormalizeCoordinationBackend(cfg.Backend) {
	case config.CoordinationBackendNone, "":
		return GatewayFleetPage{Records: []GatewayRecord{}}, nil
	case config.CoordinationBackendEtcd:
	default:
		return GatewayFleetPage{}, fmt.Errorf("unsupported coordination backend %q", cfg.Backend)
	}
	client, err := clientv3.New(clientv3.Config{Endpoints: cfg.EtcdEndpoints, DialTimeout: cfg.EtcdDialTimeout})
	if err != nil {
		return GatewayFleetPage{}, err
	}
	defer client.Close()
	return listEtcdGatewayRecordPage(ctx, client, cfg.RegistryPrefix, opts)
}

func WatchGatewayFleet(ctx context.Context, cfg GatewayConfig, afterRevision int64) (<-chan GatewayFleetEvent, error) {
	switch config.NormalizeCoordinationBackend(cfg.Backend) {
	case config.CoordinationBackendNone, "":
		out := make(chan GatewayFleetEvent)
		close(out)
		return out, nil
	case config.CoordinationBackendEtcd:
	default:
		return nil, fmt.Errorf("unsupported coordination backend %q", cfg.Backend)
	}
	if afterRevision < 0 {
		return nil, fmt.Errorf("gateway fleet watch revision must not be negative")
	}
	client, err := clientv3.New(clientv3.Config{Endpoints: cfg.EtcdEndpoints, DialTimeout: cfg.EtcdDialTimeout})
	if err != nil {
		return nil, err
	}
	prefix := GatewayRegistryPrefix(cfg.RegistryPrefix) + "/"
	opts := []clientv3.OpOption{clientv3.WithPrefix(), clientv3.WithPrevKV()}
	if afterRevision > 0 {
		opts = append(opts, clientv3.WithRev(afterRevision+1))
	}
	watch := client.Watch(ctx, prefix, opts...)
	out := make(chan GatewayFleetEvent, 16)
	go func() {
		defer client.Close()
		defer close(out)
		for resp := range watch {
			if resp.Canceled || resp.Err() != nil {
				reason := "watch canceled"
				if err := resp.Err(); err != nil {
					reason = err.Error()
				}
				out <- GatewayFleetEvent{Type: "resync_required", Revision: resp.Header.Revision, CompactRevision: resp.CompactRevision, Reason: reason}
				return
			}
			events, err := decodeGatewayFleetWatch(prefix, resp)
			if err != nil {
				out <- GatewayFleetEvent{Type: "resync_required", Revision: resp.Header.Revision, Reason: err.Error()}
				return
			}
			for _, event := range events {
				select {
				case <-ctx.Done():
					return
				case out <- event:
				}
			}
		}
	}()
	return out, nil
}

func HealthyGatewayRecords(records []GatewayRecord, now time.Time, maxHeartbeatAge time.Duration) []GatewayRecord {
	healthy := make([]GatewayRecord, 0, len(records))
	for _, record := range records {
		if strings.TrimSpace(record.InstanceID) == "" || strings.TrimSpace(record.AdvertiseEndpoint) == "" {
			continue
		}
		normalizeGatewayRecord(&record)
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
	rnd := rand.New(rand.NewSource(time.Now().UnixNano() ^ int64(lease.ID)))
	timer := time.NewTimer(jitteredHeartbeat(cfg.HeartbeatInterval, rnd))
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			if _, err := client.KeepAliveOnce(ctx, lease.ID); err != nil {
				return fmt.Errorf("keepalive gateway registry lease: %w", err)
			}
			if err := putGatewayRecord(ctx, client, key, lease.ID, cfg); err != nil {
				return err
			}
			timer.Reset(jitteredHeartbeat(cfg.HeartbeatInterval, rnd))
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

	records := []GatewayRecord{}
	opts := GatewayFleetListOptions{}
	for {
		page, err := listEtcdGatewayRecordPage(ctx, client, cfg.RegistryPrefix, opts)
		if err != nil {
			return nil, err
		}
		records = append(records, page.Records...)
		if page.NextCursor == "" {
			break
		}
		opts.Cursor = page.NextCursor
		opts.Revision = page.Revision
	}
	sortGatewayRecords(records)
	return records, nil
}

func listEtcdGatewayRecordPage(ctx context.Context, client *clientv3.Client, registryPrefix string, opts GatewayFleetListOptions) (GatewayFleetPage, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultGatewayFleetPageSize
	}
	if limit > MaxGatewayFleetPageSize {
		return GatewayFleetPage{}, fmt.Errorf("gateway fleet page limit %d exceeds maximum %d", limit, MaxGatewayFleetPageSize)
	}
	prefix := GatewayRegistryPrefix(registryPrefix) + "/"
	start := prefix
	if opts.Cursor != "" {
		if !strings.HasPrefix(opts.Cursor, prefix) {
			return GatewayFleetPage{}, fmt.Errorf("gateway fleet cursor is outside registry prefix")
		}
		start = opts.Cursor + "\x00"
	}
	getOpts := []clientv3.OpOption{
		clientv3.WithRange(clientv3.GetPrefixRangeEnd(prefix)),
		clientv3.WithLimit(limit),
		clientv3.WithSort(clientv3.SortByKey, clientv3.SortAscend),
	}
	if opts.Revision > 0 {
		getOpts = append(getOpts, clientv3.WithRev(opts.Revision))
	}
	resp, err := client.Get(ctx, start, getOpts...)
	if err != nil {
		return GatewayFleetPage{}, fmt.Errorf("list gateway registry records: %w", err)
	}
	return decodeGatewayFleetPage(prefix, resp)
}

func decodeGatewayFleetPage(prefix string, resp *clientv3.GetResponse) (GatewayFleetPage, error) {
	page := GatewayFleetPage{Records: []GatewayRecord{}}
	if resp == nil {
		return page, nil
	}
	if resp.Header != nil {
		page.Revision = resp.Header.Revision
	}
	for _, kv := range resp.Kvs {
		if !strings.HasPrefix(string(kv.Key), prefix) {
			continue
		}
		var record GatewayRecord
		if err := json.Unmarshal(kv.Value, &record); err != nil {
			return GatewayFleetPage{}, fmt.Errorf("decode gateway registry record %q: %w", string(kv.Key), err)
		}
		if strings.TrimSpace(record.InstanceID) == "" {
			continue
		}
		record.RegistryRevision = kv.ModRevision
		page.Records = append(page.Records, record)
	}
	if resp.More && len(resp.Kvs) > 0 {
		page.NextCursor = string(resp.Kvs[len(resp.Kvs)-1].Key)
	}
	return page, nil
}

func decodeGatewayFleetWatch(prefix string, resp clientv3.WatchResponse) ([]GatewayFleetEvent, error) {
	out := make([]GatewayFleetEvent, 0, len(resp.Events))
	for _, event := range resp.Events {
		if event == nil || event.Kv == nil {
			return nil, fmt.Errorf("gateway fleet watch returned an empty event")
		}
		gatewayID, ok := gatewayIDFromStatusKey(prefix, string(event.Kv.Key))
		if !ok {
			continue
		}
		revision := event.Kv.ModRevision
		if revision == 0 {
			revision = resp.Header.Revision
		}
		if event.Type == clientv3.EventTypeDelete {
			out = append(out, GatewayFleetEvent{Type: "delete", GatewayID: gatewayID, Revision: revision})
			continue
		}
		var record GatewayRecord
		if err := json.Unmarshal(event.Kv.Value, &record); err != nil {
			return nil, err
		}
		record.RegistryRevision = revision
		out = append(out, GatewayFleetEvent{Type: "put", GatewayID: gatewayID, Record: &record, Revision: revision})
	}
	return out, nil
}

func gatewayIDFromStatusKey(prefix, key string) (string, bool) {
	if !strings.HasPrefix(key, prefix) || !strings.HasSuffix(key, "/status") {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(key, prefix), "/status")
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
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
	now := time.Now().UTC()
	record := BuildGatewayRecord(cfg, now)
	record.LeaseID = strconv.FormatInt(int64(leaseID), 10)
	record.LeaseExpiresAtUnix = now.Add(cfg.LeaseTTL).Unix()
	value, err := EncodeGatewayRecord(record)
	if err != nil {
		return err
	}
	if _, err := client.Put(ctx, key, value, clientv3.WithLease(leaseID)); err != nil {
		return fmt.Errorf("put gateway registry record: %w", err)
	}
	return nil
}

func jitteredHeartbeat(base time.Duration, rnd *rand.Rand) time.Duration {
	if base <= 0 {
		return base
	}
	spread := float64(base) * 0.2
	out := time.Duration(float64(base) + (rnd.Float64()*2-1)*spread)
	if out <= 0 {
		return base
	}
	return out
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

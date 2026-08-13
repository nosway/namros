package sbsops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nosway/namros/internal/config"
)

type Config struct {
	ClusterID                        string
	AdminEndpoints                   []string
	DataEndpoints                    []string
	VolumeIDs                        []string
	VolumeDetails                    []VolumeStatus
	NAMRBDSBSObservabilityEndpoint   string
	NAMRBDSBSObservabilityTimeout    time.Duration
	NAMRBDSBSObservabilityHTTPClient *http.Client
}

type Collector struct {
	cfg      Config
	mu       sync.Mutex
	cache    Snapshot
	cacheAt  time.Time
	cacheSet bool
}

type Snapshot struct {
	SchemaVersion             string             `json:"schema_version"`
	GeneratedAt               string             `json:"generated_at"`
	Status                    string             `json:"status"`
	ReadOnly                  bool               `json:"read_only"`
	ClusterID                 string             `json:"cluster_id,omitempty"`
	SourceAuthority           string             `json:"source_authority"`
	SourceSchemaVersion       string             `json:"source_schema_version,omitempty"`
	CollectorFreshnessSeconds float64            `json:"collector_freshness_seconds,omitempty"`
	WarningCount              int                `json:"warning_count"`
	FirstError                string             `json:"first_error,omitempty"`
	LastError                 string             `json:"last_error,omitempty"`
	RBACChecked               bool               `json:"rbac_checked"`
	RedactionApplied          bool               `json:"redaction_applied"`
	UnsupportedClaimVisible   bool               `json:"unsupported_claim_visible"`
	MutationControlsEnabled   bool               `json:"mutation_controls_enabled"`
	Nodes                     NodeSummary        `json:"nodes"`
	Volumes                   VolumeSummary      `json:"volumes"`
	Stores                    []StoreStatus      `json:"stores,omitempty"`
	Capacity                  CapacitySummary    `json:"capacity,omitempty"`
	Pool                      PoolSummary        `json:"pool"`
	Reclaim                   ReclaimSummary     `json:"reclaim,omitempty"`
	Maintenance               MaintenanceSummary `json:"maintenance"`
	NodeDetails               []NodeStatus       `json:"node_details,omitempty"`
	VolumeDetails             []VolumeStatus     `json:"volume_details,omitempty"`
	Limitations               []string           `json:"limitations,omitempty"`
	NAMRBDSource              map[string]any     `json:"namrbd_source,omitempty"`
}

type NodeSummary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Abnormal int `json:"abnormal"`
	Unknown  int `json:"unknown"`
}

type VolumeSummary struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Degraded int `json:"degraded"`
	Unknown  int `json:"unknown"`
}

type MaintenanceSummary struct {
	RepairRunning    int `json:"repair_running"`
	RebalanceRunning int `json:"rebalance_running"`
	Stuck            int `json:"stuck"`
	Unknown          int `json:"unknown"`
}

type NodeStatus struct {
	NodeID   string `json:"node_id"`
	Role     string `json:"role"`
	Endpoint string `json:"endpoint"`
	State    string `json:"state"`
}

type VolumeStatus struct {
	VolumeID               string  `json:"volume_id"`
	State                  string  `json:"state"`
	ReadOnly               bool    `json:"read_only,omitempty"`
	Weight                 int     `json:"weight,omitempty"`
	AvailableBytes         uint64  `json:"available_bytes,omitempty"`
	UsedPercent            float64 `json:"used_percent,omitempty"`
	HighWatermarkPercent   float64 `json:"high_watermark_percent,omitempty"`
	AvailableBytesObserved bool    `json:"available_bytes_observed,omitempty"`
	UsedPercentObserved    bool    `json:"used_percent_observed,omitempty"`
	CapacityObserved       bool    `json:"capacity_observed,omitempty"`
	ObservedAt             string  `json:"observed_at,omitempty"`
	Source                 string  `json:"source,omitempty"`
}

type VolumeCapacityObservation struct {
	VolumeID       string
	AvailableBytes *uint64
	UsedPercent    *float64
	ObservedAt     string
	Source         string
}

type StoreStatus struct {
	StoreID        string  `json:"store_id"`
	NodeID         string  `json:"node_id,omitempty"`
	Role           string  `json:"role,omitempty"`
	State          string  `json:"state"`
	Health         string  `json:"health,omitempty"`
	VolumeID       string  `json:"volume_id,omitempty"`
	TotalBytes     uint64  `json:"total_bytes,omitempty"`
	UsedBytes      uint64  `json:"used_bytes,omitempty"`
	AvailableBytes uint64  `json:"available_bytes,omitempty"`
	UsedPercent    float64 `json:"used_percent,omitempty"`
}

type CapacitySummary struct {
	LogicalBytes     uint64  `json:"logical_bytes,omitempty"`
	TotalBytes       uint64  `json:"total_bytes,omitempty"`
	UsedBytes        uint64  `json:"used_bytes,omitempty"`
	AvailableBytes   uint64  `json:"available_bytes,omitempty"`
	ReservedBytes    uint64  `json:"reserved_bytes,omitempty"`
	UnknownBytes     uint64  `json:"unknown_bytes,omitempty"`
	ReclaimableBytes uint64  `json:"reclaimable_bytes,omitempty"`
	UsedPercent      float64 `json:"used_percent,omitempty"`
	StoresTotal      int     `json:"stores_total,omitempty"`
	VolumesTotal     int     `json:"volumes_total,omitempty"`
	Source           string  `json:"source,omitempty"`
}

type ReclaimSummary struct {
	Status           string   `json:"status,omitempty"`
	ReclaimableBytes uint64   `json:"reclaimable_bytes,omitempty"`
	Candidates       int      `json:"candidates,omitempty"`
	Running          int      `json:"running,omitempty"`
	Completed        int      `json:"completed,omitempty"`
	Failed           int      `json:"failed,omitempty"`
	Blocked          int      `json:"blocked,omitempty"`
	Limitations      []string `json:"limitations,omitempty"`
	Source           string   `json:"source,omitempty"`
}

type PoolSummary struct {
	PoolID                string          `json:"pool_id,omitempty"`
	Source                string          `json:"source,omitempty"`
	ConfiguredGeneration  uint64          `json:"configured_generation,omitempty"`
	ActiveGeneration      uint64          `json:"active_generation,omitempty"`
	MemberCount           int             `json:"member_count"`
	WritableMembers       int             `json:"writable_members,omitempty"`
	ReadOnlyMembers       int             `json:"read_only_members,omitempty"`
	HealthyMembers        int             `json:"healthy_members,omitempty"`
	DegradedMembers       int             `json:"degraded_members,omitempty"`
	UnknownMembers        int             `json:"unknown_members,omitempty"`
	AdmissionState        string          `json:"admission_state"`
	RefreshErrorCount     uint64          `json:"refresh_error_count,omitempty"`
	Stale                 bool            `json:"stale,omitempty"`
	StaleDurationSeconds  float64         `json:"stale_duration_seconds,omitempty"`
	CapacityObservedCount int             `json:"capacity_observed_count,omitempty"`
	Capacity              CapacitySummary `json:"capacity,omitempty"`
}

func NewCollector(cfg Config) *Collector {
	return &Collector{cfg: normalizeConfig(cfg)}
}

func DefaultNAMRBDTimeout() time.Duration {
	return config.DefaultNAMRBDSBSObservabilityTimeout
}

const defaultSnapshotCacheTTL = 15 * time.Second

func ConfigFromApp(cfg config.Config) Config {
	base := Config{
		NAMRBDSBSObservabilityEndpoint: cfg.NAMRBDSBSObservabilityEndpoint,
		NAMRBDSBSObservabilityTimeout:  cfg.NAMRBDSBSObservabilityTimeout,
	}
	if len(cfg.SBSVolumePool) > 0 {
		out := base
		out.ClusterID = cfg.SBSVolumeID
		out.AdminEndpoints = []string{cfg.SBSAdminEndpoint}
		out.DataEndpoints = []string{cfg.SBSDataEndpoint}
		for _, member := range cfg.SBSVolumePool {
			out.VolumeIDs = append(out.VolumeIDs, member.VolumeID)
			out.VolumeDetails = append(out.VolumeDetails, VolumeStatus{
				VolumeID:               member.VolumeID,
				State:                  volumePoolMemberState(member),
				ReadOnly:               member.ReadOnly,
				Weight:                 member.Weight,
				AvailableBytes:         member.AvailableBytes,
				UsedPercent:            member.UsedPercent,
				HighWatermarkPercent:   member.HighWatermarkPercent,
				AvailableBytesObserved: member.AvailableBytes > 0,
				UsedPercentObserved:    member.UsedPercent > 0,
				CapacityObserved:       member.AvailableBytes > 0 || member.UsedPercent > 0,
				Source:                 "namros_config",
			})
			if member.AdminEndpoint != "" {
				out.AdminEndpoints = append(out.AdminEndpoints, member.AdminEndpoint)
			}
			if member.DataEndpoint != "" {
				out.DataEndpoints = append(out.DataEndpoints, member.DataEndpoint)
			}
		}
		if out.ClusterID == "" {
			out.ClusterID = strings.Join(out.VolumeIDs, ",")
		}
		return out
	}
	base.ClusterID = cfg.SBSVolumeID
	base.AdminEndpoints = []string{cfg.SBSAdminEndpoint}
	base.DataEndpoints = []string{cfg.SBSDataEndpoint}
	base.VolumeIDs = []string{cfg.SBSVolumeID}
	return base
}

func (c *Collector) Snapshot(ctx context.Context) Snapshot {
	cfg := normalizeConfig(c.cfg)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint) != "" {
		if snapshot, ok := c.cachedSnapshot(time.Now()); ok {
			return snapshot
		}
		if snapshot, ok := c.snapshotFromNAMRBD(ctx, cfg, now); ok {
			c.storeSnapshot(snapshot, time.Now())
			return snapshot
		}
	}
	nodes := make([]NodeStatus, 0, len(cfg.AdminEndpoints)+len(cfg.DataEndpoints))
	for _, endpoint := range cfg.AdminEndpoints {
		nodes = append(nodes, NodeStatus{
			NodeID:   nodeID("admin", endpoint),
			Role:     "admin",
			Endpoint: endpoint,
			State:    "unknown",
		})
	}
	for _, endpoint := range cfg.DataEndpoints {
		nodes = append(nodes, NodeStatus{
			NodeID:   nodeID("data", endpoint),
			Role:     "data",
			Endpoint: endpoint,
			State:    "unknown",
		})
	}
	volumes := make([]VolumeStatus, 0, len(cfg.VolumeIDs))
	if len(cfg.VolumeDetails) > 0 {
		volumes = append(volumes, cfg.VolumeDetails...)
	} else {
		for _, volumeID := range cfg.VolumeIDs {
			volumes = append(volumes, VolumeStatus{VolumeID: volumeID, State: "unknown"})
		}
	}
	status := "disabled"
	limitations := []string{"SBS operations collector has no configured endpoints."}
	if len(nodes) > 0 || len(volumes) > 0 {
		status = "partial"
		limitations = []string{"SBS live RPC health collection is not enabled in this build slice; configured endpoints are reported as unknown."}
	}
	snapshot := Snapshot{
		SchemaVersion:           "namros.console.sbs.cluster.v1",
		GeneratedAt:             now,
		Status:                  status,
		ReadOnly:                true,
		ClusterID:               cfg.ClusterID,
		SourceAuthority:         "namros_sbs_adapter_fallback",
		RBACChecked:             true,
		RedactionApplied:        true,
		UnsupportedClaimVisible: true,
		MutationControlsEnabled: false,
		WarningCount:            len(limitations),
		Nodes: NodeSummary{
			Total:   len(nodes),
			Unknown: len(nodes),
		},
		Volumes:       summarizeVolumes(volumes),
		Maintenance:   MaintenanceSummary{Unknown: 1},
		NodeDetails:   nodes,
		VolumeDetails: volumes,
		Limitations:   limitations,
	}
	snapshot.Pool = summarizePool(snapshot)
	return snapshot
}

func VolumeCapacityObservations(snapshot Snapshot) []VolumeCapacityObservation {
	out := make([]VolumeCapacityObservation, 0, len(snapshot.VolumeDetails))
	source := strings.TrimSpace(snapshot.SourceAuthority)
	for _, volume := range snapshot.VolumeDetails {
		if strings.TrimSpace(volume.VolumeID) == "" || !volume.CapacityObserved {
			continue
		}
		observation := VolumeCapacityObservation{
			VolumeID:   volume.VolumeID,
			ObservedAt: volume.ObservedAt,
			Source:     firstNonEmpty(volume.Source, source),
		}
		if volume.AvailableBytesObserved {
			observation.AvailableBytes = uint64Ptr(volume.AvailableBytes)
		}
		if volume.UsedPercentObserved {
			observation.UsedPercent = float64Ptr(volume.UsedPercent)
		}
		out = append(out, observation)
	}
	return out
}

func (c *Collector) cachedSnapshot(now time.Time) (Snapshot, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.cacheSet || c.cacheAt.IsZero() || now.Sub(c.cacheAt) > defaultSnapshotCacheTTL {
		return Snapshot{}, false
	}
	return c.cache, true
}

func (c *Collector) storeSnapshot(snapshot Snapshot, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = snapshot
	c.cacheAt = now
	c.cacheSet = true
}

func (c *Collector) snapshotFromNAMRBD(ctx context.Context, cfg Config, now string) (Snapshot, bool) {
	endpoint, err := namrbdSBSObservabilityURL(cfg.NAMRBDSBSObservabilityEndpoint)
	if err != nil {
		return snapshotWithNAMRBDError(cfg, now, err), true
	}
	timeout := cfg.NAMRBDSBSObservabilityTimeout
	if timeout <= 0 {
		timeout = config.DefaultNAMRBDSBSObservabilityTimeout
	}
	if ctx == nil {
		ctx = context.Background()
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return snapshotWithNAMRBDError(cfg, now, err), true
	}
	req.Header.Set("Accept", "application/json")
	client := cfg.NAMRBDSBSObservabilityHTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return snapshotWithNAMRBDError(cfg, now, err), true
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return snapshotWithNAMRBDError(cfg, now, fmt.Errorf("NAMRBD SBS observability status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))), true
	}
	var raw map[string]any
	decoder := json.NewDecoder(io.LimitReader(resp.Body, 8<<20))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return snapshotWithNAMRBDError(cfg, now, err), true
	}
	if raw == nil {
		return snapshotWithNAMRBDError(cfg, now, fmt.Errorf("NAMRBD SBS observability returned empty JSON object")), true
	}
	return snapshotFromNAMRBDRaw(cfg, now, raw), true
}

func snapshotFromNAMRBDRaw(cfg Config, now string, raw map[string]any) Snapshot {
	generatedAt := stringField(raw, "generated_at", "collected_at", "timestamp")
	if generatedAt == "" {
		generatedAt = now
	}
	status := normalizeStatus(stringField(raw, "status", "collection_status", "health", "state"))
	if status == "" {
		status = "ok"
	}
	sourceSchema := stringField(raw, "schema_version")
	limitations := stringSliceField(raw, "limitations", "warnings")
	nodeSummary, nodeDetails := parseNAMRBDNodes(raw)
	volumeSummary, volumeDetails := parseNAMRBDVolumes(raw)
	stores := parseNAMRBDStores(raw)
	maintenance := parseNAMRBDMaintenance(raw)
	capacity := parseNAMRBDCapacity(raw, stores, volumeSummary)
	reclaim := parseNAMRBDReclaim(raw)
	warningCount := intField(raw, "warning_count", "warnings_count")
	if warningCount == 0 {
		warningCount = len(limitations)
	}
	clusterID := stringField(raw, "cluster_id", "sbs_cluster_id")
	if clusterID == "" {
		clusterID = cfg.ClusterID
	}
	snapshot := Snapshot{
		SchemaVersion:             "namros.console.sbs.cluster.v1",
		GeneratedAt:               generatedAt,
		Status:                    status,
		ReadOnly:                  true,
		ClusterID:                 clusterID,
		SourceAuthority:           "namrbd_sbs_observability",
		SourceSchemaVersion:       sourceSchema,
		CollectorFreshnessSeconds: firstPositiveFloat(floatField(raw, "collector_freshness_seconds", "freshness_seconds"), freshnessSeconds(generatedAt, now)),
		WarningCount:              warningCount,
		FirstError:                stringField(raw, "first_error"),
		LastError:                 stringField(raw, "last_error"),
		RBACChecked:               boolField(raw, true, "rbac_checked"),
		RedactionApplied:          boolField(raw, true, "redaction_applied"),
		UnsupportedClaimVisible:   true,
		MutationControlsEnabled:   false,
		Nodes:                     nodeSummary,
		Volumes:                   volumeSummary,
		Stores:                    stores,
		Capacity:                  capacity,
		Reclaim:                   reclaim,
		Maintenance:               maintenance,
		NodeDetails:               nodeDetails,
		VolumeDetails:             volumeDetails,
		Limitations:               limitations,
		NAMRBDSource:              redactMap(raw),
	}
	snapshot.Pool = parseNAMRBDPool(raw, snapshot)
	return snapshot
}

func snapshotWithNAMRBDError(cfg Config, now string, err error) Snapshot {
	fallback := NewCollector(Config{
		ClusterID:      cfg.ClusterID,
		AdminEndpoints: cfg.AdminEndpoints,
		DataEndpoints:  cfg.DataEndpoints,
		VolumeIDs:      cfg.VolumeIDs,
		VolumeDetails:  cfg.VolumeDetails,
	}).Snapshot(context.Background())
	fallback.GeneratedAt = now
	fallback.Status = "degraded"
	fallback.SourceAuthority = "namrbd_sbs_observability"
	fallback.WarningCount = len(fallback.Limitations) + 1
	fallback.FirstError = err.Error()
	fallback.LastError = err.Error()
	fallback.Limitations = append([]string{"NAMRBD SBS observability endpoint is unavailable; NAMROS is showing configured endpoint fallback only."}, fallback.Limitations...)
	return fallback
}

func normalizeConfig(cfg Config) Config {
	cfg.ClusterID = strings.TrimSpace(cfg.ClusterID)
	cfg.AdminEndpoints = cleanList(cfg.AdminEndpoints)
	cfg.DataEndpoints = cleanList(cfg.DataEndpoints)
	cfg.VolumeIDs = cleanList(cfg.VolumeIDs)
	cfg.VolumeDetails = cleanVolumeDetails(cfg.VolumeDetails)
	cfg.NAMRBDSBSObservabilityEndpoint = strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint)
	return cfg
}

func namrbdSBSObservabilityURL(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "", fmt.Errorf("NAMRBD SBS observability endpoint is empty")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "http://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("NAMRBD SBS observability endpoint must include a host")
	}
	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/api/v1/sbs/cluster"
	}
	return parsed.String(), nil
}

func parseNAMRBDNodes(raw map[string]any) (NodeSummary, []NodeStatus) {
	if nodesMap := mapField(raw, "nodes"); nodesMap != nil {
		details := parseNAMRBDNodeList(objectSliceField(nodesMap, "items", "details", "nodes"))
		summary := NodeSummary{
			Total:    intField(nodesMap, "total"),
			Healthy:  intField(nodesMap, "healthy", "ready", "up"),
			Abnormal: intField(nodesMap, "abnormal", "degraded", "down"),
			Unknown:  intField(nodesMap, "unknown"),
		}
		if summary.Total == 0 && (summary.Healthy+summary.Abnormal+summary.Unknown) > 0 {
			summary.Total = summary.Healthy + summary.Abnormal + summary.Unknown
		}
		if summary.Total == 0 && len(details) > 0 {
			summary = summarizeNodes(details)
		}
		return summary, details
	}
	details := parseNAMRBDNodeList(objectSliceField(raw, "node_details", "nodes"))
	return summarizeNodes(details), details
}

func parseNAMRBDNodeList(values []map[string]any) []NodeStatus {
	out := make([]NodeStatus, 0, len(values))
	for _, value := range values {
		node := NodeStatus{
			NodeID:   stringField(value, "node_id", "id", "name"),
			Role:     stringField(value, "role", "kind"),
			Endpoint: stringField(value, "endpoint", "address"),
			State:    normalizeNodeState(stringField(value, "state", "status", "health")),
		}
		if node.NodeID == "" {
			node.NodeID = nodeID(node.Role, node.Endpoint)
		}
		if node.Role == "" {
			node.Role = "sbs"
		}
		if node.State == "" {
			node.State = "unknown"
		}
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].NodeID < out[j].NodeID
	})
	return out
}

func summarizeNodes(nodes []NodeStatus) NodeSummary {
	out := NodeSummary{Total: len(nodes)}
	for _, node := range nodes {
		switch normalizeNodeState(node.State) {
		case "healthy":
			out.Healthy++
		case "unknown", "":
			out.Unknown++
		default:
			out.Abnormal++
		}
	}
	return out
}

func parseNAMRBDVolumes(raw map[string]any) (VolumeSummary, []VolumeStatus) {
	if volumesMap := mapField(raw, "volumes"); volumesMap != nil {
		details := parseNAMRBDVolumeList(objectSliceField(volumesMap, "items", "details", "volumes"))
		summary := VolumeSummary{
			Total:    intField(volumesMap, "total"),
			Healthy:  intField(volumesMap, "healthy", "active"),
			Degraded: intField(volumesMap, "degraded", "abnormal"),
			Unknown:  intField(volumesMap, "unknown"),
		}
		if summary.Total == 0 && (summary.Healthy+summary.Degraded+summary.Unknown) > 0 {
			summary.Total = summary.Healthy + summary.Degraded + summary.Unknown
		}
		if summary.Total == 0 && len(details) > 0 {
			summary = summarizeVolumes(details)
		}
		return summary, details
	}
	details := parseNAMRBDVolumeList(objectSliceField(raw, "volume_details", "volumes"))
	return summarizeVolumes(details), details
}

func parseNAMRBDVolumeList(values []map[string]any) []VolumeStatus {
	out := make([]VolumeStatus, 0, len(values))
	for _, value := range values {
		availableBytes, availableObserved := uint64FieldOK(value, "available_bytes", "free_bytes")
		usedPercent, usedObserved := floatFieldOK(value, "used_percent", "usage_percent")
		volume := VolumeStatus{
			VolumeID:               stringField(value, "volume_id", "id", "name"),
			State:                  normalizeVolumeState(stringField(value, "state", "status", "health")),
			ReadOnly:               boolField(value, false, "read_only", "readonly"),
			Weight:                 intField(value, "weight"),
			AvailableBytes:         availableBytes,
			UsedPercent:            usedPercent,
			HighWatermarkPercent:   floatField(value, "high_watermark_percent"),
			AvailableBytesObserved: availableObserved,
			UsedPercentObserved:    usedObserved,
			CapacityObserved:       availableObserved || usedObserved,
			ObservedAt:             stringField(value, "observed_at", "last_observed_at", "collected_at"),
			Source:                 firstNonEmpty(stringField(value, "source"), "namrbd_sbs_observability"),
		}
		if volume.State == "" {
			volume.State = "unknown"
		}
		if volume.State == "read_only" {
			volume.ReadOnly = true
		}
		if volume.VolumeID == "" {
			continue
		}
		out = append(out, volume)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].VolumeID < out[j].VolumeID
	})
	return out
}

func parseNAMRBDStores(raw map[string]any) []StoreStatus {
	values := objectSliceField(raw, "store_details", "stores")
	if storesMap := mapField(raw, "stores"); storesMap != nil {
		values = objectSliceField(storesMap, "items", "details", "stores")
	}
	out := make([]StoreStatus, 0, len(values))
	for _, value := range values {
		store := StoreStatus{
			StoreID:        stringField(value, "store_id", "id", "name"),
			NodeID:         stringField(value, "node_id", "node"),
			Role:           stringField(value, "role", "kind"),
			State:          normalizeNodeState(stringField(value, "state", "status", "health", "lifecycle")),
			Health:         normalizeNodeState(stringField(value, "health")),
			VolumeID:       stringField(value, "volume_id", "volume"),
			TotalBytes:     uint64Field(value, "total_bytes", "capacity_bytes", "physical_total_bytes"),
			UsedBytes:      uint64Field(value, "used_bytes", "physical_used_bytes"),
			AvailableBytes: uint64Field(value, "available_bytes", "free_bytes", "physical_free_bytes"),
			UsedPercent:    floatField(value, "used_percent", "usage_percent"),
		}
		if store.StoreID == "" && store.NodeID != "" {
			store.StoreID = store.NodeID
		}
		if store.StoreID == "" {
			continue
		}
		if store.Role == "" {
			store.Role = "data"
		}
		if store.State == "" {
			store.State = "unknown"
		}
		out = append(out, store)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StoreID < out[j].StoreID
	})
	return out
}

func parseNAMRBDMaintenance(raw map[string]any) MaintenanceSummary {
	maintenance := mapField(raw, "maintenance")
	if maintenance == nil {
		return MaintenanceSummary{Unknown: intField(raw, "maintenance_unknown")}
	}
	return MaintenanceSummary{
		RepairRunning:    intField(maintenance, "repair_running", "repairs_running"),
		RebalanceRunning: intField(maintenance, "rebalance_running", "rebalances_running"),
		Stuck:            intField(maintenance, "stuck"),
		Unknown:          intField(maintenance, "unknown"),
	}
}

func parseNAMRBDCapacity(raw map[string]any, stores []StoreStatus, volumes VolumeSummary) CapacitySummary {
	capacity := mapField(raw, "capacity")
	out := CapacitySummary{Source: "namrbd_sbs_observability"}
	if capacity != nil {
		out.LogicalBytes = uint64Field(capacity, "logical_bytes")
		out.TotalBytes = uint64Field(capacity, "total_bytes", "capacity_bytes", "physical_total_bytes")
		out.UsedBytes = uint64Field(capacity, "used_bytes", "physical_used_bytes")
		out.AvailableBytes = uint64Field(capacity, "available_bytes", "free_bytes", "physical_free_bytes")
		out.ReservedBytes = uint64Field(capacity, "reserved_bytes")
		out.UnknownBytes = uint64Field(capacity, "unknown_bytes", "physical_unknown_bytes")
		out.ReclaimableBytes = uint64Field(capacity, "reclaimable_bytes", "physical_reclaimable_bytes", "pending_reclaim_bytes")
		out.UsedPercent = floatField(capacity, "used_percent", "usage_percent")
		out.StoresTotal = intField(capacity, "stores_total", "store_count")
		out.VolumesTotal = intField(capacity, "volumes_total", "volume_count")
	}
	if out.StoresTotal == 0 {
		out.StoresTotal = len(stores)
	}
	if out.VolumesTotal == 0 {
		out.VolumesTotal = volumes.Total
	}
	if out.TotalBytes == 0 && out.UsedBytes == 0 && out.AvailableBytes == 0 {
		for _, store := range stores {
			out.TotalBytes += store.TotalBytes
			out.UsedBytes += store.UsedBytes
			out.AvailableBytes += store.AvailableBytes
		}
	}
	if out.TotalBytes == 0 && out.UsedBytes+out.AvailableBytes > 0 {
		out.TotalBytes = out.UsedBytes + out.AvailableBytes
	}
	if out.UsedPercent == 0 && out.TotalBytes > 0 {
		out.UsedPercent = float64(out.UsedBytes) / float64(out.TotalBytes) * 100
	}
	return out
}

func parseNAMRBDReclaim(raw map[string]any) ReclaimSummary {
	reclaim := mapField(raw, "reclaim")
	if reclaim == nil {
		return ReclaimSummary{Source: "namrbd_sbs_observability"}
	}
	status := normalizeStatus(stringField(reclaim, "status", "state"))
	if status == "" {
		switch {
		case boolField(reclaim, false, "protected_reference_check_passed") && boolField(reclaim, false, "evidence_required"):
			status = "evidence_required"
		case boolField(reclaim, false, "protected_reference_check_passed"):
			status = "ok"
		}
	}
	return ReclaimSummary{
		Status:           status,
		ReclaimableBytes: uint64Field(reclaim, "reclaimable_bytes", "estimated_bytes", "pending_reclaim_bytes"),
		Candidates:       intField(reclaim, "candidates", "candidate_count"),
		Running:          intField(reclaim, "running"),
		Completed:        intField(reclaim, "completed"),
		Failed:           intField(reclaim, "failed"),
		Blocked:          intField(reclaim, "blocked"),
		Limitations:      stringSliceField(reclaim, "limitations", "warnings"),
		Source:           "namrbd_sbs_observability",
	}
}

func parseNAMRBDPool(raw map[string]any, snapshot Snapshot) PoolSummary {
	out := summarizePool(snapshot)
	pool := mapField(raw, "pool", "volume_pool")
	if pool == nil {
		pool = raw
	}
	out.PoolID = firstNonEmpty(stringField(pool, "pool_id", "volume_pool_id", "id", "name"), out.PoolID)
	out.Source = firstNonEmpty(stringField(pool, "source"), out.Source)
	if value := uint64Field(pool, "configured_generation", "generation"); value > 0 {
		out.ConfiguredGeneration = value
	}
	if value := uint64Field(pool, "active_generation"); value > 0 {
		out.ActiveGeneration = value
	}
	if value := intField(pool, "member_count", "members_total", "volume_count"); value > 0 {
		out.MemberCount = value
	}
	if value := intField(pool, "writable_members", "writable_member_count"); value > 0 {
		out.WritableMembers = value
	}
	if value := intField(pool, "read_only_members", "readonly_members", "read_only_member_count"); value > 0 {
		out.ReadOnlyMembers = value
	}
	if value := intField(pool, "healthy_members", "healthy_member_count"); value > 0 {
		out.HealthyMembers = value
	}
	if value := intField(pool, "degraded_members", "degraded_member_count"); value > 0 {
		out.DegradedMembers = value
	}
	if value := intField(pool, "unknown_members", "unknown_member_count"); value > 0 {
		out.UnknownMembers = value
	}
	if value := normalizeAdmissionState(stringField(pool, "admission_state", "admission", "write_admission")); value != "" {
		out.AdmissionState = value
	}
	out.RefreshErrorCount = uint64Field(pool, "refresh_error_count", "refresh_errors")
	out.Stale = boolField(pool, out.Stale, "stale")
	out.StaleDurationSeconds = firstPositiveFloat(floatField(pool, "stale_duration_seconds", "stale_seconds"), out.StaleDurationSeconds)
	if out.StaleDurationSeconds > 0 {
		out.Stale = true
	}
	return out
}

func summarizePool(snapshot Snapshot) PoolSummary {
	capacity := snapshot.Capacity
	if capacity.Source == "" {
		capacity.Source = firstNonEmpty(snapshot.Capacity.Source, snapshot.SourceAuthority)
	}
	out := PoolSummary{
		PoolID:   snapshot.ClusterID,
		Source:   firstNonEmpty(snapshot.SourceAuthority, capacity.Source),
		Capacity: capacity,
	}
	volumeDetails := snapshot.VolumeDetails
	out.MemberCount = len(volumeDetails)
	if out.PoolID == "" && len(volumeDetails) == 1 {
		out.PoolID = volumeDetails[0].VolumeID
	}
	if out.MemberCount == 0 {
		out.MemberCount = snapshot.Volumes.Total
	}
	out.HealthyMembers = snapshot.Volumes.Healthy
	out.DegradedMembers = snapshot.Volumes.Degraded
	out.UnknownMembers = snapshot.Volumes.Unknown
	for _, volume := range volumeDetails {
		state := normalizeVolumeState(volume.State)
		if volume.ReadOnly || state == "read_only" {
			out.ReadOnlyMembers++
		}
		if isWritableVolumeState(state) && !volume.ReadOnly {
			out.WritableMembers++
		}
		if volume.CapacityObserved {
			out.CapacityObservedCount++
		}
		if capacity.TotalBytes == 0 && capacity.UsedBytes == 0 && capacity.AvailableBytes == 0 {
			if volume.AvailableBytesObserved {
				out.Capacity.AvailableBytes += volume.AvailableBytes
			}
			if volume.UsedPercentObserved && volume.UsedPercent > out.Capacity.UsedPercent {
				out.Capacity.UsedPercent = volume.UsedPercent
			}
		}
	}
	out.AdmissionState = poolAdmissionState(out)
	return out
}

func poolAdmissionState(pool PoolSummary) string {
	switch {
	case pool.MemberCount == 0:
		return "disabled"
	case pool.WritableMembers > 0 && pool.DegradedMembers == 0 && pool.UnknownMembers == 0:
		return "writable"
	case pool.WritableMembers > 0:
		return "degraded_writable"
	case pool.ReadOnlyMembers > 0:
		return "read_only"
	default:
		return "unknown"
	}
}

func isWritableVolumeState(state string) bool {
	switch normalizeVolumeState(state) {
	case "active", "healthy", "writable":
		return true
	default:
		return false
	}
}

func normalizeAdmissionState(state string) string {
	state = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(state, "-", "_")))
	switch state {
	case "write", "write_enabled", "admit", "admitted":
		return "writable"
	default:
		return state
	}
}

func objectSliceField(raw map[string]any, names ...string) []map[string]any {
	for _, name := range names {
		values, ok := raw[name]
		if !ok {
			continue
		}
		switch typed := values.(type) {
		case []any:
			out := make([]map[string]any, 0, len(typed))
			for _, item := range typed {
				if mapped, ok := item.(map[string]any); ok {
					out = append(out, mapped)
				}
			}
			return out
		case []map[string]any:
			return typed
		}
	}
	return nil
}

func mapField(raw map[string]any, names ...string) map[string]any {
	for _, name := range names {
		if mapped, ok := raw[name].(map[string]any); ok {
			return mapped
		}
	}
	return nil
}

func stringField(raw map[string]any, names ...string) string {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		case json.Number:
			return typed.String()
		case fmt.Stringer:
			return strings.TrimSpace(typed.String())
		}
	}
	return ""
}

func stringSliceField(raw map[string]any, names ...string) []string {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case []string:
			return cleanList(typed)
		case []any:
			out := make([]string, 0, len(typed))
			for _, item := range typed {
				switch v := item.(type) {
				case string:
					out = append(out, strings.TrimSpace(v))
				case json.Number:
					out = append(out, v.String())
				}
			}
			return cleanList(out)
		case string:
			if strings.TrimSpace(typed) != "" {
				return []string{strings.TrimSpace(typed)}
			}
		}
	}
	return nil
}

func intField(raw map[string]any, names ...string) int {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case int:
			return typed
		case int64:
			return int(typed)
		case float64:
			return int(typed)
		case json.Number:
			if parsed, err := typed.Int64(); err == nil {
				return int(parsed)
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func uint64Field(raw map[string]any, names ...string) uint64 {
	value, _ := uint64FieldOK(raw, names...)
	return value
}

func uint64FieldOK(raw map[string]any, names ...string) (uint64, bool) {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case uint64:
			return typed, true
		case int:
			if typed >= 0 {
				return uint64(typed), true
			}
		case int64:
			if typed >= 0 {
				return uint64(typed), true
			}
		case float64:
			if typed >= 0 {
				return uint64(typed), true
			}
		case json.Number:
			if parsed, err := typed.Int64(); err == nil && parsed >= 0 {
				return uint64(parsed), true
			}
		case string:
			if parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 64); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func floatField(raw map[string]any, names ...string) float64 {
	value, _ := floatFieldOK(raw, names...)
	return value
}

func floatFieldOK(raw map[string]any, names ...string) (float64, bool) {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed, true
		case int:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case json.Number:
			if parsed, err := typed.Float64(); err == nil {
				return parsed, true
			}
		case string:
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64); err == nil {
				return parsed, true
			}
		}
	}
	return 0, false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uint64Ptr(value uint64) *uint64 {
	return &value
}

func float64Ptr(value float64) *float64 {
	return &value
}

func boolField(raw map[string]any, defaultValue bool, names ...string) bool {
	for _, name := range names {
		value, ok := raw[name]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "true", "yes", "1":
				return true
			case "false", "no", "0":
				return false
			}
		}
	}
	return defaultValue
}

func freshnessSeconds(generatedAt, now string) float64 {
	generated, err := time.Parse(time.RFC3339Nano, generatedAt)
	if err != nil {
		return 0
	}
	current, err := time.Parse(time.RFC3339Nano, now)
	if err != nil {
		return 0
	}
	if current.Before(generated) {
		return 0
	}
	return current.Sub(generated).Seconds()
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(status, "-", "_")))
	switch status {
	case "ready", "up":
		return "ok"
	default:
		return status
	}
}

func normalizeNodeState(state string) string {
	state = normalizeStatus(state)
	switch state {
	case "ok", "ready", "up", "active":
		return "healthy"
	case "":
		return "unknown"
	default:
		return state
	}
}

func redactMap(raw map[string]any) map[string]any {
	out := make(map[string]any, len(raw))
	for key, value := range raw {
		out[key] = redactValue(key, value)
	}
	return out
}

func redactValue(key string, value any) any {
	if sensitiveField(key) {
		return "[REDACTED]"
	}
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed)
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactValue("", item))
		}
		return out
	default:
		return value
	}
}

func sensitiveField(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(key, "-", "_")))
	for _, needle := range []string{"secret", "token", "authorization", "credential", "password", "private_key", "access_key", "session"} {
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
}

func cleanVolumeDetails(values []VolumeStatus) []VolumeStatus {
	seen := map[string]struct{}{}
	out := make([]VolumeStatus, 0, len(values))
	for _, value := range values {
		value.VolumeID = strings.TrimSpace(value.VolumeID)
		if value.VolumeID == "" {
			continue
		}
		if _, ok := seen[value.VolumeID]; ok {
			continue
		}
		seen[value.VolumeID] = struct{}{}
		value.State = normalizeVolumeState(value.State)
		if value.State == "read_only" {
			value.ReadOnly = true
		}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].VolumeID < out[j].VolumeID
	})
	return out
}

func summarizeVolumes(volumes []VolumeStatus) VolumeSummary {
	out := VolumeSummary{Total: len(volumes)}
	for _, volume := range volumes {
		switch normalizeVolumeState(volume.State) {
		case "active", "healthy":
			out.Healthy++
		case "degraded", "draining", "full", "offline", "read_only":
			out.Degraded++
		default:
			out.Unknown++
		}
	}
	return out
}

func volumePoolMemberState(member config.SBSVolumePoolMember) string {
	state := normalizeVolumeState(member.State)
	if state == "" {
		if member.ReadOnly {
			return "read_only"
		}
		return "active"
	}
	return state
}

func normalizeVolumeState(state string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(state, "-", "_")))
}

func cleanList(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func nodeID(role, endpoint string) string {
	replacer := strings.NewReplacer("://", "-", ":", "-", "/", "-")
	return role + "-" + replacer.Replace(endpoint)
}

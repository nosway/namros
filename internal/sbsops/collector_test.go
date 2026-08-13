package sbsops

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/config"
)

func TestCollectorSnapshotReportsConfiguredEndpointsAsPartial(t *testing.T) {
	collector := NewCollector(Config{
		ClusterID:      "namros-lab",
		AdminEndpoints: []string{"sbs-admin-a:9443", "sbs-admin-a:9443", "sbs-admin-b:9443"},
		DataEndpoints:  []string{"sbs-data-a:19092"},
		VolumeIDs:      []string{"18a00001"},
	})
	snapshot := collector.Snapshot(context.Background())
	if snapshot.Status != "partial" {
		t.Fatalf("status = %q, want partial", snapshot.Status)
	}
	if snapshot.Nodes.Total != 3 || snapshot.Nodes.Unknown != 3 {
		t.Fatalf("nodes summary = %+v, want total/unknown 3", snapshot.Nodes)
	}
	if snapshot.Volumes.Total != 1 || snapshot.Volumes.Unknown != 1 {
		t.Fatalf("volumes summary = %+v, want total/unknown 1", snapshot.Volumes)
	}
	if len(snapshot.Limitations) == 0 {
		t.Fatalf("limitations should describe partial collector state")
	}
}

func TestCollectorSnapshotProxiesNAMRBDObservability(t *testing.T) {
	var requestedPath string
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requestedPath = r.URL.Path
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"schema_version": "namrbd.sbs.observability.v1",
			"generated_at": "2026-08-05T00:00:00Z",
			"status": "ok",
			"cluster_id": "namrbd-cluster",
			"nodes": [{"node_id": "node-a", "role": "data", "endpoint": "sbs-data-a:9460", "health": "healthy"}],
			"stores": [{"store_id": "store-a", "node_id": "node-a", "state": "healthy", "total_bytes": 1000, "used_bytes": 400, "available_bytes": 600}],
			"volumes": [{"volume_id": "vol-a", "state": "active"}],
			"pool": {"pool_id": "object-pool", "configured_generation": 7, "active_generation": 6, "member_count": 1, "writable_members": 1, "admission_state": "degraded_writable", "refresh_error_count": 2, "stale_seconds": 11},
			"maintenance": {"repair_running": 1},
			"capacity": {"total_bytes": 1000, "used_bytes": 400, "available_bytes": 600, "reclaimable_bytes": 80, "used_percent": 40},
			"reclaim": {"status": "ok", "reclaimable_bytes": 80, "candidates": 2},
			"secret_token": "hidden"
		}`)),
		}, nil
	})}

	collector := NewCollector(Config{
		ClusterID:                        "fallback-cluster",
		NAMRBDSBSObservabilityEndpoint:   "http://namrbd-observability",
		NAMRBDSBSObservabilityTimeout:    time.Second,
		NAMRBDSBSObservabilityHTTPClient: client,
	})
	snapshot := collector.Snapshot(context.Background())
	if requestedPath != "/api/v1/sbs/cluster" {
		t.Fatalf("requested path = %q, want /api/v1/sbs/cluster", requestedPath)
	}
	if snapshot.Status != "ok" || snapshot.ClusterID != "namrbd-cluster" {
		t.Fatalf("snapshot status/cluster = %q/%q", snapshot.Status, snapshot.ClusterID)
	}
	if snapshot.SourceAuthority != "namrbd_sbs_observability" || snapshot.SourceSchemaVersion != "namrbd.sbs.observability.v1" {
		t.Fatalf("source fields = %q/%q", snapshot.SourceAuthority, snapshot.SourceSchemaVersion)
	}
	if !snapshot.ReadOnly || snapshot.MutationControlsEnabled {
		t.Fatalf("read-only fields = read_only:%v mutation:%v", snapshot.ReadOnly, snapshot.MutationControlsEnabled)
	}
	if snapshot.Nodes.Total != 1 || snapshot.Nodes.Healthy != 1 || len(snapshot.NodeDetails) != 1 {
		t.Fatalf("nodes = %+v details=%+v", snapshot.Nodes, snapshot.NodeDetails)
	}
	if snapshot.Volumes.Total != 1 || snapshot.Volumes.Healthy != 1 || len(snapshot.VolumeDetails) != 1 {
		t.Fatalf("volumes = %+v details=%+v", snapshot.Volumes, snapshot.VolumeDetails)
	}
	if len(snapshot.Stores) != 1 || snapshot.Stores[0].StoreID != "store-a" {
		t.Fatalf("stores = %+v", snapshot.Stores)
	}
	if snapshot.Capacity.TotalBytes != 1000 || snapshot.Capacity.ReclaimableBytes != 80 {
		t.Fatalf("capacity = %+v", snapshot.Capacity)
	}
	if snapshot.Pool.PoolID != "object-pool" || snapshot.Pool.ConfiguredGeneration != 7 || snapshot.Pool.ActiveGeneration != 6 || snapshot.Pool.RefreshErrorCount != 2 {
		t.Fatalf("pool generation/refresh metrics = %+v", snapshot.Pool)
	}
	if snapshot.Pool.MemberCount != 1 || snapshot.Pool.WritableMembers != 1 || snapshot.Pool.AdmissionState != "degraded_writable" || !snapshot.Pool.Stale || snapshot.Pool.StaleDurationSeconds != 11 {
		t.Fatalf("pool admission/stale metrics = %+v", snapshot.Pool)
	}
	if snapshot.Reclaim.Candidates != 2 || snapshot.Reclaim.ReclaimableBytes != 80 {
		t.Fatalf("reclaim = %+v", snapshot.Reclaim)
	}
	if snapshot.NAMRBDSource["secret_token"] != "[REDACTED]" {
		t.Fatalf("namrbd source should redact secret_token: %+v", snapshot.NAMRBDSource)
	}
}

func TestVolumeCapacityObservationsPreserveObservedZeroValues(t *testing.T) {
	snapshot := snapshotFromNAMRBDRaw(Config{}, "2026-08-10T00:00:00Z", map[string]any{
		"volumes": []any{
			map[string]any{
				"volume_id":       "18a00001",
				"state":           "active",
				"available_bytes": float64(0),
				"used_percent":    float64(100),
				"observed_at":     "2026-08-10T00:00:01Z",
			},
			map[string]any{
				"volume_id": "18a00002",
				"state":     "active",
			},
		},
	})
	if len(snapshot.VolumeDetails) != 2 || !snapshot.VolumeDetails[0].CapacityObserved || !snapshot.VolumeDetails[0].AvailableBytesObserved || !snapshot.VolumeDetails[0].UsedPercentObserved {
		t.Fatalf("volume details = %+v", snapshot.VolumeDetails)
	}
	observations := VolumeCapacityObservations(snapshot)
	if len(observations) != 1 {
		t.Fatalf("observations len = %d, want 1: %+v", len(observations), observations)
	}
	if observations[0].AvailableBytes == nil || *observations[0].AvailableBytes != 0 {
		t.Fatalf("available observation = %v, want observed zero", observations[0].AvailableBytes)
	}
	if observations[0].UsedPercent == nil || *observations[0].UsedPercent != 100 {
		t.Fatalf("used observation = %v, want 100", observations[0].UsedPercent)
	}
	if observations[0].ObservedAt != "2026-08-10T00:00:01Z" || observations[0].Source != "namrbd_sbs_observability" {
		t.Fatalf("observation metadata = %+v", observations[0])
	}
}

func TestCollectorSnapshotParsesNAMRBDPhaseYCapacityShape(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(`{
			"schema_version": "namrbd.sbs.observability.v1",
			"generated_at": "2026-08-05T08:11:52Z",
			"collection_status": "ok",
			"cluster_id": "namrbd-example",
			"sbs_cluster_id": "sbs-example",
			"collector_freshness_seconds": 18.2,
			"nodes": [
			  {"node_id": "node-a", "lifecycle": "active", "health": "healthy", "capacity_bytes": 1024, "used_bytes": 256}
			],
			"stores": [
			  {"node_id": "node-a", "store_count": 1, "healthy_store_count": 1, "capacity_bytes": 1024, "available_bytes": 700, "used_bytes": 256}
			],
			"volumes": [
			  {"volume_id": "18a2424a", "status": "healthy", "size_bytes": 1073741824}
			],
			"capacity": {
			  "source": "sbs-service node membership and sbs-data health detail",
			  "logical_bytes": 1073741824,
			  "physical_used_bytes": 256,
			  "physical_free_bytes": 700,
			  "total_bytes": 1024,
			  "unknown_bytes": 68,
			  "store_count": 1,
			  "node_count": 1
			},
			"reclaim": {
			  "source": "sbs-service retired payload backlog",
			  "protected_reference_check_passed": true,
			  "completed_claimed": false,
			  "evidence_required": true
			}
		}`)),
		}, nil
	})}

	collector := NewCollector(Config{
		ClusterID:                        "fallback-cluster",
		NAMRBDSBSObservabilityEndpoint:   "http://namrbd-observability",
		NAMRBDSBSObservabilityTimeout:    time.Second,
		NAMRBDSBSObservabilityHTTPClient: client,
	})
	snapshot := collector.Snapshot(context.Background())
	if snapshot.Status != "ok" || snapshot.ClusterID != "namrbd-example" {
		t.Fatalf("snapshot status/cluster = %q/%q", snapshot.Status, snapshot.ClusterID)
	}
	if snapshot.CollectorFreshnessSeconds != 18.2 {
		t.Fatalf("collector freshness = %v, want NAMRBD-provided 18.2", snapshot.CollectorFreshnessSeconds)
	}
	if snapshot.Nodes.Total != 1 || snapshot.Nodes.Healthy != 1 {
		t.Fatalf("nodes summary = %+v", snapshot.Nodes)
	}
	if len(snapshot.Stores) != 1 || snapshot.Stores[0].StoreID != "node-a" || snapshot.Stores[0].TotalBytes != 1024 || snapshot.Stores[0].AvailableBytes != 700 {
		t.Fatalf("stores = %+v", snapshot.Stores)
	}
	if snapshot.Volumes.Total != 1 || snapshot.Volumes.Healthy != 1 {
		t.Fatalf("volumes summary = %+v", snapshot.Volumes)
	}
	if snapshot.Capacity.LogicalBytes != 1073741824 || snapshot.Capacity.UsedBytes != 256 || snapshot.Capacity.AvailableBytes != 700 || snapshot.Capacity.UnknownBytes != 68 {
		t.Fatalf("capacity = %+v", snapshot.Capacity)
	}
	if snapshot.Capacity.StoresTotal != 1 || snapshot.Capacity.VolumesTotal != 1 {
		t.Fatalf("capacity totals = %+v", snapshot.Capacity)
	}
	if snapshot.Reclaim.Status != "evidence_required" {
		t.Fatalf("reclaim status = %q", snapshot.Reclaim.Status)
	}
}

func TestCollectorSnapshotCachesNAMRBDObservability(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"schema_version":"namrbd.sbs.observability.v1","status":"ok"}`)),
		}, nil
	})}

	collector := NewCollector(Config{
		NAMRBDSBSObservabilityEndpoint:   "http://namrbd-observability",
		NAMRBDSBSObservabilityHTTPClient: client,
	})
	collector.Snapshot(context.Background())
	collector.Snapshot(context.Background())
	if requests != 1 {
		t.Fatalf("NAMRBD requests = %d, want cached single request", requests)
	}
}

func TestCollectorSnapshotFallsBackWhenNAMRBDUnavailable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Body:       io.NopCloser(strings.NewReader("down")),
		}, nil
	})}

	collector := NewCollector(Config{
		AdminEndpoints:                   []string{"sbs-admin-a:9443"},
		VolumeIDs:                        []string{"vol-a"},
		NAMRBDSBSObservabilityEndpoint:   "http://namrbd-observability",
		NAMRBDSBSObservabilityTimeout:    time.Second,
		NAMRBDSBSObservabilityHTTPClient: client,
	})
	snapshot := collector.Snapshot(context.Background())
	if snapshot.Status != "degraded" || snapshot.SourceAuthority != "namrbd_sbs_observability" {
		t.Fatalf("snapshot status/source = %q/%q", snapshot.Status, snapshot.SourceAuthority)
	}
	if snapshot.FirstError == "" || !strings.Contains(snapshot.FirstError, "503") {
		t.Fatalf("first error = %q, want HTTP status detail", snapshot.FirstError)
	}
	if snapshot.Nodes.Total != 1 || snapshot.Volumes.Total != 1 {
		t.Fatalf("fallback node/volume summary = %+v/%+v", snapshot.Nodes, snapshot.Volumes)
	}
	if len(snapshot.Limitations) < 2 {
		t.Fatalf("limitations should include NAMRBD error and fallback note: %+v", snapshot.Limitations)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestConfigFromAppIncludesVolumePoolMembers(t *testing.T) {
	cfg := config.Default()
	cfg.SBSAdminEndpoint = "sbs-admin-a:9443"
	cfg.SBSDataEndpoint = "sbs-data-a:9460"
	cfg.SBSVolumePool = []config.SBSVolumePoolMember{
		{VolumeID: "18a00001", State: "active", Weight: 2, AvailableBytes: 1024, UsedPercent: 25, HighWatermarkPercent: 90},
		{VolumeID: "18a00002", DataEndpoint: "sbs-data-b:9460", State: "degraded"},
	}

	collectorCfg := ConfigFromApp(cfg)
	snapshot := NewCollector(collectorCfg).Snapshot(context.Background())
	if snapshot.Volumes.Total != 2 {
		t.Fatalf("volumes total = %d, want 2", snapshot.Volumes.Total)
	}
	if snapshot.Volumes.Healthy != 1 || snapshot.Volumes.Degraded != 1 || snapshot.Volumes.Unknown != 0 {
		t.Fatalf("volumes summary = %+v, want healthy/degraded/unknown 1/1/0", snapshot.Volumes)
	}
	if snapshot.Pool.MemberCount != 2 || snapshot.Pool.WritableMembers != 1 || snapshot.Pool.DegradedMembers != 1 || snapshot.Pool.AdmissionState != "degraded_writable" {
		t.Fatalf("pool summary = %+v", snapshot.Pool)
	}
	if snapshot.Pool.CapacityObservedCount != 1 || snapshot.Pool.Capacity.AvailableBytes != 1024 || snapshot.Pool.Capacity.UsedPercent != 25 {
		t.Fatalf("pool capacity = %+v", snapshot.Pool)
	}
	if len(snapshot.VolumeDetails) != 2 {
		t.Fatalf("volume details len = %d, want 2", len(snapshot.VolumeDetails))
	}
	if got := snapshot.VolumeDetails[0]; got.VolumeID != "18a00001" || got.State != "active" || got.Weight != 2 || got.AvailableBytes != 1024 || got.UsedPercent != 25 || got.HighWatermarkPercent != 90 {
		t.Fatalf("volume detail[0] = %+v", got)
	}
	if snapshot.Nodes.Total != 3 {
		t.Fatalf("nodes total = %d, want 3", snapshot.Nodes.Total)
	}
}

func TestWritePrometheusDoesNotLeakEndpointLabel(t *testing.T) {
	collector := NewCollector(Config{
		AdminEndpoints: []string{"sbs-admin-a:9443"},
		VolumeIDs:      []string{"18a00001"},
	})
	snapshot := collector.Snapshot(context.Background())
	var buf bytes.Buffer
	if err := WritePrometheus(&buf, snapshot); err != nil {
		t.Fatalf("WritePrometheus() error = %v", err)
	}
	body := buf.String()
	for _, want := range []string{"namros_sbs_node_up", "namros_sbs_volume_state", "namros_sbs_store_state", "namros_sbs_capacity_bytes", "namros_sbs_capacity_used_percent", "namros_sbs_pool_members", "namros_sbs_pool_generation", "namros_sbs_pool_capacity_bytes", "namros_sbs_pool_admission_state", "namros_sbs_pool_refresh_errors", "namros_sbs_pool_stale_seconds", "namros_sbs_reclaim_jobs", "namros_sbs_maintenance_jobs"} {
		if !strings.Contains(body, want) {
			t.Fatalf("prometheus output missing %q\n%s", want, body)
		}
	}
	if !strings.Contains(body, `namros_sbs_pool_admission_state{pool_id="18a00001",state="unknown"} 1`) {
		t.Fatalf("prometheus output missing pool admission state\n%s", body)
	}
	if strings.Contains(body, "endpoint=") {
		t.Fatalf("prometheus output should not label raw endpoints\n%s", body)
	}
}

package coordination

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/config"
)

func TestGatewayConfigFromAppDefaultsIdentityAndAdvertiseEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.CoordinationBackend = config.CoordinationBackendEtcd
	cfg.MetadataBackend = config.MetadataBackendTiKV
	cfg.StorageBackend = config.StorageBackendSBSPhysical

	got := GatewayConfigFromApp(cfg)
	if got.Backend != config.CoordinationBackendEtcd {
		t.Fatalf("Backend = %q", got.Backend)
	}
	if got.AdvertiseEndpoint != cfg.ListenAddr {
		t.Fatalf("AdvertiseEndpoint = %q, want listen addr %q", got.AdvertiseEndpoint, cfg.ListenAddr)
	}
	if got.InstanceID == "" || strings.Contains(got.InstanceID, "/") {
		t.Fatalf("InstanceID = %q", got.InstanceID)
	}
	if got.MetadataBackend != config.MetadataBackendTiKV || got.StorageBackend != config.StorageBackendSBSPhysical {
		t.Fatalf("backends = metadata:%q storage:%q", got.MetadataBackend, got.StorageBackend)
	}
}

func TestGatewayRegistryKeySanitizesInstanceID(t *testing.T) {
	got := GatewayRegistryKey("/namros/gateways/", "host/a")
	if got != "/namros/gateways/host_a" {
		t.Fatalf("GatewayRegistryKey() = %q", got)
	}
}

func TestGatewayRegistryPrefixUsesDefault(t *testing.T) {
	if got := GatewayRegistryPrefix(" /namros/lab/gateways/ "); got != "/namros/lab/gateways" {
		t.Fatalf("GatewayRegistryPrefix() = %q", got)
	}
	if got := GatewayRegistryPrefix(""); got != config.DefaultGatewayRegistryPrefix {
		t.Fatalf("GatewayRegistryPrefix(empty) = %q, want %q", got, config.DefaultGatewayRegistryPrefix)
	}
}

func TestBuildAndEncodeGatewayRecord(t *testing.T) {
	started := time.Unix(100, 0).UTC()
	now := time.Unix(130, 0).UTC()
	record := BuildGatewayRecord(GatewayConfig{
		InstanceID:        "gw-a",
		AdvertiseEndpoint: "10.0.0.1:9000",
		ListenAddr:        "0.0.0.0:9000",
		MetadataBackend:   "tikv",
		StorageBackend:    "sbs-physical",
		StartedAt:         started,
	}, now)
	if !record.Healthy || !record.Ready || record.Status != "ready" {
		t.Fatalf("health fields = healthy:%v ready:%v status:%q", record.Healthy, record.Ready, record.Status)
	}
	if record.StartedAtUnix != 100 || record.LastHeartbeatUnix != 130 {
		t.Fatalf("timestamps = started:%d heartbeat:%d", record.StartedAtUnix, record.LastHeartbeatUnix)
	}
	encoded, err := EncodeGatewayRecord(record)
	if err != nil {
		t.Fatalf("EncodeGatewayRecord() error = %v", err)
	}
	var decoded GatewayRecord
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("encoded record is not JSON: %v", err)
	}
	if decoded.InstanceID != "gw-a" || decoded.AdvertiseEndpoint != "10.0.0.1:9000" {
		t.Fatalf("decoded record = %+v", decoded)
	}
}

func TestLeaseTTLSecondsRoundsUp(t *testing.T) {
	tests := []struct {
		ttl  time.Duration
		want int64
	}{
		{ttl: 500 * time.Millisecond, want: 1},
		{ttl: time.Second, want: 1},
		{ttl: 1500 * time.Millisecond, want: 2},
	}
	for _, tt := range tests {
		if got := leaseTTLSeconds(tt.ttl); got != tt.want {
			t.Fatalf("leaseTTLSeconds(%s) = %d, want %d", tt.ttl, got, tt.want)
		}
	}
}

func TestHealthyGatewayRecordsFiltersAndSorts(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	records := []GatewayRecord{
		{
			InstanceID:        "gw-b",
			AdvertiseEndpoint: "http://gateway-b.example:19091",
			Healthy:           true,
			Ready:             true,
			Status:            "ready",
			LastHeartbeatUnix: 199,
		},
		{
			InstanceID:        "gw-a",
			AdvertiseEndpoint: "http://gateway-a.example:19090",
			Healthy:           true,
			Ready:             true,
			Status:            "ready",
			LastHeartbeatUnix: 198,
		},
		{
			InstanceID:        "gw-unhealthy",
			AdvertiseEndpoint: "http://gateway-c.example:19092",
			Healthy:           false,
			Ready:             true,
			Status:            "ready",
			LastHeartbeatUnix: 199,
		},
		{
			InstanceID:        "gw-not-ready",
			AdvertiseEndpoint: "http://gateway-d.example:19093",
			Healthy:           true,
			Ready:             false,
			Status:            "ready",
			LastHeartbeatUnix: 199,
		},
		{
			InstanceID:        "gw-stale",
			AdvertiseEndpoint: "http://gateway-e.example:19094",
			Healthy:           true,
			Ready:             true,
			Status:            "ready",
			LastHeartbeatUnix: 100,
		},
		{
			InstanceID:        "gw-busy",
			AdvertiseEndpoint: "http://gateway-f.example:19095",
			Healthy:           true,
			Ready:             true,
			Status:            "draining",
			LastHeartbeatUnix: 199,
		},
	}

	healthy := HealthyGatewayRecords(records, now, 10*time.Second)
	if len(healthy) != 2 {
		t.Fatalf("len(healthy) = %d, want 2: %+v", len(healthy), healthy)
	}
	if healthy[0].InstanceID != "gw-a" || healthy[1].InstanceID != "gw-b" {
		t.Fatalf("healthy records not sorted as expected: %+v", healthy)
	}
}

func TestSelectFailoverGatewayExcludesFailedInstance(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	records := []GatewayRecord{
		{
			InstanceID:        "gw-a",
			AdvertiseEndpoint: "http://gateway-a.example:19090",
			Healthy:           true,
			Ready:             true,
			Status:            "ready",
			LastHeartbeatUnix: 199,
		},
		{
			InstanceID:        "gw-b",
			AdvertiseEndpoint: "http://gateway-b.example:19091",
			Healthy:           true,
			Ready:             true,
			Status:            "ready",
			LastHeartbeatUnix: 199,
		},
	}

	selected, ok := SelectFailoverGateway(records, "gw-a", now, 10*time.Second)
	if !ok {
		t.Fatal("SelectFailoverGateway() did not find a replacement")
	}
	if selected.InstanceID != "gw-b" || selected.AdvertiseEndpoint != "http://gateway-b.example:19091" {
		t.Fatalf("selected = %+v", selected)
	}
	if _, ok := SelectFailoverGateway(records[:1], "gw-a", now, 10*time.Second); ok {
		t.Fatal("SelectFailoverGateway() found a replacement when only failed gateway was healthy")
	}
}

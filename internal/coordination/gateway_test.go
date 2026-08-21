package coordination

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
	"time"

	"go.etcd.io/etcd/api/v3/etcdserverpb"
	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"

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
	if got != "/namros/gateways/host_a/status" {
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
	if record.SchemaVersion != 1 || record.Product != "namros" || record.Role != "object" ||
		record.Readiness != "ready" || record.DrainState != "active" {
		t.Fatalf("fleet envelope = %+v", record)
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

func TestGatewayFleetCadenceAndJitter(t *testing.T) {
	if config.DefaultGatewayLeaseTTL != 15*time.Second || config.DefaultGatewayHeartbeat != 5*time.Second {
		t.Fatalf("gateway cadence = %s/%s, want 15s/5s", config.DefaultGatewayLeaseTTL, config.DefaultGatewayHeartbeat)
	}
	rnd := rand.New(rand.NewSource(1))
	for i := 0; i < 1000; i++ {
		got := jitteredHeartbeat(5*time.Second, rnd)
		if got < 4*time.Second || got > 6*time.Second {
			t.Fatalf("jittered heartbeat %s is outside +/-20%%", got)
		}
	}
}

func TestLegacyGatewayRecordDecodesIntoFleetEnvelope(t *testing.T) {
	var record GatewayRecord
	if err := json.Unmarshal([]byte(`{"instance_id":"old-a","advertise_endpoint":"10.0.0.1:9000","healthy":true,"ready":true,"status":"ready","last_heartbeat_unix":10}`), &record); err != nil {
		t.Fatal(err)
	}
	if record.InstanceID != "old-a" || record.Product != "namros" || record.Role != "object" || record.Readiness != "ready" {
		t.Fatalf("legacy normalization = %+v", record)
	}
}

func TestGatewayFleetPageAndWatchCarryRevision(t *testing.T) {
	record := BuildGatewayRecord(GatewayConfig{InstanceID: "gw-a", AdvertiseEndpoint: "10.0.0.1:9000"}, time.Unix(10, 0))
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "/namros/gateways/"
	kv := &mvccpb.KeyValue{Key: []byte(prefix + "gw-a/status"), Value: payload, ModRevision: 40}
	page, err := decodeGatewayFleetPage(prefix, &clientv3.GetResponse{
		Header: &etcdserverpb.ResponseHeader{Revision: 41}, Kvs: []*mvccpb.KeyValue{kv}, More: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Revision != 41 || page.NextCursor != prefix+"gw-a/status" || page.Records[0].RegistryRevision != 40 {
		t.Fatalf("page = %+v", page)
	}
	events, err := decodeGatewayFleetWatch(prefix, clientv3.WatchResponse{
		Header: etcdserverpb.ResponseHeader{Revision: 42},
		Events: []*clientv3.Event{
			{Type: clientv3.EventTypePut, Kv: kv},
			{Type: clientv3.EventTypeDelete, Kv: &mvccpb.KeyValue{Key: []byte(prefix + "gw-b/status"), ModRevision: 42}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].Type != "put" || events[0].Revision != 40 || events[1].Type != "delete" || events[1].Revision != 42 {
		t.Fatalf("events = %+v", events)
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

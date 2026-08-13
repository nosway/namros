package mcpops

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteIncidentBundleIncludesAdminAndOperationsSnapshots(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GatewayEndpoint = ""
	cfg.AdminEndpoint = ""
	cfg.EtcdEndpoints = nil
	cfg.OperationOutputDir = t.TempDir()

	result, err := WriteIncidentBundle(context.Background(), cfg, "quota pressure")
	if err != nil {
		t.Fatalf("WriteIncidentBundle() error = %v", err)
	}
	if result["schema_version"] != "namros.mcp.incident_bundle.v1" || result["status"] != "written" {
		t.Fatalf("bundle result = %+v", result)
	}
	dir, ok := result["path"].(string)
	if !ok || dir == "" {
		t.Fatalf("bundle path = %#v", result["path"])
	}
	for _, name := range []string{"health.json", "admin-status.json", "operations-metrics.json", "worker-backlog.json", "config.json", "tikv-metrics.json", "sbs-volume-pool.json", "alerts.json", "registry.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("bundle file %s missing: %v", name, err)
		}
	}

	for _, name := range []string{"admin-status.json", "operations-metrics.json"} {
		payload, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var snapshot EndpointSnapshot
		if err := json.Unmarshal(payload, &snapshot); err != nil {
			t.Fatalf("decode %s: %v", name, err)
		}
		if snapshot.SchemaVersion != "namros.mcp.endpoint.v1" || snapshot.Status != "disabled" || snapshot.HTTPStatus != 0 {
			t.Fatalf("%s snapshot = %+v", name, snapshot)
		}
	}
}

func TestWriteIncidentBundleRedactsConfigAndExtractsWorkerBacklog(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GatewayEndpoint = "http://namros-gateway.test"
	cfg.AdminEndpoint = "http://namros-gateway.test"
	cfg.EtcdEndpoints = nil
	cfg.OperationOutputDir = t.TempDir()
	cfg.HTTPClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := map[string]string{
			"/healthz":                  `{"status":"ok"}`,
			"/readyz":                   `{"status":"ready"}`,
			"/debug/admin/status":       `{"schema_version":"namros.admin.status.v1","status":"ok"}`,
			"/debug/operations/metrics": `{"schema_version":1,"status":"ok","components":{"worker_backlog":{"enabled":true,"snapshot":{"totals":{"backlog_operations":1},"workers":[{"worker_kind":"gc","owner_id":"gateway-a","last_error":"retry"}]}}}}`,
			"/debug/config":             `{"root_secret_access_key":"super-secret","nested":{"authorization":"Bearer token"},"safe":"value"}`,
			"/debug/tikv/metrics":       `{"enabled":true,"snapshot":{"retry":{"write_conflicts":1}}}`,
			"/debug/sbs/volume-pool":    `{"schema_version":"namros.sbs.volume_pool.runtime.v1","enabled":true}`,
			"/api/v1/alerts":            `{"schema_version":"namros.console.alerts.v1","alerts":[{"id":"NamrosWorkerBacklogHigh"}]}`,
		}[req.URL.Path]
		if body == "" {
			body = `{"error":"not found"}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}
	result, err := WriteIncidentBundle(context.Background(), cfg, "redaction")
	if err != nil {
		t.Fatalf("WriteIncidentBundle() error = %v", err)
	}
	dir := result["path"].(string)
	configPayload, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var configSnapshot EndpointSnapshot
	if err := json.Unmarshal(configPayload, &configSnapshot); err != nil {
		t.Fatalf("decode config.json: %v", err)
	}
	body := configSnapshot.Body.(map[string]any)
	if body["root_secret_access_key"] != redacted {
		t.Fatalf("root secret was not redacted: %+v", body)
	}
	nested := body["nested"].(map[string]any)
	if nested["authorization"] != redacted || body["safe"] != "value" {
		t.Fatalf("config redaction body = %+v", body)
	}

	workerPayload, err := os.ReadFile(filepath.Join(dir, "worker-backlog.json"))
	if err != nil {
		t.Fatalf("read worker-backlog.json: %v", err)
	}
	var worker map[string]any
	if err := json.Unmarshal(workerPayload, &worker); err != nil {
		t.Fatalf("decode worker-backlog.json: %v", err)
	}
	if worker["schema_version"] != "namros.mcp.operations_component.v1" || worker["status"] != "ok" {
		t.Fatalf("worker backlog component = %+v", worker)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

package mcpops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"

	"github.com/nosway/namros/internal/coordination"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/version"
)

const maxEndpointBodyBytes = 1024 * 1024

type EndpointSnapshot struct {
	SchemaVersion string `json:"schema_version"`
	Endpoint      string `json:"endpoint"`
	Path          string `json:"path"`
	Status        string `json:"status"`
	HTTPStatus    int    `json:"http_status,omitempty"`
	Body          any    `json:"body,omitempty"`
	Error         string `json:"error,omitempty"`
}

type HealthCheckOutput struct {
	SchemaVersion     string            `json:"schema_version"`
	GeneratedAt       string            `json:"generated_at"`
	Status            string            `json:"status"`
	Edition           ProductEdition    `json:"edition"`
	Version           map[string]string `json:"version"`
	GatewayHealth     EndpointSnapshot  `json:"gateway_health"`
	GatewayReadiness  EndpointSnapshot  `json:"gateway_readiness"`
	AdminStatus       EndpointSnapshot  `json:"admin_status"`
	OperationsMetrics EndpointSnapshot  `json:"operations_metrics"`
	LatestRelease     ArtifactSummary   `json:"latest_release"`
}

type ProductEdition struct {
	Name     string            `json:"name"`
	Features []edition.Feature `json:"features"`
}

func BuildHealthCheck(ctx context.Context, cfg Config) HealthCheckOutput {
	cfg = cfg.Normalized()
	client := httpClient(cfg)
	health := FetchEndpoint(ctx, client, cfg.GatewayEndpoint, "/healthz")
	ready := FetchEndpoint(ctx, client, cfg.GatewayEndpoint, "/readyz")
	admin := FetchEndpoint(ctx, client, cfg.AdminEndpoint, "/debug/admin/status?count_limit=1000&recent_dedupe_limit=5&recent_gc_limit=5")
	metrics := FetchEndpoint(ctx, client, cfg.AdminEndpoint, "/debug/operations/metrics")
	status := "ok"
	for _, snapshot := range []EndpointSnapshot{health, ready, admin, metrics} {
		if snapshot.Status != "ok" {
			status = "degraded"
			break
		}
	}
	return HealthCheckOutput{
		SchemaVersion:     "namros.mcp.health.v1",
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		Status:            status,
		Edition:           CurrentProductEdition(),
		Version:           version.Info(),
		GatewayHealth:     health,
		GatewayReadiness:  ready,
		AdminStatus:       admin,
		OperationsMetrics: metrics,
		LatestRelease:     LatestArtifact("release", cfg.ReleaseReportDir),
	}
}

func httpClient(cfg Config) *http.Client {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	return &http.Client{Timeout: cfg.HTTPTimeout}
}

func CurrentProductEdition() ProductEdition {
	name := edition.Current()
	return ProductEdition{
		Name:     name,
		Features: edition.FeaturesFor(name),
	}
}

func FetchEndpoint(ctx context.Context, client *http.Client, endpoint, path string) EndpointSnapshot {
	out := EndpointSnapshot{
		SchemaVersion: "namros.mcp.endpoint.v1",
		Endpoint:      endpoint,
		Path:          path,
	}
	if strings.TrimSpace(endpoint) == "" {
		out.Status = "disabled"
		out.Error = "endpoint is not configured"
		return out
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+path, nil)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	resp, err := client.Do(req)
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	defer resp.Body.Close()
	out.HTTPStatus = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEndpointBodyBytes))
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	if len(body) > 0 {
		out.Body = Redact(decodeJSONBody(body))
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		out.Status = "ok"
	} else {
		out.Status = "error"
		out.Error = fmt.Sprintf("http status %d", resp.StatusCode)
	}
	return out
}

func decodeJSONBody(body []byte) any {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err == nil {
		return decoded
	}
	return map[string]any{"text": string(body)}
}

type GatewayRegistryOutput struct {
	SchemaVersion string                  `json:"schema_version"`
	GeneratedAt   string                  `json:"generated_at"`
	Enabled       bool                    `json:"enabled"`
	Root          string                  `json:"root"`
	Endpoints     []string                `json:"endpoints"`
	Status        string                  `json:"status"`
	Records       []GatewayRegistryRecord `json:"records,omitempty"`
	Error         string                  `json:"error,omitempty"`
}

type GatewayRegistryRecord struct {
	Key      string                      `json:"key"`
	LeaseID  int64                       `json:"lease_id,omitempty"`
	Decoded  *coordination.GatewayRecord `json:"decoded,omitempty"`
	RawValue string                      `json:"raw_value,omitempty"`
}

func ListGatewayRegistry(ctx context.Context, cfg Config) GatewayRegistryOutput {
	cfg = cfg.Normalized()
	out := GatewayRegistryOutput{
		SchemaVersion: "namros.mcp.gateway_registry.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Root:          cfg.EtcdRoot,
		Endpoints:     append([]string(nil), cfg.EtcdEndpoints...),
	}
	if len(cfg.EtcdEndpoints) == 0 {
		out.Status = "disabled"
		return out
	}
	out.Enabled = true
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.EtcdEndpoints,
		DialTimeout: cfg.HTTPTimeout,
	})
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	defer client.Close()
	getCtx, cancel := context.WithTimeout(ctx, cfg.HTTPTimeout)
	defer cancel()
	resp, err := client.Get(getCtx, cfg.EtcdRoot, clientv3.WithPrefix())
	if err != nil {
		out.Status = "error"
		out.Error = err.Error()
		return out
	}
	out.Status = "ok"
	out.Records = make([]GatewayRegistryRecord, 0, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		record := GatewayRegistryRecord{
			Key:      string(kv.Key),
			LeaseID:  kv.Lease,
			RawValue: string(kv.Value),
		}
		var decoded coordination.GatewayRecord
		if err := json.Unmarshal(kv.Value, &decoded); err == nil {
			record.Decoded = &decoded
			record.RawValue = ""
		}
		out.Records = append(out.Records, record)
	}
	return out
}

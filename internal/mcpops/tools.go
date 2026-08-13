package mcpops

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/iam"
	"github.com/nosway/namros/internal/version"
)

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

func Resources() []ResourceDefinition {
	return []ResourceDefinition{
		{URI: "namros://product/edition", Name: "Product edition", Description: "Current NAMROS edition, build version, and feature catalog.", MimeType: "application/json"},
		{URI: "namros://cluster/summary", Name: "Cluster summary", Description: "Gateway health, admin status, operations metrics, and latest release artifact.", MimeType: "application/json"},
		{URI: "namros://gateway/health", Name: "Gateway health", Description: "Gateway /healthz and /readyz snapshots.", MimeType: "application/json"},
		{URI: "namros://gateway/registry", Name: "Gateway registry", Description: "etcd gateway registry records and lease ids.", MimeType: "application/json"},
		{URI: "namros://metadata/status", Name: "Metadata status", Description: "Gateway admin metadata status endpoint snapshot.", MimeType: "application/json"},
		{URI: "namros://operations/metrics", Name: "Operations metrics", Description: "Aggregated gateway operations metrics endpoint snapshot.", MimeType: "application/json"},
		{URI: "namros://compat/latest", Name: "Latest compatibility report", Description: "Newest compatibility report artifact preview.", MimeType: "application/json"},
		{URI: "namros://release/latest", Name: "Latest release report", Description: "Newest release-readiness report artifact preview.", MimeType: "application/json"},
		{URI: "namros://chaos/latest", Name: "Latest chaos/soak report", Description: "Newest multi-node chaos/soak report artifact preview when Advanced Ops is available.", MimeType: "application/json"},
		{URI: "namros://runbooks/index", Name: "Runbook index", Description: "Maintained NAMROS operational runbooks.", MimeType: "application/json"},
	}
}

func Tools() []ToolDefinition {
	return []ToolDefinition{
		{Name: "namros.health.check", Description: "Collect gateway health, readiness, admin status, operations metrics, edition, and latest release artifact.", InputSchema: emptySchema()},
		{Name: "namros.admin.status", Description: "Fetch gateway /debug/admin/status and redact sensitive fields.", InputSchema: emptySchema()},
		{Name: "namros.operations.metrics", Description: "Fetch gateway /debug/operations/metrics and redact sensitive fields.", InputSchema: emptySchema()},
		{Name: "namros.compat.latest", Description: "Return the latest compatibility report artifact preview.", InputSchema: emptySchema()},
		{Name: "namros.release.latest", Description: "Return the latest release-readiness report artifact preview.", InputSchema: emptySchema()},
		{Name: "namros.runbook.suggest", Description: "Map an observed failure signal to NAMROS runbooks and safe next commands.", InputSchema: objectSchema(map[string]any{"signal": stringSchema("Observed failure signal or incident summary.")}, []string{"signal"})},
		{Name: "namros.incident.bundle", Description: "Write a local redacted incident snapshot bundle.", InputSchema: objectSchema(map[string]any{"label": stringSchema("Optional incident label used in the output directory name.")}, nil)},
		{Name: "namros.gateway.registry.list", Description: "Inspect etcd gateway registry records and lease ids.", InputSchema: emptySchema()},
		{Name: "namros.active_active.check", Description: "Summarize gateway health, registry records, and latest active-active evidence.", InputSchema: emptySchema()},
		{Name: "namros.tikv.status", Description: "Summarize TiKV metadata metrics and admin metadata posture from gateway debug endpoints.", InputSchema: emptySchema()},
		{Name: "namros.sbs_replicated.status", Description: "Summarize SBS replicated storage posture from Community-visible admin status.", InputSchema: emptySchema()},
		{Name: "namros.console.dashboard.summary", Description: "Produce the normalized status payload used by the web console dashboard.", InputSchema: emptySchema()},
		{Name: "namros.console.report.index", Description: "List release and compatibility report artifacts visible to the current edition.", InputSchema: emptySchema()},
		{Name: "namros.compat.user_space.run", Description: "Plan an operator-approved user-space compatibility smoke run.", InputSchema: approvalSchema()},
		{Name: "namros.metadata.backup.create", Description: "Plan an operator-approved metadata backup artifact creation.", InputSchema: approvalSchema()},
		{Name: "namros.release.readiness.run", Description: "Plan an operator-approved release-readiness target run.", InputSchema: approvalSchema()},
		{Name: "namros.gateway.health.wait", Description: "Plan an operator-approved gateway readiness wait.", InputSchema: approvalSchema()},
		{Name: "namros.lifecycle.plan", Description: "Plan a non-mutating lifecycle/GC planning pass.", InputSchema: approvalSchema()},
		{Name: "namros.etcd_registry.smoke.run", Description: "Plan an operator-approved etcd gateway registry smoke run.", InputSchema: approvalSchema()},
		{Name: "namros.active_active.smoke.run", Description: "Plan an operator-approved active-active smoke run.", InputSchema: approvalSchema()},
		{Name: "namros.sbs_cluster.status", Description: "Enterprise-only SBS cluster and EC route status.", InputSchema: emptySchema()},
		{Name: "namros.dedupe.status", Description: "Enterprise-only dedupe scheduler, worker, and scrub status.", InputSchema: emptySchema()},
		{Name: "namros.compliance.evidence.preview", Description: "Enterprise-only non-mutating compliance evidence preview.", InputSchema: emptySchema()},
		{Name: "namros.kms.status", Description: "Enterprise-only KMS key-state summary without secret material.", InputSchema: emptySchema()},
		{Name: "namros.iam.mapping.preview", Description: "Enterprise-only external IAM mapping preview.", InputSchema: emptySchema()},
		{Name: "namros.chaos.report.latest", Description: "Enterprise-only latest multi-node chaos/soak report summary.", InputSchema: emptySchema()},
		{Name: "namros.chaos_soak.latest", Description: "Enterprise-only latest multi-node chaos/soak report summary legacy alias.", InputSchema: emptySchema()},
		{Name: "namros.gc.retry", Description: "Enterprise-only approved orphan GC retry operation.", InputSchema: approvalSchema()},
		{Name: "namros.dedupe.scrub.run", Description: "Enterprise-only approved dedupe scrub/repair report operation.", InputSchema: approvalSchema()},
		{Name: "namros.dedupe.ack", Description: "Enterprise-only approved dedupe candidate acknowledgement.", InputSchema: approvalSchema()},
		{Name: "namros.compliance.evidence.create", Description: "Enterprise-only approved compliance evidence package creation.", InputSchema: approvalSchema()},
		{Name: "namros.metadata.import.plan", Description: "Enterprise-only metadata import plan against a target backend.", InputSchema: approvalSchema()},
		{Name: "namros.metadata.import.apply", Description: "Enterprise-only approved metadata import apply operation.", InputSchema: approvalSchema()},
		{Name: "namros.sbs_cluster_ec.smoke.run", Description: "Enterprise-only approved SBS EC multipart smoke operation.", InputSchema: approvalSchema()},
		{Name: "namros.kms.encryption.smoke.run", Description: "Enterprise-only approved SSE-KMS encryption smoke operation.", InputSchema: approvalSchema()},
		{Name: "namros.multi_node.soak.run", Description: "Enterprise-only approved multi-node soak operation.", InputSchema: approvalSchema()},
	}
}

func ReadResource(ctx context.Context, cfg Config, uri string) (any, error) {
	client := defaultHTTPClient(cfg)
	switch uri {
	case "namros://product/edition":
		return map[string]any{
			"schema_version": "namros.mcp.edition.v1",
			"edition":        CurrentProductEdition(),
			"version":        VersionInfo(),
		}, nil
	case "namros://cluster/summary":
		return BuildHealthCheck(ctx, cfg), nil
	case "namros://gateway/health":
		return map[string]any{
			"schema_version": "namros.mcp.gateway_health.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"health":         FetchEndpoint(ctx, client, cfg.Normalized().GatewayEndpoint, "/healthz"),
			"readiness":      FetchEndpoint(ctx, client, cfg.Normalized().GatewayEndpoint, "/readyz"),
		}, nil
	case "namros://gateway/registry":
		return ListGatewayRegistry(ctx, cfg), nil
	case "namros://metadata/status":
		return FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/admin/status?count_limit=1000&recent_dedupe_limit=5&recent_gc_limit=5"), nil
	case "namros://operations/metrics":
		return FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/operations/metrics"), nil
	case "namros://compat/latest":
		return LatestArtifact("compatibility", cfg.CompatReportDir), nil
	case "namros://release/latest":
		return LatestArtifact("release", cfg.ReleaseReportDir), nil
	case "namros://chaos/latest":
		if !edition.Allows(edition.Current(), edition.FeatureAdvancedOps) {
			return EnterpriseRequired("namros.chaos.report.latest", edition.FeatureAdvancedOps), nil
		}
		return LatestArtifact("chaos_soak", cfg.ChaosReportDir), nil
	case "namros://runbooks/index":
		return RunbookIndex(), nil
	default:
		return nil, ErrUnknownResource(uri)
	}
}

func CallTool(ctx context.Context, cfg Config, name string, args map[string]any) (any, error) {
	client := defaultHTTPClient(cfg)
	switch name {
	case "namros.health.check", "namros.console.dashboard.summary":
		return BuildHealthCheck(ctx, cfg), nil
	case "namros.admin.status":
		return FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/admin/status?count_limit=1000&recent_dedupe_limit=5&recent_gc_limit=5"), nil
	case "namros.operations.metrics":
		return FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/operations/metrics"), nil
	case "namros.compat.latest":
		return LatestArtifact("compatibility", cfg.CompatReportDir), nil
	case "namros.release.latest":
		return LatestArtifact("release", cfg.ReleaseReportDir), nil
	case "namros.chaos.report.latest", "namros.chaos_soak.latest":
		if !edition.Allows(edition.Current(), edition.FeatureAdvancedOps) {
			return EnterpriseRequired(name, edition.FeatureAdvancedOps), nil
		}
		return LatestArtifact("chaos_soak", cfg.ChaosReportDir), nil
	case "namros.console.report.index":
		return BuildReportIndex(cfg), nil
	case "namros.runbook.suggest":
		return map[string]any{
			"schema_version": "namros.mcp.runbook_suggestion.v1",
			"suggestions":    SuggestRunbooks(stringArg(args, "signal")),
		}, nil
	case "namros.incident.bundle":
		return WriteIncidentBundle(ctx, cfg, stringArg(args, "label"))
	case "namros.gateway.registry.list":
		return ListGatewayRegistry(ctx, cfg), nil
	case "namros.active_active.check":
		return map[string]any{
			"schema_version": "namros.mcp.active_active_check.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"health":         BuildHealthCheck(ctx, cfg),
			"registry":       ListGatewayRegistry(ctx, cfg),
			"latest_release": LatestArtifact("release", cfg.ReleaseReportDir),
		}, nil
	case "namros.tikv.status":
		return map[string]any{
			"schema_version": "namros.mcp.tikv_status.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"metrics":        FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/tikv/metrics"),
			"admin_status":   FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/admin/status?count_limit=1000&recent_dedupe_limit=0&recent_gc_limit=0"),
		}, nil
	case "namros.sbs_replicated.status":
		return map[string]any{
			"schema_version": "namros.mcp.sbs_replicated_status.v1",
			"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
			"admin_status":   FetchEndpoint(ctx, client, cfg.Normalized().AdminEndpoint, "/debug/admin/status?count_limit=1000&recent_dedupe_limit=0&recent_gc_limit=0"),
			"scope":          "community_replicated_only",
			"limitations":    []string{"SBS replicated readiness is inferred from Community-visible gateway/admin surfaces until a stable SBS status endpoint is available."},
		}, nil
	case "namros.multi_node.soak.run":
		envelope := BuildOperationPlan(cfg, name, RiskProbe, edition.Enterprise, map[string]any{
			"summary":           "Create or execute a multi-node chaos/soak report using the configured topology and workload targets.",
			"command":           "make chaos-soak-report",
			"execute_with":      "NAMROS_CHAOS_SOAK_EXECUTE=1 make chaos-soak-report",
			"report_directory":  cfg.Normalized().ChaosReportDir,
			"default_plan_mode": true,
		}, stringArg(args, "approval_reference"))
		return WriteLocalOperationRecord(cfg, envelope)
	}
	if op, ok := communityOperationTools[name]; ok {
		envelope := BuildOperationPlan(cfg, name, op.RiskClass, edition.Community, map[string]any{
			"summary":      op.Summary,
			"command":      op.Command,
			"dry_run_only": true,
		}, stringArg(args, "approval_reference"))
		return WriteLocalOperationRecord(cfg, envelope)
	}
	if featureID, ok := enterpriseOnlyTools[name]; ok {
		return EnterpriseRequired(name, featureID), nil
	}
	return nil, ErrUnknownTool(name)
}

type operationToolInfo struct {
	RiskClass string
	Summary   string
	Command   string
}

var communityOperationTools = map[string]operationToolInfo{
	"namros.compat.user_space.run":   {RiskClass: RiskProbe, Summary: "Run S3 client smoke against a configured endpoint.", Command: "make container-local-smoke"},
	"namros.metadata.backup.create":  {RiskClass: RiskProtect, Summary: "Create a metadata export artifact and summarize collection counts.", Command: "namros-admin metadata-export"},
	"namros.release.readiness.run":   {RiskClass: RiskProbe, Summary: "Run the configured Community release target set.", Command: "make community-release-check"},
	"namros.gateway.health.wait":     {RiskClass: RiskProbe, Summary: "Wait for gateway readiness and capture failure evidence on timeout.", Command: "curl -fsS /readyz with timeout"},
	"namros.lifecycle.plan":          {RiskClass: RiskProbe, Summary: "Run a non-mutating lifecycle/GC planning pass where supported.", Command: "internal lifecycle planner"},
	"namros.etcd_registry.smoke.run": {RiskClass: RiskProbe, Summary: "Run the etcd gateway registry smoke against configured endpoints.", Command: "make smoke-etcd-registry"},
	"namros.active_active.smoke.run": {RiskClass: RiskProbe, Summary: "Run active-active smoke against Community gateways.", Command: "make smoke-active-active"},
}

var enterpriseOnlyTools = map[string]string{
	"namros.sbs_cluster.status":          edition.FeatureErasureCoding,
	"namros.dedupe.status":               edition.FeatureDedupe,
	"namros.compliance.evidence.preview": edition.FeatureComplianceEvidence,
	"namros.kms.status":                  edition.FeatureSSEKMS,
	"namros.iam.mapping.preview":         edition.FeatureExternalIAMFederation,
	"namros.chaos_soak.latest":           edition.FeatureAdvancedOps,
	"namros.gc.retry":                    edition.FeatureAdvancedOps,
	"namros.dedupe.scrub.run":            edition.FeatureDedupe,
	"namros.dedupe.ack":                  edition.FeatureDedupe,
	"namros.compliance.evidence.create":  edition.FeatureComplianceEvidence,
	"namros.metadata.import.plan":        edition.FeatureAdvancedOps,
	"namros.metadata.import.apply":       edition.FeatureAdvancedOps,
	"namros.sbs_cluster_ec.smoke.run":    edition.FeatureErasureCoding,
	"namros.kms.encryption.smoke.run":    edition.FeatureSSEKMS,
	"namros.multi_node.soak.run":         edition.FeatureAdvancedOps,
}

func WriteIncidentBundle(ctx context.Context, cfg Config, label string) (map[string]any, error) {
	cfg = cfg.Normalized()
	dirName := "incident-" + time.Now().UTC().Format("20060102T150405Z")
	if label != "" {
		dirName += "-" + sanitizeToolName(label)
	}
	outDir := filepath.Join(cfg.OperationOutputDir, dirName)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return nil, err
	}
	health := BuildHealthCheck(ctx, cfg)
	client := httpClient(cfg)
	files := map[string]any{
		"health.json":             health,
		"admin-status.json":       health.AdminStatus,
		"operations-metrics.json": health.OperationsMetrics,
		"worker-backlog.json":     OperationsComponentSnapshot(health.OperationsMetrics, "worker_backlog"),
		"config.json":             FetchEndpoint(ctx, client, cfg.AdminEndpoint, "/debug/config"),
		"tikv-metrics.json":       FetchEndpoint(ctx, client, cfg.AdminEndpoint, "/debug/tikv/metrics"),
		"sbs-volume-pool.json":    FetchEndpoint(ctx, client, cfg.AdminEndpoint, "/debug/sbs/volume-pool"),
		"alerts.json":             FetchEndpoint(ctx, client, cfg.AdminEndpoint, "/api/v1/alerts"),
		"iam.json":                BuildIAMEvidence(),
		"runbooks.json":           RunbookIndex(),
		"reports.json":            BuildReportIndex(cfg),
		"registry.json":           ListGatewayRegistry(ctx, cfg),
	}
	written := make([]string, 0, len(files))
	for name, value := range files {
		path := filepath.Join(outDir, name)
		payload, err := json.MarshalIndent(Redact(value), "", "  ")
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	return map[string]any{
		"schema_version": "namros.mcp.incident_bundle.v1",
		"generated_at":   time.Now().UTC().Format(time.RFC3339Nano),
		"status":         "written",
		"path":           outDir,
		"files":          written,
	}, nil
}

func OperationsComponentSnapshot(snapshot EndpointSnapshot, component string) map[string]any {
	out := map[string]any{
		"schema_version": "namros.mcp.operations_component.v1",
		"component":      component,
		"source_status":  snapshot.Status,
		"status":         "missing",
	}
	body, ok := snapshot.Body.(map[string]any)
	if !ok {
		return out
	}
	components, ok := body["components"].(map[string]any)
	if !ok {
		return out
	}
	value, ok := components[component]
	if !ok {
		return out
	}
	out["status"] = "ok"
	out["snapshot"] = value
	return out
}

func BuildIAMEvidence() map[string]any {
	principal := auth.Principal{
		TenantID:      "root",
		DisplayName:   "bootstrap-root",
		Root:          true,
		PolicyVersion: "community-basic",
	}
	simulation, err := iam.SimulatePolicy(iam.PolicySimulationRequest{
		Principal: principal,
		Action:    "s3:GetObject",
		Resource:  "arn:aws:s3:::example-bucket/example-key",
	})
	var simulationValue any = simulation
	if err != nil {
		simulationValue = map[string]any{"error": err.Error()}
	}
	return map[string]any{
		"schema_version":           "namros.mcp.iam_evidence.v1",
		"principal_inspect":        iam.InspectPrincipal(principal),
		"policy_decision_evidence": simulationValue,
		"external_iam_federation":  EnterpriseRequired("namros.iam.mapping.preview", edition.FeatureExternalIAMFederation),
		"object_payload_included":  false,
		"secret_material_included": false,
	}
}

func VersionInfo() map[string]string {
	return version.Info()
}

func defaultHTTPClient(cfg Config) *http.Client {
	return httpClient(cfg.Normalized())
}

func stringArg(args map[string]any, name string) string {
	if args == nil {
		return ""
	}
	if value, ok := args[name].(string); ok {
		return value
	}
	return ""
}

func emptySchema() map[string]any {
	return objectSchema(nil, nil)
}

func approvalSchema() map[string]any {
	return objectSchema(map[string]any{
		"approval_reference": stringSchema("Optional external approval or ticket reference."),
	}, nil)
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	if properties == nil {
		properties = map[string]any{}
	}
	out := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

func stringSchema(description string) map[string]any {
	return map[string]any{
		"type":        "string",
		"description": description,
	}
}

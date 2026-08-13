package gateway

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/adminstatus"
	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/mcpops"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/opsalerts"
	"github.com/nosway/namros/internal/sbsops"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/version"
)

func registerConsoleAPI(router *gin.Engine, cfg config.Config, deps Dependencies) {
	api := router.Group("/api/v1")
	registerConsoleAuthAPI(api, cfg, deps.ConsoleAuth)
	protected := api.Group("", consoleAuthGuard(deps.ConsoleAuth))
	protected.GET("/edition", consoleEdition(cfg))
	protected.GET("/status", consoleStatus(cfg, deps))
	protected.GET("/metrics", consoleMetrics(cfg, deps))
	protected.GET("/reports", consoleReports(cfg))
	protected.GET("/operations", consoleOperations(cfg))
	protected.GET("/operations/summary", consoleOperationsSummary(cfg, deps))
	protected.GET("/operations/warnings", consoleOperationsWarnings(cfg, deps))
	protected.POST("/operations/:name/plan", consoleOperationPlan(cfg))
	protected.GET("/query/views", consoleQueryViews(cfg))
	protected.GET("/gui/summary", consoleGUISummary(cfg))
	protected.GET("/workflow/hardening", consoleWorkflowHardening(cfg))
	protected.GET("/runbooks", consoleRunbooks())
	protected.GET("/alerts", consoleAlerts(cfg, deps))
	protected.GET("/observability/datasources", consoleObservabilityDatasources(cfg))
	protected.GET("/notification/channels", consoleNotificationChannels())
	protected.GET("/notification/routes", consoleNotificationRoutes())
	protected.POST("/notification/test", consoleNotificationTest())
	protected.GET("/sbs/cluster", consoleSBSCluster(deps))
	protected.GET("/sbs/nodes", consoleSBSNodes(deps))
	protected.GET("/sbs/stores", consoleSBSStores(deps))
	protected.GET("/sbs/volumes", consoleSBSVolumes(deps))
	protected.GET("/sbs/capacity", consoleSBSCapacity(deps))
	protected.GET("/sbs/reclaim", consoleSBSReclaim(deps))
	protected.GET("/sbs/maintenance", consoleSBSMaintenance(deps))
	protected.GET("/sbs/volume-pool", consoleSBSVolumePool(deps))
	protected.GET("/object-explorer/buckets", consoleObjectExplorerBuckets(deps))
	protected.GET("/object-explorer/objects", consoleObjectExplorerObjects(deps))
	protected.GET("/object-explorer/object", consoleObjectExplorerObject(deps))
	protected.GET("/object-explorer/external-clients", consoleObjectExplorerExternalClients())
	protected.GET("/admin/users", consoleAdminUsers(deps.ConsoleAuth))
	protected.GET("/admin/groups", consoleAdminGroups(deps.ConsoleAuth))
	protected.GET("/admin/roles", consoleAdminRoles())
}

func consoleReadOnlyEnvelope(schemaVersion, generatedAt, status, sourceAuthority string, limitations []string) gin.H {
	if generatedAt == "" {
		generatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if status == "" {
		status = "ok"
	}
	if sourceAuthority == "" {
		sourceAuthority = "namros_console"
	}
	if limitations == nil {
		limitations = []string{}
	}
	return gin.H{
		"schema_version":            schemaVersion,
		"generated_at":              generatedAt,
		"status":                    status,
		"read_only":                 true,
		"operation_surface":         "read_only",
		"source_authority":          sourceAuthority,
		"limitations":               limitations,
		"warning_count":             len(limitations),
		"rbac_checked":              true,
		"redaction_applied":         true,
		"unsupported_claim_visible": true,
		"mutation_controls_enabled": false,
		"secret_redaction":          defaultConsoleRedactionPolicy(),
	}
}

func consoleMerge(base gin.H, fields gin.H) gin.H {
	for key, value := range fields {
		base[key] = value
	}
	return base
}

func consoleSBSEnvelope(schemaVersion string, snapshot sbsops.Snapshot) gin.H {
	base := consoleReadOnlyEnvelope(schemaVersion, snapshot.GeneratedAt, snapshot.Status, snapshot.SourceAuthority, snapshot.Limitations)
	base["source_schema_version"] = snapshot.SourceSchemaVersion
	base["collector_freshness_seconds"] = snapshot.CollectorFreshnessSeconds
	base["warning_count"] = snapshot.WarningCount
	base["first_error"] = snapshot.FirstError
	base["last_error"] = snapshot.LastError
	base["rbac_checked"] = snapshot.RBACChecked
	base["redaction_applied"] = snapshot.RedactionApplied
	base["unsupported_claim_visible"] = snapshot.UnsupportedClaimVisible
	base["mutation_controls_enabled"] = snapshot.MutationControlsEnabled
	return base
}

func consoleEdition(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.edition.v1", "", "ok", "namros_console", nil), gin.H{
			"edition": adminstatus.EditionFromConfig(cfg),
			"version": version.Info(),
		}))
	}
}

func consoleStatus(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Metadata == nil {
			c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.status.v1", "", "degraded", "namros_console", []string{"metadata repository is not configured"}), gin.H{
				"metadata": gin.H{"enabled": false},
			}))
			return
		}
		status, err := adminstatus.Build(c.Request.Context(), cfg, deps.Metadata, adminstatus.Request{
			CountLimit:        debugNamedLimit(c, "count_limit", 1000, 10000),
			RecentDedupeLimit: debugNamedLimit(c, "recent_dedupe_limit", 5, 100),
			RecentGCLimit:     debugNamedLimit(c, "recent_gc_limit", 5, 100),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, consoleMerge(consoleReadOnlyEnvelope("namros.console.status.v1", "", "error", "namros_console", []string{"admin status collection failed"}), gin.H{
				"error": err.Error(),
			}))
			return
		}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.status.v1", "", status.Status, "namros_console", nil), gin.H{
			"gateway":           gin.H{"health": "ok", "readiness": "ready"},
			"admin_status":      status,
			"dashboard":         consoleDashboardSummary(cfg, deps, status.Status, status.Counts),
			"alerts":            consoleAlertList(cfg, deps, status.Status),
			"secret_redaction":  defaultConsoleRedactionPolicy(),
			"operation_surface": "read_only",
		}))
	}
}

func consoleMetrics(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		operations := buildOperationsMetrics(c.Request.Context(), deps)
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.metrics.v1", "", operations.Status, "namros_console", nil), gin.H{
			"operations_metrics": operations,
			"prometheus": gin.H{
				"scrape_path":  "/metrics",
				"content_type": "text/plain; version=0.0.4",
			},
			"alerts": consoleAlertList(cfg, deps, operations.Status),
			"status": operations.Status,
		}))
	}
}

func consoleReports(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.reports.v1", "", "ok", "namros_console", nil), gin.H{
			"reports":           consoleReportCatalog(cfg),
			"secret_redaction":  defaultConsoleRedactionPolicy(),
			"operation_surface": "read_only",
		}))
	}
}

func consoleReportCatalog(cfg config.Config) []gin.H {
	mcpCfg := mcpops.DefaultConfig()
	reports := []gin.H{
		{
			"kind":            "compatibility",
			"minimum_edition": edition.Community,
			"artifact":        mcpops.LatestArtifact("compatibility", mcpCfg.CompatReportDir),
		},
		{
			"kind":            "release",
			"minimum_edition": edition.Community,
			"artifact":        mcpops.LatestArtifact("release", mcpCfg.ReleaseReportDir),
		},
		{
			"kind":            "operations_daily",
			"minimum_edition": edition.Community,
			"artifact":        mcpops.LatestArtifact("operations_daily", "ops-reports"),
		},
	}
	if edition.Allows(cfg.Edition, edition.FeatureAdvancedOps) {
		reports = append(reports, gin.H{
			"kind":            "chaos_soak",
			"minimum_edition": edition.Enterprise,
			"artifact":        mcpops.LatestArtifact("chaos_soak", mcpCfg.ChaosReportDir),
		})
	} else {
		reports = append(reports, gin.H{
			"kind":                "chaos_soak",
			"minimum_edition":     edition.Enterprise,
			"enterprise_required": mcpops.EnterpriseRequired("namros.chaos.report.latest", edition.FeatureAdvancedOps),
		})
	}
	if edition.Allows(cfg.Edition, edition.FeatureComplianceEvidence) {
		reports = append(reports, gin.H{
			"kind":            "compliance_evidence",
			"minimum_edition": edition.Enterprise,
			"artifact":        mcpops.LatestArtifact("compliance_evidence", "compliance-reports"),
		})
	} else {
		reports = append(reports, gin.H{
			"kind":                "compliance_evidence",
			"minimum_edition":     edition.Enterprise,
			"enterprise_required": mcpops.EnterpriseRequired("namros.compliance.evidence.preview", edition.FeatureComplianceEvidence),
		})
	}
	return reports
}

func consoleOperations(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.operations.v1", "", "ok", "namros_console", nil), gin.H{
			"operations":        consoleOperationCatalog(cfg),
			"history":           consoleOperationHistory(mcpops.DefaultConfig().OperationOutputDir),
			"evidence_bundles":  consoleEvidenceBundles(mcpops.DefaultConfig().OperationOutputDir),
			"approval_contract": "plan_preflight_apply_verify_audit",
			"secret_redaction":  defaultConsoleRedactionPolicy(),
		}))
	}
}

func consoleOperationsSummary(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		statusText := "degraded"
		limitations := []string{}
		counts := adminstatus.Counts{}
		var admin any
		if deps.Metadata == nil {
			limitations = append(limitations, "metadata repository is not configured")
		} else {
			status, err := adminstatus.Build(c.Request.Context(), cfg, deps.Metadata, adminstatus.Request{
				CountLimit:        debugNamedLimit(c, "count_limit", 1000, 10000),
				RecentDedupeLimit: debugNamedLimit(c, "recent_dedupe_limit", 5, 100),
				RecentGCLimit:     debugNamedLimit(c, "recent_gc_limit", 5, 100),
			})
			if err != nil {
				statusText = "error"
				limitations = append(limitations, "admin status collection failed: "+err.Error())
			} else {
				statusText = status.Status
				counts = status.Counts
				admin = status
			}
		}
		sbs := deps.SBSCollector.Snapshot(c.Request.Context())
		alerts := consoleAlertList(cfg, deps, statusText)
		reports := consoleReportCatalog(cfg)
		admission := consoleAdmissionSummary(deps)
		body := consoleReadOnlyEnvelope("namros.console.operations.summary.v1", "", statusText, "namros_console", limitations)
		body["gateway"] = gin.H{"health": statusText, "readiness": readinessText(statusText)}
		body["metadata"] = gin.H{
			"backend":                config.NormalizeMetadataBackend(cfg.MetadataBackend),
			"kms_keys":               counts.KMSKeys,
			"audit_events":           counts.AuditEvents,
			"gc_operations":          counts.GCOperations,
			"dedupe_operations":      counts.DedupeOperations,
			"shared_objects":         counts.SharedObjects,
			"shared_object_releases": counts.SharedObjectReleases,
		}
		body["admin_status"] = admin
		body["sbs"] = gin.H{
			"status":                   sbs.Status,
			"source_authority":         sbs.SourceAuthority,
			"source_schema_version":    sbs.SourceSchemaVersion,
			"nodes":                    sbs.Nodes,
			"volumes":                  sbs.Volumes,
			"pool":                     sbs.Pool,
			"stores_total":             len(sbs.Stores),
			"capacity":                 sbs.Capacity,
			"reclaim":                  sbs.Reclaim,
			"maintenance":              sbs.Maintenance,
			"limitations":              sbs.Limitations,
			"duplicate_implementation": false,
		}
		body["alerts"] = gin.H{"active": len(alerts), "items": alerts}
		body["reports"] = gin.H{"total": len(reports), "items": reports}
		body["admission"] = admission
		body["object_explorer"] = gin.H{
			"status":            "available",
			"payload_available": false,
			"operation_surface": "read_only",
			"disabled_actions":  objectExplorerDisabledActions(),
		}
		body["observability"] = gin.H{
			"prometheus":                 datasourceConfigured(cfg.ObservabilityPrometheusURL, "/metrics"),
			"grafana":                    datasourceConfigured(cfg.ObservabilityGrafanaURL, ""),
			"victoriametrics":            datasourceConfigured(cfg.ObservabilityVictoriaURL, ""),
			"namrbd_sbs_observability":   strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint) != "",
			"namrbd_sbs_endpoint_source": sbs.SourceAuthority,
		}
		c.JSON(http.StatusOK, body)
	}
}

func consoleOperationsWarnings(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		warnings := []gin.H{}
		if deps.Metadata == nil {
			warnings = append(warnings, consoleWarning("metadata_unconfigured", "metadata repository is not configured", "high"))
		}
		if strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint) == "" {
			warnings = append(warnings, consoleWarning("namrbd_sbs_observability_unconfigured", "NAMRBD SBS observability endpoint is not configured; SBS views use local fallback only.", "medium"))
		}
		sbs := deps.SBSCollector.Snapshot(c.Request.Context())
		for _, limitation := range sbs.Limitations {
			warnings = append(warnings, consoleWarning("sbs_limitation", limitation, "medium"))
		}
		if sbs.FirstError != "" {
			warnings = append(warnings, consoleWarning("sbs_source_error", sbs.FirstError, "high"))
		}
		if total := consoleAdmissionTotal(deps); total > 0 {
			warnings = append(warnings, consoleWarning("admission_rejections_observed", "Gateway admission rejections have been observed since process start; inspect operations metrics for kind/reason counts.", "medium"))
		}
		if !datasourceConfigured(cfg.ObservabilityGrafanaURL, "") {
			warnings = append(warnings, consoleWarning("grafana_unconfigured", "Grafana datasource URL is not configured for console deep links.", "low"))
		}
		body := consoleReadOnlyEnvelope("namros.console.operations.warnings.v1", "", warningStatus(len(warnings)), "namros_console", nil)
		body["warnings"] = warnings
		body["warning_count"] = len(warnings)
		c.JSON(http.StatusOK, body)
	}
}

func consoleQueryViews(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		views := []gin.H{
			consoleQueryView("operations_summary", "/api/v1/operations/summary", "namros.console.operations.summary.v1", "namros_console", true),
			consoleQueryView("operations_warnings", "/api/v1/operations/warnings", "namros.console.operations.warnings.v1", "namros_console", true),
			consoleQueryView("gateway_status", "/api/v1/status", "namros.console.status.v1", "namros_console", true),
			consoleQueryView("gateway_metrics", "/api/v1/metrics", "namros.console.metrics.v1", "namros_console", true),
			consoleQueryView("sbs_cluster", "/api/v1/sbs/cluster", "namros.console.sbs.cluster.v1", "namrbd_sbs_observability", strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint) != ""),
			consoleQueryView("sbs_capacity", "/api/v1/sbs/capacity", "namros.console.sbs.capacity.v1", "namrbd_sbs_observability", strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint) != ""),
			consoleQueryView("object_explorer_buckets", "/api/v1/object-explorer/buckets", "namros.console.object_explorer.buckets.v1", "namros_metadata", true),
			consoleQueryView("object_explorer_objects", "/api/v1/object-explorer/objects", "namros.console.object_explorer.objects.v1", "namros_metadata", true),
			consoleQueryView("reports", "/api/v1/reports", "namros.console.reports.v1", "namros_console", true),
			consoleQueryView("runbooks", "/api/v1/runbooks", "namros.console.runbooks.v1", "namros_console", true),
		}
		body := consoleReadOnlyEnvelope("namros.console.query.views.v1", "", "ok", "namros_console", nil)
		body["views"] = views
		body["contracts"] = gin.H{
			"sbs_source_schema_version":    "namrbd.sbs.observability.v1",
			"duplicate_sbs_implementation": false,
		}
		c.JSON(http.StatusOK, body)
	}
}

func consoleGUISummary(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := consoleReadOnlyEnvelope("namros.console.gui.summary.v1", "", "ok", "namros_console", nil)
		body["console"] = gin.H{
			"mount_path":            "/console/",
			"initial_view_api":      "/api/v1/operations/summary",
			"refresh_interval_secs": 30,
			"public_open_mode":      "read_only_data",
			"payload_preview":       "disabled",
		}
		body["navigation"] = []gin.H{
			{"id": "overview", "label": "Overview", "api": "/api/v1/operations/summary"},
			{"id": "gateway", "label": "Gateway", "api": "/api/v1/status"},
			{"id": "metadata", "label": "Metadata", "api": "/api/v1/status"},
			{"id": "sbs", "label": "SBS", "api": "/api/v1/sbs/cluster"},
			{"id": "capacity", "label": "Capacity", "api": "/api/v1/sbs/capacity"},
			{"id": "objects", "label": "Object Explorer Lite", "api": "/api/v1/object-explorer/buckets"},
			{"id": "alerts", "label": "Alerts", "api": "/api/v1/alerts"},
			{"id": "evidence", "label": "Reports/Evidence", "api": "/api/v1/reports"},
			{"id": "settings", "label": "Settings", "api": "/api/v1/query/views"},
		}
		body["datasources"] = []gin.H{
			observabilityDatasource("prometheus", cfg.ObservabilityPrometheusURL, "/metrics"),
			observabilityDatasource("grafana", cfg.ObservabilityGrafanaURL, ""),
			observabilityDatasource("victoriametrics", cfg.ObservabilityVictoriaURL, ""),
			observabilityDatasource("namrbd_sbs_observability", cfg.NAMRBDSBSObservabilityEndpoint, ""),
		}
		c.JSON(http.StatusOK, body)
	}
}

func consoleWorkflowHardening(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		body := consoleReadOnlyEnvelope("namros.console.workflow.hardening.v1", "", "ok", "namros_console", nil)
		body["workflow"] = gin.H{
			"apply_supported":       false,
			"human_approval":        "required_for_future_mutations",
			"plan_only_operations":  true,
			"csrf_required":         config.NormalizeConsoleAuthMode(cfg.ConsoleAuthMode) == "local",
			"session_auth_required": config.NormalizeConsoleAuthMode(cfg.ConsoleAuthMode) == "local",
			"audit_mode":            cfg.AccessAuditMode,
			"rbac_surface":          "console_read_only",
		}
		body["disabled_actions"] = gin.H{
			"sbs_drain":               "NAMRBD_owned_not_implemented_in_NAMROS",
			"sbs_remove":              "NAMRBD_owned_not_implemented_in_NAMROS",
			"sbs_rejoin":              "NAMRBD_owned_not_implemented_in_NAMROS",
			"sbs_repair":              "NAMRBD_owned_not_implemented_in_NAMROS",
			"sbs_rebalance":           "NAMRBD_owned_not_implemented_in_NAMROS",
			"sbs_reclaim":             "NAMRBD_owned_not_implemented_in_NAMROS",
			"object_payload_download": "disabled",
			"object_mutation":         "external_s3_clients_or_future_approved_operation",
		}
		c.JSON(http.StatusOK, body)
	}
}

func consoleOperationPlan(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := strings.TrimSpace(c.Param("name"))
		var selected gin.H
		for _, operation := range consoleOperationCatalog(cfg) {
			if operation["name"] == name {
				selected = operation
				break
			}
		}
		if selected == nil {
			c.JSON(http.StatusNotFound, consoleMerge(consoleReadOnlyEnvelope("namros.console.operation.plan.v1", "", "not_found", "namros_console", nil), gin.H{
				"operation": name,
			}))
			return
		}
		mode, _ := selected["mode"].(string)
		available, _ := selected["available"].(bool)
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.operation.plan.v1", "", "planned", "namros_console", nil), gin.H{
			"operation_id": "plan-" + strings.ReplaceAll(name, ".", "-"),
			"operation":    selected,
			"risk_class":   operationRiskClass(mode),
			"approval": gin.H{
				"required": mode != "read_only",
				"mode":     mode,
			},
			"plan": gin.H{
				"available":       available,
				"apply_supported": false,
				"summary":         selected["summary"],
			},
			"preflight": gin.H{
				"status": "not_run",
			},
			"apply": gin.H{
				"status": "disabled",
			},
			"verify": gin.H{
				"status": "not_run",
			},
			"audit": gin.H{
				"status": "planned_only",
			},
		}))
	}
}

func consoleOperationCatalog(cfg config.Config) []gin.H {
	return []gin.H{
		consoleOperation(cfg, "namros.health.check", edition.FeatureCoreS3API, "read_only", "Collect health/readiness/admin status and operations metrics."),
		consoleOperation(cfg, "namros.object_explorer.list", edition.FeatureCoreS3API, "read_only", "List buckets, prefixes, and object metadata without payload bytes."),
		consoleOperation(cfg, "namros.compat.user_space.run", edition.FeatureCoreS3API, "operator_approved", "Plan a user-space compatibility smoke run."),
		consoleOperation(cfg, "namros.release.readiness.run", edition.FeatureCoreS3API, "operator_approved", "Plan a release-readiness target run."),
		consoleOperation(cfg, "namros.multi_node.soak.run", edition.FeatureAdvancedOps, "operator_approved", "Plan or execute a multi-node chaos/soak report run."),
		consoleOperation(cfg, "namros.compliance.evidence.create", edition.FeatureComplianceEvidence, "operator_approved", "Create a compliance evidence package."),
	}
}

func operationRiskClass(mode string) string {
	switch mode {
	case "read_only":
		return "observe"
	case "operator_approved":
		return "probe"
	default:
		return "unknown"
	}
}

func readinessText(status string) string {
	switch status {
	case "ok":
		return "ready"
	case "error":
		return "not_ready"
	default:
		return "degraded"
	}
}

func warningStatus(count int) string {
	if count == 0 {
		return "ok"
	}
	return "warning"
}

func consoleWarning(id, message, severity string) gin.H {
	return gin.H{
		"id":       id,
		"message":  message,
		"severity": severity,
	}
}

func consoleQueryView(id, path, schemaVersion, sourceAuthority string, available bool) gin.H {
	status := "available"
	if !available {
		status = "fallback_or_unconfigured"
	}
	return gin.H{
		"id":               id,
		"path":             path,
		"schema_version":   schemaVersion,
		"source_authority": sourceAuthority,
		"read_only":        true,
		"status":           status,
		"available":        available,
	}
}

func consoleRunbooks() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.runbooks.v1", "", "ok", "namros_console", nil), gin.H{
			"runbooks": mcpops.RunbookIndex(),
		}))
	}
}

func consoleAlerts(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.alerts.v1", "", "ok", "namros_console", nil), gin.H{
			"catalog": opsalerts.Catalog(),
			"alerts":  consoleAlertList(cfg, deps, ""),
		}))
	}
}

func consoleNotificationChannels() gin.HandlerFunc {
	return func(c *gin.Context) {
		limitations := []string{"Notification delivery adapters are skeleton-only in this build slice."}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.notification.channels.v1", "", "disabled", "namros_console", limitations), gin.H{
			"channels": []gin.H{
				{
					"id":              "ops-webhook",
					"kind":            "alertmanager_webhook",
					"enabled":         false,
					"secret_redacted": true,
					"last_delivery":   nil,
				},
			},
		}))
	}
}

func consoleNotificationRoutes() gin.HandlerFunc {
	return func(c *gin.Context) {
		limitations := []string{"Alertmanager route management is documented but not yet applied by NAMROS."}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.notification.routes.v1", "", "disabled", "namros_console", limitations), gin.H{
			"routes": []gin.H{},
		}))
	}
}

func consoleNotificationTest() gin.HandlerFunc {
	return func(c *gin.Context) {
		limitations := []string{"No external notification sink is configured in this build slice."}
		c.JSON(http.StatusAccepted, consoleMerge(consoleReadOnlyEnvelope("namros.console.notification.test.v1", "", "not_delivered", "namros_console", limitations), gin.H{
			"delivery_state": "disabled",
		}))
	}
}

func consoleObservabilityDatasources(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.observability.datasources.v1", "", "ok", "namros_console", nil), gin.H{
			"datasources": []gin.H{
				observabilityDatasource("prometheus", cfg.ObservabilityPrometheusURL, "/metrics"),
				observabilityDatasource("grafana", cfg.ObservabilityGrafanaURL, ""),
				observabilityDatasource("victoriametrics", cfg.ObservabilityVictoriaURL, ""),
			},
			"secret_redaction": defaultConsoleRedactionPolicy(),
		}))
	}
}

func observabilityDatasource(kind, baseURL, localPath string) gin.H {
	status := "unconfigured"
	if strings.TrimSpace(baseURL) != "" || strings.TrimSpace(localPath) != "" {
		status = "configured"
	}
	return gin.H{
		"kind":       kind,
		"status":     status,
		"base_url":   baseURL,
		"local_path": localPath,
	}
}

func consoleSBSCluster(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, deps.SBSCollector.Snapshot(c.Request.Context()))
	}
}

func consoleSBSNodes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := deps.SBSCollector.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, consoleMerge(consoleSBSEnvelope("namros.console.sbs.nodes.v1", snapshot), gin.H{
			"nodes":   snapshot.NodeDetails,
			"summary": snapshot.Nodes,
		}))
	}
}

func consoleSBSStores(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := deps.SBSCollector.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, consoleMerge(consoleSBSEnvelope("namros.console.sbs.stores.v1", snapshot), gin.H{
			"stores":  snapshot.Stores,
			"summary": gin.H{"total": len(snapshot.Stores)},
		}))
	}
}

func consoleSBSVolumes(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := deps.SBSCollector.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, consoleMerge(consoleSBSEnvelope("namros.console.sbs.volumes.v1", snapshot), gin.H{
			"volumes": snapshot.VolumeDetails,
			"summary": snapshot.Volumes,
		}))
	}
}

func consoleSBSCapacity(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := deps.SBSCollector.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, consoleMerge(consoleSBSEnvelope("namros.console.sbs.capacity.v1", snapshot), gin.H{
			"capacity": snapshot.Capacity,
			"stores":   gin.H{"total": len(snapshot.Stores)},
			"volumes":  snapshot.Volumes,
		}))
	}
}

func consoleSBSReclaim(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := deps.SBSCollector.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, consoleMerge(consoleSBSEnvelope("namros.console.sbs.reclaim.v1", snapshot), gin.H{
			"reclaim":  snapshot.Reclaim,
			"capacity": snapshot.Capacity,
		}))
	}
}

func consoleSBSMaintenance(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		snapshot := deps.SBSCollector.Snapshot(c.Request.Context())
		c.JSON(http.StatusOK, consoleMerge(consoleSBSEnvelope("namros.console.sbs.maintenance.v1", snapshot), gin.H{
			"maintenance": snapshot.Maintenance,
		}))
	}
}

func consoleSBSVolumePool(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := disabledSBSVolumePoolRuntimeStatus(time.Now().UTC())
		limitations := []string{"sbs volume pool runtime is not configured"}
		if deps.sbsVolumePoolRuntime != nil {
			status = deps.sbsVolumePoolRuntime.Status()
			limitations = nil
		}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.sbs.volume_pool.v1", "", consoleVolumePoolConsoleStatus(status), "namros_gateway", limitations), gin.H{
			"volume_pool": status,
		}))
	}
}

func consoleVolumePoolConsoleStatus(status sbsVolumePoolRuntimeStatus) string {
	if !status.Enabled {
		return "degraded"
	}
	if status.Stale {
		return "warning"
	}
	return "ok"
}

func consoleObjectExplorerBuckets(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Metadata == nil {
			limitations := []string{"metadata repository is not configured"}
			c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.buckets.v1", "", "degraded", "namros_metadata", limitations), gin.H{
				"buckets":           []gin.H{},
				"payload_available": false,
				"disabled_actions":  objectExplorerDisabledActions(),
			}))
			return
		}
		tenantID := strings.TrimSpace(c.DefaultQuery("tenant_id", "root"))
		buckets, err := deps.Metadata.ListBuckets(c.Request.Context(), tenantID)
		if err != nil {
			consoleObjectExplorerError(c, "namros.console.object_explorer.buckets.v1", err)
			return
		}
		sort.Slice(buckets, func(i, j int) bool {
			return buckets[i].Name < buckets[j].Name
		})
		out := make([]gin.H, 0, len(buckets))
		for _, bucket := range buckets {
			out = append(out, consoleObjectExplorerBucket(bucket))
		}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.buckets.v1", "", "ok", "namros_metadata", nil), gin.H{
			"tenant_id":         tenantID,
			"buckets":           out,
			"payload_available": false,
			"operation_surface": "read_only",
			"disabled_actions":  objectExplorerDisabledActions(),
			"secret_redaction":  defaultConsoleRedactionPolicy(),
		}))
	}
}

func consoleObjectExplorerObjects(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Metadata == nil {
			limitations := []string{"metadata repository is not configured"}
			c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.objects.v1", "", "degraded", "namros_metadata", limitations), gin.H{
				"objects":           []gin.H{},
				"common_prefixes":   []string{},
				"payload_available": false,
				"disabled_actions":  objectExplorerDisabledActions(),
			}))
			return
		}
		bucket, ok := consoleObjectExplorerResolveBucket(c, deps.Metadata, "namros.console.object_explorer.objects.v1")
		if !ok {
			return
		}
		prefix := c.Query("prefix")
		delimiter := c.Query("delimiter")
		maxKeys := debugNamedLimit(c, "max_keys", 100, 1000)
		if strings.EqualFold(c.Query("versions"), "true") {
			result, err := deps.Metadata.ListObjectVersions(c.Request.Context(), meta.ListObjectVersionsRequest{
				BucketID:        bucket.BucketID,
				Prefix:          prefix,
				Delimiter:       delimiter,
				KeyMarker:       c.Query("key_marker"),
				VersionIDMarker: c.Query("version_id_marker"),
				MaxKeys:         maxKeys,
			})
			if err != nil {
				consoleObjectExplorerError(c, "namros.console.object_explorer.objects.v1", err)
				return
			}
			versions := make([]gin.H, 0, len(result.Versions))
			for _, entry := range result.Versions {
				versions = append(versions, consoleObjectExplorerVersion(entry.Version, entry.IsLatest))
			}
			deleteMarkers := make([]gin.H, 0, len(result.DeleteMarkers))
			for _, entry := range result.DeleteMarkers {
				deleteMarkers = append(deleteMarkers, consoleObjectExplorerVersion(entry.Version, entry.IsLatest))
			}
			c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.objects.v1", "", "ok", "namros_metadata", nil), gin.H{
				"bucket":                 bucket.Name,
				"bucket_id":              bucket.BucketID,
				"prefix":                 prefix,
				"delimiter":              delimiter,
				"versions_requested":     true,
				"versions":               versions,
				"delete_markers":         deleteMarkers,
				"common_prefixes":        result.CommonPrefixes,
				"is_truncated":           result.IsTruncated,
				"next_key_marker":        result.NextKeyMarker,
				"next_version_id_marker": result.NextVersionIDMarker,
				"payload_available":      false,
				"operation_surface":      "read_only",
				"disabled_actions":       objectExplorerDisabledActions(),
				"secret_redaction":       defaultConsoleRedactionPolicy(),
			}))
			return
		}
		result, err := deps.Metadata.ListObjects(c.Request.Context(), meta.ListObjectsRequest{
			BucketID:          bucket.BucketID,
			Prefix:            prefix,
			Delimiter:         delimiter,
			ContinuationToken: c.Query("continuation_token"),
			MaxKeys:           maxKeys,
		})
		if err != nil {
			consoleObjectExplorerError(c, "namros.console.object_explorer.objects.v1", err)
			return
		}
		objects := make([]gin.H, 0, len(result.Contents))
		for _, head := range result.Contents {
			objects = append(objects, consoleObjectExplorerHead(head))
		}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.objects.v1", "", "ok", "namros_metadata", nil), gin.H{
			"bucket":                  bucket.Name,
			"bucket_id":               bucket.BucketID,
			"prefix":                  prefix,
			"delimiter":               delimiter,
			"versions_requested":      false,
			"objects":                 objects,
			"common_prefixes":         result.CommonPrefixes,
			"is_truncated":            result.IsTruncated,
			"next_continuation_token": result.NextContinuationToken,
			"payload_available":       false,
			"operation_surface":       "read_only",
			"disabled_actions":        objectExplorerDisabledActions(),
			"secret_redaction":        defaultConsoleRedactionPolicy(),
		}))
	}
}

func consoleObjectExplorerObject(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Metadata == nil {
			limitations := []string{"metadata repository is not configured"}
			c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.object.v1", "", "degraded", "namros_metadata", limitations), gin.H{
				"payload_available": false,
				"disabled_actions":  objectExplorerDisabledActions(),
			}))
			return
		}
		bucket, ok := consoleObjectExplorerResolveBucket(c, deps.Metadata, "namros.console.object_explorer.object.v1")
		if !ok {
			return
		}
		key := c.Query("key")
		if key == "" {
			consoleObjectExplorerBadRequest(c, "namros.console.object_explorer.object.v1", "key query parameter is required")
			return
		}
		versionID := c.Query("version_id")
		if versionID != "" && versionID != "null" {
			version, err := deps.Metadata.GetObjectVersion(c.Request.Context(), bucket.BucketID, key, versionID)
			if err != nil {
				consoleObjectExplorerError(c, "namros.console.object_explorer.object.v1", err)
				return
			}
			c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.object.v1", "", "ok", "namros_metadata", nil), gin.H{
				"bucket":            bucket.Name,
				"bucket_id":         bucket.BucketID,
				"object":            consoleObjectExplorerVersion(version, false),
				"payload_available": false,
				"operation_surface": "read_only",
				"disabled_actions":  objectExplorerDisabledActions(),
				"secret_redaction":  defaultConsoleRedactionPolicy(),
			}))
			return
		}
		head, err := deps.Metadata.GetObjectHead(c.Request.Context(), bucket.BucketID, key)
		if err != nil {
			consoleObjectExplorerError(c, "namros.console.object_explorer.object.v1", err)
			return
		}
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.object.v1", "", "ok", "namros_metadata", nil), gin.H{
			"bucket":            bucket.Name,
			"bucket_id":         bucket.BucketID,
			"object":            consoleObjectExplorerHead(head),
			"payload_available": false,
			"operation_surface": "read_only",
			"disabled_actions":  objectExplorerDisabledActions(),
			"secret_redaction":  defaultConsoleRedactionPolicy(),
		}))
	}
}

func consoleObjectExplorerExternalClients() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, consoleMerge(consoleReadOnlyEnvelope("namros.console.object_explorer.external_clients.v1", "", "ok", "namros_console", nil), gin.H{
			"operation_surface": "read_only",
			"clients": []gin.H{
				objectExplorerExternalClient("awscli", "AWS CLI", "cli", "Baseline S3 compatibility, automation, and smoke tests.", true, false, "s3-client-compatibility-guide.html"),
				objectExplorerExternalClient("mc", "MinIO client", "cli", "Operator CLI browsing, stat, copy, mirror, and extended S3 workflows.", true, false, "s3-client-compatibility-guide.html#mc"),
				objectExplorerExternalClient("rclone", "rclone", "cli", "Migration, synchronization, scripted copy/delete workflows.", true, false, "https://rclone.org/s3/"),
				objectExplorerExternalClient("cyberduck", "Cyberduck", "desktop_gui", "Desktop GUI object browsing for operators and testers.", false, false, "https://docs.cyberduck.io/"),
				objectExplorerExternalClient("brows3", "Brows3", "desktop_gui", "Desktop S3 browser candidate pending NAMROS compatibility validation.", false, false, "https://www.brows3.app/"),
				objectExplorerExternalClient("filestash", "Filestash", "self_hosted_web", "Optional self-hosted web file manager; not bundled by default.", false, false, "https://www.filestash.app/s3-browser.html"),
				objectExplorerExternalClient("minio_console", "MinIO Console", "benchmark", "Benchmark for object browser and operations UX, not a NAMROS dependency.", false, false, "https://minio.community/community/minio-object-store/administration/minio-console.html"),
			},
			"recipe": gin.H{
				"endpoint_env":       "NAMROS_ENDPOINT",
				"region_env":         "AWS_DEFAULT_REGION",
				"access_key_env":     "AWS_ACCESS_KEY_ID",
				"secret_key_env":     "AWS_SECRET_ACCESS_KEY",
				"path_style":         true,
				"credential_source":  "environment_or_temporary_secret_store",
				"secrets_redacted":   true,
				"presigned_url":      "not_returned",
				"root_credentials":   "not_recommended",
				"compatibility_gate": "make container-local-smoke",
			},
			"disabled_actions": objectExplorerDisabledActions(),
			"secret_redaction": defaultConsoleRedactionPolicy(),
		}))
	}
}

func consoleObjectExplorerResolveBucket(c *gin.Context, repo meta.Repository, schemaVersion string) (model.Bucket, bool) {
	bucketName := strings.TrimSpace(c.Query("bucket"))
	if bucketName != "" {
		bucket, err := repo.GetBucketByName(c.Request.Context(), bucketName)
		if err != nil {
			consoleObjectExplorerError(c, schemaVersion, err)
			return model.Bucket{}, false
		}
		return bucket, true
	}
	bucketID := strings.TrimSpace(c.Query("bucket_id"))
	if bucketID == "" {
		consoleObjectExplorerBadRequest(c, schemaVersion, "bucket or bucket_id query parameter is required")
		return model.Bucket{}, false
	}
	tenantID := strings.TrimSpace(c.Query("tenant_id"))
	buckets, err := repo.ListBuckets(c.Request.Context(), tenantID)
	if err != nil {
		consoleObjectExplorerError(c, schemaVersion, err)
		return model.Bucket{}, false
	}
	for _, bucket := range buckets {
		if bucket.BucketID == bucketID {
			return bucket, true
		}
	}
	consoleObjectExplorerError(c, schemaVersion, meta.ErrNotFound)
	return model.Bucket{}, false
}

func consoleObjectExplorerBucket(bucket model.Bucket) gin.H {
	return gin.H{
		"name":               bucket.Name,
		"bucket_id":          bucket.BucketID,
		"tenant_id":          bucket.TenantID,
		"region":             bucket.Region,
		"created_at":         formatConsoleTime(bucket.CreatedAt),
		"versioning_state":   bucket.VersioningState,
		"object_lock":        consoleObjectExplorerBucketObjectLock(bucket.ObjectLock),
		"default_encryption": consoleObjectExplorerEncryption(bucket.DefaultEncryption),
	}
}

func consoleObjectExplorerHead(head model.ObjectHead) gin.H {
	return gin.H{
		"key":                    head.Key,
		"version_id":             head.VersionID,
		"size":                   head.SizeBytes,
		"etag":                   head.ETag,
		"content_type":           head.ContentType,
		"last_modified":          formatConsoleTime(head.LastModified),
		"delete_marker":          head.DeleteMarker,
		"storage_class":          consoleObjectExplorerStorageClass(head.StorageClass),
		"server_side_encryption": consoleObjectExplorerEncryption(head.ServerSideEncryption),
		"user_metadata":          consoleObjectExplorerRedactedMap(head.UserMetadata),
		"tags":                   consoleObjectExplorerRedactedMap(head.Tags),
		"object_lock":            consoleObjectExplorerObjectLock(head.ObjectLockRetention, head.ObjectLockLegalHold),
		"payload_available":      false,
	}
}

func consoleObjectExplorerVersion(version model.ObjectVersion, isLatest bool) gin.H {
	return gin.H{
		"key":                    version.Key,
		"version_id":             version.VersionID,
		"size":                   version.SizeBytes,
		"etag":                   version.ETag,
		"content_type":           version.ContentType,
		"created_at":             formatConsoleTime(version.CreatedAt),
		"committed_at":           formatConsoleTime(version.CommittedAt),
		"delete_marker":          version.DeleteMarker,
		"is_latest":              isLatest,
		"state":                  version.State,
		"storage_class":          consoleObjectExplorerStorageClass(version.StorageClass),
		"server_side_encryption": consoleObjectExplorerEncryption(version.ServerSideEncryption),
		"user_metadata":          consoleObjectExplorerRedactedMap(version.UserMetadata),
		"tags":                   consoleObjectExplorerRedactedMap(version.Tags),
		"object_lock":            consoleObjectExplorerObjectLock(version.ObjectLockRetention, version.ObjectLockLegalHold),
		"payload_available":      false,
	}
}

func consoleObjectExplorerStorageClass(storageClass storage.StorageClassSnapshot) gin.H {
	return gin.H{
		"storage_class_id": storageClass.StorageClassID,
		"backend":          storageClass.Backend,
	}
}

func consoleObjectExplorerBucketObjectLock(lock model.BucketObjectLockConfiguration) gin.H {
	return gin.H{
		"enabled": lock.Enabled,
		"default_retention": gin.H{
			"mode":  lock.DefaultRetention.Mode,
			"days":  lock.DefaultRetention.Days,
			"years": lock.DefaultRetention.Years,
		},
	}
}

func consoleObjectExplorerObjectLock(retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus) gin.H {
	return gin.H{
		"retention": gin.H{
			"mode":              retention.Mode,
			"retain_until_date": formatConsoleTime(retention.RetainUntilDate),
		},
		"legal_hold": legalHold,
	}
}

func consoleObjectExplorerEncryption(encryption model.ServerSideEncryption) gin.H {
	return gin.H{
		"algorithm":           encryption.Algorithm,
		"kms_key_id_present":  encryption.KeyID != "",
		"key_version_present": encryption.KeyVersion != "",
	}
}

func consoleObjectExplorerRedactedMap(values map[string]string) gin.H {
	out := make(map[string]string, len(values))
	redacted := false
	for key, value := range values {
		if objectExplorerSensitiveKey(key) {
			out[key] = "[REDACTED]"
			redacted = true
			continue
		}
		out[key] = value
	}
	return gin.H{
		"values":   out,
		"count":    len(values),
		"redacted": redacted,
	}
}

func objectExplorerSensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, needle := range []string{"secret", "token", "authorization", "credential", "password", "private-key", "access-key", "session"} {
		if strings.Contains(normalized, needle) {
			return true
		}
	}
	return false
}

func objectExplorerDisabledActions() gin.H {
	return gin.H{
		"payload_preview": "disabled",
		"download":        "external_tools_or_future_approved_operation",
		"upload":          "external_tools",
		"copy_move":       "external_tools",
		"delete":          "disabled_until_approved_operation_policy",
		"bulk_delete":     "disabled",
	}
}

func objectExplorerExternalClient(id, name, kind, use string, compatibilityTarget, bundled bool, docsURL string) gin.H {
	return gin.H{
		"id":                   id,
		"name":                 name,
		"kind":                 kind,
		"recommended_use":      use,
		"compatibility_target": compatibilityTarget,
		"bundled":              bundled,
		"docs_url":             docsURL,
	}
}

func consoleObjectExplorerBadRequest(c *gin.Context, schemaVersion, message string) {
	c.JSON(http.StatusBadRequest, consoleMerge(consoleReadOnlyEnvelope(schemaVersion, "", "invalid_request", "namros_metadata", nil), gin.H{
		"error":             message,
		"payload_available": false,
		"disabled_actions":  objectExplorerDisabledActions(),
	}))
}

func consoleObjectExplorerError(c *gin.Context, schemaVersion string, err error) {
	statusCode := http.StatusInternalServerError
	status := "error"
	if errors.Is(err, meta.ErrNotFound) {
		statusCode = http.StatusNotFound
		status = "not_found"
	}
	if errors.Is(err, meta.ErrInvalidArgument) {
		statusCode = http.StatusBadRequest
		status = "invalid_request"
	}
	c.JSON(statusCode, consoleMerge(consoleReadOnlyEnvelope(schemaVersion, "", status, "namros_metadata", nil), gin.H{
		"error":             err.Error(),
		"payload_available": false,
		"disabled_actions":  objectExplorerDisabledActions(),
	}))
}

func formatConsoleTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func consoleDashboardSummary(cfg config.Config, deps Dependencies, platformStatus string, counts adminstatus.Counts) gin.H {
	sbs := deps.SBSCollector.Snapshot(nil)
	alerts := consoleAlertList(cfg, deps, platformStatus)
	admission := consoleAdmissionSummary(deps)
	return gin.H{
		"platform": gin.H{
			"status": platformStatus,
			"alerts": len(alerts),
		},
		"metadata": gin.H{
			"kms_keys":               counts.KMSKeys,
			"audit_events":           counts.AuditEvents,
			"gc_operations":          counts.GCOperations,
			"dedupe_operations":      counts.DedupeOperations,
			"shared_objects":         counts.SharedObjects,
			"shared_object_releases": counts.SharedObjectReleases,
		},
		"sbs": gin.H{
			"status":      sbs.Status,
			"nodes":       sbs.Nodes,
			"volumes":     sbs.Volumes,
			"maintenance": sbs.Maintenance,
		},
		"observability": gin.H{
			"prometheus":      datasourceConfigured(cfg.ObservabilityPrometheusURL, "/metrics"),
			"grafana":         datasourceConfigured(cfg.ObservabilityGrafanaURL, ""),
			"victoriametrics": datasourceConfigured(cfg.ObservabilityVictoriaURL, ""),
		},
		"admission": gin.H{
			"rejections": admission["total_rejections"],
			"by_kind":    admission["by_kind"],
		},
	}
}

func consoleAdmissionSummary(deps Dependencies) gin.H {
	out := gin.H{
		"enabled":          deps.GatewayMetrics != nil,
		"total_rejections": uint64(0),
		"by_kind":          map[string]uint64{},
		"items":            []gin.H{},
	}
	if deps.GatewayMetrics == nil {
		return out
	}
	snapshot := deps.GatewayMetrics.Snapshot()
	total := uint64(0)
	byKind := make(map[string]uint64)
	items := make([]gin.H, 0, len(snapshot.Admissions))
	for _, admission := range snapshot.Admissions {
		total += admission.Count
		byKind[admission.Kind] += admission.Count
		items = append(items, gin.H{
			"kind":   admission.Kind,
			"reason": admission.Reason,
			"count":  admission.Count,
		})
	}
	out["total_rejections"] = total
	out["by_kind"] = byKind
	out["items"] = items
	return out
}

func consoleAdmissionTotal(deps Dependencies) uint64 {
	summary := consoleAdmissionSummary(deps)
	total, _ := summary["total_rejections"].(uint64)
	return total
}

func datasourceConfigured(baseURL, localPath string) bool {
	return strings.TrimSpace(baseURL) != "" || strings.TrimSpace(localPath) != ""
}

func consoleOperation(cfg config.Config, name, featureID, mode, summary string) gin.H {
	minimumEdition := featureMinimumEdition(featureID)
	return gin.H{
		"name":            name,
		"feature_id":      featureID,
		"minimum_edition": minimumEdition,
		"mode":            mode,
		"summary":         summary,
		"available":       edition.Allows(cfg.Edition, featureID),
	}
}

func featureMinimumEdition(featureID string) string {
	for _, feature := range edition.Catalog() {
		if feature.ID == featureID {
			return feature.MinimumEdition
		}
	}
	return edition.Enterprise
}

func consoleOperationHistory(dir string) []gin.H {
	entries, err := consoleOperationEntries(dir, false)
	if err != nil {
		return nil
	}
	return entries
}

func consoleEvidenceBundles(dir string) []gin.H {
	entries, err := consoleOperationEntries(dir, true)
	if err != nil {
		return nil
	}
	return entries
}

func consoleOperationEntries(dir string, bundles bool) ([]gin.H, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	items, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type row struct {
		body       gin.H
		modifiedAt time.Time
	}
	rows := make([]row, 0, len(items))
	for _, item := range items {
		info, err := item.Info()
		if err != nil {
			continue
		}
		name := item.Name()
		if bundles {
			if !item.IsDir() || !strings.HasPrefix(name, "incident-") {
				continue
			}
		} else {
			if item.IsDir() || !strings.HasSuffix(name, ".json") {
				continue
			}
		}
		rows = append(rows, row{
			modifiedAt: info.ModTime(),
			body: gin.H{
				"name":        name,
				"path":        filepath.Join(dir, name),
				"size_bytes":  info.Size(),
				"modified_at": info.ModTime().UTC().Format(time.RFC3339Nano),
			},
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].modifiedAt.After(rows[j].modifiedAt)
	})
	if len(rows) > 20 {
		rows = rows[:20]
	}
	out := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.body)
	}
	return out, nil
}

func consoleAlertList(cfg config.Config, deps Dependencies, status string) []gin.H {
	alerts := make([]gin.H, 0)
	if status != "" && status != "ok" {
		alerts = append(alerts, consoleAlert(opsalerts.NewInstance(opsalerts.GatewayDown, "Platform status is "+status, "")))
	}
	if deps.Metadata == nil {
		alerts = append(alerts, consoleAlert(opsalerts.NewInstance(opsalerts.MetadataUnavailable, "Metadata repository is not configured", "")))
	}
	if config.NormalizeMetadataBackend(cfg.MetadataBackend) == config.MetadataBackendTiKV && deps.TiKVMetrics == nil {
		alerts = append(alerts, consoleAlert(opsalerts.NewInstance(opsalerts.MetadataUnavailable, "TiKV metadata backend is configured without TiKV metrics", edition.FeatureTiKVMetadataCluster)))
	}
	if cfg.CoordinationBackend == config.CoordinationBackendEtcd && len(cfg.EtcdEndpoints) == 0 {
		alerts = append(alerts, consoleAlert(opsalerts.NewInstance(opsalerts.GatewayLeaseExpired, "etcd coordination is enabled without endpoints", edition.FeatureActiveActiveGateway)))
	}
	for _, featureID := range []string{edition.FeatureAdvancedOps, edition.FeatureComplianceEvidence, edition.FeatureExternalIAMFederation} {
		if !edition.Allows(cfg.Edition, featureID) {
			alerts = append(alerts, consoleAlert(opsalerts.NewInstance(opsalerts.EnterpriseFeatureDeny, "Enterprise feature is not available in this edition", featureID)))
		}
	}
	return alerts
}

func consoleAlert(alert opsalerts.Instance) gin.H {
	out := gin.H{
		"id":        alert.ID,
		"severity":  alert.Severity,
		"component": alert.Component,
		"message":   alert.Message,
	}
	if alert.FeatureID != "" {
		out["feature_id"] = alert.FeatureID
	}
	if alert.Runbook != "" {
		out["runbook"] = alert.Runbook
	}
	return out
}

func defaultConsoleRedactionPolicy() []string {
	return []string{
		"secret",
		"access_key",
		"credential",
		"session_token",
		"token",
		"authorization_header",
		"kms_material",
		"kms_key_id",
		"presigned_url",
		"object_payload",
	}
}

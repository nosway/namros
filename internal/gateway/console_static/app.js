const endpoints = {
  edition: "/api/v1/edition",
  operationsSummary: "/api/v1/operations/summary",
  operationsWarnings: "/api/v1/operations/warnings",
  status: "/api/v1/status",
  metrics: "/api/v1/metrics",
  sbsCluster: "/api/v1/sbs/cluster",
  sbsNodes: "/api/v1/sbs/nodes",
  sbsStores: "/api/v1/sbs/stores",
  sbsVolumes: "/api/v1/sbs/volumes",
  sbsCapacity: "/api/v1/sbs/capacity",
  sbsReclaim: "/api/v1/sbs/reclaim",
  sbsMaintenance: "/api/v1/sbs/maintenance",
  objectExplorerBuckets: "/api/v1/object-explorer/buckets",
  objectExplorerObjects: "/api/v1/object-explorer/objects",
  objectExplorerClients: "/api/v1/object-explorer/external-clients",
  reports: "/api/v1/reports",
  operations: "/api/v1/operations",
  runbooks: "/api/v1/runbooks",
  alerts: "/api/v1/alerts",
  datasources: "/api/v1/observability/datasources",
  queryViews: "/api/v1/query/views",
  guiSummary: "/api/v1/gui/summary",
  workflowHardening: "/api/v1/workflow/hardening",
};

const state = {
  lastGood: {},
  views: {},
};

const loaders = {
  overview: loadOverview,
  gateway: loadGateway,
  metadata: loadMetadata,
  sbs: loadSBS,
  capacity: loadCapacity,
  objectExplorer: loadObjectExplorer,
  alerts: loadAlerts,
  evidence: loadEvidence,
  operations: loadOperations,
  settings: loadSettings,
};

document.querySelectorAll(".tab").forEach((button) => {
  button.addEventListener("click", () => activateView(button.dataset.view));
});

document.querySelectorAll("[data-refresh]").forEach((button) => {
  button.addEventListener("click", () => loadView(button.dataset.refresh));
});

loadAll();

function activateView(name) {
  document.querySelectorAll(".tab").forEach((button) => {
    button.classList.toggle("active", button.dataset.view === name);
  });
  document.querySelectorAll(".view").forEach((view) => {
    view.classList.toggle("active", view.id === `view-${name}`);
  });
  loadView(name);
}

async function loadAll() {
  await loadJSON("edition").catch(() => null);
  await loadView("overview");
  await Promise.all(["gateway", "metadata", "sbs", "capacity", "objectExplorer", "alerts", "evidence", "operations", "settings"].map(loadView));
}

async function loadView(name) {
  try {
    const loader = loaders[name];
    state.views[name] = loader ? await loader() : await loadJSON(name);
    render(name, state.views[name]);
  } catch (error) {
    renderError(name, error);
  }
}

async function loadJSON(name) {
  return fetchJSON(endpoints[name], name);
}

async function fetchJSON(url, cacheKey) {
  try {
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
    });
    if (!response.ok) {
      throw new Error(`${response.status} ${response.statusText}`);
    }
    const body = await response.json();
    state.lastGood[cacheKey || url] = body;
    if (cacheKey === "edition") {
      renderEdition(body);
    }
    return body;
  } catch (error) {
    const cached = state.lastGood[cacheKey || url];
    if (cached) {
      return { ...cached, stale: true, stale_error: error.message };
    }
    throw error;
  }
}

async function loadOverview() {
  const [summary, warnings, gui] = await Promise.all([
    loadJSON("operationsSummary"),
    loadJSON("operationsWarnings"),
    loadJSON("guiSummary"),
  ]);
  return { summary, warnings, gui };
}

async function loadGateway() {
  const [status, metrics] = await Promise.all([loadJSON("status"), loadJSON("metrics")]);
  return { status, metrics };
}

async function loadMetadata() {
  return { status: await loadJSON("status") };
}

async function loadSBS() {
  const [cluster, nodes, stores, volumes, maintenance] = await Promise.all([
    loadJSON("sbsCluster"),
    loadJSON("sbsNodes"),
    loadJSON("sbsStores"),
    loadJSON("sbsVolumes"),
    loadJSON("sbsMaintenance"),
  ]);
  return { cluster, nodes, stores, volumes, maintenance };
}

async function loadCapacity() {
  const [capacity, reclaim] = await Promise.all([loadJSON("sbsCapacity"), loadJSON("sbsReclaim")]);
  return { capacity, reclaim };
}

async function loadObjectExplorer() {
  const [buckets, clients] = await Promise.all([
    fetchJSON(endpoints.objectExplorerBuckets, "objectExplorerBuckets"),
    fetchJSON(endpoints.objectExplorerClients, "objectExplorerClients"),
  ]);
  const firstBucket = (buckets.buckets || [])[0];
  let objects = null;
  if (firstBucket) {
    const query = new URLSearchParams({
      bucket: firstBucket.name,
      delimiter: "/",
      max_keys: "50",
    });
    objects = await fetchJSON(`${endpoints.objectExplorerObjects}?${query.toString()}`, `objectExplorerObjects:${firstBucket.name}`);
  }
  return {
    buckets,
    clients,
    objects,
    selected_bucket: firstBucket ? firstBucket.name : "",
  };
}

async function loadAlerts() {
  const [alerts, datasources, warnings] = await Promise.all([
    loadJSON("alerts"),
    loadJSON("datasources"),
    loadJSON("operationsWarnings"),
  ]);
  return { alerts, datasources, warnings };
}

async function loadEvidence() {
  const [reports, runbooks] = await Promise.all([loadJSON("reports"), loadJSON("runbooks")]);
  return { reports, runbooks };
}

async function loadOperations() {
  const [operations, hardening] = await Promise.all([loadJSON("operations"), loadJSON("workflowHardening")]);
  return { operations, hardening };
}

async function loadSettings() {
  const [views, gui, hardening] = await Promise.all([
    loadJSON("queryViews"),
    loadJSON("guiSummary"),
    loadJSON("workflowHardening"),
  ]);
  return { views, gui, hardening };
}

function render(name, body) {
  switch (name) {
    case "overview":
      renderOverview(body);
      break;
    case "gateway":
      renderGateway(body);
      break;
    case "metadata":
      renderMetadata(body);
      break;
    case "sbs":
      renderSBS(body);
      break;
    case "capacity":
      renderCapacity(body);
      break;
    case "objectExplorer":
      renderObjectExplorer(body);
      break;
    case "alerts":
      renderAlerts(body);
      break;
    case "evidence":
      renderEvidence(body);
      break;
    case "operations":
      renderOperations(body);
      break;
    case "settings":
      renderSettings(body);
      break;
  }
}

function renderEdition(body) {
  const edition = body.edition || {};
  const version = body.version || {};
  document.getElementById("edition-line").textContent = `${edition.name || "unknown"} / ${version.version || version.commit || "dev"}`;
}

function renderOverview(body) {
  const summary = body.summary || {};
  const sbs = summary.sbs || {};
  const metadata = summary.metadata || {};
  const warnings = body.warnings || {};
  setHealth(summary.status || "unknown");
  setReadOnlyLine(summary);
  document.getElementById("overview-summary").innerHTML = [
    metric("Gateway", `${value(summary.gateway, "health")} / ${value(summary.gateway, "readiness")}`),
    metric("Metadata Objects", metadata.shared_objects ?? "0"),
    metric("SBS Source", sbs.source_authority || "fallback"),
    metric("Warnings", warnings.warning_count ?? (warnings.warnings || []).length),
  ].join("");
  document.getElementById("overview-sbs-table").innerHTML = table(
    ["Field", "Value"],
    [
      ["Status", badge(sbs.status || "unknown", tone(sbs.status))],
      ["Nodes", compactJSON(sbs.nodes || {})],
      ["Volumes", compactJSON(sbs.volumes || {})],
      ["Stores", sbs.stores_total ?? 0],
      ["Capacity", capacityLine(sbs.capacity || {})],
      ["Reclaim", reclaimLine(sbs.reclaim || {})],
      ["NAMRBD Schema", sbs.source_schema_version || "unconfigured"],
    ],
  );
  renderWarningTable("overview-warnings-table", warnings.warnings || []);
}

function renderGateway(body) {
  const status = body.status || {};
  const metrics = body.metrics || {};
  const admin = status.admin_status || {};
  const counts = admin.counts || {};
  setHealth(status.status || "unknown");
  document.getElementById("gateway-summary").innerHTML = [
    metric("Status", status.status || "unknown"),
    metric("Readiness", value(status.gateway, "readiness")),
    metric("Audit Events", counts.audit_events ?? "0"),
    metric("Alerts", (status.alerts || []).length),
  ].join("");
  const components = (metrics.operations_metrics && metrics.operations_metrics.components) || {};
  document.getElementById("gateway-metrics-table").innerHTML = objectRows(components).length
    ? table(
        ["Component", "State", "Snapshot"],
        objectRows(components).map(([name, component]) => [
          name,
          badge(component.enabled ? "enabled" : "disabled", component.enabled ? "ok" : "warn"),
          component.snapshot ? compactJSON(component.snapshot) : "",
        ]),
      )
    : empty("No metrics components");
}

function renderMetadata(body) {
  const admin = (body.status && body.status.admin_status) || {};
  const metadata = admin.metadata || {};
  const counts = admin.counts || {};
  document.getElementById("metadata-summary").innerHTML = [
    metric("Backend", metadata.backend || "unknown"),
    metric("KMS Keys", counts.kms_keys ?? "0"),
    metric("GC Ops", counts.gc_operations ?? "0"),
    metric("Dedupe Ops", counts.dedupe_operations ?? "0"),
  ].join("");
  document.getElementById("metadata-table").innerHTML = table(
    ["Field", "Value"],
    [
      ["Path", metadata.path || ""],
      ["TiKV PD", join(metadata.tikv_pd_endpoints)],
      ["TiKV API", metadata.tikv_api_version || ""],
      ["TiKV Keyspace", metadata.tikv_keyspace || ""],
      ["Audit Chain", auditChain(admin.audit_chain)],
      ["Capabilities", compactJSON(admin.capabilities || {})],
    ],
  );
}

function renderSBS(body) {
  const cluster = body.cluster || {};
  const nodes = body.nodes || {};
  const stores = body.stores || {};
  const volumes = body.volumes || {};
  const maintenance = body.maintenance || {};
  document.getElementById("sbs-summary").innerHTML = [
    metric("Status", cluster.status || "unknown"),
    metric("Source", cluster.source_authority || "fallback"),
    metric("Nodes", value(cluster.nodes, "total", 0)),
    metric("Volumes", value(cluster.volumes, "total", 0)),
  ].join("");
  document.getElementById("sbs-nodes-table").innerHTML = (nodes.nodes || []).length
    ? table(
        ["Node", "Role", "State", "Endpoint"],
        (nodes.nodes || []).map((node) => [node.node_id || "", node.role || "", badge(node.state || "unknown", tone(node.state)), node.endpoint || ""]),
      )
    : empty("No SBS node details from NAMRBD source");
  document.getElementById("sbs-stores-table").innerHTML = (stores.stores || []).length
    ? table(
        ["Store", "Node", "State", "Used", "Available"],
        (stores.stores || []).map((store) => [
          store.store_id || "",
          store.node_id || "",
          badge(store.state || "unknown", tone(store.state)),
          formatBytes(store.used_bytes),
          formatBytes(store.available_bytes),
        ]),
      )
    : empty("No SBS store details from NAMRBD source");
  document.getElementById("sbs-volumes-table").innerHTML = (volumes.volumes || []).length
    ? table(
        ["Volume", "State", "Read-only", "Used", "High Watermark"],
        (volumes.volumes || []).map((volume) => [
          volume.volume_id || "",
          badge(volume.state || "unknown", tone(volume.state)),
          volume.read_only ? "yes" : "no",
          percent(volume.used_percent),
          percent(volume.high_watermark_percent),
        ]),
      )
    : empty("No SBS volume details from NAMRBD source");
  document.getElementById("sbs-maintenance-table").innerHTML = table(
    ["Repair", "Rebalance", "Stuck", "Unknown"],
    [[maintenanceValue(maintenance, "repair_running"), maintenanceValue(maintenance, "rebalance_running"), maintenanceValue(maintenance, "stuck"), maintenanceValue(maintenance, "unknown")]],
  );
}

function renderCapacity(body) {
  const capacity = (body.capacity && body.capacity.capacity) || {};
  const reclaim = (body.reclaim && body.reclaim.reclaim) || {};
  document.getElementById("capacity-summary").innerHTML = [
    metric("Total", formatBytes(capacity.total_bytes)),
    metric("Physical Used", formatBytes(capacity.used_bytes)),
    metric("Physical Free", formatBytes(capacity.available_bytes)),
    metric("Unknown", formatBytes(capacity.unknown_bytes)),
  ].join("");
  document.getElementById("capacity-table").innerHTML = table(
    ["Field", "Value"],
    [
      ["Logical Bytes", formatBytes(capacity.logical_bytes)],
      ["Reclaimable", formatBytes(capacity.reclaimable_bytes)],
      ["Used Percent", percent(capacity.used_percent)],
      ["Stores", capacity.stores_total ?? 0],
      ["Volumes", capacity.volumes_total ?? 0],
      ["Source", capacity.source || "namrbd_sbs_observability"],
    ],
  );
  document.getElementById("reclaim-table").innerHTML = table(
    ["Field", "Value"],
    [
      ["Status", badge(reclaim.status || "unknown", tone(reclaim.status))],
      ["Candidates", reclaim.candidates ?? 0],
      ["Running", reclaim.running ?? 0],
      ["Completed", reclaim.completed ?? 0],
      ["Failed", reclaim.failed ?? 0],
      ["Blocked", reclaim.blocked ?? 0],
      ["Limitations", join(reclaim.limitations)],
    ],
  );
}

function renderObjectExplorer(body) {
  const bucketsBody = body.buckets || {};
  const clientsBody = body.clients || {};
  const objectsBody = body.objects || {};
  const buckets = bucketsBody.buckets || [];
  const clients = clientsBody.clients || [];
  const objects = objectsBody.objects || [];
  const prefixes = objectsBody.common_prefixes || [];
  document.getElementById("object-explorer-summary").innerHTML = [
    metric("Buckets", buckets.length),
    metric("Selected Bucket", body.selected_bucket || "none"),
    metric("Top-level Entries", objects.length + prefixes.length),
    metric("Payload Access", "disabled"),
  ].join("");
  document.getElementById("object-explorer-buckets-table").innerHTML = buckets.length
    ? table(
        ["Name", "Region", "Versioning", "Object Lock", "Encryption"],
        buckets.map((bucket) => [
          bucket.name || "",
          bucket.region || "",
          bucket.versioning_state || "Unversioned",
          bucket.object_lock && bucket.object_lock.enabled ? badge("enabled", "warn") : "disabled",
          (bucket.default_encryption && bucket.default_encryption.algorithm) || "none",
        ]),
      )
    : empty("No buckets visible to Object Explorer Lite");
  const prefixRows = prefixes.map((prefix) => ["prefix", prefix, "", "", ""]);
  const objectRows = objects.map((object) => ["object", object.key || "", object.size ?? "", object.etag || "", object.last_modified || ""]);
  document.getElementById("object-explorer-objects-table").innerHTML = prefixRows.length || objectRows.length
    ? table(["Kind", "Key/Prefix", "Size", "ETag", "Modified"], prefixRows.concat(objectRows))
    : empty(body.selected_bucket ? "No top-level objects in selected bucket" : "Create a bucket to browse metadata");
  document.getElementById("object-explorer-clients-table").innerHTML = clients.length
    ? table(
        ["Tool", "Kind", "Compatibility", "Bundled", "Use"],
        clients.map((client) => [
          client.name || client.id || "",
          client.kind || "",
          client.compatibility_target ? badge("target", "ok") : "optional",
          client.bundled ? "yes" : "no",
          client.recommended_use || "",
        ]),
      )
    : empty("No external client recipes");
}

function renderAlerts(body) {
  const alerts = body.alerts || {};
  const datasources = body.datasources || {};
  const warnings = body.warnings || {};
  document.getElementById("alerts-summary").innerHTML = [
    metric("Active Alerts", (alerts.alerts || []).length),
    metric("Catalog", (alerts.catalog || []).length),
    metric("Warnings", warnings.warning_count ?? (warnings.warnings || []).length),
    metric("Datasources", (datasources.datasources || []).length),
  ].join("");
  renderWarningTable("alerts-warnings-table", warnings.warnings || []);
  document.getElementById("datasources-table").innerHTML = (datasources.datasources || []).length
    ? table(
        ["Kind", "Status", "Base URL", "Local Path"],
        (datasources.datasources || []).map((source) => [source.kind || "", badge(source.status || "unknown", tone(source.status)), source.base_url || "", source.local_path || ""]),
      )
    : empty("No datasources");
}

function renderEvidence(body) {
  const reports = body.reports || {};
  const runbooks = body.runbooks || {};
  const reportRows = (reports.reports || []).map((report) => {
    const artifact = report.artifact || {};
    const blocked = report.enterprise_required;
    return [
      report.kind || "",
      report.minimum_edition || "",
      blocked ? badge("enterprise", "warn") : badge(artifact.found ? "found" : "missing", artifact.found ? "ok" : "warn"),
      blocked ? blocked.error || "Enterprise required" : artifact.path || artifact.directory || "",
    ];
  });
  document.getElementById("reports-table").innerHTML = reportRows.length ? table(["Kind", "Edition", "State", "Artifact"], reportRows) : empty("No reports");
  const runbookList = (runbooks.runbooks && runbooks.runbooks.runbooks) || runbooks.runbooks || [];
  document.getElementById("runbooks-table").innerHTML = Array.isArray(runbookList) && runbookList.length
    ? table(
        ["ID", "Title", "Source", "HTML"],
        runbookList.map((runbook) => [runbook.id || "", runbook.title || "", runbook.path || "", runbook.html_path || ""]),
      )
    : empty("No runbooks");
}

function renderOperations(body) {
  const operations = body.operations || {};
  const hardening = body.hardening || {};
  document.getElementById("operations-summary").innerHTML = [
    metric("Apply Supported", hardening.workflow && hardening.workflow.apply_supported ? "yes" : "no"),
    metric("Plan-only", hardening.workflow && hardening.workflow.plan_only_operations ? "yes" : "no"),
    metric("Auth Mode", hardening.workflow && hardening.workflow.session_auth_required ? "local" : "disabled"),
    metric("Audit Mode", (hardening.workflow && hardening.workflow.audit_mode) || "sync"),
  ].join("");
  const rows = (operations.operations || []).map((operation) => [
    operation.name || "",
    operation.feature_id || "",
    operation.mode || "",
    badge(operation.available ? "available" : "blocked", operation.available ? "ok" : "warn"),
    operation.summary || "",
  ]);
  document.getElementById("operations-table").innerHTML = rows.length ? table(["Tool", "Feature", "Mode", "State", "Summary"], rows) : empty("No operations");
  document.getElementById("workflow-hardening-table").innerHTML = table(["Field", "Value"], objectRows(hardening.workflow || {}));
  document.getElementById("disabled-actions-table").innerHTML = table(["Action", "State"], objectRows(hardening.disabled_actions || {}));
}

function renderSettings(body) {
  const views = body.views || {};
  const gui = body.gui || {};
  const hardening = body.hardening || {};
  document.getElementById("settings-summary").innerHTML = [
    metric("Views", (views.views || []).length),
    metric("Mount", value(gui.console, "mount_path")),
    metric("Refresh", `${value(gui.console, "refresh_interval_secs", 0)}s`),
    metric("Mode", value(gui.console, "public_open_mode")),
  ].join("");
  document.getElementById("query-views-table").innerHTML = (views.views || []).length
    ? table(
        ["View", "API", "Schema", "Source", "State"],
        (views.views || []).map((view) => [view.id || "", view.path || "", view.schema_version || "", view.source_authority || "", badge(view.status || "unknown", view.available ? "ok" : "warn")]),
      )
    : empty("No query views");
  document.getElementById("gui-navigation-table").innerHTML = (gui.navigation || []).length
    ? table(
        ["ID", "Label", "API"],
        (gui.navigation || []).map((item) => [item.id || "", item.label || "", item.api || ""]),
      )
    : empty("No GUI navigation entries");
  document.getElementById("settings-hardening-table").innerHTML = table(["Field", "Value"], objectRows(hardening.workflow || {}));
}

function renderWarningTable(targetID, warnings) {
  document.getElementById(targetID).innerHTML = warnings.length
    ? table(
        ["Severity", "ID", "Message"],
        warnings.map((warning) => [badge(warning.severity || "unknown", tone(warning.severity)), warning.id || "", warning.message || ""]),
      )
    : empty("No warnings");
}

function renderError(name, error) {
  const target = document.querySelector(`#view-${name} .panel div, #view-${name} .summary-grid`);
  if (target) {
    target.innerHTML = `<div class="error-line">${escapeHTML(error.message)}</div>`;
  }
  if (name === "overview" || name === "gateway") {
    setHealth("error");
  }
}

function setHealth(status) {
  document.getElementById("health-text").textContent = status;
  document.getElementById("health-dot").className = `dot ${tone(status)}`;
}

function setReadOnlyLine(body) {
  const target = document.getElementById("readonly-line");
  if (!target) return;
  target.textContent = `${body.operation_surface || "read_only"} / ${body.source_authority || "namros_console"}`;
}

function metric(label, valueText) {
  return `<div class="metric"><div class="metric-label">${escapeHTML(label)}</div><div class="metric-value">${escapeHTML(String(valueText ?? ""))}</div></div>`;
}

function table(headers, rows) {
  return `<table><thead><tr>${headers.map((header) => `<th>${escapeHTML(header)}</th>`).join("")}</tr></thead><tbody>${rows
    .map((row) => `<tr>${row.map((cell) => `<td>${trusted(cell)}</td>`).join("")}</tr>`)
    .join("")}</tbody></table>`;
}

function badge(valueText, badgeTone) {
  return `<span class="badge ${badgeTone || ""}">${escapeHTML(String(valueText ?? ""))}</span>`;
}

function empty(valueText) {
  return `<div class="empty">${escapeHTML(valueText)}</div>`;
}

function auditChain(chain) {
  if (!chain) return "";
  return `${chain.sampled || 0} sampled / hashes ${chain.hashes_present ? "present" : "missing"}`;
}

function join(input) {
  return Array.isArray(input) ? input.join(", ") : input || "";
}

function compactJSON(input) {
  return JSON.stringify(input);
}

function objectRows(input) {
  return Object.entries(input || {}).map(([key, rowValue]) => [key, typeof rowValue === "object" && rowValue !== null ? compactJSON(rowValue) : String(rowValue ?? "")]);
}

function capacityLine(capacity) {
  return `${formatBytes(capacity.used_bytes)} used / ${formatBytes(capacity.total_bytes)} total`;
}

function reclaimLine(reclaim) {
  return `${formatBytes(reclaim.reclaimable_bytes)} / ${reclaim.candidates ?? 0} candidates`;
}

function formatBytes(input) {
  const valueNumber = Number(input || 0);
  if (!Number.isFinite(valueNumber) || valueNumber <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB", "PiB"];
  let value = valueNumber;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
}

function percent(input) {
  const valueNumber = Number(input || 0);
  if (!Number.isFinite(valueNumber) || valueNumber === 0) return "";
  return `${valueNumber.toFixed(1)}%`;
}

function maintenanceValue(input, key) {
  return input && input.maintenance ? input.maintenance[key] ?? 0 : input[key] ?? 0;
}

function value(input, key, fallback = "unknown") {
  return input && input[key] !== undefined && input[key] !== null && input[key] !== "" ? input[key] : fallback;
}

function tone(status) {
  switch (String(status || "").toLowerCase()) {
    case "ok":
    case "ready":
    case "healthy":
    case "enabled":
    case "configured":
    case "low":
      return "ok";
    case "error":
    case "not_ready":
    case "failed":
    case "high":
      return "error";
    default:
      return "warn";
  }
}

function trusted(valueText) {
  if (typeof valueText === "string" && valueText.startsWith("<span")) {
    return valueText;
  }
  return escapeHTML(String(valueText ?? ""));
}

function escapeHTML(valueText) {
  return String(valueText)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#039;");
}

M23 Operations <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS Web Console & Monitoring Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition read-only dashboard behavior and Enterprise edition only approved operations, compliance, chaos/soak, and feature-panel sections. Enterprise-only panels should render edition-boundary messages in public Community builds.

</div>

<div class="summary" markdown="1">

The web console is not a separate source of truth replacing CLI or MCP utilities. Instead, it serves as a browser-based management portal executing stable administrative APIs, debugging endpoints, reporting schemas, and MCP operation wrappers.

SBS node, store, volume, capacity, reclaim, and maintenance data is consumed from NAMRBD-owned read-only observability surfaces. NAMROS does not implement separate SBS drain, remove, rejoin, repair, rebalance, or reclaim behavior in the public console.

</div>

## Console Access And Operating Posture

The embedded operations GUI is served by each `namros-gateway` at `/console/`. In a public deployment, expose the console through the same hostname or load balancer used for gateway operations, for example `https://namros.example.com/console/`. Multi-gateway deployments can health-check `/readyz` and forward browser traffic to `/console/` on healthy gateway instances.

The public Community posture is data-centered and read-only: operators can inspect status, alerts, reports, metrics summaries, Object Explorer Lite metadata, and NAMRBD-sourced SBS observability. Mutation controls, repair workflows, SBS maintenance actions, and Enterprise feature pages must be absent, disabled, or rendered as edition-boundary messages unless a separately reviewed operation workflow enables them.

## Datasource And SBS Observability Configuration

| Setting | Gateway flag | Environment variable | Purpose |
| --- | --- | --- | --- |
| Prometheus | `-observability-prometheus-url` | `NAMROS_OBSERVABILITY_PROMETHEUS_URL` | Console deep link and datasource descriptor for Prometheus queries. |
| Grafana | `-observability-grafana-url` | `NAMROS_OBSERVABILITY_GRAFANA_URL` | Console deep link target for provisioned dashboards. Missing values surface the `grafana_unconfigured` warning. |
| VictoriaMetrics | `-observability-victoria-url` | `NAMROS_OBSERVABILITY_VICTORIA_URL` | Optional long-retention metrics datasource. Leave empty when no VictoriaMetrics deployment is available. |
| NAMRBD SBS observability | `-namrbd-sbs-observability-endpoint` | `NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT` | Read-only source for SBS cluster, node, volume, capacity, reclaim, and maintenance projections. |
| NAMRBD SBS timeout | `-namrbd-sbs-observability-timeout` | `NAMROS_NAMRBD_SBS_OBSERVABILITY_TIMEOUT` | HTTP collection timeout for NAMRBD SBS observability. |

If `NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT` is empty, SBS panels report an unconfigured or partial source until the endpoint is supplied. Use a resolvable service name, gateway-local network route, or orchestrator service URL that is reachable from every gateway instance.

## Public Deployment Example

Set the console datasource links through the gateway service environment or matching command-line flags:

```sh
export NAMROS_OBSERVABILITY_PROMETHEUS_URL=https://prometheus.example.com
export NAMROS_OBSERVABILITY_GRAFANA_URL=https://grafana.example.com
export NAMROS_OBSERVABILITY_VICTORIA_URL=https://victoria.example.com
export NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT=https://namrbd-sbs-observability.example.com
export NAMROS_NAMRBD_SBS_OBSERVABILITY_TIMEOUT=30s
```

For load-balanced fleets, configure each gateway with the same external datasource URLs and an SBS observability endpoint reachable from the gateway process. The load balancer should route `/console/` traffic to healthy gateways and use `/readyz` for health checks.

## Edition Scope

| Panel/action | Community | Enterprise |
| --- | --- | --- |
| Gateway/metadata/storage health | Read-only dashboard | Read-only dashboard |
| SBS operations data | NAMRBD observability adapter, read-only only | NAMRBD observability adapter, read-only plus separately approved NAMRBD workflows |
| Report viewer | Compatibility/release/backup reports | Compliance/chaos/soak reports included |
| Object Explorer Lite | Read-only bucket, prefix, object metadata, and external S3 browser recipes | Approved download/delete only after policy controls |
| Approved operations | Limited Community actions | <span class="badge enterprise">Enterprise edition only</span> repair/protect/evidence actions |
| Enterprise feature pages | Enterprise-required message | Enabled by distribution entitlement |

## Console API Candidate

| Endpoint | Purpose |
| --- | --- |
| `/api/v1/status` | Cluster, gateway, metadata, storage status. |
| `/api/v1/operations/summary` | Read-only overview combining gateway, metadata, SBS, alerts, reports, and object explorer state. |
| `/api/v1/operations/warnings` | Limitations and warnings surfaced for public read-only operations. |
| `/api/v1/query/views` | View catalog with schema version, source authority, and availability. |
| `/api/v1/gui/summary` | Console navigation, refresh policy, and datasource descriptors. |
| `/api/v1/workflow/hardening` | Read-only workflow boundary, disabled actions, approval posture, and audit settings. |
| `/api/v1/metrics` | Normalized operations metrics. |
| `/api/v1/reports` | Compatibility, release, backup, chaos/soak report index. |
| `/api/v1/operations` | Operation plan/preflight/apply/verify/audit history. |
| `/api/v1/edition` | Edition and entitlement catalog. |
| `/api/v1/sbs/cluster`, `/nodes`, `/stores`, `/volumes`, `/capacity`, `/reclaim`, `/maintenance` | NAMRBD-sourced SBS observability projection with read-only envelope. |
| `/api/v1/object-explorer/buckets` | Read-only bucket listing and operational status. |
| `/api/v1/object-explorer/objects` | Read-only prefix/object listing with pagination and version-aware shape. |
| `/api/v1/object-explorer/external-clients` | Redacted connection recipes for external S3 browser tools. |

## Dashboard Panels

- Gateway fleet health and etcd lease freshness.
- TiKV metadata status and transaction metrics.
- SBS replicated and Enterprise EC storage status from NAMRBD observability.
- SBS capacity and reclaim visibility without NAMROS-owned SBS mutation controls.
- Recent compatibility, release-readiness, and backup/restore reports.
- Alert summary for health degradation, quota threshold, and expected Enterprise denial.
- Object Explorer Lite with bucket/prefix/object metadata only.

## Object Explorer Boundary

Object Explorer Lite is a read-only operations view over stable S3/admin list and head surfaces. It must not decode private metadata directly, read segment-store payload bytes, or expose upload, copy/move, recursive delete, or bulk delete controls.

Full file browsing and transfer workflows should use validated external S3 tools documented in the [S3 object browser integration guide](s3-object-browser-integration-guide.md). Console responses and recipes must redact access key secrets, session tokens, KMS material, Authorization headers, and presigned URLs.

## Approval And Redaction

Every mutating or test-resource-creating action must render plan, preflight, apply, verify, and audit fields. When local console auth is enabled, mutating console API requests must include the session-derived `X-Namros-CSRF-Token` header. The console must redact access keys, Authorization headers, KMS material, presigned URLs, and object payload bytes.

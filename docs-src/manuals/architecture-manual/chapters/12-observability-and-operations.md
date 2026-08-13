Chapter 12 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# Observability And Operations

## Signals

- healthz
- admin status
- operations metrics
- reports

<div class="note" markdown="1">

**Edition scope.** This chapter includes Community edition health/status endpoints and Enterprise edition only observability surfaces for dedupe, KMS, replication, quota, compliance, and EC operations. Enterprise-only rows describe private-distribution contracts or Community denial boundaries.

</div>

## Endpoint Schemas

| Endpoint | Expected Fields | Use | Authority Level |
| --- | --- | --- | --- |
| `/healthz` | HTTP status | load balancer and liveness probes | process-local liveness only |
| `/readyz` | dependency readiness, metadata/storage reachability where configured | routing and rollout gates | aggregated process dependency view |
| `/debug/admin/status` | metadata backend identity, collection counts, capability flags, recent operations | operator status and MCP resource | metadata-derived product state |
| `/debug/operations/metrics` | TiKV, GC, dedupe worker/scheduler counters | release and incident tooling | metrics snapshot, not mutation authority |
| `/api/v1/operations/summary` | gateway, metadata, SBS source, alerts, reports, Object Explorer Lite | public read-only console first paint | aggregated read-only view |
| `/api/v1/query/views` | view id, schema version, source authority, availability | GUI and MCP view discovery | contract catalog, not data authority |
| `/api/v1/sbs/*` | cluster, nodes, stores, volumes, capacity, reclaim, maintenance | SBS observability in NAMROS console/report | NAMRBD-owned read-only source |
| `/debug/dedupe/status` | dedupe scheduler and worker snapshot | <span class="badge enterprise">Enterprise edition only</span> dedupe operations | operation state plus edition boundary |

All public console query APIs include a read-only envelope with source authority, redaction posture, visible limitations, and disabled mutation controls. SBS semantics come from NAMRBD observability; NAMROS does not implement separate SBS drain, remove, rejoin, repair, rebalance, or reclaim workflows.

## Operational State Model

| State Type | Where It Lives | Why Operators Care |
| --- | --- | --- |
| object namespace state | metadata backend | explains GET/HEAD/LIST outcomes and strong consistency failures |
| payload placement state | segment refs plus storage backend/SBS | explains range read failures, repair candidates, EC shard loss, and capacity pressure |
| SBS capacity/reclaim state | NAMRBD SBS observability API or metrics | explains capacity pressure while keeping SBS mutation authority outside NAMROS |
| coordination state | etcd leases/registry | explains gateway readiness, worker ownership, and failover behavior |
| operation progress | metadata operation records | explains resumability, retry status, and partial completion after process crash |
| audit/evidence state | metadata audit records and evidence packages | explains who changed policy/retention/key state and when |

## Metrics And Tracing Backlog

| Signal | Purpose |
| --- | --- |
| Prometheus metrics | Gateway, metadata, storage, NAMRBD SBS observability, replication, quota, KMS, dedupe, and EC dashboards. |
| TiKV retry snapshot | `/debug/tikv/metrics` and `/debug/operations/metrics` expose retry attempts, write conflicts, transient errors, exhausted retries, and backoff without object-key labels. |
| Audit log export | Admin operation, IAM decision, KMS key-state, Object Lock, and evidence chain review. |
| OpenTelemetry trace | Request path timing across gateway, metadata, storage, and external providers. |
| Health probe semantics | Separate liveness, readiness, dependency readiness, and load-balancer routing signals. |
| Benchmark artifact schema | Layered network/storage/S3/gateway/metadata reports for release and soak comparison. |

## Metric Cardinality Rules

Metrics must be useful under large bucket and tenant counts. Avoid object key, version id, request id, or raw client-provided metadata as labels. Use bounded labels such as operation family, status class, storage backend, storage class id, tenant class, gateway id, and error category. High-cardinality data belongs in logs, traces, audit events, or support bundles.

## Reports

| Artifact | Producer | Purpose |
| --- | --- | --- |
| compatibility report | `make compat-report` | external client matrix status |
| release readiness JSON/Markdown | `make release-readiness` | target status, duration, logs, git commit |
| metadata backup/restore smoke output | `make smoke-metadata-backup-restore` | export/import preflight evidence |

## Worker Operation Records

Long-running operations must be resumable. A worker can hold a lease, but durable progress belongs in metadata records such as GC operations, dedupe operations, metadata export/import results, lifecycle scans, and compliance evidence packages. Every attempt should record counts, skipped/protected items, retryable errors, and enough context for a later worker to continue safely.

| Worker | Records To Check | Common Failure Signature |
| --- | --- | --- |
| GC | GC candidates, protected refs, GC operation attempts | retryable deletes, protected skips, storage unavailable |
| Lifecycle | bucket lifecycle config, object versions, MPU records | unexpected delete marker, locked version, stale lifecycle rule |
| Dedupe | <span class="badge enterprise">Enterprise edition only</span> shared object refs, dedupe operations, pending releases | byte verify failure, protected-root blocked reclaim, refcount repair |
| KMS/compliance | <span class="badge enterprise">Enterprise edition only</span> KMS keys, audit events, compliance profile attachments | fail-closed key state, evidence limitation, missing time-source posture |

## Incident Triage

1. Capture request id, client command, endpoint, bucket/key, and timestamp.
2. Read `/healthz`, `/api/v1/operations/summary`, `/debug/admin/status`, `/debug/operations/metrics`.
3. Preserve client logs and smoke tmpdir when available.
4. Classify subsystem: client configuration, gateway routing/auth, metadata, storage, Enterprise dependency.
5. Use MCP observe tools before any operator-approved repair.

## Triage Matrix

| Symptom | First Authority To Check | Likely Next Check |
| --- | --- | --- |
| GET returns missing after successful PUT | metadata object head/version | gateway cache revision, list index, request id audit |
| GET returns `ServiceUnavailable` | segment ref placement and storage backend health | SBS chunk/shard availability, repair candidates, EC quorum |
| DELETE denied | object retention/legal hold/protected refs | governance bypass audit and storage delete admission |
| LIST differs between gateways | metadata list index and gateway cache behavior | cache invalidation, TiKV transaction errors, stale deployment |
| Enterprise feature denied | edition identity and capability flags | release boundary chapter, not a storage outage |

Reference: [admin guide](../../admin-guide.md) and [MCP operations guide](../../mcp-operations-guide.md).

Performance and chaos validation report shape is tracked in [the performance/chaos/soak guide](../../performance-chaos-soak-guide.md).

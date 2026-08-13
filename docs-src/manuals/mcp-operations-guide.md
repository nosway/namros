AI-assisted Operations <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS MCP Operations Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition observe/probe resources and Enterprise edition only MCP tools. Enterprise-only tools return standard Enterprise-required responses in public Community builds.

</div>

![MCP operations loop](architecture-manual/assets/diagrams/mcp-operations-loop.svg)

## Model

The MCP server distinguishes between observe mode and operate mode. Observe mode reads health, metrics, reports, and runbooks. Operate mode executes only explicitly approved bounded actions and verifies/audits the results.

MCP is not an automated recovery engine. The AI assistant can interpret states and propose operator-approved action plans, but state changes must pass authorization policies and audit envelopes.

## Install And Run Shape

```sh
namros-mcp \
  -mode observe \
  -gateway-endpoint http://127.0.0.1:9000 \
  -release-report-dir release-reports \
  -compat-report-dir compat-reports
```

Initial transport is stdio for local operator and desktop assistant use. Future HTTP/SSE transport must preserve the same approval and redaction rules.

## Resource Catalog

| Resource | Purpose | Edition |
| --- | --- | --- |
| `namros://product/edition` | build identity and capability flags | Community |
| `namros://gateway/health` | gateway readiness and endpoint status | Community |
| `namros://metadata/status` | metadata backend identity and collection counts | Community |
| `namros://operations/metrics` | GC/dedupe/scheduler metrics where available | Community |
| `namros://runbooks/index` | operator runbook catalog | Community |
| `namros://enterprise/ec/status` | EC/SBS health summary | Enterprise |

## Tool Classes

| Class | Default | Examples |
| --- | --- | --- |
| observe | allowed | health, admin status, release report |
| probe | approval required | compat smoke, gateway wait |
| repair | approval required | GC retry, dedupe scrub |
| protect | approval required | metadata backup, compliance evidence package |
| destructive | disabled | purge, governance bypass, crypto erase |

## MinIO-style Diagnostic Mapping

| Diagnostic area | MCP resource/tool candidate | Guide |
| --- | --- | --- |
| Replication | `namros.replication.status`, lag summary | [Replication guide](replication-disaster-recovery-guide.md) |
| Inventory/batch | `namros.inventory.status`, batch job report index | [Inventory guide](inventory-batch-operations-guide.md) |
| Quota/QoS | `namros.quota.status`, threshold alerts | [Quota guide](quota-qos-guide.md) |
| KMS | `namros.kms.status`, key-state summary | [KMS guide](kms-encryption-guide.md) |
| Chaos/soak | `namros.chaos_soak.latest` | [Soak guide](performance-chaos-soak-guide.md) |
| Support bundle | `namros.incident.bundle`, redacted report collection | [Admin guide](admin-guide.md) |

## Action Contract

All operator-approved tools must record 'plan', 'preflight', 'apply', 'verify', and 'audit' envelopes.

```json
{
  "schema_version": "namros.mcp.operation.v1",
  "operation_id": "op-...",
  "tool": "namros.compat.user_space.run",
  "risk_class": "probe",
  "mode": "operate",
  "approval": {
    "required": true,
    "policy": "external-token",
    "reference": "ticket-1234"
  },
  "plan": {},
  "preflight": {},
  "result": {},
  "verification": {},
  "audit": {
    "local_path": ".namros/mcp-operations/op-....json"
  }
}
```

## Secret Redaction And Incident Bundles

Access keys, secret keys, KMS material, presigned URLs, Authorization headers, and object payload bytes must not be emitted by default. Incident bundles should include redacted command lines, endpoint identity, status JSON, metrics JSON, runbook suggestion, and operation records.

## Example Sessions

| Situation | Observe | Approved Action | Verification |
| --- | --- | --- | --- |
| gateway readiness timeout | health, logs, admin status | `namros.gateway.health.wait` | health becomes 2xx or bundle captures timeout evidence |
| metadata backend failure | admin status, backend identity | metadata backup create if backend is reachable | collection counts stable |
| compat failure | latest compat report | `namros.compat.user_space.run` | client-specific passed/failed summary |
| Enterprise feature denied | edition resource | none by default | assistant explains expected Community behavior |

In Community builds, Enterprise-only MCP tools are blocked with standard NAMROS Enterprise Edition requirement errors.

Chapter 13 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# MCP Operations Provider

## MCP

- resources
- tools
- approval
- audit

<div class="note" markdown="1">

**Edition scope.** This chapter includes Community edition MCP resources/tools and Enterprise edition only operation resources. Enterprise-only MCP resources and tools should return edition-boundary guidance in public Community builds.

</div>

![MCP operations loop](../assets/diagrams/mcp-operations-loop.svg)

## Resource Catalog

| URI | Purpose | Backing Source | Edition |
| --- | --- | --- | --- |
| `namros://product/edition` | edition identity and capability flags | edition package plus release metadata | Community |
| `namros://gateway/health` | gateway readiness summary | `/healthz`, `/readyz`, local dependency checks | Community |
| `namros://metadata/status` | metadata backend status | `/debug/admin/status`, repository counts | Community |
| `namros://operations/recent` | recent GC/export/dedupe/compliance operation summary | metadata operation records | Community with Enterprise-only rows gated |
| `namros://storage/classes` | storage class catalog and capability explanation | storage class resolver and metadata snapshots | Community with EC denied where not entitled |
| `namros://enterprise/ec/status` | EC/SBS status | EC classroute and SBS shard health | <span class="badge enterprise">Enterprise edition only</span> |

## Tool Contract

Read-only tools are allowed by default. Probe/repair/protect tools require explicit operator approval. Destructive tools remain disabled until a separate policy model covers role, reason, dual control, audit retention, and recovery evidence.

| Tool Class | Default | Approval Requirement | Examples |
| --- | --- | --- | --- |
| observe | enabled | none beyond normal read authorization | read health, metadata status, recent operations, edition identity |
| probe | allowed with caution | explicit operator approval when it can increase load | rerun compatibility smoke, check storage range read, collect support bundle |
| repair/protect | disabled by default | operator approval, reason, bounded plan, audit event | retry GC, repair shared-object refcounts, attach compliance profile |
| destructive | disabled | separate policy model and dual control before enabling | force delete, bypass governance, crypto erase |

## Operation Envelope

Operator-approved tools must return a stable JSON envelope with `schema_version`, `operation_id`, `tool`, `risk_class`, `approval`, `plan`, `preflight`, `result`, `verification`, and `audit` fields. The envelope should be returned even on failure so incident bundles can capture partial evidence.

```json
{
  "schema_version": "namros.ops.v1",
  "operation_id": "op-...",
  "tool": "metadata.status",
  "risk_class": "observe",
  "approval": {"required": false},
  "plan": [],
  "preflight": {"metadata_reachable": true},
  "result": {"status": "ok"},
  "verification": {"checked_at": "..."},
  "audit": {"event_id": ""}
}
```

## Redaction

The MCP provider must redact access keys, secret keys, Authorization headers, KMS material, presigned URLs, and object payload bytes by default. Logs and object names are treated as untrusted input.

## Safety Boundaries

- MCP tools must not expose raw object payload bytes unless a dedicated, approved object-inspection tool is designed for that purpose.
- Presigned URLs, Authorization headers, access keys, KMS material, wrapped DEKs, and client-provided metadata that may contain secrets are redacted by default.
- An Enterprise-required response is a valid operational result, not an outage.
- Any tool that can change metadata must write the same audit and operation records that the admin CLI would write.

## Workflow Mapping

| Signal | Suggested Workflow |
| --- | --- |
| gateway not ready | health check, admin status, gateway readiness wait, incident bundle |
| metadata unreachable | admin status, backend endpoint check, backup only if reachable |
| compat failure | latest report, rerun approved smoke, preserve tmpdir |
| Enterprise feature denied | explain edition boundary; do not treat as outage |

## Developer Contract

MCP resources should be thin views over stable product interfaces, not private shortcuts into implementation details. When a resource reflects metadata state, it should name the repository collection or operation record it reads. When a tool mutates state, it should go through the same service layer and validation path as the admin CLI or gateway operation.

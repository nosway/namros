Large Namespace Operations <span class="badge planned">Planned specification</span>

# NAMROS Inventory & Batch Operations Guide

<div class="warning" markdown="1">

**Planned Enterprise specification.** This page describes proposed inventory
and batch-operation contracts. The current Community metadata export is a
building block, but no generally available scheduled inventory or approved
batch mutation service is implied.

</div>

For large namespaces, this document proposes periodic inventory materialization
and approved batch-operation workflows as an Enterprise direction. The schemas
below are design candidates unless the implementation-status table explicitly
marks a Community foundation.

## Implementation Status

| Area | Current public Community behavior | Planned specification status |
| --- | --- | --- |
| Metadata export | `namros-admin metadata-export` exports product metadata collections for backup, migration, and audit workflows. | Used as a building block for inventory evidence. |
| S3 Object Inventory | No scheduled public Community inventory worker is enabled. | Planned contract for periodic inventory materialization and report storage. |
| S3 Batch Operations | No public Community bulk mutation framework is enabled. | Planned contract for approved large-scale mutation jobs and audit envelopes. |

## Inventory Schema Candidate

| Field | Purpose |
| --- | --- |
| bucket/key/version | Object identity. |
| size/checksum/etag | Data verification and grouping. |
| storage class | Placement and lifecycle analysis. |
| encryption status | KMS posture and compliance evidence. |
| lock/retention status | WORM and delete safety. |
| replication status | DR lag and failure report. |

## Batch Job Types

| Job | Expected controls |
| --- | --- |
| Copy | Scope preview, conflict policy, KMS mapping. |
| Delete | Object Lock/protected-ref preflight and approval. |
| Tag | Policy simulation and change report. |
| Restore | Only after archive/tier restore exists. |

## Report And Audit

```text
job_id:
scope:
planned_count:
applied_count:
skipped_count:
failed_count:
audit_record:
report_path:
```

Batch jobs should use the same plan/preflight/apply/verify/audit envelope described in the [MCP operations guide](mcp-operations-guide.md).

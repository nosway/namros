Large Namespace Operations <span class="badge enterprise">Enterprise edition only</span>

# NAMROS Inventory & Batch Operations Guide

<div class="warning" markdown="1">

**Enterprise edition only.** This page describes Enterprise-only inventory and batch operation contracts. Community edition behavior is included only to document the currently available metadata export building block and the absence of scheduled inventory or batch workers.

</div>

In bucket environments with hundreds of millions of objects, standard S3 List APIs are insufficient to perform efficient full-asset inspection or state lifecycle reconciliation. To solve this, NAMROS Enterprise Edition defines automatic metadata auditing via S3 Object Inventory specifications, coupled with an S3 Batch Operations framework for large-scale bulk asset mutations.

## Implementation Status

| Area | Current public Community behavior | Enterprise/spec status |
| --- | --- | --- |
| Metadata export | `namros-admin metadata-export` exports product metadata collections for backup, migration, and audit workflows. | Used as a building block for inventory evidence. |
| S3 Object Inventory | No scheduled public Community inventory worker is enabled. | Enterprise contract for periodic inventory materialization and report storage. |
| S3 Batch Operations | No public Community bulk mutation framework is enabled. | Enterprise contract for approved large-scale mutation jobs and audit envelopes. |

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

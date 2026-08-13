Appendix <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# Interface Specifications

## Interfaces

- S3
- debug
- admin
- MCP

<div class="note" markdown="1">

**Edition scope.** This appendix inventories Community edition interfaces and Enterprise edition only reserved surfaces. Enterprise-only debug endpoints, CLI areas, or metadata fields describe private-distribution contracts or Community denial behavior.

</div>

## S3 Gateway

| Surface | Examples | Owner | Stable Contract |
| --- | --- | --- | --- |
| bucket/object API | PUT/GET/HEAD/DELETE/LIST | `internal/gateway`, `internal/s3api` | S3-compatible XML responses, request ids, error mapping |
| multipart API | CreateMultipartUpload, UploadPart, Complete, Abort, ListParts | `internal/gateway`, metadata repository | parts invisible until complete metadata publish |
| copy/tagging/versioning | CopyObject, tagging APIs, versioning APIs | gateway handlers and metadata | metadata-only updates and version lineage remain metadata-authoritative |
| Object Lock/KMS | retention, legal hold, SSE-KMS headers | <span class="badge enterprise">Enterprise edition only</span> gateway admission plus metadata | Community builds return Enterprise-required behavior |

## Metadata Repository

| Method Family | Representative Calls | Architectural Rule |
| --- | --- | --- |
| bucket config | `CreateBucket`, versioning, CORS, lifecycle, policy, Object Lock config | all gateways must observe the same bucket state after commit |
| object publish | `BeginPutObject`, `PutObjectVersion`, `CommitObjectVersion`, `DeleteObject` | object head, version record, list index, protected refs, and GC candidates move atomically |
| multipart | `CreateMultipartUpload`, `PutMultipartPart`, `CompleteMultipartUpload`, `AbortMultipartUpload` | part refs are staged state until complete publishes the final version |
| operations | GC, dedupe, metadata export/import, compliance evidence | long-running work is resumable from metadata records |

## Storage Interface

| Interface | Methods | Boundary |
| --- | --- | --- |
| `SegmentStore` | `PutSegment`, `GetSegment`, `DeleteSegment` | stores and retrieves bytes; does not decide S3 visibility |
| `SegmentValidator` | `ValidateSegment` | checks backend-specific availability/integrity before publish or repair |
| `OrphanTracker` | `MarkOrphan`, `ListGCCandidates` | records physical refs that need retryable cleanup |
| delete admission | protected-ref check, storage backend admission | must fail closed for Object Lock or unknown protection state |

## Debug Endpoints

| Endpoint | Purpose |
| --- | --- |
| `/healthz` | gateway readiness |
| `/debug/admin/status` | metadata/backend/capability status |
| `/debug/operations/metrics` | operation metrics aggregation |
| `/debug/dedupe/status` | <span class="badge enterprise">Enterprise edition only</span> dedupe scheduler status where available |

## Admin CLI

| Command Area | Purpose |
| --- | --- |
| metadata export/import/status | backup/restore and read-only status |
| dedupe | Enterprise plan/ack/scrub/repair/status |
| compliance | Enterprise evidence/profile/policy simulation |
| KMS | Enterprise key posture metadata |

## MCP

MCP resources and tools are summarized in the [MCP operations guide](../../mcp-operations-guide.md). Tool output must use stable schema versions and redact secrets.

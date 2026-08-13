Architecture <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS Architecture Manual

## Chapters

1. [Reading Guide](chapters/00-reading-guide.md)
2. [Product Overview](chapters/01-product-overview.md)
3. [S3 API And Compatibility](chapters/02-s3-api-and-compatibility.md)
4. [Gateway Stateless Active-Active](chapters/03-gateway-stateless-active-active.md)
5. [Metadata Authority](chapters/04-metadata-authority.md)
6. [Object And Segment Model](chapters/05-object-and-segment-model.md)
7. [Multipart Write Visibility](chapters/06-multipart-write-visibility.md)
8. [Backends](chapters/07-replicated-and-local-backends.md)
9. [SBS EC Enterprise](chapters/08-sbs-ec-backend-enterprise.md)
10. [Versioning Lifecycle Object Lock](chapters/09-versioning-lifecycle-object-lock.md)
11. [Dedupe Enterprise](chapters/10-dedupe-and-shared-objects-enterprise.md)
12. [Security Compliance Editions](chapters/11-security-compliance-and-editions.md)
13. [Observability Operations](chapters/12-observability-and-operations.md)
14. [MCP Operations Provider](chapters/13-mcp-operations-provider.md)
15. [Release Edition Boundaries](chapters/14-release-and-edition-boundaries.md)

## Appendices

- [Glossary](chapters/appendix-glossary.md)
- [Interface Specifications](chapters/appendix-interface-specifications.md)
- [Reference Map](chapters/appendix-reference-map.md)
- [Revision History](chapters/appendix-revision-history.md)

<div class="note" markdown="1">

**Edition scope.** This manual includes Community edition architecture and Enterprise edition only chapters/sections. Treat content marked <span class="badge enterprise">Enterprise edition only</span> as private-distribution behavior; Community behavior is included only when explicitly labeled or when denial semantics are described.

</div>

<div class="summary" markdown="1">

NAMROS expands to Network Attached Multipath Resilient Object Storage and is pronounced [nae-muh-ross].

This manual explains NAMROS as an S3-compatible object storage system with stateless gateways, authoritative metadata, object manifests, segment refs, local/SBS replicated storage, Enterprise-only EC/dedupe/WORM/KMS capabilities, and explicit Community/Enterprise boundaries.

</div>

![NAMROS platform overview](assets/diagrams/platform-overview.svg)

## Architecture In One Page

NAMROS separates S3 namespace authority from payload durability. Bucket, object head, object version, multipart, lifecycle, Object Lock, protected refs, shared objects, operation records, and evidence state live in NAMROS metadata. Payload bytes live behind `SegmentRef` values in a storage backend such as local segment storage, SBS replicated physical chunks, SBS volume pools, or <span class="badge enterprise">Enterprise edition only</span> SBS EC shards.

| Layer | Authority | Reader Takeaway |
| --- | --- | --- |
| S3 protocol | `namros-gateway` | routes requests, authenticates, maps errors, and orchestrates metadata/storage operations |
| Metadata | memory/Pebble/TiKV repository | decides object visibility and list consistency; TiKV is the distributed authority |
| Manifest | `ObjectVersion.SegmentRefs` | maps an S3-visible version to one or more storage refs |
| Storage | `SegmentStore` implementations and SBS | stores bytes, placement, range reads, delete admission, and repair signals |
| Operations | metadata operation records plus coordination leases | makes GC, lifecycle, dedupe, evidence, and repair resumable after process failure |

## Appendices

The appendices provide the glossary, interface inventory, source reference map, and revision history needed to review the HTML set against the maintained Markdown runbooks and code surfaces.

## Reading Guide

This manual is organized around ownership boundaries. NAMROS is easiest to review when each state transition is tied to the component that owns it and the backend where it becomes authoritative. S3 users, platform operators, architecture reviewers, and Enterprise feature reviewers can jump directly to their relevant sections from this page.

## Product Overview

NAMROS is Network Attached Multipath Resilient Object Storage, an S3-compatible object storage product. Its core product axes are stateless gateways, metadata-first correctness, Community active-active operation, local/SBS replicated segment storage, and an Enterprise path for EC, dedupe, WORM, KMS, and compliance evidence.

## S3 API And Compatibility

The gateway parses S3 paths, query subresources, methods, headers, and SigV4 context, then maps unsupported or invalid requests to S3-compatible XML errors. Compatibility decisions belong in the S3 gateway layer, while namespace mutation belongs in metadata repository transactions.

## Gateway Stateless Active-Active

Gateways do not own authoritative object state locally. Metadata cache is a read-through optimization and any write path that changes bucket or access-key state must invalidate local cache. Multiple gateways should see the same committed namespace through shared metadata and storage state after failover.

## Metadata Authority

Authoritative entities such as tenants, access keys, buckets, object heads, object versions, multipart uploads, lifecycle records, protected refs, shared objects, and operation records must be updated consistently inside metadata repository transaction boundaries. Pebble and memory backends support local validation, while TiKV provides distributed metadata authority.

## Object And Segment Model

The S3-visible unit is a committed object version. An object version points to a manifest, and the manifest points to segment references. Each `SegmentRef` carries storage class, placement, digest, encryption, and shared-object information so object keys do not become physical SBS addresses.

## Multipart Write Visibility

Multipart upload must keep a clear visibility boundary between individual part storage and complete publish. If part bytes are stored but metadata update fails, cleanup must record orphan candidates; if complete cannot verify storage state, it must fail before publishing the object head.

## Replicated And Local Backends

The local segment store is the Community baseline for development and compatibility validation. SBS replicated physical paths provide a Community production-like storage substrate where packaged, while <span class="badge enterprise">Enterprise edition only</span> SBS EC/classroute paths add storage-efficiency and degraded-read behavior.

## SBS EC Backend Enterprise

<span class="badge enterprise">Enterprise edition only</span> The SBS-backed Erasure Coding backend provides large-workload efficiency and fault tolerance through K+M geometry, class routing, shard placement, degraded reads, and healing workflows. Community builds must clearly reject EC classroute requests with Enterprise-required errors.

## Versioning Lifecycle Object Lock

Versioned buckets can hold multiple committed object versions and delete markers. Lifecycle planning must consider bucket rules, object versions, active MPUs, Object Lock state, and protected refs together; payload deletion must fail closed when active protected-ref lookup cannot be completed.

## Dedupe And Shared Objects Enterprise

<span class="badge enterprise">Enterprise edition only</span> Dedupe is an internal operation, not an S3-visible API. Hashes are candidate indexes only; byte verification, shared-object publish, object-version attach, protected-root accounting, and refcount repair stay inside metadata transaction and audit context.

## Security Compliance And Editions

<span class="badge enterprise">Enterprise edition only</span> NAMROS can provide control-plane evidence and enforcement surfaces for regulated workload patterns such as SEC, FINRA, CFTC, and HIPAA, but it must not claim legal certification by itself. Operators remain responsible for policy, deployment controls, external attestations, and regulatory interpretation.

## Observability And Operations

Health, report, audit, metrics, trace, and incident triage surfaces should let operators read current state and collect preflight evidence before risky work. Performance, chaos, and soak reports should capture reproducible artifacts and explicit limitations.

## MCP Operations Provider

The MCP provider should expose read-only resources and tools by default, while probe, repair, and protect tools require explicit operator approval. Destructive tools remain disabled until a separate model covers role, reason, dual control, audit retention, and recovery evidence.

## Release And Edition Boundaries

Community source has a fixed Community identity and must not expose an Enterprise runtime flag, environment switch, or public build tag that unlocks Enterprise behavior. Enterprise-only implementation packages are excluded from public source export or replaced with safe stubs.

## Glossary

The glossary keeps repeated terms and abbreviations such as gateway, metadata, segment, protected ref, and Enterprise marker consistent across the documentation set.

## Interface Specifications

Interface specifications collect the inputs, outputs, errors, and schema stability rules for the S3 gateway, debug endpoints, admin CLI, and MCP resource/tool surfaces.

## Reference Map

The reference map connects HTML documentation claims to maintained Markdown runbooks, source packages, scripts, and compatibility targets.

## Revision History

The revision history tracks how the HTML set started and which operations guides and Enterprise feature axes were added over time. Known gaps remain as inputs for future documentation expansion.

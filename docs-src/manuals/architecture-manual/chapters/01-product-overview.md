Chapter 01 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# Product Overview

## Overview

- object storage identity
- components
- control/data plane
- edition split

<div class="note" markdown="1">

**Edition scope.** This chapter describes the Community edition product baseline and the Enterprise edition only feature path. Enterprise EC, dedupe, WORM, KMS, and compliance references are private-distribution capabilities unless the text explicitly describes Community denial behavior.

</div>

![NAMROS platform overview](../assets/diagrams/platform-overview.svg)

## Goal

NAMROS expands to Network Attached Multipath Resilient Object Storage and is pronounced [nae-muh-ross]. It presents an S3-compatible object storage API. The product goal is a dependable object gateway with normal client compatibility, metadata-first correctness, Community active-active operation, and a clearly marked path to Enterprise EC, dedupe, WORM, KMS, and compliance evidence features.

<div class="summary" markdown="1">

The central architecture rule is simple: NAMROS metadata decides what object version is visible to S3 clients; SBS or another segment backend decides where bytes are durably stored. A gateway may orchestrate both sides, but it must not turn local process state into an object authority.

</div>

## Non-goals

- NAMROS is not a block-device product. NAMRBD covers that domain.
- NAMROS does not claim every AWS S3 API. The target is a documented compatibility scope.
- NAMROS does not claim regulatory certification. It provides controls and evidence workflows that operators can use in regulated environments.

## Architecture Contract

| Question | NAMROS Answer | Implication |
| --- | --- | --- |
| Where is an S3 object key authoritative? | In NAMROS metadata: bucket, object head, object version, list index, multipart state, Object Lock state, and operation records. | Listing and read-after-write consistency are metadata properties, not SBS directory scans. |
| Where are object bytes authoritative? | In the configured segment store: local store, SBS physical replicated backend, SBS volume pool, or <span class="badge enterprise">Enterprise edition only</span> SBS EC backend. | Object manifests hold references to bytes; they do not embed payload data. |
| What is the S3 visibility point? | The metadata transaction that publishes an object version and updates the object head/list index. | A successful storage write without metadata publish is an orphan candidate, not a visible S3 object. |
| Can a gateway own object state? | No. A gateway can cache read-heavy metadata, stream bytes, and hold temporary request state only. | Any healthy gateway can serve a committed object after failover. |

## Primary Components

| Component | Role | Authoritative State |
| --- | --- | --- |
| S3 clients | AWS CLI, mc, rclone, s3fs-fuse, SDKs | none |
| namros-gateway | request routing, auth, XML errors, S3 compatibility behavior | no durable object authority |
| metadata backend | bucket, object version, MPU, lifecycle, operations | authoritative namespace state |
| segment store | payload bytes | physical segment existence only |
| etcd | <span class="badge">Community</span> gateway registry and health lease | gateway presence state |
| SBS/TiKV | <span class="badge">Community</span> distributed metadata and replicated storage; <span class="badge enterprise">Enterprise edition only</span> EC storage | distributed metadata and physical shard state |

## Plane Responsibilities

| Plane | Responsibilities | Examples |
| --- | --- | --- |
| Protocol plane | S3 routing, SigV4/auth, path-style and virtual-hosted-style addressing, S3 XML response and error mapping. | `namros-gateway`, routing handlers, client compatibility behavior. |
| Metadata plane | Authoritative namespace mutation and ordered scans. | Bucket uniqueness, object head, version records, list index, MPU records, lifecycle state. |
| Storage plane | Payload persistence, placement, range read, delete, repair, and backend health. | Local segments, SBS replicated chunks, SBS EC shards, volume pool routing. |
| Coordination plane | Gateway registry, health leases, background worker ownership, and long-running operation leases. | <span class="badge">Community</span> etcd gateway registry and worker lease patterns. |
| Operations plane | Admin status, backup/restore, GC, dedupe, compliance evidence, MCP observation, metrics, and audit records. | Debug endpoints, admin CLI, operation records, support bundles. |

## End-to-end Object Path

1. An S3 client sends a request to any healthy gateway.
2. The gateway authenticates, resolves the bucket, evaluates request headers, and chooses a storage class snapshot.
3. For writes, payload bytes are streamed into a segment store and returned as one or more `SegmentRef` records.
4. The metadata transaction publishes an `ObjectVersion`, updates `ObjectHead`, updates the list index, records Object Lock/protected refs when required, and records retry/idempotency state.
5. For reads, metadata selects the current object version and its segment refs; the gateway streams exact bytes from the storage backend to the client.

## Community And Enterprise Capabilities

| Area | Community | Enterprise |
| --- | --- | --- |
| S3 compatibility | core object workflows and client smoke | same plus operational scale features |
| metadata | memory/Pebble local paths and TiKV distributed backend | same metadata model plus private compliance/evidence extensions where entitled |
| gateway fleet | single gateway or active-active gateways with shared metadata/storage and etcd registry | same pattern with additional private operations and compliance controls |
| storage | local segment store and SBS replicated physical storage where packaged | <span class="badge enterprise">Enterprise edition only</span> SBS EC/classroute storage classes and advanced healing |
| governance/compliance | Enterprise-required denial | WORM, KMS posture, evidence packages, policy simulation |

## Code Orientation

| Concept | Primary Source Area | Why Developers Read It |
| --- | --- | --- |
| S3 routing and handlers | `internal/gateway` | Protocol behavior, S3 error mapping, object request orchestration. |
| Metadata contract | `internal/meta`, `internal/meta/model` | Repository interface, bucket/object/multipart/protected-ref records. |
| Segment abstraction | `internal/storage` | `SegmentStore`, `SegmentRef`, storage class and placement snapshots. |
| SBS integration | `internal/storage/sbs`, `internal/storage/classroute`, `internal/storage/volumepool` | Replicated physical chunks, EC shard paths, and per-ref storage routing. |
| Background operations | `internal/gc`, `internal/dedupe` | Orphan cleanup, protected delete admission, dedupe planning and worker flow. |

Reference: [release and edition boundaries](14-release-and-edition-boundaries.md).

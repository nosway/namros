Chapter 07 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# Replicated And Local Backends

## Backends

- local
- SBS physical
- volume pools
- GC

<div class="note" markdown="1">

**Edition scope.** This chapter describes Community edition local and SBS replicated storage interfaces while calling out Enterprise edition only EC/classroute behavior. Enterprise-only backend paths are not public Community functionality except for documented denial behavior.

</div>

## Local Segment Store

The local segment store is the Community baseline. It stores payload bytes under a configured directory and supports put, get, range read, and delete through the storage interface. It is appropriate for development and compatibility validation, not distributed durability.

## Backend Options

| Backend | Edition Scope | How A SegmentRef Locates Bytes | Best Use |
| --- | --- | --- | --- |
| local segment store | <span class="badge">Community</span> | segment id maps to a local file/object under the configured storage directory | development, unit tests, user-space S3 compatibility smoke |
| SBS physical replicated path | <span class="badge">Community</span> where packaged with SBS endpoints | placement snapshot records volume/chunk spans and replicated backend details | distributed lab and production-like replicated object storage |
| SBS volume pool path | <span class="badge">Community</span> target shape | segment ref carries a volume id or pool-routed placement snapshot so any gateway can read it | multi-gateway shared storage with per-ref routing |
| SBS EC/classroute path | <span class="badge enterprise">Enterprise edition only</span> | placement snapshot records EC shard layout and stripe profile | large object efficiency, degraded read, EC repair |

## Storage Interface Contract

| Operation | Requirement | Metadata Interaction |
| --- | --- | --- |
| Put | write bytes and return a segment reference | metadata later publishes manifest |
| Get/Range | return exact bytes for committed segment refs | metadata selects refs |
| Delete | remove physical bytes when safe | metadata checks protected refs first |
| Verify | validate digest or backend-specific checksum when available | complete path may require it before publish |

## SBS Physical Path

SBS physical paths reuse NAMRBD substrate capabilities for object payload storage. Gateway code should still depend on the NAMROS storage abstraction rather than raw SBS metadata. This keeps S3 semantics owned by NAMROS metadata.

A replicated object write flows through `SegmentStore.PutSegment`. The SBS adapter writes one or more physical chunks and returns a `SegmentRef` containing a placement snapshot. The metadata publish stores that ref on the object version. Reads later use the stored ref, not the bucket/key, to route back to the correct SBS volume and chunk spans.

```text
PutObject payload
  -> storage class STANDARD
  -> sbs-physical PutSegment
  -> SegmentRef
       SegmentID: physical segment id
       StorageClass: STANDARD / sbs-physical
       Placement: volume id, chunk offsets, length, profile generation
  -> ObjectVersion.SegmentRefs[]
  -> ObjectHead.VersionID
```

## Shared Volume Pool Rule

In an active-active deployment, a gateway must not write to a private volume namespace that only it can read. Every committed segment ref must route to a shared logical storage pool visible to every gateway in the deployment. A gateway can have a local connection/session, but the committed ref must contain enough volume or pool routing data for a peer gateway to read the same object after failover.

| Pattern | Correctness | Reason |
| --- | --- | --- |
| private per-gateway object volume | invalid | another gateway cannot reliably read committed refs after failover |
| single shared lab volume | valid for smoke, not final scale shape | proves correctness but concentrates placement and capacity |
| shared logical volume pool with per-ref routing | target shape | supports scale-out while preserving manifest readability across gateways |

## Volume Pool Capacity Model

A volume pool is a NAMROS logical write-admission and read-routing construct over SBS capacity. Buckets and objects do not own pool capacity directly. New object writes select an active pool member and then record the chosen volume or EC placement in the object version's segment refs. Existing object versions continue to read from their original placement after the pool grows.

| Question | Answer |
| --- | --- |
| How does a bucket get more space? | Increase the capacity of the volume pool used by the gateway/storage-class policy. The bucket does not need to move to a new volume or store a new fixed volume id. |
| How is a volume pool expanded? | Add or enlarge SBS capacity through the SBS/NAMRBD operational workflow, create or materialize the new SBS volume/member, then register it in NAMROS metadata as an active pool member for the relevant storage class. |
| When can new objects use the capacity? | After the pool registry update commits, gateways observe the new generation through refresh or restart, and write admission sees the member as active and healthy. |
| Do old objects move automatically? | No. Old versions keep their recorded placement. Rebalancing, drain, or lifecycle transition is a separate worker/operation so reads remain stable and auditable. |

1. Prepare SBS physical capacity: add disks/nodes or create a new replicated/EC-capable SBS volume according to the SBS runbook.
2. Verify SBS health and available capacity through SBS observability before exposing it to object writes.
3. Register the new member in the NAMROS volume-pool metadata registry, including volume id, data endpoint, state, weight, and supported storage classes such as `STANDARD` or <span class="badge enterprise">Enterprise edition only</span> `EC_4_2`.
4. Wait for gateways to refresh the pool generation or roll them so every gateway can route reads and writes consistently.
5. Run cross-gateway PUT/GET/LIST and failover smoke before raising production traffic or changing admission weights.

## Protected Delete Hook

Before deleting a physical segment, the gateway/worker must check active protected refs. If protected-ref lookup fails, deletion must fail closed. This protects Object Lock/WORM and shared-root workflows from accidental payload removal.

## Repair And Validation Signals

Storage backends can detect readback failure, checksum mismatch, missing chunks, or backend unavailability. Those signals should become repair candidates or retryable operation records, but they should not automatically delete metadata. Object metadata remains the reachability authority until an operator or worker completes a protected, auditable repair/reclaim flow.

## Backend Tests

Storage conformance tests should validate exact readback, range reads, delete idempotency where intended, and corruption/digest failure behavior. GC tests should verify that retryable delete failures remain visible as candidates.

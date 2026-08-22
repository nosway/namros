Operations <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS Capacity Scaling & Maintenance Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition gateway scale-out, TiKV operations, and SBS replicated volume-pool operations plus Enterprise edition only EC healing and dedupe repair contracts. Direct single-volume SBS attachment is a development or compatibility validation path. Enterprise-only repair commands remain edition-gated in public Community builds.

</div>

<div class="summary" markdown="1">

This guide serves as an operations manual for stateless gateway scale-out, safe decommissioning of TiKV metadata nodes, and replicated SBS volume-pool maintenance. Erasure Coding (EC) healing and dedupe repair remain Enterprise contracts.

</div>

## Implementation Status

| Area | Current public Community behavior | Enterprise/spec status |
| --- | --- | --- |
| Gateway scale-out | Supported with stateless gateways, TiKV metadata, and etcd registry flags. | Same base pattern applies to Enterprise deployments. |
| SBS replicated storage | Community production SBS-backed paths use `sbs-cluster` with a metadata registry volume-pool id and at least two replicated members. | Enterprise deployments may add EC/classroute placement. |
| EC healing and dedupe repair | `dedupe-scrub` and `dedupe-repair` are reserved flat command names and return Enterprise-required responses in Community builds. | Private Enterprise overlay owns EC healing, dedupe repair, and rebalancing implementation. |

## Operation Scope

| Scenario | Production Implementation Requirements | Deployment Scope |
| --- | --- | --- |
| Gateway Scale-Out | Deploy additional stateless gateway instances behind L4/L7 load balancers, dynamically registering with etcd. | <span class="badge">Community</span> |
| Metadata Scaling | Add TiKV/PD nodes and safely rebalance Raft leaders and ranges. | <span class="badge">Community</span> |
| SBS Replicated Volume Scan | Verify volume-pool member integrity for replicated SBS storage and regenerate corrupt or missing replicas through the SBS maintenance workflow. | <span class="badge">Community</span> |
| SBS EC Self-Healing | Identify lost or corrupt Erasure Coding data/parity shards and invoke background healing. | <span class="badge enterprise">Enterprise edition only</span> |

## Bucket, Object, And Volume Pool Capacity Model

A NAMROS bucket is not pinned to a specific SBS volume. Bucket metadata is owned by the NAMROS metadata repository such as TiKV and stores S3 namespace state: bucket name, bucket id, versioning, lifecycle, policy, quota, and Object Lock configuration. The object list inside a bucket is also maintained by metadata object heads and list indexes, not by scanning SBS volumes.

Object payloads reach SBS through `SegmentRef` values in the committed `ObjectVersion` manifest. At the logical layer, NAMROS records bucket/key/version, storage class, volume-pool id, and segment refs. At the physical layer, those refs map to SBS replicated chunk spans or <span class="badge enterprise">Enterprise edition only</span> EC storage class stripe/shard placement. One bucket can therefore span many pool members, and one SBS volume can hold segments for many buckets.

| Question | Answer |
| --- | --- |
| How do I increase a bucket's storage space? | Do not add a volume id to the bucket. Increase the capacity of the volume pool used by the bucket's storage class or gateway policy for new object writes. |
| How is volume-pool capacity expanded? | Prepare physical capacity through the SBS/NAMRBD operational workflow, then register the new replicated or EC-capable member as active in the NAMROS metadata registry. |
| When can new objects use the added space? | After the pool registry update commits, gateways observe the new generation by refresh or restart, and the member is active/healthy for write admission. |
| Do existing objects move automatically? | No. Existing object versions keep the SegmentRef placement recorded when they were committed. Movement is a separate drain, rebalance, or lifecycle-transition operation. |

1. **Prepare SBS physical capacity:** add disks, nodes, volumes, or members and verify the replicated or EC durability posture.
2. **Register the NAMROS volume-pool member:** use the metadata registry path such as `namros-admin volume-pool-put` with volume id, admin/data endpoint, state, weight, and storage class.
3. **Confirm gateway refresh:** verify all gateways observe the new pool generation. Use rolling restart where refresh/watch is not yet available.
4. **Validate writes:** run new-object PUT/GET/LIST, cross-gateway read, and gateway failover smoke to prove the expanded capacity is reachable from the object write path.
5. **Plan old-data movement separately:** use an auditable drain/rebalance plan if older placements must be redistributed; do not rewrite SegmentRefs in place.

## Stateless Gateway Scale-Out

Since NAMROS gateways do not hold persistent authoritative state, they can be scaled out horizontally on demand under high traffic loads.

```sh
# Start a new gateway instance and register it with the etcd coordination cluster
namros-gateway \
  -http-listen 192.168.10.12:9000 \
  -deployment-profile production \
  -coordination-backend etcd \
  -etcd-endpoints 192.168.10.5:12379 \
  -gateway-registry-prefix /namros/gateways \
  -metadata-backend tikv \
  -tikv-pd-endpoints 192.168.10.6:2379 \
  -tikv-keyspace namros \
  -storage-backend sbs-cluster \
  -sbs-volume-pool-id standard-repl \
  -sbs-writer-group-id object-writers \
  -sbs-session-id gw-new-boot-1 \
  -sbs-volume-epoch 1 \
  -gc-candidate-queue metadata
```

The load balancers periodically query the `/debug/admin/status` health-check endpoint to verify successful registration before weighting and directing traffic.

## Safe Node Maintenance Flow

A safe pipeline for taking individual gateways or storage nodes offline for maintenance while maintaining strict data availability.

1. **Check Monitoring Signals:** Check active-active gateway consistency and TiKV disk utilization.
2. **Drain Traffic:** Remove the gateway from the load balancer target groups and wait for a grace period (recommended 180 seconds) to allow ongoing S3 multipart uploads and Range GET requests to close.
3. **Perform Maintenance:** Perform hardware diagnostics, OS security patches, or physical drive inspections.
4. **Verify and Restore:** After rebooting, run S3 client smoke commands to verify basic S3 IO before reintroducing it to active load balancer target groups.

## EC Shard Healing Contract

If one or more drive failures are detected within the Erasure Coding (EC_4_2) group, missing shards are regenerated to prevent data loss. This section is an Enterprise contract; the listed `namros-admin dedupe-*` commands are edition-gated in public Community builds.

### 1. Scan for Corrupt Data/Parity Shards & Deduplication References

```sh
# Scan for integrity corruption across a bucket or specific segments
namros-admin dedupe-scrub -bucket finance-reports
```

### 2. Healing Plan Simulation & Safety Checks

Before performing recovery, simulate the drive states and verify if the remaining read quorum (minimum of 4 healthy shards) is safe.

```sh
# Generate a read-only diagnostic and recovery impact preview report
namros-admin dedupe-repair -bucket finance-reports -dry-run
```

### 3. Execute Healing & Data Rebalancing

```sh
# Upon safety verification, initiate background healing and lost-shard regeneration transactions
namros-admin dedupe-repair -bucket finance-reports -apply
```

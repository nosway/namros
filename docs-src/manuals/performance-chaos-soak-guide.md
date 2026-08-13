Validation

# NAMROS Performance/Chaos/Soak Guide

<div class="summary" markdown="1">

M22 is not a single smoke test, but a milestone to repeatedly collect evidence of performance, stability, failover, recovery, and data consistency in a multi-node environment.

</div>

## Layered Benchmark Model

| Layer | Signal |
| --- | --- |
| Network | Throughput, latency, packet loss, retry spikes. |
| Storage/SBS | Segment write/read latency, shard errors, healing backlog. |
| Metadata/TiKV | Transaction latency, contention, retry count. |
| Gateway | Request latency percentiles, first-byte latency, PUT/UploadPart body-read latency, bounded S3 error code counters, cache hit/miss, auth/signature errors. |
| S3 clients | AWS CLI/mc/rclone/s3fs workload success and digest verification. |

## Soak Workload

1. Create buckets with versioning and mixed object sizes.
2. Run PUT/GET/HEAD/Range/List/Copy/Delete and multipart operations.
3. Run the noisy-tenant throttle profile to prove a throttled tenant does not starve a neighboring tenant.
4. Include optional lifecycle, KMS, dedupe, and EC profiles where entitled.
5. Verify object digests across gateways after every phase.

## Chaos Scenarios

| Scenario | Expected behavior |
| --- | --- |
| Gateway kill/restart | etcd registry removes failed gateway and remaining gateways serve committed state. |
| etcd member loss | Registry degrades according to etcd health; object metadata remains authoritative in TiKV. |
| TiKV transient failure | Gateway fails requests predictably, retries where safe, and recovers without lost committed objects. |
| SBS data restart | Writes fail or retry safely; reads validate digest after recovery. |

## Report Artifact

```text
topology:
workload:
duration:
chaos timeline:
metrics summary:
digest verification:
failures:
retained logs:
reproduction commands:
```

The MCP tools `namros.multi_node.soak.run` and `namros.chaos_soak.latest` should read and summarize this report shape. See the [MCP operations guide](mcp-operations-guide.md).

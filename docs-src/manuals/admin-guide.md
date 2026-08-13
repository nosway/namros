Day-2 Operations

# NAMROS Administrator Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition operations and Enterprise edition only operation families. Treat Enterprise-only operation rows as private distribution contracts unless the page explicitly describes Community denial behavior.

</div>

## Component Ownership

| Component | Owns | Does Not Own |
| --- | --- | --- |
| namros-gateway | S3 routing, auth, request handling, cache, debug endpoints | authoritative object state after write publication |
| Metadata backend | bucket/object/version/MPU/lifecycle/operation records | payload bytes |
| Segment storage | payload segments and delete attempts | namespace visibility |
| etcd | <span class="badge">Community</span> gateway registry and health leases | object metadata or payload |
| TiKV/SBS | <span class="badge">Community</span> distributed metadata and replicated physical storage; <span class="badge enterprise">Enterprise edition only</span> EC physical storage | metadata authority and payload storage |

## Standard Topologies

| Topology | Components | Use |
| --- | --- | --- |
| Local Community | gateway, Pebble, local segment path | development and user-space compatibility |
| Compatibility Lab | gateway plus AWS CLI/mc/rclone/s3fs clients | external client validation |
| Active-active Community | multiple gateways, TiKV, etcd, shared storage | availability and stateless gateway validation |
| SBS EC Enterprise | TiKV, SBS admin/data, EC shard routes, gateway | EC storage class verification |

## Gateway Configuration

```sh
go run ./cmd/namros-gateway \
  -listen 127.0.0.1:9000 \
  -region us-east-1 \
  -metadata-backend pebble \
  -metadata-path .namros/meta \
  -storage-backend local \
  -storage-path .namros/segments
```

For distributed metadata, Community deployments can use TiKV/PD configuration and etcd gateway coordination. SBS EC, dedupe, WORM/Object Lock enforcement, SSE-KMS, and compliance evidence remain Enterprise-gated.

## Health And Debug Endpoints

```sh
curl -fsS http://127.0.0.1:9000/healthz
curl -fsS http://127.0.0.1:9000/debug/admin/status
curl -fsS http://127.0.0.1:9000/debug/operations/metrics
```

| Endpoint | Expected Use | Failure Signal |
| --- | --- | --- |
| `/healthz` | readiness and load balancer health | non-2xx response or timeout |
| `/debug/admin/status` | metadata backend identity, collection counts, capability flags | backend unavailable or inconsistent counts |
| `/debug/operations/metrics` | GC, dedupe, TiKV, scheduler metrics snapshot | retry counters growing or stale scheduler state |

## Metadata Backup/Restore

Metadata export/import is the first safe operational recovery path. It must preserve source identity, audit hash information, collection counts, and target conflict checks.

```sh
make smoke-metadata-backup-restore
```

1. Export metadata from the source backend.
2. Validate schema and collection counts.
3. Run empty-target preflight.
4. Run non-empty target conflict preflight.
5. Apply only after target checks and conflict policy are explicit.

Reference: [TiKV operations guide](tikv-ha-cluster-install-operations-guide.md) and [upgrade and release guide](upgrade-release-operations-guide.md).

## Lifecycle, GC, Dedupe, Compliance Operations

| Operation | Community | Enterprise | Operator Rule |
| --- | --- | --- | --- |
| Lifecycle planning | non-mutating planning where available | policy-aware planning and workers | inspect blocked actions before apply |
| Orphan GC retry | local protected-ref aware cleanup | SBS/EC aware cleanup | fail closed when protected refs cannot be checked |
| Dedupe | Enterprise-required admin paths | candidate, verify, ack, scrub, repair | never attach shared refs without byte verification |
| Compliance evidence | Enterprise-required admin paths | evidence packages and policy simulation | record limitations and no-certification boundary |

## Release Readiness

```sh
make release-readiness
make check-community-export
make export-community
make html-docs-check
```

`release-readiness` creates JSON/Markdown artifacts under `release-reports/`. Community publication additionally requires no public Enterprise unlock path, valid source export, and passing user-space compatibility.

## Day-2 Operation Extensions

| Area | Guide | Purpose |
| --- | --- | --- |
| Web console and monitoring | [Console guide](web-console-monitoring-guide.md) | Dashboard, report viewer, alert summary, approved operation workflow. |
| S3 object browser integration | [Object browser guide](s3-object-browser-integration-guide.md) | Object Explorer Lite scope and external S3 browser recipes. |
| Replication and DR | [Replication guide](replication-disaster-recovery-guide.md) | Site/bucket/batch replication, lag, failover/failback planning. |
| Events | [Event guide](event-notification-guide.md) | Webhook/Kafka/NATS notifications, retry, DLQ, replay. |
| Inventory and batch | [Inventory guide](inventory-batch-operations-guide.md) | Large namespace reports and batch job envelopes. |
| Capacity and maintenance | [Capacity guide](capacity-scaling-maintenance-guide.md) | Node maintenance, decommission, healing, rebalance, shard inspection. |
| Quota and QoS | [Quota guide](quota-qos-guide.md) | Bucket/tenant quota, rate limits, usage metrics, threshold alerts. |

## Incident Checklist

1. Identify endpoint, bucket/key, request id, and client command.
2. Capture gateway health and admin status.
3. Capture operations metrics.
4. Preserve compatibility tmpdir or release report if applicable.
5. Classify subsystem: client config, gateway, metadata, storage, Enterprise dependency.
6. Run only read-only checks before operator-approved repair actions.
7. Record follow-up in an incident bundle or release report.

## Enterprise Operation Boundaries

<span class="badge enterprise">Enterprise edition only</span> SBS EC/dedupe/KMS/compliance operations are product features in the private distribution. Community commands and S3 requests that touch these surfaces should return a NAMROS Enterprise Edition requirement error rather than silently degrading or enabling partial behavior. TiKV metadata and etcd gateway coordination are Community features.

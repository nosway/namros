Community Production Operations

# TiKV HA Installation & Operations Guide

## Role

<span class="badge">Community</span> TiKV is the distributed authoritative metadata backend for active-active NAMROS deployments. Gateway processes must not treat local cache as authoritative object state.

## PD/TiKV Topology

| Component | Role | Operational Note |
| --- | --- | --- |
| PD | cluster metadata, scheduling, timestamp oracle | run odd-numbered quorum for HA |
| TiKV | distributed key-value data storage | size for metadata write/read workload |
| NAMROS gateway | transactional metadata client | uses keyspace/prefix per deployment |

TiUP-based bootstrap and host hardening should follow the TiKV operational standard used by the environment. NAMROS-specific concerns are endpoint selection, keyspace naming, and smoke validation.

## Gateway Config

```sh
namros-gateway \
  -metadata-backend tikv \
  -tikv-pd-endpoints 127.0.0.1:2379 \
  -tikv-keyspace namros \
  -storage-backend local \
  -storage-path .namros/segments
```

Use distinct keyspaces for dev/stage/prod/lab. Do not reuse a test keyspace for release evidence unless the runbook explicitly says so.

## Metadata Backup/Restore Interaction

NAMROS metadata export/import operates at the product metadata level. TiKV snapshots protect the underlying cluster, while `namros-admin metadata-export` preserves product collections and audit hashes for controlled restore workflows.

| Backup Type | Purpose | Recovery Scope |
| --- | --- | --- |
| TiKV snapshot/backup | cluster-level disaster recovery | raw KV state |
| NAMROS metadata export | product-level migration/preflight/restore | NAMROS metadata collections |

## Smoke Procedures

```sh
make smoke-active-active
make compat-sbs-cluster-ec
```

`smoke-active-active` validates cross-gateway read/write and failover behavior. `compat-sbs-cluster-ec` additionally requires SBS admin/data endpoints and a materialized SBS volume with compatible shard routes.

## Troubleshooting

| Signal | Likely Cause | Action |
| --- | --- | --- |
| PD connection refused | wrong endpoint or PD not running | verify `-tikv-pd-endpoints` and cluster health |
| metadata record not found | wrong keyspace or missing volume/control-plane state | verify keyspace and prepare required SBS state |
| transaction retry pressure | hot key or large transaction | inspect metadata operation size and retry metrics |
| active-active mismatch | cache invalidation or shared storage issue | compare gateway admin status and read-through cache behavior |

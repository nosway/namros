Reference Manual <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only</span>

# NAMROS Integrated CLI Command Reference

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition CLI commands and Enterprise edition only reserved command names. Reserved Enterprise command names return edition-boundary errors in public Community builds unless a private Enterprise distribution supplies the implementation.

</div>

<div class="summary" markdown="1">

This reference documents the current command shape of `namros-gateway` and `namros-admin`. The admin CLI uses flat command names such as `bucket-quota-put` and `metadata-export`; it does not use nested command groups such as `replication status` or `kms status`.

</div>

## Implementation Status

| Surface | Current behavior |
| --- | --- |
| Community gateway | Runs S3-compatible API, local/Pebble metadata, TiKV metadata, etcd gateway registry, local storage, and SBS-backed Community storage paths. |
| Community admin CLI | Provides metadata status, scale budgeting, volume-pool metadata, bucket max-object-size quota, worker/GC inspection, metadata export/import validation, and IAM simulation helpers. |
| Enterprise-gated admin commands | Commands for dedupe, KMS key management, and compliance evidence are present as flat names but return the NAMROS Enterprise-required response in Community builds. |

## 1. namros-gateway

The gateway process accepts S3-compatible HTTP requests. Production Community deployments use `-deployment-profile production` with TiKV metadata, etcd coordination, an SBS replicated volume pool, and SBS writer session fencing. The local/Pebble path remains a development profile.

```sh
namros-gateway \
  -listen 0.0.0.0:9000 \
  -deployment-profile production \
  -metadata-backend tikv \
  -tikv-pd-endpoints 192.168.10.6:2379 \
  -tikv-keyspace namros \
  -storage-backend sbs-cluster \
  -sbs-volume-pool-id standard-repl \
  -sbs-writer-group-id object-writers \
  -sbs-session-id gw-a-boot-1 \
  -sbs-volume-epoch 1 \
  -coordination-backend etcd \
  -etcd-endpoints 192.168.10.5:12379 \
  -gateway-registry-prefix /namros/gateways
```

### Core Gateway Flags

| Flag | Typical value | Description |
| --- | --- | --- |
| `-listen` | `127.0.0.1:9000` | HTTP listen address for S3 API requests. |
| `-deployment-profile` | `dev`, `production` | Validation profile. Production rejects development-only metadata, storage, direct single-volume, and unfenced shared-attachment shortcuts. |
| `-region` | `us-east-1` | Region expected by SigV4 clients and compatibility scripts. |
| `-metadata-backend` | `pebble`, `tikv`, `memory` | Authoritative metadata backend. |
| `-metadata-path` | `.namros/meta` | Pebble metadata path for local Community runs. |
| `-tikv-pd-endpoints` | `host:2379` | Comma-separated PD endpoints when `-metadata-backend tikv` is used. |
| `-tikv-keyspace` | `namros` | TiKV keyspace name or v1 key prefix fallback. |
| `-storage-backend` | `local`, `sbs-physical`, `sbs-cluster` | Payload segment backend. Production deployments use `sbs-cluster` with an SBS volume-pool id. |
| `-sbs-admin-endpoint` | `sbs-admin:9443` | SBS admin gRPC endpoint for SBS-backed storage. |
| `-sbs-data-endpoint` | `sbs-data:9460` | SBS data gRPC endpoint for chunk or shard IO. |
| `-sbs-volume-id` | `18a00001` | SBS volume id for `sbs-physical` or `sbs-ec` storage. |
| `-sbs-volume-pool-id` | `standard-repl` | Metadata registry volume pool id used by production SBS-backed storage. |
| `-sbs-writer-group-id` | `object-writers` | SBS shared logical writer group required by production session fencing. |
| `-sbs-session-id` | `gw-a-boot-1` | Per-gateway session id. Use a distinct value for each gateway process start. |
| `-sbs-volume-epoch` | `1` | Volume epoch used to fence stale handles and idempotency replay. |
| `-coordination-backend` | `none`, `etcd` | Gateway coordination backend. |
| `-etcd-endpoints` | `host:2379` | Comma-separated etcd endpoints for gateway coordination. |
| `-gateway-registry-prefix` | `/namros/gateways` | etcd key prefix for active gateway leases. |
| `-gateway-data-budget-bytes` | `1073741824` | Optional aggregate in-flight data-path byte budget. Excess PUT, UploadPart, CopyObject, UploadPartCopy, and GET requests return S3 `SlowDown`. |
| `-gateway-data-budget-max-requests` | `256` | Optional concurrent data-path request budget; `0` disables this limit. |
| `-gateway-data-budget-unknown-bytes` | `8388608` | Reservation size for chunked or otherwise unknown request payload sizes. |

## 2. Common Metadata Flags For namros-admin

Most metadata-backed admin commands accept the same backend selection flags. Use these flags whenever the command needs to inspect or mutate NAMROS metadata.

| Flag | When to use |
| --- | --- |
| `-metadata-backend pebble -metadata-path .namros/meta` | Local Community metadata stored on disk. |
| `-metadata-backend tikv -tikv-pd-endpoints host:2379 -tikv-keyspace namros` | Distributed metadata in TiKV. |
| `-tikv-api-version`, `-tikv-timeout`, `-tikv-tls-ca`, `-tikv-tls-cert`, `-tikv-tls-key` | TiKV API, timeout, and TLS controls. |

## 3. Community Admin Commands

| Command | Purpose | Example |
| --- | --- | --- |
| `status` | Summarize metadata health, recent operation counters, and production readiness posture. | `namros-admin status -metadata-backend tikv -tikv-pd-endpoints host:2379 -deployment-profile production -storage-backend sbs-cluster -sbs-volume-pool-id standard-repl -sbs-writer-group-id object-writers -sbs-session-id gw-a-boot-1 -sbs-volume-epoch 1 -coordination-backend etcd -etcd-endpoints host:2379 -gc-candidate-queue metadata` |
| `metadata-scale-budget` | Estimate metadata value and transaction size for multipart objects, protected refs, and GC candidates. | `namros-admin metadata-scale-budget -part-count 10000` |
| `volume-pool-put` | Write SBS volume-pool metadata used by SBS-backed gateways. | `namros-admin volume-pool-put -pool-id replicated-rf3 -member volume_id=18a00001,data_endpoint=sbs-data-a:9460,state=active` |
| `bucket-quota-put` | Set Community bucket max-object-size quota. | `namros-admin bucket-quota-put -bucket photos -max-object-size-bytes 1073741824` |
| `bucket-quota-get` | Read bucket max-object-size quota. | `namros-admin bucket-quota-get -bucket photos` |
| `bucket-quota-delete` | Remove bucket max-object-size quota. | `namros-admin bucket-quota-delete -bucket photos` |
| `tenant-quota-put` | Set tenant quota metadata records. | `namros-admin tenant-quota-put -tenant-id finance -max-bytes 1099511627776 -max-objects 1000000 -max-active-uploads 256` |
| `tenant-quota-get` | Read tenant quota metadata records. | `namros-admin tenant-quota-get -tenant-id finance` |
| `tenant-quota-delete` | Remove tenant quota metadata records. | `namros-admin tenant-quota-delete -tenant-id finance` |
| `worker-operations` | List worker operation records, optionally filtered by kind, shard, or status. | `namros-admin worker-operations -worker-kind gc -limit 20` |
| `gc-candidates` | List orphan GC candidate records from metadata. | `namros-admin gc-candidates -limit 20` |
| `gc-candidate-seed-object` | Detach an object version and enqueue its segment references as GC candidates. | `namros-admin gc-candidate-seed-object -bucket photos -key stale.bin` |
| `metadata-export` | Export product metadata as JSON for backup and audit workflows. | `namros-admin metadata-export -limit 1000` |
| `metadata-import` | Validate an export JSON, or plan/apply import with explicit target flags. | `namros-admin metadata-import -input export.json -dry-run` |
| `iam-principal-inspect` | Render the normalized IAM principal built from CLI flags. | `namros-admin iam-principal-inspect -tenant-id root -access-key-id namros -root` |
| `iam-policy-simulate` | Evaluate an inline or file-based S3/IAM policy for a principal/action/resource tuple. | `namros-admin iam-policy-simulate -action s3:GetObject -resource arn:aws:s3:::photos/a.jpg -policy-file policy.json` |
| `iam-mapping-validate` | Validate an external IAM mapping specification JSON file. | `namros-admin iam-mapping-validate -input mapping.json` |

## 4. Enterprise-gated Commands

Community builds intentionally expose no unlock switch for Enterprise-only behavior. These flat command names are reserved at the CLI boundary and return the Enterprise-required response unless a private Enterprise build supplies the implementation.

| Capability | Reserved command names |
| --- | --- |
| Dedupe and recovery | `dedupe-plan`, `dedupe-ack`, `dedupe-ops`, `dedupe-repair`, `dedupe-scrub` |
| SSE-KMS | `kms-key-put`, `kms-key-list` |
| Compliance evidence | `compliance-evidence`, `compliance-profile-plan`, `compliance-policy-simulate` |

## 5. Copy-safe Examples

### Local status check

```sh
namros-admin status \
  -metadata-backend pebble \
  -metadata-path .namros/meta
```

### TiKV metadata export

```sh
namros-admin metadata-export \
  -metadata-backend tikv \
  -tikv-pd-endpoints 192.168.10.6:2379 \
  -tikv-keyspace namros \
  -limit 1000
```

### Community Enterprise-gate verification

```sh
namros-admin kms-key-list
```

Expected Community result: the command fails with the standard NAMROS Enterprise-required response, confirming that the public build has not exposed a KMS unlock path.

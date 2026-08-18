Compatibility

# S3 Client Compatibility Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition client smoke coverage and Enterprise edition only client command families. Rows marked <span class="badge enterprise">Enterprise edition only</span> describe private distribution contracts, not public Community availability.

</div>

## Scope And Non-goals

Run these client checks against a local gateway started with `make run-dev` or a container stack started from the container deployment guide. s3fs-fuse is validated separately on a Linux host because it requires FUSE mount privileges.

| Client | Coverage | Example |
| --- | --- | --- |
| AWS CLI | put/get/head/range/list, metadata, copy, versioning, CORS, presign, MPU list/abort | `aws s3api list-buckets` |
| MinIO client | copy/cat/stat/list, server-side move, multipart-sized copy, versioned writes | `mc ls namros` |
| rclone | copy/list/read, move/delete, multipart-sized copy, versioned writes | `rclone lsd namros:` |
| s3fs-fuse | mount, file read/write/list/rename, xattr-sensitive flows | Linux FUSE host procedure |

## Common Environment

```sh
export NAMROS_ENDPOINT=http://127.0.0.1:9000
export NAMROS_ACCESS_KEY_ID=namrosroot
export NAMROS_SECRET_ACCESS_KEY=namrosrootsecret
export NAMROS_REGION=us-east-1
```

Use path-style addressing unless a specific virtual-hosted-style test is being performed. For virtual-hosted-style, DNS or `/etc/hosts` must map bucket hostnames to the gateway.

## Public Reproducer

```sh
make compat-public-s3
```

This target requires AWS CLI, `jq`, `curl`, MinIO client, and rclone. It starts a local in-memory gateway when `NAMROS_ENDPOINT` is not already ready, then runs bucket, object, multipart-sized object, and versioned object write coverage for all three clients. `make compat-user-space` remains useful on developer machines because it runs the client smokes whose tools are installed and skips the rest.

## AWS CLI Smoke

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api list-buckets
```

Expected result: the command returns a normal S3 list-buckets response for the configured endpoint.

## MinIO Client Smoke

```sh
mc alias set namros "$NAMROS_ENDPOINT" "$AWS_ACCESS_KEY_ID" "$AWS_SECRET_ACCESS_KEY"
mc ls namros
```

Common MinIO client failures are bucket alias mismatch, path-style configuration mismatch, and server-side move behavior diverging from S3 copy/delete semantics.

## rclone Smoke

```sh
rclone lsd namros:
```

rclone verifies that object size and content are correct after upload. A "corrupted on transfer: sizes differ" failure usually means the PUT path, HEAD path, or response length handling is inconsistent.

## s3fs-fuse On Linux

s3fs-fuse requires a Linux host with FUSE permissions. Required packages include s3fs-fuse and xattr tooling such as the package that provides `listfattr`.

## MinIO Client Extended Compatibility Matrix

| mc command family | NAMROS status | Related guide |
| --- | --- | --- |
| `mc retention`, `mc legalhold` | <span class="badge enterprise">Enterprise edition only</span> Object Lock/WORM behavior. | [Object Lock chapter](architecture-manual/chapters/09-versioning-lifecycle-object-lock.md) |
| `mc encrypt` | <span class="badge enterprise">Enterprise edition only</span> SSE-KMS payload encryption after M21. | [KMS guide](kms-encryption-guide.md) |
| `mc ilm` | Lifecycle config/planner/worker scope. | [Lifecycle chapter](architecture-manual/chapters/09-versioning-lifecycle-object-lock.md) |
| `mc event` | <span class="badge enterprise">Enterprise edition only</span> Real-time webhook, Kafka, and NATS event notification pipelines. | [Event guide](event-notification-guide.md) |
| `mc replicate` | <span class="badge enterprise">Enterprise edition only</span> Real-time asynchronous cross-region bucket and site replication. | [Replication guide](replication-disaster-recovery-guide.md) |
| `mc quota` | <span class="badge enterprise">Enterprise edition only</span> Multi-tenant and bucket-level storage quota and QoS bandwidth limits. | [Quota guide](quota-qos-guide.md) |
| `mc inventory` | <span class="badge enterprise">Enterprise edition only</span> Large-scale bucket metadata inventory and bulk batch mutation frameworks. | [Inventory guide](inventory-batch-operations-guide.md) |

## Known Failure Signatures

| Signal | Likely Cause | Next Step |
| --- | --- | --- |
| AWS CLI metadata assertion fails | HEAD response missing user metadata | Inspect `head-object` JSON and gateway metadata mapping. |
| MinIO object hash mismatch | copy/read path changed bytes | Compare source file, GET output, and object ETag. |
| rclone size mismatch | PUT response or HEAD size is incorrect | Check Content-Length and stored segment size. |
| CORS preflight unexpected status | OPTIONS routing or CORS config behavior mismatch | Verify bucket CORS state and OPTIONS handler. |
| s3fs cannot mount | FUSE permission, missing package, or endpoint style mismatch | Check the Linux package set and mount command options. |

## Result Record Template

```text
date:
namros commit:
gateway command:
endpoint:
client versions:
smoke target:
result:
bucket:
tmpdir/log path:
notes:
```

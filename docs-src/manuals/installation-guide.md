Install

# NAMROS Installation Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition installation paths and Enterprise edition only dependency notes. Enterprise-only dependencies do not imply that public Community builds can enable those features.

</div>

## Scope

This guide describes the procedures for running a local NAMROS gateway and performing user-space S3 compatibility verification in the Community edition. Production Community deployments can utilize etcd, TiKV, and SBS replicated storage backends, while Enterprise deployments require SBS EC, Vault KMS, and advanced integration pre-requisites detailed in separate runbooks.

<div class="note" markdown="1">

All commands are assumed to be run from the repository root. Example: `~/src/namros`.

</div>

## Supported Platforms

| Platform | Supported Use | Notes |
| --- | --- | --- |
| macOS | build, local gateway, AWS CLI/mc/rclone smoke | s3fs-fuse mount validation is not the primary path. |
| Linux user space | build, local gateway, AWS CLI/mc/rclone smoke | Use this for routine compatibility checks. |
| Linux FUSE host | s3fs-fuse compatibility | Requires FUSE packages and root privileges for mount operations. |
| Community HA lab | active-active, TiKV, etcd, SBS replicated | Requires prepared distributed dependencies. |
| Enterprise lab | SBS EC, dedupe, WORM/KMS/compliance | Requires prepared Enterprise dependencies. |

## Required Tools

| Tool | Use | Validation Command |
| --- | --- | --- |
| Go | build and test `namros-gateway`, `namros-admin` | `go version` |
| AWS CLI | S3 API compatibility smoke | `aws --version` |
| MinIO client | copy/stat/list compatibility | `mc --version` |
| rclone | copy/move/delete compatibility | `rclone version` |
| jq | smoke output assertions | `jq --version` |
| s3fs-fuse | Linux FUSE compatibility | `s3fs --version` |

Client setup and smoke workflow details are maintained in the [S3 client compatibility guide](s3-client-compatibility-guide.md).

## Production Readiness Checklist

Use `-deployment-profile production` only with a production topology: TiKV metadata, etcd gateway coordination, at least two gateway instances, an SBS replicated volume pool, and SBS writer session fencing. Memory metadata, Pebble metadata, local segment storage, and direct single-volume or shared-attachment SBS shortcuts are development or lab modes.

| Area | Check | Reference |
| --- | --- | --- |
| Security | TLS termination, admin access boundary, secret redaction, no public Enterprise unlock path. | [editions](architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| Metadata | TiKV/PD endpoints, keyspace, backup/restore smoke, transaction failure behavior. | [TiKV guide](tikv-ha-cluster-install-operations-guide.md) |
| Coordination | etcd client/peer URLs, lease TTL, registry root, member replacement runbook. | [etcd guide](etcd-ha-cluster-install-operations-guide.md) |
| Storage | Local/SBS replicated readiness and Enterprise EC route prerequisites. | [capacity guide](capacity-scaling-maintenance-guide.md) |
| Identity/KMS | IAM provider and KMS provider readiness when Enterprise features are used. | [IAM](iam-integration-guide.md), [KMS](kms-encryption-guide.md) |
| Release | Compatibility, active-active, backup/restore, source export, production-scale gate, and release-readiness reports. | [upgrade/release](upgrade-release-operations-guide.md) |

### 1. OS Kernel Parameter Tuning (Preflight Setup)

Tuning the operating system's kernel parameters is important for highly concurrent stateless gateway operations in production environments. Validate changes against the target host and workload before applying them at boot.

**Increase System Resource Limits (/etc/security/limits.conf):**

```text
namros       soft    nofile          65536
namros       hard    nofile          131072
namros       soft    nproc           32768
namros       hard    nproc           65536
```

**Kernel Virtual Memory & Socket Tuning (/etc/sysctl.conf):**

```text
net.ipv4.tcp_tw_reuse = 1
net.ipv4.ip_local_port_range = 10240 65000
net.core.somaxconn = 8192
net.core.netdev_max_backlog = 10000
net.ipv4.tcp_max_syn_backlog = 4096
vm.overcommit_memory = 1
```

### 2. Storage IOPS & DB Dedicated Infrastructure

- **Separate TiKV Disks:** isolate TiKV disks from gateway local segments and unrelated I/O. Measure the required IOPS and latency against the intended workload.
- **Dedicated etcd Storage:** use low-latency durable storage and monitor fsync latency.

### 3. TLS And Compliance Posture

Production gateways should terminate TLS at a trusted front-end proxy or use a validated gateway TLS configuration. FIPS or regulatory claims require separate validation of the complete deployed cryptographic boundary.

## Build

```sh
go build ./cmd/namros-gateway
go build ./cmd/namros-admin
go test ./...
```

Build artifacts are produced in the current working directory when `go build ./cmd/...` is used without `-o`. For repeatable local use, prefer explicit output paths such as `bin/namros-gateway` if your local workflow needs binaries.

## Local Community Quick Start

Fresh clone path for a GitHub developer:

```sh
git clone https://github.com/nosway/namros.git
cd namros
make test-community
make build-community
make run-dev
```

The default development target starts `namros-gateway` on `127.0.0.1:9000` using region `us-east-1`, Pebble metadata at `.namros/meta`, and local segment storage at `.namros/segments`.

This target is intentionally `dev` profile. It is useful for S3 client checks and local development, but it is not a large-capacity or production deployment shape.

Clean shutdown is `Ctrl-C` in the terminal running the gateway. Restart uses the same Pebble and segment paths unless they are removed by the operator.

With the gateway running, verify an S3 round trip from another shell:

```sh
export NAMROS_ENDPOINT=http://127.0.0.1:9000
export AWS_ACCESS_KEY_ID=namros
export AWS_SECRET_ACCESS_KEY=namros-secret
export AWS_DEFAULT_REGION=us-east-1

aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-bucket --bucket quickstart
printf 'hello namros\n' > /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object --bucket quickstart --key hello.txt --body /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object --bucket quickstart --key hello.txt /tmp/namros-readback.txt
```

## Community Container Deployment

The approved Docker and Compose profiles, image policy, secret handling, readiness contract, and implementation status are defined in the [Community container deployment guide](container-deployment-guide.md). Helm is a later, separately documented delivery.

For a public gateway plus SBS backend quickstart:

```sh
make container-sbs-quickstart-smoke
```

This starts one gateway, one SBS service, two SBS data nodes, and PD/TiKV test
metadata through `packaging/docker/compose.sbs-quickstart.yml`. The S3 endpoint
is `http://127.0.0.1:9002`.

For the production-shaped Kubernetes/kind scenario:

```sh
make k8s-production-render
make kind-production-deploy
# Reuse the existing cluster and loaded images
make kind-production-stop
make kind-production-start
# When finished, delete the kind cluster and its ephemeral test state
make kind-production-down
```

The default config file is `packaging/k8s/production-kind.env` and renders 2
gateways, 2 SBS services, 5 SBS data nodes, and one embedded TiKV instance.

## Community Gateway Flags

| Flag | Typical Value | Meaning |
| --- | --- | --- |
| `-http-listen` | `127.0.0.1:9000` | HTTP listen address. |
| `-deployment-profile` | `dev`, `production` | Validation profile. Production rejects memory/Pebble/local/single-volume and unfenced shared-attachment shortcuts unless the explicit lab override is set. |
| `-region` | `us-east-1` | Region used by S3 compatibility clients. |
| `-metadata-backend` | `pebble` | Authoritative metadata backend for local Community runs. |
| `-metadata-path` | `.namros/meta` | Pebble metadata path. |
| `-storage-backend` | `local` | Payload segment backend. |
| `-storage-path` | `.namros/segments` | Local segment directory. |

## Verification Sequence

1. Run Community tests and edition-boundary checks: `make test-community`.
2. Start local gateway: `make run-dev`.
3. From another shell, run the container smoke: `make container-local-smoke`.
4. To verify the public gateway with an SBS backend, run `make container-sbs-quickstart-smoke`.
5. Render the production-shaped Kubernetes config: `make k8s-production-render`.
6. For kind evaluation, run `make kind-production-deploy`.
7. For strict public AWS CLI/mc/rclone coverage, run `make compat-public-s3`.
8. For opportunistic non-container user-space client coverage on a workstation, run `make compat-user-space`.
9. Before claiming production-scale readiness, run `make production-scale-check` and review any skipped external smoke gates.
10. For FUSE coverage, use a Linux host with FUSE mount permissions.

```sh
make test-community
make container-local-smoke
make container-sbs-quickstart-smoke
make k8s-production-render
```

Expected result: each smoke prints a passed line. Failure output should include the client name, bucket name, endpoint, and temporary directory when preservation is enabled.

## Distributed And Enterprise Dependencies

<span class="badge">Community</span> Active-active gateways use TiKV metadata, etcd coordination, and SBS replicated storage. <span class="badge enterprise">Enterprise edition only</span> adds SBS EC, KMS, WORM, dedupe, and compliance services; Community builds must not expose an unlock switch for those Enterprise capabilities.

```sh
namros-gateway \
  -deployment-profile production \
  -metadata-backend tikv \
  -tikv-pd-endpoints pd-a:2379,pd-b:2379,pd-c:2379 \
  -storage-backend sbs-cluster \
  -sbs-volume-pool-id standard-repl \
  -sbs-writer-group-id object-writers \
  -sbs-session-id gw-a-boot-1 \
  -sbs-volume-epoch 1 \
  -coordination-backend etcd \
  -etcd-endpoints etcd-a:2379,etcd-b:2379,etcd-c:2379 \
  -gc-candidate-queue metadata
```

| Dependency | Role | Guide |
| --- | --- | --- |
| etcd | gateway registry and health lease | [etcd HA guide](etcd-ha-cluster-install-operations-guide.md) |
| TiKV/PD | distributed authoritative metadata | [TiKV HA guide](tikv-ha-cluster-install-operations-guide.md) |
| SBS service/data | Community replicated physical storage; Enterprise EC storage | [container deployment](container-deployment-guide.md) |
| KMS/compliance services | key posture and evidence workflows | [MCP operations guide](mcp-operations-guide.md) |

## Post-install Troubleshooting

| Signal | Likely Cause | Action |
| --- | --- | --- |
| Port 9000 already in use | existing gateway or other service | Stop the process or run gateway with another `-http-listen` address. |
| AWS CLI metadata assertion fails | stale gateway binary or metadata behavior mismatch | Rebuild, restart gateway, and rerun the client smoke. |
| rclone size mismatch | payload write/read path issue | Preserve tmpdir and inspect gateway log and object HEAD. |
| s3fs mount failure | FUSE permissions or missing Linux package | Run on a Linux host and install the required FUSE/xattr packages. |

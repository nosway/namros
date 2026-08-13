Community Packaging Contract

# NAMROS Community Container Deployment Guide

<div class="note" markdown="1">

**Edition scope.** This page defines the Community edition container deployment contract and lists Enterprise edition only capabilities only as explicit exclusions. SBS EC, dedupe, WORM/Object Lock, KMS lifecycle, and compliance evidence are not enabled by Community container images.

</div>

## Status And Scope

<div class="note" markdown="1">

**Status:** approved deployment contract with provisional implementation artifacts. Dockerfiles, Compose profiles, readiness behavior, secret-file inputs, and `container-*` targets exist in the source tree, but release availability still requires Docker runtime acceptance and published image evidence.

</div>

This guide defines the Community container experience. Docker and Docker Compose are the first delivery scope. Helm is intentionally deferred to a separate Kubernetes guide after the Compose contract is implemented and validated.

TiKV metadata, etcd gateway coordination, active-active gateways, and SBS replicated object storage are Community capabilities. <span class="badge enterprise">Enterprise edition only</span> SBS EC, dedupe, WORM/Object Lock enforcement, KMS lifecycle, and compliance evidence are private-distribution capabilities.

## Deployment Profiles

| Profile | Components | Purpose | Data |
| --- | --- | --- | --- |
| `local` | one gateway, Pebble, local segment store | fast S3 evaluation and development | named volumes |
| `community` | HAProxy, two gateways, etcd, PD and TiKV test containers, SBS load balancers, two SBS services, four SBS data nodes | active-active and replicated Community validation | named volumes |
| `observability` | Community profile plus optional Prometheus and Grafana | metrics and dashboard evaluation | profile-specific volumes |

A single `packaging/docker/compose.yaml` uses explicit Compose profiles. The local profile is the recommended first run. The Community profile is a test topology, not a production-ready etcd or TiKV deployment: its etcd, PD, and TiKV services each have a single failure domain. Production-scale release claims also require `make production-scale-check` and review of any skipped external smoke gates.

## Images, Tags, And Platforms

| Image | Contents | Initial responsibility |
| --- | --- | --- |
| `ghcr.io/nosway/namros-gateway` | `namros-gateway` | S3 service |
| `ghcr.io/nosway/namros-tools` | `namros-admin`, `namros-s3bench`, operations helpers | smoke and administration jobs |
| PD/TiKV test metadata services | pinned PD and TiKV containers | test-only metadata service; wrapper hardening remains release work |

Release images support Linux `amd64` and `arm64`. macOS and Windows users run those Linux images through Docker Desktop. Published versions use semantic tags such as `vX.Y.Z`, minor aliases such as `vX.Y`, commit tags such as `sha-<commit>`, and immutable digests. Official examples pin a release version and the validation bill of materials records digests; they do not depend on `latest`.

Runtime images use a non-root user, drop Linux capabilities, and use a read-only root filesystem. Only documented state directories and temporary paths are writable through named volumes or `tmpfs`. OCI labels record source, revision, version, license, and Community edition identity.

The current implementation uses separate PD and TiKV containers for the Community test topology. A combined wrapper or equivalent release-hardened metadata topology must still define PID 1 behavior, signal propagation, start order, readiness, log routing, data directories, and restart recovery before this path is marked release-available. Compatibility is recorded in the release bill of materials together with NAMROS, NAMRBD/SBS, etcd, and the Compose contract version.

## Local Quick Start Contract

The first-run path uses one gateway, Pebble metadata, and local segment storage. The container listens on `0.0.0.0:9000`; Compose publishes it only on `127.0.0.1:9000`. Persistent paths are `/var/lib/namros/meta` and `/var/lib/namros/segments`.

```sh
# Provisional source-tree commands; release use requires acceptance evidence.
sh scripts/container/ensure-local-files.sh
make container-local-up
make container-local-smoke
make container-local-down
```

The example environment file contains development-only credential placeholders and is excluded from Git. Operators must replace them before start. Images contain no fixed default credentials.

| Target | Effect |
| --- | --- |
| `container-local-up` | build or start the local profile |
| `container-local-smoke` | run the toolbox S3 smoke |
| `container-local-down` | stop containers and preserve named volumes |
| `container-local-reset` | stop containers and permanently remove local profile volumes |

<div class="note" markdown="1">

`container-local-reset` is destructive. Normal shutdown preserves metadata and object data.

</div>

## Distributed Community Contract

| Layer | Count | Routing and identity |
| --- | --- | --- |
| S3 front end | one HAProxy | publishes `127.0.0.1:9001` and removes unready gateways |
| Gateway | two | explicit IDs `namros-gateway-a`/`namros-gateway-b` and explicit advertised endpoints |
| Coordination | one etcd | test-only registry and leases; not HA |
| Metadata | one PD service plus one TiKV service | test-only authoritative metadata; not HA; wrapper hardening pending |
| SBS control plane | two services plus internal HAProxy | one stable admin endpoint for the current gateway interface |
| SBS data plane | four nodes plus internal HAProxy | one stable data endpoint for the current gateway interface |
| SBS bootstrap | two one-shot jobs | create configured replicated volumes, then register the metadata volume pool before gateways start |

Internal SBS load balancers are the initial compatibility solution because the gateway currently accepts one admin and one data endpoint. Client-side endpoint lists or native SBS discovery may replace them only through a documented interface change and equivalent failover tests.

```sh
# Community Compose entrypoint: packaging/docker/compose.community.yml
make container-community-up
make container-community-smoke
make container-community-failover-smoke
make container-community-down
```

The Community Make targets use `packaging/docker/compose.community.yml` by default. `sbs-bootstrap` materializes every comma-separated `NAMROS_COMMUNITY_SBS_VOLUME_IDS` volume, then `namros-pool-bootstrap` registers `NAMROS_COMMUNITY_SBS_VOLUME_POOL_ID` in TiKV. Gateways start only after the pool bootstrap succeeds and run with the production deployment profile, metadata GC queue, writer group, session id, and volume epoch settings.

The Kubernetes reference chart lives at `packaging/helm/namros-community`. Use `make helm-chart-check` to validate the chart contract. `values.local.yaml` keeps embedded etcd, PD, and TiKV enabled for local evaluation, while `values.production.yaml` requires external hardened etcd/TiKV endpoints and an existing root credential Secret.

This profile is intended to prove Community active-active and replicated behavior, but the single etcd, single PD, and single TiKV services prevent a production-HA claim. A production deployment uses `-deployment-profile production`, external hardened etcd and TiKV clusters, an SBS replicated volume pool selected by metadata registry, and SBS writer session fencing.

## Configuration And Secrets

Configuration precedence is:

```text
CLI flag > *_FILE input > ordinary environment variable > built-in default
```

A CLI flag therefore has the highest priority. If the same secret is supplied through more than one input class, startup should fail with a conflict error instead of silently selecting one. Compose uses secret files and the gateway supports at least:

```text
NAMROS_ROOT_ACCESS_KEY_ID_FILE
NAMROS_ROOT_SECRET_ACCESS_KEY_FILE
NAMROS_CONSOLE_ADMIN_PASSWORD_FILE
NAMROS_CONSOLE_SESSION_SECRET_FILE
```

Secret values must not appear in command lines, `docker inspect` environment output, application logs, debug configuration, health responses, or support bundles. The public deployment reference includes a complete environment-to-flag mapping and marks restart-required settings.

## Health, Readiness, And Administrative Exposure

| Surface | Approved contract |
| --- | --- |
| `/healthz` | process liveness only; no dependency calls |
| `/readyz` | low-cost real metadata and storage status/read operations; HTTP 503 when either is unavailable |
| etcd readiness | required and HTTP 503 for the multi-gateway Community profile; skipped when coordination is disabled |
| startup probe | separate Kubernetes contract to tolerate slow dependency initialization |
| `/debug/*` | disabled by default; available only on a separately configured admin listener |
| `/metrics` | reachable only from the internal container network by default; authentication may be enabled |

Readiness responses expose component state and stable reason codes without endpoints, credentials, keys, or other secrets. The source-tree implementation performs dependency readiness checks, but release acceptance still requires container runtime evidence.

## Smoke And Failover Validation

The fast smoke and disruptive failover smoke are separate. The Community smoke performs direct cross-gateway PUT/GET/LIST checks against `namros-gateway-a` and `namros-gateway-b`, then reuses the load-balancer compatibility smoke for bucket create/list/delete, object PUT, HEAD, GET, range GET, multipart upload, versioning, tagging, CORS, and presigned GET through the active endpoint. Explicit etcd registration and TiKV identity assertions remain release-evidence work.

The current failover smoke stops one gateway, one SBS data container, and one SBS service container in controlled stages. It runs the load-balancer smoke while each component is down, restores the component, then runs a cross-gateway recovery smoke. Each run records `summary.json` and `events.jsonl` under `.cache/container-community-failover-smoke`. The initial acceptance criterion has no fixed recovery-time SLA: retries are bounded by an explicit test timeout, and measured durations are recorded rather than claimed as a product SLA.

Minimum and recommended CPU, memory, disk, image-download size, and expected startup time are published only after measurements on supported `amd64` and `arm64` hosts. Unmeasured estimates must be labeled as provisional.

## Data Lifecycle And Operations

| Target | Data behavior |
| --- | --- |
| `container-community-down` | stop the stack and preserve volumes |
| `container-community-reset` | permanently remove etcd, TiKV, and SBS test state |
| `container-build` | build local gateway and tools images from NAMROS source; Community targets also build transition SBS images from the configured NAMRBD context |

Reset commands print the exact project and named volumes they will remove. Upgrade and rollback support is defined by a release bill of materials and an explicit metadata/storage format compatibility statement; retaining volumes across an unsupported downgrade is not permitted.

## Implementation Acceptance Checklist

- Public multi-stage images build without a development-only local NAMRBD module replacement.
- Linux amd64 and arm64 images run as non-root with a read-only root filesystem.
- The local profile starts, persists data across down/up, passes smoke, and resets only through the destructive target.
- The Community profile starts all expected replicas and passes normal and failover smoke.
- HAProxy routes only to ready gateways; etcd contains both explicit gateway identities.
- Readiness fails for unavailable mandatory metadata, storage, or multi-gateway coordination dependencies.
- Secrets use file inputs, obey the documented precedence, and do not appear in inspect, logs, debug output, or reports.
- Debug endpoints are absent from the public S3 listener and metrics are not host-published by default.
- Image digests, SBOM, provenance, vulnerability results, and the tested component bill of materials are release artifacts.
- Measured resource requirements and failover timings are attached to the tested release.

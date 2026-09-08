Object Storage Product Docs

# NAMROS

<div class="note" markdown="1">

**Capability status.** The public Community distribution is the runnable
open-source platform described first in this manual. Areas marked
<span class="badge enterprise">Enterprise development</span> describe work being
implemented or validated in NAMROS Enterprise. A roadmap/specification label
means that the documented contract is not current product behavior.

</div>

<div class="summary" markdown="1">

NAMROS expands to Network Attached Multipath Resilient Object Storage and is pronounced [nae-muh-ross].

NAMROS is an open-source, S3-compatible object storage platform. The public
NAMROS Community distribution includes normal S3 object workflows,
external-client compatibility, active-active gateway operation, TiKV metadata,
etcd coordination, SBS replicated object storage, lifecycle/GC foundations,
and read-only operations surfaces.

</div>

![NAMROS platform overview](architecture-manual/assets/diagrams/platform-overview.svg)

## Product Positioning

NAMROS is an object storage product: Network Attached Multipath Resilient Object Storage, pronounced [nae-muh-ross]. It accepts S3-compatible requests through `namros-gateway`, stores namespace state in a metadata backend, and stores payload bytes through a segment storage backend. The gateway process is intentionally stateless with respect to authoritative object state.

NAMROS is not NAMRBD. NAMRBD is a network attached block-device product. NAMROS may reuse SBS/NAMRBD substrate pieces for Enterprise physical storage, but users interact with NAMROS through S3 clients and object storage semantics.

## Supported Deployment Shapes

| Shape | Purpose | Primary Dependencies | Edition |
| --- | --- | --- | --- |
| Local Community | Development, S3 API verification, user-space compatibility smoke | single `namros-gateway`, Pebble or memory metadata, local segment store | <span class="badge">Community</span> |
| Compatibility Lab | AWS CLI, MinIO client, rclone, s3fs-fuse validation | local gateway plus client tools; Linux FUSE host when needed | <span class="badge">Community</span> |
| Active-active Metadata Lab | multi-gateway availability and cache correctness | TiKV/PD, etcd, shared segment path | <span class="badge">Community</span> |
| SBS EC Development Lab | Enterprise EC multipart and degraded-read validation | TiKV/PD, SBS service/data, prepared volume and shard routes | <span class="badge enterprise">Enterprise development</span> |

## 5-Minute Community Quick Start

This path is intended for a GitHub developer checking the public Community tree for the first time.

```sh
git clone https://github.com/nosway/namros.git
cd namros
make test-community
make build-community
make run-dev
```

Keep the gateway running, then use a second terminal for a basic S3 round trip:

```sh
export NAMROS_ENDPOINT=http://127.0.0.1:9000
export AWS_ACCESS_KEY_ID=namrosroot
export AWS_SECRET_ACCESS_KEY=namrosrootsecret
export AWS_DEFAULT_REGION=us-east-1

aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-bucket --bucket quickstart
printf 'hello namros\n' > /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object --bucket quickstart --key hello.txt --body /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object --bucket quickstart --key hello.txt /tmp/namros-readback.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api list-objects-v2 --bucket quickstart
```

Expected result: the final list includes `hello.txt`, and `/tmp/namros-readback.txt` matches the original payload.

## Current Platform And Advanced Feature Status

| Capability | Status | Meaning |
| --- | --- | --- |
| S3 bucket/object API, multipart, versioning, tags, metadata, CORS | <span class="badge">Community included</span> | Implemented in the public source distribution and covered by compatibility checks. |
| TiKV metadata, etcd gateway registry, active-active gateways | <span class="badge">Community included</span> | Distributed Community platform foundation. |
| SBS replicated object storage | <span class="badge">Community included</span> | Uses the public NAMRBD Community module. |
| Lifecycle/GC, quota records and local request controls | <span class="badge">Community included</span> | Current public foundations; aggregate cluster-wide controls remain follow-on work. |
| Read-only console, metrics, reports, MCP diagnostics | <span class="badge">Community included</span> | Current public operational and troubleshooting surfaces. |
| EC storage classes, Object Lock/WORM, verified dedupe, SSE-KMS | <span class="badge enterprise">Enterprise development</span> | Implementation foundations exist and are being hardened or validated in Enterprise. |
| Compliance evidence, external IAM, approved operations | <span class="badge enterprise">Enterprise development</span> | Partial foundations exist; provider integration, action bodies, and production hardening continue. |
| Cross-region replication/DR, event delivery, inventory/batch | <span class="badge planned">Planned specification</span> | Design targets, not currently available behavior. |

See [Advanced features](advanced-features.md) for feature-by-feature scope and
status. Community builds explicitly reject requests that require unsupported
advanced semantics; the rejection behavior is not an Enterprise availability
claim.

## Persona-Based Navigation

We recommend starting paths tailored to your specific organizational persona and technical goals:

<div class="cards" markdown="1">

<div class="card" markdown="1">

### Application Developer

Application Developer Path

Learn S3-compatible endpoints, credential authorization, multipart upload APIs, and integration runbooks using SDKs (Go, Python, Java) and AWS CLI.

[Open the S3 API compatibility reference →](s3-api-compatibility-reference.md)

[Open User Manual →](user-manual.md)

</div>

<div class="card" markdown="1">

### System Administrator

Cluster Operator Path

Configure preflight OS kernel parameter tuning, manage etcd/TiKV clusters, execute node maintenance flows, and run self-healing/rebalance procedures.

[Open Admin Guide →](admin-guide.md)

</div>

<div class="card" markdown="1">

### Architecture Reviewer

System Architect & Security Path

Analyze the current stateless active-active Community architecture, then review
the separately labeled EC, IAM, and KMS Enterprise development contracts.

[Open Architecture Manual →](architecture-manual/index.md)

</div>

<div class="card" markdown="1">

### Operations Planner

Advanced Feature Path

Review which Enterprise capabilities have implementation foundations under
validation and which remain roadmap or specification-stage work.

[Open Advanced Features →](advanced-features.md)

</div>

</div>

## Current Validation Status

The HTML documentation set is validated by `make html-docs-check`. Product behavior is validated by unit tests, source-boundary checks, and container smoke targets depending on the deployment shape.

| Target | Purpose | Reference |
| --- | --- | --- |
| `make docs-render-check` | documentation build, rendered page bodies, resolved diagram paths | `tools/check-docs-render.py` |
| `make check-community-export` | Community identity, Enterprise boundary, focused gate tests | [release boundary](architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| `make container-local-smoke` | containerized local gateway smoke | [container deployment guide](container-deployment-guide.md) |

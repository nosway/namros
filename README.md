# ![NAMROS logo](internal/gateway/console_static/namros-logo.svg) NAMROS

[![Community](https://github.com/nosway/namros/actions/workflows/community.yml/badge.svg)](https://github.com/nosway/namros/actions/workflows/community.yml)
[![Docs](https://github.com/nosway/namros/actions/workflows/docs-pages.yml/badge.svg)](https://github.com/nosway/namros/actions/workflows/docs-pages.yml)
[![Security](https://github.com/nosway/namros/actions/workflows/security.yml/badge.svg)](https://github.com/nosway/namros/actions/workflows/security.yml)
[![Release](https://img.shields.io/github/v/release/nosway/namros)](https://github.com/nosway/namros/releases)
[![License](https://img.shields.io/github/license/nosway/namros)](LICENSE)

NAMROS is an open-source, S3-compatible object storage platform built around a
metadata-first architecture and stateless gateways. It separates authoritative
object metadata from payload storage so gateway instances can be replaced or
scaled horizontally without becoming the namespace authority.

The public NAMROS Community distribution includes core S3 object workflows,
multipart uploads, versioning, lifecycle processing, distributed metadata,
active-active gateway coordination, replicated SBS object storage, and
operator-facing observability. It can run locally, in containers, or as a
multi-gateway evaluation topology on Kubernetes.

![NAMROS platform overview](docs-src/manuals/architecture-manual/assets/diagrams/platform-overview.svg)

## What You Can Run Today

NAMROS Community provides:

- S3-compatible bucket and object CRUD, list, HEAD, range GET, copy, delete,
  multipart upload, tagging, user metadata, CORS, presigned URLs, and
  versioning/delete-marker behavior.
- SigV4 authentication with local access keys and basic bucket/prefix policy
  evaluation.
- Memory and Pebble metadata for local development, and TiKV as the
  authoritative distributed metadata backend.
- Stateless active-active gateways with etcd registration, health, and
  coordination.
- Local segment storage and replicated SBS object storage through the public
  [NAMRBD Community](https://github.com/nosway/namrbd) module.
- Lifecycle planning and workers, incomplete multipart expiration,
  protected-aware garbage collection, and metadata export/restore validation
  foundations.
- Bucket maximum-object-size limits, tenant quota records, active multipart
  upload admission, tenant usage reconciliation, and gateway-local request and
  bandwidth control foundations.
- Health, readiness, admin status, Prometheus metrics, Grafana and Alertmanager
  assets, a read-only web console, report views, MCP diagnostics, runbooks, and
  incident bundles.
- Packaging for Go binaries, Docker Compose, Helm, and kind, with compatibility
  paths for AWS CLI, MinIO Client, rclone, and s3fs-fuse.

See the [manual portal](docs-src/manuals/index.md) for capability details and
the [S3 client compatibility guide](docs-src/manuals/s3-client-compatibility-guide.md)
for tested client workflows.

## Quick Start

Prerequisites:

- Go 1.26 or newer.
- POSIX shell utilities.
- Optional S3 client tools for compatibility smoke tests: AWS CLI, MinIO Client,
  and rclone.

Run a local gateway with Pebble metadata and local segment storage:

```sh
go run ./cmd/namros-gateway \
  -http-listen 127.0.0.1:9000 \
  -region us-east-1 \
  -metadata-backend pebble \
  -metadata-path .namros/meta \
  -storage-backend local \
  -storage-path .namros/segments
```

The default bootstrap credentials are for local development only:

```sh
export AWS_ACCESS_KEY_ID=namrosroot
export AWS_SECRET_ACCESS_KEY=namrosrootsecret
export AWS_DEFAULT_REGION=us-east-1
```

Verify the gateway with AWS CLI:

```sh
printf 'hello from NAMROS\n' > /tmp/namros-hello.txt
aws --endpoint-url http://127.0.0.1:9000 s3 mb s3://namros-quickstart
aws --endpoint-url http://127.0.0.1:9000 \
  s3 cp /tmp/namros-hello.txt s3://namros-quickstart/hello.txt
aws --endpoint-url http://127.0.0.1:9000 \
  s3 cp s3://namros-quickstart/hello.txt -
```

Do not reuse the bootstrap credentials for a shared or externally reachable
deployment.

## Deployment Options

Run the public gateway with an SBS replicated backend through Docker:

```sh
make container-sbs-quickstart-smoke
```

This starts one gateway, one SBS service, two SBS data nodes, and PD/TiKV test
metadata. The gateway is published at `http://127.0.0.1:9002`; stop it with
`make container-sbs-quickstart-down`.

Run the multi-gateway Kubernetes evaluation topology on kind:

```sh
make kind-production-deploy
```

The rendered topology contains two gateways, two SBS services, five SBS data
nodes, and embedded TiKV for distributed-path evaluation. Its default
`emptyDir` volumes are ephemeral and the current chart requires customization
for durable SBS PersistentVolumeClaims, so this topology must not be used for
durable data. See the
[container deployment guide](docs-src/manuals/container-deployment-guide.md)
for topology, persistence, restart, and cleanup instructions.

For a filesystem-style interface over S3, see the
[s3fs-fuse workflow](docs-src/manuals/s3-client-compatibility-guide.md#s3fs-fuse-on-linux).
Object storage does not provide every semantic or performance characteristic of
a native POSIX filesystem.

## Architecture and Deployment Maturity

NAMROS keeps bucket, object-version, multipart, policy, lifecycle, and segment
placement metadata in a metadata authority. Gateways use that authority to
publish and resolve immutable object manifests while payload storage remains a
separate backend. In the distributed Community topology, TiKV provides
authoritative metadata, etcd provides gateway coordination, and SBS provides
replicated payload storage.

The repository includes local and distributed evaluation paths and automated
compatibility, failover, release, and packaging checks. Production suitability
still depends on the selected persistence, redundancy, security, monitoring,
backup, upgrade, and failure-domain design. The bundled kind deployment is a
production-shaped evaluation environment, not a supported durable reference
deployment.

## Advanced Features

The following capabilities are being developed and validated in NAMROS
Enterprise. They are not part of the current open-source distribution, and
this section describes development direction rather than general availability.

- **Erasure-coded storage classes** — EC-backed object placement, multipart
  storage, degraded reads, and replicated/EC storage-class routing.
- **Object Lock and WORM controls** — retention, legal hold, governance and
  compliance-mode enforcement, protected references, and audit evidence.
- **Verified deduplication** — byte-verified shared-object processing,
  reference accounting, repair, and scrub workflows.
- **KMS-backed encryption** — envelope encryption, key lifecycle and key-state
  admission, rotation/revocation evidence, and external provider integration.
- **Compliance control and evidence support** — policy profiles, immutable
  audit chains, retention evidence, policy simulation, and discovery exports.
  These controls do not constitute a certification or legal compliance claim.
- **External identity integration** — IAM/IdP mapping, temporary sessions,
  policy decisions, and principal/session evidence.
- **Approved operations** — plan, preflight, approval, apply, verification, and
  audit workflows across console, CLI, and MCP operations interfaces.

Cross-region replication and disaster recovery, event notifications,
large-scale inventory and batch operations, and cluster-wide aggregate
quota/QoS are Enterprise roadmap or specification-stage capabilities. They
should not be treated as generally available functionality.

Community builds do not contain a runtime flag, environment variable, or public
build tag that enables Enterprise behavior. Requests that require unsupported
advanced semantics fail explicitly instead of being accepted without the
required enforcement. See the
[release and edition boundary guide](docs-src/manuals/architecture-manual/chapters/14-release-and-edition-boundaries.md)
for the source and runtime boundary.

## Common Checks

Build Community binaries into `bin/community`:

```sh
make build-community
```

Run the default tests and public documentation checks:

```sh
make test
make docs-render-check
```

Run container and S3 compatibility checks:

```sh
make container-local-smoke
make container-sbs-quickstart-smoke
make compat-public-s3
```

Check and export the public Community source boundary:

```sh
make check-community-export
make export-community
make check-publication-readiness
```

## Source Distribution

The release tooling keeps the public Community build identity fixed, rejects
public Enterprise unlock paths, and excludes private implementation files from
the Community source export. `make export-community` also removes the temporary
development-only local NAMRBD module replacement from exported source.

## Documentation and Community

The manual set is published at <https://nosway.github.io/namros/>. Every push to
`main` render-checks `docs-src/`; maintainers can also deploy the rendered site
through the `Docs` workflow. Documentation source remains in the repository so
the published manuals can be reproduced locally.

Useful starting points:

- [Manual portal](docs-src/manuals/index.md)
- [Installation guide](docs-src/manuals/installation-guide.md)
- [Container deployment guide](docs-src/manuals/container-deployment-guide.md)
- [S3 client compatibility guide](docs-src/manuals/s3-client-compatibility-guide.md)
- [Architecture manual](docs-src/manuals/architecture-manual/index.md)
- [Contributing guide](CONTRIBUTING.md)
- [Roadmap](ROADMAP.md)
- [Support](SUPPORT.md)
- [Governance](GOVERNANCE.md)
- [Security policy](SECURITY.md)

Build the documentation locally with:

```sh
python -m pip install -r docs-src/requirements.txt
make docs-render-check
mkdocs serve
```

## License

NAMROS is licensed under the Apache License, Version 2.0. See
[LICENSE](LICENSE) and [NOTICE](NOTICE).

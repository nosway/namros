# ![NAMROS logo](internal/gateway/console_static/namros-logo.svg) NAMROS

NAMROS is Network Attached Multipath Resilient Object Storage: a Community
Edition S3-compatible object storage gateway with a metadata-first design,
stateless gateway operation, and explicit Community/Enterprise feature
boundaries.

The Community Edition is intended to be useful for development, compatibility
testing, and baseline production-shaped deployments. Enterprise-only features
are not unlocked by flags, environment variables, or public build tags.

## Community Scope

Community includes:

- Core S3-compatible bucket and object workflows.
- Multipart upload, range GET, HEAD, copy, delete, tagging, metadata, CORS, and
  versioning behavior.
- Local, memory, Pebble, TiKV, and etcd-backed deployment shapes.
- Stateless active-active gateway coordination through etcd.
- SBS replicated object storage when paired with the public NAMRBD Community
  module.
- Read-only operational status, metrics, and basic console surfaces.

Enterprise-only features return explicit NAMROS Enterprise Edition requirement
errors in the Community build:

- WORM/Object Lock enforcement and compliance evidence.
- Deduplication workers, repair, scrub, and shared-object accounting.
- SBS erasure coding, EC storage-class routing, and classroute operation.
- SSE-KMS lifecycle, key-state admission, and external IAM federation.
- Advanced operational approval workflows and Enterprise release gates.

See the
[release and edition boundary guide](docs-src/manuals/architecture-manual/chapters/14-release-and-edition-boundaries.md)
for the public source boundary.

## Quick Start

Prerequisites:

- Go 1.26 or newer.
- POSIX shell utilities.
- Optional S3 client tools for compatibility smoke tests: AWS CLI, MinIO client,
  and rclone.

Run a local gateway with Pebble metadata and local segment storage:

```sh
go run ./cmd/namros-gateway \
  -listen 127.0.0.1:9000 \
  -region us-east-1 \
  -metadata-backend pebble \
  -metadata-path .namros/meta \
  -storage-backend local \
  -storage-path .namros/segments
```

The default bootstrap credentials for local development are:

```text
AWS_ACCESS_KEY_ID=namrosroot
AWS_SECRET_ACCESS_KEY=namrosrootsecret
AWS_DEFAULT_REGION=us-east-1
```

Use a local S3 client against `http://127.0.0.1:9000`.

To try the public gateway with an SBS backend through Docker:

```sh
make container-sbs-quickstart-smoke
```

This starts a lightweight Compose topology with one gateway, one SBS service,
two SBS data nodes, and PD/TiKV test metadata. The gateway is published at
`http://127.0.0.1:9002`; stop it with `make container-sbs-quickstart-down`.

To try the production-shaped Kubernetes topology on kind:

```sh
make kind-production-deploy
```

The default config file is `packaging/k8s/production-kind.env`. It renders a
Helm deployment with 2 gateways, 2 SBS services, 5 SBS data nodes, and one
embedded TiKV instance for evaluation. The kind gateway is mapped to
`http://127.0.0.1:9000`. When finished, delete the kind cluster and all of its
ephemeral test state:

```sh
make kind-production-down
```

## Common Checks

Build Community binaries into `bin/community`:

```sh
make build-community
```

Run unit tests:

```sh
make test
```

Run a containerized local smoke test:

```sh
make container-local-smoke
```

Run the gateway plus SBS backend Docker quickstart smoke:

```sh
make container-sbs-quickstart-smoke
```

Render the production-shaped Kubernetes/kind values and manifests:

```sh
make k8s-production-render
```

Check the Community source boundary:

```sh
make check-community-export
```

Create a Community source export:

```sh
make export-community
```

Run the publication readiness checks:

```sh
make check-publication-readiness
```

Run the strict public S3 compatibility smoke:

```sh
make compat-public-s3
```

## Source Distribution

This repository is prepared so the public Community tree can be checked and
exported without exposing Enterprise implementation bodies. The release tooling
keeps the Community identity fixed, rejects public Enterprise unlock paths, and
excludes private implementation files from the Community source export.

NAMRBD is consumed as a separate Community module at
`github.com/nosway/namrbd`. `make export-community` removes any
temporary development-only module replacement from the exported public source
tree.

## Documentation

Useful starting points:

The manual set is published at <https://nosway.github.io/namros/> after a
maintainer enables GitHub Pages and runs the `Docs` workflow with
`deploy_pages=true`. Every push to `main` render-checks `docs-src/`, but does
not require Pages to be enabled. No rendered HTML is committed, so the sources
cannot drift from what readers see.

- [Manual portal](docs-src/manuals/index.md)
- [Installation guide](docs-src/manuals/installation-guide.md)
- [Container deployment guide](docs-src/manuals/container-deployment-guide.md)
- [S3 client compatibility guide](docs-src/manuals/s3-client-compatibility-guide.md)
- [Architecture manual](docs-src/manuals/architecture-manual/index.md)

Build the site locally with:

```sh
python -m pip install -r docs-src/requirements.txt
make docs-render-check
mkdocs serve
```

## License

NAMROS is licensed under the Apache License, Version 2.0. See
[LICENSE](LICENSE) and [NOTICE](NOTICE).

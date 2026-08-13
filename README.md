# NAMROS

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

The manual set is published at <https://nosway.github.io/namros/>, built from
`docs-src/` on every push to `main`. No rendered HTML is committed, so the
sources cannot drift from what readers see.

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

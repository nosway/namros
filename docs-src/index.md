# NAMROS

NAMROS is an open-source, S3-compatible object storage platform with a
metadata-first design and stateless gateways. The public NAMROS Community
distribution includes the S3 data path, distributed metadata, active-active
gateway coordination, replicated SBS storage, deployment assets, and
operator-facing observability.

This site is built from `docs-src/` in the public source tree. It describes
what can be run from the open-source distribution first. Pages marked
<span class="badge enterprise">Enterprise development</span> document advanced
capabilities being developed or validated in NAMROS Enterprise; they do not
mean that a generally available Enterprise release exists.

## Start Here

- [Manual portal](manuals/index.md) — product positioning, deployment shapes,
  and reader paths.
- [Installation guide](manuals/installation-guide.md) — prerequisites, bring-up,
  and verification.
- [User manual](manuals/user-manual.md) — S3 workflows and client usage.
- [S3 API compatibility reference](manuals/s3-api-compatibility-reference.md) —
  current operation-by-operation support, edition gates, and known differences.
- [S3 API Swagger view](manuals/s3-api-swagger.md) — view-only OpenAPI rendering
  of the physical S3 dispatchers and logical operation catalog.
- [Admin guide](manuals/admin-guide.md) — day-2 operations and troubleshooting.
- [Architecture manual](manuals/architecture-manual/index.md) — component
  ownership, metadata authority, and storage contracts.
- [Advanced features](manuals/advanced-features.md) — Enterprise development
  status and specification-stage roadmap items.

Korean translations of the manual set are available under
[`manuals/ko/`](manuals/ko/index.md).

## Local Build

```bash
python -m pip install -r docs-src/requirements.txt
make docs-render-check
mkdocs serve
```

## Source Checks

Community source, boundary, and packaging gates live in the repository root:

```bash
make test
make check-community-export
make docs-render-check
```

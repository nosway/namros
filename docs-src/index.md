# NAMROS Community

NAMROS is Network Attached Multipath Resilient Object Storage: an
S3-compatible object storage gateway with a metadata-first design, stateless
gateway operation, and explicit Community/Enterprise feature boundaries.

This site is built from `docs-src/` in the public source tree. Pages marked
<span class="badge enterprise">Enterprise edition only</span> describe
capabilities that are not available in Community builds except for documented
denial behavior.

## Start Here

- [Manual portal](manuals/index.md) — product positioning, deployment shapes,
  and reader paths.
- [Installation guide](manuals/installation-guide.md) — prerequisites, bring-up,
  and verification.
- [User manual](manuals/user-manual.md) — S3 workflows and client usage.
- [Admin guide](manuals/admin-guide.md) — day-2 operations and troubleshooting.
- [Architecture manual](manuals/architecture-manual/index.md) — component
  ownership, metadata authority, and storage contracts.

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

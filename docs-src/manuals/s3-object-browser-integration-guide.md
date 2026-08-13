Community Operations

# S3 Object Browser Integration Guide

<div class="summary" markdown="1">

NAMROS will provide Object Explorer Lite in the operations console and document external S3 browser tools for full file-management workflows. This keeps the console inside the product operations-plane boundary while still making Community deployments easy to inspect and test.

</div>

## Recommended Product Direction

The built-in console should not become a second S3 client implementation or a direct metadata editor. Community scope is a read-only Object Explorer Lite for bucket, prefix, object, version, tag, retention, and encryption visibility. Upload, recursive copy/move, bulk delete, inline editing, and broad download workflows should be handled by validated external S3-compatible tools.

Single-object download and delete can be considered later only through the same gateway, RBAC, CSRF, plan/preflight/apply/verify/audit, Object Lock, retention, and audit controls used by other approved operations.

## NAMROS Object Explorer Lite

| Capability | Community behavior | Boundary |
| --- | --- | --- |
| Bucket listing | Show buckets and operational status. | Read-only list. |
| Prefix/object listing | Support prefix, delimiter, pagination, and version-aware listing. | No payload bytes. |
| Object detail | Show HEAD metadata, tags, ETag, size, content type, version id, delete marker, retention, lock, and encryption posture. | No private metadata decoding. |
| Download/delete/upload | Disabled by default. | Requires a later approved operation policy. |
| Metrics | Do not expose raw object keys or arbitrary prefixes as metric labels. | Use report jobs for prefix analytics. |

## External Tool Integration Matrix

| Tool | Recommended use | NAMROS policy | Reference |
| --- | --- | --- | --- |
| AWS CLI | Baseline S3 compatibility, automation, smoke tests. | First-class compatibility target. | [S3 client guide](s3-client-compatibility-guide.md) |
| MinIO client (`mc`) | Operator CLI browsing, copy, stat, mirror, and extended S3 workflows. | First-class compatibility target. | [mc smoke](s3-client-compatibility-guide.md#mc) |
| rclone | Migration, synchronization, scripted copy/delete workflows. | First-class compatibility target. | [rclone S3 backend](https://rclone.org/s3/) |
| Cyberduck | Desktop GUI object browsing for operators and testers. | Document as compatible; do not bundle. | [Cyberduck docs](https://docs.cyberduck.io/) |
| Brows3 | Desktop S3 browser candidate. | Optional after NAMROS compatibility validation. | [Brows3](https://www.brows3.app/) |
| Filestash | Self-hosted web file manager. | Optional integration only; not bundled by default. | [Filestash S3 browser](https://www.filestash.app/s3-browser.html) |
| MinIO Console | Benchmark for object browser and operations UX. | Benchmark only; not a NAMROS dependency. | [MinIO Console docs](https://minio.community/community/minio-object-store/administration/minio-console.html) |

## Security Guardrails

- Do not place root credentials, access key secrets, session tokens, or presigned URLs in HTML, logs, support bundles, or container environment that is exposed through inspection.
- Prefer temporary or least-privilege S3 credentials for external tools.
- Render object keys and metadata as untrusted strings. Escape HTML and avoid executing copied values.
- Use bucket and prefix allowlists for console visibility when a deployment is multi-tenant or shared.
- Keep destructive object operations disabled until RBAC, explicit approval, Object Lock/retention checks, and audit persistence are complete.

## Validation Checklist

1. Run S3 client smoke commands against the target endpoint.
2. Verify AWS CLI, MinIO client, and rclone can list, put, head, get, copy, move/delete, and multipart copy according to the S3 client guide.
3. Validate Cyberduck and any GUI candidate with path-style endpoint configuration before documenting it as supported for a release.
4. Confirm Object Explorer Lite responses never include payload bytes, secret values, or presigned URLs.
5. Confirm object key and prefix values are absent from Prometheus labels and high-cardinality metrics.

## Implementation Phases

| Phase | Scope | Exit condition |
| --- | --- | --- |
| Phase 1 | Document external tools and add redacted client recipes. | Compatibility guide and HTML docs pass. |
| Phase 2 | Add read-only Object Explorer Lite API and console panel. | List/head unit tests prove no payload or secret leakage. |
| Phase 3 | Evaluate optional single-object download. | Gateway-mediated path, RBAC, audit, and policy controls are complete. |
| Phase 4 | Evaluate approved single-object delete/version delete. | Plan/preflight/apply/verify/audit and retention checks are complete. |

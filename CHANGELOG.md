# Changelog

All notable changes to the public NAMROS Community edition are documented in
this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and public release and upgrade procedures are documented in the
[upgrade and release operations guide](docs-src/manuals/upgrade-release-operations-guide.md).

Enterprise-only behavior is listed only under `Edition: Enterprise only`.
Private Enterprise bundles may ship additional operator notes, but public
release history stays in this file.

## [Unreleased]

### Added

### Changed

- Updated the NAMRBD module dependency and default NAMRBD container build
  contexts from v1.1.1 to v1.1.2.

### Fixed

- Replaced the retired floating MinIO Client download URL in Community CI with
  a pinned official `minio/mc` release asset and verified SHA-256 before
  installation.

### Deprecated

### Removed

### Security

- Adopted NAMRBD v1.1.2 and gRPC-Go 1.83.2 so NAMROS does not retain the
  `GHSA-2v4p-qf9q-27wj` / `CVE-2026-84445` denial-of-service vulnerability in
  its module graph or default SBS build context.

### Edition: Community

### Edition: Enterprise only

### Support & Evidence

### Upgrade & Migration

### Compatibility

- NAMRBD v1.1.2 is now the expected SBS module and container build-context
  version. No NAMROS metadata or object-data migration is required.

### Known Limits

## [1.0.4] - 2026-09-08

### Added

- Added a machine-readable OpenAPI contract, Swagger entry point, and English
  and Korean S3 API compatibility references for the public gateway surface.
- Added public governance, maintainer, roadmap, support, issue-template, and
  security workflow material for the Community project.

### Changed

- Upgraded the NAMRBD module dependency and all default container build
  contexts to NAMRBD v1.1.1.
- Hardened S3 request routing for query-subresource and header-selected
  operations, with compatibility coverage for the documented public surface.
- Updated the public dependency and CI baseline, including current S3 gateway,
  transport security, erasure-coding, and GitHub Actions dependencies.
- Updated the Helm chart and application metadata to version 1.0.4.

### Fixed

- Enabled the NAMRBD lab store debug endpoint on SBS quickstart data nodes so
  the bootstrap can materialize replicated volumes before starting the gateway.
- Kept public Community source export, documentation rendering, and release
  workflow checks aligned with the exported source tree.

### Security

- Added dedicated public security automation for vulnerability scanning,
  CodeQL analysis, and pull-request dependency review.

### Edition: Community

- Preserved the Community-only runtime boundary and explicit Enterprise feature
  denial behavior across the expanded public API and documentation surface.

### Upgrade & Migration

- Gateway listener configuration now uses `-http-listen`; deployments using the
  former `-listen` flag must update their command-line arguments.
- Existing NAMROS metadata and object data do not require a release-specific
  migration for this patch release.

### Compatibility

- NAMRBD v1.1.1 is the expected SBS module and container build-context version
  for this release.
- The Community release gate, direct-module test suite, strict documentation
  render, and SBS service/data/S3 quickstart path pass with the v1.1.1 runtime.

## [1.0.3] - 2026-08-21

### Changed

- Updated the NAMRBD module dependency and SBS integration paths for the
  NAMRBD v1.0.0 public release.
- Updated SBS service naming, flags, container bootstrap wiring, Helm
  templates, and related English/Korean operations documentation.
- Restored the etcd-backed gateway coordination implementation and tests used
  by active-active Community deployments.

### Compatibility

- NAMRBD v1.0.0 is the expected SBS module version for this release.
- Deprecated compatibility flags removed by the NAMRBD v1.0.0 transition are
  no longer accepted.

## [1.0.2] - 2026-08-20

### Fixed

- Fixed the rclone compatibility smoke so endpoint configuration is passed in
  the form expected by the public test environment.
- Included the preceding public CI build fixes that removed an unpublished
  module dependency and restored a self-contained Community coordination path.

### Known Limits

- No `v1.0.1` tag was published; its CI hotfix commit is included in v1.0.2.

## [1.0.0] - 2026-08-20

Initial formal semver release. This version marks the current Community
publication baseline as the starting point for NAMRBD-independent NAMROS product
versioning.

### Added

- Community S3-compatible gateway, admin tooling, and publication/export
  workflow documented in the architecture manual and installation guides.
- Community release gates: edition boundary checks, source export, publication
  readiness, production-scale checks, container smoke, and release metadata
  generation.
- S3 client compatibility coverage for AWS CLI, MinIO client, and rclone in user-
  space and public compat smokes.
- Active-active gateway coordination with TiKV metadata and etcd registry in
  the Community baseline.
- SBS replicated storage integration path for Community deployments.

### Changed

- Formal release, versioning, and changelog policy established; public
  procedures are summarized in the upgrade and release operations guide.

### Edition: Community

- Community builds expose the S3-compatible distributed platform baseline
  without Enterprise unlock paths.
- Enterprise-only requests return explicit Enterprise edition requirement
  errors.

### Edition: Enterprise only

- Private overlay distribution for EC, WORM enforcement, dedupe execution, KMS,
  and compliance evidence remains outside the public Community source tree.

### Support & Evidence

- Baseline established from current repository state; full GA publication
  should attach `release-readiness` and `community-release-check` artifacts at
  tag cut time.

### Upgrade & Migration

- First GA release; no prior semver migration path.

### Compatibility

- NAMRBD/SBS context pins are recorded in release metadata and Docker compose
  defaults. Verify the tested NAMRBD revision in the release attachment
  manifest when upgrading.

### Known Limits

- Enterprise features are visible only as documented denial stubs in Community
  builds.
- Deployment suitability depends on external persistence, failure-domain,
  security, monitoring, backup, and upgrade design; the bundled kind topology
  is an ephemeral evaluation environment.

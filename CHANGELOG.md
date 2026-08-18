# Changelog

All notable changes to the public NAMROS Community edition are documented in
this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and NAMROS product versioning follows the policy in
[docs/release-versioning-changelog-policy.md](docs/release-versioning-changelog-policy.md).

Enterprise-only behavior is listed only under `Edition: Enterprise only`.
Private Enterprise bundles may ship additional operator notes, but public
release history stays in this file.

## [Unreleased]

### Added

### Changed

### Fixed

### Deprecated

### Removed

### Security

### Edition: Community

### Edition: Enterprise only

### Support & Evidence

### Upgrade & Migration

### Compatibility

### Known Limits

## [1.0.0] - 2026-08-18

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

- Formal release, versioning, and changelog policy added in
  `docs/release-versioning-changelog-policy.md`.

### Edition: Community

- Community builds expose the production-capable S3 baseline without Enterprise
  unlock paths.
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
- SBOM generation is optional until `NAMROS_RELEASE_GENERATE_SBOM=1` and syft
  are enabled in the release pipeline.

Release Operations <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS Upgrade & Release Operations Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition release gates and Enterprise edition only private distribution packaging notes. Enterprise release gates and airgapped private enterprise packaging are not public Community deliverables.

</div>

This document defines the lifecycle management runbooks for validating NAMROS release readiness, executing rolling upgrades with zero-downtime, triggering rollbacks, deploying hotfixes, and packaging airgapped private enterprise distributions.

Product semver, Git tags, and public changelog rules are recorded in
[CHANGELOG.md](https://github.com/nosway/namros/blob/main/CHANGELOG.md) and
the [Release workflow](https://github.com/nosway/namros/actions/workflows/release.yml).

## Release Gate Checklist

```sh
make test
make test-community
make html-docs-check
make check-publication-readiness
make production-scale-check
make export-community
make container-local-smoke
```

<span class="badge enterprise">Enterprise edition only</span> release gates add EC, dedupe, WORM, KMS, compliance, IAM, and chaos/soak profiles where the private distribution is entitled.

## Upgrade Flow

1. Export metadata and capture release-readiness report.
2. Verify edition identity and feature entitlement catalog.
3. Upgrade one gateway or canary environment first.
4. Run compatibility and metadata smoke before full rollout.
5. Record post-upgrade report and operation audit.

## Rollback Requirements

| Area | Requirement |
| --- | --- |
| Metadata schema | Forward/backward compatibility or explicit migration boundary. |
| Binary | Previous artifact and config retained. |
| Reports | Pre/post upgrade evidence preserved. |

## Airgapped And Private Release

Community source export and private Enterprise overlay assembly are separate release tracks. Airgapped releases need artifact checksums, dependency manifests, offline client tool references, and post-install smoke artifacts.

The Community source export writes release metadata through `scripts/release/write-release-artifact-metadata.sh`. The metadata directory contains `release-metadata.json`, `checksums.sha256`, `provenance.json`, `go-modules.txt`, and `sbom-status.json`. `check-publication-readiness` verifies the metadata tooling and keeps private planning notes, lab hostnames, and Enterprise implementation references out of the public export.

Reference: [release and edition boundaries](architecture-manual/chapters/14-release-and-edition-boundaries.md).

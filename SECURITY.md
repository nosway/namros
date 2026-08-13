# Security Policy

## Supported Versions

Security fixes are currently accepted for the latest Community Edition source
on the `main` branch. Versioned support windows will be documented after the
first public release series is established.

## Reporting a Vulnerability

Please do not report suspected security vulnerabilities in public issues.

Until a dedicated private advisory channel is configured for the public GitHub
repository, send reports to:

```text
taewoong.kim@gmail.com
```

Include:

- Affected component and version or commit.
- Reproduction steps or proof-of-concept details.
- Expected and observed behavior.
- Impact assessment, if known.
- Whether the issue is already public.

We will acknowledge valid reports as soon as practical and coordinate disclosure
timing before publishing fixes or advisories.

## Scope

Security-sensitive areas include:

- S3 authentication and SigV4 verification.
- Bucket/object authorization and policy evaluation.
- Metadata integrity and versioning correctness.
- Object deletion, retention, and Enterprise-required denial behavior.
- Console authentication and session handling.
- Gateway, TiKV, etcd, and SBS deployment configuration.

Enterprise-only security and compliance features are not enabled by Community
runtime switches. Reports that identify an unintended Enterprise unlock path in
Community are treated as security issues.

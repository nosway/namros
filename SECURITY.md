# Security Policy

## Supported Versions

Security fixes are developed on `main` and released on the latest patch of the
current minor release series. Older patch releases should be upgraded before a
security fix is requested.

| Version | Supported |
| --- | --- |
| Latest `1.0.x` patch | Yes |
| Earlier `1.0.x` patches | Upgrade to the latest patch |
| Unreleased `main` | Receives fixes before the next patch release |
| Earlier release series | No |

## Reporting a Vulnerability

Please do not report suspected security vulnerabilities in public issues.

Use the repository's **Report a vulnerability** action under the Security tab
when private vulnerability reporting is available. If that action is not
available, send the report privately to:

```text
taewoong.kim@gmail.com
```

Include:

- Affected component and version or commit.
- Reproduction steps or proof-of-concept details.
- Expected and observed behavior.
- Impact assessment, if known.
- Whether the issue is already public.

Response targets, measured in business days, are:

- Initial acknowledgement within 3 days.
- Triage and an initial severity assessment within 7 days.
- A status update at least every 14 days while remediation is active.

These are response targets rather than contractual service-level guarantees.
We will coordinate disclosure timing and ask reporters not to publish details
until a fix or mitigation is available. Security advisories will identify
affected versions, remediation, credit preferences, and any known limitations.

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

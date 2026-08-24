# NAMROS Governance

NAMROS is an open-source project with maintainer-led decision making. The
project seeks technical consensus in public, while maintainers remain
accountable for merge, release, security, and source-boundary decisions.

## Decision Process

- Small fixes and documentation improvements are decided through pull-request
  review.
- New public APIs, metadata schema changes, compatibility changes, deployment
  defaults, and large architectural work should start with an issue.
- Proposals should describe user impact, alternatives, compatibility and
  migration effects, operational risk, and verification evidence.
- Maintainers seek consensus among affected contributors. If consensus cannot
  be reached, the responsible maintainer records the decision and rationale in
  the issue or pull request.

## Public Platform And Advanced Features

The public repository must remain a useful S3-compatible object storage
platform. Core S3 workflows, distributed metadata, active-active gateway
coordination, replicated storage, and Community operations are not treated as
demonstration-only placeholders.

Advanced features may be described publicly when their status is explicit:

- `Enterprise development` means an implementation foundation exists in the
  private development line and is being integrated, hardened, or validated.
- `Planned specification` means the document is a design target and not
  current product behavior.

Documentation must not use `available` for an Enterprise capability until a
maintainer has declared a supported release and documented its installation,
upgrade, compatibility, and limitations.

## Changes To Governance

Governance changes are made by pull request and require maintainer approval.
The current maintainers are listed in [MAINTAINERS.md](MAINTAINERS.md).

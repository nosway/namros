# Contributing to NAMROS

Thank you for helping improve the open-source NAMROS platform.

## Ways To Contribute

Bug fixes, S3 compatibility improvements, tests, deployment hardening,
observability, documentation, client recipes, and reproducible performance or
failure evidence are welcome. New contributors can start with issues labeled
`good first issue` or `help wanted`.

Before starting a new public API, metadata schema, deployment default, or large
architectural change, open an issue and describe the user problem, alternatives,
compatibility and migration effects, operational risks, and verification plan.
Small fixes can go directly to a focused pull request.

## Development Setup

1. Install Go 1.26 or newer.
2. Clone this repository.
3. Run the focused checks before opening a pull request:

```sh
make test
make check-community-export
make docs-render-check
```

If your change touches S3 compatibility behavior, also run:

```sh
make compat-public-s3
make compat-user-space
```

`make compat-public-s3` is the strict public reproducer used by CI and requires
AWS CLI, `jq`, `curl`, MinIO client, and rclone. `make compat-user-space` is a
developer convenience target that runs the installed client smokes and skips
missing clients.

Some smoke targets require TiKV, etcd, NAMRBD/SBS, or an 18-node lab
environment. Document any skipped environment-dependent verification in your
pull request.

## Community and Enterprise Boundary

Public contributions must preserve the Community Edition boundary:

- Do not add a public flag, environment variable, or build tag that turns a
  Community build into an Enterprise build.
- Community code may expose compatibility stubs for Enterprise-only surfaces,
  but those paths must return explicit NAMROS Enterprise Edition requirement
  errors.
- Enterprise implementation bodies belong in private source overlays, not in
  the public Community tree.
- Public documentation may describe advanced work, but it must distinguish
  `Enterprise development` from `Planned specification` and must not claim
  general availability without a supported release.

Run this check when touching edition-sensitive code:

```sh
make check-community-export
```

## Code Style

- Follow the style already present in the package being edited.
- Keep changes focused and avoid unrelated refactors.
- Prefer small tests that cover the behavior changed by the pull request.
- Use clear error messages for user-visible or operator-visible failures.

## Pull Request Process

- Link the issue for larger changes and keep the pull request focused.
- Explain user-visible behavior, compatibility, metadata, deployment, and
  upgrade effects.
- List the exact verification commands run and identify skipped
  environment-dependent checks.
- Add or update tests before requesting review.
- Respond to review comments with a follow-up commit or a short technical
  rationale.
- Maintainers may ask to split unrelated changes or to revise a proposal before
  implementation proceeds.

## Documentation

Update documentation when behavior, configuration, deployment shape, or edition
scope changes. Edit `docs-src/`; it is the only documentation source in this
repository, and the manual set is built from it and published to GitHub Pages.
No rendered HTML is committed, so there is no second copy to keep in sync.

Manual sources wrap callouts in component `<div>` blocks styled by
`docs-src/assets/namros-docs.css`. Any such block that contains Markdown must
carry `markdown="1"`, otherwise the body publishes as raw text. Link other
pages by their `.md` source path so mkdocs can resolve and validate the link.
Validate with:

```sh
make docs-render-check
```

## Certificate of Origin

By contributing, you certify that you have the right to submit the contribution
under the Apache License, Version 2.0. Unless explicitly stated otherwise, your
contributions are submitted under the same license as this project.

Add a `Signed-off-by` trailer to each commit with:

```sh
git commit --signoff
```

The trailer records agreement with the
[Developer Certificate of Origin](https://developercertificate.org/); it is not
a copyright assignment.

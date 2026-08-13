# Contributing to NAMROS

Thank you for helping improve NAMROS Community Edition.

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
make compat-user-space
```

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

Run this check when touching edition-sensitive code:

```sh
make check-community-export
```

## Code Style

- Follow the style already present in the package being edited.
- Keep changes focused and avoid unrelated refactors.
- Prefer small tests that cover the behavior changed by the pull request.
- Use clear error messages for user-visible or operator-visible failures.

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

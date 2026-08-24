# NAMROS Support

NAMROS Community support is provided on a best-effort basis through the public
GitHub repository.

## Where To Ask

- Use a [bug report](https://github.com/nosway/namros/issues/new?template=bug_report.md)
  for reproducible defects.
- Use a [feature request](https://github.com/nosway/namros/issues/new?template=feature_request.md)
  for a user problem or proposed improvement.
- Use a regular issue with the `question` label for installation, architecture,
  or operational questions that are not covered by the manuals.
- Follow [SECURITY.md](SECURITY.md) for vulnerabilities. Do not disclose them in
  a public issue.

Before opening an issue, check the
[manuals](https://nosway.github.io/namros/), existing issues, release notes, and
the current [roadmap](ROADMAP.md).

## Information To Include

Provide the NAMROS version or commit, operating system, deployment shape,
metadata and storage backends, client and command used, expected and actual
behavior, and redacted logs or report artifacts. Remove credentials,
authorization headers, presigned URLs, object payloads, and private endpoint
names.

## Support Boundaries

- The bundled kind topology is an ephemeral evaluation environment.
- Operators are responsible for external persistence, network and TLS design,
  identity configuration, backup, monitoring, and failure-domain planning.
- Advanced features labeled `Enterprise development` or `Planned
  specification` are not supported Community behavior.
- A response or workaround in an issue is not a contractual support-level
  commitment.

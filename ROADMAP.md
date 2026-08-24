# NAMROS Roadmap

This roadmap communicates direction rather than release dates or support
commitments. Completed behavior is documented in the manuals and changelog;
work is accepted only after its implementation and validation evidence land.

## Public Platform Priorities

Near-term Community work focuses on:

- Durable Kubernetes reference patterns with explicit external etcd/TiKV and
  SBS persistence guidance.
- Bounded-memory streaming for PUT, multipart, copy, GET, and range paths.
- Metadata scale, migration, large multipart, scrub, and repair hardening.
- Multi-volume SBS routing, shared-writer/session safety, drain, and worker
  ownership across gateway fleets.
- Community quota admission, gateway-local QoS, observability, compatibility,
  rolling-upgrade, backup/restore, and chaos/soak evidence.
- Clearer client compatibility and performance baselines with reproducible
  artifacts and known limitations.

## Advanced Feature Development

The following features have Enterprise implementation foundations under
integration, hardening, or validation:

- Erasure-coded storage classes and replicated/EC routing.
- Object Lock and WORM enforcement.
- Byte-verified post-process deduplication, shared-object accounting, repair,
  and scrub.
- KMS-backed encryption and key-state admission.
- Compliance control and evidence support.
- External identity mapping and temporary-session foundations.
- Approved console, CLI, and MCP operations.

See the [Advanced features guide](docs-src/manuals/advanced-features.md) for the
scope and limitations of each item.

## Specification-Stage Direction

Cross-region replication and DR, event notifications, scheduled inventory and
batch operations, and cluster-wide aggregate quota/QoS remain specification or
roadmap work. They must not be described as available until implementation,
compatibility, operations, and release gates are complete.

## Contributing To The Roadmap

Open an issue describing the user problem, target workload, operational impact,
alternatives, and how success can be tested. A roadmap entry does not reserve a
design; implementation decisions follow [GOVERNANCE.md](GOVERNANCE.md).

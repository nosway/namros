Appendix <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# Reference Map

## References

- Markdown docs
- scripts
- packages

<div class="note" markdown="1">

**Edition scope.** This appendix maps both Community edition public guides and Enterprise edition only reference areas. Rows mentioning WORM, dedupe, KMS, compliance, EC, replication, IAM, quota, inventory, or notifications may point to private-distribution contracts or Community denial behavior.

</div>

| HTML Topic | Public Guide | Code Surface |
| --- | --- | --- |
| S3 API scope | [S3 client compatibility](../../s3-client-compatibility-guide.md) | `internal/gateway`, `internal/s3api` |
| Compatibility | [compatibility guide](../../s3-client-compatibility-guide.md) | external S3 clients and `namros-s3bench` |
| s3fs-fuse | [FUSE compatibility notes](../../s3-client-compatibility-guide.md) | Linux FUSE host procedure |
| Metadata | [TiKV operations guide](../../tikv-ha-cluster-install-operations-guide.md) | `internal/meta` |
| Object schema | [metadata authority](04-metadata-authority.md), [object and segment model](05-object-and-segment-model.md) | `internal/meta/model/model.go`, `internal/meta/repository.go` |
| Segment storage abstraction | [object and segment model](05-object-and-segment-model.md), [backends](07-replicated-and-local-backends.md) | `internal/storage/storage.go`, `internal/storage/local`, `internal/storage/volumepool` |
| SBS replicated storage | [capacity guide](../../capacity-scaling-maintenance-guide.md) | `internal/storage/sbs` |
| EC/classroute | [SBS EC chapter](08-sbs-ec-backend-enterprise.md) | <span class="badge enterprise">Enterprise edition only</span> `internal/storage/classroute`, `internal/storage/sbs` |
| WORM/Object Lock | [versioning lifecycle Object Lock](09-versioning-lifecycle-object-lock.md), [security and editions](11-security-compliance-and-editions.md) | <span class="badge enterprise">Enterprise edition only</span> metadata protected refs and Enterprise-required admission |
| Dedupe/shared objects | [dedupe chapter](10-dedupe-and-shared-objects-enterprise.md) | <span class="badge enterprise">Enterprise edition only</span> `internal/dedupe`, shared object metadata, Community stubs |
| Gateway coordination | [etcd HA guide](../../etcd-ha-cluster-install-operations-guide.md) | `internal/coordination` |
| Metadata backup/restore | [admin guide](../../admin-guide.md) | `internal/meta` export/import surfaces |
| Edition boundary | [release boundary](14-release-and-edition-boundaries.md) | `internal/edition`, release checks |
| Operations guides | [MCP operations guide](../../mcp-operations-guide.md) | IAM, KMS, replication, notification, inventory, quota, maintenance, console |

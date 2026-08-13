Appendix <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only definitions</span>

# Glossary

## Appendix

- terms
- abbreviations
- edition markers

<div class="note" markdown="1">

**Edition scope.** This glossary includes Community edition terms and Enterprise edition only feature definitions. Enterprise definitions describe private-distribution behavior and do not make those features available in public Community builds.

</div>

| Term | Meaning |
| --- | --- |
| NAMROS | Network Attached Multipath Resilient Object Storage, pronounced [nae-muh-ross]. |
| S3 | Object storage API family used as the external NAMROS compatibility target. |
| Gateway | `namros-gateway`, the S3 HTTP process. It is stateless for authoritative object metadata. |
| Metadata backend | Repository that owns bucket, object version, multipart, lifecycle, operation, and evidence state. |
| Object head | Current S3-visible pointer for a bucket/key. It references the latest visible version or delete marker. |
| Object version | Committed immutable object metadata record containing size, ETag, headers, storage class, encryption, lock state, and segment refs. |
| Segment | Physical payload byte range referenced by object manifests. |
| SegmentRef | Self-describing reference that carries segment id, storage class snapshot, placement snapshot, size, digest, encryption envelope, and optional shared object id. |
| Storage class snapshot | The storage class chosen at write/initiation time and stored on the committed version so future reads use the original contract. |
| Placement snapshot | Backend layout captured on a segment ref, such as SBS physical chunk spans or Enterprise EC shard roles. |
| Manifest | Metadata structure that orders segment references for an object version. |
| Protected ref | Metadata guard that blocks physical segment deletion. |
| Shared object | <span class="badge enterprise">Enterprise edition only</span> byte-verified dedupe root that can be referenced by multiple object versions. |
| SBS | NAMRBD storage substrate reused by NAMROS replicated physical storage and <span class="badge enterprise">Enterprise edition only</span> EC storage paths. |
| TiKV | Distributed key-value store used as Community authoritative metadata backend for production deployments. |
| etcd | Coordination store used for Community gateway registry and health leases. |
| MCP | Model Context Protocol provider for AI-assisted operations. |
| Community | Public source edition with S3 compatibility and no Enterprise unlock path. |
| Enterprise | <span class="badge enterprise">Enterprise edition only</span> Private distribution containing SBS EC, WORM, dedupe, KMS, compliance, and advanced operations features. |

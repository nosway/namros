Appendix <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only notes</span>

# Revision History

## History

- initial set
- content expansion
- known gaps

<div class="note" markdown="1">

**Edition scope.** This appendix records public documentation changes and may mention Enterprise edition only deferred work. Enterprise release automation notes are private-distribution planning items, not public Community deliverables.

</div>

| Date | Change | Validation |
| --- | --- | --- |
| 2026-07-06 | Initial static HTML documentation set created. | `make html-docs-check` |
| 2026-07-06 | Top-level guides and architecture chapters expanded from skeleton summaries. | `make html-docs-check` |
| 2026-07-06 | Appendices and missing diagrams added. | `make html-docs-check` |
| 2026-07-07 | MinIO AIStor-inspired future operations guide shells added for IAM, KMS, replication/DR, performance/soak, events, inventory/batch, capacity/maintenance, quota/QoS, web console, and upgrade/release. | `make html-docs-check` |
| 2026-08-04 | Architecture manual expanded with metadata schema, object-to-segment mapping, SBS replicated/EC storage mapping, Object Lock protected refs, dedupe shared objects, KMS/evidence architecture, operations state model, and source/interface maps. | `make html-docs-check`, `make check-publication-readiness`, `make check-community-export` |

## Known Gaps

- Some command examples are intentionally product-level and should be synchronized with future CLI flag changes.
- Detailed TiKV key encoding, online migration policy, and large-manifest chunking remain future implementation-specific documentation topics.
- EC repair scheduling and KMS provider internals remain private Enterprise implementation details; this manual documents public architecture boundaries and expected behavior.
- <span class="badge enterprise">Enterprise edition only</span> overlay automation remains deferred release-operational hardening.

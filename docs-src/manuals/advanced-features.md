Product Direction <span class="badge enterprise">Enterprise development</span>

# NAMROS Advanced Features

<div class="warning" markdown="1">

The capabilities on this page are being developed and validated in NAMROS
Enterprise. They are not enabled in the current open-source Community
distribution. This page is a development-status overview, not a statement of
general availability, support entitlement, or delivery date.

</div>

## Status Vocabulary

| Status | Meaning |
| --- | --- |
| <span class="badge">Community included</span> | Implemented in the public source distribution and exercised by its documented checks. |
| <span class="badge">Experimental</span> | Runnable from public source with documented deployment, scale, or validation limits. |
| <span class="badge enterprise">Enterprise development</span> | An implementation foundation exists in the private Enterprise development line and is being integrated, hardened, or validated. |
| <span class="badge planned">Planned specification</span> | A contract or design target exists, but it must not be presented as available product behavior. |

## In Enterprise Development And Validation

### Erasure-Coded Storage Classes

<span class="badge enterprise">Enterprise development</span>

EC storage-class resolution, replicated/EC routing, multipart EC writes,
multi-stripe reads, checksum verification, and single-segment degraded reads
have implementation foundations. Performance, recovery, failure-domain, and
long-running cluster validation continue in the Enterprise development line.

[Read the EC architecture contract →](architecture-manual/chapters/08-sbs-ec-backend-enterprise.md)

### Object Lock And WORM Controls

<span class="badge enterprise">Enterprise development</span>

Retention and legal-hold metadata, governance/compliance delete admission,
governance bypass authorization, protected references, and structured audit
events have implementation foundations. End-to-end lifecycle, storage-delete,
operations, and release validation continue in Enterprise.

[Read the versioning, lifecycle, and Object Lock model →](architecture-manual/chapters/09-versioning-lifecycle-object-lock.md)

### Verified Deduplication

<span class="badge enterprise">Enterprise development</span>

The current development scope covers byte-verified, same-tenant/same-key,
post-process deduplication on replicated storage, shared-object reference
accounting, repair, scrub, and one-shot background operations. Long-running
scheduling, broader dedupe scopes, SBS-native optimization, and verified inline
dedupe remain follow-on work.

[Read the dedupe and shared-object model →](architecture-manual/chapters/10-dedupe-and-shared-objects-enterprise.md)

### KMS-Backed Payload Encryption

<span class="badge enterprise">Enterprise development</span>

Envelope metadata, per-segment data encryption keys, ciphertext storage,
bucket default encryption, multipart/copy handling, and fail-closed key-state
admission have implementation foundations. Streaming range decryption,
production external-provider integration, and broader operational validation
are still in progress.

[Read the KMS encryption guide →](kms-encryption-guide.md)

### Compliance Control And Evidence Support

<span class="badge enterprise">Enterprise development</span>

Evidence-package envelopes, Object Lock state summaries, audit-chain
verification, profile attachment, discovery manifests, and policy simulation
have partial implementation foundations. Principal/session evidence, trusted
time-source evidence, external export and SIEM delivery, and request-admission
integration remain in development. NAMROS does not claim certification or
legal compliance by itself.

[Read the security and compliance architecture →](architecture-manual/chapters/11-security-compliance-and-editions.md)

### External Identity Integration

<span class="badge enterprise">Enterprise development</span>

The principal/session model, mapping schema validation, policy simulation, and
temporary-credential envelope exist as foundations. Production OIDC, SAML,
LDAP/Active Directory provider validation, token exchange and issuance, session
lifecycle, and complete decision evidence remain in development.

[Read the IAM integration guide →](iam-integration-guide.md)

### Approved Operations

<span class="badge enterprise">Enterprise development</span>

Console, CLI, and MCP interfaces share plan/preflight/apply/verify/audit
envelopes and expose Community read-only diagnostics. Enterprise collector and
action bodies, explicit approval capture, external identity integration,
evidence export, and production hardening remain under development.

[Read the MCP operations guide →](mcp-operations-guide.md)

## Planned Enterprise Specifications

The following documents define design targets. They are not current Community
or generally available Enterprise behavior:

- <span class="badge planned">Planned specification</span>
  [Cross-region replication and disaster recovery](replication-disaster-recovery-guide.md)
- <span class="badge planned">Planned specification</span>
  [S3 event notifications and broker delivery](event-notification-guide.md)
- <span class="badge planned">Planned specification</span>
  [Scheduled inventory and approved batch operations](inventory-batch-operations-guide.md)
- <span class="badge planned">Planned specification</span>
  [Cluster-wide aggregate quota, QoS, and threshold alerting](quota-qos-guide.md)

Configuration, XML, JSON, CLI, and workflow examples in specification-stage
documents are proposed contracts unless an implementation-status table says
otherwise. Do not use them as deployment instructions for the current public
build.

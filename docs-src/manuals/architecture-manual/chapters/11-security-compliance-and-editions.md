Chapter 11 <span class="badge enterprise">Enterprise edition only</span>

# Security Compliance And Editions

## Scope

- WORM
- KMS posture
- evidence
- limitations

<div class="warning" markdown="1">

**Enterprise edition only.** This chapter describes Enterprise-only compliance, KMS, evidence, and governance contracts. Community edition behavior is included only to document required denial and edition-boundary expectations.

</div>

## Boundary

NAMROS can provide control-plane support for SEC, FINRA, CFTC, HIPAA style regulated workloads, but it does not claim legal certification by itself. Operators remain responsible for policy, deployment controls, external attestations, and regulatory interpretation.

## Security Architecture Layers

| Layer | Authority | Enterprise Contract |
| --- | --- | --- |
| identity and request auth | gateway plus metadata-backed access key/policy records | external IAM federation, session evidence, and advanced policy provenance where entitled |
| object governance | NAMROS metadata records on bucket/object version/protected refs | Object Lock retention, legal hold, governance bypass, compliance profile attachment |
| payload encryption | encryption envelope stored with object version or segment ref | SSE-KMS key state admission, DEK wrap/unwrap, fail-closed read/delete behavior |
| storage delete safety | NAMROS protected refs plus SBS protected-root/delete admission hook | payload GC and crypto erase cannot bypass retention, legal hold, or protected roots |
| evidence and audit | metadata audit events and evidence package generator | tamper-evident chain, time-source record, access/key/object-lock summaries, explicit limitations |

## Feature Matrix

| Feature | Community | Enterprise |
| --- | --- | --- |
| Object Lock/WORM | Enterprise-required denial | retention/legal hold/governance bypass control |
| SSE-KMS posture | Enterprise-required denial | key id/version/state evidence |
| SSE-KMS payload encryption | Enterprise-required denial | DEK envelope, ciphertext storage, read/range decrypt, fail-closed key admission |
| Compliance profile | Enterprise-required denial | profile plan/apply and audit event |
| Evidence package | Enterprise-required denial | package envelope, sections, limitations |
| Policy simulation | Enterprise-required denial | delete/lifecycle/GC/governance preview |

## KMS Envelope Model

SSE-KMS payload encryption is modeled as an envelope attached to the object version or segment ref. The envelope should be enough for a later gateway instance to unwrap and decrypt the object without relying on process-local state.

```text
EncryptionEnvelope
  Algorithm
  KeyID
  KeyVersion
  WrappedDEK
  Nonce
  PlaintextSizeBytes
  CiphertextSizeBytes
  Context
```

| KMS Key State | Read/Decrypt Admission | Delete/Crypto-erase Admission |
| --- | --- | --- |
| active | allowed when policy permits | allowed when Object Lock and protected refs permit |
| disabled | denied or fail-closed according to policy | denied unless explicit evidence workflow allows non-decrypting cleanup |
| pending deletion | denied for normal decrypt | requires evidence and protection checks |
| deleted or unknown | fail closed | fail closed for protected payload |

## Evidence Package

Evidence packages should include a schema version, package id, generated timestamp, scope, section summaries, audit chain verification, retention/legal hold state, access/key posture, time-source information, and explicit limitations.

| Evidence Section | Typical Inputs | Question Answered |
| --- | --- | --- |
| scope envelope | bucket, prefix, version id, tenant, generation time | what was examined? |
| object lock state | bucket Object Lock config, object retention, legal hold, protected refs | why can or cannot this payload be deleted? |
| key posture | KMS key records, encryption envelope fields, key state | which key protected the payload and was it usable? |
| audit chain | audit event ids, previous hash, event hash, request/principal details | can the administrative history be reviewed for tampering? |
| limitations | missing providers, unsupported claims, community denial boundaries | what must the operator not infer? |

## Time And Access Evidence

Time-source assurance records trusted time authority, drift status, and fail-closed policy when configured. Access evidence records bootstrap root access-key posture and should later incorporate principal/session/policy decisions.

## KMS Payload Path

M21 extends KMS from posture/evidence into the payload path: DEK generation, KMS wrap/unwrap, envelope metadata, key sealing/delete semantics, crypto erase evidence, and provider-specific fail-closed behavior. The operator-facing procedure is tracked in [the KMS encryption guide](../../kms-encryption-guide.md).

## No-certification Language

Documentation and product output must say that NAMROS provides evidence and controls; it must not state that NAMROS alone certifies compliance with SEC, FINRA, CFTC, HIPAA, or similar regimes.

## Community Boundary

Community builds can expose status and denial behavior that helps users understand the boundary, but they must not expose working KMS, Object Lock enforcement, compliance package generation, policy simulation, or Enterprise-only governance bypass paths. Returning success without enforcement would be worse than a clear denial because it would create false security expectations.

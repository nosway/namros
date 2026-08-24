Security <span class="badge enterprise">Enterprise development</span>

# NAMROS KMS Encryption Guide

<div class="warning" markdown="1">

**Enterprise development and validation.** SSE-KMS payload encryption and
key-state admission foundations exist in the Enterprise development line.
Streaming range optimization, production external-provider integration, and
broader operational validation remain in progress. Community builds retain the
explicit Enterprise-required boundary.

</div>

<div class="summary" markdown="1">

This guide explains the server-side encryption architecture and target
operational procedures. In Community, requests containing SSE-KMS headers and
KMS admin commands are rejected by the Enterprise edition boundary.

</div>

## Implementation Status

| Area | Current public Community behavior | Advanced feature status |
| --- | --- | --- |
| SSE-KMS request admission | Denied by the Enterprise-required boundary; no KMS unlock switch is exposed. | Gateway write/read, multipart/copy, bucket default encryption, and key-state admission foundations implemented in Enterprise development. |
| KMS provider and admin lifecycle | KMS commands return Enterprise-required responses. | Local AES-GCM provider and lifecycle foundations exist; production external-provider integration and operational hardening continue. |
| Encrypted range reads | Not active because Community does not process SSE-KMS payloads. | Correctness path exists by decrypting the full encrypted segment before slicing; bounded streaming optimization remains in development. |

## Encryption Scope

| Mode | NAMROS behavior | Edition |
| --- | --- | --- |
| SSE-S3 | User-transparent symmetric encryption using internal platform-managed master keys. | <span class="badge enterprise">Enterprise edition only</span> |
| SSE-KMS | Envelope Encryption using customer-managed master keys, linked to real-time key states and detailed audit trails. | <span class="badge enterprise">Enterprise edition only</span> |
| SSE-C | S3 operations using client-supplied encryption keys with inline payload stream processing. | Future Plan |

## Payload Data Path

| Path | Required behavior |
| --- | --- |
| PUT / MPU Initiate | Analyzes incoming S3 headers -> Calls KMS to generate a Data Encryption Key (DEK) -> Encrypts the DEK with the master key -> Persists the wrapped DEK in the object metadata manifest. |
| Segment write | Before pushing payload bytes to physical storage, the gateway encrypts the segments with the symmetric DEK (AES-256-GCM). Only ciphertext exists on persistent drives. |
| Complete MPU | Verifies individual segment encryption checksums and SHA-256 signatures during multipart combination before committing the final manifest to TiKV. |
| GET / Range | Unwraps the DEK via KMS -> Decrypts payload segments inline -> Streams plaintext to the client (instantly rejected if KMS is unreachable). |
| CopyObject | Allows zero-copy segment pointer cloning (CoW) if encryption domains match. If domains differ, segments must be decrypted and re-encrypted with a new DEK. |

## KMS Integration Configuration

Example configuration layout to register an external KMS provider (such as HashiCorp Vault) as the primary cryptomodule:

```json
{
  "kms_provider": "hashicorp-vault",
  "vault_endpoint": "https://vault.internal.local:8200",
  "auth_method": "approle",
  "role_id_env": "NAMROS_VAULT_ROLE_ID",
  "secret_id_env": "NAMROS_VAULT_SECRET_ID",
  "transit_engine_path": "transit/namros-keys",
  "key_spec": {
    "master_key_id": "prod-master-key-01",
    "rotation_interval_days": 90,
    "fallback_allowed": false
  }
}
```

## Fail-closed Behavior & S3 API Error Schema

If KMS communication fails or access keys are revoked, the NAMROS gateway operates in an ultra-strict **Fail-closed** state to prevent data leaks. The API responses returned to S3 clients are mapped as follows:

### 1. KMS Unreachable or Throttled (HTTP 503 Service Unavailable)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>KMSUnavailable</Code>
  <Message>The Server-side Encryption KMS provider is temporarily unavailable. Fail-closed enforced.</Message>
  <Resource>/demo/secret.txt</Resource>
  <RequestId>req-99ab-3321-cf</RequestId>
</Error>
```

### 2. Master Key Disabled or Purged (HTTP 403 Forbidden)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>AccessDenied</Code>
  <Message>The KMS key is disabled, revoked or deleted. Decryption block audit record has been produced.</Message>
  <Resource>/demo/secret.txt</Resource>
  <RequestId>req-88bc-2231-ab</RequestId>
</Error>
```

## Smoke Verification & CLI Usage

The first command is safe to use as a Community boundary check. The remaining workflow is an Enterprise/private-build smoke shape and is not expected to pass in public Community builds.

```sh
# Community boundary check: expect the Enterprise-required response
namros-admin kms-key-list
```

Enterprise/private-build workflow to cycle KMS keys and verify cryptographic enforcement using admin tools:

```sh
# Register and activate a new master key
namros-admin kms-key-put -key-id prod-master-key-01 -state active

# Perform a secure file upload specifying SSE-KMS headers
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object \
  --bucket demo \
  --key secret.txt \
  --body plaintext.txt \
  --server-side-encryption aws:kms \
  --ssekms-key-id prod-master-key-01

# Verify successful read and inline decryption in normal state
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object \
  --bucket demo \
  --key secret.txt \
  readback.txt
```

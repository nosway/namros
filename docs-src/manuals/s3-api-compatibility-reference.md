# S3 API Compatibility Reference

This page is the current, implementation-based NAMROS S3 compatibility
contract. It was verified against the gateway router, handlers, Community
edition gates, and tests on **2026-08-24**. NAMROS does **not** implement the
complete Amazon S3 API.

The older `docs/namros-s3-api-spec-scope.md` file is a target and roadmap
document. Its P0-P3 labels are priorities, not proof that an operation is
available. When that roadmap and this page differ, this page describes the
published current surface; executable code and tests remain the final source
of runtime behavior.

## Status Definitions

| Status | Meaning |
| --- | --- |
| **Community** | Routed and implemented in the public Community gateway, within the limitations stated here. |
| **Partial** | The route works, but an important part of AWS S3 semantics is not implemented. |
| **Compatibility only** | Accepted or returned to keep clients working. It is not authoritative authorization, billing, or encryption behavior. |
| **Enterprise-gated** | The route exists, but a Community build returns S3 `NotImplemented` (HTTP 501) with a NAMROS Enterprise Edition requirement message. This is not an Enterprise general-availability claim. |
| **Not implemented** | Not part of the current supported surface. Known unsupported control subresources are rejected instead of being dispatched as ordinary object or bucket CRUD. |

The machine-readable companion is the
[NAMROS OpenAPI document](../api/namros-s3.openapi.json). The
[Swagger view](s3-api-swagger.md) renders its operation catalog and physical
HTTP dispatchers.

## Common Protocol Contract

| Area | Current behavior |
| --- | --- |
| Addressing | Path style is supported. Virtual-hosted style is recognized for hosts shaped like `<bucket>.s3.<domain>`. Object keys are opaque and may contain slashes, repeated slashes, or a trailing slash. |
| Authentication | Every S3 request except CORS `OPTIONS` requires the configured access key and AWS SigV4. Authorization-header and presigned URL forms are supported. SigV2, anonymous/public access, STS session credentials, and general bucket-policy enforcement are not supported. |
| Payload signing | `UNSIGNED-PAYLOAD` and AWS-chunked framing are accepted. NAMROS does not currently compare the payload with `x-amz-content-sha256`, validate per-chunk signatures, validate checksum trailers, or validate `Content-MD5`. Do not treat these headers as an end-to-end integrity guarantee. |
| Responses | S3-shaped XML success/error bodies, `x-amz-request-id`, and `x-amz-id-2` are returned. Current XML bodies do not include the standard S3 XML namespace. |
| Consistency | Successful metadata publication is the visibility point for GET, HEAD, and LIST. Multipart parts remain invisible until complete. |
| CORS | Stored bucket CORS rules drive `OPTIONS` and normal response headers. `OPTIONS` is the only unsigned request path. |
| Encryption | Community `AES256` values are compatibility metadata and headers; Community payload bytes are not encrypted by that path. `aws:kms` is Enterprise-gated. Requests carrying destination or copy-source SSE-C headers are rejected with S3 `NotImplemented` (HTTP 501). |
| ACL and requester payer | `private` and `bucket-owner-full-control` canned ACL headers are accepted as no-ops. `x-amz-request-payer: requester` is validated then ignored because requester-pays billing is not implemented. |

## Service and Bucket Operations

| S3 operation | Request selector | Status | Supported subset or important difference |
| --- | --- | --- | --- |
| `ListBuckets` | `GET /` | Community | Lists buckets for the authenticated tenant. |
| `CreateBucket` | `PUT /{bucket}` | Partial | Creates in the configured region. The location body and complete AWS DNS bucket-name validation are not implemented. The Object Lock creation header is Enterprise-gated. |
| `HeadBucket` | `HEAD /{bucket}` | Community | Authenticated existence check and bucket region header. |
| `DeleteBucket` | `DELETE /{bucket}` | Community | Only an empty bucket can be deleted. |
| `GetBucketLocation` | `GET /{bucket}?location` | Community | Returns the configured gateway region. |
| `GetBucketVersioning` | `GET /{bucket}?versioning` | Partial | Reports empty, `Enabled`, or `Suspended`; AWS null-version semantics for `Suspended` are incomplete. |
| `PutBucketVersioning` | `PUT /{bucket}?versioning` | Partial | Accepts `Enabled` and `Suspended`; Object Lock restrictions apply when the Enterprise feature is enabled. |
| `GetBucketCORS` | `GET /{bucket}?cors` | Community | Returns stored rules or `NoSuchCORSConfiguration`. |
| `PutBucketCORS` | `PUT /{bucket}?cors` | Community | Up to 100 rules; GET, PUT, POST, DELETE, and HEAD allowed methods. |
| `DeleteBucketCORS` | `DELETE /{bucket}?cors` | Community | Deletes the stored configuration. |
| `GetBucketLifecycleConfiguration` | `GET /{bucket}?lifecycle` | Partial | Prefix filter, expiration, noncurrent-version expiration, and abort-incomplete-MPU subset. |
| `PutBucketLifecycleConfiguration` | `PUT /{bucket}?lifecycle` | Partial | No transitions, tag/AND filters, or complete AWS lifecycle grammar. Execution also requires the lifecycle worker to be enabled. |
| `DeleteBucketLifecycle` | `DELETE /{bucket}?lifecycle` | Community | Deletes the stored lifecycle configuration. |
| `GetBucketPolicy` | `GET /{bucket}?policy` | Partial | Policy document storage is implemented. |
| `PutBucketPolicy` | `PUT /{bucket}?policy` | Partial | Supports Allow/Deny plus Principal, Action, and Resource matching; `Condition` is unsupported. General S3 data-plane authorization does not consult this policy yet. |
| `DeleteBucketPolicy` | `DELETE /{bucket}?policy` | Partial | The stored policy is currently consulted only for Object Lock governance-bypass authorization. |
| `GetBucketEncryption` | `GET /{bucket}?encryption` | Partial | Returns stored AES256 or Enterprise KMS configuration. See the encryption warning above. |
| `PutBucketEncryption` | `PUT /{bucket}?encryption` | Partial | Exactly one AES256 rule is accepted as compatibility metadata; `aws:kms` is Enterprise-gated. |
| `DeleteBucketEncryption` | `DELETE /{bucket}?encryption` | Partial | Deletes the default encryption metadata. |
| `GetBucketObjectLockConfiguration` | `GET /{bucket}?object-lock` | Enterprise-gated | Community returns S3 `NotImplemented`. |
| `PutBucketObjectLockConfiguration` | `PUT /{bucket}?object-lock` | Enterprise-gated | Community returns S3 `NotImplemented`. |
| `GetBucketAcl` | `GET /{bucket}?acl` | Compatibility only | Fixed owner `FULL_CONTROL` response. |
| `PutBucketAcl` | `PUT /{bucket}?acl` | Compatibility only | No ACL state is stored; supported private canned ACLs are no-ops. ACL XML bodies and `x-amz-grant-*` headers are not parsed and are ignored. |
| `ListObjects` | `GET /{bucket}` | Community | V1 `prefix`, `delimiter`, `marker`, `max-keys`; result limit is capped at 1000. |
| `ListObjectsV2` | `GET /{bucket}?list-type=2` | Partial | `prefix`, `delimiter`, `continuation-token`, `max-keys`. `start-after`, `fetch-owner`, and `encoding-type` are not implemented. |
| `ListObjectVersions` | `GET /{bucket}?versions` | Community | `prefix`, `delimiter`, `key-marker`, `version-id-marker`, `max-keys`. |
| `DeleteObjects` | `POST /{bucket}?delete` | Partial | Up to 1000 keys, quiet mode, version IDs, and per-key errors. `Content-MD5` is not checked, and surrounding whitespace in XML keys is currently trimmed. |
| `ListMultipartUploads` | `GET /{bucket}?uploads` | Community | `prefix`, `delimiter`, `key-marker`, `upload-id-marker`, `max-uploads`. |

## Object Operations

| S3 operation | Request selector | Status | Supported subset or important difference |
| --- | --- | --- | --- |
| `PutObject` | `PUT /{bucket}/{key}` | Partial | Binary/zero-byte body, user metadata, tags, storage class, directory markers, and `If-None-Match: *`. Other conditions and checksum validation are absent. Object Lock headers and EC classes are Enterprise-gated. |
| `CopyObject` | `PUT /{bucket}/{key}` + `x-amz-copy-source` | Partial | Self-copy, metadata `REPLACE`, and tag `COPY`/`REPLACE`. Copy conditions and source `versionId` are not implemented; copy currently rewrites bytes. |
| `GetObject` | `GET /{bucket}/{key}[?versionId=...]` | Partial | Full object or one byte range. Conditional reads, response overrides, checksum mode, and multiple ranges are not supported. |
| `HeadObject` | `HEAD /{bucket}/{key}[?versionId=...]` | Partial | Content, ETag, user metadata, storage class, version, and enabled feature headers. Conditional/checksum headers are not implemented. |
| `DeleteObject` | `DELETE /{bucket}/{key}[?versionId=...]` | Community | Unversioned delete, versioned delete marker, and explicit version deletion. Enterprise Object Lock can deny deletion. |
| `GetObjectTagging` | `GET /{bucket}/{key}?tagging[&versionId=...]` | Community | Returns up to 10 stored tags for the selected version. |
| `PutObjectTagging` | `PUT /{bucket}/{key}?tagging[&versionId=...]` | Community | Replaces the selected version's tag set. |
| `DeleteObjectTagging` | `DELETE /{bucket}/{key}?tagging[&versionId=...]` | Community | Clears the selected version's tag set. |
| `GetObjectRetention` | `GET /{bucket}/{key}?retention[&versionId=...]` | Enterprise-gated | Community returns S3 `NotImplemented`. |
| `PutObjectRetention` | `PUT /{bucket}/{key}?retention[&versionId=...]` | Enterprise-gated | Community returns S3 `NotImplemented`. |
| `GetObjectLegalHold` | `GET /{bucket}/{key}?legal-hold[&versionId=...]` | Enterprise-gated | Community returns S3 `NotImplemented`. |
| `PutObjectLegalHold` | `PUT /{bucket}/{key}?legal-hold[&versionId=...]` | Enterprise-gated | Community returns S3 `NotImplemented`. |
| `GetObjectAcl` | `GET /{bucket}/{key}?acl[&versionId=...]` | Compatibility only | Fixed owner `FULL_CONTROL` response after object/version existence check. |
| `PutObjectAcl` | `PUT /{bucket}/{key}?acl[&versionId=...]` | Compatibility only | No ACL state is stored; supported private canned ACLs are no-ops. ACL XML bodies and `x-amz-grant-*` headers are not parsed and are ignored. |

## Multipart Operations

| S3 operation | Request selector | Status | Supported subset or important difference |
| --- | --- | --- | --- |
| `CreateMultipartUpload` | `POST /{bucket}/{key}?uploads` | Community | Captures content type, metadata, tags, storage class, and configured feature state. |
| `UploadPart` | `PUT ...?partNumber=N&uploadId=ID` | Partial | Part numbers 1-10000. `Content-MD5` and AWS checksums are not validated. |
| `UploadPartCopy` | Same query + `x-amz-copy-source` | Partial | One optional `x-amz-copy-source-range`; source version and copy conditions are not implemented. |
| `ListParts` | `GET ...?uploadId=ID` | Partial | Returns all stored parts. `max-parts` and `part-number-marker` pagination are not implemented. |
| `CompleteMultipartUpload` | `POST ...?uploadId=ID` | Partial | Ordered part numbers and supplied ETags are checked. AWS minimum non-final part size and checksums are not enforced. |
| `AbortMultipartUpload` | `DELETE ...?uploadId=ID` | Community | Aborts the upload and cleans up or queues its part segments. |

## Not Implemented

The following important families are not in the current support contract:

- bucket tagging, notification, replication, policy status, public-access
  controls, website, logging, request payment, inventory, and analytics;
- POST policy/browser upload, `GetObjectAttributes`, S3 Select, restore, Object
  Lambda, torrent, and object annotations;
- S3 Express directory-bucket `CreateSession`/`RenameObject` and S3 metadata
  table/configuration APIs, including inventory, journal, and annotation tables;
- SSE-C, SigV2, anonymous public buckets, STS, and complete IAM/bucket-policy
  data-plane authorization;
- the complete AWS conditional-request and checksum families.

Known unsupported control subresources and invalid subresource/method
combinations are explicitly rejected; they do not fall through to
`CreateBucket`, `DeleteBucket`, `PutObject`, `DeleteObject`, or object listing.

## s3fs-fuse Profile

The default s3fs-fuse profile is covered by the current route surface:

- path-style mount and V1/V2 directory listing;
- HEAD, single-range GET, PUT, zero-byte/trailing-slash directory markers, and
  `x-amz-meta-*` POSIX metadata;
- DELETE, rename by copy/delete, self-copy metadata replacement, multipart
  upload, multipart copy, incomplete-MPU list/abort, and private canned ACL
  compatibility.

A Linux manual run recorded these default-profile flows as passing on
2026-07-03. That record is not an automated CI regression and does not prove
optional profiles. `enable_content_md5` is accepted without digest validation;
Community SSE-S3 is metadata-only; requester-pays is ignored; SSE-C, SigV2,
anonymous mounting, and non-private ACLs are unsupported. See the
[S3 client compatibility guide](s3-client-compatibility-guide.md) for the
current test workflow.

## Verification and Swagger

Run the contract drift check with:

```sh
make s3-api-spec-check
```

The check verifies that the 48 public logical operations (47 router operations
plus the header-dispatched `UploadPartCopy`) cover every declared Go router and
gateway dispatch case exactly once, remain present once in the OpenAPI
dispatchers, and keep the same operation names and statuses in the
English/Korean tables. It does not prove every behavioral subset; code and
tests remain the runtime authority.

After installing `docs-src/requirements.txt`, `make s3-api-openapi-check`
also validates the complete document against the OpenAPI 3.1 schema. The
documentation CI runs both checks through `make docs-render-check`.

OpenAPI cannot define several logical operations with the same path and HTTP
method. The companion document therefore exposes three physical paths and uses
`x-namros-operation-matrix` and `x-namros-dispatches` for the logical S3
operations. Swagger is a view of this contract, not a replacement for it, and
its live submit buttons are disabled because the browser does not generate
SigV4 signatures or correctly model arbitrary slash-containing object keys.

Chapter 02 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# S3 API And Compatibility

## S3 Scope

- bucket/object
- multipart
- copy/versioning
- client smoke

<div class="note" markdown="1">

**Edition scope.** This chapter lists Community edition S3 compatibility targets and Enterprise edition only request surfaces. Rows or sections marked <span class="badge enterprise">Enterprise edition only</span> are not public Community functionality except for documented Enterprise-required denial responses.

</div>

## API Family Matrix

| Family | Community Target | Notes |
| --- | --- | --- |
| Bucket lifecycle | create/list/delete, versioning, CORS, lifecycle config | Lifecycle execution is worker-driven and protected-ref aware. |
| Object I/O | PUT, GET, HEAD, range, DELETE, LIST | Strong visibility follows metadata publish. |
| Multipart | initiate, upload part, complete, list, abort | Parts remain invisible until complete. |
| Copy and tagging | copy, self-copy metadata replace, tags | Required by external clients for metadata updates. |
| Object Lock/SSE-KMS | Enterprise-required error | Community denial is intentional. |

## Request Routing

The gateway parses S3 path, query subresource, method, headers, and SigV4 context. Routing must produce S3-compatible XML errors for unsupported or invalid requests. Compatibility decisions belong in the S3 gateway layer, while namespace mutation belongs in metadata repository transactions.

## API To Metadata Mapping

| S3 Operation | Metadata Reads | Metadata Writes | Storage Interaction |
| --- | --- | --- | --- |
| `CreateBucket` | bucket name index, tenant state | `Bucket`, default versioning/encryption/Object Lock config | none |
| `PutObject` | bucket config, storage class catalog, encryption/Object Lock defaults, current head for overwrite cleanup | `ObjectVersion`, `ObjectHead`, list index, protected refs, GC candidates for replaced refs | write payload to segment store before publish |
| `GetObject`/`HeadObject` | object head or explicit version, bucket policy, retention-visible headers | audit event where configured | stream range or full object from referenced segments |
| `ListObjectsV2` | ordered list index, current object heads, delimiter/prefix state | none except audit/metrics | none |
| `DeleteObject` | object head/version, versioning state, retention/legal hold, protected refs | delete marker or version removal, list index update, GC candidates, audit | physical delete is asynchronous and admission-gated |
| `CompleteMultipartUpload` | MPU record, part records, storage class snapshot, Object Lock state | committed `ObjectVersion`, `ObjectHead`, list index, MPU completed state | validate segment availability before publish |

## Consistency Contract

NAMROS targets strong read-after-write and list-after-write behavior for successful S3 mutations. A successful PUT, copy, multipart complete, delete marker creation, or explicit version delete must be reflected by subsequent GET, HEAD, LIST, and version listing requests that reach any healthy gateway. The single visibility point is the metadata transaction; storage writes that happen before that point are durable staging, not visible object state.

| Invariant | Rationale | Failure Response |
| --- | --- | --- |
| List index and object head change together. | Clients should not see an object in LIST that HEAD cannot resolve, or the reverse after a committed mutation. | Abort the metadata transaction; leave any written segments as orphan candidates. |
| Part records are invisible to normal object reads. | Multipart upload exposes pending state only through MPU APIs until complete. | Keep MPU active or abort; do not publish partial object heads. |
| Object Lock and retention are evaluated before delete visibility changes. | Delete marker creation and explicit version delete must not weaken protected versions. | Return a stable S3 error and leave metadata unchanged. |
| Enterprise-only request surfaces are explicit denials in Community builds. | Silent no-op behavior creates false compliance or durability expectations. | Return documented Enterprise-required behavior. |

## Addressing And Auth Assumptions

Path-style requests are the baseline for local compatibility. Virtual-hosted-style is a supported path when DNS or host mapping is configured. SigV4 verification uses configured credentials and region; compatibility scripts default to `us-east-1`.

## Client-specific Behavior

| Client | Sensitive Behavior | Example Path |
| --- | --- | --- |
| AWS CLI | metadata, copy, versioning, CORS, presigned GET, MPU | `aws s3api` |
| MinIO client | copy/cat/stat/list and server-side move | `mc` alias and object commands |
| rclone | size validation, move/delete, multipart copy | `rclone` remote commands |
| s3fs-fuse | directory markers, xattr-like behavior, rename, mount operations | Linux FUSE host procedure |

## Error Mapping

S3 clients expect XML error bodies and stable error codes. Internal metadata/store errors must be translated deliberately. For example, missing buckets map to `NoSuchBucket`, missing objects to `NoSuchKey`, unsupported Enterprise features to `InvalidRequest` or the standard NAMROS Enterprise requirement response, and unavailable storage to `ServiceUnavailable`.

## Compatibility No-op Versus Authority

Some S3 surfaces are accepted for client compatibility even when they do not become core authority. Canned ACL headers, requester-pays headers, or legacy client hints can be parsed and stored or ignored according to the compatibility guide. That is different from Object Lock, retention, encryption, lifecycle, storage class, and versioning, which affect object correctness and must be represented in metadata when enabled.

Reference: [S3 client compatibility guide](../../s3-client-compatibility-guide.md).

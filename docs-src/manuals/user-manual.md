User Workflow

# NAMROS User Manual

<div class="note" markdown="1">

**Edition scope.** This page focuses on Community edition S3 workflows and calls out Enterprise edition only request surfaces where users should expect edition-boundary denial in public Community builds.

</div>

## What NAMROS Provides

NAMROS provides S3-compatible bucket and object APIs. Under the Community edition, S3 clients can run bucket creation, object put/get/head/range/list/delete, multipart uploads, copy, object tagging, and versioning workflows.

The stateless gateway decouples object payloads from metadata. The metadata backend serves as the authoritative source for bucket, object, and version status, whereas the storage backend stores payload segment bytes.

## Endpoint, Credentials, Region

```sh
export AWS_ACCESS_KEY_ID=namros
export AWS_SECRET_ACCESS_KEY=namros-secret
export AWS_DEFAULT_REGION=us-east-1
export NAMROS_ENDPOINT=http://127.0.0.1:9000
```

Local smoke scripts use deterministic test credentials from the gateway fixture. Production credential handling must use the configured auth path for the deployment.

## Bucket And Object Workflow

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-bucket --bucket demo
printf 'hello namros\n' > /tmp/hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3 cp /tmp/hello.txt s3://demo/hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object --bucket demo --key hello.txt /tmp/readback.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api list-objects-v2 --bucket demo
```

Expected result: readback bytes match the original file and list output includes `hello.txt`.

## Object Metadata And Tags

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object \
  --bucket demo \
  --key meta.txt \
  --body /tmp/hello.txt \
  --metadata color=blue,owner=qa

aws --endpoint-url "$NAMROS_ENDPOINT" s3api head-object \
  --bucket demo \
  --key meta.txt

aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object-tagging \
  --bucket demo \
  --key meta.txt \
  --tagging 'TagSet=[{Key=project,Value=namros}]'
```

Metadata and tags are object metadata state, not payload bytes. They should survive HEAD/list/copy paths according to S3 compatibility rules implemented by the gateway.

## HEAD And Range GET

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api head-object --bucket demo --key hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object \
  --bucket demo \
  --key hello.txt \
  --range bytes=0-4 \
  /tmp/range.txt
```

Range reads must return the requested byte slice and preserve normal S3 response semantics. This path is important for filesystem-style clients and resumable readers.

## Multipart Upload

Multipart parts are not visible as committed objects until `CompleteMultipartUpload` succeeds. Complete validates part order, builds the final manifest, and publishes the object version in metadata.

```sh
upload_id=$(aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-multipart-upload \
  --bucket demo \
  --key large.bin \
  --query UploadId \
  --output text)

aws --endpoint-url "$NAMROS_ENDPOINT" s3api upload-part \
  --bucket demo \
  --key large.bin \
  --upload-id "$upload_id" \
  --part-number 1 \
  --body /tmp/part1.bin
```

For complete XML formatting and ETag capture, follow the [S3 client compatibility guide](s3-client-compatibility-guide.md).

## Copy, Self-copy, Versioning

CopyObject must support ordinary copy and self-copy metadata replacement because external clients use this path for metadata updates. Versioning buckets publish new versions and use delete markers for current deletes.

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api copy-object \
  --bucket demo \
  --key copied.txt \
  --copy-source demo/hello.txt

aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-bucket-versioning \
  --bucket demo \
  --versioning-configuration Status=Enabled
```

## Client Examples

| Client | Example | Notes |
| --- | --- | --- |
| AWS CLI | `aws --endpoint-url "$NAMROS_ENDPOINT" s3 ls` | Primary compatibility reference. |
| MinIO client | `mc alias set namros "$NAMROS_ENDPOINT" namros namros-secret` | Used for copy/cat/stat/list smoke. |
| rclone | `rclone lsd namros:` | Used for copy/list/read/move/delete smoke. |
| s3fs-fuse | Linux FUSE host procedure | Requires FUSE mount permissions. |

## Common Errors And Known Limits

| Error | Likely Meaning | Action |
| --- | --- | --- |
| `NoSuchBucket` | bucket does not exist in metadata backend | Create bucket or verify endpoint/keyspace. |
| `NoSuchKey` | object version is not visible | Check key spelling, versioning, complete status. |
| `ServiceUnavailable` | storage or metadata backend unavailable | Inspect gateway logs and backend health. |
| `InvalidRequest` | unsupported or Enterprise-only request | Check edition behavior and feature scope. |

Object keys are treated as object names, not filesystem directories. Directory marker objects created by filesystem clients should be preserved as ordinary objects.

## Community Vs Enterprise-visible Behavior

| Feature | Community Behavior | Enterprise Behavior |
| --- | --- | --- |
| Basic bucket/object API | Supported | Supported |
| Object Lock/WORM | Enterprise-required error | Enforcement and evidence |
| SSE-KMS | Enterprise-required error | KMS posture and key evidence |
| Dedupe | Not S3-visible; admin paths denied | worker/scheduler/scrub workflows |
| SBS EC storage class | Enterprise-required error | EC multipart write/read path |

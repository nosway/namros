# S3 API 호환성 레퍼런스

이 문서는 현재 구현을 기준으로 한 NAMROS S3 호환성 계약입니다. 게이트웨이
라우터·핸들러·Community 에디션 게이트·테스트를 기준으로 **2026-08-24**에
검증했습니다. NAMROS는 Amazon S3 API 전체를 구현한다고 표방하지 않습니다.

기존 `docs/namros-s3-api-spec-scope.md`는 목표와 로드맵 문서입니다. 그 문서의
P0-P3는 우선순위이며 구현 완료 증거가 아닙니다. 두 문서가 다르면 현재 공개
범위는 이 문서를 따르며, 실제 실행 동작의 최종 근거는 코드와 테스트입니다.

## 상태 정의

| 상태 | 의미 |
| --- | --- |
| **Community** | 공개 Community 게이트웨이에 라우팅과 구현이 있으며, 이 문서에 적힌 부분집합을 지원합니다. |
| **부분 지원** | 라우트는 동작하지만 중요한 AWS S3 의미론 일부가 아직 없습니다. |
| **호환성 전용** | 클라이언트 호환을 위해 수용하거나 반환합니다. 인가·과금·암호화의 정본 동작은 아닙니다. |
| **Enterprise 게이트** | 라우트는 있지만 Community 빌드는 NAMROS Enterprise Edition 필요 메시지와 S3 `NotImplemented`(HTTP 501)를 반환합니다. Enterprise 일반 공급을 의미하지 않습니다. |
| **미구현** | 현재 지원 계약에 없습니다. 알려진 미지원 control subresource는 일반 bucket/object CRUD로 실행하지 않고 거부합니다. |

기계 판독용 파일은 [NAMROS OpenAPI 문서](../../api/namros-s3.openapi.json)이며,
[Swagger 화면](../s3-api-swagger.md)은 operation catalog와 실제 HTTP dispatcher를
보여줍니다.

## 공통 프로토콜 계약

| 영역 | 현재 동작 |
| --- | --- |
| 주소 방식 | path style을 지원합니다. virtual-hosted style은 `<bucket>.s3.<domain>` 형태에서 인식합니다. key는 opaque하며 slash, 반복 slash, trailing slash를 보존합니다. |
| 인증 | CORS `OPTIONS`를 제외한 모든 S3 요청에 설정된 access key와 AWS SigV4가 필요합니다. Authorization header와 presigned URL을 지원합니다. SigV2, anonymous/public access, STS session credential, 일반 bucket-policy 인가는 미지원입니다. |
| payload 서명 | `UNSIGNED-PAYLOAD`와 AWS-chunked framing을 수용합니다. 현재 `x-amz-content-sha256`와 실제 body 비교, chunk별 서명, trailer checksum, `Content-MD5`는 검증하지 않습니다. 이 헤더를 종단 간 무결성 보장으로 사용하면 안 됩니다. |
| 응답 | S3 형태의 XML 성공/오류 body, `x-amz-request-id`, `x-amz-id-2`를 반환합니다. 현재 XML에는 표준 S3 XML namespace가 없습니다. |
| 일관성 | metadata publish가 GET/HEAD/LIST의 단일 visibility point입니다. MPU part는 complete 전까지 일반 object로 보이지 않습니다. |
| CORS | 저장된 bucket CORS 규칙을 `OPTIONS`와 일반 응답 header에 적용합니다. `OPTIONS`만 unsigned 요청입니다. |
| 암호화 | Community `AES256`은 호환성 metadata/header일 뿐 해당 경로에서 payload를 암호화하지 않습니다. `aws:kms`는 Enterprise 게이트입니다. destination 또는 copy-source SSE-C header가 있는 요청은 S3 `NotImplemented`(HTTP 501)로 거부합니다. |
| ACL/requester payer | `private`, `bucket-owner-full-control` canned ACL header는 no-op으로 수용합니다. `x-amz-request-payer: requester`는 값만 검증하고 requester-pays 과금이 없어 무시합니다. |

## Service와 Bucket API

| S3 operation | 요청 selector | 상태 | 지원 부분집합 또는 주요 차이 |
| --- | --- | --- | --- |
| `ListBuckets` | `GET /` | Community | 인증 tenant의 bucket을 나열합니다. |
| `CreateBucket` | `PUT /{bucket}` | 부분 지원 | 설정 region에 생성합니다. location body와 AWS DNS bucket-name 규칙 전체를 검증하지 않습니다. Object Lock 생성 header는 Enterprise 게이트입니다. |
| `HeadBucket` | `HEAD /{bucket}` | Community | 인증된 존재 확인과 bucket region header를 제공합니다. |
| `DeleteBucket` | `DELETE /{bucket}` | Community | 빈 bucket만 삭제합니다. |
| `GetBucketLocation` | `GET /{bucket}?location` | Community | 설정된 gateway region을 반환합니다. |
| `GetBucketVersioning` | `GET /{bucket}?versioning` | 부분 지원 | empty, `Enabled`, `Suspended`를 반환하지만 `Suspended`의 AWS null-version 의미론은 불완전합니다. |
| `PutBucketVersioning` | `PUT /{bucket}?versioning` | 부분 지원 | `Enabled`, `Suspended`를 수용하며 Enterprise Object Lock 활성 시 제한을 적용합니다. |
| `GetBucketCORS` | `GET /{bucket}?cors` | Community | 저장 규칙 또는 `NoSuchCORSConfiguration`을 반환합니다. |
| `PutBucketCORS` | `PUT /{bucket}?cors` | Community | 최대 100개 규칙, GET·PUT·POST·DELETE·HEAD method를 지원합니다. |
| `DeleteBucketCORS` | `DELETE /{bucket}?cors` | Community | 저장된 설정을 삭제합니다. |
| `GetBucketLifecycleConfiguration` | `GET /{bucket}?lifecycle` | 부분 지원 | prefix filter, expiration, noncurrent-version expiration, abort-incomplete-MPU 부분집합입니다. |
| `PutBucketLifecycleConfiguration` | `PUT /{bucket}?lifecycle` | 부분 지원 | transition, tag/AND filter, AWS grammar 전체는 미지원입니다. 실행에는 lifecycle worker도 활성화해야 합니다. |
| `DeleteBucketLifecycle` | `DELETE /{bucket}?lifecycle` | Community | 저장된 lifecycle 설정을 삭제합니다. |
| `GetBucketPolicy` | `GET /{bucket}?policy` | 부분 지원 | policy 문서 저장과 조회를 지원합니다. |
| `PutBucketPolicy` | `PUT /{bucket}?policy` | 부분 지원 | Allow/Deny, Principal, Action, Resource를 지원하고 `Condition`은 미지원입니다. 일반 S3 data-plane 인가에는 아직 연결되지 않습니다. |
| `DeleteBucketPolicy` | `DELETE /{bucket}?policy` | 부분 지원 | 현재 policy는 Object Lock governance bypass 인가에만 사용됩니다. |
| `GetBucketEncryption` | `GET /{bucket}?encryption` | 부분 지원 | 저장된 AES256 또는 Enterprise KMS 설정을 반환합니다. 위 암호화 경고를 확인하십시오. |
| `PutBucketEncryption` | `PUT /{bucket}?encryption` | 부분 지원 | AES256 단일 규칙은 호환 metadata로 수용하고 `aws:kms`는 Enterprise 게이트입니다. |
| `DeleteBucketEncryption` | `DELETE /{bucket}?encryption` | 부분 지원 | 기본 암호화 metadata를 삭제합니다. |
| `GetBucketObjectLockConfiguration` | `GET /{bucket}?object-lock` | Enterprise 게이트 | Community는 S3 `NotImplemented`를 반환합니다. |
| `PutBucketObjectLockConfiguration` | `PUT /{bucket}?object-lock` | Enterprise 게이트 | Community는 S3 `NotImplemented`를 반환합니다. |
| `GetBucketAcl` | `GET /{bucket}?acl` | 호환성 전용 | 고정 owner `FULL_CONTROL` 응답입니다. |
| `PutBucketAcl` | `PUT /{bucket}?acl` | 호환성 전용 | ACL state를 저장하지 않으며 지원 private canned ACL은 no-op입니다. ACL XML body와 `x-amz-grant-*` header는 파싱하지 않고 무시합니다. |
| `ListObjects` | `GET /{bucket}` | Community | V1 `prefix`, `delimiter`, `marker`, `max-keys`; 결과는 최대 1000개입니다. |
| `ListObjectsV2` | `GET /{bucket}?list-type=2` | 부분 지원 | `prefix`, `delimiter`, `continuation-token`, `max-keys`를 지원합니다. `start-after`, `fetch-owner`, `encoding-type`은 미지원입니다. |
| `ListObjectVersions` | `GET /{bucket}?versions` | Community | `prefix`, `delimiter`, `key-marker`, `version-id-marker`, `max-keys`를 지원합니다. |
| `DeleteObjects` | `POST /{bucket}?delete` | 부분 지원 | 최대 1000개 key, quiet mode, version ID, key별 오류를 지원합니다. `Content-MD5`를 검사하지 않고 XML key 앞뒤 공백을 현재 제거합니다. |
| `ListMultipartUploads` | `GET /{bucket}?uploads` | Community | `prefix`, `delimiter`, `key-marker`, `upload-id-marker`, `max-uploads`를 지원합니다. |

## Object API

| S3 operation | 요청 selector | 상태 | 지원 부분집합 또는 주요 차이 |
| --- | --- | --- | --- |
| `PutObject` | `PUT /{bucket}/{key}` | 부분 지원 | binary/zero-byte body, user metadata, tag, storage class, directory marker, `If-None-Match: *`를 지원합니다. 다른 condition/checksum은 없습니다. Object Lock header와 EC class는 Enterprise 게이트입니다. |
| `CopyObject` | `PUT /{bucket}/{key}` + `x-amz-copy-source` | 부분 지원 | self-copy, metadata `REPLACE`, tag `COPY`/`REPLACE`를 지원합니다. copy condition과 source `versionId`는 없으며 현재 bytes를 다시 씁니다. |
| `GetObject` | `GET /{bucket}/{key}[?versionId=...]` | 부분 지원 | 전체 object 또는 단일 byte range를 지원합니다. conditional read, response override, checksum mode, multi-range는 미지원입니다. |
| `HeadObject` | `HEAD /{bucket}/{key}[?versionId=...]` | 부분 지원 | content, ETag, user metadata, storage class, version, 활성 feature header를 반환합니다. condition/checksum header는 미지원입니다. |
| `DeleteObject` | `DELETE /{bucket}/{key}[?versionId=...]` | Community | unversioned delete, delete marker, 명시 version 삭제를 지원합니다. Enterprise Object Lock은 삭제를 거부할 수 있습니다. |
| `GetObjectTagging` | `GET /{bucket}/{key}?tagging[&versionId=...]` | Community | 선택 version의 tag를 최대 10개 반환합니다. |
| `PutObjectTagging` | `PUT /{bucket}/{key}?tagging[&versionId=...]` | Community | 선택 version의 tag set을 교체합니다. |
| `DeleteObjectTagging` | `DELETE /{bucket}/{key}?tagging[&versionId=...]` | Community | 선택 version의 tag set을 비웁니다. |
| `GetObjectRetention` | `GET /{bucket}/{key}?retention[&versionId=...]` | Enterprise 게이트 | Community는 S3 `NotImplemented`를 반환합니다. |
| `PutObjectRetention` | `PUT /{bucket}/{key}?retention[&versionId=...]` | Enterprise 게이트 | Community는 S3 `NotImplemented`를 반환합니다. |
| `GetObjectLegalHold` | `GET /{bucket}/{key}?legal-hold[&versionId=...]` | Enterprise 게이트 | Community는 S3 `NotImplemented`를 반환합니다. |
| `PutObjectLegalHold` | `PUT /{bucket}/{key}?legal-hold[&versionId=...]` | Enterprise 게이트 | Community는 S3 `NotImplemented`를 반환합니다. |
| `GetObjectAcl` | `GET /{bucket}/{key}?acl[&versionId=...]` | 호환성 전용 | object/version 존재 확인 뒤 고정 owner `FULL_CONTROL`을 반환합니다. |
| `PutObjectAcl` | `PUT /{bucket}/{key}?acl[&versionId=...]` | 호환성 전용 | ACL state를 저장하지 않으며 지원 private canned ACL은 no-op입니다. ACL XML body와 `x-amz-grant-*` header는 파싱하지 않고 무시합니다. |

## Multipart API

| S3 operation | 요청 selector | 상태 | 지원 부분집합 또는 주요 차이 |
| --- | --- | --- | --- |
| `CreateMultipartUpload` | `POST /{bucket}/{key}?uploads` | Community | content type, metadata, tag, storage class와 설정 feature state를 고정합니다. |
| `UploadPart` | `PUT ...?partNumber=N&uploadId=ID` | 부분 지원 | part 1-10000을 지원하며 `Content-MD5`와 AWS checksum은 검증하지 않습니다. |
| `UploadPartCopy` | 같은 query + `x-amz-copy-source` | 부분 지원 | `x-amz-copy-source-range` 하나를 지원하며 source version/copy condition은 미지원입니다. |
| `ListParts` | `GET ...?uploadId=ID` | 부분 지원 | 저장된 part를 모두 반환합니다. `max-parts`, `part-number-marker` pagination은 미지원입니다. |
| `CompleteMultipartUpload` | `POST ...?uploadId=ID` | 부분 지원 | 정렬된 part number와 제공 ETag를 검사합니다. AWS 비최종 part 최소 크기와 checksum은 강제하지 않습니다. |
| `AbortMultipartUpload` | `DELETE ...?uploadId=ID` | Community | upload를 중단하고 part segment를 정리하거나 cleanup queue에 넣습니다. |

## 미구현 범위

다음 주요 family는 현재 지원 계약에 없습니다.

- bucket tagging, notification, replication, policy status, public-access
  control, website, logging, request payment, inventory, analytics
- POST policy/browser upload, `GetObjectAttributes`, S3 Select, restore, Object
  Lambda, torrent, object annotation
- S3 Express directory bucket의 `CreateSession`/`RenameObject`, S3 metadata
  table/configuration API(inventory, journal, annotation table 포함)
- SSE-C, SigV2, anonymous public bucket, STS, 완전한 IAM/bucket-policy
  data-plane 인가
- AWS conditional request와 checksum family 전체

알려진 미지원 control subresource와 잘못 조합된 subresource/method는 명시적으로
거부하며 `CreateBucket`, `DeleteBucket`, `PutObject`, `DeleteObject`, object
listing으로 fall-through하지 않습니다.

## s3fs-fuse 프로필

현재 route surface는 s3fs-fuse 기본 프로필에 필요한 다음 흐름을 포함합니다.

- path-style mount와 V1/V2 directory list
- HEAD, 단일 Range GET, PUT, zero-byte/trailing-slash directory marker,
  `x-amz-meta-*` POSIX metadata
- DELETE, copy/delete rename, self-copy metadata 교체, MPU, multipart copy,
  incomplete-MPU list/abort, private canned ACL 호환

2026-07-03 Linux 수동 실행 기록에는 이 기본 프로필이 통과한 것으로 남아
있습니다. 자동 CI 회귀 증거는 아니며 optional profile까지 입증하지 않습니다.
`enable_content_md5`는 digest 검증 없이 수용하고, Community SSE-S3는
metadata-only이며 requester-pays는 무시합니다. SSE-C, SigV2, anonymous mount,
non-private ACL은 미지원입니다. 현재 검증 흐름은
[S3 클라이언트 호환성 가이드](s3-client-compatibility-guide.md)를 참고하십시오.

## 검증과 Swagger

명세 drift 검사는 다음과 같이 실행합니다.

```sh
make s3-api-spec-check
```

이 검사는 공개 logical operation 48개(라우터 operation 47개와 header로 분기되는
`UploadPartCopy`)가 선언된 Go router와 gateway dispatch case를 각각 정확히 한 번
포함하는지, OpenAPI dispatcher에 각각 한 번 존재하는지, 영문/한글 표의 operation
이름과 상태가 같은지 확인합니다. 각 API의 세부 의미론 전체를 증명하지는 않으며
실행 동작의 정본은 코드와 테스트입니다.

`docs-src/requirements.txt`를 설치한 뒤 `make s3-api-openapi-check`를 실행하면
문서 전체를 OpenAPI 3.1 schema로도 검증합니다. 문서 CI는
`make docs-render-check`를 통해 두 검사를 모두 실행합니다.

OpenAPI는 같은 path와 HTTP method에 logical operation 여러 개를 정의할 수
없습니다. 따라서 보조 문서는 실제 path 3개를 제공하고
`x-namros-operation-matrix`, `x-namros-dispatches` 확장에 logical S3 operation을
기록합니다. Swagger는 이 계약을 보는 보조 화면이며 이를 대체하지 않습니다.
브라우저가 SigV4를 만들지 못하고 slash를 포함한 임의 key도 정확히 모델링하지
못하므로 live submit button은 비활성화했습니다.

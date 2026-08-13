사용자 워크플로

# NAMROS 사용자 매뉴얼

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition S3 workflow를 중심으로 설명하고, 공개 Community 빌드에서 edition-boundary denial을 기대해야 하는 Enterprise edition only 요청 표면을 별도로 표시합니다.

</div>

## NAMROS 제공 기능

NAMROS는 S3 호환 버킷/오브젝트 API를 제공합니다. Community 기준 사용자는 일반 S3 클라이언트로 버킷 생성, 오브젝트 put/get/head/range/list/delete, 멀티파트 업로드, 복사, 태깅, 버전 관리 흐름을 사용할 수 있습니다.

게이트웨이는 오브젝트 페이로드와 메타데이터를 분리해서 다룹니다. 메타데이터 백엔드는 버킷/오브젝트/버전 상태의 정본 저장소이고, 스토리지 백엔드는 페이로드 세그먼트 바이트를 보관합니다.

## 엔드포인트, 자격 증명, 리전

```sh
export AWS_ACCESS_KEY_ID=namros
export AWS_SECRET_ACCESS_KEY=namros-secret
export AWS_DEFAULT_REGION=us-east-1
export NAMROS_ENDPOINT=http://127.0.0.1:9000
```

로컬 스모크 스크립트는 게이트웨이 fixture의 결정적 테스트 자격 증명을 사용합니다. 프로덕션 자격 증명 처리는 배포에 설정된 인증 경로를 사용해야 합니다.

## 버킷 및 오브젝트 워크플로

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-bucket --bucket demo
printf 'hello namros\n' > /tmp/hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3 cp /tmp/hello.txt s3://demo/hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object --bucket demo --key hello.txt /tmp/readback.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api list-objects-v2 --bucket demo
```

기대 결과는 다시 읽은 바이트가 원본 파일과 일치하고 목록 출력에 `hello.txt`가 포함되는 것입니다.

## 오브젝트 메타데이터와 태그

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

메타데이터와 태그는 페이로드 바이트가 아니라 오브젝트 메타데이터 상태입니다. 게이트웨이가 구현한 S3 호환 규칙에 따라 HEAD/목록/복사 경로에서도 유지되어야 합니다.

## HEAD와 범위 GET

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api head-object --bucket demo --key hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object \
  --bucket demo \
  --key hello.txt \
  --range bytes=0-4 \
  /tmp/range.txt
```

범위 읽기는 요청한 바이트 조각을 반환하고 일반 S3 응답 의미를 보존해야 합니다. 이 경로는 파일시스템형 클라이언트와 재개 가능한 리더에 중요합니다.

## 멀티파트 업로드

멀티파트 파트는 `CompleteMultipartUpload`가 성공하기 전까지 커밋된 오브젝트로 보이지 않습니다. 완료 단계는 파트 순서를 검증하고 최종 매니페스트를 만든 뒤 오브젝트 버전을 메타데이터에 공개합니다.

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

완전한 XML 형식과 ETag 캡처가 필요하면 [S3 클라이언트 호환성 가이드](s3-client-compatibility-guide.md)를 따르세요.

## 복사, 자기 복사, 버전 관리

외부 클라이언트가 메타데이터 갱신에 이 경로를 사용하므로 CopyObject는 일반 복사와 자기 복사 메타데이터 교체를 지원해야 합니다. 버전 관리 버킷은 새 버전을 공개하고 현재 버전 삭제에는 삭제 마커를 사용합니다.

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api copy-object \
  --bucket demo \
  --key copied.txt \
  --copy-source demo/hello.txt

aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-bucket-versioning \
  --bucket demo \
  --versioning-configuration Status=Enabled
```

## 클라이언트 예시

| 클라이언트 | 예시 | 비고 |
| --- | --- | --- |
| AWS CLI | `aws --endpoint-url "$NAMROS_ENDPOINT" s3 ls` | 주요 호환성 기준. |
| MinIO client | `mc alias set namros "$NAMROS_ENDPOINT" namros namros-secret` | 복사/cat/stat/목록 스모크에 사용. |
| rclone | `rclone lsd namros:` | 복사/목록/읽기/이동/삭제 스모크에 사용. |
| s3fs-fuse | Linux FUSE 호스트 절차 | FUSE 마운트 권한 필요. |

## 일반 오류와 알려진 제한

| 오류 | 가능성 높은 의미 | 조치 |
| --- | --- | --- |
| `NoSuchBucket` | 메타데이터 백엔드에 버킷이 없음 | 버킷을 만들거나 엔드포인트/키스페이스를 확인합니다. |
| `NoSuchKey` | 오브젝트 버전이 보이지 않음 | 키 철자, 버전 관리, 완료 상태를 확인합니다. |
| `ServiceUnavailable` | 스토리지 또는 메타데이터 백엔드 사용 불가 | 게이트웨이 로그와 백엔드 상태를 점검합니다. |
| `InvalidRequest` | 지원하지 않거나 Enterprise 전용 요청 | 에디션 동작과 기능 범위를 확인합니다. |

오브젝트 키는 파일시스템 디렉터리가 아니라 오브젝트 이름으로 취급됩니다. 파일시스템 클라이언트가 만든 디렉터리 마커 오브젝트는 일반 오브젝트로 보존되어야 합니다.

## Community와 Enterprise에서 보이는 동작

| 기능 | Community 동작 | Enterprise 동작 |
| --- | --- | --- |
| 기본 버킷/오브젝트 API | 지원 | 지원 |
| Object Lock/WORM | Enterprise 필요 오류 | 강제 적용과 증빙 |
| SSE-KMS | Enterprise 필요 오류 | KMS 상태와 키 증빙 |
| 중복 제거 | S3에서는 보이지 않으며 관리자 경로는 거부 | 워커/스케줄러/스크럽 워크플로 |
| SBS EC 스토리지 클래스 | Enterprise 필요 오류 | EC 멀티파트 쓰기/읽기 경로 |

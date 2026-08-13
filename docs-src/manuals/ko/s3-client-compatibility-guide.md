호환성

# S3 클라이언트 호환성 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition client smoke 범위와 Enterprise edition only client command family를 함께 다룹니다. <span class="badge enterprise">Enterprise edition only</span>로 표시된 행은 공개 Community 제공 기능이 아니라 private distribution 계약입니다.

</div>

## 범위와 비목표

이 클라이언트 점검은 `make run-dev`로 시작한 로컬 게이트웨이 또는 컨테이너 배포 가이드의 컨테이너 스택을 대상으로 실행합니다. s3fs-fuse는 FUSE 마운트 권한이 필요하므로 Linux 호스트에서 별도로 검증합니다.

| 클라이언트 | 검증 범위 | 예시 |
| --- | --- | --- |
| AWS CLI | put/get/head/range/list, 메타데이터, 복사, 버전 관리, CORS, 사전 서명, MPU 목록/중단 | `aws s3api list-buckets` |
| MinIO client | copy/cat/stat/list, 서버 측 이동, 멀티파트 크기 복사 | `mc ls namros` |
| rclone | 복사/목록/읽기, 이동/삭제, 멀티파트 크기 복사 | `rclone lsd namros:` |
| s3fs-fuse | 마운트, 파일 읽기/쓰기/목록/이름 변경, xattr 민감 흐름 | Linux FUSE 호스트 절차 |

## 공통 환경

```sh
export NAMROS_ENDPOINT=http://127.0.0.1:9000
export AWS_ACCESS_KEY_ID=namros
export AWS_SECRET_ACCESS_KEY=namros-secret
export AWS_DEFAULT_REGION=us-east-1
```

특정 virtual-hosted-style 테스트를 수행하는 경우가 아니라면 path-style 주소 방식을 사용합니다. virtual-hosted-style에서는 DNS 또는 `/etc/hosts`가 버킷 호스트 이름을 게이트웨이에 매핑해야 합니다.

## AWS CLI 스모크

```sh
aws --endpoint-url "$NAMROS_ENDPOINT" s3api list-buckets
```

기대 결과는 구성한 엔드포인트에서 정상 S3 list-buckets 응답을 반환하는 것입니다.

## MinIO 클라이언트 스모크

```sh
mc alias set namros "$NAMROS_ENDPOINT" "$AWS_ACCESS_KEY_ID" "$AWS_SECRET_ACCESS_KEY"
mc ls namros
```

흔한 MinIO client 실패 원인은 버킷 alias 불일치, path-style 설정 불일치, 서버 측 이동 동작이 S3 복사/삭제 의미와 달라지는 경우입니다.

## rclone 스모크

```sh
rclone lsd namros:
```

rclone은 업로드 뒤 오브젝트 크기와 내용이 올바른지 검증합니다. `corrupted on transfer: sizes differ` 실패는 보통 PUT 경로, HEAD 경로, 응답 길이 처리 중 하나가 일관되지 않다는 뜻입니다.

## Linux의 s3fs-fuse

s3fs-fuse에는 FUSE 권한이 있는 Linux 호스트가 필요합니다. 필수 패키지에는 s3fs-fuse와 `listfattr`를 제공하는 패키지 같은 xattr 도구가 포함됩니다.

## MinIO 클라이언트 확장 호환성 매트릭스

| mc 명령 계열 | NAMROS 상태 | 관련 가이드 |
| --- | --- | --- |
| `mc retention`, `mc legalhold` | <span class="badge enterprise">Enterprise edition only</span> Object Lock/WORM 동작. | [Object Lock 장](../architecture-manual/chapters/09-versioning-lifecycle-object-lock.md) |
| `mc encrypt` | <span class="badge enterprise">Enterprise edition only</span> M21 이후 SSE-KMS 페이로드 암호화. | [KMS 가이드](kms-encryption-guide.md) |
| `mc ilm` | 라이프사이클 설정/계획기/워커 범위. | [라이프사이클 장](../architecture-manual/chapters/09-versioning-lifecycle-object-lock.md) |
| `mc event` | <span class="badge enterprise">Enterprise edition only</span> 실시간 Webhook, Kafka 및 NATS 이벤트 알림 연동 지원. | [이벤트 가이드](event-notification-guide.md) |
| `mc replicate` | <span class="badge enterprise">Enterprise edition only</span> 멀티 리전 간의 실시간 비동기 버킷/사이트 복제 구성 지원. | [복제 가이드](replication-disaster-recovery-guide.md) |
| `mc quota` | <span class="badge enterprise">Enterprise edition only</span> 테넌트 및 버킷 기반 스토리지 Quota 및 QoS 대역폭 제어 지원. | [쿼터 가이드](quota-qos-guide.md) |
| `mc inventory` | <span class="badge enterprise">Enterprise edition only</span> 대용량 버킷 인벤토리 및 배치 작업 프레임워크 지원. | [인벤토리 가이드](inventory-batch-operations-guide.md) |

## 알려진 실패 신호

| 신호 | 가능성 높은 원인 | 다음 단계 |
| --- | --- | --- |
| AWS CLI 메타데이터 검증 실패 | HEAD 응답에 사용자 메타데이터 누락 | `head-object` JSON과 게이트웨이 메타데이터 매핑을 점검합니다. |
| MinIO 오브젝트 해시 불일치 | 복사/읽기 경로가 바이트를 변경 | 소스 파일, GET 출력, 오브젝트 ETag를 비교합니다. |
| rclone 크기 불일치 | PUT 응답 또는 HEAD 크기가 잘못됨 | Content-Length와 저장된 세그먼트 크기를 확인합니다. |
| CORS 사전 요청 상태 이상 | OPTIONS 라우팅 또는 CORS 설정 동작 불일치 | 버킷 CORS 상태와 OPTIONS 핸들러를 검증합니다. |
| s3fs 마운트 불가 | FUSE 권한, 패키지 누락, 엔드포인트 스타일 불일치 | Linux 패키지 구성과 마운트 명령 옵션을 확인합니다. |

## 결과 기록 템플릿

```text
date:
namros commit:
gateway command:
endpoint:
client versions:
smoke target:
result:
bucket:
tmpdir/log path:
notes:
```

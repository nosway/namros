보안 <span class="badge enterprise">Enterprise edition only</span>

# NAMROS KMS 암호화 가이드

<div class="warning" markdown="1">

**Enterprise edition only.** 이 페이지는 Enterprise 전용 SSE-KMS, SSE-S3, key lifecycle, fail-closed encryption 계약을 설명합니다. Community edition 동작은 거부 응답과 에디션 경계를 설명하기 위해서만 포함합니다.

</div>

<div class="summary" markdown="1">

이 가이드는 NAMROS의 서버 측 암호화(SSE-KMS, SSE-S3) 통합 구조와 장애 대응 방식을 설명합니다. Community 에디션에서는 SSE-KMS 헤더가 포함된 S3 요청과 KMS 관리자 명령을 Enterprise 에디션 경계에서 거부합니다.

</div>

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| SSE-KMS 요청 admission | Enterprise-required 경계에서 거부되며 KMS unlock switch를 노출하지 않습니다. | 키 상태 admission과 audit evidence를 포함하는 Enterprise payload encryption 계약입니다. |
| KMS 관리자 CLI | `kms-key-put`과 `kms-key-list`는 예약된 flat command 이름이며 Community 빌드에서는 Enterprise-required 응답을 반환합니다. | private Enterprise overlay가 key lifecycle 구현을 소유합니다. |
| Fail-closed payload 동작 | 공개 Community는 SSE-KMS payload를 처리하지 않으므로 활성화되지 않습니다. | KMS key 또는 provider 사용 불가 시 요구되는 Enterprise 동작입니다. |

## 암호화 범위

| 모드 | NAMROS 동작 | 에디션 |
| --- | --- | --- |
| SSE-S3 | 사용자에게 투명한 대칭 암호화. 내부 관리형 마스터 키 사용. | <span class="badge enterprise">Enterprise edition only</span> |
| SSE-KMS | 고객 정의 마스터 키 기반 envelope encryption. 실시간 키 상태와 접근 감사 추적 연계. | <span class="badge enterprise">Enterprise edition only</span> |
| SSE-C | 클라이언트 제공 키 검증 및 세그먼트 인라인 복호화 스트리밍 지원. | 향후 계획 |

## 페이로드 데이터 경로

| 경로 | 필수 동작 |
| --- | --- |
| PUT / MPU Initiate | S3 요청 헤더 분석 -> KMS 연동 모듈 호출로 데이터 암호화 키(DEK) 생성 -> 마스터 키로 DEK 암호화 -> 암호화된 DEK를 오브젝트 매니페스트에 저장. |
| Segment write | 게이트웨이는 원본 데이터를 물리 스토리지에 전달하기 전에 생성된 대칭 DEK(AES-256-GCM)로 페이로드를 암호화하여 SBS 또는 로컬 세그먼트 파일에 기록합니다. 스토리지에는 암호문만 남습니다. |
| Complete MPU | 멀티파트 결합 단계에서 개별 세그먼트 암호화 체크섬 및 SHA-256 서명 데이터 검증 후 메타데이터를 TiKV에 최종 반영. |
| GET / Range | KMS 호출로 DEK unwrap -> 복호화된 DEK로 페이로드 실시간 복호화 -> 클라이언트에 평문 스트리밍. KMS 장애 시 요청을 즉시 거부합니다. |
| CopyObject | 동일 암호화 도메인에서는 물리 세그먼트 주소 복사(CoW)를 허용하고, 도메인이 다르면 반드시 복호화 후 새 DEK로 재암호화해 신규 세그먼트를 생성합니다. |

## KMS 연동 설정

외부 KMS 솔루션(예: HashiCorp Vault)을 주 암호화 모듈로 등록하는 표준 구성 스키마 예시입니다.

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

## Fail-closed 동작 및 S3 API 오류 스키마

KMS 통신 장애가 발생하거나 권한이 취소되면 NAMROS 게이트웨이는 보안 유지를 위해 엄격한 **fail-closed(장애 시 즉시 차단)** 모드로 동작합니다. 이때 S3 API 클라이언트로 전달되는 오류 응답 형식은 다음과 같습니다.

### 1. KMS 장애 또는 통신 제한 시 (HTTP 503 Service Unavailable)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>KMSUnavailable</Code>
  <Message>The Server-side Encryption KMS provider is temporarily unavailable. Fail-closed enforced.</Message>
  <Resource>/demo/secret.txt</Resource>
  <RequestId>req-99ab-3321-cf</RequestId>
</Error>
```

### 2. 마스터 키가 비활성화 또는 영구 삭제되었을 때 (HTTP 403 Forbidden)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>AccessDenied</Code>
  <Message>The KMS key is disabled, revoked or deleted. Decryption block audit record has been produced.</Message>
  <Resource>/demo/secret.txt</Resource>
  <RequestId>req-88bc-2231-ab</RequestId>
</Error>
```

## 스모크 검증 및 CLI 사용법

첫 번째 명령은 Community 경계 확인용으로 사용할 수 있습니다. 나머지 workflow는 Enterprise/private build에서 수행하는 smoke 형태이며 공개 Community 빌드에서 통과할 것으로 기대하지 않습니다.

```sh
# Community 경계 확인: Enterprise-required 응답을 기대
namros-admin kms-key-list
```

Enterprise/private build에서 KMS 마스터 키 lifecycle을 변경하고 암호화 동작을 검증하는 CLI 사용 예시입니다.

```sh
# 마스터 키 등록 및 활성화
namros-admin kms-key-put -key-id prod-master-key-01 -state active

# SSE-KMS 옵션을 지정하여 보안 파일 업로드 수행
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object \
  --bucket demo \
  --key secret.txt \
  --body plaintext.txt \
  --server-side-encryption aws:kms \
  --ssekms-key-id prod-master-key-01

# 정상 상태에서의 조회 복호화 확인
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object \
  --bucket demo \
  --key secret.txt \
  readback.txt
```

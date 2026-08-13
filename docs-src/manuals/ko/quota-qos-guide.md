테넌트 제어 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS Quota/QoS 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition 버킷 최대 오브젝트 크기 quota와 Enterprise edition only capacity, tenant quota, QoS, alerting 섹션을 함께 다룹니다. Enterprise-only quota/QoS 계약을 공개 Community 동작으로 읽지 마세요.

</div>

<div class="summary" markdown="1">

이 문서는 현재 Community quota metadata 표면과 admission enforcement 및 QoS Rate Limiting에 대한 사양을 정의합니다. Community는 현재 `namros-admin bucket-quota-*` 명령을 통해 버킷 최대 오브젝트 크기 quota를 제공하고, `namros-admin tenant-quota-*` 명령으로 tenant quota record를 제공하며, metadata tenant usage reconciliation foundation을 포함합니다. aggregate write admission enforcement, bandwidth shaping, alerting은 후속/spec 영역입니다.

</div>

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| 버킷 최대 오브젝트 크기 quota | `namros-admin bucket-quota-put`, `bucket-quota-get`, `bucket-quota-delete`로 지원합니다. | 기본 quota primitive로 사용할 수 있습니다. |
| 버킷 용량/object-count quota | 공개 Community 빌드에는 aggregate usage limiter로 구현되어 있지 않습니다. | 사용량 counter, admission, alert를 위한 Enterprise 계약입니다. |
| Tenant quota records | `namros-admin tenant-quota-put`, `tenant-quota-get`, `tenant-quota-delete`를 통해 metadata record로 지원합니다. `max_active_uploads`는 CreateMultipartUpload에서 적용하며, bytes/object admission은 후속 단계입니다. | tenant isolation, bandwidth/TPS shaping, telemetry를 위한 Enterprise 계약입니다. |
| Tenant usage records | tenant bytes, committed object-version count, active MPU count, reconciliation timestamp/id를 기록하는 reconciliation foundation으로 지원합니다. | Admission enforcement는 후속 단계입니다. |
| Gateway request concurrency | gateway-local global, per-tenant, read-class, write-class request limit로 지원합니다. 포화 시 S3 `SlowDown`을 반환합니다. | Cluster-wide policy와 bandwidth shaping은 후속 단계입니다. |
| Gateway bandwidth hooks | 기본 비활성화 상태의 gateway-local upload/download bytes-per-second shaping hook으로 지원하며 PUT/GET stream contents를 보존합니다. | Cluster-wide tenant token-bucket policy는 후속 단계입니다. |

## 제어 모델 분류

Quota 및 QoS 모델은 Community primitive와 Enterprise 멀티테넌트 제어를 분리합니다.

| 제어 범위 | 작동 메커니즘 | 수행 제약 |
| --- | --- | --- |
| 버킷 최대 오브젝트 크기 quota | configured per-bucket maximum을 초과하는 개별 write를 거부합니다. | <span class="badge">Community</span> |
| 버킷 용량/object-count quota | 개별 S3 버킷 단위로 총 스토리지 사용량 또는 누적 object count를 제한합니다. | <span class="badge enterprise">Enterprise edition only</span> |
| 테넌트 쿼터 레코드 (Tenant Quota Records) | 특정 테넌트/조직 범위의 max bytes, max objects, max active uploads를 저장하며, active MPU admission은 새 upload id 발급 전에 적용합니다. | <span class="badge">Community metadata foundation</span> |
| Tenant Usage Reconciliation | tenant bucket metadata를 스캔해 bytes/object-version/active-upload baseline을 보수적으로 기록합니다. | <span class="badge">Community metadata foundation</span> |
| Request Concurrency Limiter | gateway, tenant, read/write operation class 단위로 동시 S3 요청 수를 제한합니다. | <span class="badge">Community gateway control</span> |
| Gateway Bandwidth Hooks | upload request body와 download response writer를 gateway-local bytes-per-second shaper로 감싸며, 0이면 비활성화합니다. | <span class="badge">Community gateway control</span> |
| 대역폭 제어 (QoS Rate Limit) | gateway 전체에서 IP, API action, 테넌트 토큰별 초당 요청 수(TPS) 및 대역폭(MB/s)을 제한합니다. | <span class="badge enterprise">Enterprise edition only</span> |
| 알림 발생 (Threshold Alert) | 쿼터 초과 전 경고 임계치에 도달하면 경고 로그 및 알림을 발송합니다. | <span class="badge enterprise">Enterprise edition only</span> |

## Quota 및 QoS 설정 스키마

Community 버킷 최대 오브젝트 크기 quota와 tenant quota record는 관리자 CLI로 관리합니다.

```sh
namros-admin bucket-quota-put -bucket finance-reports -max-object-size-bytes 1073741824
namros-admin bucket-quota-get -bucket finance-reports
namros-admin bucket-quota-delete -bucket finance-reports
namros-admin tenant-quota-put -tenant-id finance -max-bytes 1099511627776 -max-objects 1000000 -max-active-uploads 256
namros-admin tenant-quota-get -tenant-id finance
namros-admin tenant-quota-delete -tenant-id finance
```

아래 JSON은 특정 버킷 및 테넌트에 적용하는 대역폭 제어와 hard quota의 Enterprise 설정 스키마 예시입니다.

```json
{
  "policy_id": "finance-tenant-limits",
  "tenant": "finance-dept",
  "quota": {
    "max_size_bytes": 1099511627776,
    "max_objects_count": 5000000,
    "warning_threshold_percent": 80
  },
  "qos_limits": {
    "read_bandwidth_mb_per_sec": 500,
    "write_bandwidth_mb_per_sec": 200,
    "max_read_tps": 5000,
    "max_write_tps": 1000
  }
}
```

## Admission 및 S3 오류 명세

제한 범위를 초과했을 때 게이트웨이가 S3 표준에 맞게 반환하는 오류 XML 형식입니다.

### 1. 스토리지 용량 쿼터 초과 시 (HTTP 409 Conflict / QuotaExceeded)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>QuotaExceeded</Code>
  <Message>The bucket write request was denied because the hard quota limit of maximum bytes has been exceeded.</Message>
  <Resource>/finance-reports/accounting/q2_report.csv</Resource>
  <RequestId>req-88cc-9921-ab</RequestId>
</Error>
```

### 2. QoS TPS 및 대역폭 제한 작동 시 (HTTP 503 Slow Down)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>SlowDown</Code>
  <Message>Please reduce your request rate. Bandwidth or TPS limit exceeded for this tenant pool.</Message>
  <Resource>/finance-reports/</Resource>
  <RequestId>req-77bc-3341-ef</RequestId>
</Error>
```

## Prometheus 메트릭 레퍼런스

아래 실시간 Prometheus 메트릭은 활성 배포판이 명시적으로 구현한 경우를 제외하면 cluster monitoring과 Grafana dashboard를 위한 Enterprise/spec telemetry입니다.

| 메트릭 키 (Metric Key) | 설명 |
| --- | --- |
| `tenant_usage_bytes` | 테넌트가 점유 중인 총 메타데이터 및 물리 페이로드 용량. |
| `bucket_usage_bytes` | 버킷 단위 스토리지 사용 바이트. |
| `bucket_object_count` | 버킷 내 누적 오브젝트의 총개수. |
| `quota_denied_requests` | 쿼터 초과로 인해 거부된 S3 쓰기(PUT/MPU) 누적 횟수. |
| `rate_limited_requests` | QoS 대역폭 제한을 초과해 HTTP 503을 반환한 횟수. |
| `namros_gateway_admission_rejections_total{kind,reason}` | 버킷 quota, request limit, data budget admission 실패를 낮은 cardinality로 집계하는 구현된 counter입니다. 동일한 `kind`/`reason` 요약은 `/debug/operations/metrics`와 console operations view에도 표시됩니다. |
| `namros_gateway_admission_decision_duration_seconds{kind,outcome,reason}` | request limit, data budget, bucket quota admission 판정 시간을 집계하는 구현된 histogram입니다. label은 admission kind, accepted/rejected outcome, 설정된 reason으로 제한합니다. |
| `namros_gateway_s3_errors_total{api,status_class,error_code}` | S3 XML error를 낮은 cardinality로 집계하는 구현된 counter입니다. 버킷 이름, 오브젝트 키, version id, request id는 label에서 제외합니다. |

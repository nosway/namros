데이터 보호 <span class="badge enterprise">Enterprise edition only</span>

# NAMROS 복제 및 재해복구 가이드

<div class="warning" markdown="1">

**Enterprise edition only.** 이 페이지는 Enterprise 전용 cross-region replication과 disaster recovery 계약을 설명합니다. Community edition 동작은 local HA와 replication, failover, failback 표면을 분리해 설명하기 위해서만 포함합니다.

</div>

<div class="summary" markdown="1">

이 가이드는 리전 간(Cross-Region) 데이터 복제와 지리적 분산 재해 복구(DR)에 대한 Enterprise 계약과 운영 모델을 정의합니다. Community active-active 게이트웨이는 단일 로컬 클러스터 안의 가용성을 제공하며, 리전 간 복제, failover 승격, failback 자동화는 Enterprise 또는 roadmap 영역으로 공개 Community 관리자 명령에 노출되지 않습니다.

</div>

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| 로컬 HA | Active-active 게이트웨이가 하나의 클러스터 안에서 TiKV 메타데이터, etcd coordination, shared/SBS-backed storage를 공유할 수 있습니다. | Enterprise topology의 기반 요소로 동일하게 활용할 수 있습니다. |
| 리전 간 복제 | 공개 Community에는 `namros-admin replication` 명령이나 replication worker가 없습니다. | 버킷, 사이트, 배치 복제를 위한 Enterprise 계약입니다. |
| DR failover/failback | 공개 Community CLI 밖의 운영자 runbook으로 처리합니다. | 승인된 Enterprise 운영, DNS/load-balancer 전환, audit evidence의 목표 동작입니다. |

## 복제 범위

Enterprise 복제 계약은 복제 단위와 시나리오에 따라 세 가지 데이터 동기화 모드를 정의합니다.

| 복제 모드 | 기술 설명 | 활용 목적 |
| --- | --- | --- |
| 버킷 복제 (Bucket Replication) | 특정 버킷에 커밋된 신규 오브젝트 및 버전을 정책에 따라 대상 버킷으로 비동기 복제. | 기밀 데이터 원격지 보관, 로컬 규제 준수 |
| 사이트 복제 (Site Replication) | 두 개 이상의 지리적 분산 사이트 사이에서 버킷 메타데이터, 사용자 계정 정책, 세그먼트 페이로드를 동기화. | 리전 단위 active-passive/active-active DR 구성 |
| 배치 복제 (Batch Replication) | 정책 활성화 이전에 존재하던 과거 버전의 오브젝트나 장애 중 전송되지 못한 누적 오브젝트를 한 번에 backfill. | 복구 이후 누락 데이터 동기화, 초기 마이그레이션 |

## 토폴로지 및 보안 전제

크로스 리전 복제를 적용하기 위해 사전 검토해야 할 토폴로지 전제 조건입니다.

- **버전 관리(Versioning) 필수 활성화:** 원본(Source)과 대상(Destination) 버킷 모두 S3 Versioning이 활성화되어 있어야 삭제 마커(Delete Marker) 누락이나 일관성 오류를 방지할 수 있습니다.
- **IAM 신뢰 관계 수립:** 대상 리전의 NAMROS 게이트웨이가 원본의 복제 작업용 백그라운드 worker 자격 증명을 검증할 수 있도록 외부 STS 토큰과 복제 전용 trust role 정책이 필요합니다.
- **SSE-KMS 키 매핑:** 마스터 키가 리전별로 다르면 원본의 `prod-key-us`로 복호화한 뒤 대상의 `prod-key-kr` 키로 재암호화해 기록하는 **KMS Key Translation** 설정을 지정해야 합니다.

## S3 호환 복제 설정 예시

특정 prefix 데이터를 암호화 상태를 유지한 채 원격 버킷으로 전송하는 JSON 정책 예시입니다.

```json
{
  "Role": "arn:aws:iam::123456789012:role/namros-replication-role",
  "Rules": [
    {
      "ID": "FinanceReportsSync",
      "Status": "Enabled",
      "Priority": 1,
      "Filter": {
        "Prefix": "accounting/"
      },
      "Destination": {
        "Bucket": "arn:aws:s3:::dr-finance-reports",
        "Account": "123456789012",
        "EncryptionConfiguration": {
          "ReplicaKmsKeyID": "arn:aws:kms:ap-northeast-2:123456789012:key/kr-master-key-01"
        },
        "ReplicationTime": {
          "Status": "Enabled",
          "Time": {
            "Minutes": 15
          }
        }
      },
      "SourceSelectionCriteria": {
        "SseKmsEncryptedObjects": {
          "Status": "Enabled"
        }
      }
    }
  ]
}
```

## 재해복구(DR) 운영

DR 사이트로의 수동 failover는 승인된 Enterprise operation 또는 사이트별 runbook으로 수행해야 합니다. 예전 문서에 있던 `namros-admin replication ...` 형태의 중첩 명령 예제는 현재 공개 CLI에 포함되어 있지 않습니다.

| 단계 | 운영자 작업 | 필수 증빙 |
| --- | --- | --- |
| 1. 지연 감시 | Enterprise replication lag, backlog bytes, last successful sync time, destination health를 조회합니다. | 타임스탬프가 포함된 lag report와 source/destination gateway health. |
| 2. DR 사이트 승격 | 가능하면 source write를 동결하고 destination bucket/site를 승격한 뒤 DNS 또는 load balancer routing을 변경합니다. | 승인 기록, 승격 결과, routing 변경, S3 smoke 결과. |
| 3. Failback | primary 복구 후 reverse backfill을 수행하고 object/version alignment를 검증한 뒤 traffic을 되돌립니다. | reverse-sync report, reconciliation summary, final smoke result. |

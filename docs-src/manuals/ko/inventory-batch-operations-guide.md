대형 네임스페이스 운영 <span class="badge enterprise">Enterprise edition only</span>

# NAMROS 인벤토리 및 배치 운영 가이드

<div class="warning" markdown="1">

**Enterprise edition only.** 이 페이지는 Enterprise 전용 inventory와 batch operation 계약을 설명합니다. Community edition 동작은 현재 사용 가능한 metadata export 기반 요소와 scheduled inventory/batch worker 부재를 설명하기 위해서만 포함합니다.

</div>

수억 개 이상의 대규모 오브젝트 네임스페이스에서는 일반적인 S3 List API 호출만으로 전체 자산 상태나 보관 정책을 파악하기 어렵습니다. NAMROS Enterprise 에디션은 버킷 자산을 자동으로 집계하는 오브젝트 인벤토리 사양과 대규모 자산 배치 작업 프레임워크를 정의합니다.

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| 메타데이터 export | `namros-admin metadata-export`가 backup, migration, audit workflow용 product metadata collection을 export합니다. | Inventory evidence의 기반 요소로 사용할 수 있습니다. |
| S3 Object Inventory | 공개 Community에는 scheduled inventory worker가 활성화되어 있지 않습니다. | 주기적 inventory materialization과 report storage를 위한 Enterprise 계약입니다. |
| S3 Batch Operations | 공개 Community에는 bulk mutation framework가 활성화되어 있지 않습니다. | 승인된 대규모 mutation job과 audit envelope를 위한 Enterprise 계약입니다. |

## 인벤토리 스키마

| 필드 | 목적 |
| --- | --- |
| bucket/key/version | 오브젝트 식별자. |
| size/checksum/etag | 데이터 검증과 그룹화. |
| storage class | 배치 위치와 라이프사이클 분석. |
| encryption status | KMS 상태와 컴플라이언스 증빙. |
| lock/retention status | WORM 및 삭제 안전성. |
| replication status | DR 지연과 실패 리포트. |

## 배치 작업 유형

| 작업 | 기대 제어 |
| --- | --- |
| 복사 | 범위 미리보기, 충돌 정책, KMS 매핑. |
| 삭제 | Object Lock/보호 참조 사전 점검과 승인. |
| 태그 | 정책 시뮬레이션과 변경 리포트. |
| 복원 | 아카이브/티어 복원이 존재한 뒤에만 허용. |

## 리포트와 감사

```text
job_id:
scope:
planned_count:
applied_count:
skipped_count:
failed_count:
audit_record:
report_path:
```

배치 작업은 [MCP 운영 가이드](mcp-operations-guide.md)에서 설명한 것과 같은 plan/preflight/apply/verify/audit envelope를 사용해야 합니다.

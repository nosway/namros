대형 네임스페이스 운영 <span class="badge planned">계획/명세 단계</span>

# NAMROS 인벤토리 및 배치 운영 가이드

<div class="warning" markdown="1">

**Enterprise 계획/명세 단계.** 이 페이지는 제안된 inventory와 batch
operation 계약을 설명합니다. 현재 Community metadata export는 기반 요소지만
일반 공급 scheduled inventory 또는 승인 기반 batch mutation service를
의미하지 않습니다.

</div>

이 문서는 대규모 namespace를 위한 주기적 inventory materialization과 승인
기반 batch workflow를 Enterprise 방향으로 제안합니다. 아래 schema는 구현
상태 표가 Community 기반으로 명시한 부분을 제외하면 설계 후보입니다.

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| 메타데이터 export | `namros-admin metadata-export`가 backup, migration, audit workflow용 product metadata collection을 export합니다. | Inventory evidence의 기반 요소로 사용할 수 있습니다. |
| S3 Object Inventory | 공개 Community에는 scheduled inventory worker가 활성화되어 있지 않습니다. | 주기적 materialization과 report storage를 위한 계획된 계약입니다. |
| S3 Batch Operations | 공개 Community에는 bulk mutation framework가 활성화되어 있지 않습니다. | 승인된 mutation job과 audit envelope를 위한 계획된 계약입니다. |

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

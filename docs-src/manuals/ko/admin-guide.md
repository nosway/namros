Day-2 운영

# NAMROS 관리자 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition 운영과 Enterprise edition only 운영군을 함께 다룹니다. Enterprise-only operation row는 페이지가 Community denial behavior를 명시한 경우를 제외하고 private distribution 계약으로 읽어야 합니다.

</div>

## 컴포넌트 소유 경계

| 컴포넌트 | 소유 범위 | 소유하지 않는 범위 |
| --- | --- | --- |
| namros-gateway | S3 라우팅, 인증, 요청 처리, 캐시, 디버그 엔드포인트 | 쓰기 게시 이후의 정본 오브젝트 상태 |
| 메타데이터 백엔드 | 버킷/오브젝트/버전/MPU/라이프사이클/작업 레코드 | 페이로드 바이트 |
| 세그먼트 스토리지 | 페이로드 세그먼트와 삭제 시도 | 네임스페이스 가시성 |
| etcd | <span class="badge">Community</span> 게이트웨이 레지스트리와 상태 임대 | 오브젝트 메타데이터 또는 페이로드 |
| TiKV/SBS | <span class="badge">Community</span> 분산 메타데이터와 복제 물리 스토리지, <span class="badge enterprise">Enterprise edition only</span> EC 물리 스토리지 | 메타데이터 권한과 페이로드 저장의 제품 경계 |

## 표준 토폴로지

| 토폴로지 | 구성 요소 | 용도 |
| --- | --- | --- |
| Local Community | 게이트웨이, Pebble, 로컬 세그먼트 경로 | 개발 및 사용자 공간 호환성 검증 |
| 호환성 실험실 | 게이트웨이와 AWS CLI/mc/rclone/s3fs 클라이언트 | 외부 클라이언트 검증 |
| Active-active Community | 여러 게이트웨이, TiKV, etcd, 공유 스토리지 | 가용성과 무상태 게이트웨이 검증 |
| SBS EC Enterprise | TiKV, SBS service/data, EC 샤드 경로, 게이트웨이 | EC 스토리지 클래스 검증 |

## 게이트웨이 설정

```sh
go run ./cmd/namros-gateway \
  -listen 127.0.0.1:9000 \
  -region us-east-1 \
  -metadata-backend pebble \
  -metadata-path .namros/meta \
  -storage-backend local \
  -storage-path .namros/segments
```

분산 메타데이터가 필요한 Community 배포는 TiKV/PD 설정과 etcd 게이트웨이 코디네이션을 사용할 수 있습니다. SBS EC, 중복 제거, WORM/Object Lock 강제, SSE-KMS, 컴플라이언스 증빙은 Enterprise 기능으로 제한됩니다.

## 상태 및 디버그 엔드포인트

```sh
curl -fsS http://127.0.0.1:9000/healthz
curl -fsS http://127.0.0.1:9000/debug/admin/status
curl -fsS http://127.0.0.1:9000/debug/operations/metrics
```

| 엔드포인트 | 용도 | 장애 신호 |
| --- | --- | --- |
| `/healthz` | 준비 상태와 로드밸런서 헬스 체크 | 2xx 이외 응답 또는 타임아웃 |
| `/debug/admin/status` | 메타데이터 백엔드 식별자, 컬렉션 수, 기능 플래그 | 백엔드 사용 불가 또는 컬렉션 수 불일치 |
| `/debug/operations/metrics` | GC, 중복 제거, TiKV, 스케줄러 메트릭 스냅샷 | 재시도 카운터 증가 또는 오래된 스케줄러 상태 |

## 메타데이터 백업/복구

메타데이터 내보내기/가져오기는 가장 먼저 사용할 수 있는 안전한 운영 복구 경로입니다. 소스 식별자, 감사 해시 정보, 컬렉션 수, 대상 충돌 검사를 반드시 보존해야 합니다.

```sh
make smoke-metadata-backup-restore
```

1. 소스 백엔드에서 메타데이터를 내보냅니다.
2. 스키마와 컬렉션 수를 검증합니다.
3. 빈 대상에 대한 사전 점검을 실행합니다.
4. 비어 있지 않은 대상의 충돌 사전 점검을 실행합니다.
5. 대상 검사와 충돌 정책이 명확해진 뒤에만 적용합니다.

참고: [TiKV 운영 가이드](tikv-ha-cluster-install-operations-guide.md)와 [업그레이드 및 릴리스 가이드](upgrade-release-operations-guide.md).

## 라이프사이클, GC, 중복 제거, 컴플라이언스 운영

| 작업 | Community | Enterprise | 운영자 규칙 |
| --- | --- | --- | --- |
| 라이프사이클 계획 | 가능한 경우 비변경 계획 | 정책 인지 계획과 워커 | 적용 전 차단된 작업을 점검 |
| 고아 객체 GC 재시도 | 로컬 보호 참조를 고려한 정리 | SBS/EC를 고려한 정리 | 보호 참조를 확인할 수 없으면 차단 상태로 실패 |
| 중복 제거 | Enterprise 필요 관리자 경로 | 후보 선정, 검증, 승인, 스크럽, 복구 | 바이트 검증 없이 공유 참조를 연결하지 않음 |
| 컴플라이언스 증빙 | Enterprise 필요 관리자 경로 | 증빙 패키지와 정책 시뮬레이션 | 제약 사항과 비인증 경계를 기록 |

## 릴리스 준비도

```sh
make release-readiness
make check-community-export
make export-community
make html-docs-check
```

`release-readiness`는 `release-reports/` 아래에 JSON/Markdown 산출물을 생성합니다. Community 공개본에는 Enterprise 기능을 여는 공개 경로가 없어야 하며, 유효한 소스 내보내기와 사용자 공간 호환성 통과가 추가로 필요합니다.

## Day-2 운영 확장

| 영역 | 가이드 | 목적 |
| --- | --- | --- |
| 웹 콘솔 및 모니터링 | [콘솔 가이드](web-console-monitoring-guide.md) | 대시보드, 리포트 뷰어, 알림 요약, 승인된 작업 흐름. |
| S3 오브젝트 브라우저 연동 | [오브젝트 브라우저 가이드](s3-object-browser-integration-guide.md) | Object Explorer Lite 범위와 외부 S3 browser recipe. |
| 복제 및 DR | [복제 가이드](replication-disaster-recovery-guide.md) | 사이트/버킷/배치 복제, 지연, 장애 전환/복귀 계획. |
| 이벤트 | [이벤트 가이드](event-notification-guide.md) | Webhook/Kafka/NATS 알림, 재시도, DLQ, 재처리. |
| 인벤토리 및 배치 | [인벤토리 가이드](inventory-batch-operations-guide.md) | 대형 네임스페이스 리포트와 배치 작업 envelope. |
| 용량 및 유지보수 | [용량 가이드](capacity-scaling-maintenance-guide.md) | 노드 유지보수, 디컴미션, 힐링, 리밸런스, 샤드 점검. |
| 쿼터 및 QoS | [쿼터 가이드](quota-qos-guide.md) | 버킷/테넌트 쿼터, 속도 제한, 사용량 메트릭, 임계치 알림. |

## 장애 대응 체크리스트

1. 엔드포인트, 버킷/키, 요청 ID, 클라이언트 명령을 식별합니다.
2. 게이트웨이 상태와 관리자 상태를 캡처합니다.
3. 운영 메트릭을 캡처합니다.
4. 해당되는 경우 호환성 임시 디렉터리 또는 릴리스 리포트를 보존합니다.
5. 클라이언트 설정, 게이트웨이, 메타데이터, 스토리지, Enterprise 의존성 중 어느 하위 시스템인지 분류합니다.
6. 운영자가 승인한 복구 작업 전에는 읽기 전용 검사만 실행합니다.
7. 후속 조치를 장애 번들 또는 릴리스 리포트에 기록합니다.

## Enterprise 운영 경계

<span class="badge enterprise">Enterprise edition only</span> SBS EC/중복 제거/KMS/컴플라이언스 운영은 사설 배포판의 제품 기능입니다. 이 영역을 건드리는 Community 명령과 S3 요청은 조용히 기능을 축소하거나 일부 동작을 켜는 대신 NAMROS Enterprise Edition 필요 오류를 반환해야 합니다. TiKV 메타데이터와 etcd 게이트웨이 코디네이션은 Community 기능입니다.

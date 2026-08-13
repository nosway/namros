M23 운영 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS 웹 콘솔 및 모니터링 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition read-only dashboard 동작과 Enterprise edition only approved operations, compliance, chaos/soak, feature-panel 섹션을 함께 다룹니다. Enterprise-only panel은 공개 Community 빌드에서 edition-boundary message를 표시해야 합니다.

</div>

<div class="summary" markdown="1">

웹 콘솔은 CLI/MCP를 대체하는 별도 truth source가 아닙니다. 안정적인 관리자 API, 디버그 엔드포인트, 리포트 산출물, MCP 작업 envelope를 재사용하는 브라우저 기반 운영 화면입니다.

SBS node, store, volume, capacity, reclaim, maintenance 데이터는 NAMRBD가 소유한 읽기 전용 observability surface에서 소비합니다. 공개 콘솔에서 NAMROS는 SBS drain, remove, rejoin, repair, rebalance, reclaim 동작을 별도로 구현하지 않습니다.

</div>

## 콘솔 접근과 운영 자세

Embedded 운영 GUI는 각 `namros-gateway`의 `/console/`에서 제공됩니다. 공개 배포에서는 gateway 운영에 사용하는 같은 hostname 또는 load balancer를 통해 console을 노출합니다. 예를 들어 `https://namros.example.com/console/`처럼 접근하게 구성할 수 있습니다. Multi-gateway 배포는 `/readyz`를 health check하고 정상 gateway instance의 `/console/`로 browser traffic을 전달하면 됩니다.

공개 Community 운영 자세는 데이터 중심 read-only입니다. 운영자는 status, alerts, reports, metrics summary, Object Explorer Lite metadata, NAMRBD에서 가져온 SBS observability를 확인할 수 있습니다. Mutation control, repair workflow, SBS maintenance action, Enterprise feature page는 별도로 검토된 operation workflow가 활성화하기 전까지 없거나 비활성화되거나 edition-boundary message로 표시되어야 합니다.

## 데이터소스와 SBS Observability 설정

| 설정 | Gateway flag | 환경 변수 | 목적 |
| --- | --- | --- | --- |
| Prometheus | `-observability-prometheus-url` | `NAMROS_OBSERVABILITY_PROMETHEUS_URL` | Prometheus query용 console deep link와 datasource descriptor. |
| Grafana | `-observability-grafana-url` | `NAMROS_OBSERVABILITY_GRAFANA_URL` | Provisioned dashboard로 이동하는 console deep link. 값이 없으면 `grafana_unconfigured` warning이 표시됩니다. |
| VictoriaMetrics | `-observability-victoria-url` | `NAMROS_OBSERVABILITY_VICTORIA_URL` | 장기 보관 metrics용 선택 datasource. VictoriaMetrics 배포가 없으면 비워 둡니다. |
| NAMRBD SBS observability | `-namrbd-sbs-observability-endpoint` | `NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT` | SBS cluster, node, volume, capacity, reclaim, maintenance projection의 읽기 전용 source. |
| NAMRBD SBS timeout | `-namrbd-sbs-observability-timeout` | `NAMROS_NAMRBD_SBS_OBSERVABILITY_TIMEOUT` | NAMRBD SBS observability HTTP 수집 timeout. |

`NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT`가 비어 있으면 SBS panel은 endpoint가 설정될 때까지 unconfigured 또는 partial source 상태를 표시합니다. 모든 gateway instance에서 접근 가능한 service name, gateway-local network route, orchestrator service URL을 사용합니다.

## 공개 배포 예시

Gateway service environment 또는 대응하는 command-line flag로 console datasource link를 설정합니다.

```sh
export NAMROS_OBSERVABILITY_PROMETHEUS_URL=https://prometheus.example.com
export NAMROS_OBSERVABILITY_GRAFANA_URL=https://grafana.example.com
export NAMROS_OBSERVABILITY_VICTORIA_URL=https://victoria.example.com
export NAMROS_NAMRBD_SBS_OBSERVABILITY_ENDPOINT=https://namrbd-sbs-observability.example.com
export NAMROS_NAMRBD_SBS_OBSERVABILITY_TIMEOUT=30s
```

Load-balanced fleet에서는 각 gateway에 같은 외부 datasource URL과 gateway process에서 접근 가능한 SBS observability endpoint를 설정합니다. Load balancer는 `/console/` traffic을 정상 gateway로 전달하고 `/readyz`를 health check로 사용합니다.

## 에디션 범위

| 패널/작업 | Community | Enterprise |
| --- | --- | --- |
| 게이트웨이/메타데이터/스토리지 상태 | 읽기 전용 대시보드 | 읽기 전용 대시보드 |
| SBS 운영 데이터 | NAMRBD observability adapter, 읽기 전용 | NAMRBD observability adapter, 읽기 전용과 별도로 승인된 NAMRBD workflow |
| 리포트 뷰어 | 호환성/릴리스/백업 리포트 | 컴플라이언스/chaos/soak 리포트 포함 |
| Object Explorer Lite | 읽기 전용 bucket, prefix, object metadata와 외부 S3 browser recipe | 정책 제어 이후 승인된 download/delete만 검토 |
| 승인된 작업 | 제한된 Community 작업 | <span class="badge enterprise">Enterprise edition only</span> 복구/보호/증빙 작업 |
| Enterprise 기능 페이지 | Enterprise 필요 메시지 | 배포 권한에 따라 활성화 |

## 콘솔 API 후보

| 엔드포인트 | 목적 |
| --- | --- |
| `/api/v1/status` | 클러스터, 게이트웨이, 메타데이터, 스토리지 상태. |
| `/api/v1/operations/summary` | 게이트웨이, 메타데이터, SBS, 알림, 리포트, Object Explorer 상태를 묶은 읽기 전용 overview. |
| `/api/v1/operations/warnings` | 공개 read-only 운영 화면에 표시할 제한 사항과 경고. |
| `/api/v1/query/views` | Schema version, source authority, availability를 포함한 view catalog. |
| `/api/v1/gui/summary` | Console navigation, refresh policy, datasource descriptor. |
| `/api/v1/workflow/hardening` | 읽기 전용 workflow 경계, disabled action, 승인 posture, audit 설정. |
| `/api/v1/metrics` | 정규화된 운영 메트릭. |
| `/api/v1/reports` | 호환성, 릴리스, 백업, chaos/soak 리포트 색인. |
| `/api/v1/operations` | 작업 plan/preflight/apply/verify/audit 이력. |
| `/api/v1/edition` | 에디션과 권한 카탈로그. |
| `/api/v1/sbs/cluster`, `/nodes`, `/stores`, `/volumes`, `/capacity`, `/reclaim`, `/maintenance` | NAMRBD에서 가져온 SBS observability projection과 read-only envelope. |
| `/api/v1/object-explorer/buckets` | 읽기 전용 bucket 목록과 운영 상태. |
| `/api/v1/object-explorer/objects` | Pagination과 version-aware shape를 갖는 읽기 전용 prefix/object listing. |
| `/api/v1/object-explorer/external-clients` | 외부 S3 browser 도구용 redacted connection recipe. |

## 대시보드 패널

- 게이트웨이 집합 상태와 etcd 임대 최신성.
- TiKV 메타데이터 상태와 트랜잭션 메트릭.
- NAMRBD observability에서 가져온 SBS 복제 및 Enterprise EC 스토리지 상태.
- NAMROS 소유 SBS mutation control 없이 SBS capacity/reclaim 표시.
- 최근 호환성, 릴리스 준비도, 백업/복구 리포트.
- 상태 저하, 쿼터 임계치, 예상된 Enterprise 거부에 대한 알림 요약.
- Bucket/prefix/object metadata만 표시하는 Object Explorer Lite.

## Object Explorer 경계

Object Explorer Lite는 안정적인 S3/admin list와 head surface 위의 읽기 전용 운영 화면입니다. Private metadata를 직접 해석하거나, segment-store payload bytes를 읽거나, upload, copy/move, recursive delete, bulk delete control을 노출하면 안 됩니다.

전체 파일 browsing과 transfer workflow는 [S3 오브젝트 브라우저 연동 가이드](s3-object-browser-integration-guide.md)에 문서화된 검증된 외부 S3 도구를 사용합니다. 콘솔 응답과 recipe는 access key secret, session token, KMS material, Authorization header, presigned URL을 마스킹해야 합니다.

## 승인 및 마스킹

상태를 변경하거나 테스트 리소스를 생성하는 모든 작업은 plan, preflight, apply, verify, audit 필드를 표시해야 합니다. Local console auth가 enabled이면 상태 변경 console API 요청은 session-derived `X-Namros-CSRF-Token` header를 포함해야 합니다. 콘솔은 접근 키, Authorization 헤더, KMS material, 사전 서명 URL, 오브젝트 페이로드 바이트를 마스킹해야 합니다.

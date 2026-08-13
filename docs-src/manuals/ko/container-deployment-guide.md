Community 패키징 계약

# NAMROS Community 컨테이너 배포 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition 컨테이너 배포 계약을 정의하며 Enterprise edition only 기능은 명시적 제외 항목으로만 나열합니다. SBS EC, 중복 제거, WORM/Object Lock, KMS lifecycle, 컴플라이언스 증빙은 Community 컨테이너 이미지에서 활성화되지 않습니다.

</div>

## 상태와 범위

<div class="note" markdown="1">

**상태:** 확정된 배포 계약이며 소스 트리에는 임시 구현 artifact가 포함되어 있습니다. Dockerfile, Compose profile, readiness 동작, secret file 입력, `container-*` 타깃은 존재하지만 릴리스 제공으로 표시하려면 Docker runtime acceptance와 게시 이미지 증빙이 필요합니다.

</div>

이 문서는 Community 컨테이너 사용 경험을 정의합니다. 첫 제공 범위는 Docker와 Docker Compose입니다. Helm은 Compose 계약을 구현하고 검증한 뒤 별도의 Kubernetes 배포 가이드에서 다룹니다.

TiKV 메타데이터, etcd 게이트웨이 coordination, active-active 게이트웨이, SBS 복제 오브젝트 스토리지는 Community 기능입니다. <span class="badge enterprise">Enterprise edition only</span> SBS EC, 중복 제거, WORM/Object Lock enforcement, KMS lifecycle, 컴플라이언스 증빙은 private distribution 기능입니다.

## 배포 프로파일

| 프로파일 | 구성요소 | 목적 | 데이터 |
| --- | --- | --- | --- |
| `local` | 게이트웨이 1개, Pebble, 로컬 세그먼트 저장소 | 빠른 S3 평가와 개발 | named volume |
| `community` | HAProxy, 게이트웨이 2개, etcd, PD/TiKV 테스트 컨테이너, SBS load balancer, SBS service 2개, SBS data 4개 | active-active와 Community 복제 검증 | named volume |
| `observability` | Community 프로파일과 선택 Prometheus/Grafana | 메트릭과 대시보드 평가 | 프로파일별 volume |

단일 `packaging/docker/compose.yaml`에서 명시적인 Compose profile을 사용합니다. 첫 실행에는 local 프로파일을 권장합니다. Community 프로파일은 테스트 topology이며 프로덕션 etcd/TiKV 배포가 아닙니다. 포함된 etcd, PD, TiKV는 각각 단일 failure domain입니다. Production-scale release claim에는 `make production-scale-check` 실행과 skip된 외부 smoke gate 검토도 필요합니다.

## 이미지, 태그와 플랫폼

| 이미지 | 포함 항목 | 초기 책임 |
| --- | --- | --- |
| `ghcr.io/nosway/namros-gateway` | `namros-gateway` | S3 서비스 |
| `ghcr.io/nosway/namros-tools` | `namros-admin`, `namros-s3bench`, 운영 helper | 스모크와 관리 작업 |
| PD/TiKV 테스트 메타데이터 서비스 | 고정된 PD와 TiKV 컨테이너 | 테스트 전용 메타데이터 서비스, wrapper hardening은 릴리스 작업 |

릴리스 이미지는 Linux `amd64`와 `arm64`를 지원합니다. macOS와 Windows 사용자는 Docker Desktop으로 Linux 이미지를 실행합니다. 게시 버전은 `vX.Y.Z`, `vX.Y`, `sha-<commit>` 태그와 불변 digest를 사용합니다. 공식 예제는 release version을 고정하고 검증 BOM에는 digest를 기록하며 `latest`에 의존하지 않습니다.

런타임 이미지는 non-root 사용자로 실행하고 Linux capability를 제거하며 read-only root filesystem을 사용합니다. 문서화한 상태 디렉터리와 임시 경로만 named volume 또는 `tmpfs`로 쓰기를 허용합니다. OCI label에는 source, revision, version, license와 Community edition identity를 기록합니다.

현재 구현은 Community 테스트 topology에서 PD와 TiKV를 별도 컨테이너로 사용합니다. 이 경로를 릴리스 가능 상태로 표시하기 전에는 combined wrapper 또는 동등한 release-hardened 메타데이터 topology가 PID 1 동작, signal 전달, 시작 순서, readiness, 로그 전달, 데이터 디렉터리, 재시작 복구를 정의해야 합니다. 호환성은 NAMROS, NAMRBD/SBS, etcd, Compose 계약 버전과 함께 릴리스 BOM에 기록합니다.

## 로컬 빠른 시작 계약

첫 실행 경로는 게이트웨이 1개, Pebble 메타데이터와 로컬 세그먼트 저장소를 사용합니다. 컨테이너는 `0.0.0.0:9000`에서 수신하고 Compose는 호스트의 `127.0.0.1:9000`에만 게시합니다. 영속 경로는 `/var/lib/namros/meta`와 `/var/lib/namros/segments`입니다.

```sh
# 소스 트리의 임시 구현 명령입니다. 릴리스 사용에는 acceptance 증빙이 필요합니다.
sh scripts/container/ensure-local-files.sh
make container-local-up
make container-local-smoke
make container-local-down
```

예제 환경 파일은 개발 전용 자격 증명 placeholder를 포함하며 Git에서 제외합니다. 사용자는 시작 전에 값을 교체해야 합니다. 이미지에는 고정된 기본 자격 증명을 내장하지 않습니다.

| 타깃 | 효과 |
| --- | --- |
| `container-local-up` | local 프로파일을 빌드하거나 시작 |
| `container-local-smoke` | toolbox S3 스모크 실행 |
| `container-local-down` | 컨테이너를 중지하고 named volume 보존 |
| `container-local-reset` | 컨테이너를 중지하고 local 프로파일 volume을 영구 삭제 |

<div class="note" markdown="1">

`container-local-reset`은 파괴적인 작업입니다. 일반 종료는 메타데이터와 오브젝트 데이터를 보존합니다.

</div>

## 분산 Community 계약

| 계층 | 수 | 라우팅과 identity |
| --- | --- | --- |
| S3 front end | HAProxy 1개 | `127.0.0.1:9001` 게시, unready gateway 제외 |
| Gateway | 2개 | 명시적 ID `namros-gateway-a`/`namros-gateway-b`와 advertise endpoint |
| Coordination | etcd 1개 | 테스트 전용 registry와 lease, HA 아님 |
| Metadata | PD service 1개와 TiKV service 1개 | 테스트 전용 정본 메타데이터, HA 아님, wrapper hardening 필요 |
| SBS control plane | service 2개와 내부 HAProxy | 현재 gateway interface용 단일 안정 admin endpoint |
| SBS data plane | data node 4개와 내부 HAProxy | 현재 gateway interface용 단일 안정 data endpoint |
| SBS bootstrap | one-shot job 2개 | 설정된 복제 volume을 생성한 뒤 gateway 시작 전에 metadata volume pool 등록 |

게이트웨이가 현재 admin endpoint와 data endpoint를 각각 하나씩 받으므로 초기 호환 방식으로 SBS 내부 load balancer를 사용합니다. client-side endpoint list 또는 SBS native discovery로 교체하려면 interface 변경과 동등한 장애 전환 테스트가 필요합니다.

```sh
# Community Compose 진입점: packaging/docker/compose.community.yml
make container-community-up
make container-community-smoke
make container-community-failover-smoke
make container-community-down
```

Community Make target은 기본적으로 `packaging/docker/compose.community.yml`을 사용합니다. `sbs-bootstrap`은 쉼표로 구분한 `NAMROS_COMMUNITY_SBS_VOLUME_IDS`의 각 volume을 materialize하고, 이어서 `namros-pool-bootstrap`이 `NAMROS_COMMUNITY_SBS_VOLUME_POOL_ID`를 TiKV에 등록합니다. Gateway는 pool bootstrap 성공 뒤에만 시작하며 production deployment profile, metadata GC queue, writer group, session id, volume epoch 설정을 사용합니다.

Kubernetes reference chart는 `packaging/helm/namros-community`에 있습니다. `make helm-chart-check`로 chart 계약을 검증합니다. `values.local.yaml`은 local 평가용 embedded etcd, PD, TiKV를 켜고, `values.production.yaml`은 외부 hardened etcd/TiKV endpoint와 기존 root credential Secret을 요구합니다.

이 프로파일은 Community active-active와 복제 동작을 입증하기 위한 것이지만 단일 etcd, 단일 PD, 단일 TiKV service 때문에 프로덕션 HA를 주장할 수 없습니다. 프로덕션 배포는 `-deployment-profile production`, 외부 hardened etcd/TiKV cluster, metadata registry가 선택하는 SBS 복제 volume pool, SBS writer session fencing을 사용합니다.

## 설정과 비밀값

설정 우선순위는 다음과 같습니다.

```text
CLI flag > *_FILE 입력 > 일반 환경변수 > 내장 기본값
```

따라서 CLI 플래그가 가장 높은 우선순위를 가집니다. 동일한 secret을 여러 입력 계층에 지정하면 하나를 조용히 선택하지 않고 충돌 오류로 시작을 중단해야 합니다. Compose는 secret file을 사용하며 gateway는 최소한 다음 입력을 지원합니다.

```text
NAMROS_ROOT_ACCESS_KEY_ID_FILE
NAMROS_ROOT_SECRET_ACCESS_KEY_FILE
NAMROS_CONSOLE_ADMIN_PASSWORD_FILE
NAMROS_CONSOLE_SESSION_SECRET_FILE
```

secret 값은 명령행, `docker inspect` 환경 출력, 애플리케이션 로그, debug 설정, health 응답, support bundle에 나타나면 안 됩니다. 공개 배포 reference는 완전한 environment-to-flag mapping과 재시작이 필요한 설정을 표시합니다.

## 상태 점검, readiness와 관리 endpoint

| 표면 | 확정 계약 |
| --- | --- |
| `/healthz` | 프로세스 생존만 검사하며 dependency call을 하지 않음 |
| `/readyz` | 저비용의 실제 metadata/storage status 또는 read 수행, 어느 하나라도 사용할 수 없으면 HTTP 503 |
| etcd readiness | 다중 gateway Community 프로파일에서는 필수이며 장애 시 HTTP 503, coordination 비활성 구성에서는 검사 제외 |
| startup probe | 느린 dependency 초기화를 허용하는 별도 Kubernetes 계약 |
| `/debug/*` | 기본 비활성, 별도로 설정한 admin listener에서만 제공 |
| `/metrics` | 기본적으로 내부 container network에서만 접근, 인증 선택 적용 가능 |

Readiness 응답은 component 상태와 안정적인 reason code를 제공하되 endpoint, credential, key 또는 다른 secret을 노출하지 않습니다. 소스 트리 구현은 dependency readiness check를 수행하지만 릴리스 승인에는 컨테이너 runtime 증빙이 필요합니다.

## 스모크와 장애 전환 검증

빠른 스모크와 파괴적인 장애 전환 스모크를 분리합니다. Community 스모크는 `namros-gateway-a`와 `namros-gateway-b`를 직접 호출해 cross-gateway PUT/GET/LIST를 확인한 뒤, 활성 endpoint를 통해 load-balancer compatibility smoke를 재사용하여 bucket create/list/delete, object PUT/HEAD/GET/range GET, multipart upload, versioning, tagging, CORS, presigned GET을 검증합니다. 명시적인 etcd registration과 TiKV identity assertion은 릴리스 증빙 작업으로 남아 있습니다.

현재 장애 전환 스모크는 gateway 1개, SBS data container 1개, SBS service container 1개를 통제된 단계로 중지합니다. 각 component가 중지된 동안 load-balancer smoke를 실행하고 component를 복구한 뒤 cross-gateway recovery smoke를 실행합니다. 각 실행은 `.cache/container-community-failover-smoke` 아래에 `summary.json`과 `events.jsonl`을 기록합니다. 초기 성공 조건에는 고정된 복구 시간 SLA를 두지 않습니다. retry는 명시적인 전체 테스트 timeout으로 제한하고 측정한 시간은 기록하되 제품 SLA로 주장하지 않습니다.

최소/권장 CPU, 메모리, 디스크, image download 크기, 예상 시작 시간은 지원되는 `amd64`/`arm64` host에서 측정한 뒤 게시합니다. 측정하지 않은 예상치는 임시값이라고 표시해야 합니다.

## 데이터 수명과 운영

| 타깃 | 데이터 동작 |
| --- | --- |
| `container-community-down` | stack을 중지하고 volume 보존 |
| `container-community-reset` | etcd, TiKV, SBS 테스트 상태를 영구 삭제 |
| `container-build` | NAMROS 소스에서 local gateway/tools 이미지를 빌드합니다. Community 타깃은 설정된 NAMRBD context에서 전환 SBS 이미지도 빌드합니다. |

Reset 명령은 삭제할 정확한 project와 named volume을 출력합니다. 업그레이드/롤백 지원 범위는 release BOM과 명시적인 metadata/storage format 호환성 선언으로 정의합니다. 지원되지 않는 downgrade에서 기존 volume을 재사용하면 안 됩니다.

## 구현 완료 조건

- 공개 multi-stage image가 개발 전용 로컬 NAMRBD module replace 없이 빌드됩니다.
- Linux amd64/arm64 이미지가 non-root와 read-only root filesystem으로 실행됩니다.
- local 프로파일이 시작되고 down/up 사이에 데이터를 보존하며 smoke를 통과하고 파괴적 타깃으로만 초기화됩니다.
- Community 프로파일이 예상 replica를 모두 시작하고 일반/장애 전환 smoke를 통과합니다.
- HAProxy는 ready gateway로만 전달하고 etcd에는 두 gateway identity가 명시적으로 기록됩니다.
- 필수 metadata, storage 또는 다중 gateway coordination dependency 장애 시 readiness가 실패합니다.
- secret이 file input을 사용하고 우선순위를 따르며 inspect, log, debug output, report에 노출되지 않습니다.
- debug endpoint가 공개 S3 listener에 없고 metrics는 기본적으로 host에 게시되지 않습니다.
- image digest, SBOM, provenance, 취약점 결과, 검증된 component BOM이 release artifact로 제공됩니다.
- 측정된 리소스 요구량과 장애 전환 시간이 검증된 release에 첨부됩니다.

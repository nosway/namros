Community 프로덕션 운영

# etcd HA 설치 및 운영 가이드

## 역할

<span class="badge">Community</span> NAMROS에서 etcd는 게이트웨이 레지스트리와 상태 임대를 관리합니다. 오브젝트 메타데이터와 페이로드의 정본 저장소가 아니라, 게이트웨이 집합의 가용성과 제어 평면 상태를 표현하는 coordination 백엔드입니다.

## 실험실 및 HA 토폴로지

| 토폴로지 | 멤버 수 | 용도 |
| --- | --- | --- |
| 단일 노드 실험실 | 1 | 로컬 스모크, 임대 만료 동작 |
| 3노드 HA | 3 | 일반적인 프로덕션 구성의 쿼럼 |
| 5노드 HA | 5 | 쓰기 쿼럼 비용은 높지만 더 큰 장애 허용 범위 |

프로덕션 배포는 TLS, 인증, 내구성 있는 스토리지, 명시적인 멤버 이름을 사용해야 합니다. 이 가이드는 NAMROS 연동 지점을 문서화하며, 클러스터 하드닝은 운영자의 책임입니다.

## 로컬 실험실 포트

```sh
etcd --name namros-local \
  --data-dir .namros/etcd \
  --listen-client-urls http://127.0.0.1:12379 \
  --advertise-client-urls http://127.0.0.1:12379 \
  --listen-peer-urls http://127.0.0.1:12380 \
  --initial-advertise-peer-urls http://127.0.0.1:12380 \
  --initial-cluster namros-local=http://127.0.0.1:12380 \
  --initial-cluster-state new
```

macOS 기본 포트 `2379/2380`은 로컬 TiKV/PD 또는 이전 etcd 실행에서 이미 사용 중인 경우가 많습니다. 문서화된 로컬 스모크 경로는 흔한 충돌을 피하기 위해 `12379/12380`을 사용합니다.

## 게이트웨이 연동

같은 그룹의 모든 게이트웨이는 동일한 엔드포인트 집합과 레지스트리 루트를 사용해야 합니다. 게이트웨이 ID는 고유해야 합니다.

```sh
NAMROS_ETCD_ENDPOINTS=127.0.0.1:12379 \
NAMROS_GATEWAY_REGISTRY_PREFIX=/namros/gateways \
make smoke-etcd-registry
```

이 스모크는 레지스트리 키 생성, heartbeat 갱신, revoke 없이 게이트웨이가 종료된 뒤 임대 만료 제거를 확인합니다.

## 운영

| 작업 | 명령/점검 | 기대 결과 |
| --- | --- | --- |
| 멤버 상태 | `etcdctl endpoint health` | 모든 엔드포인트 정상 |
| 레지스트리 목록 | `etcdctl get --prefix /namros/gateways` | 최신 임대가 붙은 게이트웨이 키 |
| 백업 | `etcdctl snapshot save` | 스냅샷 산출물을 노드 외부에 저장 |
| 멤버 교체 | 실패 멤버 제거, 교체 멤버 추가, 쿼럼 검증 | 클러스터가 정상 쿼럼으로 복귀 |

## 장애 모드

| 신호 | 가능성 높은 원인 | 대응 |
| --- | --- | --- |
| peer 포트 bind 오류 | 포트가 이미 사용 중 | 다른 client/peer 포트와 data-dir 선택 |
| 레지스트리 키 누락 | 게이트웨이 미시작, 잘못된 루트, 임대 만료 | 게이트웨이 설정과 엔드포인트 집합 검증 |
| 오래된 게이트웨이 키 | 임대 갱신 경로 고장 | heartbeat 로그와 임대 TTL 점검 |
| 쿼럼 손실 | 멤버 장애가 너무 많음 | active-active 상태에 의존하기 전에 쿼럼 복구 |

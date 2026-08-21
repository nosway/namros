Community 프로덕션 운영

# TiKV HA 설치 및 운영 가이드

## 역할

<span class="badge">Community</span> TiKV는 active-active NAMROS 배포의 분산 정본 메타데이터 백엔드입니다. 게이트웨이 프로세스는 로컬 캐시를 정본 오브젝트 상태로 취급하면 안 됩니다.

## PD/TiKV 토폴로지

| 컴포넌트 | 역할 | 운영 참고 |
| --- | --- | --- |
| PD | 클러스터 메타데이터, 스케줄링, timestamp oracle | HA를 위해 홀수 쿼럼으로 운영 |
| TiKV | 분산 키-값 데이터 스토리지 | 메타데이터 쓰기/읽기 워크로드에 맞게 용량 산정 |
| NAMROS 게이트웨이 | 트랜잭션 메타데이터 클라이언트 | 배포별 키스페이스/접두사 사용 |

TiUP 기반 부트스트랩과 호스트 하드닝은 해당 환경에서 사용하는 TiKV 운영 표준을 따라야 합니다. NAMROS 고유 관심사는 엔드포인트 선택, 키스페이스 이름, 스모크 검증입니다.

## 게이트웨이 설정

```sh
namros-gateway \
  -metadata-backend tikv \
  -tikv-pd-endpoints 127.0.0.1:2379 \
  -tikv-keyspace namros \
  -storage-backend local \
  -storage-path .namros/segments
```

dev/stage/prod/lab에는 서로 다른 키스페이스를 사용합니다. 런북이 명시적으로 지시하지 않는 한 릴리스 증빙에 테스트 키스페이스를 재사용하지 마세요.

## 메타데이터 백업/복구 연동

NAMROS 메타데이터 내보내기/가져오기는 제품 메타데이터 수준에서 동작합니다. TiKV 스냅샷은 기반 클러스터를 보호하고, `namros-admin metadata-export`는 제어된 복구 워크플로를 위해 제품 컬렉션과 감사 해시를 보존합니다.

| 백업 유형 | 목적 | 복구 범위 |
| --- | --- | --- |
| TiKV 스냅샷/백업 | 클러스터 수준 재해 복구 | 원시 KV 상태 |
| NAMROS 메타데이터 내보내기 | 제품 수준 마이그레이션/사전 점검/복구 | NAMROS 메타데이터 컬렉션 |

## 스모크 절차

```sh
make smoke-active-active
make compat-sbs-cluster-ec
```

`smoke-active-active`는 게이트웨이 간 읽기/쓰기와 장애 전환 동작을 검증합니다. `compat-sbs-cluster-ec`는 추가로 SBS service/data 엔드포인트와 호환 샤드 경로가 준비된 SBS 볼륨을 요구합니다.

## 문제 해결

| 신호 | 가능성 높은 원인 | 조치 |
| --- | --- | --- |
| PD 연결 거부 | 잘못된 엔드포인트 또는 PD 미실행 | `-tikv-pd-endpoints`와 클러스터 상태 검증 |
| 메타데이터 레코드 없음 | 잘못된 키스페이스 또는 볼륨/제어 평면 상태 누락 | 키스페이스를 검증하고 필요한 SBS 상태 준비 |
| 트랜잭션 재시도 압력 | 핫 키 또는 큰 트랜잭션 | 메타데이터 작업 크기와 재시도 메트릭 점검 |
| active-active 불일치 | 캐시 무효화 또는 공유 스토리지 문제 | 게이트웨이 관리자 상태와 read-through 캐시 동작 비교 |

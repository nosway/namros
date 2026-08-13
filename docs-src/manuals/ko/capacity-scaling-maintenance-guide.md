운영 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS 용량 확장 및 유지보수 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition gateway scale-out, TiKV 운영, SBS 복제 volume-pool 운영과 Enterprise edition only EC healing 및 dedupe repair 계약을 함께 다룹니다. 직접 단일 SBS volume 연결은 개발 또는 호환성 검증 경로입니다. Enterprise-only repair command는 공개 Community 빌드에서 edition-gated 상태입니다.

</div>

<div class="summary" markdown="1">

이 가이드는 무상태 게이트웨이의 무중단 스케일아웃, TiKV 메타데이터 클러스터의 decommission, SBS 복제 volume-pool 유지보수를 설명합니다. Erasure Coding(EC) healing과 dedupe repair는 Enterprise 계약입니다.

</div>

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| Gateway scale-out | stateless gateway, TiKV metadata, etcd registry flag 조합으로 지원합니다. | 동일한 기본 패턴을 Enterprise 배포에도 적용할 수 있습니다. |
| SBS replicated storage | Community production SBS-backed 경로는 metadata registry volume-pool id와 2개 이상의 복제 member를 가진 `sbs-cluster`를 사용합니다. | Enterprise 배포는 EC/classroute placement를 추가할 수 있습니다. |
| EC healing 및 dedupe repair | `dedupe-scrub`과 `dedupe-repair`는 예약된 flat command 이름이며 Community 빌드에서는 Enterprise-required 응답을 반환합니다. | private Enterprise overlay가 EC healing, dedupe repair, rebalancing 구현을 소유합니다. |

## 운영 절차 범위

| 운영 시나리오 | 구현 요구 사항 | 배포 범위 |
| --- | --- | --- |
| 게이트웨이 스케일아웃 | L4/L7 로드밸런서 뒤에 새 무상태 게이트웨이를 병렬 배치하고 etcd에 자동 등록. | <span class="badge">Community</span> |
| 메타데이터 용량 증설 | TiKV 및 PD 노드를 추가하고 Raft 리더와 데이터 영역을 안전하게 rebalance. | <span class="badge">Community</span> |
| SBS 복제 volume 검사 | SBS 복제 storage의 volume-pool member 무결성을 확인하고 SBS 유지보수 workflow로 손상/손실 복제본을 재생성. | <span class="badge">Community</span> |
| SBS EC 자가 복구 | 손실되거나 손상된 Erasure Coding 데이터/패리티 샤드 식별 및 백그라운드 healing. | <span class="badge enterprise">Enterprise edition only</span> |

## 버킷, 오브젝트, Volume Pool 관계와 용량 확장

NAMROS 버킷은 특정 SBS volume에 고정되지 않습니다. 버킷 메타데이터는 TiKV 같은 NAMROS metadata repository가 관리하고, 버킷 이름, bucket id, versioning, lifecycle, policy, quota, Object Lock 설정 같은 S3 namespace 상태를 보관합니다. 버킷 안의 오브젝트 목록도 SBS volume을 훑어서 만들지 않고, metadata transaction으로 갱신되는 object head와 list index를 기준으로 유지합니다.

오브젝트 payload는 `ObjectVersion`의 manifest에 들어 있는 `SegmentRef`를 통해 SBS 백엔드로 연결됩니다. 논리 레벨에서는 bucket/key/version, storage class, volume-pool id, segment ref가 보이고, 물리 레벨에서는 SBS 복제 volume의 chunk/span 또는 <span class="badge enterprise">Enterprise edition only</span> EC storage class의 stripe/shard 배치로 매핑됩니다. 따라서 하나의 버킷은 여러 SBS volume pool member에 걸쳐 저장될 수 있고, 하나의 SBS volume도 여러 버킷의 segment를 함께 담을 수 있습니다.

| 질문 | 답 |
| --- | --- |
| 버킷의 저장 공간을 늘리려면? | 버킷 자체에 volume id를 추가하지 않습니다. 해당 버킷의 새 오브젝트 쓰기가 사용하는 storage class 또는 gateway policy의 volume pool 용량을 늘립니다. |
| Volume pool 용량 확장은 어떻게 하나? | SBS/NAMRBD 운영 절차로 물리 디스크, 노드, replicated volume 또는 EC-capable volume/member를 준비한 뒤, NAMROS metadata registry에 새 pool member를 active 상태로 등록합니다. |
| 확장 후 실제 새 오브젝트 저장 공간은 언제 늘어나나? | pool registry update가 commit되고, gateway가 새 pool generation을 refresh 또는 재시작으로 관찰하고, 새 member가 active/healthy write admission 대상이 된 뒤부터 새 PUT/UploadPart가 확장된 공간을 사용할 수 있습니다. |
| 기존 오브젝트도 자동으로 이동하나? | 아니요. 기존 object version은 commit 당시 기록된 SegmentRef placement를 계속 사용합니다. 기존 데이터를 새 member로 옮기는 작업은 drain, rebalance, lifecycle transition 같은 별도 operation으로 수행해야 합니다. |

1. **SBS 물리 용량 준비:** SBS 운영 절차에 따라 디스크/노드/volume/member를 추가하고 복제 또는 EC 구성이 정상인지 확인합니다.
2. **NAMROS volume pool 등록:** `namros-admin volume-pool-put` 같은 metadata registry 경로로 새 member의 volume id, admin/data endpoint, state, weight, storage class를 기록합니다.
3. **Gateway refresh 확인:** 모든 gateway가 새 pool generation을 관찰했는지 확인합니다. refresh watch가 준비되지 않은 배포에서는 rolling restart로 같은 효과를 냅니다.
4. **쓰기 검증:** 새 오브젝트 PUT/GET/LIST, cross-gateway read, gateway failover smoke를 실행해 새 capacity가 실제 object write path에서 쓰이는지 확인합니다.
5. **기존 데이터 재배치 판단:** capacity pressure를 낮추려면 별도의 drain/rebalance 계획을 세우고, 기존 SegmentRef를 임의로 바꾸지 않습니다.

## 무상태 게이트웨이 무중단 확장

NAMROS 게이트웨이는 영속적인 정본 상태(authoritative state)를 보유하지 않으므로, 부하 증가 시 새 인스턴스를 추가해 스케일아웃할 수 있습니다.

```sh
# etcd coordination 클러스터에 새 게이트웨이를 등록하며 시작
namros-gateway \
  -listen 192.168.10.12:9000 \
  -deployment-profile production \
  -coordination-backend etcd \
  -etcd-endpoints 192.168.10.5:12379 \
  -gateway-registry-prefix /namros/gateways \
  -metadata-backend tikv \
  -tikv-pd-endpoints 192.168.10.6:2379 \
  -tikv-keyspace namros \
  -storage-backend sbs-cluster \
  -sbs-volume-pool-id standard-repl \
  -sbs-writer-group-id object-writers \
  -sbs-session-id gw-new-boot-1 \
  -sbs-volume-epoch 1 \
  -gc-candidate-queue metadata
```

로드밸런서는 `/debug/admin/status` 헬스 체크 엔드포인트를 주기적으로 스캔하여 정상 등록을 확인한 뒤 트래픽을 가중치 분배합니다.

## 안전한 노드 교체/점검 절차

특정 게이트웨이나 스토리지 노드를 점검하기 전에 데이터 안정성을 유지하며 트래픽에서 제외하는 절차입니다.

1. **모니터링 신호 확인:** active-active 게이트웨이 정합성 및 TiKV 디스크 점유율을 점검합니다.
2. **트래픽 드레인(Drain):** 대상 게이트웨이를 로드밸런서 타깃 그룹에서 탈퇴시키고, 진행 중인 S3 멀티파트 업로드 및 Range GET이 완전히 닫히는 시간(Grace Period, 권장 180초) 동안 대기합니다.
3. **유지보수 작업 수행:** 하드웨어 점검, OS 패치 및 물리 드라이브 점검을 수행합니다.
4. **검증 및 재투입:** 장비 재시작 후 S3 클라이언트 스모크 명령으로 기본 I/O를 검증하고 트래픽을 다시 연결합니다.

## EC 샤드 자가 진단 및 복구 절차

Erasure Coding(EC_4_2) 샤드 중 하나 이상의 드라이브 고장이 감지되면, 손실된 샤드를 재생성하여 데이터 유실을 방지합니다. 이 섹션은 Enterprise 계약이며, 아래 `namros-admin dedupe-*` 명령은 공개 Community 빌드에서 edition-gated 상태입니다.

### 1. 손상된 데이터/패리티 샤드 및 중복제거 참조 상태 스캔

```sh
# 전체 버킷 또는 특정 세그먼트의 무결성 손상 스캔
namros-admin dedupe-scrub -bucket finance-reports
```

### 2. 자가 복구 계획 및 안전 확인

복구 작업 전에 드라이브 상태와 남은 쿼럼(Read Quorum: 최소 4개 이상 샤드 가용)이 복구 가능한 상태인지 시뮬레이션합니다.

```sh
# 읽기 전용 진단 및 복구 영향도 미리보기 리포트 생성
namros-admin dedupe-repair -bucket finance-reports -dry-run
```

### 3. 힐링 및 데이터 재균형(Rebalance) 실행

```sh
# 안전성 검증 후 백그라운드 healing 및 손실 샤드 재생성 실행
namros-admin dedupe-repair -bucket finance-reports -apply
```

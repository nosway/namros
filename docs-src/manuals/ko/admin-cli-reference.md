레퍼런스 매뉴얼 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only</span>

# NAMROS 통합 CLI 명령어 레퍼런스

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition CLI 명령과 Enterprise edition only 예약 명령 이름을 함께 다룹니다. 예약된 Enterprise 명령은 private Enterprise distribution이 구현을 제공하지 않는 한 공개 Community 빌드에서 edition-boundary error를 반환합니다.

</div>

<div class="summary" markdown="1">

이 문서는 현재 `namros-gateway`와 `namros-admin`의 실제 명령 구조를 설명합니다. 관리자 CLI는 `bucket-quota-put`, `metadata-export`처럼 flat command 이름을 사용하며, `replication status` 또는 `kms status` 같은 중첩 명령 그룹을 사용하지 않습니다.

</div>

## 구현 상태

| 영역 | 현재 동작 |
| --- | --- |
| Community 게이트웨이 | S3 호환 API, 로컬/Pebble 메타데이터, TiKV 메타데이터, etcd 게이트웨이 레지스트리, 로컬 스토리지, SBS 기반 Community 스토리지 경로를 실행합니다. |
| Community 관리자 CLI | 메타데이터 상태, scale budget 산정, volume-pool 메타데이터, 버킷 최대 오브젝트 크기 quota, worker/GC 조회, 메타데이터 export/import 검증, IAM simulation helper를 제공합니다. |
| Enterprise-gated 관리자 명령 | 중복 제거, KMS 키 관리, 컴플라이언스 증빙 명령은 flat command 이름으로 예약되어 있으며 Community 빌드에서는 NAMROS Enterprise-required 응답을 반환합니다. |

## 1. namros-gateway

게이트웨이 프로세스는 S3 호환 HTTP 요청을 받습니다. Community production 배포는 `-deployment-profile production`과 함께 TiKV 메타데이터, etcd coordination, SBS 복제 volume pool, SBS writer session fencing을 사용합니다. 로컬/Pebble 경로는 개발 profile입니다.

```sh
namros-gateway \
  -http-listen 0.0.0.0:9000 \
  -deployment-profile production \
  -metadata-backend tikv \
  -tikv-pd-endpoints 192.168.10.6:2379 \
  -tikv-keyspace namros \
  -storage-backend sbs-cluster \
  -sbs-volume-pool-id standard-repl \
  -sbs-writer-group-id object-writers \
  -sbs-session-id gw-a-boot-1 \
  -sbs-volume-epoch 1 \
  -coordination-backend etcd \
  -etcd-endpoints 192.168.10.5:12379 \
  -gateway-registry-prefix /namros/gateways
```

### 핵심 게이트웨이 플래그

| 플래그 | 일반 값 | 설명 |
| --- | --- | --- |
| `-http-listen` | `127.0.0.1:9000` | S3 API 요청을 받을 HTTP 수신 주소. |
| `-deployment-profile` | `dev`, `production` | 검증 profile. Production은 개발 전용 메타데이터, 스토리지, 직접 단일 volume, unfenced shared attachment shortcut을 거부합니다. |
| `-region` | `us-east-1` | SigV4 클라이언트와 호환성 스크립트가 사용하는 리전. |
| `-metadata-backend` | `pebble`, `tikv`, `memory` | 정본 메타데이터 백엔드. |
| `-metadata-path` | `.namros/meta` | 로컬 Community 실행에서 사용하는 Pebble 메타데이터 경로. |
| `-tikv-pd-endpoints` | `host:2379` | `-metadata-backend tikv` 사용 시 필요한 쉼표 구분 PD endpoint 목록. |
| `-tikv-keyspace` | `namros` | TiKV keyspace 이름 또는 v1 key prefix fallback. |
| `-storage-backend` | `local`, `sbs-physical`, `sbs-cluster` | Payload segment backend. Production 배포는 SBS volume-pool id와 함께 `sbs-cluster`를 사용합니다. |
| `-sbs-service-endpoint` | `sbs-service:9443` | SBS 기반 스토리지용 SBS service gRPC endpoint. 환경변수는 `NAMROS_SBS_SERVICE_ENDPOINT`입니다. |
| `-sbs-data-endpoint` | `sbs-data:9460` | chunk 또는 shard IO용 SBS data gRPC endpoint. |
| `-sbs-volume-id` | `18a00001` | `sbs-physical` 또는 `sbs-ec` storage에 사용할 SBS volume id. |
| `-sbs-volume-pool-id` | `standard-repl` | production SBS-backed storage가 사용하는 metadata registry volume pool id. |
| `-sbs-writer-group-id` | `object-writers` | Production session fencing에 필요한 SBS shared logical writer group. |
| `-sbs-session-id` | `gw-a-boot-1` | Gateway process 시작마다 달라야 하는 per-gateway session id. |
| `-sbs-volume-epoch` | `1` | Stale handle과 idempotency replay를 fence하는 volume epoch. |
| `-coordination-backend` | `none`, `etcd` | 게이트웨이 coordination backend. |
| `-etcd-endpoints` | `host:2379` | 게이트웨이 coordination에 사용할 쉼표 구분 etcd endpoint 목록. |
| `-gateway-registry-prefix` | `/namros/gateways` | active gateway lease를 저장할 etcd key prefix. |
| `-gateway-data-budget-bytes` | `1073741824` | 선택적 aggregate in-flight data-path byte budget입니다. 초과한 PUT, UploadPart, CopyObject, UploadPartCopy, GET 요청은 S3 `SlowDown`을 반환합니다. |
| `-gateway-data-budget-max-requests` | `256` | 선택적 동시 data-path request budget입니다. `0`은 이 제한을 비활성화합니다. |
| `-gateway-data-budget-unknown-bytes` | `8388608` | Chunked 또는 크기 미확정 request payload에 예약할 byte 수입니다. |

## 2. namros-admin 공통 메타데이터 플래그

대부분의 메타데이터 기반 관리자 명령은 같은 backend 선택 플래그를 받습니다. NAMROS 메타데이터를 조회하거나 변경하는 명령에는 아래 플래그를 함께 사용합니다.

| 플래그 | 사용 시점 |
| --- | --- |
| `-metadata-backend pebble -metadata-path .namros/meta` | 로컬 Community 메타데이터를 디스크에 저장한 경우. |
| `-metadata-backend tikv -tikv-pd-endpoints host:2379 -tikv-keyspace namros` | TiKV에 분산 메타데이터를 저장한 경우. |
| `-tikv-api-version`, `-tikv-timeout`, `-tikv-tls-ca`, `-tikv-tls-cert`, `-tikv-tls-key` | TiKV API, timeout, TLS 제어가 필요한 경우. |

## 3. Community 관리자 명령

| 명령 | 목적 | 예제 |
| --- | --- | --- |
| `status` | 메타데이터 상태, 최근 operation counter, production readiness posture를 요약합니다. | `namros-admin status -metadata-backend tikv -tikv-pd-endpoints host:2379 -deployment-profile production -storage-backend sbs-cluster -sbs-volume-pool-id standard-repl -sbs-writer-group-id object-writers -sbs-session-id gw-a-boot-1 -sbs-volume-epoch 1 -coordination-backend etcd -etcd-endpoints host:2379 -gc-candidate-queue metadata` |
| `metadata-scale-budget` | multipart object, protected ref, GC candidate의 metadata value와 transaction 크기를 추정합니다. | `namros-admin metadata-scale-budget -part-count 10000` |
| `volume-pool-put` | SBS 기반 게이트웨이가 사용할 volume-pool metadata를 기록합니다. | `namros-admin volume-pool-put -pool-id replicated-rf3 -member volume_id=18a00001,service_endpoint=sbs-service-a:9443,data_endpoint=sbs-data-a:9460,state=active` |
| `bucket-quota-put` | Community 버킷 최대 오브젝트 크기 quota를 설정합니다. | `namros-admin bucket-quota-put -bucket photos -max-object-size-bytes 1073741824` |
| `bucket-quota-get` | 버킷 최대 오브젝트 크기 quota를 조회합니다. | `namros-admin bucket-quota-get -bucket photos` |
| `bucket-quota-delete` | 버킷 최대 오브젝트 크기 quota를 삭제합니다. | `namros-admin bucket-quota-delete -bucket photos` |
| `tenant-quota-put` | tenant quota metadata record를 설정합니다. | `namros-admin tenant-quota-put -tenant-id finance -max-bytes 1099511627776 -max-objects 1000000 -max-active-uploads 256` |
| `tenant-quota-get` | tenant quota metadata record를 조회합니다. | `namros-admin tenant-quota-get -tenant-id finance` |
| `tenant-quota-delete` | tenant quota metadata record를 삭제합니다. | `namros-admin tenant-quota-delete -tenant-id finance` |
| `worker-operations` | worker operation 기록을 kind, shard, status 기준으로 조회합니다. | `namros-admin worker-operations -worker-kind gc -limit 20` |
| `gc-candidates` | 메타데이터의 orphan GC candidate records를 조회합니다. | `namros-admin gc-candidates -limit 20` |
| `gc-candidate-seed-object` | 오브젝트 버전을 detach하고 해당 segment reference를 GC candidate로 enqueue합니다. | `namros-admin gc-candidate-seed-object -bucket photos -key stale.bin` |
| `metadata-export` | 백업 및 audit workflow용 product metadata JSON을 export합니다. | `namros-admin metadata-export -limit 1000` |
| `metadata-import` | export JSON을 검증하거나 명시적인 target flag와 함께 import를 plan/apply합니다. | `namros-admin metadata-import -input export.json -dry-run` |
| `iam-principal-inspect` | CLI flag로 구성된 IAM principal을 정규화해 출력합니다. | `namros-admin iam-principal-inspect -tenant-id root -access-key-id namros -root` |
| `iam-policy-simulate` | principal/action/resource 조합에 대해 inline 또는 file 기반 S3/IAM policy를 평가합니다. | `namros-admin iam-policy-simulate -action s3:GetObject -resource arn:aws:s3:::photos/a.jpg -policy-file policy.json` |
| `iam-mapping-validate` | 외부 IAM mapping specification JSON 파일을 검증합니다. | `namros-admin iam-mapping-validate -input mapping.json` |

정적 volume-pool member specification에는 `service_endpoint`를 사용합니다. `namros-sbs-exporter`와 `namros-ops-report`는 복수형 `-sbs-service-endpoints` 플래그를 사용합니다.

## 4. Enterprise-gated 명령

Community 빌드는 Enterprise-only 동작을 활성화하는 unlock switch를 의도적으로 노출하지 않습니다. 아래 flat command 이름은 CLI 경계에서 예약되어 있으며, private Enterprise build가 구현을 제공하지 않는 한 Enterprise-required 응답을 반환합니다.

| 기능 | 예약 명령 이름 |
| --- | --- |
| 중복 제거와 복구 | `dedupe-plan`, `dedupe-ack`, `dedupe-ops`, `dedupe-repair`, `dedupe-scrub` |
| SSE-KMS | `kms-key-put`, `kms-key-list` |
| 컴플라이언스 증빙 | `compliance-evidence`, `compliance-profile-plan`, `compliance-policy-simulate` |

## 5. 복사 가능한 예제

### 로컬 상태 점검

```sh
namros-admin status \
  -metadata-backend pebble \
  -metadata-path .namros/meta
```

### TiKV 메타데이터 export

```sh
namros-admin metadata-export \
  -metadata-backend tikv \
  -tikv-pd-endpoints 192.168.10.6:2379 \
  -tikv-keyspace namros \
  -limit 1000
```

### Community Enterprise gate 검증

```sh
namros-admin kms-key-list
```

Community 기대 결과: 명령은 표준 NAMROS Enterprise-required 응답으로 실패하며, public build가 KMS unlock path를 노출하지 않는다는 점을 확인합니다.

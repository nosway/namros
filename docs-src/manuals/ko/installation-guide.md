설치

# NAMROS 설치 가이드

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition 설치 경로와 Enterprise edition only 의존성 메모를 함께 다룹니다. Enterprise-only 의존성 설명은 공개 Community 빌드에서 해당 기능을 활성화할 수 있다는 뜻이 아닙니다.

</div>

## 범위

이 가이드는 Community 기준 로컬 NAMROS 게이트웨이를 실행하고 S3 사용자 공간 호환성 스모크 테스트를 수행하는 절차를 설명합니다. 프로덕션 Community 구성은 TiKV/etcd/SBS 복제 의존성을 사용할 수 있고, Enterprise 구성은 SBS EC/KMS/컴플라이언스/중복 제거 의존성을 준비한 뒤 별도 런북의 순서를 따릅니다.

<div class="note" markdown="1">

모든 명령은 저장소 루트에서 실행하는 것을 전제로 합니다. 예: `~/src/namros`.

</div>

## 지원 플랫폼

| 플랫폼 | 지원 용도 | 주의 사항 |
| --- | --- | --- |
| macOS | 빌드, 로컬 게이트웨이, AWS CLI/mc/rclone 스모크 | s3fs-fuse 마운트 검증의 주 경로는 아닙니다. |
| Linux 사용자 공간 | 빌드, 로컬 게이트웨이, AWS CLI/mc/rclone 스모크 | 일상적인 호환성 점검에 사용합니다. |
| Linux FUSE 호스트 | s3fs-fuse 호환성 | 마운트 작업에는 FUSE 패키지와 root 권한이 필요합니다. |
| Community HA 실험실 | active-active, TiKV, etcd, SBS 복제 | 분산 의존성을 미리 준비해야 합니다. |
| Enterprise 실험실 | SBS EC, 중복 제거, WORM/KMS/컴플라이언스 | Enterprise 의존성을 미리 준비해야 합니다. |

## 필수 도구

| 도구 | 용도 | 검증 명령 |
| --- | --- | --- |
| Go | `namros-gateway`, `namros-admin` 빌드와 테스트 | `go version` |
| AWS CLI | S3 API 호환성 스모크 | `aws --version` |
| MinIO client | 복사/상태/목록 호환성 | `mc --version` |
| rclone | 복사/이동/삭제 호환성 | `rclone version` |
| jq | 스모크 출력 검증 | `jq --version` |
| s3fs-fuse | Linux FUSE 호환성 | `s3fs --version` |

클라이언트 설정과 스모크 워크플로 세부사항은 [S3 클라이언트 호환성 가이드](s3-client-compatibility-guide.md)에서 관리합니다.

## 프로덕션 준비도 체크리스트

`-deployment-profile production`은 production topology에서만 사용합니다. 이 topology는 TiKV 메타데이터, etcd 게이트웨이 coordination, 2개 이상의 게이트웨이 인스턴스, SBS 복제 volume pool, SBS writer session fencing을 요구합니다. 메모리 메타데이터, Pebble 메타데이터, 로컬 세그먼트 저장소, 직접 단일 SBS volume 또는 unfenced shared attachment shortcut은 개발 또는 lab 모드입니다.

| 영역 | 점검 항목 | 참고 가이드 |
| --- | --- | --- |
| 보안 | TLS 종단, 관리자 접근 경계, 비밀값 마스킹, 공개 Enterprise 잠금 해제 경로 부재. | [에디션](../architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| 메타데이터 | TiKV/PD 엔드포인트, 키스페이스, 백업/복구 스모크, 트랜잭션 실패 동작. | [TiKV 가이드](tikv-ha-cluster-install-operations-guide.md) |
| 코디네이션 | etcd client/peer URL, 임대 TTL, 레지스트리 루트, 멤버 교체 런북. | [etcd 가이드](etcd-ha-cluster-install-operations-guide.md) |
| 스토리지 | Local/SBS 복제 준비도와 Enterprise EC 경로 사전 요건. | [용량 가이드](capacity-scaling-maintenance-guide.md) |
| Identity/KMS | Enterprise 기능 사용 시 IAM 제공자와 KMS 제공자 준비도. | [IAM](iam-integration-guide.md), [KMS](kms-encryption-guide.md) |
| 릴리스 | 호환성, active-active, 백업/복구, 소스 내보내기, production-scale gate, 릴리스 준비도 리포트. | [업그레이드/릴리스](upgrade-release-operations-guide.md) |

### 1. OS 커널 파라미터 튜닝 (사전 준비)

프로덕션 규모의 무상태 분산 게이트웨이를 안정적으로 운영하려면 OS 커널 파라미터 튜닝이 필요합니다. 아래 설정은 시스템 시작 시 적용하는 것을 권장합니다.

**시스템 제한 해제 (/etc/security/limits.conf):**

```text
# 열린 파일 수 및 프로세스 제한 상향
namros       soft    nofile          65536
namros       hard    nofile          131072
namros       soft    nproc           32768
namros       hard    nproc           65536
```

**커널 가상 메모리 및 소켓 튜닝 (/etc/sysctl.conf):**

```text
# TCP 포트 소모 및 TIME_WAIT 대기열 재활용 최적화
net.ipv4.tcp_tw_reuse = 1
net.ipv4.ip_local_port_range = 10240 65000

# 최대 백로그 및 소켓 대기 큐 확장 (패킷 유실 방지)
net.core.somaxconn = 8192
net.core.netdev_max_backlog = 10000
net.ipv4.tcp_max_syn_backlog = 4096

# 가상메모리 오버커밋 허용 설정
vm.overcommit_memory = 1
```

### 2. 스토리지 IOPS와 DB 전용 인프라

- **TiKV 전용 디스크 분리:** TiKV 메타데이터 클러스터의 디스크는 게이트웨이 로컬 세그먼트 및 일반 I/O와 완전히 분리된 NVMe SSD(최소 랜덤 쓰기 10,000 IOPS 이상)에 배치하여 디스크 쓰기 정체로 인한 S3 트랜잭션 타임아웃을 방지해야 합니다.
- **etcd 전용 스토리지:** etcd Raft 로그 기록 성능 유지를 위해 지연 시간이 극히 낮은 스토리지를 할당하고, 필요 시 fsync 지연 시간을 별도로 프로파일링해야 합니다.

### 3. TLS 및 FIPS 컴플라이언스 상태

프로덕션 게이트웨이는 프런트 프록시(Nginx/HAProxy)에서 TLS를 종료하거나, 게이트웨이 자체에 자체 서명이 아닌 신뢰 가능한 CA 체인과 암호 스위트 규격을 명시하여 운영해야 합니다.

## 빌드

```sh
go build ./cmd/namros-gateway
go build ./cmd/namros-admin
go test ./...
```

`-o` 없이 `go build ./cmd/...`를 사용하면 빌드 산출물이 현재 작업 디렉터리에 생성됩니다. 로컬 워크플로에서 바이너리가 필요하다면 `bin/namros-gateway`처럼 명시적인 출력 경로를 사용하는 편이 반복 실행에 좋습니다.

## Local Community 빠른 시작

GitHub 개발자가 새 clone에서 바로 확인하는 흐름입니다.

```sh
git clone https://github.com/nosway/namros.git
cd namros
make test-community
make build-community
make run-dev
```

기본 개발 타깃은 `127.0.0.1:9000`에서 `namros-gateway`를 시작하며, 리전은 `us-east-1`, Pebble 메타데이터 경로는 `.namros/meta`, 로컬 세그먼트 저장소는 `.namros/segments`를 사용합니다.

이 타깃은 의도적으로 `dev` profile입니다. S3 클라이언트 점검과 로컬 개발에는 유용하지만, 대용량 또는 production 배포 형태가 아닙니다.

정상 종료는 게이트웨이가 실행 중인 터미널에서 `Ctrl-C`로 수행합니다. 운영자가 경로를 삭제하지 않는 한 재시작 시 동일한 Pebble 및 세그먼트 경로를 사용합니다.

게이트웨이를 실행한 상태에서 다른 셸로 S3 왕복을 검증합니다.

```sh
export NAMROS_ENDPOINT=http://127.0.0.1:9000
export AWS_ACCESS_KEY_ID=namros
export AWS_SECRET_ACCESS_KEY=namros-secret
export AWS_DEFAULT_REGION=us-east-1

aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-bucket --bucket quickstart
printf 'hello namros\n' > /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object --bucket quickstart --key hello.txt --body /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object --bucket quickstart --key hello.txt /tmp/namros-readback.txt
```

## Community 컨테이너 배포

확정된 Docker/Compose 프로파일, 이미지 정책, 비밀값 처리, readiness 계약과 구현 상태는 [Community 컨테이너 배포 가이드](container-deployment-guide.md)를 참고합니다. Helm은 후속 단계에서 별도 문서로 제공합니다.

공개 gateway + SBS backend 빠른 시작은 다음 명령으로 실행합니다.

```sh
make container-sbs-quickstart-smoke
```

이 명령은 `packaging/docker/compose.sbs-quickstart.yml`을 통해 gateway 1개,
SBS service 1개, SBS data node 2개, PD/TiKV 테스트 메타데이터를 시작합니다.
S3 endpoint는 `http://127.0.0.1:9002`입니다.

production-shaped Kubernetes/kind 시나리오는 다음 명령으로 확인합니다.

```sh
make k8s-production-render
make kind-production-deploy
# 기존 cluster와 로드된 image를 유지한 채 중지 및 재실행
make kind-production-stop
make kind-production-start
# 완료 후 kind cluster와 임시 테스트 상태 삭제
make kind-production-down
```

기본 설정 파일은 `packaging/k8s/production-kind.env`이며 gateway 2개,
SBS service 2개, SBS data node 5개, embedded TiKV 1개를 렌더합니다.

## Community 게이트웨이 플래그

| 플래그 | 일반 값 | 의미 |
| --- | --- | --- |
| `-listen` | `127.0.0.1:9000` | HTTP 수신 주소. |
| `-deployment-profile` | `dev`, `production` | 검증 profile. Production은 명시적 lab override가 없으면 memory/Pebble/local/단일 volume 및 unfenced shared attachment shortcut을 거부합니다. |
| `-region` | `us-east-1` | S3 호환 클라이언트가 사용하는 리전. |
| `-metadata-backend` | `pebble` | 로컬 Community 실행의 정본 메타데이터 백엔드. |
| `-metadata-path` | `.namros/meta` | Pebble 메타데이터 경로. |
| `-storage-backend` | `local` | 페이로드 세그먼트 백엔드. |
| `-storage-path` | `.namros/segments` | 로컬 세그먼트 디렉터리. |

## 검증 순서

1. Community 테스트와 edition-boundary 검사를 실행합니다: `make test-community`.
2. 로컬 게이트웨이를 시작합니다: `make run-dev`.
3. 다른 셸에서 컨테이너 스모크를 실행합니다: `make container-local-smoke`.
4. 공개 gateway를 SBS backend로 검증하려면 `make container-sbs-quickstart-smoke`를 실행합니다.
5. Production-shaped Kubernetes config를 렌더합니다: `make k8s-production-render`.
6. kind 평가에는 `make kind-production-deploy`를 실행합니다.
7. 공개 AWS CLI/mc/rclone 검증은 `make compat-public-s3`로 엄격하게 실행합니다.
8. 개발자 장비에서 컨테이너를 쓰지 않는 user-space client 범위는 `make compat-user-space`로 설치된 도구만 검증합니다.
9. Production-scale readiness를 주장하기 전 `make production-scale-check`를 실행하고 skip된 외부 smoke gate를 검토합니다.
10. FUSE 범위는 FUSE 마운트 권한이 있는 Linux 호스트에서 검증합니다.

```sh
make test-community
make container-local-smoke
make container-sbs-quickstart-smoke
make k8s-production-render
```

기대 결과는 각 스모크 테스트가 통과 메시지를 출력하는 것입니다. 실패 출력에는 보존 옵션이 활성화된 경우 클라이언트 이름, 버킷 이름, 엔드포인트, 임시 디렉터리가 포함되어야 합니다.

## 분산 및 Enterprise 의존성

<span class="badge">Community</span> Active-active 게이트웨이는 TiKV 메타데이터, etcd coordination, SBS 복제 스토리지를 사용합니다. <span class="badge enterprise">Enterprise edition only</span>는 SBS EC, KMS, WORM, 중복 제거, 컴플라이언스 서비스를 추가하며 Community 빌드는 이를 활성화하는 unlock switch를 노출해서는 안 됩니다.

```sh
namros-gateway \
  -deployment-profile production \
  -metadata-backend tikv \
  -tikv-pd-endpoints pd-a:2379,pd-b:2379,pd-c:2379 \
  -storage-backend sbs-cluster \
  -sbs-volume-pool-id standard-repl \
  -sbs-writer-group-id object-writers \
  -sbs-session-id gw-a-boot-1 \
  -sbs-volume-epoch 1 \
  -coordination-backend etcd \
  -etcd-endpoints etcd-a:2379,etcd-b:2379,etcd-c:2379 \
  -gc-candidate-queue metadata
```

| 의존성 | 역할 | 가이드 |
| --- | --- | --- |
| etcd | 게이트웨이 레지스트리와 상태 임대 | [etcd HA 가이드](etcd-ha-cluster-install-operations-guide.md) |
| TiKV/PD | 분산 정본 메타데이터 | [TiKV HA 가이드](tikv-ha-cluster-install-operations-guide.md) |
| SBS service/data | Community 복제 물리 스토리지, Enterprise EC 스토리지 | [컨테이너 배포](container-deployment-guide.md) |
| KMS/컴플라이언스 서비스 | 키 상태와 증빙 워크플로 | [MCP 운영 가이드](mcp-operations-guide.md) |

## 설치 후 문제 해결

| 신호 | 가능성 높은 원인 | 조치 |
| --- | --- | --- |
| 9000 포트가 이미 사용 중 | 기존 게이트웨이 또는 다른 서비스 | 프로세스를 중지하거나 다른 `-listen` 주소로 게이트웨이를 실행합니다. |
| AWS CLI 메타데이터 검증 실패 | 오래된 게이트웨이 바이너리 또는 메타데이터 동작 불일치 | 다시 빌드하고 게이트웨이를 재시작한 뒤 클라이언트 스모크를 재실행합니다. |
| rclone 크기 불일치 | 페이로드 쓰기/읽기 경로 문제 | 임시 디렉터리를 보존하고 게이트웨이 로그와 오브젝트 HEAD를 점검합니다. |
| s3fs 마운트 실패 | FUSE 권한 또는 Linux 패키지 누락 | Linux 호스트에서 실행하고 필요한 FUSE/xattr 패키지를 설치합니다. |

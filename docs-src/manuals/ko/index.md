오브젝트 스토리지 제품 문서

# NAMROS

<div class="note" markdown="1">

**기능 상태.** 이 매뉴얼은 실행 가능한 오픈소스 Community 플랫폼을 먼저
설명합니다. <span class="badge enterprise">Enterprise 개발 중</span> 표시는
NAMROS Enterprise에서 구현 또는 검증 중인 기능을 뜻합니다. 계획/명세 단계
표시는 현재 제품 동작이 아닙니다.

</div>

<div class="summary" markdown="1">

NAMROS는 Network Attached Multipath Resilient Object Storage의 약자이며 [nae-muh-ross]로 발음합니다.

NAMROS는 오픈소스 S3 호환 오브젝트 스토리지 플랫폼입니다. 공개 NAMROS
Community 배포판은 일반 S3 오브젝트 워크플로, 외부 클라이언트 호환성,
active-active 게이트웨이, TiKV 메타데이터, etcd coordination, SBS 복제
스토리지, lifecycle/GC 기반과 읽기 전용 운영 화면을 포함합니다.

</div>

![NAMROS 플랫폼 개요](../architecture-manual/assets/diagrams/platform-overview.svg)

## 제품 포지셔닝

NAMROS는 오브젝트 스토리지 제품입니다. `namros-gateway`를 통해 S3 호환 요청을 받고, 네임스페이스 상태는 메타데이터 백엔드에 저장하며, 페이로드 바이트는 세그먼트 스토리지 백엔드에 저장합니다. 게이트웨이 프로세스는 정본 오브젝트 상태를 로컬에 보유하지 않도록 설계되어 있습니다.

NAMROS는 NAMRBD가 아닙니다. NAMRBD는 네트워크 연결 블록 디바이스 제품입니다. NAMROS는 Enterprise 물리 스토리지에 SBS/NAMRBD 기반 요소를 재사용할 수 있지만, 사용자는 S3 클라이언트와 오브젝트 스토리지 의미론을 통해 NAMROS와 상호작용합니다.

## 지원 배포 형태

| 형태 | 목적 | 주요 의존성 | 에디션 |
| --- | --- | --- | --- |
| Local Community | 개발, S3 API 검증, 사용자 공간 호환성 스모크 | 단일 `namros-gateway`, Pebble 또는 메모리 메타데이터, 로컬 세그먼트 저장소 | <span class="badge">Community</span> |
| 호환성 실험실 | AWS CLI, MinIO client, rclone, s3fs-fuse 검증 | 로컬 게이트웨이와 클라이언트 도구, 필요 시 Linux FUSE 호스트 | <span class="badge">Community</span> |
| Active-active 메타데이터 실험실 | 다중 게이트웨이 가용성과 캐시 정확성 | TiKV/PD, etcd, 공유 세그먼트 경로 | <span class="badge">Community</span> |
| SBS EC 개발 실험실 | Enterprise EC multipart와 degraded-read 검증 | TiKV/PD, SBS service/data, 준비된 볼륨과 shard 경로 | <span class="badge enterprise">Enterprise 개발 중</span> |

## 5분 Community 빠른 시작

이 경로는 GitHub에서 공개 Community tree를 처음 확인하는 개발자를 위한 최소 흐름입니다.

```sh
git clone https://github.com/nosway/namros.git
cd namros
make test-community
make build-community
make run-dev
```

게이트웨이를 계속 실행한 상태에서, 두 번째 터미널에서 기본 S3 왕복을 확인합니다.

```sh
export NAMROS_ENDPOINT=http://127.0.0.1:9000
export AWS_ACCESS_KEY_ID=namros
export AWS_SECRET_ACCESS_KEY=namros-secret
export AWS_DEFAULT_REGION=us-east-1

aws --endpoint-url "$NAMROS_ENDPOINT" s3api create-bucket --bucket quickstart
printf 'hello namros\n' > /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api put-object --bucket quickstart --key hello.txt --body /tmp/namros-hello.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api get-object --bucket quickstart --key hello.txt /tmp/namros-readback.txt
aws --endpoint-url "$NAMROS_ENDPOINT" s3api list-objects-v2 --bucket quickstart
```

기대 결과: 마지막 list에 `hello.txt`가 포함되고, `/tmp/namros-readback.txt`가 원본 payload와 일치합니다.

## 현재 플랫폼과 고급 기능 상태

| 기능 | 상태 | 의미 |
| --- | --- | --- |
| S3 API, multipart, versioning, tag, metadata, CORS | <span class="badge">Community 포함</span> | 공개 소스와 호환성 검사에 포함됩니다. |
| TiKV metadata, etcd registry, active-active gateway | <span class="badge">Community 포함</span> | 분산 Community 플랫폼 기반입니다. |
| SBS replicated object storage | <span class="badge">Community 포함</span> | 공개 NAMRBD Community 모듈을 사용합니다. |
| lifecycle/GC, quota record, gateway-local request control | <span class="badge">Community 포함</span> | 공개 기반 기능이며 cluster-wide aggregate 제어는 후속 항목입니다. |
| read-only console, metric, report, MCP diagnostics | <span class="badge">Community 포함</span> | 현재 공개 운영 및 진단 화면입니다. |
| EC, Object Lock/WORM, verified dedupe, SSE-KMS | <span class="badge enterprise">Enterprise 개발 중</span> | 구현 기반을 Enterprise에서 강화·검증하고 있습니다. |
| compliance evidence, external IAM, approved operations | <span class="badge enterprise">Enterprise 개발 중</span> | 부분 기반이 있으며 provider 연동과 운영 강화가 진행 중입니다. |
| cross-region replication/DR, event, inventory/batch | <span class="badge planned">계획/명세 단계</span> | 현재 사용할 수 있는 기능이 아닌 설계 목표입니다. |

기능별 범위는 [고급 기능](advanced-features.md)을 참고하십시오. Community
빌드의 명시적 거부 동작은 Enterprise 기능의 공급 상태를 의미하지 않습니다.

## 역할 맞춤형 시작점

귀하의 역할과 업무 목적에 맞춰 최적의 NAMROS 가이드 문서를 추천합니다.

<div class="cards" markdown="1">

<div class="card" markdown="1">

### 애플리케이션 개발자

애플리케이션 개발자 경로

표준 S3 호환 클라이언트와 SDK(Go, Python, Java)를 사용해 엔드포인트 인증, 버킷 작업, 대용량 멀티파트 업로드 연동 방식을 파악합니다.

[사용자 매뉴얼 열기 →](user-manual.md)

</div>

<div class="card" markdown="1">

### 시스템 관리자

클러스터 인프라 운영자 경로

NAMROS 클러스터 프리플라이트 OS 커널 파라미터 최적화, TiKV/etcd 3중화 클러스터 관리, 자가 치유(Healing) 런북 및 데이터 백업 복구 절차를 학습합니다.

[관리자 가이드 열기 →](admin-guide.md)

</div>

<div class="card" markdown="1">

### 아키텍처 리뷰어

시스템 설계 및 보안 아키텍트 경로

현재 Community의 stateless active-active 구조를 먼저 확인한 뒤 EC, IAM,
KMS Enterprise 개발 계약을 구분하여 검토합니다.

[아키텍처 매뉴얼 열기 →](../architecture-manual/ko/index.md)

</div>

<div class="card" markdown="1">

### 운영 기획자

고급 기능 검토 경로

Enterprise 기능 중 구현 기반을 검증 중인 항목과 계획/명세 단계에 머문 항목을
구분하여 검토합니다.

[고급 기능 열기 →](advanced-features.md)

</div>

</div>

## 현재 검증 상태

HTML 문서 세트는 `make html-docs-check`로 검증합니다. 제품 동작은 배포 형태에 따라 단위 테스트, 소스 경계 검사, 컨테이너 스모크 타깃으로 검증합니다.

| 타깃 | 목적 | 참고 |
| --- | --- | --- |
| `make docs-render-check` | 문서 빌드, 렌더링된 본문, 다이어그램 경로 해석 | `tools/check-docs-render.py` |
| `make check-community-export` | Community identity, Enterprise 경계, 집중 gate 테스트 | [릴리스 경계](../architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| `make container-local-smoke` | 컨테이너 기반 로컬 게이트웨이 스모크 | [컨테이너 배포 가이드](container-deployment-guide.md) |

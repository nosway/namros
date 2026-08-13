오브젝트 스토리지 제품 문서

# NAMROS

<div class="note" markdown="1">

**Edition scope.** 이 페이지는 Community edition 동작과 Enterprise edition only 섹션을 함께 다룹니다. <span class="badge enterprise">Enterprise edition only</span>로 표시된 영역은 명시된 거부 동작을 제외하고 공개 Community 빌드에서 사용할 수 없습니다.

</div>

<div class="summary" markdown="1">

NAMROS는 Network Attached Multipath Resilient Object Storage의 약자이며 [nae-muh-ross]로 발음합니다.

NAMROS는 S3 호환 오브젝트 스토리지 프로젝트입니다. Community 에디션은 일반 S3 오브젝트 워크플로, 외부 클라이언트 호환성, active-active 게이트웨이 운영, TiKV 메타데이터, etcd coordination, SBS 복제 오브젝트 스토리지를 포함합니다. Enterprise 기능은 SBS 기반 EC 스토리지, WORM/Object Lock enforcement, 중복 제거, KMS 상태 관리, 컴플라이언스 증빙, 고급 MCP 보조 운영을 추가합니다.

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
| SBS EC 실험실 | EC 멀티파트 쓰기/읽기 경로 | TiKV/PD, SBS admin/data, 준비된 볼륨과 샤드 경로 | <span class="badge enterprise">Enterprise edition only</span> |

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

## Community 및 Enterprise 요약

| 기능 | Community | Enterprise |
| --- | --- | --- |
| S3 버킷/오브젝트 API | 포함 | 포함 |
| AWS CLI/mc/rclone 스모크 | 포함 | 포함 |
| s3fs-fuse 기본 프로파일 | 호환성 대상 | 호환성 대상 |
| TiKV 메타데이터와 etcd 게이트웨이 레지스트리 | 포함 | 포함 |
| SBS 복제 오브젝트 스토리지 | 포함, 소스 내보내기에는 NAMRBD Community 모듈 패키징 필요 | 포함 |
| SBS EC/classroute | Enterprise 필요 오류 | 사용 가능 |
| WORM/Object Lock enforcement, 중복 제거, KMS, 컴플라이언스 증빙 | Enterprise 필요 오류 | 사용 가능 |
| 웹 콘솔 및 모니터링 | 읽기 전용 대시보드와 리포트 뷰어 | 승인된 작업과 Enterprise 기능 패널 |
| S3 오브젝트 브라우저 연동 | Object Explorer Lite와 외부 S3 browser recipe | 정책 제어 이후 승인된 오브젝트 작업 |
| 사설 overlay와 고급 릴리스 게이트 | 없음 | 사설 배포 |

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

무상태 active-active 게이트웨이 구조, Erasure Coding 4+2 쿼럼 모델, OIDC IAM 접근 정책, HashiCorp Vault SSE-KMS 같은 핵심 설계 사양을 분석합니다.

[아키텍처 매뉴얼 열기 →](../architecture-manual/ko/index.md)

</div>

<div class="card" markdown="1">

### 운영 기획자

비즈니스 기획 및 엔터프라이즈 설계자 경로

Community 빌드에서 활성화되어 있다고 가정하지 않고, Cross-Region replication, event notification, tenant quota/QoS, approved operation에 대한 Enterprise 계약을 검토합니다.

[운영 가이드 열기 →](web-console-monitoring-guide.md)

</div>

</div>

## 현재 검증 상태

HTML 문서 세트는 `make html-docs-check`로 검증합니다. 제품 동작은 배포 형태에 따라 단위 테스트, 소스 경계 검사, 컨테이너 스모크 타깃으로 검증합니다.

| 타깃 | 목적 | 참고 |
| --- | --- | --- |
| `make docs-render-check` | 문서 빌드, 렌더링된 본문, 다이어그램 경로 해석 | `tools/check-docs-render.py` |
| `make check-community-export` | Community identity, Enterprise 경계, 집중 gate 테스트 | [릴리스 경계](../architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| `make container-local-smoke` | 컨테이너 기반 로컬 게이트웨이 스모크 | [컨테이너 배포 가이드](container-deployment-guide.md) |

아키텍처 매뉴얼 <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS 아키텍처 매뉴얼

## 주요 아키텍처 단락

1. [읽기 안내](../chapters/00-reading-guide.md)
2. [제품 개요](../chapters/01-product-overview.md)
3. [S3 API 호환성](../chapters/02-s3-api-and-compatibility.md)
4. [무상태 Active-Active](../chapters/03-gateway-stateless-active-active.md)
5. [TiKV 메타데이터 정본](../chapters/04-metadata-authority.md)
6. [오브젝트와 세그먼트 모델](../chapters/05-object-and-segment-model.md)
7. [멀티파트 쓰기 가시성](../chapters/06-multipart-write-visibility.md)
8. [복제 및 로컬 백엔드](../chapters/07-replicated-and-local-backends.md)
9. [SBS EC 백엔드 Enterprise](../chapters/08-sbs-ec-backend-enterprise.md)
10. [버저닝, 라이프사이클, Object Lock](../chapters/09-versioning-lifecycle-object-lock.md)
11. [중복제거 Enterprise](../chapters/10-dedupe-and-shared-objects-enterprise.md)
12. [보안, 컴플라이언스, 에디션](../chapters/11-security-compliance-and-editions.md)
13. [관측성과 운영](../chapters/12-observability-and-operations.md)
14. [MCP 운영 Provider](../chapters/13-mcp-operations-provider.md)
15. [릴리스와 에디션 경계](../chapters/14-release-and-edition-boundaries.md)

## 부록

- [용어 사전](../chapters/appendix-glossary.md)
- [인터페이스 명세](../chapters/appendix-interface-specifications.md)
- [소스 레퍼런스 맵](../chapters/appendix-reference-map.md)
- [개정 이력](../chapters/appendix-revision-history.md)

<div class="note" markdown="1">

**Edition scope.** 이 매뉴얼은 Community edition 아키텍처와 Enterprise edition only 장/섹션을 함께 다룹니다. <span class="badge enterprise">Enterprise edition only</span>로 표시된 내용은 private distribution 동작이며, Community 동작은 명시적으로 표시되었거나 거부 semantics를 설명하는 경우에만 해당합니다.

</div>

<div class="summary" markdown="1">

NAMROS는 S3 API 호환성과 무상태 active-active 게이트웨이 구조를 제공하는 고가용성 분산 오브젝트 스토리지입니다. S3 namespace 정본은 NAMROS metadata에 두고, payload bytes는 SegmentRef를 통해 local/SBS replicated/Enterprise EC backend에 매핑합니다.

</div>

이 아키텍처 매뉴얼은 시스템 설계자, 보안 아키텍트, 운영 기획자를 위해 작성되었습니다. 분산 데이터 저장 계층과 제어 계층의 책임 분리, Raft 기반 TiKV 메타데이터 소유권 모델, Enterprise급 Erasure Coding 분산 저장 구조의 세부 사양을 한곳에서 설명합니다.

## 다국어(i18n) 지원 목적

NAMROS 문서는 영문과 국문 포털의 정합성을 유지하도록 구성되어 있습니다. 개별 상세 기술 챕터는 영문 표준 용어를 기준으로 유지하고, 이 국문 포털에서는 전체 아키텍처 개념을 빠르게 훑고 각 세부 주제로 이동할 수 있습니다.

## 핵심 아키텍처 요약

NAMROS의 핵심 규칙은 S3 client에게 보이는 object visibility는 metadata transaction이 결정하고, 실제 byte durability와 placement는 storage backend가 담당한다는 것입니다. ObjectHead와 ObjectVersion은 metadata에 있고, ObjectVersion의 SegmentRefs가 SBS physical chunk 또는 <span class="badge enterprise">Enterprise edition only</span> EC shard layout으로 연결됩니다.

| 계층 | 정본 위치 | 의미 |
| --- | --- | --- |
| S3 protocol | `namros-gateway` | 요청 routing, auth, S3 error mapping, metadata/storage orchestration |
| Metadata | memory/Pebble/TiKV repository | bucket, object head, version, list index, MPU, protected ref, operation record의 정본 |
| Manifest | `ObjectVersion.SegmentRefs` | S3-visible version을 실제 저장 segment로 연결 |
| Storage | `SegmentStore`와 SBS | byte 저장, placement, range read, delete admission, repair signal |

## 읽기 안내

이 매뉴얼은 NAMROS 상태 전이를 어떤 컴포넌트가 소유하고, 어느 백엔드가 정본 상태를 보관하는지 추적하는 방식으로 구성되어 있습니다. S3 사용자, 플랫폼 운영자, 아키텍처 리뷰어, Enterprise 기능 리뷰어가 필요한 장으로 바로 이동할 수 있도록 구성했습니다.

## 제품 개요

NAMROS는 S3 호환 오브젝트 스토리지 API를 제공하는 Network Attached Multipath Resilient Object Storage입니다. 무상태 게이트웨이, 메타데이터 중심 일관성, Community active-active 운영, Enterprise EC/dedupe/WORM/KMS/compliance 기능 확장 경로를 핵심 제품 축으로 둡니다.

## S3 API 호환성

게이트웨이는 S3 path, query subresource, method, header, SigV4 context를 해석하고, 지원하지 않거나 잘못된 요청을 S3 호환 XML 오류로 매핑합니다. 호환성 판단은 S3 gateway layer가 맡고, namespace 변경은 metadata repository transaction에서 처리합니다.

## 무상태 Active-Active 게이트웨이

게이트웨이는 정본 객체 상태를 로컬에 보유하지 않습니다. metadata cache는 read-through 최적화일 뿐이며, bucket 또는 access-key 상태를 바꾸는 write path는 local cache를 무효화해야 합니다. 여러 gateway는 같은 metadata/storage 상태를 바라보며 장애 전환 후에도 동일한 committed namespace를 보아야 합니다.

## TiKV 메타데이터 정본

bucket, object version, multipart upload, lifecycle, protected ref, shared object, operation record 같은 정본 엔터티는 metadata repository transaction 경계 안에서 일관되게 갱신되어야 합니다. Pebble과 memory backend는 로컬 검증 경로로 쓰이고, TiKV는 분산 메타데이터 정본 저장소 역할을 합니다.

버킷 메타데이터는 SBS가 아니라 NAMROS metadata repository가 관리합니다. 버킷 이름은 bucket id로 해석되고, bucket config, ObjectHead, ObjectVersion, list index가 같은 metadata authority 아래에 유지됩니다. 버킷 내부 오브젝트 목록은 SBS volume을 스캔해 만들지 않고, object publish/delete transaction이 갱신하는 list index를 range scan해 응답합니다.

## 오브젝트와 세그먼트 모델

S3에 보이는 단위는 committed object version입니다. object version은 manifest를 가리키고, manifest는 segment reference를 가리킵니다. SegmentRef는 storage class, placement, digest, encryption, shared object 정보를 담아 S3 key가 물리 SBS 주소가 되지 않게 합니다.

SBS 저장은 논리 레벨과 물리 레벨을 분리합니다. 논리 레벨에는 bucket/key/version, storage class, volume-pool id, SegmentRef가 있고, 물리 레벨에는 SBS 복제 volume의 chunk/span 또는 <span class="badge enterprise">Enterprise edition only</span> EC stripe/shard 배치가 있습니다. 따라서 버킷은 특정 volume 하나에 고정되지 않고, 각 object version의 SegmentRef가 실제 read/write routing 정보를 보존합니다.

## 멀티파트 쓰기 가시성

multipart upload는 개별 part 저장과 complete publish 사이의 가시성 경계가 분명해야 합니다. part bytes가 저장되었지만 metadata 갱신이 실패하면 orphan cleanup 후보로 남기고, complete 단계에서 storage state를 검증할 수 없으면 object head를 게시하기 전에 실패해야 합니다.

## 복제 및 로컬 백엔드

local segment store는 Community baseline으로 개발과 호환성 검증에 적합합니다. SBS replicated physical path는 패키징된 환경에서 Community production-like substrate로 사용할 수 있고, <span class="badge enterprise">Enterprise edition only</span> SBS EC/classroute path는 storage efficiency와 degraded read를 제공합니다.

버킷 저장 공간을 늘릴 때는 버킷에 volume id를 붙이는 방식이 아니라, 해당 storage class가 사용하는 volume pool의 active member를 늘립니다. SBS/NAMRBD 절차로 새 물리 capacity를 준비하고, NAMROS metadata registry에 새 pool member를 등록한 뒤 gateway가 새 generation을 refresh하면 새 PUT/UploadPart가 확장된 공간을 사용할 수 있습니다. 기존 오브젝트는 commit 당시 기록된 placement로 계속 읽히며, 재배치는 별도 drain/rebalance operation입니다.

## SBS EC 백엔드 Enterprise

<span class="badge enterprise">Enterprise edition only</span> SBS 기반 Erasure Coding 백엔드는 K+M 구조, classroute, shard placement, degraded read, healing workflow를 통해 대규모 워크로드의 효율성과 내결함성을 제공합니다. Community build에서는 EC classroute 요청이 Enterprise-required 오류로 명확히 거절되어야 합니다.

## 버저닝, 라이프사이클, Object Lock

versioned bucket은 여러 committed object version과 delete marker를 가질 수 있습니다. lifecycle planner는 bucket rule, object version, active MPU, Object Lock state, protected ref를 함께 고려해야 하며, payload 삭제는 active protected ref 확인에 실패하면 fail-closed로 동작해야 합니다.

## 중복제거 Enterprise

<span class="badge enterprise">Enterprise edition only</span> dedupe는 S3-visible API가 아니라 내부 운영 기능입니다. hash는 candidate index일 뿐이며 byte verification, shared object publish, object-version attach, protected-root accounting, refcount repair는 metadata transaction과 audit context 안에서 다룹니다.

## 보안, 컴플라이언스, 에디션

<span class="badge enterprise">Enterprise edition only</span> NAMROS는 SEC, FINRA, CFTC, HIPAA 유형의 규제 워크로드를 지원하기 위한 control-plane evidence와 enforcement surface를 제공할 수 있지만, 제품 단독으로 법적 인증을 주장하지 않습니다. 운영자는 정책, 배포 통제, 외부 attestations, 규제 해석을 별도로 책임집니다.

## 관측성과 운영

health, report, audit, metrics, trace, incident triage surface는 운영자가 현재 상태를 읽고 위험한 작업 전에 preflight evidence를 확보할 수 있게 구성되어야 합니다. 성능, chaos, soak report는 재현 가능한 artifact와 제한 사항을 함께 기록해야 합니다.

## MCP 운영 Provider

MCP provider는 기본적으로 read-only resource와 tool을 제공하고, probe/repair/protect tool은 명시적인 운영자 승인을 요구해야 합니다. destructive tool은 별도의 role, reason, dual control, audit retention, recovery evidence 모델이 생기기 전까지 비활성화 상태로 남깁니다.

## 릴리스와 에디션 경계

Community source는 고정된 Community identity를 가지며, Enterprise runtime flag, environment switch, public build tag로 Enterprise 동작을 열 수 없어야 합니다. Enterprise 전용 구현은 public source export에서 제외되거나 안전한 stub으로 대체되어야 합니다.

## 용어 사전

용어 사전은 gateway, metadata, segment, protected ref, Enterprise marker처럼 문서 전체에서 반복되는 핵심 용어와 약어를 같은 의미로 읽을 수 있게 정리합니다.

## 인터페이스 명세

인터페이스 명세는 S3 gateway, debug endpoint, admin CLI, MCP resource/tool surface의 입력, 출력, 오류, schema stability 기준을 모아 검토할 수 있게 합니다.

## 소스 레퍼런스 맵

소스 레퍼런스 맵은 HTML 문서의 주장과 관련 Markdown runbook, source package, script, compatibility target 사이의 연결을 제공합니다.

## 개정 이력

개정 이력은 HTML 문서 세트가 어떤 범위로 시작했고 어떤 운영 가이드와 Enterprise 기능 축이 추가되었는지 추적합니다. 알려진 gap은 이후 문서 확장 작업의 기준으로 남깁니다.

제품 방향 <span class="badge enterprise">Enterprise 개발 중</span>

# NAMROS 고급 기능

<div class="warning" markdown="1">

이 페이지의 기능은 NAMROS Enterprise에서 개발 및 검증 중입니다. 현재
오픈소스 Community 배포판에서는 활성화되지 않습니다. 이 페이지는 개발
상태를 설명하며, 일반 공급, 지원 권리 또는 제공 일정을 보장하지 않습니다.

</div>

## 상태 용어

| 상태 | 의미 |
| --- | --- |
| <span class="badge">Community 포함</span> | 공개 소스 배포판에 구현되어 있으며 문서화된 검사로 검증합니다. |
| <span class="badge">실험적</span> | 공개 소스에서 실행할 수 있지만 배포·규모·검증 한계가 문서화되어 있습니다. |
| <span class="badge enterprise">Enterprise 개발 중</span> | 사설 Enterprise 개발선에 구현 기반이 있으며 통합·강화·검증 중입니다. |
| <span class="badge planned">계획/명세 단계</span> | 계약 또는 설계 목표만 존재하며 현재 사용할 수 있는 기능으로 표현해서는 안 됩니다. |

## Enterprise에서 개발 및 검증 중인 기능

### Erasure Coding 스토리지 클래스

<span class="badge enterprise">Enterprise 개발 중</span>

EC 스토리지 클래스 해석, replicated/EC 라우팅, multipart EC 쓰기,
multi-stripe 읽기, checksum 검증, 단일 세그먼트 degraded read의 구현 기반이
있습니다. 성능, 복구, failure domain, 장시간 클러스터 검증을 계속 진행하고
있습니다.

[EC architecture 계약 보기 →](../architecture-manual/chapters/08-sbs-ec-backend-enterprise.md)

### Object Lock 및 WORM 제어

<span class="badge enterprise">Enterprise 개발 중</span>

retention과 legal hold 메타데이터, governance/compliance 삭제 제어,
governance bypass 권한, protected reference, 구조화된 audit event의 구현
기반이 있습니다. lifecycle, 스토리지 삭제, 운영 및 릴리스 통합 검증을
계속 진행하고 있습니다.

[Versioning, lifecycle, Object Lock 모델 보기 →](../architecture-manual/chapters/09-versioning-lifecycle-object-lock.md)

### 검증 기반 중복 제거

<span class="badge enterprise">Enterprise 개발 중</span>

현재 범위는 replicated storage에서 동일 tenant/key를 대상으로 한 byte 검증
기반 post-process dedupe, shared-object reference accounting, repair, scrub,
one-shot background operation입니다. 장기 scheduler, 범위 확장, SBS-native
최적화, verified inline dedupe는 후속 개발 항목입니다.

[Dedupe와 shared-object 모델 보기 →](../architecture-manual/chapters/10-dedupe-and-shared-objects-enterprise.md)

### KMS 기반 payload 암호화

<span class="badge enterprise">Enterprise 개발 중</span>

envelope metadata, segment별 data encryption key, ciphertext 저장, bucket
default encryption, multipart/copy 처리, key-state fail-closed 제어의 구현
기반이 있습니다. streaming range 복호화, 외부 provider 운영 통합과 확장
검증은 개발 중입니다.

[KMS 암호화 가이드 보기 →](kms-encryption-guide.md)

### 컴플라이언스 제어와 증빙 지원

<span class="badge enterprise">Enterprise 개발 중</span>

evidence package, Object Lock 상태 요약, audit-chain 검증, profile 적용,
discovery manifest와 policy simulation의 부분 구현 기반이 있습니다.
principal/session 증빙, 신뢰 가능한 시간 증빙, 외부 export/SIEM 전달과
request admission 통합은 개발 중입니다. NAMROS 자체가 인증 또는 법적 준수를
보장하지는 않습니다.

[Security와 compliance architecture 보기 →](../architecture-manual/chapters/11-security-compliance-and-editions.md)

### 외부 Identity 연동

<span class="badge enterprise">Enterprise 개발 중</span>

principal/session 모델, mapping schema 검증, policy simulation,
temporary-credential envelope 기반이 있습니다. 실제 OIDC, SAML, LDAP/Active
Directory provider 검증, token 교환·발급, session lifecycle과 완전한 decision
evidence는 개발 중입니다.

[IAM 연동 가이드 보기 →](iam-integration-guide.md)

### 승인 기반 운영

<span class="badge enterprise">Enterprise 개발 중</span>

Console, CLI, MCP는 plan/preflight/apply/verify/audit envelope를 공유하며
Community에서 읽기 전용 진단을 제공합니다. Enterprise collector/action,
명시적 승인 수집, 외부 identity 연동, evidence export와 운영 강화는 개발
중입니다.

[MCP 운영 가이드 보기 →](mcp-operations-guide.md)

## 계획 또는 명세 단계

다음 항목은 설계 목표이며 현재 Community 또는 일반 공급 Enterprise 기능이
아닙니다.

- [Cross-region replication 및 disaster recovery](replication-disaster-recovery-guide.md)
- [S3 event notification과 broker 전달](event-notification-guide.md)
- [Scheduled inventory와 승인 기반 batch operation](inventory-batch-operations-guide.md)
- [클러스터 전체 aggregate quota, QoS, threshold alerting](quota-qos-guide.md)

명세 단계 문서에 포함된 설정, XML, JSON, CLI와 workflow 예시는 구현 상태
표가 달리 명시하지 않는 한 제안된 계약입니다. 현재 공개 빌드의 실행
절차로 사용해서는 안 됩니다.

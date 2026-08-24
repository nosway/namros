ID 및 접근 제어 <span class="badge">Community</span> <span class="badge enterprise">Enterprise 개발 섹션</span>

# NAMROS IAM 연동 가이드

<div class="note" markdown="1">

**기능 상태.** Community는 local access key와 기본 policy evaluation을
포함합니다. 외부 IAM mapping model과 dry-run tool은 Enterprise 개발 기반이
있으며 실제 provider 검증, token/session 발급과 완전한 decision evidence는
개발 중입니다.

</div>

<div class="summary" markdown="1">

이 문서는 NAMROS IAM federation과 외부 Identity Provider(IdP) 매핑을 위한 연동 규격을 설명합니다. Community 에디션은 로컬 Access Key와 기본적인 버킷/프리픽스 수준 정책 평가를 제공하며, 외부 IdP(OIDC/LDAP/AD) 연동 및 임시 세션 토큰(STS) 발급 기능은 Enterprise 전용 영역입니다.

</div>

## 에디션 범위

| 기능 | Community | Enterprise |
| --- | --- | --- |
| 부트스트랩/root 접근 키 | 지원 (로컬 설정) | 지원 (로컬 및 비밀 저장소) |
| 기본 버킷/접두사 정책 | 지원 (로컬 평가기) | 지원 (분산 엔진) |
| OIDC/LDAP/AD/SAML 매핑 | Enterprise 필요 오류, mapping schema dry-run 검증 가능 | <span class="badge enterprise">Enterprise 개발 중</span> 실제 provider 검증과 claim mapping이 진행 중입니다. |
| STS 스타일 세션 | Enterprise 필요 오류 | <span class="badge enterprise">Enterprise 개발 중</span> credential envelope 기반이 있으며 token 발급과 session lifecycle이 남았습니다. |
| 주체/세션 증빙 | 제한적 로컬 감사 | <span class="badge enterprise">Enterprise 개발 중</span> request/audit 기반이 있으며 compliance evidence 통합이 남았습니다. |

## 주체 및 세션 모델

| 필드 | 설명 / 예시 |
| --- | --- |
| `tenant` | 관리 단위 격리 경계. 예: `finance-dept` |
| `subject` | 로컬 사용자 ID 또는 외부 IAM subject 클레임. 예: `alice@company.com` |
| `groups/roles` | 정책 바인딩 입력. 예: `["finance-admin", "audit-auditor"]` |
| `session_id` | 임시 자격 증명과 감사 상관관계. 예: `sts-tx-9921c-ab` |
| `issuer` | 외부 제공자 식별자. 예: `https://keycloak.local/auth/realms/namros` |
| `policy_version` | 결정 재현성 해시. 예: `v1.2` |

## 설정 스키마 (iam-provider.json)

외부 OIDC 자격 증명 연동을 활성화하기 위한 구성 파일 예시는 다음과 같습니다.

```json
{
  "provider_id": "keycloak-oidc",
  "issuer": "https://keycloak.local/auth/realms/namros",
  "client_id": "namros-gateway",
  "jwks_uri": "https://keycloak.local/auth/realms/namros/protocol/openid-connect/certs",
  "mapping_rules": {
    "tenant_claim": "organization",
    "subject_claim": "preferred_username",
    "groups_claim": "resource_access.namros-gateway.roles",
    "default_tenant": "default"
  },
  "session_control": {
    "max_session_duration_seconds": 43200,
    "require_mfa": true
  }
}
```

## S3 호환 IAM 정책 예시

NAMROS는 AWS S3 정책 형식을 지원해 기존 S3 생태계와 호환되도록 합니다.

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "AllowReadAndWriteWithPrefix",
      "Effect": "Allow",
      "Principal": "*",
      "Action": [
        "s3:GetObject",
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::finance-reports/accounting/*"
    },
    {
      "Sid": "GovernanceBypassRestriction",
      "Effect": "Deny",
      "Principal": "*",
      "Action": "s3:BypassGovernanceRetention",
      "Resource": "arn:aws:s3:::finance-reports/*",
      "Condition": {
        "StringNotEquals": {
          "aws:PrincipalTag/Role": "SecurityAdmin"
        }
      }
    }
  ]
}
```

## 정책 시뮬레이션 CLI

게이트웨이를 실제로 시작하기 전에 정책 평가 엔진을 검증하는 CLI 사용 예시는 다음과 같습니다.

```sh
# 자격증명 연동 스키마 유효성 검사
namros-admin iam-mapping-validate -config iam-provider.json

# 특정 조건 하에서의 S3 접근 권한 판정 시뮬레이션
namros-admin iam-policy-simulate \
  -principal alice \
  -action s3:GetObject \
  -bucket finance-reports \
  -key accounting/q2_report.csv \
  -policy-file policy-finance.json
```

## 운영 참고 문서

| 필요 사항 | 참고 |
| --- | --- |
| 게이트웨이 운영 | [관리자 가이드](admin-guide.md) |
| 에디션 동작 | [Community 및 Enterprise 경계](../architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| MCP 증빙 | [MCP 운영 가이드](mcp-operations-guide.md) |

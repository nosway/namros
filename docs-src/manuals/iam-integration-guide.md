Identity And Access <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS IAM Integration Guide

<div class="note" markdown="1">

**Edition scope.** This page includes Community edition local access-key behavior and Enterprise edition only external IAM federation sections. External IdP mappings, STS-style sessions, and session evidence are unavailable in public Community builds except for documented denial behavior.

</div>

<div class="summary" markdown="1">

This document defines the interface specifications for NAMROS IAM federation and external Identity Provider (IdP) mappings. The Community edition supports local Access Keys and basic bucket/prefix-level policy evaluation, whereas external identity providers (OIDC/LDAP/AD) and temporary Security Token Service (STS) tokens are Enterprise capabilities.

</div>

## Edition Scope

| Capability | Community | Enterprise |
| --- | --- | --- |
| Bootstrap/root access key | Supported (Local Config) | Supported (Local & Secret Storage) |
| Basic bucket/prefix policy | Supported (Local Evaluator) | Supported (Distributed Engine) |
| OIDC/LDAP/AD/SAML mapping | Enterprise-required error | <span class="badge enterprise">Enterprise edition only</span> Active Mapping Provider |
| STS-style session | Enterprise-required error | <span class="badge enterprise">Enterprise edition only</span> Temporary Credentials (JWT Session) |
| Principal/session evidence | Limited local audit | <span class="badge enterprise">Enterprise edition only</span> Evidence Cryptographic Chain |

## Principal And Session Model

| Field | Description / Example |
| --- | --- |
| `tenant` | Administrative isolation boundary. Example: `finance-dept` |
| `subject` | Local user id or external IAM subject claim. Example: `alice@company.com` |
| `groups/roles` | Policy binding inputs. Example: `["finance-admin", "audit-auditor"]` |
| `session_id` | Temporary credential and audit correlation. Example: `sts-tx-9921c-ab` |
| `issuer` | External provider identity. Example: `https://keycloak.local/auth/realms/namros` |
| `policy_version` | Decision reproducibility hash. Example: `v1.2` |

## Configuration Schema (iam-provider.json)

Example configuration file layout to activate external OIDC integration:

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

## S3-Compatible IAM Policy Examples

NAMROS supports standard AWS S3 access policy formats to ensure seamless integration with the existing S3 ecosystem.

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

## Policy Simulation CLI

Use these validation tools to test the policy evaluation engine before launching the gateway:

```sh
# Validate identity federation schema
namros-admin iam-mapping-validate -config iam-provider.json

# Simulate S3 access evaluation under specific constraints
namros-admin iam-policy-simulate \
  -principal alice \
  -action s3:GetObject \
  -bucket finance-reports \
  -key accounting/q2_report.csv \
  -policy-file policy-finance.json
```

## Operational References

| Need | Reference |
| --- | --- |
| Gateway operations | [admin guide](admin-guide.md) |
| Edition behavior | [Community and Enterprise boundary](architecture-manual/chapters/14-release-and-edition-boundaries.md) |
| MCP evidence | [MCP operations guide](mcp-operations-guide.md) |

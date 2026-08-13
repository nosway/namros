Data Protection <span class="badge enterprise">Enterprise edition only</span>

# NAMROS Replication & Disaster Recovery Guide

<div class="warning" markdown="1">

**Enterprise edition only.** This page describes Enterprise-only cross-region replication and disaster recovery contracts. Community edition behavior is included only to separate local HA from replication, failover, and failback surfaces.

</div>

<div class="summary" markdown="1">

This guide defines the Enterprise contract and operational model for Cross-Region bucket/site replication and geo-distributed disaster recovery (DR). Community active-active gateways provide availability within one local cluster; cross-region replication, failover promotion, and failback automation are Enterprise or roadmap surfaces and are not exposed as public Community admin commands.

</div>

## Implementation Status

| Area | Current public Community behavior | Enterprise/spec status |
| --- | --- | --- |
| Local HA | Active-active gateways can share TiKV metadata, etcd coordination, and shared/SBS-backed storage inside one cluster. | Same base behavior can be used as a building block for Enterprise topologies. |
| Cross-region replication | No public Community `namros-admin replication` command or replication worker is available. | Enterprise contract for bucket, site, and batch replication. |
| DR failover/failback | Handled outside the public Community CLI with operator runbooks. | Target behavior for approved Enterprise operations, DNS/load-balancer cutover, and audit evidence. |

## Replication Scope

The Enterprise replication contract defines three distinct replication modes based on synchronization boundaries and scenarios:

| Replication Mode | Technical Description | Use Case |
| --- | --- | --- |
| Bucket Replication | Policy-based real-time asynchronous replication of new objects and versions from a source bucket to a target destination bucket. | Off-site archival of sensitive datasets, compliance with local data localization regulations. |
| Site Replication | Full site synchronization mapping bucket metadata, access control policies, user identities, and segment payloads across geodistributed locations. | Active-Passive or Active-Active geo-clustering for entire regional failover. |
| Batch Replication | On-demand backfill replication of pre-existing objects or unsynced backlog accumulated during a network partition. | Reconciling unsynced delta after disaster recovery, initial large-scale dataset migration. |

## Topology & Security Assumptions

Topology pre-requisites required to run Cross-Region Replication:

- **Versioning Enabled:** S3 Versioning must be activated on both source and destination buckets to prevent delete-marker synchronization anomalies and split-brain metadata states.
- **Establish IAM Trust Relationships:** Binds STS delegation roles so that destination gateways can validate background replication worker credentials originating from the source cluster.
- **SSE-KMS Key Remapping:** If master keys differ across regions, specify **KMS Key Translation** properties to decrypt segments via the source key and re-encrypt via the destination key inline.

## S3-Compatible Replication Configuration Example

Example JSON policy configuration to replicate prefix-matched S3 objects while preserving encryption postures:

```json
{
  "Role": "arn:aws:iam::123456789012:role/namros-replication-role",
  "Rules": [
    {
      "ID": "FinanceReportsSync",
      "Status": "Enabled",
      "Priority": 1,
      "Filter": {
        "Prefix": "accounting/"
      },
      "Destination": {
        "Bucket": "arn:aws:s3:::dr-finance-reports",
        "Account": "123456789012",
        "EncryptionConfiguration": {
          "ReplicaKmsKeyID": "arn:aws:kms:ap-northeast-2:123456789012:key/kr-master-key-01"
        },
        "ReplicationTime": {
          "Status": "Enabled",
          "Time": {
            "Minutes": 15
          }
        }
      },
      "SourceSelectionCriteria": {
        "SseKmsEncryptedObjects": {
          "Status": "Enabled"
        }
      }
    }
  ]
}
```

## Disaster Recovery (DR) Operations

Manual failover to the DR site must be driven by approved Enterprise operations or site-specific runbooks. Do not use the older nested `namros-admin replication ...` examples; that command group is not part of the current public CLI.

| Step | Operator action | Required evidence |
| --- | --- | --- |
| 1. Monitor lag | Read Enterprise replication lag, backlog bytes, last successful sync time, and destination health. | Timestamped lag report and source/destination gateway health. |
| 2. Promote DR site | Freeze source writes when possible, promote the destination bucket/site, and update DNS or load-balancer routing. | Approval record, promotion result, routing change, and S3 smoke result. |
| 3. Failback | After primary recovery, run reverse backfill, verify object/version alignment, and hand traffic back. | Reverse-sync report, reconciliation summary, and final smoke result. |

Integration <span class="badge enterprise">Enterprise edition only</span>

# NAMROS Event Notifications Guide

<div class="warning" markdown="1">

**Enterprise edition only.** This page describes Enterprise-only event notification contracts. Community edition behavior is included only to document denial and edition-boundary expectations.

</div>

<div class="summary" markdown="1">

This document defines the Enterprise contract and compatibility target for real-time S3 Event Notification. Community builds do not deliver event notifications; Enterprise builds are expected to detect object creation, deletion, and copy events and publish them to message brokers or Webhook endpoints with bounded latency and replayable failure handling.

</div>

## Implementation Status

| Area | Current public Community behavior | Enterprise/spec status |
| --- | --- | --- |
| Bucket notification APIs | Enterprise-only request paths must be denied by the edition boundary. | Private Enterprise contract for bucket notification configuration and retrieval. |
| Event delivery | No broker, Webhook, DLQ, or replay worker is started by the public Community build. | Target behavior for Webhook, Kafka, NATS, buffering, and DLQ replay. |
| MinIO streaming API | Not implemented. | Future compatibility candidate, not a current guarantee. |

## Configuration Scope

| S3 API Spec | Support Level | Details |
| --- | --- | --- |
| `GetBucketNotification` | <span class="badge enterprise">Enterprise edition only</span> | Retrieve real-time event broker endpoints and filter rules configured on a bucket. |
| `PutBucketNotification` | <span class="badge enterprise">Enterprise edition only</span> | Register prefix/suffix filters and target event types (Put, Delete, Copy) on a bucket. |
| `ListenNotification` | Future Plan | Planned expansion for compatibility with MinIO-specific notification streaming APIs. |

## Bucket Notification Config XML Example

Example XML specification to bind a Webhook endpoint and text filters to a bucket in compliance with S3 standards:

```xml
<NotificationConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <QueueConfiguration>
    <Id>WebhookImageResizeTrigger</Id>
    <Filter>
      <S3Key>
        <FilterRule>
          <Name>prefix</Name>
          <Value>uploads/</Value>
        </FilterRule>
        <FilterRule>
          <Name>suffix</Name>
          <Value>.png</Value>
        </FilterRule>
      </S3Key>
    </Filter>
    <Queue>arn:aws:sqs:us-east-1:123456789012:namros-webhook-receiver</Queue>
    <Event>s3:ObjectCreated:Put</Event>
    <Event>s3:ObjectCreated:CompleteMultipartUpload</Event>
  </QueueConfiguration>
</NotificationConfiguration>
```

## Event Payload Schema

Standard format for the JSON payload published as HTTP POST body or Kafka messages upon event triggers:

```json
{
  "Records": [
    {
      "eventVersion": "2.1",
      "eventSource": "aws:s3",
      "awsRegion": "us-east-1",
      "eventTime": "2026-07-09T09:15:00.000Z",
      "eventName": "ObjectCreated:Put",
      "userIdentity": {
        "principalId": "alice@company.com"
      },
      "requestParameters": {
        "sourceIPAddress": "192.168.1.100"
      },
      "responseElements": {
        "x-amz-request-id": "req-99ab-3321-cf"
      },
      "s3": {
        "s3SchemaVersion": "1.0",
        "configurationId": "WebhookImageResizeTrigger",
        "bucket": {
          "name": "finance-reports",
          "ownerIdentity": {
            "principalId": "namros-admin"
          },
          "arn": "arn:aws:s3:::finance-reports"
        },
        "object": {
          "key": "uploads/invoice-q2.png",
          "size": 1048576,
          "eTag": "b10a8db164e0754105b7a99be72e3fe5",
          "versionId": "v.9921_abc_x"
        }
      }
    }
  ]
}
```

## Destinations & Resiliency

- **Webhook (HTTP POST):** Enterprise delivery should include SHA-256 HMAC-based `X-Namros-Signature` headers so that endpoints can verify authenticity and integrity.
- **Kafka / NATS Integration:** Enterprise delivery should use a durable queue architecture. If downstream backpressure is detected, messages are buffered to prevent gateway S3 write operations from stalling.
- **Dead Letter Queue (DLQ):** If an endpoint is offline and transmissions fail repeatedly, messages should be isolated in metadata for manual replay later.

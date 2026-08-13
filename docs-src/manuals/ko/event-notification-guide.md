연동 <span class="badge enterprise">Enterprise edition only</span>

# NAMROS 이벤트 알림 가이드

<div class="warning" markdown="1">

**Enterprise edition only.** 이 페이지는 Enterprise 전용 이벤트 알림 계약을 설명합니다. Community edition 동작은 거부 응답과 에디션 경계를 설명하기 위해서만 포함합니다.

</div>

<div class="summary" markdown="1">

이 문서는 실시간 S3 이벤트 알림(Event Notification)에 대한 Enterprise 계약과 호환성 목표를 정의합니다. Community 빌드는 이벤트 알림을 전달하지 않으며, Enterprise 빌드는 오브젝트 생성, 삭제, 복사 이벤트를 감지해 메시지 브로커나 Webhook 엔드포인트로 제한된 지연 시간 안에 전달하고 실패 시 재처리할 수 있어야 합니다.

</div>

## 구현 상태

| 영역 | 현재 공개 Community 동작 | Enterprise/spec 상태 |
| --- | --- | --- |
| 버킷 알림 API | Enterprise 전용 요청 경로는 에디션 경계에서 거부되어야 합니다. | 버킷 알림 설정과 조회를 위한 private Enterprise 계약입니다. |
| 이벤트 전달 | 공개 Community 빌드는 broker, Webhook, DLQ, replay worker를 시작하지 않습니다. | Webhook, Kafka, NATS, buffering, DLQ replay에 대한 목표 동작입니다. |
| MinIO streaming API | 구현되어 있지 않습니다. | 향후 호환성 후보이며 현재 보장 기능은 아닙니다. |

## 기능 지원 범위

| S3 API 명세 | 지원 수준 | 세부 설명 |
| --- | --- | --- |
| `GetBucketNotification` | <span class="badge enterprise">Enterprise edition only</span> | 특정 버킷에 구성된 실시간 이벤트 브로커 엔드포인트와 필터링 규칙 조회. |
| `PutBucketNotification` | <span class="badge enterprise">Enterprise edition only</span> | 특정 버킷에 prefix/suffix 필터와 대상 이벤트 종류(Put/Delete/Copy) 등록. |
| `ListenNotification` | 향후 계획 | MinIO 전용 알림 스트리밍 API와의 연동 호환용 규격 확장 예정. |

## 버킷 알림 설정 XML 예시

S3 API 표준에 따라 Webhook 엔드포인트와 key 필터를 버킷에 연결하는 XML 설정 예시입니다.

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

## 이벤트 페이로드 스키마

이벤트가 발생했을 때 Webhook HTTP POST 본문이나 Kafka 메시지로 전달되는 JSON 페이로드 형식입니다.

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

## 알림 브로커 연동 및 내결함성

- **Webhook (HTTP POST):** Enterprise 전달 경로는 엔드포인트가 무결성 서명을 검증할 수 있도록 SHA-256 HMAC 기반 `X-Namros-Signature` 요청 헤더를 포함해야 합니다.
- **Kafka / NATS 연동:** Enterprise 전달 경로는 내구성 있는 전송 큐를 사용해야 하며, 백프레셔가 감지되면 일반 S3 쓰기 응답이 지연되지 않도록 메시지를 버퍼링해야 합니다.
- **DLQ (Dead Letter Queue):** 대상 엔드포인트 장애로 반복 전송 실패가 발생한 알림 메시지는 이후 수동 replay가 가능하도록 메타데이터에 격리되어야 합니다.

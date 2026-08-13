Tenant Controls <span class="badge">Community</span> <span class="badge enterprise">Enterprise edition only sections</span>

# NAMROS Quota and QoS Guide

<div class="note" markdown="1">

**Edition scope.** This page includes the Community edition bucket max-object-size quota and Enterprise edition only capacity, tenant quota, QoS, and alerting sections. Do not read Enterprise-only quota/QoS contracts as public Community behavior.

</div>

<div class="summary" markdown="1">

This document defines the current Community quota metadata surface and the Enterprise specifications for admission enforcement and Quality of Service (QoS Rate Limiting). Community currently exposes bucket max-object-size quota through `namros-admin bucket-quota-*`, tenant quota records through `namros-admin tenant-quota-*`, and a metadata tenant usage reconciliation foundation; aggregate write admission enforcement, bandwidth shaping, and alerting are follow-on/spec surfaces.

</div>

## Implementation Status

| Area | Current public Community behavior | Enterprise/spec status |
| --- | --- | --- |
| Bucket max-object-size quota | Supported through `namros-admin bucket-quota-put`, `bucket-quota-get`, and `bucket-quota-delete`. | Available as the baseline quota primitive. |
| Bucket capacity/object-count quota | Not implemented as an aggregate usage limiter in public Community builds. | Enterprise contract for usage counters, admission, and alerts. |
| Tenant quota records | Supported as metadata records through `namros-admin tenant-quota-put`, `tenant-quota-get`, and `tenant-quota-delete`. `max_active_uploads` is enforced on CreateMultipartUpload; bytes/object admission follows later. | Enterprise contract for tenant isolation, bandwidth/TPS shaping, and telemetry. |
| Tenant usage records | Supported as a reconciliation foundation that records tenant bytes, committed object-version count, active MPU count, reconciliation timestamp, and reconciliation id. | Admission enforcement follows later. |
| Gateway request concurrency | Supported with gateway-local global, per-tenant, read-class, and write-class request limits. Saturation returns S3 `SlowDown`. | Cluster-wide policy and bandwidth shaping follow later. |
| Gateway bandwidth hooks | Supported with gateway-local upload/download byte-per-second shaping hooks that default to disabled and preserve PUT/GET stream contents. | Cluster-wide tenant token-bucket policy follows later. |

## Control Types

The quota and QoS model separates the Community primitive from Enterprise multi-tenant controls.

| Control Range | Operating Mechanism | Execution Constraints |
| --- | --- | --- |
| Bucket max-object-size quota | Rejects individual writes whose object size exceeds the configured per-bucket maximum. | <span class="badge">Community</span> |
| Bucket capacity/object-count quota | Limits total storage capacity or cumulative object count on an individual bucket basis. | <span class="badge enterprise">Enterprise edition only</span> |
| Tenant Quota Records | Stores max bytes, max objects, and max active uploads for a tenant/organization scope; active MPU admission is enforced before a new upload id is allocated. | <span class="badge">Community metadata foundation</span> |
| Tenant Usage Reconciliation | Scans tenant bucket metadata to write a conservative bytes/object-version/active-upload baseline. | <span class="badge">Community metadata foundation</span> |
| Request Concurrency Limiter | Limits concurrent S3 requests per gateway, tenant, and read/write operation class. | <span class="badge">Community gateway control</span> |
| Gateway Bandwidth Hooks | Wraps upload request bodies and download response writers with gateway-local byte-per-second shaping; zero disables the hook. | <span class="badge">Community gateway control</span> |
| Bandwidth Control (QoS Rate Limit) | Throttles requests per second (TPS) and bandwidth (MB/s) per IP, API action, or tenant token across gateways. | <span class="badge enterprise">Enterprise edition only</span> |
| Threshold Alert | Sends warning logs and alerts when entering warning thresholds before quota exhaustion. | <span class="badge enterprise">Enterprise edition only</span> |

## Quota & QoS Configuration Schema

Community bucket max-object-size quota and tenant quota records are managed with the admin CLI:

```sh
namros-admin bucket-quota-put -bucket finance-reports -max-object-size-bytes 1073741824
namros-admin bucket-quota-get -bucket finance-reports
namros-admin bucket-quota-delete -bucket finance-reports
namros-admin tenant-quota-put -tenant-id finance -max-bytes 1099511627776 -max-objects 1000000 -max-active-uploads 256
namros-admin tenant-quota-get -tenant-id finance
namros-admin tenant-quota-delete -tenant-id finance
```

The following JSON is an Enterprise configuration schema example for bandwidth control and hard-limit quotas applied to specific buckets and tenants:

```json
{
  "policy_id": "finance-tenant-limits",
  "tenant": "finance-dept",
  "quota": {
    "max_size_bytes": 1099511627776,
    "max_objects_count": 5000000,
    "warning_threshold_percent": 80
  },
  "qos_limits": {
    "read_bandwidth_mb_per_sec": 500,
    "write_bandwidth_mb_per_sec": 200,
    "max_read_tps": 5000,
    "max_write_tps": 1000
  }
}
```

## Admission & S3 Error Specification

Error XML specifications returned by the gateway in compliance with S3 standards when limits are exceeded:

### 1. Storage Capacity Quota Exceeded (HTTP 409 Conflict / QuotaExceeded)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>QuotaExceeded</Code>
  <Message>The bucket write request was denied because the hard quota limit of maximum bytes has been exceeded.</Message>
  <Resource>/finance-reports/accounting/q2_report.csv</Resource>
  <RequestId>req-88cc-9921-ab</RequestId>
</Error>
```

### 2. QoS Bandwidth and TPS Throttling (HTTP 503 Slow Down)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<Error>
  <Code>SlowDown</Code>
  <Message>Please reduce your request rate. Bandwidth or TPS limit exceeded for this tenant pool.</Message>
  <Resource>/finance-reports/</Resource>
  <RequestId>req-77bc-3341-ef</RequestId>
</Error>
```

## Prometheus Metrics Reference

The following real-time Prometheus metrics are Enterprise/spec telemetry for cluster monitoring and Grafana dashboards unless explicitly implemented by the active distribution:

| Metric Key | Description |
| --- | --- |
| `tenant_usage_bytes` | Total metadata and physical payload bytes occupied by a given tenant. |
| `bucket_usage_bytes` | S3 storage utilization bytes evaluated per bucket. |
| `bucket_object_count` | Accumulated count of object versions residing in a given bucket. |
| `quota_denied_requests` | Total accumulated write requests (PUT/Multipart Upload) rejected due to quota exhaustion. |
| `rate_limited_requests` | Total requests throttled with HTTP 503 Slow Down due to QoS bandwidth limit violations. |
| `namros_gateway_admission_rejections_total{kind,reason}` | Implemented low-cardinality rejection counter for bucket quota, request limit, and data budget admission failures. The same `kind`/`reason` summary is exposed in `/debug/operations/metrics` and console operations views. |
| `namros_gateway_admission_decision_duration_seconds{kind,outcome,reason}` | Implemented histogram for request limit, data budget, and bucket quota admission timing. Labels stay bounded to admission kind, accepted/rejected outcome, and configured reason. |
| `namros_gateway_s3_errors_total{api,status_class,error_code}` | Implemented low-cardinality S3 XML error counter. Bucket names, object keys, version ids, and request ids are intentionally excluded from labels. |

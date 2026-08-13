package opsmetrics

import (
	"testing"
	"time"
)

func TestGatewayMetricsSnapshotRecordsLayerBreakdown(t *testing.T) {
	metrics := NewGatewayMetrics(BuildInfo{Edition: "community", Version: "test", Commit: "abc"})

	metrics.ObserveS3(S3Observation{API: "GetObject", StatusCode: 404, ResponseBytes: 256, Duration: 10 * time.Millisecond, ErrorCode: "NoSuchKey"})
	metrics.ObserveS3FirstByte("GetObject", 404, 4*time.Millisecond)
	metrics.ObserveS3RequestBodyRead("PutObject", 200, 3*time.Millisecond)
	metrics.ObserveStorage("put_segment", "sbs-physical", 25*time.Millisecond, 4096, nil)
	metrics.ObserveSBSPhysicalAllocation(5*time.Millisecond, 2, nil)
	metrics.ObserveSBSPhysicalChunk("write", 15*time.Millisecond, 4096, nil)
	metrics.ObserveSBSPhysicalReadback(20*time.Millisecond, 4096, nil)
	metrics.ObserveSBSECShard("write", "store-a", "data", 0, 30*time.Millisecond, 1024, nil)
	metrics.ObserveAdmissionDecision("request_limit", "allowed", true, 1*time.Millisecond)
	metrics.ObserveAdmissionDecision("request_limit", "global", false, 2*time.Millisecond)
	metrics.ObserveAdmissionRejection("request_limit", "global")
	metrics.ObserveAdmissionRejection("request_limit", "global")
	metrics.ObserveAdmissionRejection("bucket_quota", "max_object_size")

	snapshot := metrics.Snapshot()
	assertLayer(t, snapshot, "s3", "GetObject", "4xx", 1, 10, 256, 1)
	assertLayer(t, snapshot, "s3_first_byte", "GetObject", "4xx", 1, 4, 0, 1)
	assertLayer(t, snapshot, "s3_request_body_read", "PutObject", "2xx", 1, 3, 0, 1)
	assertLayer(t, snapshot, "storage:sbs-physical", "put_segment", "ok", 1, 25, 4096, 1)
	assertLayer(t, snapshot, "sbs_physical", "allocate_chunk_ids", "ok", 1, 5, 0, 2)
	assertLayer(t, snapshot, "sbs_physical", "chunk_write", "ok", 1, 15, 4096, 1)
	assertLayer(t, snapshot, "sbs_physical", "write_readback", "ok", 1, 20, 4096, 1)
	assertLayer(t, snapshot, "sbs_ec:store-a", "shard_write_data", "ok", 1, 30, 1024, 1)
	assertLayer(t, snapshot, "admission", "request_limit", "accepted", 1, 1, 0, 1)
	assertLayer(t, snapshot, "admission", "request_limit", "rejected", 1, 2, 0, 1)
	assertAdmission(t, snapshot, "request_limit", "global", 3)
	assertAdmission(t, snapshot, "bucket_quota", "max_object_size", 1)
	assertS3Error(t, snapshot, "GetObject", "4xx", "NoSuchKey", 1)
}

func assertLayer(t *testing.T, snapshot MetricsSnapshot, component, operation, status string, count uint64, avgMs, bytes, units uint64) {
	t.Helper()
	for _, layer := range snapshot.Layers {
		if layer.Component == component && layer.Operation == operation && layer.Status == status {
			if layer.Count != count || uint64(layer.AvgMs) != avgMs || layer.Bytes != bytes || layer.Units != units {
				t.Fatalf("layer %s/%s/%s = %+v", component, operation, status, layer)
			}
			return
		}
	}
	t.Fatalf("missing layer %s/%s/%s in %+v", component, operation, status, snapshot.Layers)
}

func assertAdmission(t *testing.T, snapshot MetricsSnapshot, kind, reason string, count uint64) {
	t.Helper()
	for _, admission := range snapshot.Admissions {
		if admission.Kind == kind && admission.Reason == reason {
			if admission.Count != count {
				t.Fatalf("admission %s/%s count = %d, want %d", kind, reason, admission.Count, count)
			}
			return
		}
	}
	t.Fatalf("missing admission %s/%s in %+v", kind, reason, snapshot.Admissions)
}

func assertS3Error(t *testing.T, snapshot MetricsSnapshot, api, statusClass, errorCode string, count uint64) {
	t.Helper()
	for _, item := range snapshot.S3Errors {
		if item.API == api && item.StatusClass == statusClass && item.ErrorCode == errorCode {
			if item.Count != count {
				t.Fatalf("s3 error %s/%s/%s count = %d, want %d", api, statusClass, errorCode, item.Count, count)
			}
			return
		}
	}
	t.Fatalf("missing s3 error %s/%s/%s in %+v", api, statusClass, errorCode, snapshot.S3Errors)
}

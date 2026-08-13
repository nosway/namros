package gateway

import (
	"testing"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/s3api/routing"
)

func TestRequestLimiterEnforcesGlobalTenantAndClassLimits(t *testing.T) {
	limiter := newRequestLimiter(config.Config{
		GatewayRequestMaxConcurrent:          3,
		GatewayRequestMaxConcurrentPerTenant: 1,
		GatewayRequestMaxConcurrentReads:     2,
		GatewayRequestMaxConcurrentWrites:    1,
	})
	releaseTenantA, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-a", Class: requestLimitClassRead})
	if !ok {
		t.Fatalf("tryAcquire(tenant-a read) ok=false reason=%q", reason)
	}
	if _, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-a", Class: requestLimitClassWrite}); ok || reason != "tenant" {
		t.Fatalf("tryAcquire(tenant-a second) ok=%v reason=%q, want tenant", ok, reason)
	}
	releaseTenantBWrite, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-b", Class: requestLimitClassWrite})
	if !ok {
		t.Fatalf("tryAcquire(tenant-b write) ok=false reason=%q", reason)
	}
	if _, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-c", Class: requestLimitClassWrite}); ok || reason != "write_class" {
		t.Fatalf("tryAcquire(second write) ok=%v reason=%q, want write_class", ok, reason)
	}
	releaseTenantCRead, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-c", Class: requestLimitClassRead})
	if !ok {
		t.Fatalf("tryAcquire(tenant-c read) ok=false reason=%q", reason)
	}
	if _, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-d", Class: requestLimitClassOther}); ok || reason != "global" {
		t.Fatalf("tryAcquire(over global) ok=%v reason=%q, want global", ok, reason)
	}
	releaseTenantA()
	releaseTenantBWrite()
	releaseTenantCRead()
	if release, reason, ok := limiter.tryAcquire(requestLimitScope{TenantID: "tenant-a", Class: requestLimitClassWrite}); !ok {
		t.Fatalf("tryAcquire(after release) ok=false reason=%q", reason)
	} else {
		release()
	}
}

func TestRequestLimitClassForOperation(t *testing.T) {
	for _, op := range []routing.Operation{
		routing.OperationPutObject,
		routing.OperationCreateMultipartUpload,
		routing.OperationCompleteMultipart,
	} {
		if got := requestLimitClassForOperation(op); got != requestLimitClassWrite {
			t.Fatalf("requestLimitClassForOperation(%s) = %q, want write", op, got)
		}
	}
	for _, op := range []routing.Operation{
		routing.OperationListBuckets,
		routing.OperationGetObject,
		routing.OperationHeadObject,
	} {
		if got := requestLimitClassForOperation(op); got != requestLimitClassRead {
			t.Fatalf("requestLimitClassForOperation(%s) = %q, want read", op, got)
		}
	}
	if got := requestLimitClassForOperation(routing.OperationUnsupported); got != requestLimitClassOther {
		t.Fatalf("requestLimitClassForOperation(unsupported) = %q, want other", got)
	}
}

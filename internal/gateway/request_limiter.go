package gateway

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/s3api/routing"
	"github.com/nosway/namros/internal/s3api/s3err"
)

const (
	requestLimitClassRead  = "read"
	requestLimitClassWrite = "write"
	requestLimitClassOther = "other"
)

type requestLimiter struct {
	maxConcurrent          int
	maxConcurrentPerTenant int
	maxConcurrentReads     int
	maxConcurrentWrites    int

	mu       sync.Mutex
	inFlight int
	byTenant map[string]int
	byClass  map[string]int
}

type requestLimitScope struct {
	TenantID string
	Class    string
}

func newRequestLimiter(cfg config.Config) *requestLimiter {
	if cfg.GatewayRequestMaxConcurrent <= 0 &&
		cfg.GatewayRequestMaxConcurrentPerTenant <= 0 &&
		cfg.GatewayRequestMaxConcurrentReads <= 0 &&
		cfg.GatewayRequestMaxConcurrentWrites <= 0 {
		return nil
	}
	return &requestLimiter{
		maxConcurrent:          cfg.GatewayRequestMaxConcurrent,
		maxConcurrentPerTenant: cfg.GatewayRequestMaxConcurrentPerTenant,
		maxConcurrentReads:     cfg.GatewayRequestMaxConcurrentReads,
		maxConcurrentWrites:    cfg.GatewayRequestMaxConcurrentWrites,
		byTenant:               make(map[string]int),
		byClass:                make(map[string]int),
	}
}

func (l *requestLimiter) tryAcquire(scope requestLimitScope) (func(), string, bool) {
	if l == nil {
		return func() {}, "", true
	}
	class := normalizeRequestLimitClass(scope.Class)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxConcurrent > 0 && l.inFlight+1 > l.maxConcurrent {
		return nil, "global", false
	}
	if l.maxConcurrentPerTenant > 0 && scope.TenantID != "" && l.byTenant[scope.TenantID]+1 > l.maxConcurrentPerTenant {
		return nil, "tenant", false
	}
	if limit := l.classLimit(class); limit > 0 && l.byClass[class]+1 > limit {
		return nil, class + "_class", false
	}
	l.inFlight++
	if scope.TenantID != "" {
		l.byTenant[scope.TenantID]++
	}
	l.byClass[class]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.release(scope.TenantID, class)
		})
	}, "", true
}

func (l *requestLimiter) release(tenantID, class string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.inFlight > 0 {
		l.inFlight--
	}
	if tenantID != "" {
		if l.byTenant[tenantID] <= 1 {
			delete(l.byTenant, tenantID)
		} else {
			l.byTenant[tenantID]--
		}
	}
	if l.byClass[class] <= 1 {
		delete(l.byClass, class)
	} else {
		l.byClass[class]--
	}
}

func (l *requestLimiter) classLimit(class string) int {
	switch class {
	case requestLimitClassRead:
		return l.maxConcurrentReads
	case requestLimitClassWrite:
		return l.maxConcurrentWrites
	default:
		return 0
	}
}

func normalizeRequestLimitClass(class string) string {
	switch class {
	case requestLimitClassRead, requestLimitClassWrite:
		return class
	default:
		return requestLimitClassOther
	}
}

func requestLimitClassForOperation(op routing.Operation) string {
	if isGatewayDrainWriteOperation(op) {
		return requestLimitClassWrite
	}
	switch op {
	case routing.OperationUnsupported:
		return requestLimitClassOther
	default:
		return requestLimitClassRead
	}
}

func (h s3Handler) reserveRequestLimit(c *gin.Context, tenantID string, op routing.Operation) (func(), bool) {
	start := time.Now()
	release, reason, ok := h.requestLimiter.tryAcquire(requestLimitScope{
		TenantID: tenantID,
		Class:    requestLimitClassForOperation(op),
	})
	if ok {
		h.deps.GatewayMetrics.ObserveAdmissionDecision("request_limit", "allowed", true, time.Since(start))
		return release, true
	}
	h.deps.GatewayMetrics.ObserveAdmissionDecision("request_limit", reason, false, time.Since(start))
	writeS3Error(c, s3err.SlowDown("gateway request concurrency limit exceeded"))
	return nil, false
}

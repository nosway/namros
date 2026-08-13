package gateway

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/s3api/s3err"
)

const maxBudgetReservationBytes = ^uint64(0)

type dataBudget struct {
	maxBytes     uint64
	maxRequests  int
	unknownBytes uint64

	mu       sync.Mutex
	bytes    uint64
	requests int
}

func newDataBudget(cfg config.Config) *dataBudget {
	if cfg.GatewayDataBudgetBytes == 0 && cfg.GatewayDataBudgetMaxRequests <= 0 {
		return nil
	}
	return &dataBudget{
		maxBytes:     cfg.GatewayDataBudgetBytes,
		maxRequests:  cfg.GatewayDataBudgetMaxRequests,
		unknownBytes: cfg.GatewayDataBudgetUnknownBytes,
	}
}

func (b *dataBudget) tryAcquire(sizeBytes uint64, sizeKnown bool) (func(), string, bool) {
	if b == nil {
		return func() {}, "", true
	}
	reservedBytes := sizeBytes
	if !sizeKnown {
		reservedBytes = b.unknownBytes
		if b.maxBytes > 0 && reservedBytes == 0 {
			return nil, "unknown_size", false
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.maxRequests > 0 && b.requests+1 > b.maxRequests {
		return nil, "request_limit", false
	}
	if b.maxBytes > 0 {
		if reservedBytes > b.maxBytes || b.bytes+reservedBytes < b.bytes || b.bytes+reservedBytes > b.maxBytes {
			return nil, "bytes_limit", false
		}
	}
	b.requests++
	b.bytes += reservedBytes
	var once sync.Once
	return func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if b.requests > 0 {
				b.requests--
			}
			if reservedBytes > b.bytes {
				b.bytes = 0
			} else {
				b.bytes -= reservedBytes
			}
		})
	}, "", true
}

func (h s3Handler) reserveDataBudget(c *gin.Context, sizeBytes uint64, sizeKnown bool) (func(), bool) {
	start := time.Now()
	release, reason, ok := h.dataBudget.tryAcquire(sizeBytes, sizeKnown)
	if ok {
		h.deps.GatewayMetrics.ObserveAdmissionDecision("data_budget", "allowed", true, time.Since(start))
		return release, true
	}
	h.deps.GatewayMetrics.ObserveAdmissionDecision("data_budget", reason, false, time.Since(start))
	writeS3Error(c, s3err.SlowDown("gateway data budget exceeded"))
	return nil, false
}

func copyDataBudgetBytes(sizeBytes uint64) uint64 {
	if sizeBytes > maxBudgetReservationBytes/2 {
		return maxBudgetReservationBytes
	}
	return sizeBytes * 2
}

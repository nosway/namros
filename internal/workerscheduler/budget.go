package workerscheduler

import (
	"errors"
	"fmt"
	"sync"
)

var ErrThrottled = errors.New("worker scheduler throttled")

type BudgetConfig struct {
	MaxConcurrent          int
	MaxConcurrentPerTenant int
	MaxConcurrentPerPool   int
}

type BudgetScope struct {
	WorkerKind string
	ShardID    string
	OwnerID    string
	TenantID   string
	PoolID     string
}

type BudgetSnapshot struct {
	MaxConcurrent          int            `json:"max_concurrent,omitempty"`
	MaxConcurrentPerTenant int            `json:"max_concurrent_per_tenant,omitempty"`
	MaxConcurrentPerPool   int            `json:"max_concurrent_per_pool,omitempty"`
	InUse                  int            `json:"in_use"`
	InUseByTenant          map[string]int `json:"in_use_by_tenant,omitempty"`
	InUseByPool            map[string]int `json:"in_use_by_pool,omitempty"`
}

type Budget struct {
	cfg BudgetConfig

	mu      sync.Mutex
	inUse   int
	tenants map[string]int
	pools   map[string]int
}

func NewBudget(cfg BudgetConfig) *Budget {
	if cfg.MaxConcurrent <= 0 && cfg.MaxConcurrentPerTenant <= 0 && cfg.MaxConcurrentPerPool <= 0 {
		return nil
	}
	return &Budget{
		cfg:     cfg,
		tenants: make(map[string]int),
		pools:   make(map[string]int),
	}
}

func (b *Budget) TryAcquire(scope BudgetScope) (func(), string, bool) {
	if b == nil {
		return func() {}, "", true
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cfg.MaxConcurrent > 0 && b.inUse+1 > b.cfg.MaxConcurrent {
		return nil, "global_concurrency", false
	}
	if scope.TenantID != "" && b.cfg.MaxConcurrentPerTenant > 0 && b.tenants[scope.TenantID]+1 > b.cfg.MaxConcurrentPerTenant {
		return nil, "tenant_concurrency", false
	}
	if scope.PoolID != "" && b.cfg.MaxConcurrentPerPool > 0 && b.pools[scope.PoolID]+1 > b.cfg.MaxConcurrentPerPool {
		return nil, "pool_concurrency", false
	}
	b.inUse++
	if scope.TenantID != "" {
		b.tenants[scope.TenantID]++
	}
	if scope.PoolID != "" {
		b.pools[scope.PoolID]++
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			b.release(scope)
		})
	}, "", true
}

func (b *Budget) Snapshot() BudgetSnapshot {
	if b == nil {
		return BudgetSnapshot{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return BudgetSnapshot{
		MaxConcurrent:          b.cfg.MaxConcurrent,
		MaxConcurrentPerTenant: b.cfg.MaxConcurrentPerTenant,
		MaxConcurrentPerPool:   b.cfg.MaxConcurrentPerPool,
		InUse:                  b.inUse,
		InUseByTenant:          cloneCounterMap(b.tenants),
		InUseByPool:            cloneCounterMap(b.pools),
	}
}

func (b *Budget) release(scope BudgetScope) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inUse > 0 {
		b.inUse--
	}
	decrementCounter(b.tenants, scope.TenantID)
	decrementCounter(b.pools, scope.PoolID)
}

func throttleError(reason string) error {
	if reason == "" {
		return ErrThrottled
	}
	return fmt.Errorf("%w: %s", ErrThrottled, reason)
}

func cloneCounterMap(in map[string]int) map[string]int {
	out := make(map[string]int)
	for key, value := range in {
		if value > 0 {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func decrementCounter(values map[string]int, key string) {
	if key == "" {
		return
	}
	if values[key] <= 1 {
		delete(values, key)
		return
	}
	values[key]--
}

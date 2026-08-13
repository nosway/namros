package tikv

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
)

func TestValidateConfig(t *testing.T) {
	t.Run("accepts minimal config", func(t *testing.T) {
		err := ValidateConfig(Config{PDEndpoints: []string{"127.0.0.1:2379"}})
		if err != nil {
			t.Fatalf("ValidateConfig() error = %v", err)
		}
	})
	t.Run("rejects missing pd endpoints", func(t *testing.T) {
		if err := ValidateConfig(Config{}); err == nil {
			t.Fatal("ValidateConfig() error = nil, want error")
		}
	})
	t.Run("rejects unsupported api version", func(t *testing.T) {
		err := ValidateConfig(Config{PDEndpoints: []string{"127.0.0.1:2379"}, APIVersion: "v1ttl"})
		if err == nil {
			t.Fatal("ValidateConfig() error = nil, want error")
		}
	})
	t.Run("rejects partial tls config", func(t *testing.T) {
		err := ValidateConfig(Config{
			PDEndpoints: []string{"127.0.0.1:2379"},
			TLS:         TLSConfig{CAPath: "/certs/ca.crt"},
		})
		if err == nil {
			t.Fatal("ValidateConfig() error = nil, want error")
		}
	})
	t.Run("rejects invalid retry policy", func(t *testing.T) {
		err := ValidateConfig(Config{
			PDEndpoints: []string{"127.0.0.1:2379"},
			Retry: RetryPolicy{
				InitialBackoff: 200 * time.Millisecond,
				MaxBackoff:     100 * time.Millisecond,
			},
		})
		if err == nil {
			t.Fatal("ValidateConfig() error = nil, want error")
		}
	})
}

func TestPrefixedKVIsolatesKeysForV1KeyspaceFallback(t *testing.T) {
	ctx := context.Background()
	base := newFakeKV()
	kvA := newPrefixedKV(base, "tenant-a")
	kvB := newPrefixedKV(base, "tenant-b")

	if err := kvA.Set(ctx, "namros/buckets/a", []byte("a")); err != nil {
		t.Fatalf("kvA.Set() error = %v", err)
	}
	if err := kvB.Set(ctx, "namros/buckets/a", []byte("b")); err != nil {
		t.Fatalf("kvB.Set() error = %v", err)
	}
	gotA, found, err := kvA.Get(ctx, "namros/buckets/a")
	if err != nil || !found || string(gotA) != "a" {
		t.Fatalf("kvA.Get() = %q found %v err %v, want a/true/nil", gotA, found, err)
	}
	gotB, found, err := kvB.Get(ctx, "namros/buckets/a")
	if err != nil || !found || string(gotB) != "b" {
		t.Fatalf("kvB.Get() = %q found %v err %v, want b/true/nil", gotB, found, err)
	}
	if _, found, err := base.Get(ctx, "namros/buckets/a"); err != nil || found {
		t.Fatalf("base unprefixed Get found = %v err = %v, want false/nil", found, err)
	}
}

func TestPrefixedKVListReturnsUnprefixedKeysAndCursor(t *testing.T) {
	ctx := context.Background()
	base := newFakeKV()
	kv := newPrefixedKV(base, "tenant-list")
	for _, key := range []string{
		"namros/objects/a",
		"namros/objects/b",
		"namros/uploads/a",
	} {
		if err := kv.Set(ctx, key, []byte(key)); err != nil {
			t.Fatalf("Set(%s) error = %v", key, err)
		}
	}
	keys, cursor, err := kv.List(ctx, "namros/objects/", "", 1)
	if err != nil {
		t.Fatalf("List first page error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "namros/objects/a" || cursor != "namros/objects/a" {
		t.Fatalf("first page keys = %v cursor = %q", keys, cursor)
	}
	keys, cursor, err = kv.List(ctx, "namros/objects/", cursor, 1)
	if err != nil {
		t.Fatalf("List second page error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "namros/objects/b" || cursor != "" {
		t.Fatalf("second page keys = %v cursor = %q", keys, cursor)
	}
}

func TestPrefixedKVListRangeBoundsKeyspaceAndCursor(t *testing.T) {
	ctx := context.Background()
	base := newFakeKV()
	kvA := newPrefixedKV(base, "tenant-range-a")
	kvB := newPrefixedKV(base, "tenant-range-b")
	for _, key := range []string{
		"namros/objects/a",
		"namros/objects/aa",
		"namros/objects/b",
		"namros/objects/c",
		"namros/uploads/a",
	} {
		if err := kvA.Set(ctx, key, []byte(key)); err != nil {
			t.Fatalf("kvA.Set(%s) error = %v", key, err)
		}
	}
	if err := kvB.Set(ctx, "namros/objects/ab", []byte("other-keyspace")); err != nil {
		t.Fatalf("kvB.Set() error = %v", err)
	}
	keys, cursor, err := kvA.ListRange(ctx, "namros/objects/a", "namros/objects/c", "", 2)
	if err != nil {
		t.Fatalf("ListRange first page error = %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"namros/objects/a", "namros/objects/aa"}) || cursor != "namros/objects/aa" {
		t.Fatalf("first page keys = %v cursor = %q", keys, cursor)
	}
	keys, cursor, err = kvA.ListRange(ctx, "namros/objects/a", "namros/objects/c", cursor, 2)
	if err != nil {
		t.Fatalf("ListRange second page error = %v", err)
	}
	if !reflect.DeepEqual(keys, []string{"namros/objects/b"}) || cursor != "" {
		t.Fatalf("second page keys = %v cursor = %q", keys, cursor)
	}
}

func TestPrefixedKVTransactionPrefixesKeys(t *testing.T) {
	ctx := context.Background()
	base := newFakeKV()
	kv := newPrefixedKV(base, "tenant-tx")

	if err := RunInTransaction(ctx, kv, func(tx ReadWriter) error {
		return tx.Set(ctx, "namros/bootstrap", []byte("tx"))
	}); err != nil {
		t.Fatalf("RunInTransaction() error = %v", err)
	}
	got, found, err := base.Get(ctx, "keyspaces/tenant-tx/namros/bootstrap")
	if err != nil || !found || string(got) != "tx" {
		t.Fatalf("base.Get(prefixed) = %q found %v err %v, want tx/true/nil", got, found, err)
	}
}

func TestInstrumentedKVRecordsHotspotMetrics(t *testing.T) {
	ctx := context.Background()
	base := newFakeKV()
	metrics := NewMetrics(WithHotspotShardCount(4), WithHotspotTopN(10))
	kv := newPrefixedKV(newInstrumentedKV(base, metrics), "tenant-metrics")
	key := "/namros/v1/buckets/bucket-000000000001/list/photos%2Fsmall.txt"

	if err := RunInTransaction(ctx, kv, func(tx ReadWriter) error {
		if err := tx.Set(ctx, key, []byte("small")); err != nil {
			return err
		}
		if _, _, err := tx.Get(ctx, key); err != nil {
			return err
		}
		_, _, err := tx.List(ctx, "/namros/v1/buckets/bucket-000000000001/list/", "", 10)
		return err
	}); err != nil {
		t.Fatalf("RunInTransaction() error = %v", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.Operations[OperationSet].Calls != 1 {
		t.Fatalf("set calls = %d, want 1", snapshot.Operations[OperationSet].Calls)
	}
	if snapshot.Operations[OperationGet].Calls != 1 {
		t.Fatalf("get calls = %d, want 1", snapshot.Operations[OperationGet].Calls)
	}
	if snapshot.Operations[OperationList].Calls != 1 || snapshot.Operations[OperationList].KeysReturned != 1 {
		t.Fatalf("list metrics = %+v, want one call and one returned key", snapshot.Operations[OperationList])
	}
	if snapshot.Operations[OperationTxn].Calls != 1 || snapshot.Operations[OperationTxn].DurationSamples != 1 {
		t.Fatalf("transaction metrics = %+v, want one timed transaction", snapshot.Operations[OperationTxn])
	}
	if snapshot.Operations[OperationSet].DurationSamples != 1 {
		t.Fatalf("set duration samples = %d, want 1", snapshot.Operations[OperationSet].DurationSamples)
	}

	var operations uint64
	for _, keyRange := range snapshot.HotspotRanges {
		if keyRange.Range == "/namros/v1/buckets/bucket-000000000001/list" {
			operations += keyRange.Operations
		}
	}
	if operations != 3 {
		t.Fatalf("list hotspot operations = %d, want 3", operations)
	}
}

func TestInstrumentedKVTreatsDomainTransactionAbortAsNonTiKVError(t *testing.T) {
	ctx := context.Background()
	base := newFakeKV()
	metrics := NewMetrics()
	kv := newInstrumentedKV(base, metrics)

	err := RunInTransaction(ctx, kv, func(ReadWriter) error {
		return meta.ErrNotFound
	})
	if !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("RunInTransaction() error = %v, want ErrNotFound", err)
	}

	snapshot := metrics.Snapshot()
	if snapshot.Operations[OperationTxn].Calls != 1 || snapshot.Operations[OperationTxn].Errors != 0 {
		t.Fatalf("transaction metrics = %+v, want one non-error domain abort", snapshot.Operations[OperationTxn])
	}
}

type fakeKV struct {
	mu     sync.Mutex
	values map[string][]byte
}

func newFakeKV() *fakeKV {
	return &fakeKV{values: make(map[string][]byte)}
}

func (kv *fakeKV) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := kv.values[key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), value...), true, nil
}

func (kv *fakeKV) Set(_ context.Context, key string, value []byte) error {
	kv.values[key] = append([]byte(nil), value...)
	return nil
}

func (kv *fakeKV) Delete(_ context.Context, key string) error {
	delete(kv.values, key)
	return nil
}

func (kv *fakeKV) List(_ context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	return kv.listRange(prefix, string(prefixRangeEnd([]byte(prefix))), cursor, limit, prefix)
}

func (kv *fakeKV) ListRange(_ context.Context, start, end, cursor string, limit int) ([]string, string, error) {
	return kv.listRange(start, end, cursor, limit, "")
}

func (kv *fakeKV) listRange(start, end, cursor string, limit int, prefixFilter string) ([]string, string, error) {
	if end != "" && start >= end {
		return nil, "", nil
	}
	keys := make([]string, 0, len(kv.values))
	for key := range kv.values {
		if key < start || (cursor != "" && key <= cursor) {
			continue
		}
		if end != "" && key >= end {
			continue
		}
		if prefixFilter != "" && !strings.HasPrefix(key, prefixFilter) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit > 0 && len(keys) > limit {
		return keys[:limit], keys[limit-1], nil
	}
	return keys, "", nil
}

func (kv *fakeKV) RunInTransaction(ctx context.Context, fn func(tx ReadWriter) error) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	before := make(map[string][]byte, len(kv.values))
	for key, value := range kv.values {
		before[key] = append([]byte(nil), value...)
	}
	if err := fn(kv); err != nil {
		kv.values = before
		return err
	}
	return nil
}

func TestFakeKVRollsBackTransactionsForTestCoverage(t *testing.T) {
	ctx := context.Background()
	kv := newFakeKV()
	err := kv.RunInTransaction(ctx, func(tx ReadWriter) error {
		if err := tx.Set(ctx, "key", []byte("value")); err != nil {
			return err
		}
		return errors.New("rollback")
	})
	if err == nil {
		t.Fatal("RunInTransaction() error = nil, want rollback error")
	}
	if _, found, err := kv.Get(ctx, "key"); err != nil || found {
		t.Fatalf("Get(after rollback) found = %v err = %v, want false/nil", found, err)
	}
}

func TestRunWithRetryRetriesTransientWriteConflict(t *testing.T) {
	ctx := context.Background()
	attempts := 0
	delays := make([]time.Duration, 0)
	metrics := NewMetrics()
	err := runWithRetry(ctx, RetryPolicy{
		MaxAttempts:    3,
		InitialBackoff: 5 * time.Millisecond,
		MaxBackoff:     20 * time.Millisecond,
	}, func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}, func() error {
		attempts++
		if attempts < 3 {
			return errors.New("write conflict optimistic transaction")
		}
		return nil
	}, metrics.ObserveRetry)
	if err != nil {
		t.Fatalf("runWithRetry() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if len(delays) != 2 || delays[0] != 5*time.Millisecond || delays[1] != 10*time.Millisecond {
		t.Fatalf("delays = %v, want [5ms 10ms]", delays)
	}
	snapshot := metrics.Snapshot()
	if snapshot.Retry.Transactions != 1 || snapshot.Retry.Attempts != 3 || snapshot.Retry.RetryAttempts != 2 || snapshot.Retry.RetriedTransactions != 1 {
		t.Fatalf("retry metrics = %+v, want one transaction with two retries", snapshot.Retry)
	}
	if snapshot.Retry.WriteConflicts != 2 || snapshot.Retry.TransientErrors != 0 || snapshot.Retry.Exhausted != 0 {
		t.Fatalf("retry conflict metrics = %+v, want two conflicts and no exhaustion", snapshot.Retry)
	}
	if snapshot.Retry.BackoffMs != 15 || snapshot.Retry.MaxAttemptsObserved != 3 || snapshot.Retry.FinalStatuses["ok"] != 1 {
		t.Fatalf("retry backoff/status metrics = %+v, want 15ms backoff and ok status", snapshot.Retry)
	}
}

func TestRunWithRetryDoesNotRetryDomainErrors(t *testing.T) {
	attempts := 0
	err := runWithRetry(context.Background(), RetryPolicy{MaxAttempts: 3}, nil, func() error {
		attempts++
		return meta.ErrNotFound
	})
	if !errors.Is(err, meta.ErrNotFound) {
		t.Fatalf("runWithRetry() error = %v, want ErrNotFound", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRunWithRetryNormalizesExhaustedWriteConflict(t *testing.T) {
	attempts := 0
	metrics := NewMetrics()
	err := runWithRetry(context.Background(), RetryPolicy{
		MaxAttempts:    2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	}, func(context.Context, time.Duration) error { return nil }, func() error {
		attempts++
		return errors.New("write conflict")
	}, metrics.ObserveRetry)
	if !errors.Is(err, meta.ErrCASConflict) {
		t.Fatalf("runWithRetry() error = %v, want ErrCASConflict", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	snapshot := metrics.Snapshot()
	if snapshot.Retry.WriteConflicts != 2 || snapshot.Retry.Exhausted != 1 || snapshot.Retry.FinalStatuses["cas_conflict"] != 1 {
		t.Fatalf("retry exhausted conflict metrics = %+v", snapshot.Retry)
	}
}

func TestRunWithRetryNormalizesExhaustedUnavailable(t *testing.T) {
	attempts := 0
	metrics := NewMetrics()
	err := runWithRetry(context.Background(), RetryPolicy{
		MaxAttempts:    2,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     time.Millisecond,
	}, func(context.Context, time.Duration) error { return nil }, func() error {
		attempts++
		return errors.New("rpc error: unavailable")
	}, metrics.ObserveRetry)
	if !errors.Is(err, meta.ErrUnavailable) {
		t.Fatalf("runWithRetry() error = %v, want ErrUnavailable", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	snapshot := metrics.Snapshot()
	if snapshot.Retry.TransientErrors != 2 || snapshot.Retry.Exhausted != 1 || snapshot.Retry.FinalStatuses["unavailable"] != 1 {
		t.Fatalf("retry exhausted unavailable metrics = %+v", snapshot.Retry)
	}
}

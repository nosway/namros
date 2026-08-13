package tikv

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/pingcap/kvproto/pkg/kvrpcpb"
	tikvconfig "github.com/tikv/client-go/v2/config"
	tikverr "github.com/tikv/client-go/v2/error"
	"github.com/tikv/client-go/v2/txnkv"

	"github.com/nosway/namros/internal/meta"
)

const (
	APIVersionV1               = "v1"
	APIVersionV2               = "v2"
	DefaultTimeout             = 3 * time.Second
	DefaultRetryMaxAttempts    = 3
	DefaultRetryInitialBackoff = 10 * time.Millisecond
	DefaultRetryMaxBackoff     = 100 * time.Millisecond
)

type Config struct {
	PDEndpoints []string
	Timeout     time.Duration
	APIVersion  string
	Keyspace    string
	TLS         TLSConfig
	Retry       RetryPolicy
	Metrics     *Metrics
}

type TLSConfig struct {
	CAPath   string
	CertPath string
	KeyPath  string
}

type RetryPolicy struct {
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

type KV interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error)
	ListRange(ctx context.Context, start, end, cursor string, limit int) ([]string, string, error)
}

type ReadWriter interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error)
	ListRange(ctx context.Context, start, end, cursor string, limit int) ([]string, string, error)
}

type transactionalKV interface {
	RunInTransaction(ctx context.Context, fn func(tx ReadWriter) error) error
}

func ValidateConfig(cfg Config) error {
	if len(cleanStringList(cfg.PDEndpoints)) == 0 {
		return errors.New("tikv pd endpoints are required")
	}
	switch normalizeAPIVersion(cfg.APIVersion) {
	case APIVersionV1, APIVersionV2:
	default:
		return fmt.Errorf("unsupported tikv api version %q", cfg.APIVersion)
	}
	if cfg.Timeout < 0 {
		return errors.New("tikv timeout cannot be negative")
	}
	tlsFields := 0
	for _, value := range []string{cfg.TLS.CAPath, cfg.TLS.CertPath, cfg.TLS.KeyPath} {
		if strings.TrimSpace(value) != "" {
			tlsFields++
		}
	}
	if tlsFields != 0 && tlsFields != 3 {
		return errors.New("tikv tls requires ca, cert, and key paths")
	}
	if err := validateRetryPolicy(cfg.Retry); err != nil {
		return err
	}
	return nil
}

func OpenKV(_ context.Context, cfg Config) (KV, func() error, error) {
	if err := ValidateConfig(cfg); err != nil {
		return nil, nil, err
	}
	restore := tikvconfig.UpdateGlobal(func(conf *tikvconfig.Config) {
		conf.Security = tikvconfig.NewSecurity(
			cfg.TLS.CAPath,
			cfg.TLS.CertPath,
			cfg.TLS.KeyPath,
			nil,
		)
	})
	defer restore()

	clientOpts := []txnkv.ClientOpt{txnkv.WithAPIVersion(parseAPIVersion(cfg.APIVersion))}
	if strings.TrimSpace(cfg.Keyspace) != "" && normalizeAPIVersion(cfg.APIVersion) == APIVersionV2 {
		clientOpts = append(clientOpts, txnkv.WithKeyspace(cfg.Keyspace))
	}
	client, err := txnkv.NewClient(cleanStringList(cfg.PDEndpoints), clientOpts...)
	if err != nil {
		return nil, nil, err
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultTimeout
	}
	kv := &txnKV{
		client:  client,
		timeout: timeout,
		retry:   normalizeRetryPolicy(cfg.Retry),
		sleeper: sleepRetry,
		metrics: cfg.Metrics,
	}
	store := newInstrumentedKV(kv, cfg.Metrics)
	if strings.TrimSpace(cfg.Keyspace) != "" && normalizeAPIVersion(cfg.APIVersion) != APIVersionV2 {
		return newPrefixedKV(store, cfg.Keyspace), kv.Close, nil
	}
	return store, kv.Close, nil
}

func RunInTransaction(ctx context.Context, store KV, fn func(tx ReadWriter) error) error {
	runner, ok := store.(transactionalKV)
	if !ok {
		return errors.New("tikv kv does not support transactions")
	}
	return runner.RunInTransaction(ctx, fn)
}

type txnKV struct {
	client  *txnkv.Client
	timeout time.Duration
	retry   RetryPolicy
	sleeper retrySleeper
	metrics *Metrics
}

func (kv *txnKV) Get(ctx context.Context, key string) ([]byte, bool, error) {
	ctx, cancel := kv.withTimeout(ctx)
	defer cancel()

	txn, err := kv.client.Begin()
	if err != nil {
		return nil, false, err
	}
	value, err := txn.Get(ctx, []byte(key))
	if tikverr.IsErrNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return append([]byte(nil), value...), true, nil
}

func (kv *txnKV) Set(ctx context.Context, key string, value []byte) error {
	return kv.runTxn(ctx, func(txn *txnkv.KVTxn) error {
		return txn.Set([]byte(key), append([]byte(nil), value...))
	})
}

func (kv *txnKV) Delete(ctx context.Context, key string) error {
	return kv.runTxn(ctx, func(txn *txnkv.KVTxn) error {
		return txn.Delete([]byte(key))
	})
}

func (kv *txnKV) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	return kv.listRange(ctx, prefix, string(prefixRangeEnd([]byte(prefix))), cursor, limit, prefix)
}

func (kv *txnKV) ListRange(ctx context.Context, start, end, cursor string, limit int) ([]string, string, error) {
	return kv.listRange(ctx, start, end, cursor, limit, "")
}

func (kv *txnKV) listRange(ctx context.Context, start, end, cursor string, limit int, prefixFilter string) ([]string, string, error) {
	if limit <= 0 {
		limit = 128
	}
	if end != "" && start >= end {
		return nil, "", nil
	}
	ctx, cancel := kv.withTimeout(ctx)
	defer cancel()

	snapshot := kv.client.GetSnapshot(math.MaxUint64)
	startKey := []byte(start)
	if cursor != "" && (start == "" || cursor >= start) && (end == "" || cursor < end) {
		startKey = nextLexicographicKey([]byte(cursor))
	} else if cursor != "" && end != "" && cursor >= end {
		return nil, "", nil
	}
	var endKey []byte
	if end != "" {
		endKey = []byte(end)
	}
	iter, err := snapshot.Iter(startKey, endKey)
	if err != nil {
		return nil, "", err
	}
	defer iter.Close()

	keys := make([]string, 0, limit)
	for iter.Valid() {
		key := string(iter.Key())
		if end != "" && key >= end {
			break
		}
		if prefixFilter != "" && !strings.HasPrefix(key, prefixFilter) {
			break
		}
		keys = append(keys, key)
		if len(keys) >= limit {
			return keys, key, nil
		}
		if err := iter.Next(); err != nil {
			return nil, "", err
		}
	}
	return keys, "", nil
}

func (kv *txnKV) RunInTransaction(ctx context.Context, fn func(tx ReadWriter) error) error {
	return kv.runTxn(ctx, func(txn *txnkv.KVTxn) error {
		return fn(&txnReadWriter{txn: txn})
	})
}

func (kv *txnKV) runTxn(ctx context.Context, fn func(txn *txnkv.KVTxn) error) error {
	ctx, cancel := kv.withTimeout(ctx)
	defer cancel()

	return runWithRetry(ctx, kv.retry, kv.sleeper, func() error {
		attemptStart := time.Now()
		txn, err := kv.client.Begin()
		if err != nil {
			kv.observeTransactionAttempt(time.Since(attemptStart), err)
			return err
		}
		if err := fn(txn); err != nil {
			kv.observeTransactionAttempt(time.Since(attemptStart), transactionMetricError(err))
			return err
		}
		commitStart := time.Now()
		err = txn.Commit(ctx)
		kv.observeTransactionCommit(time.Since(commitStart), err)
		kv.observeTransactionAttempt(time.Since(attemptStart), err)
		return err
	}, kv.observeRetry)
}

func (kv *txnKV) observeTransactionAttempt(duration time.Duration, err error) {
	if kv == nil || kv.metrics == nil {
		return
	}
	kv.metrics.ObserveOperationDuration(OperationAttempt, "/namros/v1/tikv/transactions", duration, err)
}

func (kv *txnKV) observeTransactionCommit(duration time.Duration, err error) {
	if kv == nil || kv.metrics == nil {
		return
	}
	kv.metrics.ObserveOperationDuration(OperationCommit, "/namros/v1/tikv/transactions", duration, err)
}

func (kv *txnKV) observeRetry(obs RetryObservation) {
	if kv == nil || kv.metrics == nil {
		return
	}
	kv.metrics.ObserveRetry(obs)
}

func transactionMetricError(err error) error {
	if err == nil {
		return nil
	}
	for _, domainErr := range []error{
		meta.ErrInvalidArgument,
		meta.ErrNotFound,
		meta.ErrAlreadyExists,
		meta.ErrBucketNotEmpty,
		meta.ErrCASConflict,
		meta.ErrObjectLocked,
	} {
		if errors.Is(err, domainErr) {
			return nil
		}
	}
	return err
}

func (kv *txnKV) Close() error {
	if kv == nil || kv.client == nil {
		return nil
	}
	return kv.client.Close()
}

func (kv *txnKV) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok || kv.timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, kv.timeout)
}

type txnReadWriter struct {
	txn *txnkv.KVTxn
}

func (tx *txnReadWriter) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := tx.txn.Get(ctx, []byte(key))
	if tikverr.IsErrNotFound(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return append([]byte(nil), value...), true, nil
}

func (tx *txnReadWriter) Set(_ context.Context, key string, value []byte) error {
	return tx.txn.Set([]byte(key), append([]byte(nil), value...))
}

func (tx *txnReadWriter) Delete(_ context.Context, key string) error {
	return tx.txn.Delete([]byte(key))
}

func (tx *txnReadWriter) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	return tx.listRange(ctx, prefix, string(prefixRangeEnd([]byte(prefix))), cursor, limit, prefix)
}

func (tx *txnReadWriter) ListRange(ctx context.Context, start, end, cursor string, limit int) ([]string, string, error) {
	return tx.listRange(ctx, start, end, cursor, limit, "")
}

func (tx *txnReadWriter) listRange(ctx context.Context, start, end, cursor string, limit int, prefixFilter string) ([]string, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if limit <= 0 {
		limit = 128
	}
	if end != "" && start >= end {
		return nil, "", nil
	}
	startKey := []byte(start)
	if cursor != "" && (start == "" || cursor >= start) && (end == "" || cursor < end) {
		startKey = nextLexicographicKey([]byte(cursor))
	} else if cursor != "" && end != "" && cursor >= end {
		return nil, "", nil
	}
	var endKey []byte
	if end != "" {
		endKey = []byte(end)
	}
	iter, err := tx.txn.Iter(startKey, endKey)
	if err != nil {
		return nil, "", err
	}
	defer iter.Close()

	keys := make([]string, 0, limit)
	for iter.Valid() {
		key := string(iter.Key())
		if end != "" && key >= end {
			break
		}
		if prefixFilter != "" && !strings.HasPrefix(key, prefixFilter) {
			break
		}
		keys = append(keys, key)
		if len(keys) >= limit {
			return keys, key, nil
		}
		if err := iter.Next(); err != nil {
			return nil, "", err
		}
	}
	return keys, "", nil
}

func isWriteConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "write conflict") || strings.Contains(msg, "optimistic")
}

type retrySleeper func(context.Context, time.Duration) error
type retryObserver func(RetryObservation)

func runWithRetry(ctx context.Context, policy RetryPolicy, sleeper retrySleeper, fn func() error, observers ...retryObserver) (retErr error) {
	policy = normalizeRetryPolicy(policy)
	if sleeper == nil {
		sleeper = sleepRetry
	}
	obs := RetryObservation{MaxAttempts: policy.MaxAttempts}
	defer func() {
		obs.FinalError = retErr
		for _, observer := range observers {
			if observer != nil {
				observer(obs)
			}
		}
	}()
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		obs.Attempts++
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		retryable := shouldRetryTransactionError(err)
		if isWriteConflict(err) {
			obs.WriteConflicts++
		} else if retryable {
			obs.TransientErrors++
		}
		if attempt >= policy.MaxAttempts || !retryable {
			obs.Exhausted = retryable && attempt >= policy.MaxAttempts
			return normalizeTransactionError(err)
		}
		delay := retryDelay(policy, attempt)
		obs.RetryAttempts++
		obs.Backoff += delay
		if err := sleeper(ctx, delay); err != nil {
			return err
		}
	}
	return normalizeTransactionError(lastErr)
}

func sleepRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func shouldRetryTransactionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	for _, domainErr := range []error{
		meta.ErrInvalidArgument,
		meta.ErrNotFound,
		meta.ErrAlreadyExists,
		meta.ErrBucketNotEmpty,
		meta.ErrCASConflict,
	} {
		if errors.Is(err, domainErr) {
			return false
		}
	}
	if isWriteConflict(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, token := range []string{
		"backoff",
		"connection refused",
		"connection reset",
		"epoch not match",
		"not leader",
		"region is unavailable",
		"rpc error",
		"server is busy",
		"stale epoch",
		"temporarily unavailable",
		"transport is closing",
		"try again",
		"unavailable",
	} {
		if strings.Contains(msg, token) {
			return true
		}
	}
	return false
}

func normalizeTransactionError(err error) error {
	if isWriteConflict(err) {
		return meta.ErrCASConflict
	}
	if shouldRetryTransactionError(err) {
		return fmt.Errorf("%w: %v", meta.ErrUnavailable, err)
	}
	return err
}

func retryDelay(policy RetryPolicy, attempt int) time.Duration {
	delay := policy.InitialBackoff
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= policy.MaxBackoff {
			return policy.MaxBackoff
		}
	}
	if delay > policy.MaxBackoff {
		return policy.MaxBackoff
	}
	return delay
}

func validateRetryPolicy(policy RetryPolicy) error {
	if policy.MaxAttempts < 0 {
		return errors.New("tikv retry max attempts cannot be negative")
	}
	if policy.InitialBackoff < 0 {
		return errors.New("tikv retry initial backoff cannot be negative")
	}
	if policy.MaxBackoff < 0 {
		return errors.New("tikv retry max backoff cannot be negative")
	}
	normalized := normalizeRetryPolicy(policy)
	if normalized.InitialBackoff > normalized.MaxBackoff {
		return errors.New("tikv retry initial backoff cannot exceed max backoff")
	}
	return nil
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	if policy.MaxAttempts == 0 {
		policy.MaxAttempts = DefaultRetryMaxAttempts
	}
	if policy.InitialBackoff == 0 {
		policy.InitialBackoff = DefaultRetryInitialBackoff
	}
	if policy.MaxBackoff == 0 {
		policy.MaxBackoff = DefaultRetryMaxBackoff
	}
	return policy
}

func parseAPIVersion(version string) kvrpcpb.APIVersion {
	switch normalizeAPIVersion(version) {
	case APIVersionV2:
		return kvrpcpb.APIVersion_V2
	default:
		return kvrpcpb.APIVersion_V1
	}
}

func normalizeAPIVersion(version string) string {
	version = strings.ToLower(strings.TrimSpace(version))
	if version == "" {
		return APIVersionV1
	}
	return version
}

func prefixRangeEnd(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	end := append([]byte(nil), prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xFF {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}

func nextLexicographicKey(key []byte) []byte {
	next := make([]byte, len(key)+1)
	copy(next, key)
	return next
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

package tikv

import (
	"context"
	"strings"
)

type prefixedKV struct {
	base   KV
	prefix string
}

type prefixedTxn struct {
	base   ReadWriter
	prefix string
}

func newPrefixedKV(base KV, keyspace string) KV {
	keyspace = strings.Trim(strings.TrimSpace(keyspace), "/")
	if keyspace == "" {
		return base
	}
	return &prefixedKV{
		base:   base,
		prefix: "keyspaces/" + keyspace + "/",
	}
}

func (kv *prefixedKV) prefixed(key string) string {
	return kv.prefix + key
}

func (kv *prefixedKV) unprefixed(key string) string {
	return strings.TrimPrefix(key, kv.prefix)
}

func (kv *prefixedKV) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return kv.base.Get(ctx, kv.prefixed(key))
}

func (kv *prefixedKV) Set(ctx context.Context, key string, value []byte) error {
	return kv.base.Set(ctx, kv.prefixed(key), value)
}

func (kv *prefixedKV) Delete(ctx context.Context, key string) error {
	return kv.base.Delete(ctx, kv.prefixed(key))
}

func (kv *prefixedKV) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	prefixedCursor := ""
	if cursor != "" {
		prefixedCursor = kv.prefixed(cursor)
	}
	keys, next, err := kv.base.List(ctx, kv.prefixed(prefix), prefixedCursor, limit)
	if err != nil {
		return nil, "", err
	}
	for i, key := range keys {
		keys[i] = kv.unprefixed(key)
	}
	if next != "" {
		next = kv.unprefixed(next)
	}
	return keys, next, nil
}

func (kv *prefixedKV) ListRange(ctx context.Context, start, end, cursor string, limit int) ([]string, string, error) {
	prefixedCursor := ""
	if cursor != "" {
		prefixedCursor = kv.prefixed(cursor)
	}
	prefixedEnd := string(prefixRangeEnd([]byte(kv.prefix)))
	if end != "" {
		prefixedEnd = kv.prefixed(end)
	}
	keys, next, err := kv.base.ListRange(ctx, kv.prefixed(start), prefixedEnd, prefixedCursor, limit)
	if err != nil {
		return nil, "", err
	}
	for i, key := range keys {
		keys[i] = kv.unprefixed(key)
	}
	if next != "" {
		next = kv.unprefixed(next)
	}
	return keys, next, nil
}

func (kv *prefixedKV) RunInTransaction(ctx context.Context, fn func(tx ReadWriter) error) error {
	return RunInTransaction(ctx, kv.base, func(tx ReadWriter) error {
		return fn(&prefixedTxn{base: tx, prefix: kv.prefix})
	})
}

func (tx *prefixedTxn) prefixed(key string) string {
	return tx.prefix + key
}

func (tx *prefixedTxn) Get(ctx context.Context, key string) ([]byte, bool, error) {
	return tx.base.Get(ctx, tx.prefixed(key))
}

func (tx *prefixedTxn) Set(ctx context.Context, key string, value []byte) error {
	return tx.base.Set(ctx, tx.prefixed(key), value)
}

func (tx *prefixedTxn) Delete(ctx context.Context, key string) error {
	return tx.base.Delete(ctx, tx.prefixed(key))
}

func (tx *prefixedTxn) List(ctx context.Context, prefix, cursor string, limit int) ([]string, string, error) {
	prefixedCursor := ""
	if cursor != "" {
		prefixedCursor = tx.prefixed(cursor)
	}
	keys, next, err := tx.base.List(ctx, tx.prefixed(prefix), prefixedCursor, limit)
	if err != nil {
		return nil, "", err
	}
	for i, key := range keys {
		keys[i] = strings.TrimPrefix(key, tx.prefix)
	}
	if next != "" {
		next = strings.TrimPrefix(next, tx.prefix)
	}
	return keys, next, nil
}

func (tx *prefixedTxn) ListRange(ctx context.Context, start, end, cursor string, limit int) ([]string, string, error) {
	prefixedCursor := ""
	if cursor != "" {
		prefixedCursor = tx.prefixed(cursor)
	}
	prefixedEnd := string(prefixRangeEnd([]byte(tx.prefix)))
	if end != "" {
		prefixedEnd = tx.prefixed(end)
	}
	keys, next, err := tx.base.ListRange(ctx, tx.prefixed(start), prefixedEnd, prefixedCursor, limit)
	if err != nil {
		return nil, "", err
	}
	for i, key := range keys {
		keys[i] = strings.TrimPrefix(key, tx.prefix)
	}
	if next != "" {
		next = strings.TrimPrefix(next, tx.prefix)
	}
	return keys, next, nil
}

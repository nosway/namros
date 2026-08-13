package tikv

import (
	"context"
	"time"

	"github.com/nosway/namros/internal/meta/kvrepo"
)

type Repository = kvrepo.Repository

func Open(ctx context.Context, cfg Config) (*Repository, error) {
	return OpenWithClock(ctx, cfg, func() time.Time { return time.Now().UTC() })
}

func OpenWithClock(ctx context.Context, cfg Config, now func() time.Time) (*Repository, error) {
	kv, cleanup, err := OpenKV(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return kvrepo.NewWithClock(&store{kv: kv, cleanup: cleanup}, now), nil
}

type store struct {
	kv      KV
	cleanup func() error
}

func (s *store) RunInTransaction(ctx context.Context, fn func(tx kvrepo.ReadWriter) error) error {
	return RunInTransaction(ctx, s.kv, func(tx ReadWriter) error {
		return fn(txAdapter{ctx: ctx, tx: tx})
	})
}

func (s *store) Close() error {
	if s.cleanup == nil {
		return nil
	}
	cleanup := s.cleanup
	s.cleanup = nil
	return cleanup()
}

type txAdapter struct {
	ctx context.Context
	tx  ReadWriter
}

func (tx txAdapter) Get(key string) ([]byte, bool, error) {
	return tx.tx.Get(tx.ctx, key)
}

func (tx txAdapter) Set(key string, value []byte) error {
	return tx.tx.Set(tx.ctx, key, value)
}

func (tx txAdapter) Delete(key string) error {
	return tx.tx.Delete(tx.ctx, key)
}

func (tx txAdapter) List(prefix, cursor string, limit int) ([]string, string, error) {
	return tx.tx.List(tx.ctx, prefix, cursor, limit)
}

func (tx txAdapter) ListRange(start, end, cursor string, limit int) ([]string, string, error) {
	return tx.tx.ListRange(tx.ctx, start, end, cursor, limit)
}

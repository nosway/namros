package pebble

import (
	"context"
	"errors"
	"strings"
	"sync"

	pebbledb "github.com/cockroachdb/pebble"

	"github.com/nosway/namros/internal/meta/kvrepo"
)

type store struct {
	mu sync.Mutex
	db *pebbledb.DB
}

func newStore(db *pebbledb.DB) *store {
	return &store{db: db}
}

func (s *store) RunInTransaction(ctx context.Context, fn func(tx kvrepo.ReadWriter) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return errors.New("pebble repository is closed")
	}
	batch := s.db.NewIndexedBatch()
	defer batch.Close()
	tx := pebbleTx{batch: batch}
	if err := fn(tx); err != nil {
		return err
	}
	return batch.Commit(pebbledb.Sync)
}

func (s *store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	db := s.db
	s.db = nil
	return db.Close()
}

type pebbleTx struct {
	batch *pebbledb.Batch
}

func (tx pebbleTx) Get(key string) ([]byte, bool, error) {
	value, closer, err := tx.batch.Get([]byte(key))
	if errors.Is(err, pebbledb.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer closer.Close()
	return append([]byte(nil), value...), true, nil
}

func (tx pebbleTx) Set(key string, value []byte) error {
	return tx.batch.Set([]byte(key), append([]byte(nil), value...), pebbledb.NoSync)
}

func (tx pebbleTx) Delete(key string) error {
	return tx.batch.Delete([]byte(key), pebbledb.NoSync)
}

func (tx pebbleTx) List(prefix, cursor string, limit int) ([]string, string, error) {
	return tx.listRange(prefix, string(prefixUpperBound([]byte(prefix))), cursor, limit, prefix)
}

func (tx pebbleTx) ListRange(start, end, cursor string, limit int) ([]string, string, error) {
	return tx.listRange(start, end, cursor, limit, "")
}

func (tx pebbleTx) listRange(start, end, cursor string, limit int, prefixFilter string) ([]string, string, error) {
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
	var lowerBound []byte
	if start != "" {
		lowerBound = []byte(start)
	}
	var upperBound []byte
	if end != "" {
		upperBound = []byte(end)
	}
	iter, err := tx.batch.NewIter(&pebbledb.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return nil, "", err
	}
	defer iter.Close()

	keys := make([]string, 0, limit)
	for valid := iter.SeekGE(startKey); valid; valid = iter.Next() {
		key := string(append([]byte(nil), iter.Key()...))
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
	}
	return keys, "", iter.Error()
}

func prefixUpperBound(prefix []byte) []byte {
	if len(prefix) == 0 {
		return nil
	}
	out := append([]byte(nil), prefix...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] != 0xff {
			out[i]++
			return out[:i+1]
		}
	}
	return nil
}

func nextLexicographicKey(key []byte) []byte {
	next := make([]byte, len(key)+1)
	copy(next, key)
	return next
}

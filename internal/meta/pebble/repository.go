package pebble

import (
	"fmt"
	"strings"
	"time"

	pebbledb "github.com/cockroachdb/pebble"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/kvrepo"
)

type Repository = kvrepo.Repository

func Open(path string) (*Repository, error) {
	return OpenWithClock(path, func() time.Time { return time.Now().UTC() })
}

func OpenWithClock(path string, now func() time.Time) (*Repository, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: pebble metadata path is required", meta.ErrInvalidArgument)
	}
	db, err := pebbledb.Open(path, &pebbledb.Options{})
	if err != nil {
		return nil, err
	}
	return kvrepo.NewWithClock(newStore(db), now), nil
}

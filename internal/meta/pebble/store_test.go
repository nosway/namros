package pebble

import (
	"fmt"
	"reflect"
	"testing"

	pebbledb "github.com/cockroachdb/pebble"

	"github.com/nosway/namros/internal/meta/kvrepo"
)

func TestStoreListRangeBoundsCursorAndLimit(t *testing.T) {
	db, err := pebbledb.Open(t.TempDir(), &pebbledb.Options{})
	if err != nil {
		t.Fatalf("pebble.Open() error = %v", err)
	}
	store := newStore(db)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	err = store.RunInTransaction(t.Context(), func(tx kvrepo.ReadWriter) error {
		for _, key := range []string{
			"/range/a",
			"/range/aa",
			"/range/b",
			"/range/c",
			"/range/z",
			"/range2/a",
		} {
			if err := tx.Set(key, []byte(key)); err != nil {
				return err
			}
		}
		keys, cursor, err := tx.ListRange("/range/a", "/range/c", "", 2)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(keys, []string{"/range/a", "/range/aa"}) || cursor != "/range/aa" {
			return fmt.Errorf("first ListRange page keys = %v cursor = %q", keys, cursor)
		}
		keys, cursor, err = tx.ListRange("/range/a", "/range/c", cursor, 2)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(keys, []string{"/range/b"}) || cursor != "" {
			return fmt.Errorf("second ListRange page keys = %v cursor = %q", keys, cursor)
		}
		keys, cursor, err = tx.ListRange("/range/a", "/range/c", "/range/c", 2)
		if err != nil {
			return err
		}
		if len(keys) != 0 || cursor != "" {
			return fmt.Errorf("past-end ListRange keys = %v cursor = %q", keys, cursor)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTransaction() error = %v", err)
	}
}

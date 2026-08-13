package local_test

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/storage/local"
	"github.com/nosway/namros/internal/storage/testsuite"
)

func TestSegmentStoreSuite(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	testsuite.RunSegmentStoreTests(t, func(t *testing.T) testsuite.SegmentStoreUnderTest {
		t.Helper()
		store, err := local.NewWithClock(t.TempDir(), func() time.Time { return now })
		if err != nil {
			t.Fatalf("local.NewWithClock() error = %v", err)
		}
		return store
	})
}

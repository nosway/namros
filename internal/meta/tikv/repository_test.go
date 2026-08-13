package tikv

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta/kvrepo"
	"github.com/nosway/namros/internal/meta/testsuite"
)

func TestRepositorySuiteWithFakeKV(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	testsuite.RunRepositoryTests(t, func(t *testing.T) testsuite.RepositoryUnderTest {
		t.Helper()
		return kvrepo.NewWithClock(&store{
			kv:      newFakeKV(),
			cleanup: func() error { return nil },
		}, func() time.Time { return now })
	})
}

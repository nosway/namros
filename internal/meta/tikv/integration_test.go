package tikv

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta/kvrepo"
	"github.com/nosway/namros/internal/meta/testsuite"
)

func TestRepositorySuiteIntegration(t *testing.T) {
	endpoints := cleanStringList(strings.Split(os.Getenv("NAMROS_TIKV_PD_ENDPOINTS"), ","))
	if len(endpoints) == 0 {
		t.Skip("set NAMROS_TIKV_PD_ENDPOINTS to run TiKV integration tests")
	}
	apiVersion := strings.TrimSpace(os.Getenv("NAMROS_TIKV_API_VERSION"))
	if apiVersion == "" {
		apiVersion = APIVersionV1
	}
	keyspacePrefix := strings.TrimSpace(os.Getenv("NAMROS_TIKV_KEYSPACE_PREFIX"))
	if keyspacePrefix == "" {
		keyspacePrefix = "namros-it"
	}
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	var sequence uint64

	testsuite.RunRepositoryTests(t, func(t *testing.T) testsuite.RepositoryUnderTest {
		t.Helper()
		keyspace := fmt.Sprintf("%s-%d-%d", keyspacePrefix, time.Now().UnixNano(), atomic.AddUint64(&sequence, 1))
		kv, cleanup, err := OpenKV(t.Context(), Config{
			PDEndpoints: endpoints,
			APIVersion:  apiVersion,
			Keyspace:    keyspace,
			Timeout:     10 * time.Second,
		})
		if err != nil {
			t.Fatalf("OpenKV() error = %v", err)
		}
		repo := kvrepo.NewWithClock(&store{kv: kv, cleanup: cleanup}, func() time.Time { return now })
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := cleanupKV(cleanupCtx, kv); err != nil {
				t.Fatalf("cleanup TiKV keyspace %q: %v", keyspace, err)
			}
			if err := repo.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
		return repo
	})
}

func cleanupKV(ctx context.Context, kv KV) error {
	cursor := ""
	for {
		keys, next, err := kv.List(ctx, "", cursor, 128)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := kv.Delete(ctx, key); err != nil {
				return err
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

package pebble_test

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/pebble"
	"github.com/nosway/namros/internal/meta/testsuite"
)

func TestRepositorySuite(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	testsuite.RunRepositoryTests(t, func(t *testing.T) testsuite.RepositoryUnderTest {
		t.Helper()
		repo, err := pebble.OpenWithClock(t.TempDir(), func() time.Time { return now })
		if err != nil {
			t.Fatalf("OpenWithClock() error = %v", err)
		}
		t.Cleanup(func() {
			if err := repo.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		})
		return repo
	})
}

func TestRepositorySurvivesReopen(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	path := t.TempDir()
	repo, err := pebble.OpenWithClock(path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("OpenWithClock() error = %v", err)
	}
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "photos",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "raw/image.jpg",
		SizeBytes: 12,
		ETag:      `"etag-1"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "raw/image.jpg",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if err := repo.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := pebble.OpenWithClock(path, func() time.Time { return now })
	if err != nil {
		t.Fatalf("reopen OpenWithClock() error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Fatalf("reopened Close() error = %v", err)
		}
	})
	gotBucket, err := reopened.GetBucketByName(t.Context(), "photos")
	if err != nil {
		t.Fatalf("GetBucketByName() after reopen error = %v", err)
	}
	if gotBucket.BucketID != bucket.BucketID {
		t.Fatalf("bucket id after reopen = %q, want %q", gotBucket.BucketID, bucket.BucketID)
	}
	head, err := reopened.GetObjectHead(t.Context(), bucket.BucketID, "raw/image.jpg")
	if err != nil {
		t.Fatalf("GetObjectHead() after reopen error = %v", err)
	}
	if head.VersionID != pending.Version.VersionID || head.ETag != `"etag-1"` {
		t.Fatalf("head after reopen = %+v", head)
	}
	second, err := reopened.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "archive",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(second) error = %v", err)
	}
	if second.BucketID == bucket.BucketID {
		t.Fatalf("second bucket reused id %q", second.BucketID)
	}
}

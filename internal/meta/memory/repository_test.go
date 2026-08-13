package memory_test

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	metaid "github.com/nosway/namros/internal/meta/id"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/meta/testsuite"
)

func TestRepositorySuite(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	testsuite.RunRepositoryTests(t, func(t *testing.T) testsuite.RepositoryUnderTest {
		t.Helper()
		return memory.NewWithClock(func() time.Time { return now })
	})
}

func TestCreateBucketUsesDistributedIDGenerator(t *testing.T) {
	repo := memory.NewWithClock(
		func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) },
		memory.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
			metaid.KindBucket: {"bkt_test_bucket_1"},
		})),
	)

	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "distributed-id",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if bucket.BucketID != "bkt_test_bucket_1" {
		t.Fatalf("BucketID = %q, want generated id", bucket.BucketID)
	}
}

func TestCreateBucketRetriesDistributedIDCollision(t *testing.T) {
	repo := memory.NewWithClock(
		func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) },
		memory.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
			metaid.KindBucket: {"bkt_collision", "bkt_collision", "bkt_after_collision"},
		})),
	)

	first, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "first",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(first) error = %v", err)
	}
	second, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "second",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(second) error = %v", err)
	}
	if first.BucketID != "bkt_collision" || second.BucketID != "bkt_after_collision" {
		t.Fatalf("bucket ids = %q / %q, want collision retry to allocate second id", first.BucketID, second.BucketID)
	}
}

func TestObjectWritesUseDistributedIDGenerator(t *testing.T) {
	repo := memory.NewWithClock(
		func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) },
		memory.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
			metaid.KindBucket:  {"bkt_object_ids"},
			metaid.KindVersion: {"ver_pending", "ver_direct", "ver_marker"},
			metaid.KindUpload:  {"upl_upload"},
		})),
	)

	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "object-ids",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "pending.txt",
		ETag:     `"pending"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	if pending.Version.VersionID != "ver_pending" || pending.Version.VersionSortKey != "ver_pending" {
		t.Fatalf("pending version = %+v, want generated version id and sort key", pending.Version)
	}
	if _, err := repo.PutBucketVersioning(t.Context(), meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	direct, err := repo.PutObjectVersion(t.Context(), meta.PutObjectVersionRequest{
		BucketID: bucket.BucketID,
		Key:      "direct.txt",
		ETag:     `"direct"`,
	})
	if err != nil {
		t.Fatalf("PutObjectVersion() error = %v", err)
	}
	if direct.Head.VersionID != "ver_direct" {
		t.Fatalf("direct head version = %q, want generated id", direct.Head.VersionID)
	}
	upload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "multipart.txt",
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	if upload.UploadID != "upl_upload" {
		t.Fatalf("UploadID = %q, want generated upload id", upload.UploadID)
	}
	deleted, err := repo.DeleteObject(t.Context(), meta.DeleteObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "direct.txt",
	})
	if err != nil {
		t.Fatalf("DeleteObject() error = %v", err)
	}
	if deleted.DeletedVersionID != "ver_marker" || deleted.DeletedVersion.VersionSortKey != "ver_marker" {
		t.Fatalf("delete marker = %+v, want generated version id and sort key", deleted)
	}
}

func TestObjectWriteIDsRetryDistributedIDCollision(t *testing.T) {
	repo := memory.NewWithClock(
		func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) },
		memory.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
			metaid.KindBucket:  {"bkt_object_collision"},
			metaid.KindVersion: {"ver_collision", "ver_collision", "ver_after_collision"},
			metaid.KindUpload:  {"upl_collision", "upl_collision", "upl_after_collision"},
		})),
	)

	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "object-id-collision",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	firstVersion, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{BucketID: bucket.BucketID, Key: "same.txt"})
	if err != nil {
		t.Fatalf("BeginPutObject(first) error = %v", err)
	}
	secondVersion, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{BucketID: bucket.BucketID, Key: "same.txt"})
	if err != nil {
		t.Fatalf("BeginPutObject(second) error = %v", err)
	}
	if firstVersion.Version.VersionID != "ver_collision" || secondVersion.Version.VersionID != "ver_after_collision" {
		t.Fatalf("version ids = %q / %q, want collision retry", firstVersion.Version.VersionID, secondVersion.Version.VersionID)
	}
	firstUpload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{BucketID: bucket.BucketID, Key: "a.bin"})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(first) error = %v", err)
	}
	secondUpload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{BucketID: bucket.BucketID, Key: "b.bin"})
	if err != nil {
		t.Fatalf("CreateMultipartUpload(second) error = %v", err)
	}
	if firstUpload.UploadID != "upl_collision" || secondUpload.UploadID != "upl_after_collision" {
		t.Fatalf("upload ids = %q / %q, want collision retry", firstUpload.UploadID, secondUpload.UploadID)
	}
}

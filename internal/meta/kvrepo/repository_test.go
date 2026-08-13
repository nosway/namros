package kvrepo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	metaid "github.com/nosway/namros/internal/meta/id"
	"github.com/nosway/namros/internal/meta/keyspace"
	"github.com/nosway/namros/internal/meta/kvrepo"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/meta/testsuite"
	"github.com/nosway/namros/internal/storage"
)

func TestRepositorySuite(t *testing.T) {
	now := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	testsuite.RunRepositoryTests(t, func(t *testing.T) testsuite.RepositoryUnderTest {
		t.Helper()
		return kvrepo.NewWithClock(newMemoryStore(), func() time.Time { return now })
	})
}

func TestListIndexStoresLightweightEntry(t *testing.T) {
	now := time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "bucket-list-lightweight",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "large.bin",
		SizeBytes: 2,
		ETag:      `"etag"`,
		SegmentRefs: []storage.SegmentRef{
			{SegmentID: "segment-1", SizeBytes: 1},
			{SegmentID: "segment-2", SizeBytes: 1},
		},
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "large.bin",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}

	rawHead := store.values[keyspace.ObjectHead(bucket.BucketID, "large.bin")]
	if len(rawHead) == 0 {
		t.Fatalf("missing object head value")
	}
	if bytes.Contains(rawHead, []byte("SegmentRefs")) || bytes.Contains(rawHead, []byte("segment-1")) {
		t.Fatalf("object head value contains manifest data: %s", string(rawHead))
	}
	hydrated, err := repo.GetObjectHead(t.Context(), bucket.BucketID, "large.bin")
	if err != nil {
		t.Fatalf("GetObjectHead() error = %v", err)
	}
	if len(hydrated.SegmentRefs) != 2 || hydrated.SegmentRefs[0].SegmentID != "segment-1" {
		t.Fatalf("hydrated head refs = %+v", hydrated.SegmentRefs)
	}

	raw := store.values[keyspace.ListObject(bucket.BucketID, "large.bin")]
	if len(raw) == 0 {
		t.Fatalf("missing list index value")
	}
	if bytes.Contains(raw, []byte("SegmentRefs")) || bytes.Contains(raw, []byte("segment-1")) {
		t.Fatalf("list index value contains manifest data: %s", string(raw))
	}
	var entry model.ObjectListEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		t.Fatalf("unmarshal list entry: %v", err)
	}
	if entry.Key != "large.bin" || entry.VersionID != pending.Version.VersionID || entry.SizeBytes != 2 {
		t.Fatalf("list entry = %+v", entry)
	}
	listed, err := repo.ListObjects(t.Context(), meta.ListObjectsRequest{BucketID: bucket.BucketID})
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	if len(listed.Contents) != 1 || len(listed.Contents[0].SegmentRefs) != 0 {
		t.Fatalf("listed contents = %+v, want lightweight head without segment refs", listed.Contents)
	}
}

func TestGetMultipartPartsUsesPointReads(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "part-point-read",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	upload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "object.bin",
		StorageClass: storage.StorageClassSnapshot{
			StorageClassID: "STANDARD",
			Backend:        "local",
		},
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	for partNumber := 1; partNumber <= 3; partNumber++ {
		if _, _, err := repo.PutMultipartPart(t.Context(), meta.PutMultipartPartRequest{
			BucketID:   bucket.BucketID,
			Key:        "object.bin",
			UploadID:   upload.UploadID,
			PartNumber: partNumber,
			SizeBytes:  int64(partNumber),
			ETag:       fmt.Sprintf(`"%032x"`, partNumber),
			SegmentRef: storage.SegmentRef{
				SegmentID: fmt.Sprintf("segment-%d", partNumber),
				SizeBytes: uint64(partNumber),
			},
		}); err != nil {
			t.Fatalf("PutMultipartPart(%d) error = %v", partNumber, err)
		}
	}
	store.resetListTracking()
	store.mu.Lock()
	store.failListPrefix = strings.TrimSuffix(keyspace.MultipartPart(bucket.BucketID, upload.UploadID, 0), "00000")
	store.mu.Unlock()

	parts, err := repo.GetMultipartParts(t.Context(), meta.GetMultipartPartsRequest{
		BucketID:    bucket.BucketID,
		Key:         "object.bin",
		UploadID:    upload.UploadID,
		PartNumbers: []int{2, 4, 1},
	})
	if err != nil {
		t.Fatalf("GetMultipartParts() error = %v", err)
	}
	if len(parts) != 2 || parts[0].PartNumber != 2 || parts[1].PartNumber != 1 {
		t.Fatalf("parts = %+v, want point-read results in requested order", parts)
	}
	rangeCalls, listCalls := store.listTracking()
	if len(rangeCalls) != 0 || len(listCalls) != 0 {
		t.Fatalf("list calls = range:%+v prefix:%+v, want none", rangeCalls, listCalls)
	}
}

func TestObjectVersionStoresChunkedManifest(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "bucket-chunked-manifest",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	refs := chunkedManifestTestRefs(meta.ObjectManifestChunkRefTarget + 1)
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:    bucket.BucketID,
		Key:         "chunked.bin",
		SizeBytes:   int64(len(refs)),
		ETag:        `"chunked"`,
		SegmentRefs: refs,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	rawVersion := store.values[keyspace.ObjectVersion(bucket.BucketID, "chunked.bin", pending.Version.VersionSortKey)]
	if len(rawVersion) == 0 {
		t.Fatalf("missing raw object version")
	}
	if bytes.Contains(rawVersion, []byte(refs[0].SegmentID)) {
		t.Fatalf("raw object version still contains inline segment refs")
	}
	var stored model.ObjectVersion
	if err := json.Unmarshal(rawVersion, &stored); err != nil {
		t.Fatalf("raw object version unmarshal: %v", err)
	}
	if stored.Manifest.Encoding != model.ObjectManifestEncodingChunked ||
		stored.Manifest.RefCount != len(refs) ||
		stored.Manifest.ChunkCount == 0 {
		t.Fatalf("stored manifest descriptor = %+v", stored.Manifest)
	}
	chunkPrefix := keyspace.ObjectManifestChunk(bucket.BucketID, pending.Version.VersionID, 0)
	chunkCount := 0
	for key := range store.values {
		if strings.HasPrefix(key, chunkPrefix) {
			chunkCount++
		}
	}
	if chunkCount != stored.Manifest.ChunkCount {
		t.Fatalf("chunk count = %d, descriptor = %+v", chunkCount, stored.Manifest)
	}
	head, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "chunked.bin",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	})
	if err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if len(head.SegmentRefs) != len(refs) {
		t.Fatalf("committed head refs = %d, want %d", len(head.SegmentRefs), len(refs))
	}
	if head.SegmentRefs[0].SegmentID != refs[0].SegmentID {
		t.Fatalf("committed head first ref = %q, want %q", head.SegmentRefs[0].SegmentID, refs[0].SegmentID)
	}
	got, err := repo.GetObjectVersion(t.Context(), bucket.BucketID, "chunked.bin", pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion() error = %v", err)
	}
	if len(got.SegmentRefs) != len(refs) {
		t.Fatalf("hydrated version refs = %d, want %d", len(got.SegmentRefs), len(refs))
	}
	if got.SegmentRefs[len(refs)-1].SegmentID != refs[len(refs)-1].SegmentID {
		t.Fatalf("hydrated version last ref = %q, want %q", got.SegmentRefs[len(refs)-1].SegmentID, refs[len(refs)-1].SegmentID)
	}
}

func TestCreateBucketUsesDistributedIDGeneratorWithoutBucketSequence(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(
		store,
		func() time.Time { return now },
		kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
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
	if _, ok := store.values["/namros/v1/sequences/bucket"]; ok {
		t.Fatalf("bucket sequence key was written")
	}
}

func TestCreateBucketRetriesDistributedIDCollision(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(
		store,
		func() time.Time { return now },
		kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
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
	if _, ok := store.values["/namros/v1/sequences/bucket"]; ok {
		t.Fatalf("bucket sequence key was written")
	}
}

func TestObjectWritesUseDistributedIDsWithoutObjectSequence(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(
		store,
		func() time.Time { return now },
		kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
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
	if _, ok := store.values["/namros/v1/sequences/object"]; ok {
		t.Fatalf("object sequence key was written")
	}
}

func TestObjectWriteIDsRetryDistributedIDCollision(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(
		store,
		func() time.Time { return now },
		kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
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
	if _, ok := store.values["/namros/v1/sequences/object"]; ok {
		t.Fatalf("object sequence key was written")
	}
}

func TestGetObjectVersionUsesDirectVersionIndex(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(
		store,
		func() time.Time { return now },
		kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
			metaid.KindBucket:  {"bkt_version_index"},
			metaid.KindVersion: {"ver_indexed"},
		})),
	)

	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "version-index",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID: bucket.BucketID,
		Key:      "object.txt",
		ETag:     `"indexed"`,
	})
	if err != nil {
		t.Fatalf("BeginPutObject() error = %v", err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucket.BucketID,
		Key:                   "object.txt",
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion() error = %v", err)
	}
	if _, ok := store.values[keyspace.ObjectVersionByID(bucket.BucketID, pending.Version.VersionID)]; !ok {
		t.Fatalf("missing direct version index")
	}

	store.failListPrefix = keyspace.ObjectVersion(bucket.BucketID, "object.txt", "")
	got, err := repo.GetObjectVersion(t.Context(), bucket.BucketID, "object.txt", pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion() error = %v", err)
	}
	if got.VersionID != pending.Version.VersionID || got.VersionSortKey != pending.Version.VersionID {
		t.Fatalf("version = %+v, want indexed version", got)
	}
}

func TestDeleteNonCurrentObjectVersionUsesDirectVersionIndex(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(
		store,
		func() time.Time { return now },
		kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
			metaid.KindBucket:  {"bkt_delete_index"},
			metaid.KindVersion: {"ver_delete_old", "ver_delete_head"},
		})),
	)

	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "delete-index",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if _, err := repo.PutBucketVersioning(t.Context(), meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	first, err := repo.PutObjectVersion(t.Context(), meta.PutObjectVersionRequest{
		BucketID: bucket.BucketID,
		Key:      "object.txt",
		ETag:     `"old"`,
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(first) error = %v", err)
	}
	second, err := repo.PutObjectVersion(t.Context(), meta.PutObjectVersionRequest{
		BucketID: bucket.BucketID,
		Key:      "object.txt",
		ETag:     `"head"`,
	})
	if err != nil {
		t.Fatalf("PutObjectVersion(second) error = %v", err)
	}
	if second.Head.VersionID != "ver_delete_head" {
		t.Fatalf("second head version = %q, want generated head id", second.Head.VersionID)
	}

	store.failListPrefix = keyspace.ObjectVersion(bucket.BucketID, "object.txt", "")
	deleted, err := repo.DeleteObject(t.Context(), meta.DeleteObjectRequest{
		BucketID:  bucket.BucketID,
		Key:       "object.txt",
		VersionID: first.Head.VersionID,
	})
	if err != nil {
		t.Fatalf("DeleteObject(non-current version) error = %v", err)
	}
	if deleted.DeletedVersionID != first.Head.VersionID {
		t.Fatalf("deleted version = %q, want %q", deleted.DeletedVersionID, first.Head.VersionID)
	}
	if _, ok := store.values[keyspace.ObjectVersionByID(bucket.BucketID, first.Head.VersionID)]; ok {
		t.Fatalf("direct version index remained after delete")
	}
}

func TestLegacySequentialMetadataRemainsReadableAndBackfillsVersionIndex(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	bucket := model.Bucket{
		BucketID:  "bucket-000000000001",
		TenantID:  "tenant-1",
		Name:      "legacy-bucket",
		Region:    "us-east-1",
		CreatedAt: now,
	}
	version := model.ObjectVersion{
		BucketID:       bucket.BucketID,
		Key:            "legacy.txt",
		VersionID:      "version-000000000001",
		VersionSortKey: "00000000000000000001#version-000000000001",
		ETag:           `"legacy"`,
		State:          model.ObjectVersionCommitted,
		CreatedAt:      now,
		CommittedAt:    now,
	}
	upload := model.MultipartUpload{
		UploadID:  "upload-000000000001",
		BucketID:  bucket.BucketID,
		Key:       "legacy-mpu.bin",
		State:     model.MultipartUploadActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	putStoreJSON(t, store, keyspace.BucketByID(bucket.BucketID), bucket)
	putStoreJSON(t, store, keyspace.BucketByName(bucket.Name), bucket.BucketID)
	putStoreJSON(t, store, keyspace.ObjectVersion(bucket.BucketID, version.Key, version.VersionSortKey), version)
	putStoreJSON(t, store, keyspace.ObjectHead(bucket.BucketID, version.Key), model.ObjectHeadEntry{
		BucketID:     bucket.BucketID,
		Key:          version.Key,
		VersionID:    version.VersionID,
		ETag:         version.ETag,
		LastModified: now,
	})
	putStoreJSON(t, store, keyspace.ListObject(bucket.BucketID, version.Key), model.ObjectListEntry{
		BucketID:     bucket.BucketID,
		Key:          version.Key,
		VersionID:    version.VersionID,
		ETag:         version.ETag,
		LastModified: now,
	})
	putStoreJSON(t, store, keyspace.MultipartUpload(bucket.BucketID, upload.UploadID), upload)

	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	gotBucket, err := repo.GetBucketByName(t.Context(), bucket.Name)
	if err != nil {
		t.Fatalf("GetBucketByName(legacy) error = %v", err)
	}
	if gotBucket.BucketID != bucket.BucketID {
		t.Fatalf("legacy bucket id = %q, want %q", gotBucket.BucketID, bucket.BucketID)
	}
	head, err := repo.GetObjectHead(t.Context(), bucket.BucketID, version.Key)
	if err != nil {
		t.Fatalf("GetObjectHead(legacy) error = %v", err)
	}
	if head.VersionID != version.VersionID {
		t.Fatalf("legacy head version = %q, want %q", head.VersionID, version.VersionID)
	}
	gotVersion, err := repo.GetObjectVersion(t.Context(), bucket.BucketID, version.Key, version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(legacy) error = %v", err)
	}
	if gotVersion.VersionID != version.VersionID || gotVersion.VersionSortKey != version.VersionSortKey {
		t.Fatalf("legacy version = %+v, want %+v", gotVersion, version)
	}
	if _, ok := store.values[keyspace.ObjectVersionByID(bucket.BucketID, version.VersionID)]; !ok {
		t.Fatalf("legacy version lookup did not backfill direct index")
	}
	store.failListPrefix = keyspace.ObjectVersion(bucket.BucketID, version.Key, "")
	gotAgain, err := repo.GetObjectVersion(t.Context(), bucket.BucketID, version.Key, version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(legacy indexed) error = %v", err)
	}
	if gotAgain.VersionID != version.VersionID {
		t.Fatalf("legacy indexed version = %+v, want %q", gotAgain, version.VersionID)
	}
	gotUpload, err := repo.GetMultipartUpload(t.Context(), meta.MultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      upload.Key,
		UploadID: upload.UploadID,
	})
	if err != nil {
		t.Fatalf("GetMultipartUpload(legacy) error = %v", err)
	}
	if gotUpload.UploadID != upload.UploadID || gotUpload.State != model.MultipartUploadActive {
		t.Fatalf("legacy upload = %+v, want active %q", gotUpload, upload.UploadID)
	}
}

func TestConcurrentUnrelatedWritesDoNotUseGlobalSequences(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })

	const bucketWriters = 8
	var bucketWG sync.WaitGroup
	bucketErrs := make(chan error, bucketWriters)
	for i := range bucketWriters {
		i := i
		bucketWG.Add(1)
		go func() {
			defer bucketWG.Done()
			_, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
				TenantID: "tenant-1",
				Name:     fmt.Sprintf("concurrent-bucket-%02d", i),
				Region:   "us-east-1",
			})
			if err != nil {
				bucketErrs <- err
			}
		}()
	}
	bucketWG.Wait()
	close(bucketErrs)
	for err := range bucketErrs {
		if err != nil {
			t.Fatalf("CreateBucket(concurrent) error = %v", err)
		}
	}

	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "concurrent-objects",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket(object parent) error = %v", err)
	}
	const objectWriters = 12
	var objectWG sync.WaitGroup
	objectErrs := make(chan error, objectWriters)
	for i := range objectWriters {
		i := i
		objectWG.Add(1)
		go func() {
			defer objectWG.Done()
			key := fmt.Sprintf("objects/%02d.txt", i)
			pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
				BucketID: bucket.BucketID,
				Key:      key,
				ETag:     fmt.Sprintf(`"etag-%02d"`, i),
			})
			if err != nil {
				objectErrs <- err
				return
			}
			if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
				BucketID:              bucket.BucketID,
				Key:                   key,
				VersionID:             pending.Version.VersionID,
				ExpectedHeadVersionID: pending.BaseHeadVersionID,
			}); err != nil {
				objectErrs <- err
			}
		}()
	}
	objectWG.Wait()
	close(objectErrs)
	for err := range objectErrs {
		if err != nil {
			t.Fatalf("object write(concurrent) error = %v", err)
		}
	}
	store.mu.Lock()
	_, wroteBucketSequence := store.values["/namros/v1/sequences/bucket"]
	_, wroteObjectSequence := store.values["/namros/v1/sequences/object"]
	store.mu.Unlock()
	if wroteBucketSequence {
		t.Fatalf("bucket sequence key was written")
	}
	if wroteObjectSequence {
		t.Fatalf("object sequence key was written")
	}
}

func TestMemoryStoreListRangeBoundsCursorAndLimit(t *testing.T) {
	store := newMemoryStore()
	err := store.RunInTransaction(t.Context(), func(tx kvrepo.ReadWriter) error {
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

func TestListObjectsUsesBoundedListRange(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "bounded-list",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	for _, key := range []string{
		"outside/00.txt",
		"target/00.txt",
		"target/01.txt",
		"target/02.txt",
		"target/03.txt",
		"target/deep/00.txt",
		"target/deep/01.txt",
		"target/other/00.txt",
		"zzz/00.txt",
	} {
		pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
			BucketID: bucket.BucketID,
			Key:      key,
			ETag:     fmt.Sprintf(`"%s"`, key),
		})
		if err != nil {
			t.Fatalf("BeginPutObject(%s) error = %v", key, err)
		}
		if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
			BucketID:              bucket.BucketID,
			Key:                   key,
			VersionID:             pending.Version.VersionID,
			ExpectedHeadVersionID: pending.BaseHeadVersionID,
		}); err != nil {
			t.Fatalf("CommitObjectVersion(%s) error = %v", key, err)
		}
	}

	store.resetListTracking()
	page, err := repo.ListObjects(t.Context(), meta.ListObjectsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "target/",
		MaxKeys:  2,
	})
	if err != nil {
		t.Fatalf("ListObjects(page1) error = %v", err)
	}
	if got := objectHeadKeys(page.Contents); !reflect.DeepEqual(got, []string{"target/00.txt", "target/01.txt"}) {
		t.Fatalf("page1 keys = %v", got)
	}
	if !page.IsTruncated || page.NextContinuationToken != "target/01.txt" {
		t.Fatalf("page1 truncated = %v token = %q", page.IsTruncated, page.NextContinuationToken)
	}
	calls, prefixCalls := store.listTracking()
	if len(prefixCalls) != 0 {
		t.Fatalf("ListObjects used prefix List calls: %+v", prefixCalls)
	}
	wantStart := keyspace.ListObject(bucket.BucketID, "target/")
	if len(calls) != 1 || calls[0].start != wantStart || calls[0].end != testPrefixRangeEnd(wantStart) || calls[0].cursor != "" || calls[0].limit != 3 {
		t.Fatalf("ListRange calls = %+v, want one bounded target/ scan", calls)
	}

	store.resetListTracking()
	next, err := repo.ListObjects(t.Context(), meta.ListObjectsRequest{
		BucketID:          bucket.BucketID,
		Prefix:            "target/",
		ContinuationToken: page.NextContinuationToken,
		MaxKeys:           2,
	})
	if err != nil {
		t.Fatalf("ListObjects(page2) error = %v", err)
	}
	if got := objectHeadKeys(next.Contents); !reflect.DeepEqual(got, []string{"target/02.txt", "target/03.txt"}) {
		t.Fatalf("page2 keys = %v", got)
	}
	if !next.IsTruncated || next.NextContinuationToken != "target/03.txt" {
		t.Fatalf("page2 truncated = %v token = %q", next.IsTruncated, next.NextContinuationToken)
	}
	calls, prefixCalls = store.listTracking()
	wantCursor := keyspace.ListObject(bucket.BucketID, page.NextContinuationToken)
	if len(prefixCalls) != 0 || len(calls) != 1 || calls[0].cursor != wantCursor {
		t.Fatalf("page2 scan calls = %+v prefix calls = %+v, want cursor %q", calls, prefixCalls, wantCursor)
	}

	store.resetListTracking()
	delimited, err := repo.ListObjects(t.Context(), meta.ListObjectsRequest{
		BucketID:  bucket.BucketID,
		Prefix:    "target/",
		Delimiter: "/",
		MaxKeys:   5,
	})
	if err != nil {
		t.Fatalf("ListObjects(delimited page1) error = %v", err)
	}
	if got := objectHeadKeys(delimited.Contents); !reflect.DeepEqual(got, []string{"target/00.txt", "target/01.txt", "target/02.txt", "target/03.txt"}) {
		t.Fatalf("delimited page1 keys = %v", got)
	}
	if !reflect.DeepEqual(delimited.CommonPrefixes, []string{"target/deep/"}) {
		t.Fatalf("delimited page1 common prefixes = %v", delimited.CommonPrefixes)
	}
	if !delimited.IsTruncated || delimited.NextContinuationToken != "target/deep/" {
		t.Fatalf("delimited page1 truncated = %v token = %q", delimited.IsTruncated, delimited.NextContinuationToken)
	}
	calls, prefixCalls = store.listTracking()
	wantSkipStart := testPrefixRangeEnd(keyspace.ListObject(bucket.BucketID, "target/deep/"))
	if len(prefixCalls) != 0 || len(calls) < 2 || calls[1].start != wantSkipStart {
		t.Fatalf("delimited page1 scan calls = %+v prefix calls = %+v, want skip start %q", calls, prefixCalls, wantSkipStart)
	}

	store.resetListTracking()
	delimitedNext, err := repo.ListObjects(t.Context(), meta.ListObjectsRequest{
		BucketID:          bucket.BucketID,
		Prefix:            "target/",
		Delimiter:         "/",
		ContinuationToken: delimited.NextContinuationToken,
		MaxKeys:           5,
	})
	if err != nil {
		t.Fatalf("ListObjects(delimited page2) error = %v", err)
	}
	if len(delimitedNext.Contents) != 0 || !reflect.DeepEqual(delimitedNext.CommonPrefixes, []string{"target/other/"}) {
		t.Fatalf("delimited page2 = %+v", delimitedNext)
	}
	if delimitedNext.IsTruncated || delimitedNext.NextContinuationToken != "" {
		t.Fatalf("delimited page2 truncated = %v token = %q", delimitedNext.IsTruncated, delimitedNext.NextContinuationToken)
	}
	calls, prefixCalls = store.listTracking()
	if len(prefixCalls) != 0 || len(calls) == 0 || calls[0].start != wantSkipStart || calls[0].cursor != "" {
		t.Fatalf("delimited page2 scan calls = %+v prefix calls = %+v, want start %q with empty cursor", calls, prefixCalls, wantSkipStart)
	}
}

func TestListObjectVersionsUsesBoundedRanges(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "bounded-versions",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if _, err := repo.PutBucketVersioning(t.Context(), meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	putVersion := func(key string) model.ObjectHead {
		t.Helper()
		out, err := repo.PutObjectVersion(t.Context(), meta.PutObjectVersionRequest{
			BucketID: bucket.BucketID,
			Key:      key,
			ETag:     fmt.Sprintf(`"%s"`, key),
		})
		if err != nil {
			t.Fatalf("PutObjectVersion(%s) error = %v", key, err)
		}
		return out.Head
	}
	outside := putVersion("outside/00.txt")
	targetA1 := putVersion("target/a.txt")
	targetA2 := putVersion("target/a.txt")
	putVersion("target/b.txt")
	putVersion("target/deep/00.txt")
	putVersion("target/deep/01.txt")
	putVersion("target/other/00.txt")

	store.resetListTracking()
	page, err := repo.ListObjectVersions(t.Context(), meta.ListObjectVersionsRequest{
		BucketID: bucket.BucketID,
		Prefix:   "target/",
		MaxKeys:  2,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(page1) error = %v", err)
	}
	if got := objectVersionEntryIDs(page.Versions); !reflect.DeepEqual(got, []string{targetA2.VersionID, targetA1.VersionID}) {
		t.Fatalf("page1 versions = %v", got)
	}
	if !page.IsTruncated || page.NextKeyMarker != "target/a.txt" || page.NextVersionIDMarker != targetA1.VersionID {
		t.Fatalf("page1 truncated = %v key marker = %q version marker = %q", page.IsTruncated, page.NextKeyMarker, page.NextVersionIDMarker)
	}
	calls, prefixCalls := store.listTracking()
	wantStart := testVersionListPrefix(bucket.BucketID, "target/")
	if len(prefixCalls) != 0 || len(calls) < 2 || calls[0].start != wantStart || calls[0].limit != 1 {
		t.Fatalf("version page1 calls = %+v prefix calls = %+v, want bounded target/ range", calls, prefixCalls)
	}

	store.resetListTracking()
	clamped, err := repo.ListObjectVersions(t.Context(), meta.ListObjectVersionsRequest{
		BucketID:        bucket.BucketID,
		Prefix:          "target/",
		KeyMarker:       "outside/00.txt",
		VersionIDMarker: outside.VersionID,
		MaxKeys:         1,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(clamped marker) error = %v", err)
	}
	if got := objectVersionEntryIDs(clamped.Versions); !reflect.DeepEqual(got, []string{targetA2.VersionID}) {
		t.Fatalf("clamped marker versions = %v", got)
	}
	calls, prefixCalls = store.listTracking()
	if len(prefixCalls) != 0 || len(calls) == 0 || calls[0].start != wantStart {
		t.Fatalf("clamped marker calls = %+v prefix calls = %+v, want start %q", calls, prefixCalls, wantStart)
	}

	store.resetListTracking()
	next, err := repo.ListObjectVersions(t.Context(), meta.ListObjectVersionsRequest{
		BucketID:        bucket.BucketID,
		Prefix:          "target/",
		KeyMarker:       page.NextKeyMarker,
		VersionIDMarker: page.NextVersionIDMarker,
		MaxKeys:         2,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(page2) error = %v", err)
	}
	if len(next.Versions) != 2 || next.Versions[0].Version.Key != "target/b.txt" || next.Versions[1].Version.Key != "target/deep/00.txt" {
		t.Fatalf("page2 versions = %+v", next.Versions)
	}
	calls, prefixCalls = store.listTracking()
	wantAfterTargetA := testPrefixRangeEnd(keyspace.ObjectVersion(bucket.BucketID, "target/a.txt", ""))
	if len(prefixCalls) != 0 || len(calls) < 2 || calls[1].start != wantAfterTargetA {
		t.Fatalf("version page2 calls = %+v prefix calls = %+v, want scan after target/a exact prefix %q", calls, prefixCalls, wantAfterTargetA)
	}

	store.resetListTracking()
	delimited, err := repo.ListObjectVersions(t.Context(), meta.ListObjectVersionsRequest{
		BucketID:  bucket.BucketID,
		Prefix:    "target/",
		Delimiter: "/",
		MaxKeys:   4,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(delimited page1) error = %v", err)
	}
	if len(delimited.Versions) != 3 || !reflect.DeepEqual(delimited.CommonPrefixes, []string{"target/deep/"}) {
		t.Fatalf("delimited page1 = %+v", delimited)
	}
	if !delimited.IsTruncated || delimited.NextKeyMarker != "target/deep/" || delimited.NextVersionIDMarker != "" {
		t.Fatalf("delimited page1 truncated = %v key marker = %q version marker = %q", delimited.IsTruncated, delimited.NextKeyMarker, delimited.NextVersionIDMarker)
	}
	calls, prefixCalls = store.listTracking()
	wantSkipStart := testPrefixRangeEnd(testVersionListPrefix(bucket.BucketID, "target/deep/"))
	if len(prefixCalls) != 0 || len(calls) < 2 || calls[len(calls)-1].start != wantSkipStart {
		t.Fatalf("delimited version page1 calls = %+v prefix calls = %+v, want skip start %q", calls, prefixCalls, wantSkipStart)
	}

	store.resetListTracking()
	delimitedNext, err := repo.ListObjectVersions(t.Context(), meta.ListObjectVersionsRequest{
		BucketID:  bucket.BucketID,
		Prefix:    "target/",
		Delimiter: "/",
		KeyMarker: delimited.NextKeyMarker,
		MaxKeys:   4,
	})
	if err != nil {
		t.Fatalf("ListObjectVersions(delimited page2) error = %v", err)
	}
	if len(delimitedNext.Versions) != 0 || !reflect.DeepEqual(delimitedNext.CommonPrefixes, []string{"target/other/"}) {
		t.Fatalf("delimited version page2 = %+v", delimitedNext)
	}
	calls, prefixCalls = store.listTracking()
	if len(prefixCalls) != 0 || len(calls) == 0 || calls[0].start != wantSkipStart {
		t.Fatalf("delimited version page2 calls = %+v prefix calls = %+v, want start %q", calls, prefixCalls, wantSkipStart)
	}
}

func TestListMultipartUploadsUsesBoundedIndex(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now })
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "bounded-uploads",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	createUpload := func(key string) model.MultipartUpload {
		t.Helper()
		upload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{
			BucketID: bucket.BucketID,
			Key:      key,
		})
		if err != nil {
			t.Fatalf("CreateMultipartUpload(%s) error = %v", key, err)
		}
		return upload
	}
	createUpload("outside/00.bin")
	targetA1 := createUpload("target/a.bin")
	targetA2 := createUpload("target/a.bin")
	createUpload("target/b.bin")
	createUpload("target/deep/00.bin")
	createUpload("target/deep/01.bin")
	createUpload("target/other/00.bin")

	store.resetListTracking()
	page, err := repo.ListMultipartUploads(t.Context(), meta.ListMultipartUploadsRequest{
		BucketID:   bucket.BucketID,
		Prefix:     "target/",
		MaxUploads: 2,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(page1) error = %v", err)
	}
	if got := multipartUploadIDs(page.Uploads); !reflect.DeepEqual(got, []string{targetA1.UploadID, targetA2.UploadID}) {
		t.Fatalf("page1 uploads = %v", got)
	}
	if !page.IsTruncated || page.NextKeyMarker != "target/a.bin" || page.NextUploadIDMarker != targetA2.UploadID {
		t.Fatalf("page1 truncated = %v key marker = %q upload marker = %q", page.IsTruncated, page.NextKeyMarker, page.NextUploadIDMarker)
	}
	calls, prefixCalls := store.listTracking()
	wantStart := testMultipartUploadListPrefix(bucket.BucketID, "target/")
	if len(prefixCalls) != 0 || len(calls) != 1 || calls[0].start != wantStart || calls[0].limit != 3 {
		t.Fatalf("upload page1 calls = %+v prefix calls = %+v, want bounded target/ index", calls, prefixCalls)
	}

	store.resetListTracking()
	next, err := repo.ListMultipartUploads(t.Context(), meta.ListMultipartUploadsRequest{
		BucketID:       bucket.BucketID,
		Prefix:         "target/",
		KeyMarker:      page.NextKeyMarker,
		UploadIDMarker: page.NextUploadIDMarker,
		MaxUploads:     2,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(page2) error = %v", err)
	}
	if len(next.Uploads) != 2 || next.Uploads[0].Key != "target/b.bin" || next.Uploads[1].Key != "target/deep/00.bin" {
		t.Fatalf("page2 uploads = %+v", next.Uploads)
	}
	calls, prefixCalls = store.listTracking()
	wantCursor := keyspace.MultipartUploadByKey(bucket.BucketID, page.NextKeyMarker, page.NextUploadIDMarker)
	if len(prefixCalls) != 0 || len(calls) != 1 || calls[0].cursor != wantCursor {
		t.Fatalf("upload page2 calls = %+v prefix calls = %+v, want cursor %q", calls, prefixCalls, wantCursor)
	}

	store.resetListTracking()
	delimited, err := repo.ListMultipartUploads(t.Context(), meta.ListMultipartUploadsRequest{
		BucketID:   bucket.BucketID,
		Prefix:     "target/",
		Delimiter:  "/",
		MaxUploads: 4,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(delimited page1) error = %v", err)
	}
	if len(delimited.Uploads) != 3 || !reflect.DeepEqual(delimited.CommonPrefixes, []string{"target/deep/"}) {
		t.Fatalf("delimited page1 = %+v", delimited)
	}
	if !delimited.IsTruncated || delimited.NextKeyMarker != "target/deep/" || delimited.NextUploadIDMarker != "" {
		t.Fatalf("delimited page1 truncated = %v key marker = %q upload marker = %q", delimited.IsTruncated, delimited.NextKeyMarker, delimited.NextUploadIDMarker)
	}
	calls, prefixCalls = store.listTracking()
	wantSkipStart := testPrefixRangeEnd(testMultipartUploadListPrefix(bucket.BucketID, "target/deep/"))
	if len(prefixCalls) != 0 || len(calls) < 2 || calls[1].start != wantSkipStart {
		t.Fatalf("delimited upload page1 calls = %+v prefix calls = %+v, want skip start %q", calls, prefixCalls, wantSkipStart)
	}

	store.resetListTracking()
	delimitedNext, err := repo.ListMultipartUploads(t.Context(), meta.ListMultipartUploadsRequest{
		BucketID:   bucket.BucketID,
		Prefix:     "target/",
		Delimiter:  "/",
		KeyMarker:  delimited.NextKeyMarker,
		MaxUploads: 4,
	})
	if err != nil {
		t.Fatalf("ListMultipartUploads(delimited page2) error = %v", err)
	}
	if len(delimitedNext.Uploads) != 0 || !reflect.DeepEqual(delimitedNext.CommonPrefixes, []string{"target/other/"}) {
		t.Fatalf("delimited page2 = %+v", delimitedNext)
	}
	calls, prefixCalls = store.listTracking()
	if len(prefixCalls) != 0 || len(calls) == 0 || calls[0].start != wantSkipStart {
		t.Fatalf("delimited upload page2 calls = %+v prefix calls = %+v, want start %q", calls, prefixCalls, wantSkipStart)
	}

	if _, err := repo.CompleteMultipartUpload(t.Context(), meta.CompleteMultipartUploadRequest{
		BucketID:        bucket.BucketID,
		Key:             targetA1.Key,
		UploadID:        targetA1.UploadID,
		ObjectVersionID: "ver-completed",
		ETag:            `"completed"`,
		PartCount:       1,
	}); err != nil {
		t.Fatalf("CompleteMultipartUpload() error = %v", err)
	}
	if _, ok := store.values[keyspace.MultipartUploadByKey(bucket.BucketID, targetA1.Key, targetA1.UploadID)]; ok {
		t.Fatalf("completed upload index still exists")
	}
}

func TestRepairListIndexesDetectsAndRepairsKVDrift(t *testing.T) {
	now := time.Date(2026, 8, 10, 1, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	repo := kvrepo.NewWithClock(store, func() time.Time { return now }, kvrepo.WithIDGenerator(metaid.NewDeterministicGenerator(map[metaid.Kind][]string{
		metaid.KindBucket:  {"bkt-repair"},
		metaid.KindVersion: {"ver-alpha", "ver-beta"},
		metaid.KindUpload:  {"upl-active"},
	})))
	bucket, err := repo.CreateBucket(t.Context(), meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "repair-list-indexes",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	putVersion := func(key string) model.ObjectHead {
		t.Helper()
		out, err := repo.PutObjectVersion(t.Context(), meta.PutObjectVersionRequest{
			BucketID: bucket.BucketID,
			Key:      key,
			ETag:     fmt.Sprintf(`"%s"`, key),
		})
		if err != nil {
			t.Fatalf("PutObjectVersion(%s) error = %v", key, err)
		}
		return out.Head
	}
	alpha := putVersion("alpha.txt")
	beta := putVersion("beta.txt")
	upload, err := repo.CreateMultipartUpload(t.Context(), meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "uploads/active.bin",
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	delete(store.values, keyspace.ListObject(bucket.BucketID, alpha.Key))
	putStoreJSON(t, store, keyspace.ListObject(bucket.BucketID, beta.Key), model.ObjectListEntry{
		BucketID:  bucket.BucketID,
		Key:       beta.Key,
		VersionID: "stale-version",
		Revision:  beta.Revision,
	})
	putStoreJSON(t, store, keyspace.ListObject(bucket.BucketID, "ghost.txt"), model.ObjectListEntry{
		BucketID:  bucket.BucketID,
		Key:       "ghost.txt",
		VersionID: "ghost-version",
		Revision:  1,
	})
	delete(store.values, keyspace.MultipartUploadByKey(bucket.BucketID, upload.Key, upload.UploadID))
	putStoreJSON(t, store, keyspace.MultipartUploadByKey(bucket.BucketID, "uploads/ghost.bin", "upl-ghost"), map[string]string{
		"upload_id": "upl-ghost",
	})

	dryRun, err := repo.RepairListIndexes(t.Context(), meta.RepairListIndexesRequest{
		BucketID: bucket.BucketID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("RepairListIndexes(dry-run) error = %v", err)
	}
	if dryRun.MissingObjectListEntries != 1 || dryRun.StaleObjectListEntries != 2 ||
		dryRun.MissingMultipartUploadIndexes != 1 || dryRun.StaleMultipartUploadIndexes != 1 ||
		dryRun.RepairedObjectListEntries != 0 || dryRun.RemovedObjectListEntries != 0 {
		t.Fatalf("dry-run repair = %+v", dryRun)
	}
	if _, ok := store.values[keyspace.ListObject(bucket.BucketID, alpha.Key)]; ok {
		t.Fatalf("dry-run repaired missing list entry")
	}

	applied, err := repo.RepairListIndexes(t.Context(), meta.RepairListIndexesRequest{
		BucketID: bucket.BucketID,
		Limit:    100,
		Apply:    true,
	})
	if err != nil {
		t.Fatalf("RepairListIndexes(apply) error = %v", err)
	}
	if applied.MissingObjectListEntries != 1 || applied.StaleObjectListEntries != 2 ||
		applied.RepairedObjectListEntries != 2 || applied.RemovedObjectListEntries != 1 ||
		applied.MissingMultipartUploadIndexes != 1 || applied.StaleMultipartUploadIndexes != 1 ||
		applied.RepairedMultipartUploadIndexes != 1 || applied.RemovedMultipartUploadIndexes != 1 {
		t.Fatalf("apply repair = %+v", applied)
	}
	listed, err := repo.ListObjects(t.Context(), meta.ListObjectsRequest{BucketID: bucket.BucketID, MaxKeys: 10})
	if err != nil {
		t.Fatalf("ListObjects() error = %v", err)
	}
	if got := objectHeadKeys(listed.Contents); !reflect.DeepEqual(got, []string{"alpha.txt", "beta.txt"}) {
		t.Fatalf("ListObjects keys = %+v", got)
	}
	uploads, err := repo.ListMultipartUploads(t.Context(), meta.ListMultipartUploadsRequest{BucketID: bucket.BucketID, MaxUploads: 10})
	if err != nil {
		t.Fatalf("ListMultipartUploads() error = %v", err)
	}
	if len(uploads.Uploads) != 1 || uploads.Uploads[0].UploadID != upload.UploadID {
		t.Fatalf("ListMultipartUploads() = %+v", uploads)
	}
	clean, err := repo.RepairListIndexes(t.Context(), meta.RepairListIndexesRequest{
		BucketID: bucket.BucketID,
		Limit:    100,
	})
	if err != nil {
		t.Fatalf("RepairListIndexes(clean) error = %v", err)
	}
	if clean.MissingObjectListEntries != 0 || clean.StaleObjectListEntries != 0 ||
		clean.MissingMultipartUploadIndexes != 0 || clean.StaleMultipartUploadIndexes != 0 {
		t.Fatalf("clean repair = %+v", clean)
	}
}

func chunkedManifestTestRefs(count int) []storage.SegmentRef {
	refs := make([]storage.SegmentRef, 0, count)
	for i := 0; i < count; i++ {
		refs = append(refs, storage.SegmentRef{
			SegmentID: fmt.Sprintf("chunked-segment-%05d", i+1),
			Placement: storage.PlacementSnapshot{
				Backend: "test",
				Layout:  "chunked-manifest",
				Parameters: map[string]string{
					"padding": strings.Repeat("p", 32*1024),
				},
			},
			SizeBytes: 1,
		})
	}
	return refs
}

func putStoreJSON(t *testing.T, store *memoryStore, key string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", key, err)
	}
	store.values[key] = encoded
}

type memoryStore struct {
	mu             sync.Mutex
	values         map[string][]byte
	failListPrefix string
	closed         bool
	listCalls      []listCall
	listRangeCalls []listRangeCall
}

func newMemoryStore() *memoryStore {
	return &memoryStore{values: make(map[string][]byte)}
}

func (s *memoryStore) RunInTransaction(ctx context.Context, fn func(tx kvrepo.ReadWriter) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("memory store is closed")
	}
	tx := &memoryTx{
		values:         cloneValues(s.values),
		failListPrefix: s.failListPrefix,
		listCalls:      &s.listCalls,
		listRangeCalls: &s.listRangeCalls,
	}
	if err := fn(tx); err != nil {
		return err
	}
	s.values = tx.values
	return nil
}

func (s *memoryStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *memoryStore) resetListTracking() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listCalls = nil
	s.listRangeCalls = nil
}

func (s *memoryStore) listTracking() ([]listRangeCall, []listCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]listRangeCall(nil), s.listRangeCalls...), append([]listCall(nil), s.listCalls...)
}

type memoryTx struct {
	values         map[string][]byte
	failListPrefix string
	listCalls      *[]listCall
	listRangeCalls *[]listRangeCall
}

type listCall struct {
	prefix string
	cursor string
	limit  int
}

type listRangeCall struct {
	start  string
	end    string
	cursor string
	limit  int
}

func (tx *memoryTx) Get(key string) ([]byte, bool, error) {
	value, ok := tx.values[key]
	if !ok {
		return nil, false, nil
	}
	return append([]byte(nil), value...), true, nil
}

func (tx *memoryTx) Set(key string, value []byte) error {
	tx.values[key] = append([]byte(nil), value...)
	return nil
}

func (tx *memoryTx) Delete(key string) error {
	delete(tx.values, key)
	return nil
}

func (tx *memoryTx) List(prefix, cursor string, limit int) ([]string, string, error) {
	if tx.failListPrefix != "" && strings.HasPrefix(prefix, tx.failListPrefix) {
		return nil, "", errors.New("list disabled for prefix " + prefix)
	}
	if tx.listCalls != nil {
		*tx.listCalls = append(*tx.listCalls, listCall{prefix: prefix, cursor: cursor, limit: limit})
	}
	return tx.listRange(prefix, "", cursor, limit, prefix)
}

func (tx *memoryTx) ListRange(start, end, cursor string, limit int) ([]string, string, error) {
	if tx.listRangeCalls != nil {
		*tx.listRangeCalls = append(*tx.listRangeCalls, listRangeCall{start: start, end: end, cursor: cursor, limit: limit})
	}
	return tx.listRange(start, end, cursor, limit, "")
}

func (tx *memoryTx) listRange(start, end, cursor string, limit int, prefixFilter string) ([]string, string, error) {
	if tx.failListPrefix != "" && strings.HasPrefix(start, tx.failListPrefix) {
		return nil, "", errors.New("list disabled for prefix " + start)
	}
	if end != "" && start >= end {
		return nil, "", nil
	}
	keys := make([]string, 0)
	for key := range tx.values {
		if key < start || (cursor != "" && key <= cursor) {
			continue
		}
		if end != "" && key >= end {
			continue
		}
		if prefixFilter != "" && !strings.HasPrefix(key, prefixFilter) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if limit <= 0 || len(keys) <= limit {
		return keys, "", nil
	}
	return keys[:limit], keys[limit-1], nil
}

func objectHeadKeys(heads []model.ObjectHead) []string {
	out := make([]string, 0, len(heads))
	for _, head := range heads {
		out = append(out, head.Key)
	}
	return out
}

func objectVersionEntryIDs(entries []model.ObjectVersionEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Version.VersionID)
	}
	return out
}

func multipartUploadIDs(uploads []model.MultipartUpload) []string {
	out := make([]string, 0, len(uploads))
	for _, upload := range uploads {
		out = append(out, upload.UploadID)
	}
	return out
}

func testVersionListPrefix(bucketID, objectKeyPrefix string) string {
	return "/namros/v1/buckets/" + keyspace.EscapePathSegment(bucketID) + "/versions/" + keyspace.EscapeObjectKey(objectKeyPrefix)
}

func testMultipartUploadListPrefix(bucketID, objectKeyPrefix string) string {
	return "/namros/v1/buckets/" + keyspace.EscapePathSegment(bucketID) + "/multipart-by-key/" + keyspace.EscapeObjectKey(objectKeyPrefix)
}

func testPrefixRangeEnd(prefix string) string {
	if prefix == "" {
		return ""
	}
	end := []byte(prefix)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			out := append([]byte(nil), end[:i+1]...)
			out[len(out)-1]++
			return string(out)
		}
	}
	return ""
}

func cloneValues(values map[string][]byte) map[string][]byte {
	out := make(map[string][]byte, len(values))
	for key, value := range values {
		out[key] = append([]byte(nil), value...)
	}
	return out
}

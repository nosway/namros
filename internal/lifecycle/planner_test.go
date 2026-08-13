package lifecycle

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func TestBuildPlanEvaluatesLifecycleCandidates(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	ctx := t.Context()

	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID: "tenant-1",
		Name:     "plans",
		Region:   "us-east-1",
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	if _, err := repo.PutBucketVersioning(ctx, meta.PutBucketVersioningRequest{
		BucketID: bucket.BucketID,
		State:    model.BucketVersioningEnabled,
	}); err != nil {
		t.Fatalf("PutBucketVersioning() error = %v", err)
	}
	first := putLifecycleTestObject(t, repo, bucket.BucketID, "logs/object.txt", storage.SegmentRef{SegmentID: "segment-first", SizeBytes: 5})
	clock = now.AddDate(0, 0, -10)
	upload, err := repo.CreateMultipartUpload(ctx, meta.CreateMultipartUploadRequest{
		BucketID: bucket.BucketID,
		Key:      "logs/pending.bin",
	})
	if err != nil {
		t.Fatalf("CreateMultipartUpload() error = %v", err)
	}
	clock = now.AddDate(0, 0, -2)
	second := putLifecycleTestObject(t, repo, bucket.BucketID, "logs/object.txt", storage.SegmentRef{SegmentID: "segment-second", SizeBytes: 6})
	_ = second
	clock = now
	if _, err := repo.PutBucketLifecycle(ctx, meta.BucketLifecycleRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketLifecycleConfiguration{
			Rules: []model.LifecycleRule{{
				ID:     "logs-rule",
				Status: model.LifecycleRuleEnabled,
				Prefix: "logs/",
				Expiration: model.LifecycleExpiration{
					Days: 30,
				},
				NoncurrentVersionExpiration: model.LifecycleNoncurrentVersionExpiration{
					NoncurrentDays: 7,
				},
				AbortIncompleteMultipartUpload: model.LifecycleAbortIncompleteMultipartUpload{
					DaysAfterInitiation: 3,
				},
			}},
		},
	}); err != nil {
		t.Fatalf("PutBucketLifecycle() error = %v", err)
	}

	plan, err := BuildPlan(ctx, repo, PlanRequest{BucketID: bucket.BucketID, Now: now})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Actions) != 2 {
		t.Fatalf("actions = %+v, want noncurrent expiration and abort MPU", plan.Actions)
	}
	assertLifecycleAction(t, plan.Actions, ActionExpireNoncurrentVersion, first.VersionID, "")
	assertLifecycleAction(t, plan.Actions, ActionAbortIncompleteMultipart, "", upload.UploadID)
}

func TestBuildPlanBlocksObjectLockProtectedVersion(t *testing.T) {
	now := time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	clock := now.AddDate(0, 0, -40)
	repo := memory.NewWithClock(func() time.Time { return clock })
	ctx := t.Context()

	bucket, err := repo.CreateBucket(ctx, meta.CreateBucketRequest{
		TenantID:          "tenant-1",
		Name:              "locked-plans",
		Region:            "us-east-1",
		ObjectLockEnabled: true,
	})
	if err != nil {
		t.Fatalf("CreateBucket() error = %v", err)
	}
	version := putLifecycleTestObject(t, repo, bucket.BucketID, "locked/object.txt", storage.SegmentRef{SegmentID: "segment-locked", SizeBytes: 5}, model.ObjectLockRetention{
		Mode:            model.ObjectLockModeCompliance,
		RetainUntilDate: now.AddDate(0, 0, 30),
	})
	clock = now
	if _, err := repo.PutBucketLifecycle(ctx, meta.BucketLifecycleRequest{
		BucketID: bucket.BucketID,
		Configuration: model.BucketLifecycleConfiguration{
			Rules: []model.LifecycleRule{{
				ID:     "locked-rule",
				Status: model.LifecycleRuleEnabled,
				Prefix: "locked/",
				Expiration: model.LifecycleExpiration{
					Days: 30,
				},
			}},
		},
	}); err != nil {
		t.Fatalf("PutBucketLifecycle() error = %v", err)
	}

	plan, err := BuildPlan(ctx, repo, PlanRequest{BucketID: bucket.BucketID, Now: now})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if len(plan.Actions) != 1 {
		t.Fatalf("actions = %+v, want one blocked action", plan.Actions)
	}
	action := plan.Actions[0]
	if action.Kind != ActionExpireCurrentObject || action.VersionID != version.VersionID || action.Status != ActionBlocked || action.BlockReason != BlockReasonObjectLock {
		t.Fatalf("blocked action = %+v", action)
	}
}

func putLifecycleTestObject(t *testing.T, repo meta.Repository, bucketID, key string, ref storage.SegmentRef, retention ...model.ObjectLockRetention) model.ObjectVersion {
	t.Helper()
	var objectRetention model.ObjectLockRetention
	if len(retention) > 0 {
		objectRetention = retention[0]
	}
	pending, err := repo.BeginPutObject(t.Context(), meta.BeginPutObjectRequest{
		BucketID:            bucketID,
		Key:                 key,
		SizeBytes:           int64(ref.SizeBytes),
		ETag:                `"` + ref.SegmentID + `"`,
		SegmentRef:          ref,
		ObjectLockRetention: objectRetention,
	})
	if err != nil {
		t.Fatalf("BeginPutObject(%s) error = %v", key, err)
	}
	if _, err := repo.CommitObjectVersion(t.Context(), meta.CommitObjectVersionRequest{
		BucketID:              bucketID,
		Key:                   key,
		VersionID:             pending.Version.VersionID,
		ExpectedHeadVersionID: pending.BaseHeadVersionID,
	}); err != nil {
		t.Fatalf("CommitObjectVersion(%s) error = %v", key, err)
	}
	version, err := repo.GetObjectVersion(t.Context(), bucketID, key, pending.Version.VersionID)
	if err != nil {
		t.Fatalf("GetObjectVersion(%s) error = %v", key, err)
	}
	return version
}

func assertLifecycleAction(t *testing.T, actions []Action, kind ActionKind, versionID, uploadID string) {
	t.Helper()
	for _, action := range actions {
		if action.Kind == kind && action.VersionID == versionID && action.UploadID == uploadID && action.Status == ActionEligible {
			return
		}
	}
	t.Fatalf("missing lifecycle action kind=%s version=%q upload=%q in %+v", kind, versionID, uploadID, actions)
}

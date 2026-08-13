package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
)

type ActionKind string

const (
	ActionExpireCurrentObject      ActionKind = "expire_current_object"
	ActionExpireDeleteMarker       ActionKind = "expire_delete_marker"
	ActionExpireNoncurrentVersion  ActionKind = "expire_noncurrent_version"
	ActionAbortIncompleteMultipart ActionKind = "abort_incomplete_multipart"
)

type ActionStatus string

const (
	ActionEligible ActionStatus = "eligible"
	ActionBlocked  ActionStatus = "blocked"
)

type BlockReason string

const (
	BlockReasonObjectLock   BlockReason = "object_lock"
	BlockReasonProtectedRef BlockReason = "protected_ref"
)

type PlanRequest struct {
	BucketID   string
	Now        time.Time
	MaxKeys    int
	MaxUploads int
}

type Plan struct {
	BucketID string
	Actions  []Action
}

type Action struct {
	Kind        ActionKind
	Status      ActionStatus
	BlockReason BlockReason
	RuleID      string
	BucketID    string
	Key         string
	VersionID   string
	UploadID    string
}

func BuildPlan(ctx context.Context, repo meta.Repository, req PlanRequest) (Plan, error) {
	if repo == nil {
		return Plan{}, errors.New("metadata repository is required")
	}
	if req.BucketID == "" {
		return Plan{}, errors.New("bucket id is required")
	}
	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	maxKeys := req.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	maxUploads := req.MaxUploads
	if maxUploads <= 0 {
		maxUploads = 1000
	}
	configuration, err := repo.GetBucketLifecycle(ctx, req.BucketID)
	if err != nil {
		if errors.Is(err, meta.ErrNotFound) {
			return Plan{BucketID: req.BucketID}, nil
		}
		return Plan{}, err
	}
	plan := Plan{BucketID: req.BucketID}
	versions, err := repo.ListObjectVersions(ctx, meta.ListObjectVersionsRequest{
		BucketID: req.BucketID,
		MaxKeys:  maxKeys,
	})
	if err != nil {
		return Plan{}, err
	}
	for _, rule := range configuration.Rules {
		if rule.Status != model.LifecycleRuleEnabled {
			continue
		}
		planVersionExpiration(ctx, repo, &plan, rule, versions, now)
		if rule.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 {
			uploads, err := repo.ListMultipartUploads(ctx, meta.ListMultipartUploadsRequest{
				BucketID:   req.BucketID,
				Prefix:     rule.Prefix,
				MaxUploads: maxUploads,
			})
			if err != nil {
				return Plan{}, err
			}
			planAbortIncompleteMultipart(&plan, rule, uploads.Uploads, now)
		}
	}
	return plan, nil
}

func planVersionExpiration(ctx context.Context, repo meta.Repository, plan *Plan, rule model.LifecycleRule, versions model.ListObjectVersionsResult, now time.Time) {
	for _, entry := range versions.Versions {
		version := entry.Version
		if !lifecycleRuleMatches(rule, version.Key) {
			continue
		}
		switch {
		case entry.IsLatest && currentVersionExpired(rule, version, now):
			plan.Actions = append(plan.Actions, guardedVersionAction(ctx, repo, rule, version, ActionExpireCurrentObject, now))
		case !entry.IsLatest && noncurrentVersionExpired(rule, version, now):
			plan.Actions = append(plan.Actions, guardedVersionAction(ctx, repo, rule, version, ActionExpireNoncurrentVersion, now))
		}
	}
	for _, entry := range versions.DeleteMarkers {
		version := entry.Version
		if !entry.IsLatest || !lifecycleRuleMatches(rule, version.Key) || !rule.Expiration.ExpiredObjectDeleteMarker {
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Kind:      ActionExpireDeleteMarker,
			Status:    ActionEligible,
			RuleID:    rule.ID,
			BucketID:  version.BucketID,
			Key:       version.Key,
			VersionID: version.VersionID,
		})
	}
}

func planAbortIncompleteMultipart(plan *Plan, rule model.LifecycleRule, uploads []model.MultipartUpload, now time.Time) {
	threshold := now.AddDate(0, 0, -rule.AbortIncompleteMultipartUpload.DaysAfterInitiation)
	for _, upload := range uploads {
		if upload.CreatedAt.After(threshold) {
			continue
		}
		plan.Actions = append(plan.Actions, Action{
			Kind:     ActionAbortIncompleteMultipart,
			Status:   ActionEligible,
			RuleID:   rule.ID,
			BucketID: upload.BucketID,
			Key:      upload.Key,
			UploadID: upload.UploadID,
		})
	}
}

func guardedVersionAction(ctx context.Context, repo meta.Repository, rule model.LifecycleRule, version model.ObjectVersion, kind ActionKind, now time.Time) Action {
	action := Action{
		Kind:      kind,
		Status:    ActionEligible,
		RuleID:    rule.ID,
		BucketID:  version.BucketID,
		Key:       version.Key,
		VersionID: version.VersionID,
	}
	if objectVersionProtected(version, now) {
		action.Status = ActionBlocked
		action.BlockReason = BlockReasonObjectLock
		return action
	}
	refs, err := repo.ListProtectedRefs(ctx, meta.ListProtectedRefsRequest{
		BucketID:   version.BucketID,
		Key:        version.Key,
		VersionID:  version.VersionID,
		ActiveOnly: true,
		Limit:      1,
	})
	if err != nil || len(refs) > 0 {
		action.Status = ActionBlocked
		action.BlockReason = BlockReasonProtectedRef
	}
	return action
}

func lifecycleRuleMatches(rule model.LifecycleRule, key string) bool {
	return rule.Prefix == "" || len(key) >= len(rule.Prefix) && key[:len(rule.Prefix)] == rule.Prefix
}

func currentVersionExpired(rule model.LifecycleRule, version model.ObjectVersion, now time.Time) bool {
	if rule.Expiration.ExpiredObjectDeleteMarker {
		return false
	}
	if rule.Expiration.Days > 0 {
		return !versionTime(version).After(now.AddDate(0, 0, -rule.Expiration.Days))
	}
	if !rule.Expiration.Date.IsZero() {
		return !now.Before(rule.Expiration.Date)
	}
	return false
}

func noncurrentVersionExpired(rule model.LifecycleRule, version model.ObjectVersion, now time.Time) bool {
	if rule.NoncurrentVersionExpiration.NoncurrentDays <= 0 {
		return false
	}
	return !versionTime(version).After(now.AddDate(0, 0, -rule.NoncurrentVersionExpiration.NoncurrentDays))
}

func versionTime(version model.ObjectVersion) time.Time {
	if !version.CommittedAt.IsZero() {
		return version.CommittedAt.UTC()
	}
	return version.CreatedAt.UTC()
}

func objectVersionProtected(version model.ObjectVersion, now time.Time) bool {
	if version.DeleteMarker {
		return false
	}
	if version.ObjectLockLegalHold == model.ObjectLockLegalHoldOn {
		return true
	}
	retention := version.ObjectLockRetention
	if retention.Mode == "" {
		return false
	}
	if retention.RetainUntilDate.IsZero() {
		return true
	}
	return now.UTC().Before(retention.RetainUntilDate.UTC())
}

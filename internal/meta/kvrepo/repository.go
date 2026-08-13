package kvrepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/meta"
	metaid "github.com/nosway/namros/internal/meta/id"
	"github.com/nosway/namros/internal/meta/keyspace"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

const (
	sequenceAudit             = "audit"
	sequenceMetadataMigration = "metadata-migration-operation"
	sequenceGCOperation       = "gc-operation"
	sequenceDedupeOperation   = "dedupe-operation"
	sequenceWorkerOperation   = "worker-operation"
	sequenceVolumeDrain       = "volume-drain-operation"

	idCollisionRetryLimit = 8
)

type Repository struct {
	store Store
	now   func() time.Time
	ids   metaid.Generator
}

type Option func(*Repository)

func WithIDGenerator(generator metaid.Generator) Option {
	return func(r *Repository) {
		if generator != nil {
			r.ids = generator
		}
	}
}

type sequenceRecord struct {
	Value int `json:"value"`
}

type objectVersionIndexRecord struct {
	Key            string `json:"key"`
	VersionSortKey string `json:"version_sort_key"`
}

type multipartUploadIndexRecord struct {
	UploadID string `json:"upload_id"`
}

type auditHeadRecord struct {
	LastHash string `json:"last_hash"`
}

type Store interface {
	RunInTransaction(ctx context.Context, fn func(tx ReadWriter) error) error
	Close() error
}

type ReadWriter interface {
	Get(key string) ([]byte, bool, error)
	Set(key string, value []byte) error
	Delete(key string) error
	List(prefix, cursor string, limit int) ([]string, string, error)
	ListRange(start, end, cursor string, limit int) ([]string, string, error)
}

func New(store Store, options ...Option) *Repository {
	return NewWithClock(store, func() time.Time { return time.Now().UTC() }, options...)
}

func NewWithClock(store Store, now func() time.Time, options ...Option) *Repository {
	repo := &Repository{
		store: store,
		now:   now,
	}
	for _, option := range options {
		if option != nil {
			option(repo)
		}
	}
	if repo.ids == nil {
		repo.ids = metaid.MustNewProcessGenerator(metaid.WithClock(now))
	}
	return repo
}

func (r *Repository) Close() error {
	if r.store == nil {
		return nil
	}
	store := r.store
	r.store = nil
	return store.Close()
}

func (r *Repository) run(ctx context.Context, fn func(tx ReadWriter) error) error {
	if r.store == nil {
		return errors.New("metadata repository is closed")
	}
	return r.store.RunInTransaction(ctx, fn)
}

func (r *Repository) GetMetadataSchema(ctx context.Context) (model.MetadataSchemaRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.MetadataSchemaRecord{}, err
	}
	var record model.MetadataSchemaRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		value, ok, err := getJSON[model.MetadataSchemaRecord](tx, keyspace.MetadataSchema())
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		record = value
		return nil
	})
	if err != nil {
		return model.MetadataSchemaRecord{}, err
	}
	return record, nil
}

func (r *Repository) PutMetadataSchema(ctx context.Context, req meta.PutMetadataSchemaRequest) (model.MetadataSchemaRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.MetadataSchemaRecord{}, err
	}
	var record model.MetadataSchemaRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		existing, _, err := getJSON[model.MetadataSchemaRecord](tx, keyspace.MetadataSchema())
		if err != nil {
			return err
		}
		record, err = meta.MetadataSchemaRecordFromRequest(req, r.now(), existing)
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.MetadataSchema(), record)
	})
	if err != nil {
		return model.MetadataSchemaRecord{}, err
	}
	return record, nil
}

func (r *Repository) CreateTenant(ctx context.Context, req meta.CreateTenantRequest) (model.Tenant, error) {
	if err := ctx.Err(); err != nil {
		return model.Tenant{}, err
	}
	if req.TenantID == "" {
		return model.Tenant{}, fmt.Errorf("%w: tenant id is required", meta.ErrInvalidArgument)
	}
	var tenant model.Tenant
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(req.TenantID)); err != nil {
			return err
		} else if ok {
			return meta.ErrAlreadyExists
		}
		tenant = model.Tenant{
			TenantID:    req.TenantID,
			DisplayName: req.DisplayName,
			CreatedAt:   r.now(),
		}
		return setJSON(tx, keyspace.Tenant(tenant.TenantID), tenant)
	})
	return tenant, err
}

func (r *Repository) GetTenant(ctx context.Context, tenantID string) (model.Tenant, error) {
	if err := ctx.Err(); err != nil {
		return model.Tenant{}, err
	}
	var tenant model.Tenant
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		tenant, ok, err = getJSON[model.Tenant](tx, keyspace.Tenant(tenantID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		return nil
	})
	return tenant, err
}

func (r *Repository) PutTenantQuota(ctx context.Context, req meta.TenantQuotaRequest) (model.TenantQuota, error) {
	if err := ctx.Err(); err != nil {
		return model.TenantQuota{}, err
	}
	var quota model.TenantQuota
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(req.TenantID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		existing, _, err := getJSON[model.TenantQuota](tx, keyspace.TenantQuota(req.TenantID))
		if err != nil {
			return err
		}
		quota, err = meta.BuildTenantQuota(existing, req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.TenantQuota(req.TenantID), quota)
	})
	return meta.CloneTenantQuotaRecord(quota), err
}

func (r *Repository) GetTenantQuota(ctx context.Context, tenantID string) (model.TenantQuota, error) {
	if err := ctx.Err(); err != nil {
		return model.TenantQuota{}, err
	}
	var quota model.TenantQuota
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(tenantID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		var ok bool
		var err error
		quota, ok, err = getJSON[model.TenantQuota](tx, keyspace.TenantQuota(tenantID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		return nil
	})
	return meta.CloneTenantQuotaRecord(quota), err
}

func (r *Repository) DeleteTenantQuota(ctx context.Context, tenantID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(tenantID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		if _, ok, err := getJSON[model.TenantQuota](tx, keyspace.TenantQuota(tenantID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		return tx.Delete(keyspace.TenantQuota(tenantID))
	})
}

func (r *Repository) PutTenantUsage(ctx context.Context, req meta.TenantUsageRequest) (model.TenantUsage, error) {
	if err := ctx.Err(); err != nil {
		return model.TenantUsage{}, err
	}
	var usage model.TenantUsage
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(req.TenantID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		existing, _, err := getJSON[model.TenantUsage](tx, keyspace.TenantUsage(req.TenantID))
		if err != nil {
			return err
		}
		usage, err = meta.BuildTenantUsage(existing, req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.TenantUsage(req.TenantID), usage)
	})
	return meta.CloneTenantUsageRecord(usage), err
}

func (r *Repository) GetTenantUsage(ctx context.Context, tenantID string) (model.TenantUsage, error) {
	if err := ctx.Err(); err != nil {
		return model.TenantUsage{}, err
	}
	var usage model.TenantUsage
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(tenantID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		var ok bool
		var err error
		usage, ok, err = getJSON[model.TenantUsage](tx, keyspace.TenantUsage(tenantID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		return nil
	})
	return meta.CloneTenantUsageRecord(usage), err
}

func enforceTenantActiveUploadQuota(tx ReadWriter, tenantID string) error {
	if tenantID == "" {
		return nil
	}
	quota, ok, err := getJSON[model.TenantQuota](tx, keyspace.TenantQuota(tenantID))
	if err != nil {
		return err
	}
	if !ok || quota.MaxActiveUploads <= 0 {
		return nil
	}
	usage, ok, err := getJSON[model.TenantUsage](tx, keyspace.TenantUsage(tenantID))
	if err != nil {
		return err
	}
	activeUploads := int64(0)
	if ok {
		activeUploads = usage.ActiveUploads
	}
	if activeUploads >= quota.MaxActiveUploads {
		return fmt.Errorf("%w: tenant %q active multipart uploads %d reached max %d", meta.ErrQuotaExceeded, tenantID, activeUploads, quota.MaxActiveUploads)
	}
	return nil
}

func applyTenantActiveUploadDeltaForBucket(tx ReadWriter, bucketID string, delta int64, now time.Time) error {
	bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	return applyTenantActiveUploadDelta(tx, bucket.TenantID, delta, now)
}

func applyTenantActiveUploadDelta(tx ReadWriter, tenantID string, delta int64, now time.Time) error {
	if tenantID == "" {
		return nil
	}
	if _, ok, err := getJSON[model.Tenant](tx, keyspace.Tenant(tenantID)); err != nil {
		return err
	} else if !ok {
		return nil
	}
	existing, _, err := getJSON[model.TenantUsage](tx, keyspace.TenantUsage(tenantID))
	if err != nil {
		return err
	}
	usage, err := meta.ApplyTenantActiveUploadDelta(existing, tenantID, delta, now)
	if err != nil {
		return err
	}
	return setJSON(tx, keyspace.TenantUsage(tenantID), usage)
}

func (r *Repository) PutAccessKey(ctx context.Context, req meta.PutAccessKeyRequest) (model.AccessKey, error) {
	if err := ctx.Err(); err != nil {
		return model.AccessKey{}, err
	}
	if req.TenantID == "" || req.AccessKeyID == "" || req.SecretHash == "" {
		return model.AccessKey{}, fmt.Errorf("%w: access key fields are required", meta.ErrInvalidArgument)
	}
	status := req.Status
	if status == "" {
		status = model.AccessKeyActive
	}
	accessKey := model.AccessKey{
		TenantID:    req.TenantID,
		AccessKeyID: req.AccessKeyID,
		SecretHash:  req.SecretHash,
		Status:      status,
		Permissions: cloneStringSlice(req.Permissions),
		CreatedAt:   r.now(),
	}
	err := r.run(ctx, func(tx ReadWriter) error {
		return setJSON(tx, keyspace.AccessKey(accessKey.AccessKeyID), accessKey)
	})
	return cloneAccessKey(accessKey), err
}

func (r *Repository) GetAccessKey(ctx context.Context, accessKeyID string) (model.AccessKey, error) {
	if err := ctx.Err(); err != nil {
		return model.AccessKey{}, err
	}
	var accessKey model.AccessKey
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		accessKey, ok, err = getJSON[model.AccessKey](tx, keyspace.AccessKey(accessKeyID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		accessKey = cloneAccessKey(accessKey)
		return nil
	})
	return accessKey, err
}

func (r *Repository) PutKMSKey(ctx context.Context, req meta.PutKMSKeyRequest) (model.KMSKeyRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.KMSKeyRecord{}, err
	}
	keyID := strings.TrimSpace(req.KeyID)
	if keyID == "" {
		return model.KMSKeyRecord{}, fmt.Errorf("%w: kms key id is required", meta.ErrInvalidArgument)
	}
	state := model.NormalizeKMSKeyState(req.State)
	now := r.now()
	var record model.KMSKeyRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		existing, ok, err := getJSON[model.KMSKeyRecord](tx, keyspace.KMSKey(keyID))
		if err != nil {
			return err
		}
		record = existing
		if !ok {
			record = model.KMSKeyRecord{
				KeyID:     keyID,
				CreatedAt: now,
			}
		}
		record.KeyVersion = strings.TrimSpace(req.KeyVersion)
		record.State = state
		record.UpdatedAt = now
		return setJSON(tx, keyspace.KMSKey(record.KeyID), record)
	})
	return record, err
}

func (r *Repository) GetKMSKey(ctx context.Context, keyID string) (model.KMSKeyRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.KMSKeyRecord{}, err
	}
	keyID = strings.TrimSpace(keyID)
	var record model.KMSKeyRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		record, ok, err = getJSON[model.KMSKeyRecord](tx, keyspace.KMSKey(keyID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		return nil
	})
	return record, err
}

func (r *Repository) ListKMSKeys(ctx context.Context, req meta.ListKMSKeysRequest) ([]model.KMSKeyRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.KMSKeyRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for len(records) < limit {
			keys, next, err := tx.List(keyspace.KMSKey(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.KMSKeyRecord](tx, key)
				if err != nil {
					return err
				}
				if ok {
					records = append(records, record)
					if len(records) >= limit {
						break
					}
				}
			}
			if next == "" || len(records) >= limit {
				break
			}
			cursor = next
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].KeyID < records[j].KeyID
	})
	return records, nil
}

func (r *Repository) PutComplianceProfileAttachment(ctx context.Context, req meta.PutComplianceProfileAttachmentRequest) (model.ComplianceProfileAttachment, error) {
	if err := ctx.Err(); err != nil {
		return model.ComplianceProfileAttachment{}, err
	}
	if err := validateComplianceProfileAttachmentRequest(req); err != nil {
		return model.ComplianceProfileAttachment{}, err
	}
	profileID := strings.TrimSpace(req.ProfileID)
	now := r.now()
	var record model.ComplianceProfileAttachment
	err := r.run(ctx, func(tx ReadWriter) error {
		existing, ok, err := getJSON[model.ComplianceProfileAttachment](tx, keyspace.ComplianceProfileAttachment(profileID))
		if err != nil {
			return err
		}
		record = existing
		if !ok {
			record = model.ComplianceProfileAttachment{
				ProfileID: profileID,
				CreatedAt: now,
			}
		}
		record.Regulation = strings.TrimSpace(req.Regulation)
		record.RecordClass = strings.TrimSpace(req.RecordClass)
		record.BucketID = strings.TrimSpace(req.BucketID)
		record.Prefix = strings.TrimSpace(req.Prefix)
		record.ObjectClass = strings.TrimSpace(req.ObjectClass)
		record.RetentionMode = req.RetentionMode
		record.RetentionDays = req.RetentionDays
		record.RetentionYears = req.RetentionYears
		record.LegalHoldPolicy = strings.TrimSpace(req.LegalHoldPolicy)
		record.GovernanceBypassPolicy = strings.TrimSpace(req.GovernanceBypassPolicy)
		record.EvidenceExportPolicy = strings.TrimSpace(req.EvidenceExportPolicy)
		record.UpdatedAt = now
		return setJSON(tx, keyspace.ComplianceProfileAttachment(record.ProfileID), record)
	})
	return record, err
}

func (r *Repository) ListComplianceProfileAttachments(ctx context.Context, req meta.ListComplianceProfileAttachmentsRequest) ([]model.ComplianceProfileAttachment, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	bucketID := strings.TrimSpace(req.BucketID)
	var records []model.ComplianceProfileAttachment
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for len(records) < limit {
			keys, next, err := tx.List(keyspace.ComplianceProfileAttachment(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.ComplianceProfileAttachment](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if bucketID != "" && record.BucketID != bucketID {
					continue
				}
				records = append(records, record)
				if len(records) >= limit {
					break
				}
			}
			if next == "" || len(records) >= limit {
				break
			}
			cursor = next
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].ProfileID < records[j].ProfileID
	})
	return records, nil
}

func (r *Repository) CreateBucket(ctx context.Context, req meta.CreateBucketRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.TenantID == "" || req.Name == "" || req.Region == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket fields are required", meta.ErrInvalidArgument)
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[string](tx, keyspace.BucketByName(req.Name)); err != nil {
			return err
		} else if ok {
			return meta.ErrAlreadyExists
		}
		for attempt := 0; attempt < idCollisionRetryLimit; attempt++ {
			bucketID, err := r.ids.NewID(metaid.KindBucket)
			if err != nil {
				return fmt.Errorf("%w: generate bucket id: %v", meta.ErrUnavailable, err)
			}
			if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID)); err != nil {
				return err
			} else if ok {
				continue
			}
			bucket = model.Bucket{
				BucketID:  bucketID,
				TenantID:  req.TenantID,
				Name:      req.Name,
				Region:    req.Region,
				CreatedAt: r.now(),
			}
			if req.ObjectLockEnabled {
				bucket.ObjectLock = model.BucketObjectLockConfiguration{Enabled: true}
				bucket.VersioningState = model.BucketVersioningEnabled
			}
			if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
				return err
			}
			return setJSON(tx, keyspace.BucketByName(bucket.Name), bucket.BucketID)
		}
		return fmt.Errorf("%w: bucket id collision retry budget exhausted", meta.ErrUnavailable)
	})
	return bucket, err
}

func (r *Repository) PutBucketVersioning(ctx context.Context, req meta.PutBucketVersioningRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if req.State != model.BucketVersioningEnabled && req.State != model.BucketVersioningSuspended {
		return model.Bucket{}, fmt.Errorf("%w: versioning state must be Enabled or Suspended", meta.ErrInvalidArgument)
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if bucket.ObjectLock.Enabled && req.State != model.BucketVersioningEnabled {
			return fmt.Errorf("%w: object lock bucket versioning cannot be suspended", meta.ErrInvalidArgument)
		}
		bucket.VersioningState = req.State
		return setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket)
	})
	return bucket, err
}

func (r *Repository) PutBucketCORS(ctx context.Context, req meta.BucketCORSRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.CORSRules = cloneCORSRules(req.Rules)
		return setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket)
	})
	return bucket, err
}

func (r *Repository) GetBucketCORS(ctx context.Context, bucketID string) ([]model.CORSRule, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var rules []model.CORSRule
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		rules = cloneCORSRules(bucket.CORSRules)
		return nil
	})
	return rules, err
}

func (r *Repository) DeleteBucketCORS(ctx context.Context, bucketID string) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.CORSRules = nil
		return setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket)
	})
	return bucket, err
}

func (r *Repository) PutBucketLifecycle(ctx context.Context, req meta.BucketLifecycleRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if err := validateBucketLifecycleConfiguration(req.Configuration); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.Lifecycle = cloneLifecycleConfiguration(req.Configuration)
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(req.Audit, model.AuditActionPutBucketLifecycle, bucket.BucketID, "", "", map[string]string{
			"rule_count": strconv.Itoa(len(req.Configuration.Rules)),
		}))
	})
	return cloneBucket(bucket), err
}

func (r *Repository) GetBucketLifecycle(ctx context.Context, bucketID string) (model.BucketLifecycleConfiguration, error) {
	if err := ctx.Err(); err != nil {
		return model.BucketLifecycleConfiguration{}, err
	}
	var configuration model.BucketLifecycleConfiguration
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if len(bucket.Lifecycle.Rules) == 0 {
			return meta.ErrNotFound
		}
		configuration = cloneLifecycleConfiguration(bucket.Lifecycle)
		return nil
	})
	return configuration, err
}

func (r *Repository) DeleteBucketLifecycle(ctx context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.Lifecycle = model.BucketLifecycleConfiguration{}
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(audit, model.AuditActionDeleteBucketLifecycle, bucket.BucketID, "", "", nil))
	})
	return cloneBucket(bucket), err
}

func (r *Repository) PutBucketEncryption(ctx context.Context, req meta.BucketEncryptionRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if err := meta.ValidateBucketEncryption(req.Encryption); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.DefaultEncryption = req.Encryption
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(req.Audit, model.AuditActionPutBucketEncryption, bucket.BucketID, "", "", map[string]string{
			"algorithm": string(req.Encryption.Algorithm),
			"key_id":    req.Encryption.KeyID,
		}))
	})
	return cloneBucket(bucket), err
}

func (r *Repository) GetBucketEncryption(ctx context.Context, bucketID string) (model.ServerSideEncryption, error) {
	if err := ctx.Err(); err != nil {
		return model.ServerSideEncryption{}, err
	}
	var encryption model.ServerSideEncryption
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if bucket.DefaultEncryption.Algorithm == "" {
			return meta.ErrNotFound
		}
		encryption = bucket.DefaultEncryption
		return nil
	})
	return encryption, err
}

func (r *Repository) DeleteBucketEncryption(ctx context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.DefaultEncryption = model.ServerSideEncryption{}
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(audit, model.AuditActionDeleteBucketEncryption, bucket.BucketID, "", "", nil))
	})
	return cloneBucket(bucket), err
}

func (r *Repository) PutBucketQuota(ctx context.Context, req meta.BucketQuotaRequest) (model.BucketQuota, error) {
	if err := ctx.Err(); err != nil {
		return model.BucketQuota{}, err
	}
	var quota model.BucketQuota
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		existing, _, err := getJSON[model.BucketQuota](tx, keyspace.BucketQuota(req.BucketID))
		if err != nil {
			return err
		}
		quota, err = meta.BuildBucketQuota(existing, req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.BucketQuota(req.BucketID), quota)
	})
	return meta.CloneBucketQuotaRecord(quota), err
}

func (r *Repository) GetBucketQuota(ctx context.Context, bucketID string) (model.BucketQuota, error) {
	if err := ctx.Err(); err != nil {
		return model.BucketQuota{}, err
	}
	var quota model.BucketQuota
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		var ok bool
		var err error
		quota, ok, err = getJSON[model.BucketQuota](tx, keyspace.BucketQuota(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		return nil
	})
	return meta.CloneBucketQuotaRecord(quota), err
}

func (r *Repository) DeleteBucketQuota(ctx context.Context, bucketID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		if _, ok, err := getJSON[model.BucketQuota](tx, keyspace.BucketQuota(bucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		return tx.Delete(keyspace.BucketQuota(bucketID))
	})
}

func (r *Repository) PutBucketPolicy(ctx context.Context, req meta.BucketPolicyRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if len(req.Policy.Statements) == 0 {
		return model.Bucket{}, fmt.Errorf("%w: bucket policy statements are required", meta.ErrInvalidArgument)
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.Policy = clonePolicy(req.Policy)
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(req.Audit, model.AuditActionPutBucketPolicy, bucket.BucketID, "", "", map[string]string{
			"statement_count": strconv.Itoa(len(req.Policy.Statements)),
		}))
	})
	return cloneBucket(bucket), err
}

func (r *Repository) GetBucketPolicy(ctx context.Context, bucketID string) (auth.PolicyDocument, error) {
	if err := ctx.Err(); err != nil {
		return auth.PolicyDocument{}, err
	}
	var policy auth.PolicyDocument
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if len(bucket.Policy.Statements) == 0 {
			return meta.ErrNotFound
		}
		policy = clonePolicy(bucket.Policy)
		return nil
	})
	return policy, err
}

func (r *Repository) DeleteBucketPolicy(ctx context.Context, bucketID string, audit meta.AuditContext) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.Policy = auth.PolicyDocument{}
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(audit, model.AuditActionDeleteBucketPolicy, bucket.BucketID, "", "", nil))
	})
	return cloneBucket(bucket), err
}

func (r *Repository) PutBucketObjectLock(ctx context.Context, req meta.BucketObjectLockRequest) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	if req.BucketID == "" {
		return model.Bucket{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	if err := validateBucketObjectLockConfiguration(req.Configuration); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var ok bool
		var err error
		bucket, ok, err = getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		bucket.ObjectLock = req.Configuration
		bucket.VersioningState = model.BucketVersioningEnabled
		if err := setJSON(tx, keyspace.BucketByID(bucket.BucketID), bucket); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(req.Audit, model.AuditActionPutBucketObjectLock, bucket.BucketID, "", "", bucketObjectLockAuditDetails(req.Configuration)))
	})
	return bucket, err
}

func (r *Repository) GetBucketObjectLock(ctx context.Context, bucketID string) (model.BucketObjectLockConfiguration, error) {
	if err := ctx.Err(); err != nil {
		return model.BucketObjectLockConfiguration{}, err
	}
	var configuration model.BucketObjectLockConfiguration
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		configuration = bucket.ObjectLock
		return nil
	})
	return configuration, err
}

func (r *Repository) ListBuckets(ctx context.Context, tenantID string) ([]model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	buckets := make([]model.Bucket, 0)
	err := r.run(ctx, func(tx ReadWriter) error {
		return scanJSON[model.Bucket](tx, bucketByIDPrefix(), func(bucket model.Bucket) error {
			if tenantID == "" || bucket.TenantID == tenantID {
				buckets = append(buckets, bucket)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(buckets, func(i, j int) bool {
		return buckets[i].Name < buckets[j].Name
	})
	return buckets, nil
}

func (r *Repository) GetBucketByName(ctx context.Context, name string) (model.Bucket, error) {
	if err := ctx.Err(); err != nil {
		return model.Bucket{}, err
	}
	var bucket model.Bucket
	err := r.run(ctx, func(tx ReadWriter) error {
		var err error
		bucket, err = getBucketByName(tx, name)
		return err
	})
	return bucket, err
}

func (r *Repository) DeleteBucket(ctx context.Context, bucketID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		empty := true
		if err := scanJSON[model.ObjectHead](tx, objectHeadPrefix(bucketID), func(model.ObjectHead) error {
			empty = false
			return errStopScan
		}); err != nil {
			return err
		}
		if empty {
			if err := scanJSON[model.ObjectVersion](tx, versionsBucketPrefix(bucketID), func(version model.ObjectVersion) error {
				if version.State == model.ObjectVersionCommitted {
					empty = false
					return errStopScan
				}
				return nil
			}); err != nil {
				return err
			}
		}
		if !empty {
			return meta.ErrBucketNotEmpty
		}
		if err := tx.Delete(keyspace.BucketByID(bucketID)); err != nil {
			return err
		}
		if err := tx.Delete(keyspace.BucketQuota(bucketID)); err != nil {
			return err
		}
		return tx.Delete(keyspace.BucketByName(bucket.Name))
	})
}

func (r *Repository) BeginPutObject(ctx context.Context, req meta.BeginPutObjectRequest) (model.PendingObjectVersion, error) {
	if err := ctx.Err(); err != nil {
		return model.PendingObjectVersion{}, err
	}
	if req.BucketID == "" || req.Key == "" {
		return model.PendingObjectVersion{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.ObjectLockRetention, req.ObjectLockLegalHold); err != nil {
		return model.PendingObjectVersion{}, err
	}
	segmentRefs := normalizeSegmentRefs(req.SegmentRef, req.SegmentRefs)
	if err := meta.ValidateObjectManifestScale(segmentRefs); err != nil {
		return model.PendingObjectVersion{}, err
	}
	var pending model.PendingObjectVersion
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		currentHead, currentHeadFound, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if currentHeadFound {
			currentHead, err = hydrateHeadManifest(tx, currentHead)
			if err != nil {
				return err
			}
		}
		versionID, err := r.nextObjectVersionID(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		now := r.now()
		versionSortKey := versionID
		version := model.ObjectVersion{
			BucketID:             req.BucketID,
			Key:                  req.Key,
			VersionID:            versionID,
			VersionSortKey:       versionSortKey,
			SizeBytes:            req.SizeBytes,
			ETag:                 req.ETag,
			ContentType:          req.ContentType,
			StorageClass:         cloneStorageClass(req.StorageClass),
			ServerSideEncryption: req.ServerSideEncryption,
			SegmentRef:           firstSegmentRef(segmentRefs),
			SegmentRefs:          cloneSegmentRefs(segmentRefs),
			UserMetadata:         cloneStringMap(req.UserMetadata),
			Tags:                 cloneStringMap(req.Tags),
			ObjectLockRetention:  req.ObjectLockRetention,
			ObjectLockLegalHold:  req.ObjectLockLegalHold,
			State:                model.ObjectVersionPending,
			CreatedAt:            now,
		}
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		pending = model.PendingObjectVersion{
			Version:           version,
			BaseHeadVersionID: currentHead.VersionID,
			BaseHead:          cloneHead(currentHead),
			BaseHeadFound:     currentHeadFound,
		}
		return nil
	})
	return pending, err
}

func (r *Repository) CommitObjectVersion(ctx context.Context, req meta.CommitObjectVersionRequest) (model.ObjectHead, error) {
	if err := ctx.Err(); err != nil {
		return model.ObjectHead{}, err
	}
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return model.ObjectHead{}, fmt.Errorf("%w: commit fields are required", meta.ErrInvalidArgument)
	}
	var head model.ObjectHead
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		currentHead, _, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if currentHead.VersionID != req.ExpectedHeadVersionID {
			return meta.ErrCASConflict
		}
		_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if version.State == model.ObjectVersionCommitted {
			head, err = hydrateHeadManifest(tx, currentHead)
			return nil
		}
		now := r.now()
		version.State = model.ObjectVersionCommitted
		version.CommittedAt = now
		head = model.ObjectHead{
			BucketID:             version.BucketID,
			Key:                  version.Key,
			VersionID:            version.VersionID,
			SizeBytes:            version.SizeBytes,
			ETag:                 version.ETag,
			ContentType:          version.ContentType,
			StorageClass:         cloneStorageClass(version.StorageClass),
			ServerSideEncryption: version.ServerSideEncryption,
			SegmentRef:           version.SegmentRef,
			SegmentRefs:          cloneSegmentRefs(version.SegmentRefs),
			UserMetadata:         cloneStringMap(version.UserMetadata),
			Tags:                 cloneStringMap(version.Tags),
			ObjectLockRetention:  version.ObjectLockRetention,
			ObjectLockLegalHold:  version.ObjectLockLegalHold,
			LastModified:         now,
			DeleteMarker:         version.DeleteMarker,
		}
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		head, err = setObjectHead(tx, head)
		if err != nil {
			return err
		}
		if err := setListObject(tx, head); err != nil {
			return err
		}
		if err := r.syncProtectedRefsForVersion(tx, version, now); err != nil {
			return err
		}
		if bucket.VersioningState != model.BucketVersioningEnabled && currentHead.VersionID != "" && currentHead.VersionID != version.VersionID {
			oldKey, oldVersion, ok, err := findVersionByID(tx, req.BucketID, req.Key, currentHead.VersionID)
			if err != nil {
				return err
			}
			if ok {
				if err := deleteProtectedRefsForVersion(tx, oldVersion.BucketID, oldVersion.Key, oldVersion.VersionID); err != nil {
					return err
				}
				return deleteObjectVersionRecord(tx, oldKey, oldVersion)
			}
		}
		return nil
	})
	return head, err
}

func (r *Repository) PutObjectVersion(ctx context.Context, req meta.PutObjectVersionRequest) (meta.PutObjectVersionResult, error) {
	if err := ctx.Err(); err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	if req.BucketID == "" || req.Key == "" {
		return meta.PutObjectVersionResult{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.ObjectLockRetention, req.ObjectLockLegalHold); err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	segmentRefs := normalizeSegmentRefs(req.SegmentRef, req.SegmentRefs)
	if err := meta.ValidateObjectManifestScale(segmentRefs); err != nil {
		return meta.PutObjectVersionResult{}, err
	}
	var result meta.PutObjectVersionResult
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		currentHead, currentHeadFound, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if currentHeadFound {
			currentHead, err = hydrateHeadManifest(tx, currentHead)
			if err != nil {
				return err
			}
			result.ReplacedHead = cloneHead(currentHead)
			result.ReplacedHeadFound = true
		}
		versionID, err := r.nextObjectVersionID(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		now := r.now()
		versionSortKey := versionID
		version := model.ObjectVersion{
			BucketID:             req.BucketID,
			Key:                  req.Key,
			VersionID:            versionID,
			VersionSortKey:       versionSortKey,
			SizeBytes:            req.SizeBytes,
			ETag:                 req.ETag,
			ContentType:          req.ContentType,
			StorageClass:         cloneStorageClass(req.StorageClass),
			ServerSideEncryption: req.ServerSideEncryption,
			SegmentRef:           firstSegmentRef(segmentRefs),
			SegmentRefs:          cloneSegmentRefs(segmentRefs),
			UserMetadata:         cloneStringMap(req.UserMetadata),
			Tags:                 cloneStringMap(req.Tags),
			ObjectLockRetention:  req.ObjectLockRetention,
			ObjectLockLegalHold:  req.ObjectLockLegalHold,
			State:                model.ObjectVersionCommitted,
			CreatedAt:            now,
			CommittedAt:          now,
		}
		head := model.ObjectHead{
			BucketID:             version.BucketID,
			Key:                  version.Key,
			VersionID:            version.VersionID,
			SizeBytes:            version.SizeBytes,
			ETag:                 version.ETag,
			ContentType:          version.ContentType,
			StorageClass:         cloneStorageClass(version.StorageClass),
			ServerSideEncryption: version.ServerSideEncryption,
			SegmentRef:           version.SegmentRef,
			SegmentRefs:          cloneSegmentRefs(version.SegmentRefs),
			UserMetadata:         cloneStringMap(version.UserMetadata),
			Tags:                 cloneStringMap(version.Tags),
			ObjectLockRetention:  version.ObjectLockRetention,
			ObjectLockLegalHold:  version.ObjectLockLegalHold,
			LastModified:         now,
			DeleteMarker:         version.DeleteMarker,
		}
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		head, err = setObjectHead(tx, head)
		if err != nil {
			return err
		}
		if err := setListObject(tx, head); err != nil {
			return err
		}
		if err := r.syncProtectedRefsForVersion(tx, version, now); err != nil {
			return err
		}
		if bucket.VersioningState != model.BucketVersioningEnabled && currentHead.VersionID != "" && currentHead.VersionID != version.VersionID {
			oldKey, oldVersion, ok, err := findVersionByID(tx, req.BucketID, req.Key, currentHead.VersionID)
			if err != nil {
				return err
			}
			if ok {
				if err := deleteProtectedRefsForVersion(tx, oldVersion.BucketID, oldVersion.Key, oldVersion.VersionID); err != nil {
					return err
				}
				if err := deleteObjectVersionRecord(tx, oldKey, oldVersion); err != nil {
					return err
				}
			}
		}
		result.Head = cloneHead(head)
		return nil
	})
	return result, err
}

func (r *Repository) GetObjectHead(ctx context.Context, bucketID, objectKey string) (model.ObjectHead, error) {
	if err := ctx.Err(); err != nil {
		return model.ObjectHead{}, err
	}
	var head model.ObjectHead
	err := r.run(ctx, func(tx ReadWriter) error {
		got, ok, err := getObjectHead(tx, bucketID, objectKey)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		head, err = hydrateHeadManifest(tx, got)
		return nil
	})
	return head, err
}

func (r *Repository) GetObjectVersion(ctx context.Context, bucketID, objectKey, versionID string) (model.ObjectVersion, error) {
	if err := ctx.Err(); err != nil {
		return model.ObjectVersion{}, err
	}
	var version model.ObjectVersion
	err := r.run(ctx, func(tx ReadWriter) error {
		_, got, ok, err := findVersionByID(tx, bucketID, objectKey, versionID)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		version = cloneVersion(got)
		return nil
	})
	return version, err
}

func (r *Repository) DeleteObject(ctx context.Context, req meta.DeleteObjectRequest) (model.DeleteResult, error) {
	if err := ctx.Err(); err != nil {
		return model.DeleteResult{}, err
	}
	if req.BucketID == "" || req.Key == "" {
		return model.DeleteResult{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	var result model.DeleteResult
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if req.VersionID != "" {
			result, err = r.deleteObjectVersion(tx, req, r.now())
			return err
		}
		if bucket.VersioningState == model.BucketVersioningEnabled {
			head, ok, err := getObjectHead(tx, req.BucketID, req.Key)
			if err != nil {
				return err
			}
			if ok {
				if err := checkDeleteKMSAdmission(tx, head.ServerSideEncryption); err != nil {
					return err
				}
			}
			result, err = r.createDeleteMarker(tx, req)
			return err
		}
		head, ok, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if !ok {
			result = model.DeleteResult{Deleted: false}
			return nil
		}
		if err := checkDeleteKMSAdmission(tx, head.ServerSideEncryption); err != nil {
			return err
		}
		now := r.now()
		if objectHeadProtectedByObjectLock(head, now, false) {
			if objectHeadProtectedByObjectLock(head, now, true) || !req.BypassGovernanceRetention || !validGovernanceBypassAudit(req.BypassAudit) {
				return meta.ErrObjectLocked
			}
			if err := r.appendAuditEvent(tx, governanceBypassAuditEvent(req, model.AuditActionGovernanceBypassDeleteObject, head.VersionID, map[string]string{
				"target": "object_head",
			})); err != nil {
				return err
			}
		}
		if err := tx.Delete(keyspace.ObjectHead(req.BucketID, req.Key)); err != nil {
			return err
		}
		if err := tx.Delete(keyspace.ListObject(req.BucketID, req.Key)); err != nil {
			return err
		}
		versionKey, deleted, ok, err := findVersionByID(tx, req.BucketID, req.Key, head.VersionID)
		if err != nil {
			return err
		}
		if ok {
			if err := deleteProtectedRefsForVersion(tx, deleted.BucketID, deleted.Key, deleted.VersionID); err != nil {
				return err
			}
			if err := deleteObjectVersionRecord(tx, versionKey, deleted); err != nil {
				return err
			}
		}
		result = model.DeleteResult{
			Deleted:          true,
			DeletedVersionID: head.VersionID,
			DeletedVersion:   cloneVersion(deleted),
		}
		return nil
	})
	return result, err
}

func (r *Repository) ListObjects(ctx context.Context, req meta.ListObjectsRequest) (model.ListObjectsResult, error) {
	if err := ctx.Err(); err != nil {
		return model.ListObjectsResult{}, err
	}
	if req.BucketID == "" {
		return model.ListObjectsResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	maxKeys := req.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	return r.listObjectsRange(ctx, req, maxKeys)
}

func (r *Repository) listObjectsRange(ctx context.Context, req meta.ListObjectsRequest, maxKeys int) (model.ListObjectsResult, error) {
	entries := make([]listEntry, 0)
	commonPrefixes := make(map[string]struct{})
	if err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		start := keyspace.ListObject(req.BucketID, req.Prefix)
		end := prefixRangeEndString(start)
		cursor := ""
		if req.ContinuationToken != "" {
			tokenKey := keyspace.ListObject(req.BucketID, req.ContinuationToken)
			if req.Delimiter != "" && strings.HasSuffix(req.ContinuationToken, req.Delimiter) {
				start = prefixRangeEndString(tokenKey)
			} else {
				cursor = tokenKey
			}
		}
		for len(entries) <= maxKeys {
			keys, next, err := tx.ListRange(start, end, cursor, maxKeys+1)
			if err != nil {
				return err
			}
			if len(keys) == 0 {
				return nil
			}
			cursor = ""
			skipTo := ""
			for _, key := range keys {
				entry, ok, err := getJSON[model.ObjectListEntry](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					cursor = key
					continue
				}
				head := headFromListEntry(entry)
				if !strings.HasPrefix(head.Key, req.Prefix) {
					cursor = key
					continue
				}
				if req.Delimiter != "" {
					rest := strings.TrimPrefix(head.Key, req.Prefix)
					if index := strings.Index(rest, req.Delimiter); index >= 0 {
						commonPrefix := req.Prefix + rest[:index+len(req.Delimiter)]
						if _, ok := commonPrefixes[commonPrefix]; !ok {
							commonPrefixes[commonPrefix] = struct{}{}
							entries = append(entries, listEntry{name: commonPrefix, prefix: true})
						}
						skipTo = prefixRangeEndString(keyspace.ListObject(req.BucketID, commonPrefix))
						break
					}
				}
				entries = append(entries, listEntry{name: head.Key, head: head})
				cursor = key
				if len(entries) > maxKeys {
					return nil
				}
			}
			if skipTo != "" {
				start = skipTo
				cursor = ""
				continue
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
		return nil
	}); err != nil {
		return model.ListObjectsResult{}, err
	}
	result := model.ListObjectsResult{}
	if len(entries) > maxKeys {
		result.IsTruncated = true
		result.NextContinuationToken = entries[maxKeys-1].name
		entries = entries[:maxKeys]
	}
	for _, entry := range entries {
		if entry.prefix {
			result.CommonPrefixes = append(result.CommonPrefixes, entry.name)
			continue
		}
		result.Contents = append(result.Contents, cloneHead(entry.head))
	}
	return result, nil
}

func (r *Repository) ListObjectVersions(ctx context.Context, req meta.ListObjectVersionsRequest) (model.ListObjectVersionsResult, error) {
	if err := ctx.Err(); err != nil {
		return model.ListObjectVersionsResult{}, err
	}
	if req.BucketID == "" {
		return model.ListObjectVersionsResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	maxKeys := req.MaxKeys
	if maxKeys <= 0 {
		maxKeys = 1000
	}
	entries := make([]versionListEntry, 0)
	commonPrefixes := make(map[string]struct{})
	if err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		start := versionListPrefix(req.BucketID, req.Prefix)
		end := prefixRangeEndString(start)
		if req.KeyMarker != "" {
			if req.Delimiter != "" && req.VersionIDMarker == "" && strings.HasSuffix(req.KeyMarker, req.Delimiter) {
				start = maxString(start, prefixRangeEndString(versionListPrefix(req.BucketID, req.KeyMarker)))
			} else {
				exactMarkerPrefix := versionPrefix(req.BucketID, req.KeyMarker)
				if req.VersionIDMarker != "" {
					if exactMarkerPrefix >= start && (end == "" || exactMarkerPrefix < end) {
						markerEntries, err := collectVersionEntriesForKey(tx, req.BucketID, req.KeyMarker)
						if err != nil {
							return err
						}
						markerEntries = versionEntriesAfter(markerEntries, req.KeyMarker, req.VersionIDMarker)
						entries = append(entries, markerEntries...)
					}
				}
				start = maxString(start, prefixRangeEndString(exactMarkerPrefix))
			}
		}
		for len(entries) <= maxKeys {
			keys, _, err := tx.ListRange(start, end, "", 1)
			if err != nil {
				return err
			}
			if len(keys) == 0 {
				return nil
			}
			version, ok, err := firstVersionFromKeys(tx, keys)
			if err != nil {
				return err
			}
			if !ok {
				start = prefixRangeEndString(start)
				continue
			}
			objectKey := version.Key
			exactObjectPrefix := versionPrefix(req.BucketID, objectKey)
			if !strings.HasPrefix(objectKey, req.Prefix) {
				start = prefixRangeEndString(exactObjectPrefix)
				continue
			}
			if req.Delimiter != "" {
				rest := strings.TrimPrefix(objectKey, req.Prefix)
				if index := strings.Index(rest, req.Delimiter); index >= 0 {
					commonPrefix := req.Prefix + rest[:index+len(req.Delimiter)]
					if _, ok := commonPrefixes[commonPrefix]; !ok {
						commonPrefixes[commonPrefix] = struct{}{}
						entries = append(entries, versionListEntry{name: commonPrefix, prefix: true})
					}
					start = prefixRangeEndString(versionListPrefix(req.BucketID, commonPrefix))
					continue
				}
			}
			objectEntries, err := collectVersionEntriesForKey(tx, req.BucketID, objectKey)
			if err != nil {
				return err
			}
			entries = append(entries, objectEntries...)
			start = prefixRangeEndString(exactObjectPrefix)
		}
		return nil
	}); err != nil {
		return model.ListObjectVersionsResult{}, err
	}
	result := model.ListObjectVersionsResult{}
	if len(entries) > maxKeys {
		result.IsTruncated = true
		result.NextKeyMarker = entries[maxKeys-1].name
		result.NextVersionIDMarker = entries[maxKeys-1].versionID
		entries = entries[:maxKeys]
	}
	for _, entry := range entries {
		if entry.prefix {
			result.CommonPrefixes = append(result.CommonPrefixes, entry.name)
			continue
		}
		versionEntry := model.ObjectVersionEntry{
			Version:  cloneVersion(entry.version),
			IsLatest: entry.isLatest,
		}
		if entry.version.DeleteMarker {
			result.DeleteMarkers = append(result.DeleteMarkers, versionEntry)
		} else {
			result.Versions = append(result.Versions, versionEntry)
		}
	}
	return result, nil
}

func (r *Repository) GetObjectTags(ctx context.Context, req meta.ObjectTagsRequest) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.BucketID == "" || req.Key == "" {
		return nil, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	var tags map[string]string
	err := r.run(ctx, func(tx ReadWriter) error {
		if req.VersionID != "" {
			_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
			if err != nil {
				return err
			}
			if !ok || version.State != model.ObjectVersionCommitted {
				return meta.ErrNotFound
			}
			tags = cloneStringMap(version.Tags)
			return nil
		}
		head, ok, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		tags = cloneStringMap(head.Tags)
		return nil
	})
	return tags, err
}

func (r *Repository) PutObjectTags(ctx context.Context, req meta.ObjectTagsRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.BucketID == "" || req.Key == "" {
		return fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	tags := cloneStringMap(req.Tags)
	return r.run(ctx, func(tx ReadWriter) error {
		if req.VersionID != "" {
			_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
			if err != nil {
				return err
			}
			if !ok || version.State != model.ObjectVersionCommitted {
				return meta.ErrNotFound
			}
			version.Tags = tags
			if err := setObjectVersion(tx, version); err != nil {
				return err
			}
			head, headOK, err := getObjectHead(tx, req.BucketID, req.Key)
			if err != nil {
				return err
			}
			if headOK && head.VersionID == req.VersionID {
				head.Tags = cloneStringMap(tags)
				head, err = setObjectHead(tx, head)
				if err != nil {
					return err
				}
				if err := setListObject(tx, head); err != nil {
					return err
				}
			}
			return nil
		}
		head, ok, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		head.Tags = tags
		head, err = setObjectHead(tx, head)
		if err != nil {
			return err
		}
		if err := setListObject(tx, head); err != nil {
			return err
		}
		if head.VersionID != "" {
			_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, head.VersionID)
			if err != nil {
				return err
			}
			if ok {
				version.Tags = cloneStringMap(tags)
				if err := setObjectVersion(tx, version); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *Repository) DeleteObjectTags(ctx context.Context, req meta.ObjectTagsRequest) error {
	req.Tags = nil
	return r.PutObjectTags(ctx, req)
}

func (r *Repository) GetObjectRetention(ctx context.Context, req meta.ObjectRetentionRequest) (model.ObjectLockRetention, error) {
	if err := ctx.Err(); err != nil {
		return model.ObjectLockRetention{}, err
	}
	if req.BucketID == "" || req.Key == "" {
		return model.ObjectLockRetention{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	var retention model.ObjectLockRetention
	err := r.run(ctx, func(tx ReadWriter) error {
		_, version, err := objectLockTarget(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		retention = version.ObjectLockRetention
		return nil
	})
	return retention, err
}

func (r *Repository) PutObjectRetention(ctx context.Context, req meta.ObjectRetentionRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.BucketID == "" || req.Key == "" {
		return fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.Retention, ""); err != nil {
		return err
	}
	now := r.now()
	if !req.Retention.RetainUntilDate.After(now) {
		return fmt.Errorf("%w: object lock retain-until date must be in the future", meta.ErrInvalidArgument)
	}
	return r.run(ctx, func(tx ReadWriter) error {
		_, version, err := objectLockTarget(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		if retentionUpdateBlocked(version.ObjectLockRetention, req.Retention, now, false) {
			if retentionUpdateBlocked(version.ObjectLockRetention, req.Retention, now, true) || !req.BypassGovernanceRetention || !validGovernanceBypassAudit(req.BypassAudit) {
				return meta.ErrObjectLocked
			}
			if err := r.appendAuditEvent(tx, governanceBypassAuditEvent(meta.DeleteObjectRequest{
				BucketID:    req.BucketID,
				Key:         req.Key,
				VersionID:   version.VersionID,
				BypassAudit: req.BypassAudit,
			}, model.AuditActionGovernanceBypassPutObjectRetention, version.VersionID, map[string]string{
				"current_mode":         string(version.ObjectLockRetention.Mode),
				"current_retain_until": version.ObjectLockRetention.RetainUntilDate.UTC().Format(time.RFC3339Nano),
				"next_mode":            string(req.Retention.Mode),
				"next_retain_until":    req.Retention.RetainUntilDate.UTC().Format(time.RFC3339Nano),
			})); err != nil {
				return err
			}
		}
		previous := version.ObjectLockRetention
		version.ObjectLockRetention = req.Retention
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		if err := updateHeadObjectLock(tx, version); err != nil {
			return err
		}
		if err := r.syncProtectedRefsForVersion(tx, version, now); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(req.Audit, model.AuditActionPutObjectRetention, req.BucketID, req.Key, version.VersionID, map[string]string{
			"previous_mode":         string(previous.Mode),
			"previous_retain_until": formatAuditTime(previous.RetainUntilDate),
			"next_mode":             string(req.Retention.Mode),
			"next_retain_until":     formatAuditTime(req.Retention.RetainUntilDate),
		}))
	})
}

func (r *Repository) GetObjectLegalHold(ctx context.Context, req meta.ObjectLegalHoldRequest) (model.ObjectLockLegalHoldStatus, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if req.BucketID == "" || req.Key == "" {
		return "", fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	var legalHold model.ObjectLockLegalHoldStatus
	err := r.run(ctx, func(tx ReadWriter) error {
		_, version, err := objectLockTarget(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		legalHold = version.ObjectLockLegalHold
		return nil
	})
	return legalHold, err
}

func (r *Repository) PutObjectLegalHold(ctx context.Context, req meta.ObjectLegalHoldRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if req.BucketID == "" || req.Key == "" {
		return fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	switch req.LegalHold {
	case model.ObjectLockLegalHoldOn, model.ObjectLockLegalHoldOff:
	default:
		return fmt.Errorf("%w: object lock legal hold status is invalid", meta.ErrInvalidArgument)
	}
	return r.run(ctx, func(tx ReadWriter) error {
		_, version, err := objectLockTarget(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		previous := version.ObjectLockLegalHold
		version.ObjectLockLegalHold = req.LegalHold
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		if err := updateHeadObjectLock(tx, version); err != nil {
			return err
		}
		if err := r.syncProtectedRefsForVersion(tx, version, r.now()); err != nil {
			return err
		}
		return r.appendAuditEvent(tx, transitionAuditEvent(req.Audit, model.AuditActionPutObjectLegalHold, req.BucketID, req.Key, version.VersionID, map[string]string{
			"previous_legal_hold": string(previous),
			"next_legal_hold":     string(req.LegalHold),
		}))
	})
}

func (r *Repository) ListAuditEvents(ctx context.Context, req meta.ListAuditEventsRequest) ([]model.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var events []model.AuditEvent
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for len(events) < limit {
			keys, next, err := tx.List(keyspace.AuditEvent(""), cursor, limit-len(events))
			if err != nil {
				return err
			}
			for _, key := range keys {
				event, ok, err := getJSON[model.AuditEvent](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.BucketID != "" && event.BucketID != req.BucketID {
					continue
				}
				if req.Key != "" && event.Key != req.Key {
					continue
				}
				if req.Action != "" && event.Action != req.Action {
					continue
				}
				events = append(events, cloneAuditEvent(event))
				if len(events) >= limit {
					break
				}
			}
			if next == "" || len(events) >= limit {
				return nil
			}
			cursor = next
		}
		return nil
	})
	return events, err
}

func (r *Repository) PutAdminAuditEvent(ctx context.Context, req meta.PutAdminAuditEventRequest) (model.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return model.AuditEvent{}, err
	}
	if req.Action == "" {
		return model.AuditEvent{}, fmt.Errorf("%w: audit action is required", meta.ErrInvalidArgument)
	}

	var event model.AuditEvent
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.nextSequence(tx, sequenceAudit)
		if err != nil {
			return err
		}
		head, _, err := getJSON[auditHeadRecord](tx, keyspace.AuditHead())
		if err != nil {
			return err
		}
		event = transitionAuditEvent(req.Audit, req.Action, req.BucketID, req.Key, req.VersionID, req.Details)
		event.EventID = fmt.Sprintf("audit-%020d", sequence)
		event.CreatedAt = r.now()
		event.PreviousHash = head.LastHash
		event.EventHash = auditEventHash(event)
		if err := setJSON(tx, keyspace.AuditEvent(event.EventID), event); err != nil {
			return err
		}
		head.LastHash = event.EventHash
		return setJSON(tx, keyspace.AuditHead(), head)
	})
	return cloneAuditEvent(event), err
}

func (r *Repository) PutAdminAuditEvents(ctx context.Context, req meta.PutAdminAuditEventsRequest) ([]model.AuditEvent, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(req.Events) == 0 {
		return nil, nil
	}
	for _, eventReq := range req.Events {
		if eventReq.Action == "" {
			return nil, fmt.Errorf("%w: audit action is required", meta.ErrInvalidArgument)
		}
	}

	events := make([]model.AuditEvent, 0, len(req.Events))
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.reserveSequences(tx, sequenceAudit, len(req.Events))
		if err != nil {
			return err
		}
		head, _, err := getJSON[auditHeadRecord](tx, keyspace.AuditHead())
		if err != nil {
			return err
		}
		now := r.now()
		for _, eventReq := range req.Events {
			event := transitionAuditEvent(eventReq.Audit, eventReq.Action, eventReq.BucketID, eventReq.Key, eventReq.VersionID, eventReq.Details)
			event.EventID = fmt.Sprintf("audit-%020d", sequence)
			event.CreatedAt = now
			event.PreviousHash = head.LastHash
			event.EventHash = auditEventHash(event)
			if err := setJSON(tx, keyspace.AuditEvent(event.EventID), event); err != nil {
				return err
			}
			head.LastHash = event.EventHash
			events = append(events, cloneAuditEvent(event))
			sequence++
		}
		return setJSON(tx, keyspace.AuditHead(), head)
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (r *Repository) ImportOperationalMetadata(ctx context.Context, req meta.ImportOperationalMetadataRequest) (meta.ImportOperationalMetadataResult, error) {
	if err := ctx.Err(); err != nil {
		return meta.ImportOperationalMetadataResult{}, err
	}
	var result meta.ImportOperationalMetadataResult
	err := r.run(ctx, func(tx ReadWriter) error {
		if req.RequireEmptyTarget {
			empty, err := operationalMetadataEmpty(tx)
			if err != nil {
				return err
			}
			if !empty {
				return fmt.Errorf("%w: target operational metadata is not empty", meta.ErrAlreadyExists)
			}
		}
		if err := validateOperationalMetadataImport(tx, req); err != nil {
			return err
		}

		if req.MetadataSchema != nil {
			if err := setJSON(tx, keyspace.MetadataSchema(), *req.MetadataSchema); err != nil {
				return err
			}
		}
		for _, record := range req.MetadataMigrationOperations {
			record = meta.CloneMetadataMigrationOperationRecord(record)
			if err := setJSON(tx, keyspace.MetadataMigrationOperation(record.OperationID), record); err != nil {
				return err
			}
			if err := updateSequenceAtLeast(tx, sequenceMetadataMigration, numericIDSuffix(record.OperationID, "metadata-migration-")); err != nil {
				return err
			}
		}
		var head auditHeadRecord
		for _, event := range req.AuditEvents {
			event = cloneAuditEvent(event)
			if err := setJSON(tx, keyspace.AuditEvent(event.EventID), event); err != nil {
				return err
			}
			head.LastHash = event.EventHash
			if err := updateSequenceAtLeast(tx, sequenceAudit, numericIDSuffix(event.EventID, "audit-")); err != nil {
				return err
			}
		}
		if len(req.AuditEvents) > 0 {
			if err := setJSON(tx, keyspace.AuditHead(), head); err != nil {
				return err
			}
		}
		for _, record := range req.KMSKeys {
			record.KeyID = strings.TrimSpace(record.KeyID)
			record.State = model.NormalizeKMSKeyState(record.State)
			if err := setJSON(tx, keyspace.KMSKey(record.KeyID), record); err != nil {
				return err
			}
		}
		for _, record := range req.GCOperations {
			record = cloneGCOperationRecord(record)
			if err := setJSON(tx, keyspace.GCOperation(record.OperationID), record); err != nil {
				return err
			}
			if err := updateSequenceAtLeast(tx, sequenceGCOperation, numericIDSuffix(record.OperationID, "gc-")); err != nil {
				return err
			}
		}
		for _, record := range req.DedupeOperations {
			record = cloneDedupeOperationRecord(record)
			if err := setJSON(tx, keyspace.DedupeOperation(record.OperationID), record); err != nil {
				return err
			}
			if err := updateSequenceAtLeast(tx, sequenceDedupeOperation, numericIDSuffix(record.OperationID, "dedupe-")); err != nil {
				return err
			}
		}
		for _, shared := range req.SharedObjects {
			if err := setJSON(tx, keyspace.SharedObject(shared.SharedObjectID), cloneSharedObject(shared)); err != nil {
				return err
			}
		}
		for _, release := range req.SharedObjectReleases {
			release = cloneSharedObjectRelease(release)
			if err := setJSON(tx, keyspace.SharedObjectRelease(release.SharedObjectID, release.SegmentID), release); err != nil {
				return err
			}
		}
		for _, pool := range req.VolumePools {
			pool = meta.CloneVolumePool(pool)
			if err := setJSON(tx, keyspace.VolumePool(pool.PoolID), pool); err != nil {
				return err
			}
		}
		for _, record := range req.VolumeDrainOperations {
			record = meta.CloneVolumeDrainOperationRecord(record)
			if err := setJSON(tx, keyspace.VolumeDrainOperation(record.OperationID), record); err != nil {
				return err
			}
			if err := updateSequenceAtLeast(tx, sequenceVolumeDrain, numericIDSuffix(record.OperationID, "drain-")); err != nil {
				return err
			}
		}
		for _, lease := range req.WorkerLeases {
			lease = meta.CloneWorkerLease(lease)
			if err := setJSON(tx, keyspace.WorkerLease(lease.WorkerKind, lease.ShardID), lease); err != nil {
				return err
			}
		}
		for _, record := range req.WorkerOperations {
			record = meta.CloneWorkerOperationRecord(record)
			if err := setJSON(tx, keyspace.WorkerOperation(record.OperationID), record); err != nil {
				return err
			}
			if err := updateSequenceAtLeast(tx, sequenceWorkerOperation, numericIDSuffix(record.OperationID, "worker-op-")); err != nil {
				return err
			}
		}

		result = meta.ImportOperationalMetadataResult{
			MetadataSchema:              boolCount(req.MetadataSchema != nil),
			MetadataMigrationOperations: len(req.MetadataMigrationOperations),
			KMSKeys:                     len(req.KMSKeys),
			AuditEvents:                 len(req.AuditEvents),
			GCOperations:                len(req.GCOperations),
			DedupeOperations:            len(req.DedupeOperations),
			SharedObjects:               len(req.SharedObjects),
			SharedObjectReleases:        len(req.SharedObjectReleases),
			VolumePools:                 len(req.VolumePools),
			VolumeDrainOperations:       len(req.VolumeDrainOperations),
			WorkerLeases:                len(req.WorkerLeases),
			WorkerOperations:            len(req.WorkerOperations),
		}
		return nil
	})
	return result, err
}

func (r *Repository) ListProtectedRefs(ctx context.Context, req meta.ListProtectedRefsRequest) ([]model.ProtectedRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	now := r.now()
	var refs []model.ProtectedRef
	err := r.run(ctx, func(tx ReadWriter) error {
		prefix := protectedRefSegmentPrefix(req.SegmentID)
		if req.SegmentID == "" && req.BucketID != "" && req.Key != "" && req.VersionID != "" {
			prefix = protectedRefVersionPrefix(req.BucketID, req.Key, req.VersionID)
		}
		cursor := ""
		for len(refs) < limit {
			keys, next, err := tx.List(prefix, cursor, limit-len(refs))
			if err != nil {
				return err
			}
			for _, key := range keys {
				ref, ok, err := getJSON[model.ProtectedRef](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.BucketID != "" && ref.BucketID != req.BucketID {
					continue
				}
				if req.Key != "" && ref.Key != req.Key {
					continue
				}
				if req.VersionID != "" && ref.VersionID != req.VersionID {
					continue
				}
				if req.ActiveOnly && !protectedRefActive(ref, now) {
					continue
				}
				refs = append(refs, cloneProtectedRef(ref))
				if len(refs) >= limit {
					break
				}
			}
			if next == "" || len(refs) >= limit {
				return nil
			}
			cursor = next
		}
		return nil
	})
	return refs, err
}

func (r *Repository) PutGCCandidate(ctx context.Context, req meta.PutGCCandidateRequest) (model.GCCandidateRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.GCCandidateRecord{}, err
	}
	segmentID := strings.TrimSpace(req.SegmentRef.SegmentID)
	var record model.GCCandidateRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.GCCandidate(segmentID)
		existing, _, err := getJSON[model.GCCandidateRecord](tx, key)
		if err != nil {
			return err
		}
		record, err = meta.BuildGCCandidate(existing, req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.GCCandidate(record.SegmentID), record)
	})
	return meta.CloneGCCandidateRecord(record), err
}

func (r *Repository) ListGCCandidates(ctx context.Context, req meta.ListGCCandidatesRequest) ([]model.GCCandidateRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.GCCandidateRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.GCCandidate(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.GCCandidateRecord](tx, key)
				if err != nil {
					return err
				}
				if ok {
					records = append(records, meta.CloneGCCandidateRecord(record))
				}
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt.Equal(records[j].CreatedAt) {
			return records[i].SegmentID < records[j].SegmentID
		}
		return records[i].CreatedAt.Before(records[j].CreatedAt)
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, err
}

func (r *Repository) DeleteGCCandidate(ctx context.Context, segmentID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	segmentID = strings.TrimSpace(segmentID)
	if segmentID == "" {
		return fmt.Errorf("%w: gc candidate segment id is required", meta.ErrInvalidArgument)
	}
	return r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.GCCandidate(segmentID)
		_, ok, err := getJSON[model.GCCandidateRecord](tx, key)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		return tx.Delete(key)
	})
}

func (r *Repository) PutGCOperation(ctx context.Context, req meta.PutGCOperationRequest) (model.GCOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.GCOperationRecord{}, err
	}
	var record model.GCOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.nextSequence(tx, sequenceGCOperation)
		if err != nil {
			return err
		}
		record = model.GCOperationRecord{
			OperationID:         fmt.Sprintf("gc-%020d", sequence),
			ResumeOfOperationID: req.ResumeOfOperationID,
			Status:              normalizeGCOperationStatus(req.Status, req.Retryable),
			StartedAt:           req.StartedAt.UTC(),
			FinishedAt:          req.FinishedAt.UTC(),
			Scanned:             req.Scanned,
			Deleted:             req.Deleted,
			Skipped:             req.Skipped,
			Retryable:           req.Retryable,
			Attempts:            cloneGCOperationAttempts(req.Attempts),
			CreatedAt:           r.now(),
		}
		return setJSON(tx, keyspace.GCOperation(record.OperationID), record)
	})
	return cloneGCOperationRecord(record), err
}

func (r *Repository) ListGCOperations(ctx context.Context, req meta.ListGCOperationsRequest) ([]model.GCOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.GCOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.GCOperation(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.GCOperationRecord](tx, key)
				if err != nil {
					return err
				}
				if ok {
					records = append(records, cloneGCOperationRecord(record))
				}
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(records, func(i, j int) bool {
		return records[i].OperationID > records[j].OperationID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, err
}

func (r *Repository) PutDedupeOperation(ctx context.Context, req meta.PutDedupeOperationRequest) (model.DedupeOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.DedupeOperationRecord{}, err
	}
	var record model.DedupeOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.nextSequence(tx, sequenceDedupeOperation)
		if err != nil {
			return err
		}
		record = model.DedupeOperationRecord{
			OperationID:         fmt.Sprintf("dedupe-%020d", sequence),
			ResumeOfOperationID: req.ResumeOfOperationID,
			Status:              normalizeDedupeOperationStatus(req.Status, req.Retryable),
			StartedAt:           req.StartedAt.UTC(),
			FinishedAt:          req.FinishedAt.UTC(),
			Scanned:             req.Scanned,
			Acked:               req.Acked,
			Skipped:             req.Skipped,
			Retryable:           req.Retryable,
			Attempts:            cloneDedupeOperationAttempts(req.Attempts),
			CreatedAt:           r.now(),
		}
		return setJSON(tx, keyspace.DedupeOperation(record.OperationID), record)
	})
	return cloneDedupeOperationRecord(record), err
}

func (r *Repository) ListDedupeOperations(ctx context.Context, req meta.ListDedupeOperationsRequest) ([]model.DedupeOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.DedupeOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.DedupeOperation(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.DedupeOperationRecord](tx, key)
				if err != nil {
					return err
				}
				if ok {
					records = append(records, cloneDedupeOperationRecord(record))
				}
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(records, func(i, j int) bool {
		return records[i].OperationID > records[j].OperationID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, err
}

func (r *Repository) AcquireDedupeOperationLock(ctx context.Context, req meta.AcquireDedupeOperationLockRequest) (model.DedupeOperationLock, error) {
	if err := ctx.Err(); err != nil {
		return model.DedupeOperationLock{}, err
	}
	lockID := strings.TrimSpace(req.LockID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if lockID == "" || ownerID == "" {
		return model.DedupeOperationLock{}, fmt.Errorf("%w: dedupe lock id and owner id are required", meta.ErrInvalidArgument)
	}
	if req.TTL <= 0 {
		return model.DedupeOperationLock{}, fmt.Errorf("%w: dedupe lock ttl must be positive", meta.ErrInvalidArgument)
	}

	var lock model.DedupeOperationLock
	err := r.run(ctx, func(tx ReadWriter) error {
		now := r.now().UTC()
		key := keyspace.DedupeOperationLock(lockID)
		existing, ok, err := getJSON[model.DedupeOperationLock](tx, key)
		if err != nil {
			return err
		}
		if ok && existing.ExpiresAt.After(now) && existing.OwnerID != ownerID {
			return fmt.Errorf("%w: dedupe lock %q is held by %q until %s", meta.ErrCASConflict, lockID, existing.OwnerID, existing.ExpiresAt.Format(time.RFC3339Nano))
		}

		acquiredAt := now
		if ok && existing.OwnerID == ownerID && existing.ExpiresAt.After(now) {
			acquiredAt = existing.AcquiredAt.UTC()
		}
		lock = model.DedupeOperationLock{
			LockID:     lockID,
			OwnerID:    ownerID,
			AcquiredAt: acquiredAt,
			UpdatedAt:  now,
			ExpiresAt:  now.Add(req.TTL),
		}
		return setJSON(tx, key, lock)
	})
	return cloneDedupeOperationLock(lock), err
}

func (r *Repository) ReleaseDedupeOperationLock(ctx context.Context, req meta.ReleaseDedupeOperationLockRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	lockID := strings.TrimSpace(req.LockID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if lockID == "" || ownerID == "" {
		return fmt.Errorf("%w: dedupe lock id and owner id are required", meta.ErrInvalidArgument)
	}
	return r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.DedupeOperationLock(lockID)
		existing, ok, err := getJSON[model.DedupeOperationLock](tx, key)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if existing.OwnerID != ownerID {
			return fmt.Errorf("%w: dedupe lock %q is held by %q", meta.ErrCASConflict, lockID, existing.OwnerID)
		}
		return tx.Delete(key)
	})
}

func (r *Repository) PutSharedObjectRelease(ctx context.Context, req meta.PutSharedObjectReleaseRequest) (model.SharedObjectRelease, error) {
	if err := ctx.Err(); err != nil {
		return model.SharedObjectRelease{}, err
	}
	sharedObjectID := strings.TrimSpace(req.SharedObjectID)
	if sharedObjectID == "" {
		return model.SharedObjectRelease{}, fmt.Errorf("%w: shared object id is required", meta.ErrInvalidArgument)
	}
	segmentID := strings.TrimSpace(req.SegmentRef.SegmentID)
	if segmentID == "" {
		return model.SharedObjectRelease{}, fmt.Errorf("%w: segment id is required", meta.ErrInvalidArgument)
	}
	status := normalizeSharedObjectReleaseStatus(req.Status)
	var release model.SharedObjectRelease
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.SharedObjectRelease(sharedObjectID, segmentID)
		existing, ok, err := getJSON[model.SharedObjectRelease](tx, key)
		if err != nil {
			return err
		}
		now := r.now()
		if ok {
			release = existing
		}
		if release.ReleaseID == "" {
			release.ReleaseID = sharedObjectReleaseID(sharedObjectID, segmentID)
			release.SharedObjectID = sharedObjectID
			release.SegmentID = segmentID
			release.CreatedAt = now
		}
		release.SegmentRef = cloneSegmentRef(req.SegmentRef)
		release.SegmentRef.SharedObjectID = sharedObjectID
		release.Reason = req.Reason
		if release.Reason == "" {
			release.Reason = storage.DeleteReasonManualGC
		}
		release.Status = status
		release.UpdatedAt = now
		return setJSON(tx, key, release)
	})
	return cloneSharedObjectRelease(release), err
}

func (r *Repository) ListSharedObjectReleases(ctx context.Context, req meta.ListSharedObjectReleasesRequest) ([]model.SharedObjectRelease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	prefix := keyspace.SharedObjectReleasePrefix(req.SharedObjectID)
	var releases []model.SharedObjectRelease
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(prefix, cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				release, ok, err := getJSON[model.SharedObjectRelease](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.Status != "" && release.Status != req.Status {
					continue
				}
				releases = append(releases, cloneSharedObjectRelease(release))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(releases, func(i, j int) bool {
		return releases[i].ReleaseID < releases[j].ReleaseID
	})
	if len(releases) > limit {
		releases = releases[:limit]
	}
	return releases, err
}

func (r *Repository) PutVolumePool(ctx context.Context, req meta.PutVolumePoolRequest) (model.VolumePool, error) {
	if err := ctx.Err(); err != nil {
		return model.VolumePool{}, err
	}
	poolID := strings.TrimSpace(req.PoolID)
	if poolID == "" {
		return model.VolumePool{}, fmt.Errorf("%w: volume pool id is required", meta.ErrInvalidArgument)
	}
	var pool model.VolumePool
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.VolumePool(poolID)
		existing, _, err := getJSON[model.VolumePool](tx, key)
		if err != nil {
			return err
		}
		pool, err = meta.BuildVolumePool(existing, req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, key, pool)
	})
	return meta.CloneVolumePool(pool), err
}

func (r *Repository) GetVolumePool(ctx context.Context, poolID string) (model.VolumePool, error) {
	if err := ctx.Err(); err != nil {
		return model.VolumePool{}, err
	}
	poolID = strings.TrimSpace(poolID)
	if poolID == "" {
		return model.VolumePool{}, fmt.Errorf("%w: volume pool id is required", meta.ErrInvalidArgument)
	}
	var pool model.VolumePool
	err := r.run(ctx, func(tx ReadWriter) error {
		found, ok, err := getJSON[model.VolumePool](tx, keyspace.VolumePool(poolID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		pool = meta.CloneVolumePool(found)
		return nil
	})
	return meta.CloneVolumePool(pool), err
}

func (r *Repository) ListVolumePools(ctx context.Context, req meta.ListVolumePoolsRequest) ([]model.VolumePool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var pools []model.VolumePool
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.VolumePool(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				pool, ok, err := getJSON[model.VolumePool](tx, key)
				if err != nil {
					return err
				}
				if ok {
					pools = append(pools, meta.CloneVolumePool(pool))
				}
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].PoolID < pools[j].PoolID
	})
	if len(pools) > limit {
		pools = pools[:limit]
	}
	return pools, err
}

func (r *Repository) PutVolumeDrainOperation(ctx context.Context, req meta.PutVolumeDrainOperationRequest) (model.VolumeDrainOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.VolumeDrainOperationRecord{}, err
	}
	var record model.VolumeDrainOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.nextSequence(tx, sequenceVolumeDrain)
		if err != nil {
			return err
		}
		record, err = meta.BuildVolumeDrainOperation(fmt.Sprintf("drain-%020d", sequence), req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.VolumeDrainOperation(record.OperationID), record)
	})
	return meta.CloneVolumeDrainOperationRecord(record), err
}

func (r *Repository) ListVolumeDrainOperations(ctx context.Context, req meta.ListVolumeDrainOperationsRequest) ([]model.VolumeDrainOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.VolumeDrainOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.VolumeDrainOperation(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.VolumeDrainOperationRecord](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.SourceVolumeID != "" && record.SourceVolumeID != req.SourceVolumeID {
					continue
				}
				if req.TargetVolumeID != "" && record.TargetVolumeID != req.TargetVolumeID {
					continue
				}
				if req.Status != "" && record.Status != req.Status {
					continue
				}
				records = append(records, meta.CloneVolumeDrainOperationRecord(record))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(records, func(i, j int) bool {
		return records[i].OperationID > records[j].OperationID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, err
}

func (r *Repository) AcquireWorkerLease(ctx context.Context, req meta.AcquireWorkerLeaseRequest) (model.WorkerLease, error) {
	if err := ctx.Err(); err != nil {
		return model.WorkerLease{}, err
	}
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	leaseID := meta.WorkerLeaseID(workerKind, shardID)
	var lease model.WorkerLease
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.WorkerLease(workerKind, shardID)
		existing, _, err := getJSON[model.WorkerLease](tx, key)
		if err != nil {
			return err
		}
		lease, err = meta.BuildWorkerLease(existing, req, r.now())
		if err != nil {
			return err
		}
		if lease.LeaseID != leaseID {
			return fmt.Errorf("%w: worker lease id mismatch", meta.ErrInvalidArgument)
		}
		return setJSON(tx, key, lease)
	})
	return meta.CloneWorkerLease(lease), err
}

func (r *Repository) ReleaseWorkerLease(ctx context.Context, req meta.ReleaseWorkerLeaseRequest) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if workerKind == "" || shardID == "" || ownerID == "" {
		return fmt.Errorf("%w: worker kind, shard id, and owner id are required", meta.ErrInvalidArgument)
	}
	return r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.WorkerLease(workerKind, shardID)
		existing, ok, err := getJSON[model.WorkerLease](tx, key)
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if existing.OwnerID != ownerID {
			return fmt.Errorf("%w: worker lease %q is held by %q", meta.ErrCASConflict, existing.LeaseID, existing.OwnerID)
		}
		now := r.now()
		existing.UpdatedAt = now
		existing.ExpiresAt = now
		return setJSON(tx, key, existing)
	})
}

func (r *Repository) ListWorkerLeases(ctx context.Context, req meta.ListWorkerLeasesRequest) ([]model.WorkerLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	prefix := keyspace.WorkerLease(workerKind, "")
	var leases []model.WorkerLease
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(prefix, cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				lease, ok, err := getJSON[model.WorkerLease](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if workerKind != "" && lease.WorkerKind != workerKind {
					continue
				}
				if shardID != "" && lease.ShardID != shardID {
					continue
				}
				leases = append(leases, meta.CloneWorkerLease(lease))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(leases, func(i, j int) bool {
		return leases[i].LeaseID < leases[j].LeaseID
	})
	if len(leases) > limit {
		leases = leases[:limit]
	}
	return leases, err
}

func (r *Repository) PutWorkerOperation(ctx context.Context, req meta.PutWorkerOperationRequest) (model.WorkerOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.WorkerOperationRecord{}, err
	}
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	ownerID := strings.TrimSpace(req.OwnerID)
	if workerKind == "" || shardID == "" || ownerID == "" {
		return model.WorkerOperationRecord{}, fmt.Errorf("%w: worker kind, shard id, and owner id are required", meta.ErrInvalidArgument)
	}
	var record model.WorkerOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.nextSequence(tx, sequenceWorkerOperation)
		if err != nil {
			return err
		}
		record = model.WorkerOperationRecord{
			OperationID: fmt.Sprintf("worker-op-%020d", sequence),
			WorkerKind:  workerKind,
			ShardID:     shardID,
			OwnerID:     ownerID,
			LeaseID:     req.LeaseID,
			Status:      meta.NormalizeWorkerOperationStatus(req.Status, req.Retryable),
			Cursor:      strings.TrimSpace(req.Cursor),
			Scanned:     req.Scanned,
			Processed:   req.Processed,
			Skipped:     req.Skipped,
			Retryable:   req.Retryable,
			LastError:   req.LastError,
			StartedAt:   req.StartedAt.UTC(),
			FinishedAt:  req.FinishedAt.UTC(),
			CreatedAt:   r.now(),
		}
		return setJSON(tx, keyspace.WorkerOperation(record.OperationID), record)
	})
	return meta.CloneWorkerOperationRecord(record), err
}

func (r *Repository) ListWorkerOperations(ctx context.Context, req meta.ListWorkerOperationsRequest) ([]model.WorkerOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.WorkerOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.WorkerOperation(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.WorkerOperationRecord](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.WorkerKind != "" && record.WorkerKind != req.WorkerKind {
					continue
				}
				if req.ShardID != "" && record.ShardID != req.ShardID {
					continue
				}
				if req.Status != "" && record.Status != req.Status {
					continue
				}
				records = append(records, meta.CloneWorkerOperationRecord(record))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(records, func(i, j int) bool {
		return records[i].OperationID > records[j].OperationID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, err
}

func (r *Repository) PutMetadataMigrationOperation(ctx context.Context, req meta.PutMetadataMigrationOperationRequest) (model.MetadataMigrationOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.MetadataMigrationOperationRecord{}, err
	}
	var record model.MetadataMigrationOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		sequence, err := r.nextSequence(tx, sequenceMetadataMigration)
		if err != nil {
			return err
		}
		record, err = meta.BuildMetadataMigrationOperation(fmt.Sprintf("metadata-migration-%020d", sequence), req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.MetadataMigrationOperation(record.OperationID), record)
	})
	return meta.CloneMetadataMigrationOperationRecord(record), err
}

func (r *Repository) ListMetadataMigrationOperations(ctx context.Context, req meta.ListMetadataMigrationOperationsRequest) ([]model.MetadataMigrationOperationRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var records []model.MetadataMigrationOperationRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.MetadataMigrationOperation(""), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				record, ok, err := getJSON[model.MetadataMigrationOperationRecord](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.Status != "" && record.Status != req.Status {
					continue
				}
				records = append(records, meta.CloneMetadataMigrationOperationRecord(record))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(records, func(i, j int) bool {
		return records[i].OperationID > records[j].OperationID
	})
	if len(records) > limit {
		records = records[:limit]
	}
	return records, err
}

func (r *Repository) PutWorkerControl(ctx context.Context, req meta.PutWorkerControlRequest) (model.WorkerControlRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.WorkerControlRecord{}, err
	}
	var record model.WorkerControlRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.WorkerControl(req.WorkerKind, req.ShardID)
		existing, _, err := getJSON[model.WorkerControlRecord](tx, key)
		if err != nil {
			return err
		}
		record, err = meta.BuildWorkerControl(existing, req, r.now())
		if err != nil {
			return err
		}
		return setJSON(tx, keyspace.WorkerControl(record.WorkerKind, record.ShardID), record)
	})
	return meta.CloneWorkerControlRecord(record), err
}

func (r *Repository) GetWorkerControl(ctx context.Context, req meta.GetWorkerControlRequest) (model.WorkerControlRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.WorkerControlRecord{}, err
	}
	workerKind := strings.TrimSpace(req.WorkerKind)
	shardID := strings.TrimSpace(req.ShardID)
	if workerKind == "" || shardID == "" {
		return model.WorkerControlRecord{}, fmt.Errorf("%w: worker kind and shard id are required", meta.ErrInvalidArgument)
	}
	var record model.WorkerControlRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		value, ok, err := getJSON[model.WorkerControlRecord](tx, keyspace.WorkerControl(workerKind, shardID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		record = value
		return nil
	})
	return meta.CloneWorkerControlRecord(record), err
}

func (r *Repository) PublishSharedObject(ctx context.Context, req meta.PublishSharedObjectRequest) (model.SharedObject, error) {
	if err := ctx.Err(); err != nil {
		return model.SharedObject{}, err
	}
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return model.SharedObject{}, fmt.Errorf("%w: bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	var shared model.SharedObject
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
			return meta.ErrNotFound
		}
		now := r.now()
		next, err := sharedObjectFromVersion(bucket, version, now)
		if err != nil {
			return err
		}
		count, err := sharedObjectProtectedRootCount(tx, next, now)
		if err != nil {
			return err
		}
		next.ProtectedRootCount = count
		existing, ok, err := getJSON[model.SharedObject](tx, keyspace.SharedObject(next.SharedObjectID))
		if err != nil {
			return err
		}
		if ok {
			count, err := sharedObjectProtectedRootCount(tx, existing, now)
			if err != nil {
				return err
			}
			existing.ProtectedRootCount = count
			existing.UpdatedAt = now
			if err := setJSON(tx, keyspace.SharedObject(existing.SharedObjectID), existing); err != nil {
				return err
			}
			shared = existing
			ref := sharedObjectRefFromVersion(existing, version, now)
			if _, ok, err := getJSON[model.SharedObjectRef](tx, keyspace.SharedObjectRef(ref.SharedObjectID, ref.BucketID, ref.Key, ref.VersionID)); err != nil {
				return err
			} else if !ok {
				return setJSON(tx, keyspace.SharedObjectRef(ref.SharedObjectID, ref.BucketID, ref.Key, ref.VersionID), ref)
			}
			return nil
		}
		shared = next
		if err := setJSON(tx, keyspace.SharedObject(shared.SharedObjectID), shared); err != nil {
			return err
		}
		ref := sharedObjectRefFromVersion(shared, version, shared.CreatedAt)
		return setJSON(tx, keyspace.SharedObjectRef(ref.SharedObjectID, ref.BucketID, ref.Key, ref.VersionID), ref)
	})
	return cloneSharedObject(shared), err
}

func (r *Repository) GetSharedObject(ctx context.Context, sharedObjectID string) (model.SharedObject, error) {
	if err := ctx.Err(); err != nil {
		return model.SharedObject{}, err
	}
	var shared model.SharedObject
	err := r.run(ctx, func(tx ReadWriter) error {
		got, ok, err := getJSON[model.SharedObject](tx, keyspace.SharedObject(sharedObjectID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		shared = got
		return nil
	})
	return cloneSharedObject(shared), err
}

func (r *Repository) ListSharedObjects(ctx context.Context, req meta.ListSharedObjectsRequest) ([]model.SharedObject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var objects []model.SharedObject
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		for {
			keys, next, err := tx.List(keyspace.SharedObjectPrefix(), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				shared, ok, err := getJSON[model.SharedObject](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.TenantID != "" && shared.TenantID != req.TenantID {
					continue
				}
				if req.BucketID != "" && shared.BucketID != req.BucketID {
					continue
				}
				if req.Key != "" && shared.Key != req.Key {
					continue
				}
				objects = append(objects, cloneSharedObject(shared))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].SharedObjectID < objects[j].SharedObjectID
	})
	if len(objects) > limit {
		objects = objects[:limit]
	}
	return objects, err
}

func (r *Repository) AttachObjectVersionToSharedObject(ctx context.Context, req meta.AttachObjectVersionToSharedObjectRequest) (model.AttachSharedObjectResult, error) {
	if err := ctx.Err(); err != nil {
		return model.AttachSharedObjectResult{}, err
	}
	if req.SharedObjectID == "" || req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return model.AttachSharedObjectResult{}, fmt.Errorf("%w: shared object id, bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	var result model.AttachSharedObjectResult
	err := r.run(ctx, func(tx ReadWriter) error {
		shared, ok, err := getJSON[model.SharedObject](tx, keyspace.SharedObject(req.SharedObjectID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if shared.BucketID != req.BucketID || shared.Key != req.Key {
			return fmt.Errorf("%w: shared object scope mismatch", meta.ErrInvalidArgument)
		}
		_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
			return meta.ErrNotFound
		}
		now := r.now()
		if objectVersionProtectedByObjectLock(version, now, false) {
			return meta.ErrObjectLocked
		}
		if !objectVersionMatchesSharedObject(version, shared) {
			return fmt.Errorf("%w: object version does not match shared object digest/size", meta.ErrInvalidArgument)
		}
		previousRefs := objectSegmentRefsFromVersion(version)
		sharedRefs := cloneSegmentRefs(shared.SegmentRefs)
		version.SegmentRefs = sharedRefs
		version.SegmentRef = firstSegmentRef(sharedRefs)
		version.StorageClass = cloneStorageClass(shared.StorageClass)
		ref := sharedObjectRefFromVersion(shared, version, now)
		refKey := keyspace.SharedObjectRef(ref.SharedObjectID, ref.BucketID, ref.Key, ref.VersionID)
		if _, ok, err := getJSON[model.SharedObjectRef](tx, refKey); err != nil {
			return err
		} else if !ok {
			shared.RefCount++
		}
		count, err := sharedObjectProtectedRootCount(tx, shared, now)
		if err != nil {
			return err
		}
		shared.ProtectedRootCount = count
		shared.UpdatedAt = now
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		if err := setJSON(tx, keyspace.SharedObject(shared.SharedObjectID), shared); err != nil {
			return err
		}
		if err := setJSON(tx, refKey, ref); err != nil {
			return err
		}
		head, ok, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if ok && head.VersionID == version.VersionID {
			head.SegmentRefs = cloneSegmentRefs(version.SegmentRefs)
			head.SegmentRef = cloneSegmentRef(version.SegmentRef)
			head.StorageClass = cloneStorageClass(version.StorageClass)
			head.LastModified = now
			head, err = setObjectHead(tx, head)
			if err != nil {
				return err
			}
			if err := setListObject(tx, head); err != nil {
				return err
			}
		}
		result = model.AttachSharedObjectResult{
			Version:             cloneVersion(version),
			SharedObject:        cloneSharedObject(shared),
			Ref:                 cloneSharedObjectRef(ref),
			PreviousSegmentRefs: cloneSegmentRefs(previousRefs),
		}
		return nil
	})
	return result, err
}

func (r *Repository) PublishObjectVersionRefs(ctx context.Context, req meta.PublishObjectVersionRefsRequest) (meta.PublishObjectVersionRefsResult, error) {
	if err := ctx.Err(); err != nil {
		return meta.PublishObjectVersionRefsResult{}, err
	}
	if req.BucketID == "" || req.Key == "" || req.VersionID == "" {
		return meta.PublishObjectVersionRefsResult{}, fmt.Errorf("%w: bucket id, key, and version id are required", meta.ErrInvalidArgument)
	}
	refs := cloneSegmentRefs(req.SegmentRefs)
	if len(refs) == 0 {
		return meta.PublishObjectVersionRefsResult{}, fmt.Errorf("%w: replacement segment refs are required", meta.ErrInvalidArgument)
	}
	var result meta.PublishObjectVersionRefsResult
	err := r.run(ctx, func(tx ReadWriter) error {
		_, version, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return err
		}
		if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
			return meta.ErrNotFound
		}
		if objectVersionProtectedByObjectLock(version, r.now(), false) {
			return meta.ErrObjectLocked
		}
		previousRefs := objectSegmentRefsFromVersion(version)
		if !meta.SegmentRefsContainVolume(previousRefs, req.ExpectedSourceVolumeID) {
			return fmt.Errorf("%w: object version has no refs on source volume %q", meta.ErrInvalidArgument, req.ExpectedSourceVolumeID)
		}
		version.SegmentRefs = refs
		version.SegmentRef = firstSegmentRef(refs)
		if err := setObjectVersion(tx, version); err != nil {
			return err
		}
		head, ok, err := getObjectHead(tx, req.BucketID, req.Key)
		if err != nil {
			return err
		}
		if ok && head.VersionID == version.VersionID {
			head.SegmentRefs = cloneSegmentRefs(version.SegmentRefs)
			head.SegmentRef = cloneSegmentRef(version.SegmentRef)
			head, err = setObjectHead(tx, head)
			if err != nil {
				return err
			}
			if err := setListObject(tx, head); err != nil {
				return err
			}
		}
		result = meta.PublishObjectVersionRefsResult{
			Version:             cloneVersion(version),
			PreviousSegmentRefs: cloneSegmentRefs(previousRefs),
		}
		return nil
	})
	return result, err
}

func (r *Repository) ListSharedObjectRefs(ctx context.Context, req meta.ListSharedObjectRefsRequest) ([]model.SharedObjectRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	var refs []model.SharedObjectRef
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		prefix := keyspace.SharedObjectRefPrefix(req.SharedObjectID)
		for {
			keys, next, err := tx.List(prefix, cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				ref, ok, err := getJSON[model.SharedObjectRef](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.BucketID != "" && ref.BucketID != req.BucketID {
					continue
				}
				if req.Key != "" && ref.Key != req.Key {
					continue
				}
				if req.VersionID != "" && ref.VersionID != req.VersionID {
					continue
				}
				refs = append(refs, cloneSharedObjectRef(ref))
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
	})
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].RefID < refs[j].RefID
	})
	if len(refs) > limit {
		refs = refs[:limit]
	}
	return refs, err
}

func (r *Repository) RepairSharedObjectRefCounts(ctx context.Context, req meta.RepairSharedObjectRefCountsRequest) (model.SharedObjectRepairResult, error) {
	if err := ctx.Err(); err != nil {
		return model.SharedObjectRepairResult{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	result := model.SharedObjectRepairResult{}
	err := r.run(ctx, func(tx ReadWriter) error {
		cursor := ""
		now := r.now()
		for result.Scanned < limit {
			keys, next, err := tx.List(keyspace.SharedObjectPrefix(), cursor, 128)
			if err != nil {
				return err
			}
			for _, key := range keys {
				if result.Scanned >= limit {
					return nil
				}
				shared, ok, err := getJSON[model.SharedObject](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					continue
				}
				if req.SharedObjectID != "" && shared.SharedObjectID != req.SharedObjectID {
					continue
				}
				result.Scanned++
				refCount, err := sharedObjectRefCount(tx, shared.SharedObjectID)
				if err != nil {
					return err
				}
				protectedRootCount, err := sharedObjectProtectedRootCount(tx, shared, now)
				if err != nil {
					return err
				}
				if shared.RefCount == refCount && shared.ProtectedRootCount == protectedRootCount {
					continue
				}
				shared.RefCount = refCount
				shared.ProtectedRootCount = protectedRootCount
				shared.UpdatedAt = now
				if err := setJSON(tx, keyspace.SharedObject(shared.SharedObjectID), shared); err != nil {
					return err
				}
				result.Updated++
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
		return nil
	})
	return result, err
}

func (r *Repository) RepairListIndexes(ctx context.Context, req meta.RepairListIndexesRequest) (model.ListIndexRepairResult, error) {
	if err := ctx.Err(); err != nil {
		return model.ListIndexRepairResult{}, err
	}
	if req.BucketID == "" {
		return model.ListIndexRepairResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 1000
	}
	result := model.ListIndexRepairResult{}
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		handledListKeys := make(map[string]struct{})
		handledUploadIndexKeys := make(map[string]struct{})
		if err := repairObjectListIndexFromHeads(tx, req.BucketID, limit, req.Apply, &result, handledListKeys); err != nil {
			return err
		}
		if err := repairStaleObjectListIndexes(tx, req.BucketID, limit, req.Apply, &result, handledListKeys); err != nil {
			return err
		}
		if err := repairMultipartUploadIndexesFromState(tx, req.BucketID, limit, req.Apply, &result, handledUploadIndexKeys); err != nil {
			return err
		}
		return repairStaleMultipartUploadIndexes(tx, req.BucketID, limit, req.Apply, &result, handledUploadIndexKeys)
	})
	return result, err
}

func (r *Repository) CreateMultipartUpload(ctx context.Context, req meta.CreateMultipartUploadRequest) (model.MultipartUpload, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartUpload{}, err
	}
	if req.BucketID == "" || req.Key == "" {
		return model.MultipartUpload{}, fmt.Errorf("%w: bucket id and object key are required", meta.ErrInvalidArgument)
	}
	if err := validateObjectLockState(req.ObjectLockRetention, req.ObjectLockLegalHold); err != nil {
		return model.MultipartUpload{}, err
	}
	var upload model.MultipartUpload
	err := r.run(ctx, func(tx ReadWriter) error {
		bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID))
		if err != nil {
			return err
		}
		if !ok {
			return meta.ErrNotFound
		}
		if err := enforceTenantActiveUploadQuota(tx, bucket.TenantID); err != nil {
			return err
		}
		uploadID, err := r.nextMultipartUploadID(tx, req.BucketID)
		if err != nil {
			return err
		}
		now := r.now()
		upload = model.MultipartUpload{
			UploadID:             uploadID,
			BucketID:             req.BucketID,
			Key:                  req.Key,
			ContentType:          req.ContentType,
			StorageClass:         cloneStorageClass(req.StorageClass),
			ServerSideEncryption: req.ServerSideEncryption,
			UserMetadata:         cloneStringMap(req.UserMetadata),
			Tags:                 cloneStringMap(req.Tags),
			ObjectLockRetention:  req.ObjectLockRetention,
			ObjectLockLegalHold:  req.ObjectLockLegalHold,
			State:                model.MultipartUploadActive,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
		if err := setMultipartUpload(tx, upload); err != nil {
			return err
		}
		return applyTenantActiveUploadDelta(tx, bucket.TenantID, 1, now)
	})
	return cloneUpload(upload), err
}

func (r *Repository) GetMultipartUpload(ctx context.Context, req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartUpload{}, err
	}
	var upload model.MultipartUpload
	err := r.run(ctx, func(tx ReadWriter) error {
		got, err := getUpload(tx, req)
		if err != nil {
			return err
		}
		upload = cloneUpload(got)
		return nil
	})
	return upload, err
}

func (r *Repository) ListMultipartUploads(ctx context.Context, req meta.ListMultipartUploadsRequest) (model.ListMultipartUploadsResult, error) {
	if err := ctx.Err(); err != nil {
		return model.ListMultipartUploadsResult{}, err
	}
	if req.BucketID == "" {
		return model.ListMultipartUploadsResult{}, fmt.Errorf("%w: bucket id is required", meta.ErrInvalidArgument)
	}
	maxUploads := req.MaxUploads
	if maxUploads <= 0 {
		maxUploads = 1000
	}
	entries := make([]multipartUploadEntry, 0)
	commonPrefixes := make(map[string]struct{})
	if err := r.run(ctx, func(tx ReadWriter) error {
		if _, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(req.BucketID)); err != nil {
			return err
		} else if !ok {
			return meta.ErrNotFound
		}
		start := multipartUploadListPrefix(req.BucketID, req.Prefix)
		end := prefixRangeEndString(start)
		cursor := ""
		if req.KeyMarker != "" {
			if req.Delimiter != "" && req.UploadIDMarker == "" && strings.HasSuffix(req.KeyMarker, req.Delimiter) {
				start = maxString(start, prefixRangeEndString(multipartUploadListPrefix(req.BucketID, req.KeyMarker)))
			} else if req.UploadIDMarker != "" {
				cursor = keyspace.MultipartUploadByKey(req.BucketID, req.KeyMarker, req.UploadIDMarker)
				start = maxString(start, keyspace.MultipartUploadByKey(req.BucketID, req.KeyMarker, ""))
			} else {
				start = maxString(start, prefixRangeEndString(keyspace.MultipartUploadByKey(req.BucketID, req.KeyMarker, "")))
			}
		}
		for len(entries) <= maxUploads {
			keys, next, err := tx.ListRange(start, end, cursor, maxUploads+1)
			if err != nil {
				return err
			}
			if len(keys) == 0 {
				return nil
			}
			cursor = ""
			skipTo := ""
			for _, key := range keys {
				record, ok, err := getJSON[multipartUploadIndexRecord](tx, key)
				if err != nil {
					return err
				}
				if !ok {
					cursor = key
					continue
				}
				upload, ok, err := getJSON[model.MultipartUpload](tx, keyspace.MultipartUpload(req.BucketID, record.UploadID))
				if err != nil {
					return err
				}
				if !ok || upload.State != model.MultipartUploadActive || !strings.HasPrefix(upload.Key, req.Prefix) {
					cursor = key
					continue
				}
				if req.Delimiter != "" {
					rest := strings.TrimPrefix(upload.Key, req.Prefix)
					if index := strings.Index(rest, req.Delimiter); index >= 0 {
						commonPrefix := req.Prefix + rest[:index+len(req.Delimiter)]
						if _, ok := commonPrefixes[commonPrefix]; !ok {
							commonPrefixes[commonPrefix] = struct{}{}
							entries = append(entries, multipartUploadEntry{name: commonPrefix, prefix: true})
						}
						skipTo = prefixRangeEndString(multipartUploadListPrefix(req.BucketID, commonPrefix))
						break
					}
				}
				entries = append(entries, multipartUploadEntry{name: upload.Key, uploadID: upload.UploadID, upload: upload})
				cursor = key
				if len(entries) > maxUploads {
					return nil
				}
			}
			if skipTo != "" {
				start = skipTo
				cursor = ""
				continue
			}
			if next == "" {
				return nil
			}
			cursor = next
		}
		return nil
	}); err != nil {
		return model.ListMultipartUploadsResult{}, err
	}
	result := model.ListMultipartUploadsResult{}
	if len(entries) > maxUploads {
		result.IsTruncated = true
		result.NextKeyMarker = entries[maxUploads-1].name
		result.NextUploadIDMarker = entries[maxUploads-1].uploadID
		entries = entries[:maxUploads]
	}
	for _, entry := range entries {
		if entry.prefix {
			result.CommonPrefixes = append(result.CommonPrefixes, entry.name)
			continue
		}
		result.Uploads = append(result.Uploads, cloneUpload(entry.upload))
	}
	return result, nil
}

func (r *Repository) PutMultipartPart(ctx context.Context, req meta.PutMultipartPartRequest) (model.MultipartPart, *model.MultipartPart, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartPart{}, nil, err
	}
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" || req.PartNumber < 1 || req.PartNumber > meta.MaxMultipartParts {
		return model.MultipartPart{}, nil, fmt.Errorf("%w: multipart part fields are invalid", meta.ErrInvalidArgument)
	}
	var part model.MultipartPart
	var previous *model.MultipartPart
	err := r.run(ctx, func(tx ReadWriter) error {
		uploadReq := meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID}
		upload, err := getActiveUpload(tx, uploadReq)
		if err != nil {
			return err
		}
		key := keyspace.MultipartPart(req.BucketID, req.UploadID, req.PartNumber)
		if old, ok, err := getJSON[model.MultipartPart](tx, key); err != nil {
			return err
		} else if ok {
			oldCopy := clonePart(old)
			previous = &oldCopy
		}
		now := r.now()
		part = model.MultipartPart{
			UploadID:   req.UploadID,
			PartNumber: req.PartNumber,
			SizeBytes:  req.SizeBytes,
			ETag:       req.ETag,
			SegmentRef: cloneSegmentRef(req.SegmentRef),
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		if previous != nil {
			part.CreatedAt = previous.CreatedAt
		}
		if err := setJSON(tx, key, part); err != nil {
			return err
		}
		meta.ApplyMultipartPartSummary(&upload, part, previous, now)
		return setMultipartUpload(tx, upload)
	})
	return clonePart(part), previous, err
}

func (r *Repository) ListMultipartParts(ctx context.Context, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var parts []model.MultipartPart
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, err := getActiveUpload(tx, req); err != nil {
			return err
		}
		var err error
		parts, err = listParts(tx, req)
		return err
	})
	return parts, err
}

func (r *Repository) GetMultipartParts(ctx context.Context, req meta.GetMultipartPartsRequest) ([]model.MultipartPart, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" {
		return nil, fmt.Errorf("%w: multipart upload fields are required", meta.ErrInvalidArgument)
	}
	if err := meta.ValidateMultipartPartNumberSelection(req.PartNumbers); err != nil {
		return nil, err
	}
	parts := make([]model.MultipartPart, 0, len(req.PartNumbers))
	err := r.run(ctx, func(tx ReadWriter) error {
		if _, err := getActiveUpload(tx, meta.MultipartUploadRequest{
			BucketID: req.BucketID,
			Key:      req.Key,
			UploadID: req.UploadID,
		}); err != nil {
			return err
		}
		for _, partNumber := range req.PartNumbers {
			part, ok, err := getJSON[model.MultipartPart](tx, keyspace.MultipartPart(req.BucketID, req.UploadID, partNumber))
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			parts = append(parts, clonePart(part))
		}
		return nil
	})
	return parts, err
}

func (r *Repository) GetMultipartCompletion(ctx context.Context, req meta.MultipartUploadRequest) (model.MultipartCompletionRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartCompletionRecord{}, err
	}
	var record model.MultipartCompletionRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		got, ok, err := getJSON[model.MultipartCompletionRecord](tx, keyspace.MultipartCompletion(req.BucketID, req.UploadID))
		if err != nil {
			return err
		}
		if !ok || got.Key != req.Key {
			return meta.ErrNotFound
		}
		record = cloneMultipartCompletionRecord(got)
		return nil
	})
	return record, err
}

func (r *Repository) PrepareMultipartCompletion(ctx context.Context, req meta.PrepareMultipartCompletionRequest) (model.MultipartCompletionRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartCompletionRecord{}, err
	}
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" || req.ObjectVersionID == "" || req.ETag == "" || req.PartCount < 1 {
		return model.MultipartCompletionRecord{}, fmt.Errorf("%w: multipart completion fields are required", meta.ErrInvalidArgument)
	}
	var record model.MultipartCompletionRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.MultipartCompletion(req.BucketID, req.UploadID)
		existing, ok, err := getJSON[model.MultipartCompletionRecord](tx, key)
		if err != nil {
			return err
		}
		if ok {
			if !multipartCompletionMatches(existing, req) {
				return meta.ErrCASConflict
			}
			record = cloneMultipartCompletionRecord(existing)
			return nil
		}
		upload, err := getUpload(tx, meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID})
		if err != nil {
			return err
		}
		if upload.State != model.MultipartUploadActive {
			return meta.ErrNotFound
		}
		now := r.now()
		record = model.MultipartCompletionRecord{
			BucketID:              req.BucketID,
			Key:                   req.Key,
			UploadID:              req.UploadID,
			ObjectVersionID:       req.ObjectVersionID,
			ExpectedHeadVersionID: req.ExpectedHeadVersionID,
			ETag:                  req.ETag,
			SizeBytes:             req.SizeBytes,
			PartCount:             req.PartCount,
			State:                 model.MultipartCompletionPrepared,
			CreatedAt:             now,
			UpdatedAt:             now,
		}
		return setJSON(tx, key, record)
	})
	return cloneMultipartCompletionRecord(record), err
}

func (r *Repository) MarkMultipartCompletionPublished(ctx context.Context, req meta.MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error) {
	return r.markMultipartCompletionState(ctx, req, model.MultipartCompletionPublished)
}

func (r *Repository) MarkMultipartCompletionCompleted(ctx context.Context, req meta.MultipartCompletionStateRequest) (model.MultipartCompletionRecord, error) {
	return r.markMultipartCompletionState(ctx, req, model.MultipartCompletionCompleted)
}

func (r *Repository) markMultipartCompletionState(ctx context.Context, req meta.MultipartCompletionStateRequest, state model.MultipartCompletionState) (model.MultipartCompletionRecord, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartCompletionRecord{}, err
	}
	var record model.MultipartCompletionRecord
	err := r.run(ctx, func(tx ReadWriter) error {
		key := keyspace.MultipartCompletion(req.BucketID, req.UploadID)
		got, ok, err := getJSON[model.MultipartCompletionRecord](tx, key)
		if err != nil {
			return err
		}
		if !ok || got.Key != req.Key {
			return meta.ErrNotFound
		}
		if !multipartCompletionStateAtLeast(got.State, state) {
			got.State = state
			got.UpdatedAt = r.now()
			if err := setJSON(tx, key, got); err != nil {
				return err
			}
		}
		record = cloneMultipartCompletionRecord(got)
		return nil
	})
	return record, err
}

func (r *Repository) CompleteMultipartUpload(ctx context.Context, req meta.CompleteMultipartUploadRequest) (model.MultipartUpload, error) {
	if err := ctx.Err(); err != nil {
		return model.MultipartUpload{}, err
	}
	if req.ObjectVersionID == "" || req.ETag == "" {
		return model.MultipartUpload{}, fmt.Errorf("%w: completed object version and etag are required", meta.ErrInvalidArgument)
	}
	uploadReq := meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID}
	var upload model.MultipartUpload
	err := r.run(ctx, func(tx ReadWriter) error {
		got, err := getUpload(tx, uploadReq)
		if err != nil {
			return err
		}
		switch got.State {
		case model.MultipartUploadCompleted:
			upload = cloneUpload(got)
			return nil
		case model.MultipartUploadAborted:
			return meta.ErrNotFound
		case model.MultipartUploadActive:
			if err := meta.ValidateCompletedMultipartPartCount(req.PartCount); err != nil {
				return err
			}
		default:
			return meta.ErrNotFound
		}
		now := r.now()
		got.State = model.MultipartUploadCompleted
		got.CompletedVersionID = req.ObjectVersionID
		got.CompletedETag = req.ETag
		got.CompletedSizeBytes = req.SizeBytes
		got.CompletedPartCount = req.PartCount
		got.CompletedAt = now
		got.PartsCleanupState = model.MultipartPartsCleanupPending
		got.PartsCleanupDeleted = 0
		got.PartsCleanupUpdatedAt = now
		got.UpdatedAt = now
		if err := setMultipartUpload(tx, got); err != nil {
			return err
		}
		if err := applyTenantActiveUploadDeltaForBucket(tx, got.BucketID, -1, now); err != nil {
			return err
		}
		upload = cloneUpload(got)
		return nil
	})
	return upload, err
}

func (r *Repository) AbortMultipartUpload(ctx context.Context, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var parts []model.MultipartPart
	err := r.run(ctx, func(tx ReadWriter) error {
		upload, err := getUpload(tx, req)
		if err != nil {
			return err
		}
		switch upload.State {
		case model.MultipartUploadAborted:
			parts, err = listParts(tx, req)
			return err
		case model.MultipartUploadCompleted:
			return meta.ErrNotFound
		case model.MultipartUploadActive:
		default:
			return meta.ErrNotFound
		}
		parts, err = listParts(tx, req)
		if err != nil {
			return err
		}
		upload.State = model.MultipartUploadAborted
		now := r.now()
		upload.PartsCleanupState = model.MultipartPartsCleanupPending
		upload.PartsCleanupDeleted = 0
		upload.PartsCleanupUpdatedAt = now
		upload.UpdatedAt = now
		if err := setMultipartUpload(tx, upload); err != nil {
			return err
		}
		return applyTenantActiveUploadDeltaForBucket(tx, upload.BucketID, -1, now)
	})
	return parts, err
}

func (r *Repository) CleanupMultipartUploadParts(ctx context.Context, req meta.CleanupMultipartUploadPartsRequest) (meta.CleanupMultipartUploadPartsResult, error) {
	if err := ctx.Err(); err != nil {
		return meta.CleanupMultipartUploadPartsResult{}, err
	}
	uploadReq := meta.MultipartUploadRequest{BucketID: req.BucketID, Key: req.Key, UploadID: req.UploadID}
	limit := meta.NormalizeMultipartCleanupLimit(req.Limit)
	var result meta.CleanupMultipartUploadPartsResult
	err := r.run(ctx, func(tx ReadWriter) error {
		upload, err := getUpload(tx, uploadReq)
		if err != nil {
			return err
		}
		switch upload.State {
		case model.MultipartUploadCompleted, model.MultipartUploadAborted:
		case model.MultipartUploadActive:
			return fmt.Errorf("%w: active multipart upload parts cannot be cleaned", meta.ErrInvalidArgument)
		default:
			return meta.ErrNotFound
		}
		keys, hasMore, err := partKeysForCleanup(tx, uploadReq, limit)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := tx.Delete(key); err != nil {
				return err
			}
		}
		now := r.now()
		upload.PartsCleanupDeleted += len(keys)
		upload.PartsCleanupUpdatedAt = now
		upload.UpdatedAt = now
		if hasMore {
			upload.PartsCleanupState = model.MultipartPartsCleanupPending
		} else {
			upload.PartsCleanupState = model.MultipartPartsCleanupDone
		}
		if err := setMultipartUpload(tx, upload); err != nil {
			return err
		}
		result = meta.CleanupMultipartUploadPartsResult{
			Upload:       cloneUpload(upload),
			DeletedParts: len(keys),
			HasMore:      hasMore,
		}
		return nil
	})
	return result, err
}

func (r *Repository) nextObjectVersionID(tx ReadWriter, bucketID, key string) (string, error) {
	for attempt := 0; attempt < idCollisionRetryLimit; attempt++ {
		versionID, err := r.ids.NewID(metaid.KindVersion)
		if err != nil {
			return "", fmt.Errorf("%w: generate object version id: %v", meta.ErrUnavailable, err)
		}
		if _, ok, err := getJSON[model.ObjectVersion](tx, keyspace.ObjectVersion(bucketID, key, versionID)); err != nil {
			return "", err
		} else if ok {
			continue
		}
		return versionID, nil
	}
	return "", fmt.Errorf("%w: object version id collision retry budget exhausted", meta.ErrUnavailable)
}

func (r *Repository) nextMultipartUploadID(tx ReadWriter, bucketID string) (string, error) {
	for attempt := 0; attempt < idCollisionRetryLimit; attempt++ {
		uploadID, err := r.ids.NewID(metaid.KindUpload)
		if err != nil {
			return "", fmt.Errorf("%w: generate multipart upload id: %v", meta.ErrUnavailable, err)
		}
		if _, ok, err := getJSON[model.MultipartUpload](tx, keyspace.MultipartUpload(bucketID, uploadID)); err != nil {
			return "", err
		} else if ok {
			continue
		}
		return uploadID, nil
	}
	return "", fmt.Errorf("%w: multipart upload id collision retry budget exhausted", meta.ErrUnavailable)
}

func (r *Repository) nextSequence(tx ReadWriter, name string) (int, error) {
	return r.reserveSequences(tx, name, 1)
}

func (r *Repository) reserveSequences(tx ReadWriter, name string, count int) (int, error) {
	if count <= 0 {
		return 0, fmt.Errorf("%w: sequence reservation count must be positive", meta.ErrInvalidArgument)
	}
	key := sequenceKey(name)
	record, _, err := getJSON[sequenceRecord](tx, key)
	if err != nil {
		return 0, err
	}
	first := record.Value + 1
	record.Value += count
	if err := setJSON(tx, key, record); err != nil {
		return 0, err
	}
	return first, nil
}

func updateSequenceAtLeast(tx ReadWriter, name string, value int) error {
	if value <= 0 {
		return nil
	}
	key := sequenceKey(name)
	record, _, err := getJSON[sequenceRecord](tx, key)
	if err != nil {
		return err
	}
	if record.Value >= value {
		return nil
	}
	record.Value = value
	return setJSON(tx, key, record)
}

func operationalMetadataEmpty(tx ReadWriter) (bool, error) {
	prefixes := []string{
		keyspace.KMSKey(""),
		keyspace.AuditEvent(""),
		keyspace.MetadataMigrationOperation(""),
		keyspace.GCOperation(""),
		keyspace.DedupeOperation(""),
		keyspace.SharedObjectPrefix(),
		keyspace.SharedObjectReleasePrefix(""),
		keyspace.VolumePool(""),
		keyspace.VolumeDrainOperation(""),
		keyspace.WorkerLease("", ""),
		keyspace.WorkerOperation(""),
	}
	for _, prefix := range prefixes {
		keys, _, err := tx.List(prefix, "", 1)
		if err != nil {
			return false, err
		}
		if len(keys) > 0 {
			return false, nil
		}
	}
	return true, nil
}

func validateOperationalMetadataImport(tx ReadWriter, req meta.ImportOperationalMetadataRequest) error {
	if req.MetadataSchema != nil {
		if err := meta.ValidateMetadataSchemaRecord(*req.MetadataSchema); err != nil {
			return err
		}
	}
	seenMigrations := make(map[string]struct{}, len(req.MetadataMigrationOperations))
	for _, record := range req.MetadataMigrationOperations {
		operationID := strings.TrimSpace(record.OperationID)
		if operationID == "" {
			return fmt.Errorf("%w: metadata migration operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenMigrations[operationID]; ok {
			return fmt.Errorf("%w: duplicate metadata migration operation %q", meta.ErrAlreadyExists, operationID)
		}
		seenMigrations[operationID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.MetadataMigrationOperation(operationID), "metadata migration operation", operationID); err != nil {
			return err
		}
	}

	seenKMS := make(map[string]struct{}, len(req.KMSKeys))
	for _, record := range req.KMSKeys {
		keyID := strings.TrimSpace(record.KeyID)
		if keyID == "" {
			return fmt.Errorf("%w: kms key id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenKMS[keyID]; ok {
			return fmt.Errorf("%w: duplicate kms key %q", meta.ErrAlreadyExists, keyID)
		}
		seenKMS[keyID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.KMSKey(keyID), "kms key", keyID); err != nil {
			return err
		}
	}

	seenAudit := make(map[string]struct{}, len(req.AuditEvents))
	for _, event := range req.AuditEvents {
		if strings.TrimSpace(event.EventID) == "" {
			return fmt.Errorf("%w: audit event id is required", meta.ErrInvalidArgument)
		}
		if strings.TrimSpace(event.EventHash) == "" {
			return fmt.Errorf("%w: audit event hash is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenAudit[event.EventID]; ok {
			return fmt.Errorf("%w: duplicate audit event %q", meta.ErrAlreadyExists, event.EventID)
		}
		seenAudit[event.EventID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.AuditEvent(event.EventID), "audit event", event.EventID); err != nil {
			return err
		}
	}

	seenGC := make(map[string]struct{}, len(req.GCOperations))
	for _, record := range req.GCOperations {
		if strings.TrimSpace(record.OperationID) == "" {
			return fmt.Errorf("%w: gc operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenGC[record.OperationID]; ok {
			return fmt.Errorf("%w: duplicate gc operation %q", meta.ErrAlreadyExists, record.OperationID)
		}
		seenGC[record.OperationID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.GCOperation(record.OperationID), "gc operation", record.OperationID); err != nil {
			return err
		}
	}

	seenDedupe := make(map[string]struct{}, len(req.DedupeOperations))
	for _, record := range req.DedupeOperations {
		if strings.TrimSpace(record.OperationID) == "" {
			return fmt.Errorf("%w: dedupe operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenDedupe[record.OperationID]; ok {
			return fmt.Errorf("%w: duplicate dedupe operation %q", meta.ErrAlreadyExists, record.OperationID)
		}
		seenDedupe[record.OperationID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.DedupeOperation(record.OperationID), "dedupe operation", record.OperationID); err != nil {
			return err
		}
	}

	seenShared := make(map[string]struct{}, len(req.SharedObjects))
	for _, shared := range req.SharedObjects {
		if strings.TrimSpace(shared.SharedObjectID) == "" {
			return fmt.Errorf("%w: shared object id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenShared[shared.SharedObjectID]; ok {
			return fmt.Errorf("%w: duplicate shared object %q", meta.ErrAlreadyExists, shared.SharedObjectID)
		}
		seenShared[shared.SharedObjectID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.SharedObject(shared.SharedObjectID), "shared object", shared.SharedObjectID); err != nil {
			return err
		}
	}

	seenRelease := make(map[string]struct{}, len(req.SharedObjectReleases))
	for _, release := range req.SharedObjectReleases {
		if strings.TrimSpace(release.ReleaseID) == "" {
			return fmt.Errorf("%w: shared object release id is required", meta.ErrInvalidArgument)
		}
		if strings.TrimSpace(release.SharedObjectID) == "" {
			return fmt.Errorf("%w: shared object release shared object id is required", meta.ErrInvalidArgument)
		}
		if strings.TrimSpace(release.SegmentID) == "" {
			return fmt.Errorf("%w: shared object release segment id is required", meta.ErrInvalidArgument)
		}
		key := keyspace.SharedObjectRelease(release.SharedObjectID, release.SegmentID)
		if _, ok := seenRelease[key]; ok {
			return fmt.Errorf("%w: duplicate shared object release %q", meta.ErrAlreadyExists, release.ReleaseID)
		}
		seenRelease[key] = struct{}{}
		if err := ensureImportKeyAbsent(tx, key, "shared object release", release.ReleaseID); err != nil {
			return err
		}
	}
	seenPools := make(map[string]struct{}, len(req.VolumePools))
	for _, pool := range req.VolumePools {
		poolID := strings.TrimSpace(pool.PoolID)
		if poolID == "" {
			return fmt.Errorf("%w: volume pool id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenPools[poolID]; ok {
			return fmt.Errorf("%w: duplicate volume pool %q", meta.ErrAlreadyExists, poolID)
		}
		seenPools[poolID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.VolumePool(poolID), "volume pool", poolID); err != nil {
			return err
		}
	}

	seenDrainOps := make(map[string]struct{}, len(req.VolumeDrainOperations))
	for _, record := range req.VolumeDrainOperations {
		operationID := strings.TrimSpace(record.OperationID)
		if operationID == "" {
			return fmt.Errorf("%w: volume drain operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenDrainOps[operationID]; ok {
			return fmt.Errorf("%w: duplicate volume drain operation %q", meta.ErrAlreadyExists, operationID)
		}
		seenDrainOps[operationID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.VolumeDrainOperation(operationID), "volume drain operation", operationID); err != nil {
			return err
		}
	}

	seenLeases := make(map[string]struct{}, len(req.WorkerLeases))
	for _, lease := range req.WorkerLeases {
		leaseID := strings.TrimSpace(lease.LeaseID)
		workerKind := strings.TrimSpace(lease.WorkerKind)
		shardID := strings.TrimSpace(lease.ShardID)
		if leaseID == "" || workerKind == "" || shardID == "" {
			return fmt.Errorf("%w: worker lease id, kind, and shard id are required", meta.ErrInvalidArgument)
		}
		if want := meta.WorkerLeaseID(workerKind, shardID); leaseID != want {
			return fmt.Errorf("%w: worker lease id %q does not match worker kind/shard %q", meta.ErrInvalidArgument, leaseID, want)
		}
		if _, ok := seenLeases[leaseID]; ok {
			return fmt.Errorf("%w: duplicate worker lease %q", meta.ErrAlreadyExists, leaseID)
		}
		seenLeases[leaseID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.WorkerLease(workerKind, shardID), "worker lease", leaseID); err != nil {
			return err
		}
	}

	seenWorkerOps := make(map[string]struct{}, len(req.WorkerOperations))
	for _, record := range req.WorkerOperations {
		operationID := strings.TrimSpace(record.OperationID)
		if operationID == "" {
			return fmt.Errorf("%w: worker operation id is required", meta.ErrInvalidArgument)
		}
		if _, ok := seenWorkerOps[operationID]; ok {
			return fmt.Errorf("%w: duplicate worker operation %q", meta.ErrAlreadyExists, operationID)
		}
		seenWorkerOps[operationID] = struct{}{}
		if err := ensureImportKeyAbsent(tx, keyspace.WorkerOperation(operationID), "worker operation", operationID); err != nil {
			return err
		}
	}
	return nil
}

func ensureImportKeyAbsent(tx ReadWriter, key, kind, id string) error {
	_, ok, err := tx.Get(key)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("%w: %s %q", meta.ErrAlreadyExists, kind, id)
	}
	return nil
}

func (r *Repository) appendAuditEvent(tx ReadWriter, event model.AuditEvent) error {
	sequence, err := r.nextSequence(tx, sequenceAudit)
	if err != nil {
		return err
	}
	head, _, err := getJSON[auditHeadRecord](tx, keyspace.AuditHead())
	if err != nil {
		return err
	}
	event.EventID = fmt.Sprintf("audit-%020d", sequence)
	event.CreatedAt = r.now()
	event.PreviousHash = head.LastHash
	event.EventHash = auditEventHash(event)
	if err := setJSON(tx, keyspace.AuditEvent(event.EventID), event); err != nil {
		return err
	}
	head.LastHash = event.EventHash
	return setJSON(tx, keyspace.AuditHead(), head)
}

func (r *Repository) syncProtectedRefsForVersion(tx ReadWriter, version model.ObjectVersion, now time.Time) error {
	if err := deleteProtectedRefsForVersion(tx, version.BucketID, version.Key, version.VersionID); err != nil {
		return err
	}
	if !objectVersionProtectedByObjectLock(version, now, false) {
		return nil
	}
	refs := objectSegmentRefsFromVersion(version)
	for _, segmentRef := range refs {
		if segmentRef.SegmentID == "" {
			continue
		}
		ref := protectedRefFromVersion(version, segmentRef, now)
		if err := setJSON(tx, keyspace.ProtectedRefByVersion(ref.BucketID, ref.Key, ref.VersionID, ref.RefID), ref); err != nil {
			return err
		}
		if err := setJSON(tx, keyspace.ProtectedRefBySegment(ref.SegmentID, ref.RefID), ref); err != nil {
			return err
		}
	}
	return nil
}

func deleteProtectedRefsForVersion(tx ReadWriter, bucketID, key, versionID string) error {
	cursor := ""
	for {
		keys, next, err := tx.List(protectedRefVersionPrefix(bucketID, key, versionID), cursor, 1000)
		if err != nil {
			return err
		}
		for _, versionKey := range keys {
			ref, ok, err := getJSON[model.ProtectedRef](tx, versionKey)
			if err != nil {
				return err
			}
			if ok {
				if err := tx.Delete(keyspace.ProtectedRefBySegment(ref.SegmentID, ref.RefID)); err != nil {
					return err
				}
			}
			if err := tx.Delete(versionKey); err != nil {
				return err
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

func (r *Repository) createDeleteMarker(tx ReadWriter, req meta.DeleteObjectRequest) (model.DeleteResult, error) {
	versionID, err := r.nextObjectVersionID(tx, req.BucketID, req.Key)
	if err != nil {
		return model.DeleteResult{}, err
	}
	now := r.now()
	version := model.ObjectVersion{
		BucketID:       req.BucketID,
		Key:            req.Key,
		VersionID:      versionID,
		VersionSortKey: versionID,
		State:          model.ObjectVersionCommitted,
		CreatedAt:      now,
		CommittedAt:    now,
		DeleteMarker:   true,
	}
	head := headFromVersion(version, now)
	if err := setObjectVersion(tx, version); err != nil {
		return model.DeleteResult{}, err
	}
	head, err = setObjectHead(tx, head)
	if err != nil {
		return model.DeleteResult{}, err
	}
	if err := tx.Delete(keyspace.ListObject(req.BucketID, req.Key)); err != nil {
		return model.DeleteResult{}, err
	}
	return model.DeleteResult{
		Deleted:          true,
		DeletedVersionID: versionID,
		DeleteMarker:     true,
		DeletedVersion:   cloneVersion(version),
	}, nil
}

func (r *Repository) deleteObjectVersion(tx ReadWriter, req meta.DeleteObjectRequest, now time.Time) (model.DeleteResult, error) {
	versionKey, deleted, ok, err := findVersionByID(tx, req.BucketID, req.Key, req.VersionID)
	if err != nil {
		return model.DeleteResult{}, err
	}
	if !ok {
		return model.DeleteResult{}, meta.ErrNotFound
	}
	if err := checkDeleteKMSAdmission(tx, deleted.ServerSideEncryption); err != nil {
		return model.DeleteResult{}, err
	}
	if objectVersionProtectedByObjectLock(deleted, now, false) {
		if objectVersionProtectedByObjectLock(deleted, now, true) || !req.BypassGovernanceRetention || !validGovernanceBypassAudit(req.BypassAudit) {
			return model.DeleteResult{}, meta.ErrObjectLocked
		}
		if err := r.appendAuditEvent(tx, governanceBypassAuditEvent(req, model.AuditActionGovernanceBypassDeleteObject, deleted.VersionID, map[string]string{
			"target": "object_version",
		})); err != nil {
			return model.DeleteResult{}, err
		}
	}
	if err := deleteObjectVersionRecord(tx, versionKey, deleted); err != nil {
		return model.DeleteResult{}, err
	}
	if err := deleteProtectedRefsForVersion(tx, deleted.BucketID, deleted.Key, deleted.VersionID); err != nil {
		return model.DeleteResult{}, err
	}
	head, headOK, err := getObjectHead(tx, req.BucketID, req.Key)
	if err != nil {
		return model.DeleteResult{}, err
	}
	if headOK && head.VersionID == req.VersionID {
		promoted, promotedOK, err := latestCommittedVersion(tx, req.BucketID, req.Key, req.VersionID)
		if err != nil {
			return model.DeleteResult{}, err
		}
		if promotedOK {
			promotedHead := headFromVersion(promoted, promoted.CommittedAt)
			promotedHead, err = setObjectHead(tx, promotedHead)
			if err != nil {
				return model.DeleteResult{}, err
			}
			if promoted.DeleteMarker {
				if err := tx.Delete(keyspace.ListObject(req.BucketID, req.Key)); err != nil {
					return model.DeleteResult{}, err
				}
			} else if err := setListObject(tx, promotedHead); err != nil {
				return model.DeleteResult{}, err
			}
		} else {
			if err := tx.Delete(keyspace.ObjectHead(req.BucketID, req.Key)); err != nil {
				return model.DeleteResult{}, err
			}
			if err := tx.Delete(keyspace.ListObject(req.BucketID, req.Key)); err != nil {
				return model.DeleteResult{}, err
			}
		}
	}
	return model.DeleteResult{
		Deleted:          true,
		DeletedVersionID: deleted.VersionID,
		DeleteMarker:     deleted.DeleteMarker,
		DeletedVersion:   cloneVersion(deleted),
	}, nil
}

func checkDeleteKMSAdmission(tx ReadWriter, encryption model.ServerSideEncryption) error {
	if encryption.Algorithm != model.ServerSideEncryptionAWSKMS {
		return nil
	}
	keyID := strings.TrimSpace(encryption.KeyID)
	if keyID == "" {
		return nil
	}
	key, ok, err := getJSON[model.KMSKeyRecord](tx, keyspace.KMSKey(keyID))
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if !model.KMSKeyAllowsDelete(key.State) {
		return fmt.Errorf("%w: kms key %q state %q does not allow delete", meta.ErrKMSKeyUnavailable, key.KeyID, key.State)
	}
	return nil
}

func latestCommittedVersion(tx ReadWriter, bucketID, objectKey, skipVersionID string) (model.ObjectVersion, bool, error) {
	var latest model.ObjectVersion
	if err := scanJSON[model.ObjectVersion](tx, versionPrefix(bucketID, objectKey), func(version model.ObjectVersion) error {
		if version.State != model.ObjectVersionCommitted || version.VersionID == skipVersionID {
			return nil
		}
		if latest.VersionID == "" || version.VersionSortKey > latest.VersionSortKey {
			latest = version
		}
		return nil
	}); err != nil {
		return model.ObjectVersion{}, false, err
	}
	if latest.VersionID == "" {
		return model.ObjectVersion{}, false, nil
	}
	latest, err := hydrateObjectVersionManifest(tx, latest)
	if err != nil {
		return model.ObjectVersion{}, false, err
	}
	return latest, true, nil
}

func getBucketByName(tx ReadWriter, name string) (model.Bucket, error) {
	bucketID, ok, err := getJSON[string](tx, keyspace.BucketByName(name))
	if err != nil {
		return model.Bucket{}, err
	}
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	bucket, ok, err := getJSON[model.Bucket](tx, keyspace.BucketByID(bucketID))
	if err != nil {
		return model.Bucket{}, err
	}
	if !ok {
		return model.Bucket{}, meta.ErrNotFound
	}
	return bucket, nil
}

func getUpload(tx ReadWriter, req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	if req.BucketID == "" || req.Key == "" || req.UploadID == "" {
		return model.MultipartUpload{}, fmt.Errorf("%w: multipart upload fields are required", meta.ErrInvalidArgument)
	}
	upload, ok, err := getJSON[model.MultipartUpload](tx, keyspace.MultipartUpload(req.BucketID, req.UploadID))
	if err != nil {
		return model.MultipartUpload{}, err
	}
	if !ok || upload.Key != req.Key {
		return model.MultipartUpload{}, meta.ErrNotFound
	}
	return upload, nil
}

func getActiveUpload(tx ReadWriter, req meta.MultipartUploadRequest) (model.MultipartUpload, error) {
	upload, err := getUpload(tx, req)
	if err != nil {
		return model.MultipartUpload{}, err
	}
	if upload.State != model.MultipartUploadActive {
		return model.MultipartUpload{}, meta.ErrNotFound
	}
	return upload, nil
}

func listParts(tx ReadWriter, req meta.MultipartUploadRequest) ([]model.MultipartPart, error) {
	parts := make([]model.MultipartPart, 0)
	if err := scanJSON[model.MultipartPart](tx, partPrefix(req.BucketID, req.UploadID), func(part model.MultipartPart) error {
		parts = append(parts, clonePart(part))
		return nil
	}); err != nil {
		return nil, err
	}
	sort.Slice(parts, func(i, j int) bool {
		return parts[i].PartNumber < parts[j].PartNumber
	})
	return parts, nil
}

func partKeysForCleanup(tx ReadWriter, req meta.MultipartUploadRequest, limit int) ([]string, bool, error) {
	keys, _, err := tx.List(partPrefix(req.BucketID, req.UploadID), "", limit+1)
	if err != nil {
		return nil, false, err
	}
	if len(keys) > limit {
		return keys[:limit], true, nil
	}
	return keys, false, nil
}

func objectLockTarget(tx ReadWriter, bucketID, objectKey, versionID string) (string, model.ObjectVersion, error) {
	if versionID == "" {
		head, ok, err := getObjectHead(tx, bucketID, objectKey)
		if err != nil {
			return "", model.ObjectVersion{}, err
		}
		if !ok || head.DeleteMarker {
			return "", model.ObjectVersion{}, meta.ErrNotFound
		}
		versionID = head.VersionID
	}
	vkey, version, ok, err := findVersionByID(tx, bucketID, objectKey, versionID)
	if err != nil {
		return "", model.ObjectVersion{}, err
	}
	if !ok || version.State != model.ObjectVersionCommitted || version.DeleteMarker {
		return "", model.ObjectVersion{}, meta.ErrNotFound
	}
	return vkey, version, nil
}

func updateHeadObjectLock(tx ReadWriter, version model.ObjectVersion) error {
	head, ok, err := getObjectHead(tx, version.BucketID, version.Key)
	if err != nil {
		return err
	}
	if !ok || head.VersionID != version.VersionID {
		return nil
	}
	head.ObjectLockRetention = version.ObjectLockRetention
	head.ObjectLockLegalHold = version.ObjectLockLegalHold
	head, err = setObjectHead(tx, head)
	if err != nil {
		return err
	}
	if head.DeleteMarker {
		return tx.Delete(keyspace.ListObject(version.BucketID, version.Key))
	}
	return setListObject(tx, head)
}

func setObjectVersion(tx ReadWriter, version model.ObjectVersion) error {
	plan, err := meta.PlanObjectVersionManifestStorage(version)
	if err != nil {
		return err
	}
	if version.Manifest.Encoding == model.ObjectManifestEncodingChunked || len(plan.Chunks) > 0 {
		if err := deleteObjectManifestChunks(tx, version.BucketID, version.VersionID); err != nil {
			return err
		}
	}
	for _, chunk := range plan.Chunks {
		if err := setJSON(tx, keyspace.ObjectManifestChunk(chunk.BucketID, chunk.VersionID, chunk.ChunkNumber), chunk); err != nil {
			return err
		}
	}
	stored := plan.StoredVersion
	if err := tx.Set(keyspace.ObjectVersion(stored.BucketID, stored.Key, stored.VersionSortKey), plan.StoredValue); err != nil {
		return err
	}
	locator := objectVersionIndexRecord{
		Key:            stored.Key,
		VersionSortKey: stored.VersionSortKey,
	}
	return setJSON(tx, keyspace.ObjectVersionByID(stored.BucketID, stored.VersionID), locator)
}

func deleteObjectVersionRecord(tx ReadWriter, versionKey string, version model.ObjectVersion) error {
	if version.BucketID != "" && version.VersionID != "" {
		if err := tx.Delete(keyspace.ObjectVersionByID(version.BucketID, version.VersionID)); err != nil {
			return err
		}
		if err := deleteObjectManifestChunks(tx, version.BucketID, version.VersionID); err != nil {
			return err
		}
	}
	if versionKey == "" {
		return nil
	}
	return tx.Delete(versionKey)
}

func deleteObjectManifestChunks(tx ReadWriter, bucketID, versionID string) error {
	if bucketID == "" || versionID == "" {
		return nil
	}
	prefix := keyspace.ObjectManifestChunk(bucketID, versionID, 0)
	cursor := ""
	for {
		keys, next, err := tx.List(prefix, cursor, 128)
		if err != nil {
			return err
		}
		for _, key := range keys {
			if err := tx.Delete(key); err != nil {
				return err
			}
			cursor = key
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

func setMultipartUpload(tx ReadWriter, upload model.MultipartUpload) error {
	if err := setJSON(tx, keyspace.MultipartUpload(upload.BucketID, upload.UploadID), upload); err != nil {
		return err
	}
	indexKey := keyspace.MultipartUploadByKey(upload.BucketID, upload.Key, upload.UploadID)
	if upload.State != model.MultipartUploadActive {
		return tx.Delete(indexKey)
	}
	return setJSON(tx, indexKey, multipartUploadIndexRecord{UploadID: upload.UploadID})
}

func findVersionByID(tx ReadWriter, bucketID, objectKey, versionID string) (string, model.ObjectVersion, bool, error) {
	if versionID == "" {
		return "", model.ObjectVersion{}, false, nil
	}
	locator, ok, err := getJSON[objectVersionIndexRecord](tx, keyspace.ObjectVersionByID(bucketID, versionID))
	if err != nil {
		return "", model.ObjectVersion{}, false, err
	}
	if ok {
		if locator.Key != objectKey || locator.VersionSortKey == "" {
			return "", model.ObjectVersion{}, false, nil
		}
		versionKey := keyspace.ObjectVersion(bucketID, locator.Key, locator.VersionSortKey)
		version, found, err := getJSON[model.ObjectVersion](tx, versionKey)
		if err != nil {
			return "", model.ObjectVersion{}, false, err
		}
		if !found || version.VersionID != versionID || version.Key != objectKey {
			return "", model.ObjectVersion{}, false, nil
		}
		version, err = hydrateObjectVersionManifest(tx, version)
		if err != nil {
			return "", model.ObjectVersion{}, false, err
		}
		return versionKey, version, true, nil
	}
	var foundKey string
	var found model.ObjectVersion
	if err := scanJSONWithKey[model.ObjectVersion](tx, versionPrefix(bucketID, objectKey), func(key string, version model.ObjectVersion) error {
		if version.VersionID != versionID {
			return nil
		}
		foundKey = key
		found = version
		return errStopScan
	}); err != nil {
		return "", model.ObjectVersion{}, false, err
	}
	if foundKey == "" {
		return "", model.ObjectVersion{}, false, nil
	}
	if found.VersionID != "" && found.VersionSortKey != "" {
		locator := objectVersionIndexRecord{
			Key:            found.Key,
			VersionSortKey: found.VersionSortKey,
		}
		if err := setJSON(tx, keyspace.ObjectVersionByID(found.BucketID, found.VersionID), locator); err != nil {
			return "", model.ObjectVersion{}, false, err
		}
	}
	found, err = hydrateObjectVersionManifest(tx, found)
	if err != nil {
		return "", model.ObjectVersion{}, false, err
	}
	return foundKey, found, true, nil
}

func validateComplianceProfileAttachmentRequest(req meta.PutComplianceProfileAttachmentRequest) error {
	if strings.TrimSpace(req.ProfileID) == "" {
		return fmt.Errorf("%w: compliance profile id is required", meta.ErrInvalidArgument)
	}
	if strings.TrimSpace(req.Regulation) == "" || strings.TrimSpace(req.RecordClass) == "" {
		return fmt.Errorf("%w: compliance profile regulation and record class are required", meta.ErrInvalidArgument)
	}
	if req.RetentionMode != model.ObjectLockModeGovernance && req.RetentionMode != model.ObjectLockModeCompliance {
		return fmt.Errorf("%w: compliance profile retention mode is invalid", meta.ErrInvalidArgument)
	}
	if req.RetentionDays < 0 || req.RetentionYears < 0 {
		return fmt.Errorf("%w: compliance profile retention days and years cannot be negative", meta.ErrInvalidArgument)
	}
	if req.RetentionDays <= 0 && req.RetentionYears <= 0 {
		return fmt.Errorf("%w: compliance profile retention duration is required", meta.ErrInvalidArgument)
	}
	if req.RetentionDays > 0 && req.RetentionYears > 0 {
		return fmt.Errorf("%w: compliance profile retention days and years are mutually exclusive", meta.ErrInvalidArgument)
	}
	return nil
}

func getJSON[T any](reader ReadWriter, key string) (T, bool, error) {
	var out T
	value, ok, err := reader.Get(key)
	if err != nil {
		return out, false, err
	}
	if !ok {
		return out, false, nil
	}
	valueCopy := append([]byte(nil), value...)
	if err := json.Unmarshal(valueCopy, &out); err != nil {
		return out, false, err
	}
	return out, true, nil
}

func setJSON(tx ReadWriter, key string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return tx.Set(key, encoded)
}

func scanJSON[T any](reader ReadWriter, prefix string, fn func(T) error) error {
	return scanJSONWithKey(reader, prefix, func(_ string, value T) error {
		return fn(value)
	})
}

func scanJSONWithKey[T any](reader ReadWriter, prefix string, fn func(string, T) error) error {
	return scanRaw(reader, prefix, func(key string, value []byte) error {
		var decoded T
		if err := json.Unmarshal(value, &decoded); err != nil {
			return err
		}
		return fn(key, decoded)
	})
}

var errStopScan = errors.New("stop scan")

func scanRaw(reader ReadWriter, prefix string, fn func(string, []byte) error) error {
	cursor := ""
	for {
		keys, next, err := reader.List(prefix, cursor, 128)
		if err != nil {
			return err
		}
		for _, key := range keys {
			value, ok, err := reader.Get(key)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			if err := fn(key, append([]byte(nil), value...)); err != nil {
				if errors.Is(err, errStopScan) {
					return nil
				}
				return err
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
}

func repairObjectListIndexFromHeads(tx ReadWriter, bucketID string, limit int, apply bool, result *model.ListIndexRepairResult, handled map[string]struct{}) error {
	start := objectHeadPrefix(bucketID)
	end := prefixRangeEndString(start)
	cursor := ""
	for result.ScannedObjectHeads < limit {
		keys, next, err := tx.ListRange(start, end, cursor, repairBatchSize(limit, result.ScannedObjectHeads))
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, key := range keys {
			headEntry, ok, err := getJSON[model.ObjectHeadEntry](tx, key)
			if err != nil {
				return err
			}
			cursor = key
			if !ok {
				continue
			}
			head := headFromHeadEntry(headEntry)
			if head.BucketID != bucketID || head.Key == "" {
				continue
			}
			result.ScannedObjectHeads++
			listKey := keyspace.ListObject(bucketID, head.Key)
			listEntry, found, err := getJSON[model.ObjectListEntry](tx, listKey)
			if err != nil {
				return err
			}
			if head.DeleteMarker {
				if found {
					result.StaleObjectListEntries++
					handled[listKey] = struct{}{}
					if apply {
						if err := tx.Delete(listKey); err != nil {
							return err
						}
						result.RemovedObjectListEntries++
					}
				}
				continue
			}
			if !found {
				result.MissingObjectListEntries++
				handled[listKey] = struct{}{}
				if apply {
					if err := setJSON(tx, listKey, listEntryFromHead(head)); err != nil {
						return err
					}
					result.RepairedObjectListEntries++
				}
				continue
			}
			if !objectListEntryMatchesHead(listEntry, head) {
				result.StaleObjectListEntries++
				handled[listKey] = struct{}{}
				if apply {
					if err := setJSON(tx, listKey, listEntryFromHead(head)); err != nil {
						return err
					}
					result.RepairedObjectListEntries++
				}
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
	return nil
}

func repairStaleObjectListIndexes(tx ReadWriter, bucketID string, limit int, apply bool, result *model.ListIndexRepairResult, handled map[string]struct{}) error {
	start := listPrefix(bucketID)
	end := prefixRangeEndString(start)
	cursor := ""
	for result.ScannedObjectListEntries < limit {
		keys, next, err := tx.ListRange(start, end, cursor, repairBatchSize(limit, result.ScannedObjectListEntries))
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, key := range keys {
			listEntry, ok, err := getJSON[model.ObjectListEntry](tx, key)
			if err != nil {
				return err
			}
			cursor = key
			if !ok {
				continue
			}
			result.ScannedObjectListEntries++
			if _, alreadyHandled := handled[key]; alreadyHandled {
				continue
			}
			head, found, err := getObjectHead(tx, bucketID, listEntry.Key)
			if err != nil {
				return err
			}
			if !found || head.DeleteMarker {
				result.StaleObjectListEntries++
				handled[key] = struct{}{}
				if apply {
					if err := tx.Delete(key); err != nil {
						return err
					}
					result.RemovedObjectListEntries++
				}
				continue
			}
			if !objectListEntryMatchesHead(listEntry, head) {
				result.StaleObjectListEntries++
				handled[key] = struct{}{}
				if apply {
					if err := setJSON(tx, key, listEntryFromHead(head)); err != nil {
						return err
					}
					result.RepairedObjectListEntries++
				}
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
	return nil
}

func repairMultipartUploadIndexesFromState(tx ReadWriter, bucketID string, limit int, apply bool, result *model.ListIndexRepairResult, handled map[string]struct{}) error {
	start := multipartUploadStatePrefix(bucketID)
	end := prefixRangeEndString(start)
	cursor := ""
	scannedKeys := 0
	for scannedKeys < limit {
		keys, next, err := tx.ListRange(start, end, cursor, repairBatchSize(limit, scannedKeys))
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, key := range keys {
			scannedKeys++
			cursor = key
			if !strings.HasSuffix(key, "/state") {
				continue
			}
			upload, ok, err := getJSON[model.MultipartUpload](tx, key)
			if err != nil {
				return err
			}
			if !ok || upload.BucketID != bucketID || upload.UploadID == "" {
				continue
			}
			result.ScannedMultipartUploads++
			indexKey := keyspace.MultipartUploadByKey(bucketID, upload.Key, upload.UploadID)
			record, found, err := getJSON[multipartUploadIndexRecord](tx, indexKey)
			if err != nil {
				return err
			}
			if upload.State != model.MultipartUploadActive {
				if found {
					result.StaleMultipartUploadIndexes++
					handled[indexKey] = struct{}{}
					if apply {
						if err := tx.Delete(indexKey); err != nil {
							return err
						}
						result.RemovedMultipartUploadIndexes++
					}
				}
				continue
			}
			if !found {
				result.MissingMultipartUploadIndexes++
				handled[indexKey] = struct{}{}
				if apply {
					if err := setJSON(tx, indexKey, multipartUploadIndexRecord{UploadID: upload.UploadID}); err != nil {
						return err
					}
					result.RepairedMultipartUploadIndexes++
				}
				continue
			}
			if record.UploadID != upload.UploadID {
				result.StaleMultipartUploadIndexes++
				handled[indexKey] = struct{}{}
				if apply {
					if err := setJSON(tx, indexKey, multipartUploadIndexRecord{UploadID: upload.UploadID}); err != nil {
						return err
					}
					result.RepairedMultipartUploadIndexes++
				}
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
	return nil
}

func repairStaleMultipartUploadIndexes(tx ReadWriter, bucketID string, limit int, apply bool, result *model.ListIndexRepairResult, handled map[string]struct{}) error {
	start := multipartUploadListPrefix(bucketID, "")
	end := prefixRangeEndString(start)
	cursor := ""
	for result.ScannedMultipartUploadIndexes < limit {
		keys, next, err := tx.ListRange(start, end, cursor, repairBatchSize(limit, result.ScannedMultipartUploadIndexes))
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		for _, key := range keys {
			record, ok, err := getJSON[multipartUploadIndexRecord](tx, key)
			if err != nil {
				return err
			}
			cursor = key
			if !ok {
				continue
			}
			result.ScannedMultipartUploadIndexes++
			if _, alreadyHandled := handled[key]; alreadyHandled {
				continue
			}
			stale := record.UploadID == ""
			if !stale {
				upload, found, err := getJSON[model.MultipartUpload](tx, keyspace.MultipartUpload(bucketID, record.UploadID))
				if err != nil {
					return err
				}
				stale = !found ||
					upload.BucketID != bucketID ||
					upload.State != model.MultipartUploadActive ||
					keyspace.MultipartUploadByKey(bucketID, upload.Key, upload.UploadID) != key
			}
			if stale {
				result.StaleMultipartUploadIndexes++
				handled[key] = struct{}{}
				if apply {
					if err := tx.Delete(key); err != nil {
						return err
					}
					result.RemovedMultipartUploadIndexes++
				}
			}
		}
		if next == "" {
			return nil
		}
		cursor = next
	}
	return nil
}

func repairBatchSize(limit, scanned int) int {
	remaining := limit - scanned
	if remaining <= 0 {
		return 1
	}
	if remaining < 128 {
		return remaining
	}
	return 128
}

func objectListEntryMatchesHead(entry model.ObjectListEntry, head model.ObjectHead) bool {
	return entry.BucketID == head.BucketID &&
		entry.Key == head.Key &&
		entry.VersionID == head.VersionID &&
		entry.Revision == head.Revision &&
		entry.SizeBytes == head.SizeBytes &&
		entry.ETag == head.ETag &&
		entry.ContentType == head.ContentType &&
		reflect.DeepEqual(entry.StorageClass, head.StorageClass) &&
		entry.LastModified.Equal(head.LastModified) &&
		entry.DeleteMarker == head.DeleteMarker
}

func sequenceKey(name string) string {
	return "/namros/v1/sequences/" + keyspace.EscapePathSegment(name)
}

func bucketByIDPrefix() string {
	return keyspace.BucketByID("")
}

func objectHeadPrefix(bucketID string) string {
	return strings.TrimSuffix(keyspace.ObjectHead(bucketID, ""), "/head")
}

func listPrefix(bucketID string) string {
	return keyspace.ListObject(bucketID, "")
}

func versionPrefix(bucketID, objectKey string) string {
	return keyspace.ObjectVersion(bucketID, objectKey, "")
}

func versionsBucketPrefix(bucketID string) string {
	escapedBucketID := keyspace.EscapePathSegment(bucketID)
	return "/namros/v1/buckets/" + escapedBucketID + "/versions/"
}

func versionListPrefix(bucketID, objectKeyPrefix string) string {
	return versionsBucketPrefix(bucketID) + keyspace.EscapeObjectKey(objectKeyPrefix)
}

func multipartUploadListPrefix(bucketID, objectKeyPrefix string) string {
	escapedBucketID := keyspace.EscapePathSegment(bucketID)
	return "/namros/v1/buckets/" + escapedBucketID + "/multipart-by-key/" + keyspace.EscapeObjectKey(objectKeyPrefix)
}

func multipartUploadStatePrefix(bucketID string) string {
	escapedBucketID := keyspace.EscapePathSegment(bucketID)
	return "/namros/v1/buckets/" + escapedBucketID + "/multipart/"
}

func partPrefix(bucketID, uploadID string) string {
	return strings.TrimSuffix(keyspace.MultipartPart(bucketID, uploadID, 0), "00000")
}

func prefixRangeEndString(prefix string) string {
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

func maxString(left, right string) string {
	if right > left {
		return right
	}
	return left
}

type listEntry struct {
	name   string
	prefix bool
	head   model.ObjectHead
}

type versionListEntry struct {
	name      string
	versionID string
	sortKey   string
	prefix    bool
	isLatest  bool
	version   model.ObjectVersion
}

func firstVersionFromKeys(reader ReadWriter, keys []string) (model.ObjectVersion, bool, error) {
	for _, key := range keys {
		version, ok, err := getJSON[model.ObjectVersion](reader, key)
		if err != nil {
			return model.ObjectVersion{}, false, err
		}
		if ok {
			return version, true, nil
		}
	}
	return model.ObjectVersion{}, false, nil
}

func collectVersionEntriesForKey(reader ReadWriter, bucketID, objectKey string) ([]versionListEntry, error) {
	prefix := versionPrefix(bucketID, objectKey)
	end := prefixRangeEndString(prefix)
	cursor := ""
	versions := make([]model.ObjectVersion, 0)
	for {
		keys, next, err := reader.ListRange(prefix, end, cursor, 128)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			version, ok, err := getJSON[model.ObjectVersion](reader, key)
			if err != nil {
				return nil, err
			}
			if !ok || version.State != model.ObjectVersionCommitted {
				cursor = key
				continue
			}
			version, err = hydrateObjectVersionManifest(reader, version)
			if err != nil {
				return nil, err
			}
			versions = append(versions, version)
			cursor = key
		}
		if next == "" {
			break
		}
		cursor = next
	}
	head, hasHead, err := getObjectHead(reader, bucketID, objectKey)
	if err != nil {
		return nil, err
	}
	entries := make([]versionListEntry, 0, len(versions))
	for _, version := range versions {
		entries = append(entries, versionListEntry{
			name:      version.Key,
			versionID: version.VersionID,
			sortKey:   version.VersionSortKey,
			isLatest:  hasHead && head.VersionID == version.VersionID,
			version:   version,
		})
	}
	sortVersionEntries(entries)
	return entries, nil
}

func sortVersionEntries(entries []versionListEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name == entries[j].name {
			if entries[i].prefix != entries[j].prefix {
				return entries[i].prefix
			}
			return entries[i].sortKey > entries[j].sortKey
		}
		return entries[i].name < entries[j].name
	})
}

func versionEntriesAfter(entries []versionListEntry, keyMarker, versionIDMarker string) []versionListEntry {
	index := sort.Search(len(entries), func(i int) bool {
		if entries[i].name != keyMarker {
			return entries[i].name > keyMarker
		}
		if versionIDMarker == "" {
			return entries[i].name > keyMarker
		}
		return entries[i].versionID == versionIDMarker
	})
	if index < len(entries) && entries[index].name == keyMarker && entries[index].versionID == versionIDMarker {
		index++
	}
	return entries[index:]
}

type multipartUploadEntry struct {
	name     string
	uploadID string
	prefix   bool
	upload   model.MultipartUpload
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneAuditEvent(in model.AuditEvent) model.AuditEvent {
	out := in
	out.Principal.Groups = cloneStringSlice(in.Principal.Groups)
	out.Principal.Roles = cloneStringSlice(in.Principal.Roles)
	out.Details = cloneStringMap(in.Details)
	return out
}

func cloneAccessKey(in model.AccessKey) model.AccessKey {
	out := in
	out.Permissions = cloneStringSlice(in.Permissions)
	return out
}

func cloneBucket(in model.Bucket) model.Bucket {
	out := in
	out.CORSRules = cloneCORSRules(in.CORSRules)
	out.Lifecycle = cloneLifecycleConfiguration(in.Lifecycle)
	out.Policy = clonePolicy(in.Policy)
	return out
}

func clonePolicy(in auth.PolicyDocument) auth.PolicyDocument {
	out := in
	if len(in.Statements) > 0 {
		out.Statements = make([]auth.PolicyStatement, 0, len(in.Statements))
		for _, statement := range in.Statements {
			out.Statements = append(out.Statements, clonePolicyStatement(statement))
		}
	}
	return out
}

func clonePolicyStatement(in auth.PolicyStatement) auth.PolicyStatement {
	out := in
	out.Principals = cloneStringSlice(in.Principals)
	out.Actions = cloneStringSlice(in.Actions)
	out.Resources = cloneStringSlice(in.Resources)
	return out
}

func cloneProtectedRef(in model.ProtectedRef) model.ProtectedRef {
	out := in
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	return out
}

func cloneGCOperationRecord(in model.GCOperationRecord) model.GCOperationRecord {
	out := in
	out.Attempts = cloneGCOperationAttempts(in.Attempts)
	return out
}

func cloneGCOperationAttempts(in []model.GCOperationAttempt) []model.GCOperationAttempt {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.GCOperationAttempt, len(in))
	copy(out, in)
	return out
}

func cloneDedupeOperationRecord(in model.DedupeOperationRecord) model.DedupeOperationRecord {
	out := in
	out.Attempts = cloneDedupeOperationAttempts(in.Attempts)
	return out
}

func cloneDedupeOperationAttempts(in []model.DedupeOperationAttempt) []model.DedupeOperationAttempt {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.DedupeOperationAttempt, len(in))
	copy(out, in)
	return out
}

func cloneDedupeOperationLock(in model.DedupeOperationLock) model.DedupeOperationLock {
	return in
}

func cloneSharedObjectRelease(in model.SharedObjectRelease) model.SharedObjectRelease {
	out := in
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	return out
}

func cloneSharedObject(in model.SharedObject) model.SharedObject {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	return out
}

func cloneSharedObjectRef(in model.SharedObjectRef) model.SharedObjectRef {
	out := in
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	return out
}

func sharedObjectRefFromVersion(shared model.SharedObject, version model.ObjectVersion, now time.Time) model.SharedObjectRef {
	return model.SharedObjectRef{
		RefID:          sharedObjectRefID(shared.SharedObjectID, version.BucketID, version.Key, version.VersionID),
		SharedObjectID: shared.SharedObjectID,
		BucketID:       version.BucketID,
		Key:            version.Key,
		VersionID:      version.VersionID,
		SegmentRefs:    cloneSegmentRefs(shared.SegmentRefs),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func objectVersionMatchesSharedObject(version model.ObjectVersion, shared model.SharedObject) bool {
	if version.SizeBytes != shared.SizeBytes {
		return false
	}
	digest, ok := objectVersionDigest(version)
	if !ok {
		return false
	}
	return strings.EqualFold(digest.Algorithm, shared.Digest.Algorithm) && strings.EqualFold(digest.Hex, shared.Digest.Hex)
}

func sharedObjectProtectedRootCount(tx ReadWriter, shared model.SharedObject, now time.Time) (int, error) {
	active := make(map[string]struct{})
	for _, segmentRef := range shared.SegmentRefs {
		if segmentRef.SegmentID == "" {
			continue
		}
		cursor := ""
		for {
			keys, next, err := tx.List(protectedRefSegmentPrefix(segmentRef.SegmentID), cursor, 128)
			if err != nil {
				return 0, err
			}
			for _, key := range keys {
				ref, ok, err := getJSON[model.ProtectedRef](tx, key)
				if err != nil {
					return 0, err
				}
				if !ok || !protectedRefActive(ref, now) {
					continue
				}
				active[ref.RefID] = struct{}{}
			}
			if next == "" {
				break
			}
			cursor = next
		}
	}
	return len(active), nil
}

func sharedObjectRefCount(tx ReadWriter, sharedObjectID string) (int, error) {
	count := 0
	cursor := ""
	for {
		keys, next, err := tx.List(keyspace.SharedObjectRefPrefix(sharedObjectID), cursor, 128)
		if err != nil {
			return 0, err
		}
		for _, key := range keys {
			if _, ok, err := getJSON[model.SharedObjectRef](tx, key); err != nil {
				return 0, err
			} else if ok {
				count++
			}
		}
		if next == "" {
			return count, nil
		}
		cursor = next
	}
}

func sharedObjectFromVersion(bucket model.Bucket, version model.ObjectVersion, now time.Time) (model.SharedObject, error) {
	refs := objectSegmentRefsFromVersion(version)
	if len(refs) == 0 {
		return model.SharedObject{}, fmt.Errorf("%w: source version has no segment refs", meta.ErrInvalidArgument)
	}
	digest, ok := objectVersionDigest(version)
	if !ok {
		return model.SharedObject{}, fmt.Errorf("%w: source version has no stable object digest", meta.ErrInvalidArgument)
	}
	sharedObjectID := sharedObjectID(bucket.TenantID, version.BucketID, version.Key, digest, version.SizeBytes)
	for i := range refs {
		refs[i].SharedObjectID = sharedObjectID
	}
	return model.SharedObject{
		SharedObjectID:     sharedObjectID,
		TenantID:           bucket.TenantID,
		BucketID:           version.BucketID,
		Key:                version.Key,
		SourceVersionID:    version.VersionID,
		SizeBytes:          version.SizeBytes,
		Digest:             digest,
		StorageClass:       cloneStorageClass(version.StorageClass),
		SegmentRefs:        refs,
		RefCount:           1,
		ProtectedRootCount: 0,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}

func objectVersionDigest(version model.ObjectVersion) (storage.Digest, bool) {
	if version.SegmentRef.Digest.Algorithm != "" && version.SegmentRef.Digest.Hex != "" {
		return version.SegmentRef.Digest, true
	}
	if len(version.SegmentRefs) == 1 && version.SegmentRefs[0].Digest.Algorithm != "" && version.SegmentRefs[0].Digest.Hex != "" {
		return version.SegmentRefs[0].Digest, true
	}
	return storage.Digest{}, false
}

func sharedObjectID(tenantID, bucketID, key string, digest storage.Digest, sizeBytes int64) string {
	sum := sha256.Sum256([]byte(tenantID + "\x00" + bucketID + "\x00" + key + "\x00" + strings.ToLower(digest.Algorithm) + "\x00" + strings.ToLower(digest.Hex) + "\x00" + strconv.FormatInt(sizeBytes, 10)))
	return "so-" + hex.EncodeToString(sum[:16])
}

func sharedObjectRefID(sharedObjectID, bucketID, key, versionID string) string {
	sum := sha256.Sum256([]byte(sharedObjectID + "\x00" + bucketID + "\x00" + key + "\x00" + versionID))
	return "soref-" + hex.EncodeToString(sum[:16])
}

func normalizeGCOperationStatus(status model.GCOperationStatus, retryable int) model.GCOperationStatus {
	if status != "" {
		return status
	}
	if retryable > 0 {
		return model.GCOperationRetryPending
	}
	return model.GCOperationSucceeded
}

func normalizeDedupeOperationStatus(status model.DedupeOperationStatus, retryable int) model.DedupeOperationStatus {
	if status != "" {
		return status
	}
	if retryable > 0 {
		return model.DedupeOperationRetryPending
	}
	return model.DedupeOperationSucceeded
}

func normalizeSharedObjectReleaseStatus(status model.SharedObjectReleaseStatus) model.SharedObjectReleaseStatus {
	if status != "" {
		return status
	}
	return model.SharedObjectReleasePending
}

func sharedObjectReleaseID(sharedObjectID, segmentID string) string {
	return sharedObjectID + "/" + segmentID
}

func numericIDSuffix(id, prefix string) int {
	suffix := strings.TrimPrefix(id, prefix)
	if suffix == id || suffix == "" {
		return 0
	}
	value, err := strconv.Atoi(suffix)
	if err != nil {
		return 0
	}
	return value
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneCORSRules(in []model.CORSRule) []model.CORSRule {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.CORSRule, 0, len(in))
	for _, rule := range in {
		out = append(out, model.CORSRule{
			AllowedOrigins: cloneStringSlice(rule.AllowedOrigins),
			AllowedMethods: cloneStringSlice(rule.AllowedMethods),
			AllowedHeaders: cloneStringSlice(rule.AllowedHeaders),
			ExposeHeaders:  cloneStringSlice(rule.ExposeHeaders),
			MaxAgeSeconds:  rule.MaxAgeSeconds,
		})
	}
	return out
}

func cloneLifecycleConfiguration(in model.BucketLifecycleConfiguration) model.BucketLifecycleConfiguration {
	if len(in.Rules) == 0 {
		return model.BucketLifecycleConfiguration{}
	}
	out := model.BucketLifecycleConfiguration{
		Rules: make([]model.LifecycleRule, len(in.Rules)),
	}
	copy(out.Rules, in.Rules)
	return out
}

func validateBucketLifecycleConfiguration(configuration model.BucketLifecycleConfiguration) error {
	if len(configuration.Rules) == 0 {
		return fmt.Errorf("%w: lifecycle configuration must include at least one rule", meta.ErrInvalidArgument)
	}
	if len(configuration.Rules) > 1000 {
		return fmt.Errorf("%w: lifecycle configuration cannot contain more than 1000 rules", meta.ErrInvalidArgument)
	}
	for _, rule := range configuration.Rules {
		switch rule.Status {
		case model.LifecycleRuleEnabled, model.LifecycleRuleDisabled:
		default:
			return fmt.Errorf("%w: lifecycle rule status must be Enabled or Disabled", meta.ErrInvalidArgument)
		}
		hasAction := false
		if rule.Expiration.Days > 0 || !rule.Expiration.Date.IsZero() || rule.Expiration.ExpiredObjectDeleteMarker {
			hasAction = true
		}
		if rule.NoncurrentVersionExpiration.NoncurrentDays > 0 {
			hasAction = true
		}
		if rule.AbortIncompleteMultipartUpload.DaysAfterInitiation > 0 {
			hasAction = true
		}
		if !hasAction {
			return fmt.Errorf("%w: lifecycle rule must include at least one action", meta.ErrInvalidArgument)
		}
		if rule.Expiration.Days < 0 || rule.NoncurrentVersionExpiration.NoncurrentDays < 0 || rule.AbortIncompleteMultipartUpload.DaysAfterInitiation < 0 {
			return fmt.Errorf("%w: lifecycle day values must be positive", meta.ErrInvalidArgument)
		}
	}
	return nil
}

func validateBucketObjectLockConfiguration(configuration model.BucketObjectLockConfiguration) error {
	if !configuration.Enabled {
		return fmt.Errorf("%w: object lock configuration must be enabled", meta.ErrInvalidArgument)
	}
	retention := configuration.DefaultRetention
	if retention.Mode == "" && retention.Days == 0 && retention.Years == 0 {
		return nil
	}
	if !isValidObjectLockMode(retention.Mode) {
		return fmt.Errorf("%w: object lock default retention mode is invalid", meta.ErrInvalidArgument)
	}
	if retention.Days <= 0 && retention.Years <= 0 {
		return fmt.Errorf("%w: object lock default retention period is required", meta.ErrInvalidArgument)
	}
	if retention.Days > 0 && retention.Years > 0 {
		return fmt.Errorf("%w: object lock default retention must use days or years", meta.ErrInvalidArgument)
	}
	return nil
}

func validateObjectLockState(retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus) error {
	if retention.Mode == "" {
		if !retention.RetainUntilDate.IsZero() {
			return fmt.Errorf("%w: object lock retention mode is required", meta.ErrInvalidArgument)
		}
	} else {
		if !isValidObjectLockMode(retention.Mode) {
			return fmt.Errorf("%w: object lock retention mode is invalid", meta.ErrInvalidArgument)
		}
		if retention.RetainUntilDate.IsZero() {
			return fmt.Errorf("%w: object lock retain-until date is required", meta.ErrInvalidArgument)
		}
	}
	switch legalHold {
	case "", model.ObjectLockLegalHoldOn, model.ObjectLockLegalHoldOff:
		return nil
	default:
		return fmt.Errorf("%w: object lock legal hold status is invalid", meta.ErrInvalidArgument)
	}
}

func isValidObjectLockMode(mode model.ObjectLockMode) bool {
	return mode == model.ObjectLockModeGovernance || mode == model.ObjectLockModeCompliance
}

func objectHeadProtectedByObjectLock(head model.ObjectHead, now time.Time, bypassGovernance bool) bool {
	return objectLockProtected(head.DeleteMarker, head.ObjectLockRetention, head.ObjectLockLegalHold, now, bypassGovernance)
}

func objectVersionProtectedByObjectLock(version model.ObjectVersion, now time.Time, bypassGovernance bool) bool {
	return objectLockProtected(version.DeleteMarker, version.ObjectLockRetention, version.ObjectLockLegalHold, now, bypassGovernance)
}

func objectLockProtected(deleteMarker bool, retention model.ObjectLockRetention, legalHold model.ObjectLockLegalHoldStatus, now time.Time, bypassGovernance bool) bool {
	if deleteMarker {
		return false
	}
	if legalHold == model.ObjectLockLegalHoldOn {
		return true
	}
	if retention.Mode == "" {
		return false
	}
	if retention.RetainUntilDate.IsZero() {
		return true
	}
	if !now.UTC().Before(retention.RetainUntilDate.UTC()) {
		return false
	}
	switch retention.Mode {
	case model.ObjectLockModeCompliance:
		return true
	case model.ObjectLockModeGovernance:
		return !bypassGovernance
	default:
		return true
	}
}

func retentionUpdateBlocked(current, next model.ObjectLockRetention, now time.Time, bypassGovernance bool) bool {
	if current.Mode == "" {
		return false
	}
	if current.RetainUntilDate.IsZero() {
		return true
	}
	if !now.UTC().Before(current.RetainUntilDate.UTC()) {
		return false
	}
	switch current.Mode {
	case model.ObjectLockModeCompliance:
		return next.Mode != model.ObjectLockModeCompliance || next.RetainUntilDate.Before(current.RetainUntilDate)
	case model.ObjectLockModeGovernance:
		return !bypassGovernance
	default:
		return true
	}
}

func validGovernanceBypassAudit(audit meta.AuditContext) bool {
	return strings.TrimSpace(audit.Reason) != "" && strings.TrimSpace(audit.Principal.AccessKeyID) != ""
}

func governanceBypassAuditEvent(req meta.DeleteObjectRequest, action model.AuditAction, versionID string, details map[string]string) model.AuditEvent {
	return model.AuditEvent{
		Action:    action,
		BucketID:  req.BucketID,
		Key:       req.Key,
		VersionID: versionID,
		RequestID: req.BypassAudit.RequestID,
		Reason:    strings.TrimSpace(req.BypassAudit.Reason),
		Principal: req.BypassAudit.Principal,
		Details:   cloneStringMap(details),
	}
}

func transitionAuditEvent(audit meta.AuditContext, action model.AuditAction, bucketID, key, versionID string, details map[string]string) model.AuditEvent {
	reason := strings.TrimSpace(audit.Reason)
	if reason == "" {
		reason = string(action)
	}
	return model.AuditEvent{
		Action:    action,
		BucketID:  bucketID,
		Key:       key,
		VersionID: versionID,
		RequestID: audit.RequestID,
		Reason:    reason,
		Principal: audit.Principal,
		Details:   cloneStringMap(details),
	}
}

func bucketObjectLockAuditDetails(config model.BucketObjectLockConfiguration) map[string]string {
	details := map[string]string{
		"enabled": strconv.FormatBool(config.Enabled),
	}
	if config.DefaultRetention.Mode != "" {
		details["default_retention_mode"] = string(config.DefaultRetention.Mode)
	}
	if config.DefaultRetention.Days > 0 {
		details["default_retention_days"] = strconv.Itoa(config.DefaultRetention.Days)
	}
	if config.DefaultRetention.Years > 0 {
		details["default_retention_years"] = strconv.Itoa(config.DefaultRetention.Years)
	}
	return details
}

func formatAuditTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func auditEventHash(event model.AuditEvent) string {
	payload := struct {
		EventID      string
		Action       model.AuditAction
		BucketID     string
		Key          string
		VersionID    string
		RequestID    string
		Reason       string
		Principal    model.AuditPrincipal
		Details      map[string]string
		PreviousHash string
		CreatedAt    string
	}{
		EventID:      event.EventID,
		Action:       event.Action,
		BucketID:     event.BucketID,
		Key:          event.Key,
		VersionID:    event.VersionID,
		RequestID:    event.RequestID,
		Reason:       event.Reason,
		Principal:    event.Principal,
		Details:      cloneStringMap(event.Details),
		PreviousHash: event.PreviousHash,
		CreatedAt:    event.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func protectedRefFromVersion(version model.ObjectVersion, segmentRef storage.SegmentRef, now time.Time) model.ProtectedRef {
	return model.ProtectedRef{
		RefID:           protectedRefID(version, segmentRef),
		Reason:          model.ProtectedRefReasonObjectLock,
		BucketID:        version.BucketID,
		Key:             version.Key,
		VersionID:       version.VersionID,
		SegmentID:       segmentRef.SegmentID,
		SegmentRef:      cloneSegmentRef(segmentRef),
		RetentionMode:   version.ObjectLockRetention.Mode,
		RetainUntilDate: version.ObjectLockRetention.RetainUntilDate,
		LegalHold:       version.ObjectLockLegalHold,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func protectedRefID(version model.ObjectVersion, segmentRef storage.SegmentRef) string {
	sum := sha256.Sum256([]byte(version.BucketID + "\x00" + version.Key + "\x00" + version.VersionID + "\x00" + segmentRef.SegmentID))
	return hex.EncodeToString(sum[:])
}

func protectedRefActive(ref model.ProtectedRef, now time.Time) bool {
	if ref.LegalHold == model.ObjectLockLegalHoldOn {
		return true
	}
	if ref.RetentionMode == "" {
		return false
	}
	if ref.RetainUntilDate.IsZero() {
		return true
	}
	return now.UTC().Before(ref.RetainUntilDate.UTC())
}

func protectedRefVersionPrefix(bucketID, key, versionID string) string {
	return keyspace.ProtectedRefByVersion(bucketID, key, versionID, "")
}

func protectedRefSegmentPrefix(segmentID string) string {
	if segmentID == "" {
		return strings.TrimSuffix(keyspace.ProtectedRefBySegment("", ""), "/")
	}
	return keyspace.ProtectedRefBySegment(segmentID, "")
}

func cloneStorageClass(in storage.StorageClassSnapshot) storage.StorageClassSnapshot {
	return storage.CloneStorageClassSnapshot(in)
}

func cloneSegmentRef(in storage.SegmentRef) storage.SegmentRef {
	return storage.CloneSegmentRef(in)
}

func cloneSegmentRefs(in []storage.SegmentRef) []storage.SegmentRef {
	return storage.CloneSegmentRefs(in)
}

func normalizeSegmentRefs(single storage.SegmentRef, refs []storage.SegmentRef) []storage.SegmentRef {
	if len(refs) > 0 {
		return cloneSegmentRefs(refs)
	}
	if single.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{cloneSegmentRef(single)}
}

func objectSegmentRefsFromVersion(version model.ObjectVersion) []storage.SegmentRef {
	return normalizeSegmentRefs(version.SegmentRef, version.SegmentRefs)
}

func firstSegmentRef(refs []storage.SegmentRef) storage.SegmentRef {
	if len(refs) == 0 {
		return storage.SegmentRef{}
	}
	return cloneSegmentRef(refs[0])
}

func cloneHead(in model.ObjectHead) model.ObjectHead {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	out.UserMetadata = cloneStringMap(in.UserMetadata)
	out.Tags = cloneStringMap(in.Tags)
	return out
}

func hydrateHeadManifest(tx ReadWriter, head model.ObjectHead) (model.ObjectHead, error) {
	out := cloneHead(head)
	if out.DeleteMarker || out.VersionID == "" || out.SegmentRef.SegmentID != "" || len(out.SegmentRefs) > 0 {
		return out, nil
	}
	_, version, ok, err := findVersionByID(tx, out.BucketID, out.Key, out.VersionID)
	if err != nil {
		return model.ObjectHead{}, err
	}
	if !ok {
		return out, nil
	}
	out.SegmentRef = cloneSegmentRef(version.SegmentRef)
	out.SegmentRefs = cloneSegmentRefs(version.SegmentRefs)
	return out, nil
}

func hydrateObjectVersionManifest(tx ReadWriter, version model.ObjectVersion) (model.ObjectVersion, error) {
	out := cloneVersion(version)
	if out.Manifest.Encoding != model.ObjectManifestEncodingChunked {
		return out, nil
	}
	if out.Manifest.ChunkCount <= 0 || out.Manifest.RefCount < 0 {
		return model.ObjectVersion{}, fmt.Errorf("%w: object manifest descriptor is invalid for version %q", meta.ErrUnavailable, out.VersionID)
	}
	refs := make([]storage.SegmentRef, 0, out.Manifest.RefCount)
	for chunkNumber := 1; chunkNumber <= out.Manifest.ChunkCount; chunkNumber++ {
		chunk, ok, err := getJSON[model.ObjectManifestChunk](tx, keyspace.ObjectManifestChunk(out.BucketID, out.VersionID, chunkNumber))
		if err != nil {
			return model.ObjectVersion{}, err
		}
		if !ok {
			return model.ObjectVersion{}, fmt.Errorf("%w: object manifest chunk %d is missing for version %q", meta.ErrUnavailable, chunkNumber, out.VersionID)
		}
		if chunk.BucketID != out.BucketID || chunk.VersionID != out.VersionID || chunk.ChunkNumber != chunkNumber {
			return model.ObjectVersion{}, fmt.Errorf("%w: object manifest chunk %d does not match version %q", meta.ErrUnavailable, chunkNumber, out.VersionID)
		}
		refs = append(refs, cloneSegmentRefs(chunk.SegmentRefs)...)
	}
	if len(refs) != out.Manifest.RefCount {
		return model.ObjectVersion{}, fmt.Errorf("%w: object manifest ref count is %d, want %d for version %q", meta.ErrUnavailable, len(refs), out.Manifest.RefCount, out.VersionID)
	}
	out.SegmentRefs = refs
	out.SegmentRef = firstSegmentRef(refs)
	return out, nil
}

func getObjectHead(tx ReadWriter, bucketID, objectKey string) (model.ObjectHead, bool, error) {
	entry, ok, err := getJSON[model.ObjectHeadEntry](tx, keyspace.ObjectHead(bucketID, objectKey))
	if err != nil || !ok {
		return model.ObjectHead{}, ok, err
	}
	return headFromHeadEntry(entry), true, nil
}

func setObjectHead(tx ReadWriter, head model.ObjectHead) (model.ObjectHead, error) {
	previous, found, err := getObjectHead(tx, head.BucketID, head.Key)
	if err != nil {
		return model.ObjectHead{}, err
	}
	head.Revision = nextObjectHeadRevision(previous, found)
	return head, setJSON(tx, keyspace.ObjectHead(head.BucketID, head.Key), headEntryFromHead(head))
}

func nextObjectHeadRevision(previous model.ObjectHead, found bool) uint64 {
	if !found || previous.Revision == 0 {
		return 1
	}
	return previous.Revision + 1
}

func headEntryFromHead(head model.ObjectHead) model.ObjectHeadEntry {
	return model.ObjectHeadEntry{
		BucketID:             head.BucketID,
		Key:                  head.Key,
		VersionID:            head.VersionID,
		Revision:             head.Revision,
		SizeBytes:            head.SizeBytes,
		ETag:                 head.ETag,
		ContentType:          head.ContentType,
		StorageClass:         cloneStorageClass(head.StorageClass),
		ServerSideEncryption: head.ServerSideEncryption,
		UserMetadata:         cloneStringMap(head.UserMetadata),
		Tags:                 cloneStringMap(head.Tags),
		ObjectLockRetention:  head.ObjectLockRetention,
		ObjectLockLegalHold:  head.ObjectLockLegalHold,
		LastModified:         head.LastModified,
		DeleteMarker:         head.DeleteMarker,
	}
}

func headFromHeadEntry(entry model.ObjectHeadEntry) model.ObjectHead {
	return model.ObjectHead{
		BucketID:             entry.BucketID,
		Key:                  entry.Key,
		VersionID:            entry.VersionID,
		Revision:             entry.Revision,
		SizeBytes:            entry.SizeBytes,
		ETag:                 entry.ETag,
		ContentType:          entry.ContentType,
		StorageClass:         cloneStorageClass(entry.StorageClass),
		ServerSideEncryption: entry.ServerSideEncryption,
		UserMetadata:         cloneStringMap(entry.UserMetadata),
		Tags:                 cloneStringMap(entry.Tags),
		ObjectLockRetention:  entry.ObjectLockRetention,
		ObjectLockLegalHold:  entry.ObjectLockLegalHold,
		LastModified:         entry.LastModified,
		DeleteMarker:         entry.DeleteMarker,
	}
}

func setListObject(tx ReadWriter, head model.ObjectHead) error {
	if head.DeleteMarker {
		return tx.Delete(keyspace.ListObject(head.BucketID, head.Key))
	}
	return setJSON(tx, keyspace.ListObject(head.BucketID, head.Key), listEntryFromHead(head))
}

func listEntryFromHead(head model.ObjectHead) model.ObjectListEntry {
	return model.ObjectListEntry{
		BucketID:     head.BucketID,
		Key:          head.Key,
		VersionID:    head.VersionID,
		Revision:     head.Revision,
		SizeBytes:    head.SizeBytes,
		ETag:         head.ETag,
		ContentType:  head.ContentType,
		StorageClass: cloneStorageClass(head.StorageClass),
		LastModified: head.LastModified,
		DeleteMarker: head.DeleteMarker,
	}
}

func headFromListEntry(entry model.ObjectListEntry) model.ObjectHead {
	return model.ObjectHead{
		BucketID:     entry.BucketID,
		Key:          entry.Key,
		VersionID:    entry.VersionID,
		Revision:     entry.Revision,
		SizeBytes:    entry.SizeBytes,
		ETag:         entry.ETag,
		ContentType:  entry.ContentType,
		StorageClass: cloneStorageClass(entry.StorageClass),
		LastModified: entry.LastModified,
		DeleteMarker: entry.DeleteMarker,
	}
}

func cloneVersion(in model.ObjectVersion) model.ObjectVersion {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	out.SegmentRefs = cloneSegmentRefs(in.SegmentRefs)
	out.UserMetadata = cloneStringMap(in.UserMetadata)
	out.Tags = cloneStringMap(in.Tags)
	return out
}

func cloneUpload(in model.MultipartUpload) model.MultipartUpload {
	out := in
	out.StorageClass = cloneStorageClass(in.StorageClass)
	out.UserMetadata = cloneStringMap(in.UserMetadata)
	out.Tags = cloneStringMap(in.Tags)
	return out
}

func clonePart(in model.MultipartPart) model.MultipartPart {
	out := in
	out.SegmentRef = cloneSegmentRef(in.SegmentRef)
	return out
}

func cloneMultipartCompletionRecord(in model.MultipartCompletionRecord) model.MultipartCompletionRecord {
	return in
}

func multipartCompletionMatches(record model.MultipartCompletionRecord, req meta.PrepareMultipartCompletionRequest) bool {
	return record.BucketID == req.BucketID &&
		record.Key == req.Key &&
		record.UploadID == req.UploadID &&
		record.ObjectVersionID == req.ObjectVersionID &&
		record.ExpectedHeadVersionID == req.ExpectedHeadVersionID &&
		record.ETag == req.ETag &&
		record.SizeBytes == req.SizeBytes &&
		record.PartCount == req.PartCount
}

func multipartCompletionStateAtLeast(state, target model.MultipartCompletionState) bool {
	return multipartCompletionStateRank(state) >= multipartCompletionStateRank(target)
}

func multipartCompletionStateRank(state model.MultipartCompletionState) int {
	switch state {
	case model.MultipartCompletionCompleted:
		return 3
	case model.MultipartCompletionPublished:
		return 2
	case model.MultipartCompletionPrepared:
		return 1
	default:
		return 0
	}
}

func headFromVersion(version model.ObjectVersion, lastModified time.Time) model.ObjectHead {
	return model.ObjectHead{
		BucketID:             version.BucketID,
		Key:                  version.Key,
		VersionID:            version.VersionID,
		SizeBytes:            version.SizeBytes,
		ETag:                 version.ETag,
		ContentType:          version.ContentType,
		StorageClass:         cloneStorageClass(version.StorageClass),
		ServerSideEncryption: version.ServerSideEncryption,
		SegmentRef:           cloneSegmentRef(version.SegmentRef),
		SegmentRefs:          cloneSegmentRefs(version.SegmentRefs),
		UserMetadata:         cloneStringMap(version.UserMetadata),
		Tags:                 cloneStringMap(version.Tags),
		ObjectLockRetention:  version.ObjectLockRetention,
		ObjectLockLegalHold:  version.ObjectLockLegalHold,
		LastModified:         lastModified,
		DeleteMarker:         version.DeleteMarker,
	}
}

var _ meta.Repository = (*Repository)(nil)

package adminstatus

import (
	"context"
	"strings"
	"time"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/coordination"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
)

type Request struct {
	CountLimit        int
	RecentDedupeLimit int
	RecentGCLimit     int
}

type Output struct {
	SchemaVersion          int                 `json:"schema_version"`
	GeneratedAt            string              `json:"generated_at"`
	Status                 string              `json:"status"`
	Metadata               Metadata            `json:"metadata"`
	MetadataSchema         MetadataSchema      `json:"metadata_schema"`
	Limits                 Limits              `json:"limits"`
	Counts                 Counts              `json:"counts"`
	AuditChain             AuditChain          `json:"audit_chain"`
	RecentDedupeOperations []DedupeOperation   `json:"recent_dedupe_operations,omitempty"`
	RecentGCOperations     []GCOperation       `json:"recent_gc_operations,omitempty"`
	MetadataRestore        MetadataRestore     `json:"metadata_restore"`
	ProductionReadiness    ProductionReadiness `json:"production_readiness"`
	Capabilities           Capabilities        `json:"capabilities"`
	Edition                EditionStatus       `json:"edition"`
}

type Metadata struct {
	Backend           string   `json:"backend"`
	Path              string   `json:"path,omitempty"`
	TiKVPDEndpoints   []string `json:"tikv_pd_endpoints,omitempty"`
	TiKVAPIVersion    string   `json:"tikv_api_version,omitempty"`
	TiKVKeyspace      string   `json:"tikv_keyspace,omitempty"`
	TiKVTimeout       string   `json:"tikv_timeout,omitempty"`
	TiKVRetryAttempts int      `json:"tikv_retry_attempts,omitempty"`
}

type MetadataSchema struct {
	Status               string `json:"status"`
	Reason               string `json:"reason"`
	CurrentVersion       int    `json:"current_version"`
	MinimumReaderVersion int    `json:"minimum_reader_version"`
	MinimumWriterVersion int    `json:"minimum_writer_version"`
	SchemaVersion        int    `json:"schema_version,omitempty"`
	MinReaderVersion     int    `json:"min_reader_version,omitempty"`
	MinWriterVersion     int    `json:"min_writer_version,omitempty"`
	UpdatedBy            string `json:"updated_by,omitempty"`
	CreatedAt            string `json:"created_at,omitempty"`
	UpdatedAt            string `json:"updated_at,omitempty"`
	MigrationRequired    bool   `json:"migration_required,omitempty"`
	UnsupportedFuture    bool   `json:"unsupported_future,omitempty"`
	Error                string `json:"error,omitempty"`
}

type Limits struct {
	CountLimit        int `json:"count_limit"`
	RecentDedupeLimit int `json:"recent_dedupe_limit"`
	RecentGCLimit     int `json:"recent_gc_limit"`
}

type Counts struct {
	KMSKeys              int `json:"kms_keys"`
	AuditEvents          int `json:"audit_events"`
	GCOperations         int `json:"gc_operations"`
	DedupeOperations     int `json:"dedupe_operations"`
	SharedObjects        int `json:"shared_objects"`
	SharedObjectReleases int `json:"shared_object_releases"`
}

type Capabilities struct {
	MetadataExport            bool `json:"metadata_export"`
	MetadataImportDryRun      bool `json:"metadata_import_dry_run"`
	MetadataImportApplyPlan   bool `json:"metadata_import_apply_plan"`
	MetadataImportApply       bool `json:"metadata_import_apply"`
	ComplianceEvidencePackage bool `json:"compliance_evidence_package"`
	ComplianceAccessEvidence  bool `json:"compliance_access_evidence"`
	ComplianceTimeSource      bool `json:"compliance_time_source"`
	ComplianceProfilePlan     bool `json:"compliance_profile_plan"`
	CompliancePolicySimulate  bool `json:"compliance_policy_simulate"`
	DedupeOperations          bool `json:"dedupe_operations"`
}

type EditionStatus struct {
	Name     string            `json:"name"`
	Features []edition.Feature `json:"features"`
}

type AuditChain struct {
	Sampled       int    `json:"sampled"`
	FirstEventID  string `json:"first_event_id,omitempty"`
	LastEventID   string `json:"last_event_id,omitempty"`
	LastHash      string `json:"last_hash,omitempty"`
	HashesPresent bool   `json:"hashes_present"`
}

type MetadataRestore struct {
	SchemaVersion             int      `json:"schema_version"`
	ConflictPolicy            string   `json:"conflict_policy"`
	RequireEmptyTargetDefault bool     `json:"require_empty_target_default"`
	PreserveSourceIDs         bool     `json:"preserve_source_ids"`
	PreserveAuditHashes       bool     `json:"preserve_audit_hashes"`
	Collections               []string `json:"collections"`
}

type ProductionReadiness struct {
	SchemaVersion                  string   `json:"schema_version"`
	Status                         string   `json:"status"`
	DeploymentProfile              string   `json:"deployment_profile"`
	AllowUnsafeProductionShortcuts bool     `json:"allow_unsafe_production_shortcuts"`
	MetadataBackend                string   `json:"metadata_backend"`
	CoordinationBackend            string   `json:"coordination_backend"`
	GatewayCountKnown              bool     `json:"gateway_count_known"`
	GatewayCount                   int      `json:"gateway_count"`
	HealthyGatewayCount            int      `json:"healthy_gateway_count"`
	GatewayRegistryError           string   `json:"gateway_registry_error,omitempty"`
	StorageBackend                 string   `json:"storage_backend"`
	SBSVolumePoolSource            string   `json:"sbs_volume_pool_source"`
	SBSVolumePoolID                string   `json:"sbs_volume_pool_id,omitempty"`
	SBSVolumePoolGeneration        uint64   `json:"sbs_volume_pool_generation,omitempty"`
	SBSVolumePoolMemberCount       int      `json:"sbs_volume_pool_member_count"`
	SBSVolumePoolError             string   `json:"sbs_volume_pool_error,omitempty"`
	SBSSessionFencingConfigured    bool     `json:"sbs_session_fencing_configured"`
	GCCandidateQueue               string   `json:"gc_candidate_queue"`
	UnsupportedClaims              []string `json:"unsupported_claims,omitempty"`
}

type DedupeOperation struct {
	OperationID         string                      `json:"operation_id"`
	ResumeOfOperationID string                      `json:"resume_of_operation_id,omitempty"`
	Status              model.DedupeOperationStatus `json:"status"`
	StartedAt           string                      `json:"started_at"`
	FinishedAt          string                      `json:"finished_at"`
	Scanned             int                         `json:"scanned"`
	Acked               int                         `json:"acked"`
	Skipped             int                         `json:"skipped"`
	Retryable           int                         `json:"retryable"`
	Attempts            int                         `json:"attempts"`
	CreatedAt           string                      `json:"created_at"`
}

type GCOperation struct {
	OperationID         string                  `json:"operation_id"`
	ResumeOfOperationID string                  `json:"resume_of_operation_id,omitempty"`
	Status              model.GCOperationStatus `json:"status"`
	StartedAt           string                  `json:"started_at"`
	FinishedAt          string                  `json:"finished_at"`
	Scanned             int                     `json:"scanned"`
	Deleted             int                     `json:"deleted"`
	Skipped             int                     `json:"skipped"`
	Retryable           int                     `json:"retryable"`
	Attempts            int                     `json:"attempts"`
	CreatedAt           string                  `json:"created_at"`
}

func Build(ctx context.Context, cfg config.Config, repo meta.Repository, req Request) (Output, error) {
	if req.CountLimit <= 0 {
		req.CountLimit = 1000
	}
	if req.RecentDedupeLimit < 0 {
		req.RecentDedupeLimit = 0
	}
	if req.RecentGCLimit < 0 {
		req.RecentGCLimit = 0
	}
	counts, err := RepositoryCounts(ctx, repo, req.CountLimit)
	if err != nil {
		return Output{}, err
	}
	auditEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{Limit: req.CountLimit})
	if err != nil {
		return Output{}, err
	}
	var recentDedupe []model.DedupeOperationRecord
	if req.RecentDedupeLimit > 0 {
		recentDedupe, err = repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: req.RecentDedupeLimit})
		if err != nil {
			return Output{}, err
		}
	}
	var recentGC []model.GCOperationRecord
	if req.RecentGCLimit > 0 {
		recentGC, err = repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: req.RecentGCLimit})
		if err != nil {
			return Output{}, err
		}
	}
	return Output{
		SchemaVersion:          1,
		GeneratedAt:            formatTime(time.Now().UTC()),
		Status:                 "ok",
		Metadata:               MetadataFromConfig(cfg),
		MetadataSchema:         MetadataSchemaFromPosture(meta.CheckMetadataSchema(ctx, repo)),
		Limits:                 Limits{CountLimit: req.CountLimit, RecentDedupeLimit: req.RecentDedupeLimit, RecentGCLimit: req.RecentGCLimit},
		Counts:                 counts,
		AuditChain:             AuditChainFromEvents(auditEvents),
		RecentDedupeOperations: DedupeOperations(recentDedupe),
		RecentGCOperations:     GCOperations(recentGC),
		MetadataRestore:        DefaultMetadataRestore(),
		ProductionReadiness:    BuildProductionReadiness(ctx, cfg, repo),
		Capabilities:           DefaultCapabilities(),
		Edition:                EditionFromConfig(cfg),
	}, nil
}

func MetadataSchemaFromPosture(posture meta.MetadataSchemaPosture) MetadataSchema {
	record := posture.Record
	return MetadataSchema{
		Status:               posture.Status,
		Reason:               posture.Reason,
		CurrentVersion:       posture.CurrentVersion,
		MinimumReaderVersion: posture.MinimumReaderVersion,
		MinimumWriterVersion: posture.MinimumWriterVersion,
		SchemaVersion:        record.SchemaVersion,
		MinReaderVersion:     record.MinReaderVersion,
		MinWriterVersion:     record.MinWriterVersion,
		UpdatedBy:            record.UpdatedBy,
		CreatedAt:            formatTime(record.CreatedAt),
		UpdatedAt:            formatTime(record.UpdatedAt),
		MigrationRequired:    posture.MigrationRequired,
		UnsupportedFuture:    posture.UnsupportedFuture,
		Error:                posture.Error,
	}
}

func MetadataFromConfig(cfg config.Config) Metadata {
	backend := config.NormalizeMetadataBackend(cfg.MetadataBackend)
	out := Metadata{Backend: string(backend)}
	switch backend {
	case config.MetadataBackendPebble:
		out.Path = cfg.MetadataPath
	case config.MetadataBackendTiKV:
		out.TiKVPDEndpoints = append([]string(nil), cfg.TiKVPDEndpoints...)
		out.TiKVAPIVersion = cfg.TiKVAPIVersion
		out.TiKVKeyspace = cfg.TiKVKeyspace
		out.TiKVTimeout = cfg.TiKVTimeout.String()
		out.TiKVRetryAttempts = cfg.TiKVRetryAttempts
	}
	return out
}

func RepositoryCounts(ctx context.Context, repo meta.Repository, limit int) (Counts, error) {
	if limit <= 0 {
		limit = 1000
	}
	kmsKeys, err := repo.ListKMSKeys(ctx, meta.ListKMSKeysRequest{Limit: limit})
	if err != nil {
		return Counts{}, err
	}
	auditEvents, err := repo.ListAuditEvents(ctx, meta.ListAuditEventsRequest{Limit: limit})
	if err != nil {
		return Counts{}, err
	}
	gcOperations, err := repo.ListGCOperations(ctx, meta.ListGCOperationsRequest{Limit: limit})
	if err != nil {
		return Counts{}, err
	}
	dedupeOperations, err := repo.ListDedupeOperations(ctx, meta.ListDedupeOperationsRequest{Limit: limit})
	if err != nil {
		return Counts{}, err
	}
	sharedObjects, err := repo.ListSharedObjects(ctx, meta.ListSharedObjectsRequest{Limit: limit})
	if err != nil {
		return Counts{}, err
	}
	sharedReleases, err := repo.ListSharedObjectReleases(ctx, meta.ListSharedObjectReleasesRequest{Limit: limit})
	if err != nil {
		return Counts{}, err
	}
	return Counts{
		KMSKeys:              len(kmsKeys),
		AuditEvents:          len(auditEvents),
		GCOperations:         len(gcOperations),
		DedupeOperations:     len(dedupeOperations),
		SharedObjects:        len(sharedObjects),
		SharedObjectReleases: len(sharedReleases),
	}, nil
}

func DedupeOperations(records []model.DedupeOperationRecord) []DedupeOperation {
	if len(records) == 0 {
		return nil
	}
	out := make([]DedupeOperation, 0, len(records))
	for _, record := range records {
		out = append(out, DedupeOperation{
			OperationID:         record.OperationID,
			ResumeOfOperationID: record.ResumeOfOperationID,
			Status:              record.Status,
			StartedAt:           formatTime(record.StartedAt),
			FinishedAt:          formatTime(record.FinishedAt),
			Scanned:             record.Scanned,
			Acked:               record.Acked,
			Skipped:             record.Skipped,
			Retryable:           record.Retryable,
			Attempts:            len(record.Attempts),
			CreatedAt:           formatTime(record.CreatedAt),
		})
	}
	return out
}

func GCOperations(records []model.GCOperationRecord) []GCOperation {
	if len(records) == 0 {
		return nil
	}
	out := make([]GCOperation, 0, len(records))
	for _, record := range records {
		out = append(out, GCOperation{
			OperationID:         record.OperationID,
			ResumeOfOperationID: record.ResumeOfOperationID,
			Status:              record.Status,
			StartedAt:           formatTime(record.StartedAt),
			FinishedAt:          formatTime(record.FinishedAt),
			Scanned:             record.Scanned,
			Deleted:             record.Deleted,
			Skipped:             record.Skipped,
			Retryable:           record.Retryable,
			Attempts:            len(record.Attempts),
			CreatedAt:           formatTime(record.CreatedAt),
		})
	}
	return out
}

func AuditChainFromEvents(events []model.AuditEvent) AuditChain {
	if len(events) == 0 {
		return AuditChain{}
	}
	first := events[0]
	last := events[len(events)-1]
	hashesPresent := true
	for _, event := range events {
		if event.EventHash == "" {
			hashesPresent = false
			break
		}
	}
	return AuditChain{
		Sampled:       len(events),
		FirstEventID:  first.EventID,
		LastEventID:   last.EventID,
		LastHash:      last.EventHash,
		HashesPresent: hashesPresent,
	}
}

func DefaultMetadataRestore() MetadataRestore {
	return MetadataRestore{
		SchemaVersion:             1,
		ConflictPolicy:            "fail_if_exists",
		RequireEmptyTargetDefault: true,
		PreserveSourceIDs:         true,
		PreserveAuditHashes:       true,
		Collections: []string{
			"metadata_schema",
			"metadata_migration_operations",
			"kms_keys",
			"audit_events",
			"gc_operations",
			"dedupe_operations",
			"shared_objects",
			"shared_object_releases",
			"volume_pools",
			"volume_drain_operations",
			"worker_leases",
			"worker_operations",
		},
	}
}

func BuildProductionReadiness(ctx context.Context, cfg config.Config, repo meta.Repository) ProductionReadiness {
	out := ProductionReadiness{
		SchemaVersion:                  "namros.production_readiness.v1",
		Status:                         "blocked",
		DeploymentProfile:              config.NormalizeDeploymentProfile(cfg.DeploymentProfile),
		AllowUnsafeProductionShortcuts: cfg.AllowUnsafeProductionShortcuts,
		MetadataBackend:                config.NormalizeMetadataBackend(cfg.MetadataBackend),
		CoordinationBackend:            config.NormalizeCoordinationBackend(cfg.CoordinationBackend),
		StorageBackend:                 config.NormalizeStorageBackend(cfg.StorageBackend),
		SBSSessionFencingConfigured:    config.SBSSessionFencingConfigured(cfg),
		GCCandidateQueue:               config.NormalizeGCCandidateQueue(cfg.GCCandidateQueue),
	}
	populateGatewayCounts(ctx, cfg, &out)
	populateVolumePool(ctx, cfg, repo, &out)
	out.UnsupportedClaims = productionUnsupportedClaims(out)
	if len(out.UnsupportedClaims) == 0 {
		out.Status = "ready"
	}
	return out
}

func populateGatewayCounts(ctx context.Context, cfg config.Config, out *ProductionReadiness) {
	if !coordination.Enabled(cfg) {
		out.GatewayCountKnown = true
		return
	}
	timeout := cfg.EtcdDialTimeout + cfg.GatewayHeartbeat
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	gatewayCfg := coordination.GatewayConfigFromApp(cfg)
	records, err := coordination.ListGatewayRecords(checkCtx, gatewayCfg)
	if err != nil {
		out.GatewayRegistryError = err.Error()
		return
	}
	out.GatewayCountKnown = true
	out.GatewayCount = len(records)
	out.HealthyGatewayCount = len(coordination.HealthyGatewayRecords(records, time.Now().UTC(), coordination.GatewayMaxHeartbeatAge(gatewayCfg)))
}

func populateVolumePool(ctx context.Context, cfg config.Config, repo meta.Repository, out *ProductionReadiness) {
	if poolID := strings.TrimSpace(cfg.SBSVolumePoolID); poolID != "" {
		out.SBSVolumePoolSource = "metadata_registry"
		out.SBSVolumePoolID = poolID
		if repo == nil {
			out.SBSVolumePoolError = "metadata repository is not configured"
			return
		}
		pool, err := repo.GetVolumePool(ctx, poolID)
		if err != nil {
			out.SBSVolumePoolError = err.Error()
			return
		}
		out.SBSVolumePoolGeneration = pool.Generation
		out.SBSVolumePoolMemberCount = len(pool.Members)
		return
	}
	if len(cfg.SBSVolumePool) > 0 {
		out.SBSVolumePoolSource = "static_config"
		out.SBSVolumePoolMemberCount = len(cfg.SBSVolumePool)
		return
	}
	switch config.NormalizeStorageBackend(cfg.StorageBackend) {
	case config.StorageBackendSBSPhysical, config.StorageBackendSBSEC, config.StorageBackendSBSCluster:
		out.SBSVolumePoolSource = "direct_single_volume"
		out.SBSVolumePoolMemberCount = 1
	default:
		out.SBSVolumePoolSource = "not_configured"
	}
}

func productionUnsupportedClaims(readiness ProductionReadiness) []string {
	var claims []string
	if readiness.DeploymentProfile != config.DeploymentProfileProduction {
		claims = append(claims, "deployment_profile_not_production")
	}
	if readiness.AllowUnsafeProductionShortcuts {
		claims = append(claims, "unsafe_production_shortcuts_allowed")
	}
	if readiness.MetadataBackend != config.MetadataBackendTiKV {
		claims = append(claims, "metadata_backend_not_tikv")
	}
	if readiness.CoordinationBackend != config.CoordinationBackendEtcd {
		claims = append(claims, "coordination_backend_not_etcd")
	}
	if readiness.GatewayRegistryError != "" {
		claims = append(claims, "gateway_registry_unavailable")
	} else if !readiness.GatewayCountKnown {
		claims = append(claims, "gateway_count_unknown")
	} else if readiness.HealthyGatewayCount < 2 {
		claims = append(claims, "healthy_gateway_count_below_2")
	}
	switch readiness.StorageBackend {
	case config.StorageBackendSBSPhysical, config.StorageBackendSBSEC, config.StorageBackendSBSCluster:
	default:
		claims = append(claims, "storage_backend_not_sbs_volume_pool")
	}
	if readiness.SBSVolumePoolError != "" {
		claims = append(claims, "sbs_volume_pool_unavailable")
	} else if readiness.SBSVolumePoolMemberCount < 2 {
		claims = append(claims, "sbs_volume_pool_member_count_below_2")
	}
	if !readiness.SBSSessionFencingConfigured {
		claims = append(claims, "sbs_writer_session_fencing_not_configured")
	}
	if readiness.GCCandidateQueue != config.GCCandidateQueueMetadata {
		claims = append(claims, "gc_candidate_queue_not_metadata")
	}
	return claims
}

func DefaultCapabilities() Capabilities {
	return Capabilities{
		MetadataExport:            true,
		MetadataImportDryRun:      true,
		MetadataImportApplyPlan:   true,
		MetadataImportApply:       true,
		ComplianceEvidencePackage: true,
		ComplianceAccessEvidence:  true,
		ComplianceTimeSource:      true,
		ComplianceProfilePlan:     true,
		CompliancePolicySimulate:  true,
		DedupeOperations:          true,
	}
}

func EditionFromConfig(cfg config.Config) EditionStatus {
	name := edition.Normalize(cfg.Edition)
	return EditionStatus{
		Name:     name,
		Features: edition.FeaturesFor(name),
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

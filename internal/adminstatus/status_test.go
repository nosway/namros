package adminstatus

import (
	"context"
	"slices"
	"testing"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
)

func TestBuildProductionReadinessReportsDevShortcuts(t *testing.T) {
	repo := memory.New()
	cfg := config.Default()

	readiness := BuildProductionReadiness(context.Background(), cfg, repo)
	if readiness.SchemaVersion != "namros.production_readiness.v1" || readiness.Status != "blocked" {
		t.Fatalf("readiness envelope = %+v", readiness)
	}
	if readiness.MetadataBackend != config.MetadataBackendPebble || readiness.StorageBackend != config.StorageBackendMemory || readiness.CoordinationBackend != config.CoordinationBackendNone {
		t.Fatalf("readiness backends = %+v", readiness)
	}
	for _, claim := range []string{
		"deployment_profile_not_production",
		"metadata_backend_not_tikv",
		"coordination_backend_not_etcd",
		"storage_backend_not_sbs_volume_pool",
		"sbs_volume_pool_member_count_below_2",
		"sbs_writer_session_fencing_not_configured",
		"gc_candidate_queue_not_metadata",
	} {
		if !slices.Contains(readiness.UnsupportedClaims, claim) {
			t.Fatalf("unsupported claims = %v, want %q", readiness.UnsupportedClaims, claim)
		}
	}
}

func TestBuildProductionReadinessLoadsRegistryVolumePool(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	if _, err := repo.PutVolumePool(ctx, meta.PutVolumePoolRequest{
		PoolID: "object-pool",
		Members: []model.VolumePoolMember{
			{VolumeID: "18a00001", State: model.VolumePoolStateActive},
			{VolumeID: "18a00002", State: model.VolumePoolStateActive},
		},
	}); err != nil {
		t.Fatalf("PutVolumePool() error = %v", err)
	}

	cfg := config.Default()
	cfg.DeploymentProfile = config.DeploymentProfileProduction
	cfg.MetadataBackend = config.MetadataBackendTiKV
	cfg.StorageBackend = config.StorageBackendSBSPhysical
	cfg.SBSVolumePoolID = "object-pool"
	cfg.CoordinationBackend = config.CoordinationBackendNone
	cfg.GCCandidateQueue = config.GCCandidateQueueMetadata

	readiness := BuildProductionReadiness(ctx, cfg, repo)
	if readiness.SBSVolumePoolSource != "metadata_registry" || readiness.SBSVolumePoolID != "object-pool" || readiness.SBSVolumePoolMemberCount != 2 || readiness.SBSVolumePoolGeneration != 1 {
		t.Fatalf("volume pool readiness = %+v", readiness)
	}
	if !slices.Contains(readiness.UnsupportedClaims, "coordination_backend_not_etcd") {
		t.Fatalf("unsupported claims = %v, want coordination claim", readiness.UnsupportedClaims)
	}
	if slices.Contains(readiness.UnsupportedClaims, "sbs_volume_pool_member_count_below_2") {
		t.Fatalf("unsupported claims should not reject two-member pool: %v", readiness.UnsupportedClaims)
	}
}

func TestBuildReportsMetadataSchemaPosture(t *testing.T) {
	ctx := context.Background()
	repo := memory.New()
	cfg := config.Default()
	out, err := Build(ctx, cfg, repo, Request{CountLimit: 10})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if out.MetadataSchema.Status != meta.MetadataSchemaStatusCurrent || out.MetadataSchema.SchemaVersion != meta.CurrentMetadataSchemaVersion {
		t.Fatalf("metadata schema = %+v", out.MetadataSchema)
	}

	if _, err := repo.PutMetadataSchema(ctx, meta.PutMetadataSchemaRequest{
		SchemaVersion:    meta.CurrentMetadataSchemaVersion + 1,
		MinReaderVersion: meta.CurrentMetadataSchemaVersion + 1,
		MinWriterVersion: meta.CurrentMetadataSchemaVersion + 1,
		UpdatedBy:        "future-release",
	}); err != nil {
		t.Fatalf("PutMetadataSchema() error = %v", err)
	}
	out, err = Build(ctx, cfg, repo, Request{CountLimit: 10})
	if err != nil {
		t.Fatalf("Build(future schema) error = %v", err)
	}
	if out.MetadataSchema.Status != meta.MetadataSchemaStatusUnsupported || !out.MetadataSchema.UnsupportedFuture {
		t.Fatalf("future metadata schema = %+v", out.MetadataSchema)
	}
}

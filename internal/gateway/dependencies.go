package gateway

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nosway/namros/internal/config"
	dedupeworker "github.com/nosway/namros/internal/dedupe"
	"github.com/nosway/namros/internal/edition"
	"github.com/nosway/namros/internal/encryption"
	gcworker "github.com/nosway/namros/internal/gc"
	lifecycleworker "github.com/nosway/namros/internal/lifecycle"
	"github.com/nosway/namros/internal/meta"
	metacache "github.com/nosway/namros/internal/meta/cache"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	pebblemeta "github.com/nosway/namros/internal/meta/pebble"
	tikvmeta "github.com/nosway/namros/internal/meta/tikv"
	"github.com/nosway/namros/internal/opsauth"
	"github.com/nosway/namros/internal/opsmetrics"
	"github.com/nosway/namros/internal/sbsops"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storage/local"
	sbsegments "github.com/nosway/namros/internal/storage/sbs"
	"github.com/nosway/namros/internal/storage/volumepool"
	"github.com/nosway/namros/internal/workerscheduler"
)

type Dependencies struct {
	Metadata              meta.Repository
	Storage               storage.SegmentStore
	Orphans               storage.OrphanTracker
	Encryption            encryption.Provider
	TiKVMetrics           *tikvmeta.Metrics
	GatewayMetrics        *opsmetrics.GatewayMetrics
	SBSCollector          *sbsops.Collector
	ConsoleAuth           *opsauth.Manager
	GCMetrics             *gcworker.Metrics
	GCSchedulerStatus     *gcworker.SchedulerStatus
	DedupeMetrics         *dedupeworker.Metrics
	DedupeSchedulerStatus *dedupeworker.SchedulerStatus
	LifecycleStatus       *lifecycleworker.SchedulerStatus
	WorkerBudget          *workerscheduler.Budget
	AccessAudit           accessAuditRecorder
	CoordinationReadiness func(context.Context) error
	GatewayDrain          *GatewayDrainController
	requestLimiter        *requestLimiter
	bandwidthLimiter      *bandwidthLimiter
	sbsVolumePoolRuntime  *sbsVolumePoolRuntime
}

var openSBSPhysicalStorage = sbsegments.OpenPhysical
var openSBSECStorage = sbsegments.OpenEC
var openSBSClusterStorage = sbsegments.OpenCluster

func OpenDependencies(ctx context.Context, cfg config.Config) (Dependencies, func() error, error) {
	deps := Dependencies{}
	var cleanups []func() error
	sbsSessionCache := sbsegments.NewVolumeSessionCache()
	switch config.NormalizeMetadataBackend(cfg.MetadataBackend) {
	case config.MetadataBackendMemory:
		deps.Metadata = memory.New()
	case config.MetadataBackendPebble:
		repo, err := pebblemeta.Open(cfg.MetadataPath)
		if err != nil {
			return Dependencies{}, nil, err
		}
		deps.Metadata = repo
		cleanups = append(cleanups, repo.Close)
	case config.MetadataBackendTiKV:
		metrics := tikvmeta.NewMetrics()
		repo, err := tikvmeta.Open(ctx, tikvmeta.Config{
			PDEndpoints: cfg.TiKVPDEndpoints,
			APIVersion:  cfg.TiKVAPIVersion,
			Keyspace:    cfg.TiKVKeyspace,
			Timeout:     cfg.TiKVTimeout,
			TLS: tikvmeta.TLSConfig{
				CAPath:   cfg.TiKVTLSCA,
				CertPath: cfg.TiKVTLSCert,
				KeyPath:  cfg.TiKVTLSKey,
			},
			Retry: tikvmeta.RetryPolicy{
				MaxAttempts:    cfg.TiKVRetryAttempts,
				InitialBackoff: cfg.TiKVRetryInitialBackoff,
				MaxBackoff:     cfg.TiKVRetryMaxBackoff,
			},
			Metrics: metrics,
		})
		if err != nil {
			return Dependencies{}, nil, err
		}
		deps.Metadata = repo
		deps.TiKVMetrics = metrics
		cleanups = append(cleanups, repo.Close)
	default:
		return Dependencies{}, nil, fmt.Errorf("unsupported metadata backend %q", cfg.MetadataBackend)
	}
	if cfg.MetadataCacheTTL > 0 {
		deps.Metadata = metacache.New(deps.Metadata, cfg.MetadataCacheTTL)
	}
	if strings.TrimSpace(cfg.SBSVolumePoolID) != "" {
		var err error
		cfg, err = loadSBSVolumePoolFromRegistry(ctx, deps.Metadata, cfg)
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
	}
	deps.GatewayMetrics = opsmetrics.NewGatewayMetrics(opsmetrics.BuildInfoFromConfig(cfg))
	deps.WorkerBudget = workerBudgetFromConfig(cfg)
	deps.Metadata = newMetricsRepository(deps.Metadata, deps.GatewayMetrics)
	if config.NormalizeAccessAuditMode(cfg.AccessAuditMode) == config.AccessAuditModeAsync {
		recorder := newAsyncAccessAuditRecorder(deps.Metadata, accessAuditConfigFromApp(cfg))
		deps.AccessAudit = recorder
		cleanups = append(cleanups, recorder.Close)
	} else {
		deps.AccessAudit = syncAccessAuditRecorder{repo: deps.Metadata}
	}
	switch config.NormalizeStorageBackend(cfg.StorageBackend) {
	case config.StorageBackendMemory:
		segmentStore, cleanupFn, err := newEphemeralLocalStore()
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
		deps.Storage = segmentStore
		deps.Orphans = segmentStore
		cleanups = append(cleanups, cleanupFn)
	case config.StorageBackendLocal:
		segmentStore, err := local.New(cfg.StoragePath)
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
		deps.Storage = segmentStore
		deps.Orphans = segmentStore
	case config.StorageBackendSBS, config.StorageBackendSBSLocal:
		segmentStore, err := sbsegments.Open(ctx, sbsegments.Config{
			Path:            cfg.StoragePath,
			StatePath:       cfg.SBSStatePath,
			DeleteAdmission: protectedRefDeleteAdmission(deps.Metadata),
		})
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
		deps.Storage = segmentStore
		deps.Orphans = segmentStore
		cleanups = append(cleanups, segmentStore.Close)
	case config.StorageBackendSBSPhysical:
		if len(cfg.SBSVolumePool) > 0 {
			segmentStore, poolRuntime, cleanupFn, err := openSBSPhysicalVolumePool(ctx, cfg, deps.GatewayMetrics, protectedRefDeleteAdmission(deps.Metadata), sbsSessionCache)
			if err != nil {
				cleanupAll(cleanups)
				return Dependencies{}, nil, err
			}
			deps.Storage = segmentStore
			deps.Orphans = segmentStore
			deps.sbsVolumePoolRuntime = poolRuntime
			cleanups = append(cleanups, cleanupFn)
			if refresherCleanup := startSBSVolumePoolRegistryRefresher(ctx, deps.Metadata, cfg, poolRuntime); refresherCleanup != nil {
				cleanups = append(cleanups, refresherCleanup)
			}
			break
		}
		segmentStore, cleanupFn, err := openSBSPhysicalStorage(ctx, sbsegments.PhysicalOpenConfig{
			AdminEndpoint:              cfg.SBSAdminEndpoint,
			DataEndpoint:               cfg.SBSDataEndpoint,
			VolumeID:                   cfg.SBSVolumeID,
			ChunkSizeBytes:             cfg.SBSChunkSizeBytes,
			GatewayID:                  cfg.SBSGatewayID,
			AttachmentID:               cfg.SBSAttachmentID,
			Generation:                 cfg.SBSGeneration,
			SessionIdentity:            sbsSessionIdentityForSingleVolume(cfg),
			SessionCache:               sbsSessionCache,
			VerifyReadback:             cfg.SBSVerifyReadback,
			WriteConcurrency:           cfg.SBSPhysicalWriteConcurrency,
			FullChunkWriteMinBytes:     cfg.SBSPhysicalFullChunkWriteMinBytes,
			FullChunkWriteMaxBytes:     cfg.SBSPhysicalFullChunkWriteMaxBytes,
			ChunkCacheBytes:            cfg.SBSPhysicalChunkCacheBytes,
			ChunkIDAllocationCacheSize: cfg.SBSChunkIDAllocationCacheSize,
			Metrics:                    deps.GatewayMetrics,
			DeleteAdmission:            protectedRefDeleteAdmission(deps.Metadata),
		})
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
		deps.Storage = segmentStore
		deps.Orphans = segmentStore
		cleanups = append(cleanups, cleanupFn)
	case config.StorageBackendSBSEC:
		if len(cfg.SBSVolumePool) > 0 {
			segmentStore, poolRuntime, cleanupFn, err := openSBSECVolumePool(ctx, cfg, deps.GatewayMetrics, protectedRefDeleteAdmission(deps.Metadata), sbsSessionCache)
			if err != nil {
				cleanupAll(cleanups)
				return Dependencies{}, nil, err
			}
			deps.Storage = segmentStore
			deps.Orphans = segmentStore
			deps.sbsVolumePoolRuntime = poolRuntime
			cleanups = append(cleanups, cleanupFn)
			if refresherCleanup := startSBSVolumePoolRegistryRefresher(ctx, deps.Metadata, cfg, poolRuntime); refresherCleanup != nil {
				cleanups = append(cleanups, refresherCleanup)
			}
			break
		}
		segmentStore, cleanupFn, err := openSBSECStorage(ctx, sbsegments.ECOpenConfig{
			DataEndpoint:     cfg.SBSDataEndpoint,
			VolumeID:         cfg.SBSVolumeID,
			GatewayID:        cfg.SBSGatewayID,
			AttachmentID:     cfg.SBSAttachmentID,
			Generation:       cfg.SBSGeneration,
			SessionIdentity:  sbsSessionIdentityForSingleVolume(cfg),
			SessionCache:     sbsSessionCache,
			ShardStoreIDs:    cfg.SBSShardStoreIDs,
			ShardConcurrency: cfg.SBSECShardConcurrency,
			Metrics:          deps.GatewayMetrics,
			DeleteAdmission:  protectedRefDeleteAdmission(deps.Metadata),
		})
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
		deps.Storage = segmentStore
		deps.Orphans = segmentStore
		cleanups = append(cleanups, cleanupFn)
	case config.StorageBackendSBSCluster:
		if len(cfg.SBSVolumePool) > 0 {
			segmentStore, poolRuntime, cleanupFn, err := openSBSClusterVolumePool(ctx, cfg, deps.GatewayMetrics, protectedRefDeleteAdmission(deps.Metadata), sbsSessionCache)
			if err != nil {
				cleanupAll(cleanups)
				return Dependencies{}, nil, err
			}
			deps.Storage = segmentStore
			deps.Orphans = segmentStore
			deps.sbsVolumePoolRuntime = poolRuntime
			cleanups = append(cleanups, cleanupFn)
			if refresherCleanup := startSBSVolumePoolRegistryRefresher(ctx, deps.Metadata, cfg, poolRuntime); refresherCleanup != nil {
				cleanups = append(cleanups, refresherCleanup)
			}
			break
		}
		sessionIdentity := sbsSessionIdentityForSingleVolume(cfg)
		clusterCfg := sbsegments.ClusterOpenConfig{
			AdminEndpoint:              cfg.SBSAdminEndpoint,
			DataEndpoint:               cfg.SBSDataEndpoint,
			VolumeID:                   cfg.SBSVolumeID,
			ChunkSizeBytes:             cfg.SBSChunkSizeBytes,
			GatewayID:                  cfg.SBSGatewayID,
			AttachmentID:               cfg.SBSAttachmentID,
			Generation:                 cfg.SBSGeneration,
			SessionIdentity:            sessionIdentity,
			SessionCache:               sbsSessionCache,
			VerifyReadback:             cfg.SBSVerifyReadback,
			WriteConcurrency:           cfg.SBSPhysicalWriteConcurrency,
			FullChunkWriteMinBytes:     cfg.SBSPhysicalFullChunkWriteMinBytes,
			FullChunkWriteMaxBytes:     cfg.SBSPhysicalFullChunkWriteMaxBytes,
			ChunkCacheBytes:            cfg.SBSPhysicalChunkCacheBytes,
			ChunkIDAllocationCacheSize: cfg.SBSChunkIDAllocationCacheSize,
			Metrics:                    deps.GatewayMetrics,
			ECMetrics:                  deps.GatewayMetrics,
			ShardStoreIDs:              cfg.SBSShardStoreIDs,
			ECShardConcurrency:         cfg.SBSECShardConcurrency,
			DeleteAdmission:            protectedRefDeleteAdmission(deps.Metadata),
		}
		var segmentStore storage.SegmentStore
		var cleanupFn func() error
		var err error
		if edition.Allows(cfg.Edition, edition.FeatureErasureCoding) {
			segmentStore, cleanupFn, err = openSBSClusterStorage(ctx, clusterCfg)
		} else {
			segmentStore, cleanupFn, err = openSBSPhysicalStorage(ctx, sbsegments.PhysicalOpenConfig{
				AdminEndpoint:              clusterCfg.AdminEndpoint,
				DataEndpoint:               clusterCfg.DataEndpoint,
				VolumeID:                   clusterCfg.VolumeID,
				ChunkSizeBytes:             clusterCfg.ChunkSizeBytes,
				GatewayID:                  clusterCfg.GatewayID,
				AttachmentID:               clusterCfg.AttachmentID,
				Generation:                 clusterCfg.Generation,
				SessionIdentity:            sessionIdentity,
				SessionCache:               sbsSessionCache,
				VerifyReadback:             clusterCfg.VerifyReadback,
				WriteConcurrency:           clusterCfg.WriteConcurrency,
				FullChunkWriteMinBytes:     clusterCfg.FullChunkWriteMinBytes,
				FullChunkWriteMaxBytes:     clusterCfg.FullChunkWriteMaxBytes,
				ChunkCacheBytes:            clusterCfg.ChunkCacheBytes,
				ChunkIDAllocationCacheSize: clusterCfg.ChunkIDAllocationCacheSize,
				Metrics:                    deps.GatewayMetrics,
				DeleteAdmission:            clusterCfg.DeleteAdmission,
			})
		}
		if err != nil {
			cleanupAll(cleanups)
			return Dependencies{}, nil, err
		}
		orphanTracker, ok := segmentStore.(storage.OrphanTracker)
		if !ok {
			cleanupAll(cleanups)
			return Dependencies{}, nil, fmt.Errorf("sbs-cluster storage does not support orphan tracking")
		}
		deps.Storage = segmentStore
		deps.Orphans = orphanTracker
		cleanups = append(cleanups, cleanupFn)
	default:
		cleanupAll(cleanups)
		return Dependencies{}, nil, fmt.Errorf("unsupported storage backend %q", cfg.StorageBackend)
	}
	if err := configureGCCandidateQueue(&deps, cfg); err != nil {
		cleanupAll(cleanups)
		return Dependencies{}, nil, err
	}
	instrumentStorageDependencies(&deps, cfg)
	deps.GCMetrics = gcworker.NewMetrics()
	deps.SBSCollector = sbsops.NewCollector(sbsops.ConfigFromApp(cfg))
	if capacityCleanup := startSBSVolumePoolCapacityRefresher(ctx, deps.SBSCollector, cfg, deps.sbsVolumePoolRuntime); capacityCleanup != nil {
		cleanups = append(cleanups, capacityCleanup)
	}
	consoleAuth, err := opsauth.New(opsauth.ConfigFromApp(cfg))
	if err != nil {
		cleanupAll(cleanups)
		return Dependencies{}, nil, err
	}
	deps.ConsoleAuth = consoleAuth
	deps.GCSchedulerStatus = gcworker.NewSchedulerStatus()
	deps.DedupeMetrics = dedupeworker.NewMetrics()
	deps.DedupeSchedulerStatus = dedupeworker.NewSchedulerStatus()
	deps.LifecycleStatus = lifecycleworker.NewSchedulerStatus()
	deps.GatewayDrain = NewGatewayDrainController()
	return deps, func() error {
		return cleanupAll(cleanups)
	}, nil
}

func openSBSPhysicalVolumePool(ctx context.Context, cfg config.Config, metrics *opsmetrics.GatewayMetrics, deleteAdmission storage.DeleteAdmissionFunc, sessionCache *sbsegments.VolumeSessionCache) (*volumepool.Store, *sbsVolumePoolRuntime, func() error, error) {
	return openSBSVolumePool(ctx, cfg, metrics, func(ctx context.Context, member config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error) {
		return openSBSPhysicalStorage(ctx, sbsegments.PhysicalOpenConfig{
			AdminEndpoint:              member.AdminEndpoint,
			DataEndpoint:               member.DataEndpoint,
			VolumeID:                   member.VolumeID,
			ChunkSizeBytes:             member.ChunkSizeBytes,
			GatewayID:                  member.GatewayID,
			AttachmentID:               member.AttachmentID,
			Generation:                 member.Generation,
			SessionIdentity:            sbsSessionIdentityForMember(cfg, member),
			SessionCache:               sessionCache,
			VerifyReadback:             member.VerifyReadback,
			WriteConcurrency:           member.WriteConcurrency,
			FullChunkWriteMinBytes:     cfg.SBSPhysicalFullChunkWriteMinBytes,
			FullChunkWriteMaxBytes:     cfg.SBSPhysicalFullChunkWriteMaxBytes,
			ChunkCacheBytes:            cfg.SBSPhysicalChunkCacheBytes,
			ChunkIDAllocationCacheSize: cfg.SBSChunkIDAllocationCacheSize,
			Metrics:                    metrics,
			DeleteAdmission:            deleteAdmission,
		})
	})
}

func openSBSECVolumePool(ctx context.Context, cfg config.Config, metrics *opsmetrics.GatewayMetrics, deleteAdmission storage.DeleteAdmissionFunc, sessionCache *sbsegments.VolumeSessionCache) (*volumepool.Store, *sbsVolumePoolRuntime, func() error, error) {
	return openSBSVolumePool(ctx, cfg, metrics, func(ctx context.Context, member config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error) {
		return openSBSECStorage(ctx, sbsegments.ECOpenConfig{
			DataEndpoint:     member.DataEndpoint,
			VolumeID:         member.VolumeID,
			GatewayID:        member.GatewayID,
			AttachmentID:     member.AttachmentID,
			Generation:       member.Generation,
			SessionIdentity:  sbsSessionIdentityForMember(cfg, member),
			SessionCache:     sessionCache,
			ShardStoreIDs:    member.ShardStoreIDs,
			ShardConcurrency: cfg.SBSECShardConcurrency,
			Metrics:          metrics,
			DeleteAdmission:  deleteAdmission,
		})
	})
}

func openSBSClusterVolumePool(ctx context.Context, cfg config.Config, metrics *opsmetrics.GatewayMetrics, deleteAdmission storage.DeleteAdmissionFunc, sessionCache *sbsegments.VolumeSessionCache) (*volumepool.Store, *sbsVolumePoolRuntime, func() error, error) {
	return openSBSVolumePool(ctx, cfg, metrics, func(ctx context.Context, member config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error) {
		sessionIdentity := sbsSessionIdentityForMember(cfg, member)
		clusterCfg := sbsegments.ClusterOpenConfig{
			AdminEndpoint:              member.AdminEndpoint,
			DataEndpoint:               member.DataEndpoint,
			VolumeID:                   member.VolumeID,
			ChunkSizeBytes:             member.ChunkSizeBytes,
			GatewayID:                  member.GatewayID,
			AttachmentID:               member.AttachmentID,
			Generation:                 member.Generation,
			SessionIdentity:            sessionIdentity,
			SessionCache:               sessionCache,
			VerifyReadback:             member.VerifyReadback,
			WriteConcurrency:           member.WriteConcurrency,
			FullChunkWriteMinBytes:     cfg.SBSPhysicalFullChunkWriteMinBytes,
			FullChunkWriteMaxBytes:     cfg.SBSPhysicalFullChunkWriteMaxBytes,
			ChunkCacheBytes:            cfg.SBSPhysicalChunkCacheBytes,
			ChunkIDAllocationCacheSize: cfg.SBSChunkIDAllocationCacheSize,
			Metrics:                    metrics,
			ECMetrics:                  metrics,
			ShardStoreIDs:              member.ShardStoreIDs,
			ECShardConcurrency:         cfg.SBSECShardConcurrency,
			DeleteAdmission:            deleteAdmission,
		}
		if edition.Allows(cfg.Edition, edition.FeatureErasureCoding) {
			return openSBSClusterStorage(ctx, clusterCfg)
		}
		return openSBSPhysicalStorage(ctx, sbsegments.PhysicalOpenConfig{
			AdminEndpoint:              clusterCfg.AdminEndpoint,
			DataEndpoint:               clusterCfg.DataEndpoint,
			VolumeID:                   clusterCfg.VolumeID,
			ChunkSizeBytes:             clusterCfg.ChunkSizeBytes,
			GatewayID:                  clusterCfg.GatewayID,
			AttachmentID:               clusterCfg.AttachmentID,
			Generation:                 clusterCfg.Generation,
			SessionIdentity:            sessionIdentity,
			SessionCache:               sessionCache,
			VerifyReadback:             clusterCfg.VerifyReadback,
			WriteConcurrency:           clusterCfg.WriteConcurrency,
			FullChunkWriteMinBytes:     clusterCfg.FullChunkWriteMinBytes,
			FullChunkWriteMaxBytes:     clusterCfg.FullChunkWriteMaxBytes,
			ChunkCacheBytes:            clusterCfg.ChunkCacheBytes,
			ChunkIDAllocationCacheSize: clusterCfg.ChunkIDAllocationCacheSize,
			Metrics:                    metrics,
			DeleteAdmission:            clusterCfg.DeleteAdmission,
		})
	})
}

func openSBSVolumePool(ctx context.Context, cfg config.Config, metrics *opsmetrics.GatewayMetrics, opener sbsVolumePoolMemberOpener) (*volumepool.Store, *sbsVolumePoolRuntime, func() error, error) {
	runtime, err := openSBSVolumePoolRuntime(ctx, cfg, opener, metrics)
	if err != nil {
		return nil, nil, nil, err
	}
	return runtime.Store(), runtime, runtime.Close, nil
}

func loadSBSVolumePoolFromRegistry(ctx context.Context, repo meta.Repository, cfg config.Config) (config.Config, error) {
	poolID := strings.TrimSpace(cfg.SBSVolumePoolID)
	pool, err := repo.GetVolumePool(ctx, poolID)
	if err != nil {
		return config.Config{}, fmt.Errorf("load sbs volume pool %q: %w", poolID, err)
	}
	if len(pool.Members) == 0 {
		return config.Config{}, fmt.Errorf("%w: sbs volume pool %q has no members", storage.ErrInvalidArgument, poolID)
	}
	return applySBSVolumePoolRegistrySnapshot(cfg, pool)
}

func sbsVolumePoolMemberFromRegistry(member model.VolumePoolMember) config.SBSVolumePoolMember {
	return config.SBSVolumePoolMember{
		VolumeID:             member.VolumeID,
		AdminEndpoint:        member.AdminEndpoint,
		DataEndpoint:         member.DataEndpoint,
		GatewayID:            member.GatewayID,
		AttachmentID:         member.AttachmentID,
		Generation:           member.Generation,
		ChunkSizeBytes:       member.ChunkSizeBytes,
		ReadOnly:             member.ReadOnly,
		State:                string(member.State),
		Weight:               member.Weight,
		AvailableBytes:       member.AvailableBytes,
		UsedPercent:          member.UsedPercent,
		HighWatermarkPercent: member.HighWatermarkPercent,
	}
}

func inheritSBSVolumePoolMember(cfg config.Config, member config.SBSVolumePoolMember, poolSize int) config.SBSVolumePoolMember {
	if member.AdminEndpoint == "" {
		member.AdminEndpoint = cfg.SBSAdminEndpoint
	}
	if member.DataEndpoint == "" {
		member.DataEndpoint = cfg.SBSDataEndpoint
	}
	if member.GatewayID == "" {
		member.GatewayID = cfg.SBSGatewayID
	}
	if member.Generation == 0 {
		member.Generation = cfg.SBSGeneration
	}
	if member.WriterGroupID == "" {
		member.WriterGroupID = cfg.SBSWriterGroupID
	}
	if member.VolumeEpoch == 0 {
		member.VolumeEpoch = cfg.SBSVolumeEpoch
	}
	if member.ChunkSizeBytes == 0 {
		member.ChunkSizeBytes = cfg.SBSChunkSizeBytes
	}
	if len(member.ShardStoreIDs) == 0 {
		member.ShardStoreIDs = append([]string(nil), cfg.SBSShardStoreIDs...)
	}
	if !member.VerifyReadback {
		member.VerifyReadback = cfg.SBSVerifyReadback
	}
	if member.WriteConcurrency == 0 {
		member.WriteConcurrency = cfg.SBSPhysicalWriteConcurrency
	}
	if member.AttachmentID == "" {
		member.AttachmentID = cfg.SBSAttachmentID
		if member.AttachmentID != "" && poolSize > 1 && !strings.Contains(member.AttachmentID, "{volume_id}") {
			member.AttachmentID += "-" + member.VolumeID
		}
	}
	member.AttachmentID = strings.ReplaceAll(member.AttachmentID, "{volume_id}", member.VolumeID)
	return member
}

func sbsSessionIdentityForSingleVolume(cfg config.Config) sbsegments.SessionIdentity {
	return sbsegments.SessionIdentity{
		PoolID:            cfg.SBSVolumePoolID,
		PoolGeneration:    cfg.SBSVolumePoolGeneration,
		MemberGeneration:  cfg.SBSGeneration,
		VolumeEpoch:       cfg.SBSVolumeEpoch,
		WriterGroupID:     cfg.SBSWriterGroupID,
		GatewayID:         cfg.SBSGatewayID,
		GatewayInstanceID: cfg.GatewayInstanceID,
		SessionID:         cfg.SBSSessionID,
		SessionTTL:        cfg.SBSSessionTTL,
		HeartbeatInterval: cfg.SBSSessionHeartbeat,
	}
}

func sbsSessionIdentityForMember(cfg config.Config, member config.SBSVolumePoolMember) sbsegments.SessionIdentity {
	return sbsegments.SessionIdentity{
		PoolID:            cfg.SBSVolumePoolID,
		PoolGeneration:    cfg.SBSVolumePoolGeneration,
		VolumeID:          member.VolumeID,
		MemberGeneration:  member.Generation,
		VolumeEpoch:       member.VolumeEpoch,
		WriterGroupID:     member.WriterGroupID,
		GatewayID:         member.GatewayID,
		GatewayInstanceID: cfg.GatewayInstanceID,
		SessionID:         cfg.SBSSessionID,
		SessionTTL:        cfg.SBSSessionTTL,
		HeartbeatInterval: cfg.SBSSessionHeartbeat,
	}
}

func (d Dependencies) withDefaults(cfg config.Config) Dependencies {
	if d.Metadata == nil {
		d.Metadata = memory.New()
	}
	if d.Storage == nil {
		segmentStore, _, err := newEphemeralLocalStore()
		if err != nil {
			panic(err)
		}
		d.Storage = segmentStore
		if d.Orphans == nil {
			d.Orphans = segmentStore
		}
	}
	if d.GatewayMetrics == nil {
		d.GatewayMetrics = opsmetrics.NewGatewayMetrics(opsmetrics.BuildInfo{})
	}
	if d.WorkerBudget == nil {
		d.WorkerBudget = workerBudgetFromConfig(cfg)
	}
	if d.requestLimiter == nil {
		d.requestLimiter = newRequestLimiter(cfg)
	}
	if d.bandwidthLimiter == nil {
		d.bandwidthLimiter = newBandwidthLimiter(cfg)
	}
	d.Metadata = newMetricsRepository(d.Metadata, d.GatewayMetrics)
	if d.AccessAudit == nil {
		d.AccessAudit = syncAccessAuditRecorder{repo: d.Metadata}
	}
	if d.GatewayDrain == nil {
		d.GatewayDrain = NewGatewayDrainController()
	}
	if d.Orphans == nil {
		if orphanTracker, ok := d.Storage.(storage.OrphanTracker); ok {
			d.Orphans = orphanTracker
		}
	}
	if err := configureGCCandidateQueue(&d, cfg); err != nil {
		panic(err)
	}
	instrumentStorageDependencies(&d, cfg)
	if d.SBSCollector == nil {
		d.SBSCollector = sbsops.NewCollector(sbsops.Config{})
	}
	if d.ConsoleAuth == nil {
		manager, err := opsauth.New(opsauth.ConfigFromApp(cfg))
		if err != nil {
			panic(err)
		}
		d.ConsoleAuth = manager
	}
	return d
}

func workerBudgetFromConfig(cfg config.Config) *workerscheduler.Budget {
	return workerscheduler.NewBudget(workerscheduler.BudgetConfig{
		MaxConcurrent:          cfg.BackgroundWorkerMaxConcurrent,
		MaxConcurrentPerTenant: cfg.BackgroundWorkerMaxConcurrentPerTenant,
		MaxConcurrentPerPool:   cfg.BackgroundWorkerMaxConcurrentPerPool,
	})
}

func backgroundWorkerBudgetScope(cfg config.Config) workerscheduler.BudgetScope {
	return workerscheduler.BudgetScope{
		PoolID: backgroundWorkerPoolID(cfg),
	}
}

func backgroundWorkerPoolID(cfg config.Config) string {
	if poolID := strings.TrimSpace(cfg.SBSVolumePoolID); poolID != "" {
		return poolID
	}
	if len(cfg.SBSVolumePool) > 0 {
		return "static"
	}
	return ""
}

func instrumentStorageDependencies(deps *Dependencies, cfg config.Config) {
	if deps == nil || deps.Storage == nil || deps.GatewayMetrics == nil {
		return
	}
	if _, ok := deps.Storage.(*metricsSegmentStore); ok {
		return
	}
	wrapped := newMetricsSegmentStore(deps.Storage, deps.GatewayMetrics, config.NormalizeStorageBackend(cfg.StorageBackend))
	deps.Storage = wrapped
	if config.NormalizeGCCandidateQueue(cfg.GCCandidateQueue) == config.GCCandidateQueueStorage {
		deps.Orphans = wrapped
	}
}

func configureGCCandidateQueue(deps *Dependencies, cfg config.Config) error {
	switch config.NormalizeGCCandidateQueue(cfg.GCCandidateQueue) {
	case config.GCCandidateQueueStorage:
		return nil
	case config.GCCandidateQueueMetadata:
		if deps.Metadata == nil {
			return errors.New("metadata repository is required for metadata gc candidate queue")
		}
		deps.Orphans = gcworker.MetadataOrphanTracker{Store: deps.Metadata}
		return nil
	default:
		return fmt.Errorf("unsupported gc candidate queue %q", cfg.GCCandidateQueue)
	}
}

func newEphemeralLocalStore() (*local.Store, func() error, error) {
	root, err := os.MkdirTemp("", "namros-segments-*")
	if err != nil {
		return nil, nil, err
	}
	segmentStore, err := local.New(root)
	if err != nil {
		return nil, nil, errors.Join(err, os.RemoveAll(root))
	}
	return segmentStore, func() error {
		return os.RemoveAll(root)
	}, nil
}

func cleanupAll(cleanups []func() error) error {
	var err error
	for i := len(cleanups) - 1; i >= 0; i-- {
		err = errors.Join(err, cleanups[i]())
	}
	return err
}

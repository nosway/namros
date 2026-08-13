package gateway

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nosway/namros/internal/adminstatus"
	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/auth/credentials"
	"github.com/nosway/namros/internal/auth/sigv4"
	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/coordination"
	dedupeworker "github.com/nosway/namros/internal/dedupe"
	gcworker "github.com/nosway/namros/internal/gc"
	lifecycleworker "github.com/nosway/namros/internal/lifecycle"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/s3api/s3err"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/trace"
	"github.com/nosway/namros/internal/version"
	"github.com/nosway/namros/internal/workerops"
	"github.com/nosway/namros/internal/workerscheduler"
)

const (
	requestIDHeader = s3err.RequestIDHeader
	hostIDHeader    = s3err.HostIDHeader
)

func NewHandler(cfg config.Config) http.Handler {
	return NewHandlerWithDeps(cfg, Dependencies{})
}

func NewHandlerWithDeps(cfg config.Config, deps Dependencies) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(gin.Recovery())
	router.Use(requestHeadersMiddleware())
	deps = deps.withDefaults(cfg)
	router.Use(corsResponseMiddleware(deps))
	router.Use(s3MetricsMiddleware(deps.GatewayMetrics))
	router.GET("/healthz", healthz)
	readyHandler := readyz(cfg, deps)
	router.GET("/readyz", readyHandler)
	router.HEAD("/readyz", readyHandler)
	router.GET("/metrics", prometheusMetricsHandler(deps))
	router.GET("/debug/config", debugConfig(cfg))
	router.GET("/debug/tikv/metrics", debugTiKVMetrics(deps))
	router.GET("/debug/gc/metrics", debugGCMetrics(deps))
	router.GET("/debug/dedupe/metrics", debugDedupeMetrics(deps))
	router.GET("/debug/dedupe/status", debugDedupeStatus(deps))
	router.GET("/debug/operations/metrics", debugOperationsMetrics(deps))
	router.GET("/debug/admin/status", debugAdminStatus(cfg, deps))
	router.GET("/debug/gateway/drain", debugGatewayDrainStatus(deps))
	router.GET("/debug/sbs/volume-pool", debugSBSVolumePoolStatus(deps))
	router.GET("/debug/gateways", debugGatewayRegistry(cfg))
	registerConsoleAPI(router, cfg, deps)
	registerConsoleStatic(router, deps.ConsoleAuth)

	authenticator := newAuthenticator(cfg)
	s3 := s3Handler{cfg: cfg, deps: deps, dataBudget: newDataBudget(cfg), requestLimiter: deps.requestLimiter, bandwidthLimiter: deps.bandwidthLimiter}
	router.Any("/", authMiddleware(authenticator), s3.handle)
	router.Any("/:bucket", authMiddleware(authenticator), s3.handle)
	router.Any("/:bucket/*key", authMiddleware(authenticator), s3.handle)
	router.NoRoute(authMiddleware(authenticator), s3.handle)
	router.NoMethod(authMiddleware(authenticator), methodNotAllowed)
	return router
}

func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	deps, closeDeps, err := OpenDependencies(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := closeDeps(); err != nil {
			logger.Error("gateway dependency cleanup failed", "error", err)
		}
	}()
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           NewHandlerWithDeps(cfg, deps),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	registryErrCh := make(chan error, 1)
	schedulerErrCh := make(chan error, 1)
	runtimeGatewayCfg := coordination.GatewayConfigFromApp(cfg)
	if coordination.Enabled(cfg) {
		go func() {
			registryErrCh <- coordination.RunGatewayRegistry(runCtx, runtimeGatewayCfg, logger)
		}()
	}
	if cfg.DedupeSchedulerEnabled {
		go func() {
			err := runDedupeScheduler(runCtx, cfg, runtimeGatewayCfg.InstanceID, deps, logger)
			if err != nil {
				schedulerErrCh <- err
			}
		}()
	}
	if cfg.GCWorkerEnabled {
		go func() {
			err := runGCScheduler(runCtx, cfg, runtimeGatewayCfg.InstanceID, deps, logger)
			if err != nil {
				schedulerErrCh <- err
			}
		}()
	}
	if cfg.LifecycleWorkerEnabled {
		go func() {
			err := runLifecycleScheduler(runCtx, cfg, runtimeGatewayCfg.InstanceID, deps, logger)
			if err != nil {
				schedulerErrCh <- err
			}
		}()
	}
	go func() {
		if config.NormalizeDeploymentProfile(cfg.DeploymentProfile) == config.DeploymentProfileProduction && cfg.AllowUnsafeProductionShortcuts {
			logger.Warn("unsafe production shortcuts enabled", startupLogAttrs(cfg)...)
		}
		logger.Info("starting namros gateway", startupLogAttrs(cfg)...)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		if deps.GatewayDrain != nil {
			status := deps.GatewayDrain.StartDrain()
			logger.Info("gateway drain started", "state", status.State, "in_flight_writes", status.InFlightWrites)
		}
		cancelRun()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		if deps.GatewayDrain != nil {
			if err := deps.GatewayDrain.WaitDrained(shutdownCtx); err != nil {
				return err
			}
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		cancelRun()
		return err
	case err := <-registryErrCh:
		cancelRun()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil && err == nil {
			return shutdownErr
		}
		if err == nil {
			return nil
		}
		return err
	case err := <-schedulerErrCh:
		cancelRun()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := server.Shutdown(shutdownCtx); shutdownErr != nil && err == nil {
			return shutdownErr
		}
		if err == nil {
			return nil
		}
		return err
	}
}

func startupLogAttrs(cfg config.Config) []any {
	return []any{
		"listen", cfg.ListenAddr,
		"deployment_profile", config.NormalizeDeploymentProfile(cfg.DeploymentProfile),
		"allow_unsafe_production_shortcuts", cfg.AllowUnsafeProductionShortcuts,
		"metadata_backend", config.NormalizeMetadataBackend(cfg.MetadataBackend),
		"storage_backend", config.NormalizeStorageBackend(cfg.StorageBackend),
		"coordination_backend", config.NormalizeCoordinationBackend(cfg.CoordinationBackend),
		"gateway_data_budget_bytes", cfg.GatewayDataBudgetBytes,
		"gateway_data_budget_max_requests", cfg.GatewayDataBudgetMaxRequests,
		"gateway_data_budget_unknown_bytes", cfg.GatewayDataBudgetUnknownBytes,
		"gateway_request_max_concurrent", cfg.GatewayRequestMaxConcurrent,
		"gateway_request_max_concurrent_per_tenant", cfg.GatewayRequestMaxConcurrentPerTenant,
		"gateway_request_max_concurrent_reads", cfg.GatewayRequestMaxConcurrentReads,
		"gateway_request_max_concurrent_writes", cfg.GatewayRequestMaxConcurrentWrites,
		"gateway_upload_bandwidth_bytes_per_second", cfg.GatewayUploadBandwidthBytesPerSecond,
		"gateway_download_bandwidth_bytes_per_second", cfg.GatewayDownloadBandwidthBytesPerSecond,
		"background_worker_max_concurrent", cfg.BackgroundWorkerMaxConcurrent,
		"background_worker_max_concurrent_per_tenant", cfg.BackgroundWorkerMaxConcurrentPerTenant,
		"background_worker_max_concurrent_per_pool", cfg.BackgroundWorkerMaxConcurrentPerPool,
		"sbs_volume_pool_id", cfg.SBSVolumePoolID,
		"sbs_volume_pool_members", len(cfg.SBSVolumePool),
	}
}

func runDedupeScheduler(ctx context.Context, cfg config.Config, ownerID string, deps Dependencies, logger *slog.Logger) error {
	schedulerLogger := logger
	if schedulerLogger == nil {
		schedulerLogger = slog.Default()
	}
	scheduler := dedupeworker.Scheduler{
		Worker: dedupeworker.BackgroundWorker{
			Repository:     deps.Metadata,
			Storage:        deps.Storage,
			Orphans:        deps.Orphans,
			OperationStore: deps.Metadata,
			Metrics:        deps.DedupeMetrics,
		},
		LockStore: deps.Metadata,
		Status:    deps.DedupeSchedulerStatus,
		Budget:    deps.WorkerBudget,
		BudgetScope: workerscheduler.BudgetScope{
			TenantID: cfg.DedupeSchedulerTenantID,
			PoolID:   backgroundWorkerPoolID(cfg),
		},
		Config: dedupeworker.SchedulerConfig{
			OwnerID:  ownerID,
			LockTTL:  cfg.DedupeSchedulerLockTTL,
			Interval: cfg.DedupeSchedulerInterval,
			Request: dedupeworker.RunRequest{
				Scan: dedupeworker.ScanRequest{
					TenantID:     cfg.DedupeSchedulerTenantID,
					BucketID:     cfg.DedupeSchedulerBucketID,
					Prefix:       cfg.DedupeSchedulerPrefix,
					MaxKeys:      cfg.DedupeSchedulerMaxKeys,
					Limit:        cfg.DedupeSchedulerLimit,
					Enabled:      true,
					Mode:         dedupeworker.Mode(cfg.DedupeSchedulerMode),
					VerifyBytes:  cfg.DedupeSchedulerVerifyBytes,
					ByteVerified: !cfg.DedupeSchedulerVerifyBytes,
				},
			},
		},
		Logger: schedulerLogger.With("component", "dedupe.scheduler"),
	}
	return scheduler.Run(ctx)
}

func runGCScheduler(ctx context.Context, cfg config.Config, ownerID string, deps Dependencies, logger *slog.Logger) error {
	schedulerLogger := logger
	if schedulerLogger == nil {
		schedulerLogger = slog.Default()
	}
	scheduler := gcworker.Scheduler{
		Worker: gcworker.Worker{
			Storage: deps.Storage,
			Orphans: deps.Orphans,
			Admission: func(ctx context.Context, ref storage.SegmentRef) error {
				admission := admitProtectedRefSegmentDelete(ctx, deps.Metadata, ref)
				if admission.Err != nil {
					return admission.Err
				}
				if !admission.Admitted {
					return storage.ErrProtected
				}
				return nil
			},
			OperationStore:       deps.Metadata,
			SharedObjectReleases: deps.Metadata,
			Metrics:              deps.GCMetrics,
		},
		Repository:  deps.Metadata,
		Status:      deps.GCSchedulerStatus,
		Budget:      deps.WorkerBudget,
		BudgetScope: backgroundWorkerBudgetScope(cfg),
		Config: gcworker.SchedulerConfig{
			OwnerID:  ownerID,
			ShardID:  cfg.GCWorkerShardID,
			LeaseTTL: cfg.GCWorkerLeaseTTL,
			Interval: cfg.GCWorkerInterval,
			Limit:    cfg.GCWorkerLimit,
		},
		Logger: schedulerLogger.With("component", "gc.scheduler"),
	}
	return scheduler.Run(ctx)
}

func runLifecycleScheduler(ctx context.Context, cfg config.Config, ownerID string, deps Dependencies, logger *slog.Logger) error {
	schedulerLogger := logger
	if schedulerLogger == nil {
		schedulerLogger = slog.Default()
	}
	scheduler := lifecycleworker.Scheduler{
		Worker: lifecycleworker.Worker{
			Metadata: deps.Metadata,
			Storage:  deps.Storage,
			Orphans:  deps.Orphans,
		},
		Repository:  deps.Metadata,
		Status:      deps.LifecycleStatus,
		Budget:      deps.WorkerBudget,
		BudgetScope: backgroundWorkerBudgetScope(cfg),
		Config: lifecycleworker.SchedulerConfig{
			OwnerID:    ownerID,
			ShardID:    cfg.LifecycleWorkerShardID,
			BucketID:   cfg.LifecycleWorkerBucketID,
			LeaseTTL:   cfg.LifecycleWorkerLeaseTTL,
			Interval:   cfg.LifecycleWorkerInterval,
			MaxKeys:    cfg.LifecycleWorkerMaxKeys,
			MaxUploads: cfg.LifecycleWorkerMaxUploads,
		},
		Logger: schedulerLogger.With("component", "lifecycle.scheduler"),
	}
	return scheduler.Run(ctx)
}

func healthz(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

type readinessComponent struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

type readinessResponse struct {
	SchemaVersion string               `json:"schema_version"`
	Status        string               `json:"status"`
	GeneratedAt   string               `json:"generated_at"`
	Components    []readinessComponent `json:"components"`
}

func readyz(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), readinessCheckTimeout(cfg))
		defer cancel()

		components := []readinessComponent{
			checkMetadataReadiness(ctx, deps),
			checkStorageReadiness(ctx, deps),
			checkCoordinationReadiness(ctx, cfg, deps),
			checkGatewayDrainReadiness(deps),
		}
		status := "ready"
		httpStatus := http.StatusOK
		for _, component := range components {
			if component.Status == "not_ready" {
				status = "not_ready"
				httpStatus = http.StatusServiceUnavailable
				break
			}
		}
		response := readinessResponse{
			SchemaVersion: "namros.gateway.readiness.v1",
			Status:        status,
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
			Components:    components,
		}
		if c.Request.Method == http.MethodHead {
			c.Status(httpStatus)
			return
		}
		c.JSON(httpStatus, response)
	}
}

func readinessCheckTimeout(cfg config.Config) time.Duration {
	timeout := 2 * time.Second
	for _, candidate := range []time.Duration{cfg.TiKVTimeout, cfg.EtcdDialTimeout} {
		if candidate > 0 && candidate < timeout {
			timeout = candidate
		}
	}
	return timeout
}

func checkMetadataReadiness(ctx context.Context, deps Dependencies) readinessComponent {
	component := readinessComponent{Name: "metadata", Status: "ready", Reason: "ok"}
	if deps.Metadata == nil {
		component.Status = "not_ready"
		component.Reason = "metadata_missing"
		return component
	}
	if _, err := deps.Metadata.ListBuckets(ctx, "root"); err != nil {
		component.Status = "not_ready"
		component.Reason = "metadata_unavailable"
		return component
	}
	schema := meta.CheckMetadataSchema(ctx, deps.Metadata)
	switch schema.Status {
	case meta.MetadataSchemaStatusUnsupported:
		component.Status = "not_ready"
		component.Reason = "metadata_schema_unsupported_future"
	case meta.MetadataSchemaStatusMigration:
		component.Status = "warning"
		component.Reason = "metadata_schema_migration_required"
	case meta.MetadataSchemaStatusError:
		component.Status = "not_ready"
		component.Reason = "metadata_schema_unavailable"
	}
	return component
}

func checkStorageReadiness(ctx context.Context, deps Dependencies) readinessComponent {
	component := readinessComponent{Name: "storage", Status: "ready", Reason: "ok"}
	if deps.Storage == nil {
		component.Status = "not_ready"
		component.Reason = "storage_missing"
		return component
	}
	if checker, ok := deps.Storage.(storage.HealthChecker); ok {
		if err := checker.CheckHealth(ctx); err != nil {
			component.Status = "not_ready"
			component.Reason = "storage_unavailable"
		}
	}
	return component
}

func checkCoordinationReadiness(ctx context.Context, cfg config.Config, deps Dependencies) readinessComponent {
	component := readinessComponent{Name: "coordination", Status: "skipped", Reason: "coordination_disabled"}
	if !coordination.Enabled(cfg) {
		return component
	}
	component.Status = "ready"
	component.Reason = "ok"
	if deps.CoordinationReadiness != nil {
		if err := deps.CoordinationReadiness(ctx); err != nil {
			component.Status = "not_ready"
			component.Reason = "coordination_unavailable"
		}
		return component
	}
	if _, err := coordination.ListGatewayRecords(ctx, coordination.GatewayConfigFromApp(cfg)); err != nil {
		component.Status = "not_ready"
		component.Reason = "coordination_unavailable"
	}
	return component
}

func checkGatewayDrainReadiness(deps Dependencies) readinessComponent {
	component := readinessComponent{Name: "gateway_drain", Status: "ready", Reason: string(GatewayDrainActive)}
	if deps.GatewayDrain == nil {
		component.Status = "skipped"
		component.Reason = "drain_controller_missing"
		return component
	}
	status := deps.GatewayDrain.Status()
	if status.State != GatewayDrainActive {
		component.Status = "not_ready"
		component.Reason = string(status.State)
	}
	return component
}

func debugGatewayDrainStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := GatewayDrainStatus{State: GatewayDrainActive}
		if deps.GatewayDrain != nil {
			status = deps.GatewayDrain.Status()
		}
		c.JSON(http.StatusOK, gin.H{
			"schema_version":   "namros.gateway.drain.v1",
			"state":            status.State,
			"in_flight_writes": status.InFlightWrites,
		})
	}
}

func debugConfig(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"config":  cfg,
			"version": version.Info(),
		})
	}
}

func debugGatewayRegistry(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		gatewayCfg := coordination.GatewayConfigFromApp(cfg)
		prefix := coordination.GatewayRegistryPrefix(gatewayCfg.RegistryPrefix)
		backend := config.NormalizeCoordinationBackend(gatewayCfg.Backend)
		if !coordination.Enabled(cfg) {
			c.JSON(http.StatusOK, gin.H{
				"schema_version":   "namros.gateway.registry.v1",
				"enabled":          false,
				"backend":          backend,
				"registry_prefix":  prefix,
				"gateway_count":    0,
				"healthy_count":    0,
				"gateways":         []coordination.GatewayRecord{},
				"healthy_gateways": []coordination.GatewayRecord{},
			})
			return
		}

		timeout := cfg.EtcdDialTimeout + cfg.GatewayHeartbeat
		if timeout <= 0 {
			timeout = 3 * time.Second
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		records, err := coordination.ListGatewayRecords(ctx, gatewayCfg)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"schema_version":  "namros.gateway.registry.v1",
				"enabled":         true,
				"backend":         backend,
				"registry_prefix": prefix,
				"error":           err.Error(),
			})
			return
		}
		now := time.Now().UTC()
		healthy := coordination.HealthyGatewayRecords(records, now, coordination.GatewayMaxHeartbeatAge(gatewayCfg))
		c.JSON(http.StatusOK, gin.H{
			"schema_version":   "namros.gateway.registry.v1",
			"generated_at":     now.Format(time.RFC3339),
			"enabled":          true,
			"backend":          backend,
			"registry_prefix":  prefix,
			"gateway_count":    len(records),
			"healthy_count":    len(healthy),
			"gateways":         records,
			"healthy_gateways": healthy,
		})
	}
}

func debugTiKVMetrics(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.TiKVMetrics == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":  true,
			"snapshot": deps.TiKVMetrics.Snapshot(),
		})
	}
}

func debugGCMetrics(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.GCMetrics == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":  true,
			"snapshot": deps.GCMetrics.Snapshot(),
		})
	}
}

func debugDedupeMetrics(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.DedupeMetrics == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"enabled":  true,
			"snapshot": deps.DedupeMetrics.Snapshot(),
		})
	}
}

type operationsMetricsOutput struct {
	SchemaVersion int                         `json:"schema_version"`
	GeneratedAt   string                      `json:"generated_at"`
	Status        string                      `json:"status"`
	Components    operationsMetricsComponents `json:"components"`
}

type operationsMetricsComponents struct {
	Gateway         operationsMetricComponent `json:"gateway"`
	TiKV            operationsMetricComponent `json:"tikv"`
	GC              operationsMetricComponent `json:"gc"`
	GCScheduler     operationsMetricComponent `json:"gc_scheduler"`
	Lifecycle       operationsMetricComponent `json:"lifecycle"`
	Dedupe          operationsMetricComponent `json:"dedupe"`
	DedupeScheduler operationsMetricComponent `json:"dedupe_scheduler"`
	WorkerBudget    operationsMetricComponent `json:"worker_budget"`
	WorkerBacklog   operationsMetricComponent `json:"worker_backlog"`
	SBSVolumePool   operationsMetricComponent `json:"sbs_volume_pool"`
}

type operationsMetricComponent struct {
	Enabled  bool   `json:"enabled"`
	Snapshot any    `json:"snapshot,omitempty"`
	Error    string `json:"error,omitempty"`
}

func debugOperationsMetrics(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, buildOperationsMetrics(c.Request.Context(), deps))
	}
}

func buildOperationsMetrics(ctx context.Context, deps Dependencies) operationsMetricsOutput {
	output := operationsMetricsOutput{
		SchemaVersion: 1,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Status:        "ok",
		Components: operationsMetricsComponents{
			Gateway:         operationsMetricComponent{Enabled: deps.GatewayMetrics != nil},
			TiKV:            operationsMetricComponent{Enabled: deps.TiKVMetrics != nil},
			GC:              operationsMetricComponent{Enabled: deps.GCMetrics != nil},
			GCScheduler:     operationsMetricComponent{Enabled: deps.GCSchedulerStatus != nil},
			Lifecycle:       operationsMetricComponent{Enabled: deps.LifecycleStatus != nil},
			Dedupe:          operationsMetricComponent{Enabled: deps.DedupeMetrics != nil},
			DedupeScheduler: operationsMetricComponent{Enabled: deps.DedupeSchedulerStatus != nil},
			WorkerBudget:    operationsMetricComponent{Enabled: deps.WorkerBudget != nil},
			WorkerBacklog:   operationsMetricComponent{Enabled: deps.Metadata != nil},
			SBSVolumePool:   operationsMetricComponent{Enabled: deps.sbsVolumePoolRuntime != nil},
		},
	}
	if deps.GatewayMetrics != nil {
		output.Components.Gateway.Snapshot = deps.GatewayMetrics.Snapshot()
	}
	if deps.TiKVMetrics != nil {
		output.Components.TiKV.Snapshot = deps.TiKVMetrics.Snapshot()
	}
	if deps.GCMetrics != nil {
		output.Components.GC.Snapshot = deps.GCMetrics.Snapshot()
	}
	if deps.GCSchedulerStatus != nil {
		output.Components.GCScheduler.Snapshot = deps.GCSchedulerStatus.Snapshot()
	}
	if deps.LifecycleStatus != nil {
		output.Components.Lifecycle.Snapshot = deps.LifecycleStatus.Snapshot()
	}
	if deps.DedupeMetrics != nil {
		output.Components.Dedupe.Snapshot = deps.DedupeMetrics.Snapshot()
	}
	if deps.DedupeSchedulerStatus != nil {
		output.Components.DedupeScheduler.Snapshot = deps.DedupeSchedulerStatus.Snapshot()
	}
	if deps.WorkerBudget != nil {
		output.Components.WorkerBudget.Snapshot = deps.WorkerBudget.Snapshot()
	}
	if deps.Metadata != nil {
		snapshot, err := workerops.Summarize(ctx, deps.Metadata, workerops.Config{})
		if err != nil {
			output.Status = "partial"
			output.Components.WorkerBacklog.Error = err.Error()
		} else {
			output.Components.WorkerBacklog.Snapshot = snapshot
			if deps.GatewayMetrics != nil {
				deps.GatewayMetrics.SetWorkerBacklog(snapshot)
			}
		}
	}
	if deps.sbsVolumePoolRuntime != nil {
		output.Components.SBSVolumePool.Snapshot = deps.sbsVolumePoolRuntime.Status()
	}
	return output
}

func debugSBSVolumePoolStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.sbsVolumePoolRuntime == nil {
			c.JSON(http.StatusOK, disabledSBSVolumePoolRuntimeStatus(time.Now().UTC()))
			return
		}
		c.JSON(http.StatusOK, deps.sbsVolumePoolRuntime.Status())
	}
}

func debugDedupeStatus(deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Metadata == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		limit := debugLimit(c, 20, 100)
		operations, err := deps.Metadata.ListDedupeOperations(c.Request.Context(), meta.ListDedupeOperationsRequest{
			Limit: limit,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"enabled": true,
				"error":   err.Error(),
			})
			return
		}
		body := gin.H{
			"enabled":    true,
			"limit":      limit,
			"operations": dedupeDebugOperations(operations),
		}
		if deps.DedupeMetrics != nil {
			body["metrics"] = deps.DedupeMetrics.Snapshot()
		}
		if deps.DedupeSchedulerStatus != nil {
			body["scheduler"] = deps.DedupeSchedulerStatus.Snapshot()
		}
		c.JSON(http.StatusOK, body)
	}
}

func debugAdminStatus(cfg config.Config, deps Dependencies) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deps.Metadata == nil {
			c.JSON(http.StatusOK, gin.H{"enabled": false})
			return
		}
		countLimit := debugNamedLimit(c, "count_limit", 1000, 10000)
		recentDedupeLimit := debugNamedLimit(c, "recent_dedupe_limit", 5, 100)
		recentGCLimit := debugNamedLimit(c, "recent_gc_limit", 5, 100)
		output, err := adminstatus.Build(c.Request.Context(), cfg, deps.Metadata, adminstatus.Request{
			CountLimit:        countLimit,
			RecentDedupeLimit: recentDedupeLimit,
			RecentGCLimit:     recentGCLimit,
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"schema_version": 1,
				"status":         "error",
				"error":          err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, output)
	}
}

type dedupeDebugOperation struct {
	OperationID         string                      `json:"operation_id"`
	ResumeOfOperationID string                      `json:"resume_of_operation_id,omitempty"`
	Status              model.DedupeOperationStatus `json:"status"`
	StartedAt           time.Time                   `json:"started_at"`
	FinishedAt          time.Time                   `json:"finished_at"`
	Scanned             int                         `json:"scanned"`
	Acked               int                         `json:"acked"`
	Skipped             int                         `json:"skipped"`
	Retryable           int                         `json:"retryable"`
	Attempts            int                         `json:"attempts"`
	CreatedAt           time.Time                   `json:"created_at"`
}

func dedupeDebugOperations(records []model.DedupeOperationRecord) []dedupeDebugOperation {
	if len(records) == 0 {
		return nil
	}
	out := make([]dedupeDebugOperation, 0, len(records))
	for _, record := range records {
		out = append(out, dedupeDebugOperation{
			OperationID:         record.OperationID,
			ResumeOfOperationID: record.ResumeOfOperationID,
			Status:              record.Status,
			StartedAt:           record.StartedAt,
			FinishedAt:          record.FinishedAt,
			Scanned:             record.Scanned,
			Acked:               record.Acked,
			Skipped:             record.Skipped,
			Retryable:           record.Retryable,
			Attempts:            len(record.Attempts),
			CreatedAt:           record.CreatedAt,
		})
	}
	return out
}

func debugLimit(c *gin.Context, defaultLimit, maxLimit int) int {
	return debugNamedLimit(c, "limit", defaultLimit, maxLimit)
}

func debugNamedLimit(c *gin.Context, name string, defaultLimit, maxLimit int) int {
	limit := defaultLimit
	if raw := c.Query(name); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}

func newAuthenticator(cfg config.Config) *sigv4.Verifier {
	root, err := credentials.NewRootCredential(cfg.RootAccessKeyID, cfg.RootSecretAccessKey)
	if err != nil {
		panic(err)
	}
	store, err := credentials.NewStaticStore(root)
	if err != nil {
		panic(err)
	}
	return sigv4.NewVerifier(sigv4.Config{
		Region:      cfg.Region,
		Credentials: store,
	})
}

func authMiddleware(verifier *sigv4.Verifier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		result, err := verifier.Verify(c.Request.Context(), c.Request)
		if err != nil {
			writeS3Error(c, authError(err))
			return
		}
		ctx := auth.WithPrincipal(c.Request.Context(), result.Principal)
		ctx = auth.WithPolicyDecision(ctx, auth.PolicyDecision{
			Allowed:       true,
			Source:        "sigv4",
			Reason:        "signature verified",
			Principal:     result.Principal.Clone(),
			PolicyVersion: result.Principal.PolicyVersion,
		})
		c.Request = c.Request.WithContext(ctx)
		c.Set("sigv4_canonical_request", result.CanonicalRequest)
		c.Next()
	}
}

func authError(err error) s3err.Error {
	switch {
	case errors.Is(err, sigv4.ErrAccessDenied):
		return s3err.AccessDenied(err.Error())
	case errors.Is(err, sigv4.ErrSignatureDoesNotMatch):
		return s3err.SignatureDoesNotMatch(err.Error())
	case errors.Is(err, sigv4.ErrRequestTimeTooSkewed), errors.Is(err, sigv4.ErrExpiredPresign):
		return s3err.RequestTimeTooSkewed(err.Error())
	default:
		return s3err.InvalidRequest(err.Error())
	}
}

func methodNotAllowed(c *gin.Context) {
	writeS3Error(c, s3err.MethodNotAllowed("method is not allowed for this resource"))
}

func writeS3Error(c *gin.Context, err s3err.Error) {
	if err.Code != "" {
		c.Set(s3MetricsErrorCodeKey, err.Code)
	}
	s3err.Write(c.Writer, c.Request, err)
	c.Abort()
}

func requestHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := requestID(c.Request)
		c.Request = c.Request.WithContext(trace.WithRequestID(c.Request.Context(), reqID))
		c.Header(requestIDHeader, reqID)
		c.Header(hostIDHeader, "namros-bootstrap")
		c.Next()
	}
}

func requestID(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		host = "local"
	}
	return time.Now().UTC().Format("20060102T150405.000000000") + "-" + host
}

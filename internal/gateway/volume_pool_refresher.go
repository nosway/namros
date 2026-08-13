package gateway

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/opsmetrics"
	"github.com/nosway/namros/internal/sbsops"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storage/volumepool"
)

type sbsVolumePoolMemberOpener func(context.Context, config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error)

type sbsVolumePoolRuntime struct {
	mu       sync.Mutex
	cfg      config.Config
	opener   sbsVolumePoolMemberOpener
	metrics  *opsmetrics.GatewayMetrics
	store    *volumepool.Store
	cleanups map[string]func() error
	status   sbsVolumePoolRuntimeStatus
}

type sbsVolumePoolRuntimeStatus struct {
	SchemaVersion            string  `json:"schema_version"`
	GeneratedAt              string  `json:"generated_at"`
	Enabled                  bool    `json:"enabled"`
	Source                   string  `json:"source"`
	PoolID                   string  `json:"pool_id,omitempty"`
	ConfiguredGeneration     uint64  `json:"configured_generation"`
	ActiveGeneration         uint64  `json:"active_generation"`
	MemberCount              int     `json:"member_count"`
	RefreshEnabled           bool    `json:"refresh_enabled"`
	RefreshIntervalSeconds   float64 `json:"refresh_interval_seconds"`
	RefreshCount             uint64  `json:"refresh_count"`
	RefreshErrorCount        uint64  `json:"refresh_error_count"`
	LastRefreshAt            string  `json:"last_refresh_at,omitempty"`
	LastSuccessAt            string  `json:"last_success_at,omitempty"`
	LastErrorAt              string  `json:"last_error_at,omitempty"`
	LastError                string  `json:"last_error,omitempty"`
	Stale                    bool    `json:"stale"`
	StaleDurationSeconds     float64 `json:"stale_duration_seconds"`
	CapacityRefreshCount     uint64  `json:"capacity_refresh_count,omitempty"`
	CapacityObservationCount int     `json:"capacity_observation_count,omitempty"`
	CapacitySource           string  `json:"capacity_source,omitempty"`
	LastCapacityRefreshAt    string  `json:"last_capacity_refresh_at,omitempty"`
	staleSince               time.Time
	lastRefreshAt            time.Time
	lastSuccessAt            time.Time
	lastErrorAt              time.Time
	lastCapacityRefreshAt    time.Time
}

func openSBSVolumePoolRuntime(ctx context.Context, cfg config.Config, opener sbsVolumePoolMemberOpener, metrics *opsmetrics.GatewayMetrics) (*sbsVolumePoolRuntime, error) {
	members, cleanups, err := openSBSVolumePoolMembers(ctx, cfg, opener)
	if err != nil {
		return nil, err
	}
	store, err := volumepool.New(members)
	if err != nil {
		_ = cleanupVolumePoolMemberStores(cleanups)
		return nil, err
	}
	now := time.Now().UTC()
	runtime := &sbsVolumePoolRuntime{
		cfg:      cfg,
		opener:   opener,
		metrics:  metrics,
		store:    store,
		cleanups: cleanups,
		status: sbsVolumePoolRuntimeStatus{
			SchemaVersion:          "namros.gateway.sbs.volume_pool.runtime.v1",
			Enabled:                true,
			Source:                 sbsVolumePoolSource(cfg),
			PoolID:                 strings.TrimSpace(cfg.SBSVolumePoolID),
			ConfiguredGeneration:   cfg.SBSVolumePoolGeneration,
			ActiveGeneration:       cfg.SBSVolumePoolGeneration,
			MemberCount:            len(cfg.SBSVolumePool),
			RefreshEnabled:         strings.TrimSpace(cfg.SBSVolumePoolID) != "" && cfg.SBSVolumePoolRefreshInterval > 0,
			RefreshIntervalSeconds: cfg.SBSVolumePoolRefreshInterval.Seconds(),
			lastSuccessAt:          now,
		},
	}
	runtime.emitStatus(now)
	return runtime, nil
}

func (r *sbsVolumePoolRuntime) Store() *volumepool.Store {
	if r == nil {
		return nil
	}
	return r.store
}

func (r *sbsVolumePoolRuntime) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	cleanups := r.cleanups
	r.cleanups = nil
	r.mu.Unlock()
	return cleanupVolumePoolMemberStores(cleanups)
}

func (r *sbsVolumePoolRuntime) Status() sbsVolumePoolRuntimeStatus {
	return r.statusAt(time.Now().UTC())
}

func (r *sbsVolumePoolRuntime) statusAt(now time.Time) sbsVolumePoolRuntimeStatus {
	if r == nil {
		return disabledSBSVolumePoolRuntimeStatus(now)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked(now)
}

func (r *sbsVolumePoolRuntime) statusLocked(now time.Time) sbsVolumePoolRuntimeStatus {
	status := r.status
	status.GeneratedAt = now.Format(time.RFC3339Nano)
	status.LastRefreshAt = formatSBSVolumePoolRuntimeTime(status.lastRefreshAt)
	status.LastSuccessAt = formatSBSVolumePoolRuntimeTime(status.lastSuccessAt)
	status.LastErrorAt = formatSBSVolumePoolRuntimeTime(status.lastErrorAt)
	status.LastCapacityRefreshAt = formatSBSVolumePoolRuntimeTime(status.lastCapacityRefreshAt)
	status.Stale = status.ConfiguredGeneration != status.ActiveGeneration || status.LastError != ""
	if status.Stale {
		if status.staleSince.IsZero() {
			status.staleSince = now
		}
		status.StaleDurationSeconds = now.Sub(status.staleSince).Seconds()
		if status.StaleDurationSeconds < 0 {
			status.StaleDurationSeconds = 0
		}
	} else {
		status.StaleDurationSeconds = 0
	}
	status.staleSince = time.Time{}
	status.lastRefreshAt = time.Time{}
	status.lastSuccessAt = time.Time{}
	status.lastErrorAt = time.Time{}
	status.lastCapacityRefreshAt = time.Time{}
	return status
}

func disabledSBSVolumePoolRuntimeStatus(now time.Time) sbsVolumePoolRuntimeStatus {
	return sbsVolumePoolRuntimeStatus{
		SchemaVersion: "namros.gateway.sbs.volume_pool.runtime.v1",
		GeneratedAt:   now.Format(time.RFC3339Nano),
		Enabled:       false,
		Source:        "not_configured",
	}
}

func (r *sbsVolumePoolRuntime) RefreshFromRegistry(ctx context.Context, repo meta.Repository) error {
	if r == nil || repo == nil {
		return nil
	}
	r.mu.Lock()
	base := r.cfg
	currentGeneration := r.status.ActiveGeneration
	r.mu.Unlock()
	poolID := strings.TrimSpace(base.SBSVolumePoolID)
	if poolID == "" {
		return nil
	}
	now := time.Now().UTC()
	pool, err := repo.GetVolumePool(ctx, poolID)
	if err != nil {
		err = fmt.Errorf("refresh sbs volume pool %q: %w", poolID, err)
		r.recordRefresh(now, 0, err)
		return err
	}
	if pool.Generation != 0 && pool.Generation == currentGeneration {
		r.recordRefresh(now, pool.Generation, nil)
		return nil
	}
	nextCfg, err := applySBSVolumePoolRegistrySnapshot(base, pool)
	if err != nil {
		r.recordRefresh(now, pool.Generation, err)
		return err
	}
	members, cleanups, err := openSBSVolumePoolMembers(ctx, nextCfg, r.opener)
	if err != nil {
		r.recordRefresh(now, pool.Generation, err)
		return err
	}
	if err := r.store.UpdateMembers(members); err != nil {
		_ = cleanupVolumePoolMemberStores(cleanups)
		r.recordRefresh(now, pool.Generation, err)
		return err
	}
	r.mu.Lock()
	oldCleanups := r.cleanups
	r.cfg = nextCfg
	r.cleanups = cleanups
	r.status.ActiveGeneration = pool.Generation
	r.status.MemberCount = len(nextCfg.SBSVolumePool)
	r.mu.Unlock()
	r.recordRefresh(now, pool.Generation, nil)
	return cleanupVolumePoolMemberStores(oldCleanups)
}

func (r *sbsVolumePoolRuntime) recordRefresh(now time.Time, configuredGeneration uint64, err error) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if configuredGeneration != 0 {
		r.status.ConfiguredGeneration = configuredGeneration
	}
	r.status.lastRefreshAt = now
	r.status.RefreshCount++
	if err != nil {
		r.status.RefreshErrorCount++
		r.status.lastErrorAt = now
		r.status.LastError = err.Error()
		if r.status.staleSince.IsZero() {
			r.status.staleSince = now
		}
	} else {
		r.status.LastError = ""
		r.status.lastErrorAt = time.Time{}
		r.status.lastSuccessAt = now
		if r.status.ConfiguredGeneration == r.status.ActiveGeneration {
			r.status.staleSince = time.Time{}
		} else if r.status.staleSince.IsZero() {
			r.status.staleSince = now
		}
	}
	r.mu.Unlock()
	r.emitStatus(now)
}

func (r *sbsVolumePoolRuntime) emitStatus(now time.Time) {
	if r == nil || r.metrics == nil {
		return
	}
	status := r.statusAt(now)
	r.metrics.SetSBSVolumePoolStatus(opsmetrics.SBSVolumePoolObservation{
		PoolID:               status.PoolID,
		Source:               status.Source,
		ConfiguredGeneration: status.ConfiguredGeneration,
		ActiveGeneration:     status.ActiveGeneration,
		RefreshErrorCount:    status.RefreshErrorCount,
		StaleSeconds:         status.StaleDurationSeconds,
	})
}

func (r *sbsVolumePoolRuntime) ApplyCapacitySnapshot(snapshot sbsops.Snapshot) int {
	if r == nil || r.store == nil {
		return 0
	}
	observations := sbsVolumePoolCapacityObservations(snapshot)
	updated := r.store.UpdateMemberObservations(observations)
	r.recordCapacityRefresh(time.Now().UTC(), snapshot.SourceAuthority, updated)
	return updated
}

func (r *sbsVolumePoolRuntime) recordCapacityRefresh(now time.Time, source string, updated int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.status.CapacityRefreshCount++
	r.status.CapacityObservationCount = updated
	r.status.CapacitySource = strings.TrimSpace(source)
	r.status.lastCapacityRefreshAt = now
	r.mu.Unlock()
	r.emitStatus(now)
}

func startSBSVolumePoolRegistryRefresher(parent context.Context, repo meta.Repository, cfg config.Config, runtime *sbsVolumePoolRuntime) func() error {
	if repo == nil || runtime == nil || strings.TrimSpace(cfg.SBSVolumePoolID) == "" || cfg.SBSVolumePoolRefreshInterval <= 0 {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cfg.SBSVolumePoolRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = runtime.RefreshFromRegistry(ctx, repo)
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		return nil
	}
}

func startSBSVolumePoolCapacityRefresher(parent context.Context, collector *sbsops.Collector, cfg config.Config, runtime *sbsVolumePoolRuntime) func() error {
	if collector == nil || runtime == nil || cfg.SBSVolumePoolRefreshInterval <= 0 || strings.TrimSpace(cfg.NAMRBDSBSObservabilityEndpoint) == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.ApplyCapacitySnapshot(collector.Snapshot(ctx))
		ticker := time.NewTicker(cfg.SBSVolumePoolRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				runtime.ApplyCapacitySnapshot(collector.Snapshot(ctx))
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() error {
		cancel()
		<-done
		return nil
	}
}

func applySBSVolumePoolRegistrySnapshot(cfg config.Config, pool model.VolumePool) (config.Config, error) {
	if len(pool.Members) == 0 {
		return config.Config{}, fmt.Errorf("%w: sbs volume pool %q has no members", storage.ErrInvalidArgument, pool.PoolID)
	}
	members := make([]config.SBSVolumePoolMember, 0, len(pool.Members))
	for _, member := range pool.Members {
		members = append(members, sbsVolumePoolMemberFromRegistry(member))
	}
	cfg.SBSVolumePool = members
	cfg.SBSVolumePoolGeneration = pool.Generation
	cfg.SBSVolumeID = members[0].VolumeID
	return cfg, nil
}

func sbsVolumePoolSource(cfg config.Config) string {
	if strings.TrimSpace(cfg.SBSVolumePoolID) != "" {
		return "metadata_registry"
	}
	if len(cfg.SBSVolumePool) > 0 {
		return "static_config"
	}
	return "not_configured"
}

func formatSBSVolumePoolRuntimeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func sbsVolumePoolCapacityObservations(snapshot sbsops.Snapshot) []volumepool.MemberObservation {
	raw := sbsops.VolumeCapacityObservations(snapshot)
	out := make([]volumepool.MemberObservation, 0, len(raw))
	for _, observation := range raw {
		out = append(out, volumepool.MemberObservation{
			VolumeID:       observation.VolumeID,
			AvailableBytes: observation.AvailableBytes,
			UsedPercent:    observation.UsedPercent,
		})
	}
	return out
}

func openSBSVolumePoolMembers(ctx context.Context, cfg config.Config, opener sbsVolumePoolMemberOpener) ([]volumepool.Member, map[string]func() error, error) {
	poolMembers := make([]volumepool.Member, 0, len(cfg.SBSVolumePool))
	cleanups := make(map[string]func() error, len(cfg.SBSVolumePool))
	for _, rawMember := range cfg.SBSVolumePool {
		member := inheritSBSVolumePoolMember(cfg, rawMember, len(cfg.SBSVolumePool))
		segmentStore, cleanupFn, err := opener(ctx, member)
		if err != nil {
			_ = cleanupVolumePoolMemberStores(cleanups)
			return nil, nil, err
		}
		cleanups[member.VolumeID] = cleanupFn
		poolMembers = append(poolMembers, volumepool.Member{
			VolumeID:             member.VolumeID,
			Store:                segmentStore,
			ReadOnly:             member.ReadOnly,
			State:                member.State,
			Weight:               member.Weight,
			AvailableBytes:       member.AvailableBytes,
			UsedPercent:          member.UsedPercent,
			HighWatermarkPercent: member.HighWatermarkPercent,
		})
	}
	return poolMembers, cleanups, nil
}

func cleanupVolumePoolMemberStores(cleanups map[string]func() error) error {
	if len(cleanups) == 0 {
		return nil
	}
	fns := make([]func() error, 0, len(cleanups))
	for _, cleanup := range cleanups {
		if cleanup != nil {
			fns = append(fns, cleanup)
		}
	}
	return cleanupAll(fns)
}

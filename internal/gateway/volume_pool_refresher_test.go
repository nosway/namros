package gateway

import (
	"context"
	"errors"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/opsmetrics"
	"github.com/nosway/namros/internal/sbsops"
	"github.com/nosway/namros/internal/storage"
	"github.com/nosway/namros/internal/storage/volumepool"
)

func TestSBSVolumePoolRuntimeStatusTracksFailedRegistryRefresh(t *testing.T) {
	cfg := config.Default()
	cfg.SBSVolumePoolID = "object-pool"
	cfg.SBSVolumePoolGeneration = 3
	cfg.SBSVolumePoolRefreshInterval = time.Second
	cfg.SBSVolumePool = []config.SBSVolumePoolMember{{VolumeID: "18a00001"}}
	metrics := opsmetrics.NewGatewayMetrics(opsmetrics.BuildInfo{})
	openErr := errors.New("member open rejected")
	runtime, err := openSBSVolumePoolRuntime(t.Context(), cfg, func(_ctx context.Context, member config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error) {
		if member.VolumeID == "18a00002" {
			return nil, nil, openErr
		}
		return runtimeTestSegmentStore{}, func() error { return nil }, nil
	}, metrics)
	if err != nil {
		t.Fatalf("openSBSVolumePoolRuntime() error = %v", err)
	}
	defer runtime.Close()

	repo := memory.New()
	if _, err := repo.PutVolumePool(t.Context(), meta.PutVolumePoolRequest{
		PoolID:     "object-pool",
		Generation: 4,
		Members: []model.VolumePoolMember{
			{VolumeID: "18a00002", State: model.VolumePoolStateActive},
		},
	}); err != nil {
		t.Fatalf("PutVolumePool() error = %v", err)
	}
	if err := runtime.RefreshFromRegistry(t.Context(), repo); !errors.Is(err, openErr) {
		t.Fatalf("RefreshFromRegistry() error = %v, want %v", err, openErr)
	}

	status := runtime.Status()
	if status.ConfiguredGeneration != 4 || status.ActiveGeneration != 3 {
		t.Fatalf("generations = configured:%d active:%d, want 4/3", status.ConfiguredGeneration, status.ActiveGeneration)
	}
	if !status.Stale || status.RefreshErrorCount != 1 || status.LastError == "" {
		t.Fatalf("stale/error status = %+v", status)
	}
	if status.RefreshCount != 1 || status.MemberCount != 1 || !status.RefreshEnabled {
		t.Fatalf("refresh status = %+v", status)
	}
	assertGatewayGauge(t, metrics.Gatherer(), "namros_gateway_sbs_volume_pool_configured_generation", 4)
	assertGatewayGauge(t, metrics.Gatherer(), "namros_gateway_sbs_volume_pool_active_generation", 3)
	assertGatewayGauge(t, metrics.Gatherer(), "namros_gateway_sbs_volume_pool_refresh_errors", 1)
}

func TestDebugSBSVolumePoolStatusEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.SBSVolumePoolID = "object-pool"
	cfg.SBSVolumePoolGeneration = 9
	cfg.SBSVolumePoolRefreshInterval = time.Second
	cfg.SBSVolumePool = []config.SBSVolumePoolMember{{VolumeID: "18a00001"}}
	runtime, err := openSBSVolumePoolRuntime(t.Context(), cfg, func(_ctx context.Context, _ config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error) {
		return runtimeTestSegmentStore{}, func() error { return nil }, nil
	}, nil)
	if err != nil {
		t.Fatalf("openSBSVolumePoolRuntime() error = %v", err)
	}
	defer runtime.Close()

	handler := NewHandlerWithDeps(config.Default(), Dependencies{
		Metadata:             memory.New(),
		sbsVolumePoolRuntime: runtime,
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/sbs/volume-pool", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{`"enabled":true`, `"pool_id":"object-pool"`, `"configured_generation":9`, `"active_generation":9`} {
		if !strings.Contains(body, want) {
			t.Fatalf("debug body missing %s: %s", want, body)
		}
	}

	operations := buildOperationsMetrics(context.Background(), Dependencies{sbsVolumePoolRuntime: runtime})
	if !operations.Components.SBSVolumePool.Enabled || operations.Components.SBSVolumePool.Snapshot == nil {
		t.Fatalf("operations volume pool component = %+v", operations.Components.SBSVolumePool)
	}
}

func TestSBSVolumePoolRuntimeAppliesCapacitySnapshotWithoutChangingRegistryState(t *testing.T) {
	cfg := config.Default()
	cfg.SBSVolumePoolID = "object-pool"
	cfg.SBSVolumePoolGeneration = 3
	cfg.SBSVolumePoolRefreshInterval = time.Second
	cfg.SBSVolumePool = []config.SBSVolumePoolMember{
		{VolumeID: "18a00001", State: "active", AvailableBytes: 3},
		{VolumeID: "18a00002", State: "draining", AvailableBytes: 3},
	}
	runtime, err := openSBSVolumePoolRuntime(t.Context(), cfg, func(_ctx context.Context, _ config.SBSVolumePoolMember) (storage.SegmentStore, func() error, error) {
		return runtimeTestSegmentStore{}, func() error { return nil }, nil
	}, nil)
	if err != nil {
		t.Fatalf("openSBSVolumePoolRuntime() error = %v", err)
	}
	defer runtime.Close()
	if plan := runtime.Store().PlanWrite(volumepool.WriteAdmissionRequest{SizeBytes: 7}); plan.Admitted {
		t.Fatalf("initial PlanWrite() = %+v, want rejected by configured capacity", plan)
	}

	updated := runtime.ApplyCapacitySnapshot(sbsops.Snapshot{
		SourceAuthority: "namrbd_sbs_observability",
		VolumeDetails: []sbsops.VolumeStatus{
			{VolumeID: "18a00001", AvailableBytes: 128, UsedPercent: 10, AvailableBytesObserved: true, UsedPercentObserved: true, CapacityObserved: true},
			{VolumeID: "18a00002", AvailableBytes: 128, UsedPercent: 10, AvailableBytesObserved: true, UsedPercentObserved: true, CapacityObserved: true},
		},
	})
	if updated != 2 {
		t.Fatalf("ApplyCapacitySnapshot() = %d, want 2", updated)
	}
	ref, err := runtime.Store().PutSegment(t.Context(), storage.PutSegmentRequest{Reader: strings.NewReader("payload"), SizeBytes: 7})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	if got := ref.Placement.Parameters["volume_id"]; got != "18a00001" {
		t.Fatalf("PutSegment() volume = %q, want active member 18a00001", got)
	}
	plan := runtime.Store().PlanWrite(volumepool.WriteAdmissionRequest{SizeBytes: 7})
	reasons := map[string]volumepool.AdmissionReason{}
	for _, decision := range plan.MemberDecisions {
		reasons[decision.VolumeID] = decision.Reason
	}
	if reasons["18a00002"] != volumepool.AdmissionReasonStateNotActive {
		t.Fatalf("draining member reason = %q, want metadata state preserved; plan=%+v", reasons["18a00002"], plan)
	}
	status := runtime.Status()
	if status.CapacityRefreshCount != 1 || status.CapacityObservationCount != 2 || status.CapacitySource != "namrbd_sbs_observability" {
		t.Fatalf("capacity status = %+v", status)
	}
}

type runtimeTestSegmentStore struct{}

func (runtimeTestSegmentStore) PutSegment(_ context.Context, _ storage.PutSegmentRequest) (storage.SegmentRef, error) {
	return storage.SegmentRef{SegmentID: "test"}, nil
}

func (runtimeTestSegmentStore) GetSegment(_ context.Context, _ storage.SegmentRef, _, _ uint64) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (runtimeTestSegmentStore) DeleteSegment(_ context.Context, _ storage.SegmentRef, _ storage.DeleteReason) error {
	return nil
}

func assertGatewayGauge(t *testing.T, gatherer prometheus.Gatherer, name string, want float64) {
	t.Helper()
	families, err := gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metric.GetGauge() == nil {
				continue
			}
			if got := metric.GetGauge().GetValue(); math.Abs(got-want) > 0.000001 {
				t.Fatalf("%s = %v, want %v", name, got, want)
			}
			return
		}
	}
	t.Fatalf("missing gauge %s", name)
}

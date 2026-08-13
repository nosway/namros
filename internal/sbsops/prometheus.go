package sbsops

import (
	"fmt"
	"io"
	"strings"
)

func WritePrometheus(w io.Writer, snapshot Snapshot) error {
	poolID := label(firstNonEmpty(snapshot.Pool.PoolID, snapshot.ClusterID, "default"))
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_node_up SBS node configured/up status. 0 means unknown or unavailable."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_node_up gauge"); err != nil {
		return err
	}
	for _, node := range snapshot.NodeDetails {
		value := 0
		if node.State == "healthy" {
			value = 1
		}
		if _, err := fmt.Fprintf(w, "namros_sbs_node_up{node_id=%q,role=%q,state=%q} %d\n", label(node.NodeID), label(node.Role), label(node.State), value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_volume_state SBS volume state marker."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_volume_state gauge"); err != nil {
		return err
	}
	for _, volume := range snapshot.VolumeDetails {
		if _, err := fmt.Fprintf(w, "namros_sbs_volume_state{volume_id=%q,state=%q} 1\n", label(volume.VolumeID), label(volume.State)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_store_state SBS store state marker from NAMRBD observability."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_store_state gauge"); err != nil {
		return err
	}
	for _, store := range snapshot.Stores {
		if _, err := fmt.Fprintf(w, "namros_sbs_store_state{store_id=%q,state=%q} 1\n", label(store.StoreID), label(store.State)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_capacity_bytes SBS capacity byte gauges from NAMRBD observability."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_capacity_bytes gauge"); err != nil {
		return err
	}
	for kind, value := range map[string]uint64{
		"available":   snapshot.Capacity.AvailableBytes,
		"reclaimable": snapshot.Capacity.ReclaimableBytes,
		"reserved":    snapshot.Capacity.ReservedBytes,
		"total":       snapshot.Capacity.TotalBytes,
		"used":        snapshot.Capacity.UsedBytes,
	} {
		if _, err := fmt.Fprintf(w, "namros_sbs_capacity_bytes{kind=%q} %d\n", kind, value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_capacity_used_percent SBS capacity used percentage from NAMRBD observability."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_capacity_used_percent gauge"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "namros_sbs_capacity_used_percent %g\n", snapshot.Capacity.UsedPercent); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_pool_members SBS volume pool member counts by state."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_pool_members gauge"); err != nil {
		return err
	}
	for state, value := range map[string]int{
		"degraded":  snapshot.Pool.DegradedMembers,
		"healthy":   snapshot.Pool.HealthyMembers,
		"read_only": snapshot.Pool.ReadOnlyMembers,
		"unknown":   snapshot.Pool.UnknownMembers,
		"writable":  snapshot.Pool.WritableMembers,
	} {
		if _, err := fmt.Fprintf(w, "namros_sbs_pool_members{pool_id=%q,state=%q} %d\n", poolID, state, value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_pool_generation SBS volume pool generation gauges."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_pool_generation gauge"); err != nil {
		return err
	}
	for kind, value := range map[string]uint64{
		"active":     snapshot.Pool.ActiveGeneration,
		"configured": snapshot.Pool.ConfiguredGeneration,
	} {
		if _, err := fmt.Fprintf(w, "namros_sbs_pool_generation{pool_id=%q,kind=%q} %d\n", poolID, kind, value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_pool_capacity_bytes SBS volume pool capacity byte gauges."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_pool_capacity_bytes gauge"); err != nil {
		return err
	}
	for kind, value := range map[string]uint64{
		"available":   snapshot.Pool.Capacity.AvailableBytes,
		"reclaimable": snapshot.Pool.Capacity.ReclaimableBytes,
		"reserved":    snapshot.Pool.Capacity.ReservedBytes,
		"total":       snapshot.Pool.Capacity.TotalBytes,
		"used":        snapshot.Pool.Capacity.UsedBytes,
	} {
		if _, err := fmt.Fprintf(w, "namros_sbs_pool_capacity_bytes{pool_id=%q,kind=%q} %d\n", poolID, kind, value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_pool_admission_state SBS volume pool write admission state marker."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_pool_admission_state gauge"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "namros_sbs_pool_admission_state{pool_id=%q,state=%q} 1\n", poolID, label(snapshot.Pool.AdmissionState)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_pool_refresh_errors SBS volume pool refresh errors observed by the collector source."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_pool_refresh_errors gauge"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "namros_sbs_pool_refresh_errors{pool_id=%q} %d\n", poolID, snapshot.Pool.RefreshErrorCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_pool_stale_seconds SBS volume pool stale duration in seconds."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_pool_stale_seconds gauge"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "namros_sbs_pool_stale_seconds{pool_id=%q} %g\n", poolID, snapshot.Pool.StaleDurationSeconds); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_reclaim_jobs SBS reclaim summary from NAMRBD observability."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_reclaim_jobs gauge"); err != nil {
		return err
	}
	for state, value := range map[string]int{
		"blocked":    snapshot.Reclaim.Blocked,
		"candidates": snapshot.Reclaim.Candidates,
		"completed":  snapshot.Reclaim.Completed,
		"failed":     snapshot.Reclaim.Failed,
		"running":    snapshot.Reclaim.Running,
	} {
		if _, err := fmt.Fprintf(w, "namros_sbs_reclaim_jobs{state=%q} %d\n", state, value); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, "# HELP namros_sbs_maintenance_jobs SBS maintenance job summary."); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "# TYPE namros_sbs_maintenance_jobs gauge"); err != nil {
		return err
	}
	for state, value := range map[string]int{
		"repair_running":    snapshot.Maintenance.RepairRunning,
		"rebalance_running": snapshot.Maintenance.RebalanceRunning,
		"stuck":             snapshot.Maintenance.Stuck,
		"unknown":           snapshot.Maintenance.Unknown,
	} {
		if _, err := fmt.Fprintf(w, "namros_sbs_maintenance_jobs{state=%q} %d\n", state, value); err != nil {
			return err
		}
	}
	return nil
}

func label(value string) string {
	return strings.ReplaceAll(value, "\n", " ")
}

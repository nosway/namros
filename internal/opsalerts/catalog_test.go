package opsalerts

import "testing"

func TestCatalogHasStableUniqueIDs(t *testing.T) {
	seen := map[string]struct{}{}
	for _, definition := range Catalog() {
		if definition.ID == "" {
			t.Fatalf("definition has empty id: %+v", definition)
		}
		if _, ok := seen[definition.ID]; ok {
			t.Fatalf("duplicate alert id %q", definition.ID)
		}
		seen[definition.ID] = struct{}{}
		if definition.Severity == "" || definition.Component == "" || definition.Summary == "" {
			t.Fatalf("definition is incomplete: %+v", definition)
		}
	}
	for _, id := range []string{GatewayDown, GatewayLeaseExpired, MetadataUnavailable, SBSNodeAbnormal, SBSCapacityHigh, SBSPoolBlocked, S3FiveXXElevated, MaintenanceStuck, WorkerBacklogHigh, QuotaAdmissionHigh} {
		if _, ok := seen[id]; !ok {
			t.Fatalf("catalog missing %s", id)
		}
	}
}

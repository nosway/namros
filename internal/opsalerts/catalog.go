package opsalerts

type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

type Definition struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Severity  Severity `json:"severity"`
	Component string   `json:"component"`
	Runbook   string   `json:"runbook,omitempty"`
	Summary   string   `json:"summary"`
}

type Instance struct {
	ID        string   `json:"id"`
	Severity  Severity `json:"severity"`
	Component string   `json:"component"`
	Message   string   `json:"message"`
	FeatureID string   `json:"feature_id,omitempty"`
	Runbook   string   `json:"runbook,omitempty"`
}

const (
	GatewayDown           = "NamrosGatewayDown"
	GatewayLeaseExpired   = "NamrosGatewayLeaseExpired"
	MetadataUnavailable   = "NamrosMetadataUnavailable"
	SBSNodeAbnormal       = "NamrosSBSNodeAbnormal"
	SBSCapacityHigh       = "NamrosSBSCapacityHigh"
	SBSPoolBlocked        = "NamrosSBSPoolBlocked"
	S3FiveXXElevated      = "NamrosS3FiveXXElevated"
	MaintenanceStuck      = "NamrosMaintenanceStuck"
	WorkerBacklogHigh     = "NamrosWorkerBacklogHigh"
	QuotaAdmissionHigh    = "NamrosQuotaAdmissionHigh"
	EnterpriseFeatureDeny = "NamrosEnterpriseFeatureDenied"
)

func Catalog() []Definition {
	return []Definition{
		{ID: GatewayDown, Name: "Gateway down", Severity: SeverityCritical, Component: "gateway", Runbook: "namros-gateway-coordination-runbook", Summary: "No healthy NAMROS gateway is reachable."},
		{ID: GatewayLeaseExpired, Name: "Gateway lease expired", Severity: SeverityWarning, Component: "coordination", Runbook: "namros-gateway-coordination-runbook", Summary: "A gateway registry lease expired or is missing."},
		{ID: MetadataUnavailable, Name: "Metadata unavailable", Severity: SeverityCritical, Component: "metadata", Runbook: "namros-metadata-backup-restore-runbook", Summary: "The metadata repository is unavailable or not configured."},
		{ID: SBSNodeAbnormal, Name: "SBS node abnormal", Severity: SeverityWarning, Component: "sbs", Runbook: "namros-18node-lab-experiment-plan", Summary: "One or more SBS nodes are abnormal or unknown."},
		{ID: SBSCapacityHigh, Name: "SBS capacity high", Severity: SeverityWarning, Component: "sbs", Runbook: "namros-18node-lab-experiment-plan", Summary: "SBS capacity usage is above the configured threshold."},
		{ID: SBSPoolBlocked, Name: "SBS pool blocked", Severity: SeverityCritical, Component: "sbs", Runbook: "namros-sbs-volume-pool-policy", Summary: "An SBS volume pool has no writable admission state."},
		{ID: S3FiveXXElevated, Name: "S3 5xx elevated", Severity: SeverityWarning, Component: "gateway", Runbook: "namros-compatibility-test-runbook", Summary: "S3 5xx responses are elevated."},
		{ID: MaintenanceStuck, Name: "Maintenance stuck", Severity: SeverityCritical, Component: "maintenance", Runbook: "namros-metadata-backup-restore-runbook", Summary: "Repair, rebalance, GC, or dedupe maintenance appears stuck."},
		{ID: WorkerBacklogHigh, Name: "Worker backlog high", Severity: SeverityWarning, Component: "maintenance", Runbook: "namros-metadata-backup-restore-runbook", Summary: "Worker retry backlog is accumulating."},
		{ID: QuotaAdmissionHigh, Name: "Quota admission high", Severity: SeverityInfo, Component: "quota", Runbook: "namros-s3-api-spec-scope", Summary: "Request, quota, or data-budget admission rejections are elevated."},
		{ID: EnterpriseFeatureDeny, Name: "Enterprise feature denied", Severity: SeverityInfo, Component: "edition", Runbook: "namros-editions", Summary: "An Enterprise-only feature was requested from a Community build."},
	}
}

func DefinitionByID(id string) (Definition, bool) {
	for _, definition := range Catalog() {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

func NewInstance(id, message, featureID string) Instance {
	definition, ok := DefinitionByID(id)
	if !ok {
		return Instance{ID: id, Severity: SeverityWarning, Message: message, FeatureID: featureID}
	}
	return Instance{
		ID:        definition.ID,
		Severity:  definition.Severity,
		Component: definition.Component,
		Message:   message,
		FeatureID: featureID,
		Runbook:   definition.Runbook,
	}
}

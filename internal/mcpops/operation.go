package mcpops

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nosway/namros/internal/edition"
)

const (
	RiskObserve = "observe"
	RiskProbe   = "probe"
	RiskRepair  = "repair"
	RiskProtect = "protect"
)

type OperationEnvelope struct {
	SchemaVersion   string         `json:"schema_version"`
	OperationID     string         `json:"operation_id"`
	Tool            string         `json:"tool"`
	RiskClass       string         `json:"risk_class"`
	EditionRequired string         `json:"edition_required"`
	Mode            string         `json:"mode"`
	Approval        ApprovalState  `json:"approval"`
	Plan            map[string]any `json:"plan"`
	Preflight       map[string]any `json:"preflight"`
	Result          map[string]any `json:"result"`
	Verification    map[string]any `json:"verification"`
	Audit           map[string]any `json:"audit"`
}

type ApprovalState struct {
	Required  bool   `json:"required"`
	Policy    string `json:"policy"`
	Reference string `json:"reference,omitempty"`
}

func BuildOperationPlan(cfg Config, tool, riskClass, editionRequired string, plan map[string]any, approvalReference string) OperationEnvelope {
	cfg = cfg.Normalized()
	required := riskClass != RiskObserve
	preflight := map[string]any{
		"status":  "planned",
		"edition": CurrentProductEdition(),
	}
	result := map[string]any{
		"status":  "not_executed",
		"message": "operator-approved execution is not wired in this implementation slice",
	}
	if editionRequired == "" {
		editionRequired = edition.Community
	}
	if editionRequired == edition.Enterprise && edition.Current() != edition.Enterprise {
		preflight["status"] = "blocked"
		preflight["reason"] = fmt.Sprintf("tool %q requires NAMROS Enterprise Edition", tool)
		result["status"] = "blocked"
		result["message"] = "NAMROS Enterprise Edition is required"
	}
	if cfg.Mode != ModeOperate && required {
		preflight["status"] = "blocked"
		preflight["reason"] = "MCP provider is running in observe mode"
	}
	return OperationEnvelope{
		SchemaVersion:   "namros.mcp.operation.v1",
		OperationID:     newOperationID(tool),
		Tool:            tool,
		RiskClass:       riskClass,
		EditionRequired: editionRequired,
		Mode:            cfg.Mode,
		Approval: ApprovalState{
			Required:  required,
			Policy:    cfg.ApprovalPolicy,
			Reference: approvalReference,
		},
		Plan:         plan,
		Preflight:    preflight,
		Result:       result,
		Verification: map[string]any{"status": "not_run"},
		Audit: map[string]any{
			"local_output_dir": cfg.OperationOutputDir,
			"status":           "not_written",
		},
	}
}

func WriteLocalOperationRecord(cfg Config, envelope OperationEnvelope) (OperationEnvelope, error) {
	cfg = cfg.Normalized()
	if cfg.OperationOutputDir == "" {
		envelope.Audit = map[string]any{
			"status": "disabled",
		}
		return envelope, nil
	}
	if err := os.MkdirAll(cfg.OperationOutputDir, 0o700); err != nil {
		return envelope, err
	}
	path := filepath.Join(cfg.OperationOutputDir, envelope.OperationID+".json")
	envelope.Audit = map[string]any{
		"local_path": path,
		"status":     "written",
	}
	payload, err := json.MarshalIndent(Redact(envelope), "", "  ")
	if err != nil {
		return envelope, err
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return envelope, err
	}
	return envelope, nil
}

func EnterpriseRequired(tool, featureID string) map[string]any {
	return map[string]any{
		"schema_version": "namros.mcp.enterprise_required.v1",
		"tool":           tool,
		"feature_id":     featureID,
		"status":         "blocked",
		"error":          fmt.Sprintf("feature %q is supported in NAMROS Enterprise Edition", featureID),
		"edition":        CurrentProductEdition(),
	}
}

func newOperationID(tool string) string {
	return fmt.Sprintf("op-%s-%d", sanitizeToolName(tool), time.Now().UTC().UnixNano())
}

func sanitizeToolName(value string) string {
	out := make([]rune, 0, len(value))
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			out = append(out, r)
			continue
		}
		out = append(out, '-')
	}
	if len(out) == 0 {
		return "unknown"
	}
	return string(out)
}

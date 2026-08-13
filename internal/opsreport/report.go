package opsreport

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/nosway/namros/internal/adminstatus"
	"github.com/nosway/namros/internal/sbsops"
)

type Input struct {
	Scope       string
	Metadata    adminstatus.Counts
	SBS         sbsops.Snapshot
	GeneratedAt time.Time
}

type Report struct {
	SchemaVersion   string             `json:"schema_version"`
	ReportID        string             `json:"report_id"`
	GeneratedAt     string             `json:"generated_at"`
	Scope           string             `json:"scope"`
	Metadata        adminstatus.Counts `json:"metadata"`
	SBS             sbsops.Snapshot    `json:"sbs"`
	Recommendations []Recommendation   `json:"recommendations"`
	Limitations     []string           `json:"limitations,omitempty"`
}

type Recommendation struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

func Build(input Input) Report {
	if input.Scope == "" {
		input.Scope = "cluster"
	}
	if input.GeneratedAt.IsZero() {
		input.GeneratedAt = time.Now().UTC()
	}
	report := Report{
		SchemaVersion: "namros.ops.report.v1",
		GeneratedAt:   input.GeneratedAt.UTC().Format(time.RFC3339Nano),
		Scope:         input.Scope,
		Metadata:      input.Metadata,
		SBS:           input.SBS,
		Limitations: []string{
			"Bucket/prefix trend analytics and CSV/Parquet export are not implemented in this build slice.",
		},
	}
	report.Recommendations = recommendations(input)
	report.ReportID = reportID(report.GeneratedAt, report.Scope)
	return report
}

func recommendations(input Input) []Recommendation {
	out := make([]Recommendation, 0)
	if input.SBS.Status == "disabled" {
		out = append(out, Recommendation{
			ID:       "sbs-collector-disabled",
			Severity: "warning",
			Summary:  "Configure SBS exporter endpoints before using the operations dashboard for storage health decisions.",
		})
	}
	if input.SBS.Nodes.Unknown > 0 {
		out = append(out, Recommendation{
			ID:       "sbs-node-health-unknown",
			Severity: "warning",
			Summary:  "SBS node health is unknown; enable live SBS RPC collection in the lab before production validation.",
		})
	}
	if input.Metadata.AuditEvents == 0 {
		out = append(out, Recommendation{
			ID:       "audit-chain-empty",
			Severity: "info",
			Summary:  "No admin audit events are visible in the report scope.",
		})
	}
	return out
}

func reportID(generatedAt, scope string) string {
	sum := sha256.Sum256([]byte(generatedAt + "|" + scope))
	return "ops-" + hex.EncodeToString(sum[:8])
}

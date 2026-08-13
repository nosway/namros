package opsreport

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/sbsops"
)

func TestBuildReportAddsRecommendationsAndStableEnvelope(t *testing.T) {
	report := Build(Input{
		Scope:       "namros18",
		GeneratedAt: time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC),
		SBS:         sbsops.NewCollector(sbsops.Config{}).Snapshot(nil),
	})
	if report.SchemaVersion != "namros.ops.report.v1" || report.ReportID == "" {
		t.Fatalf("report envelope = %+v", report)
	}
	if report.Scope != "namros18" {
		t.Fatalf("scope = %q", report.Scope)
	}
	if len(report.Recommendations) == 0 || len(report.Limitations) == 0 {
		t.Fatalf("report should include recommendations and limitations: %+v", report)
	}
}

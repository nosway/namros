package meta

import (
	"errors"
	"testing"
)

func TestEstimateMetadataScaleBudgetDefaultMaxMultipart(t *testing.T) {
	report, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{})
	if err != nil {
		t.Fatalf("EstimateMetadataScaleBudget() error = %v", err)
	}
	if report.SchemaVersion != MetadataScaleBudgetSchemaVersion {
		t.Fatalf("schema version = %q", report.SchemaVersion)
	}
	if report.PartCount != MaxMultipartParts ||
		report.SegmentRefCount != MaxObjectManifestSegmentRefs ||
		report.ProtectedRefCount != MaxObjectManifestSegmentRefs ||
		report.GCCandidateCount != MaxObjectManifestSegmentRefs {
		t.Fatalf("counts = part:%d refs:%d", report.PartCount, report.SegmentRefCount)
	}
	if len(report.Records) != 8 {
		t.Fatalf("records = %+v", report.Records)
	}
	if recordByName(report.Records, "protected_ref_by_version").Count != MaxObjectManifestSegmentRefs {
		t.Fatalf("protected ref by version record = %+v", recordByName(report.Records, "protected_ref_by_version"))
	}
	if recordByName(report.Records, "protected_ref_by_segment").Count != MaxObjectManifestSegmentRefs {
		t.Fatalf("protected ref by segment record = %+v", recordByName(report.Records, "protected_ref_by_segment"))
	}
	if recordByName(report.Records, "gc_candidate").Count != MaxObjectManifestSegmentRefs {
		t.Fatalf("gc candidate record = %+v", recordByName(report.Records, "gc_candidate"))
	}
	if report.CompleteTransaction.ApproxTotalBytes <= 0 || report.CompleteTransaction.RecordCountTouched <= MaxMultipartParts {
		t.Fatalf("complete transaction = %+v", report.CompleteTransaction)
	}
	if len(report.Gates) != 5 {
		t.Fatalf("gates = %+v", report.Gates)
	}
	for _, gate := range report.Gates {
		if gate.Status == "" || gate.ValueBytes <= 0 || gate.BudgetBytes <= 0 {
			t.Fatalf("gate = %+v", gate)
		}
	}
	if report.ReleaseGate.Status != "warning" || len(report.ReleaseGate.WarningGates) == 0 {
		t.Fatalf("release gate = %+v, want warning gates for default max profile", report.ReleaseGate)
	}
}

func TestEstimateMetadataScaleBudgetIncludesProtectionAndGCCandidatesInTransaction(t *testing.T) {
	base, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{
		PartCount:                  2,
		SegmentRefCount:            2,
		ProtectedRefCount:          2,
		GCCandidateCount:           2,
		IncludeListIndexWriteBytes: true,
	})
	if err != nil {
		t.Fatalf("EstimateMetadataScaleBudget(base) error = %v", err)
	}
	full, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{
		PartCount:                  2,
		SegmentRefCount:            2,
		ProtectedRefCount:          2,
		GCCandidateCount:           2,
		IncludeListIndexWriteBytes: true,
		IncludeProtectedRefBytes:   true,
		IncludeGCCandidateBytes:    true,
	})
	if err != nil {
		t.Fatalf("EstimateMetadataScaleBudget(full) error = %v", err)
	}
	if full.CompleteTransaction.ApproxTotalBytes <= base.CompleteTransaction.ApproxTotalBytes {
		t.Fatalf("full transaction did not include protection/GC bytes: base=%+v full=%+v", base.CompleteTransaction, full.CompleteTransaction)
	}
	if got, want := full.CompleteTransaction.RecordCountTouched-base.CompleteTransaction.RecordCountTouched, 6; got != want {
		t.Fatalf("extra touched records = %d, want %d; base=%+v full=%+v", got, want, base.CompleteTransaction, full.CompleteTransaction)
	}
}

func TestEstimateMetadataScaleBudgetRejectsOverflow(t *testing.T) {
	if _, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{PartCount: MaxMultipartParts + 1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("part overflow error = %v, want ErrInvalidArgument", err)
	}
	if _, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{SegmentRefCount: MaxObjectManifestSegmentRefs + 1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("segment ref overflow error = %v, want ErrInvalidArgument", err)
	}
	if _, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{ChunksPerSegment: 1025}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("chunk overflow error = %v, want ErrInvalidArgument", err)
	}
	if _, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{ProtectedRefCount: MaxObjectManifestSegmentRefs + 1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("protected ref overflow error = %v, want ErrInvalidArgument", err)
	}
	if _, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{GCCandidateCount: MaxObjectManifestSegmentRefs + 1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GC candidate overflow error = %v, want ErrInvalidArgument", err)
	}
}

func TestMetadataScaleBudgetReleaseGateEvaluation(t *testing.T) {
	report, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{
		PartCount: 2,
	})
	if err != nil {
		t.Fatalf("EstimateMetadataScaleBudget() error = %v", err)
	}
	if got := EvaluateMetadataScaleBudgetReleaseGate(report, false); got.Status != "passed" {
		t.Fatalf("small release gate = %+v, want passed", got)
	}
	defaultReport, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{})
	if err != nil {
		t.Fatalf("EstimateMetadataScaleBudget(default) error = %v", err)
	}
	if got := EvaluateMetadataScaleBudgetReleaseGate(defaultReport, true); got.Status != "failed" || len(got.FailedGates) == 0 {
		t.Fatalf("strict release gate = %+v, want failed watch gates", got)
	}
	overBudget, err := EstimateMetadataScaleBudget(MetadataScaleBudgetRequest{
		PartCount:              1,
		ValueBudgetBytes:       1,
		CompleteTxnBudgetBytes: 1,
	})
	if err != nil {
		t.Fatalf("EstimateMetadataScaleBudget(over) error = %v", err)
	}
	if got := EvaluateMetadataScaleBudgetReleaseGate(overBudget, false); got.Status != "failed" || len(got.FailedGates) == 0 {
		t.Fatalf("over-budget release gate = %+v, want failed", got)
	}
}

func recordByName(records []MetadataRecordSizeEstimate, name string) MetadataRecordSizeEstimate {
	for _, record := range records {
		if record.Name == name {
			return record
		}
	}
	return MetadataRecordSizeEstimate{}
}

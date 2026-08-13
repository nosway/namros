package meta

import (
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

func TestMetadataSchemaPostureForRecord(t *testing.T) {
	now := time.Date(2026, 8, 10, 8, 0, 0, 0, time.UTC)
	current := DefaultMetadataSchemaRecord(now)
	if got := MetadataSchemaPostureForRecord(current); got.Status != MetadataSchemaStatusCurrent || got.Reason != "ok" {
		t.Fatalf("current posture = %+v", got)
	}

	future := current
	future.SchemaVersion = CurrentMetadataSchemaVersion + 1
	future.MinReaderVersion = CurrentMetadataSchemaVersion + 1
	future.MinWriterVersion = CurrentMetadataSchemaVersion + 1
	if got := MetadataSchemaPostureForRecord(future); got.Status != MetadataSchemaStatusUnsupported || !got.UnsupportedFuture {
		t.Fatalf("future posture = %+v", got)
	}

	invalid := model.MetadataSchemaRecord{}
	if got := MetadataSchemaPostureForRecord(invalid); got.Status != MetadataSchemaStatusMigration || !got.MigrationRequired {
		t.Fatalf("invalid posture = %+v", got)
	}
}

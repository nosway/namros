package meta

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

const (
	CurrentMetadataSchemaVersion    = 1
	MinimumMetadataReaderVersion    = 1
	MinimumMetadataWriterVersion    = 1
	MetadataSchemaStatusCurrent     = "current"
	MetadataSchemaStatusMigration   = "migration_required"
	MetadataSchemaStatusUnsupported = "unsupported_future"
	MetadataSchemaStatusError       = "error"
)

type MetadataSchemaPosture struct {
	Status               string
	Reason               string
	CurrentVersion       int
	MinimumReaderVersion int
	MinimumWriterVersion int
	Record               model.MetadataSchemaRecord
	MigrationRequired    bool
	UnsupportedFuture    bool
	Error                string
}

func DefaultMetadataSchemaRecord(now time.Time) model.MetadataSchemaRecord {
	now = now.UTC()
	return model.MetadataSchemaRecord{
		SchemaVersion:    CurrentMetadataSchemaVersion,
		MinReaderVersion: MinimumMetadataReaderVersion,
		MinWriterVersion: MinimumMetadataWriterVersion,
		UpdatedBy:        "namros",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func ValidateMetadataSchemaRecord(record model.MetadataSchemaRecord) error {
	if record.SchemaVersion <= 0 {
		return fmt.Errorf("%w: metadata schema version must be positive", ErrInvalidArgument)
	}
	if record.MinReaderVersion <= 0 {
		return fmt.Errorf("%w: metadata schema min reader version must be positive", ErrInvalidArgument)
	}
	if record.MinWriterVersion <= 0 {
		return fmt.Errorf("%w: metadata schema min writer version must be positive", ErrInvalidArgument)
	}
	if record.MinReaderVersion > record.SchemaVersion {
		return fmt.Errorf("%w: metadata schema min reader version cannot exceed schema version", ErrInvalidArgument)
	}
	if record.MinWriterVersion > record.SchemaVersion {
		return fmt.Errorf("%w: metadata schema min writer version cannot exceed schema version", ErrInvalidArgument)
	}
	return nil
}

func MetadataSchemaPostureForRecord(record model.MetadataSchemaRecord) MetadataSchemaPosture {
	out := MetadataSchemaPosture{
		Status:               MetadataSchemaStatusCurrent,
		Reason:               "ok",
		CurrentVersion:       CurrentMetadataSchemaVersion,
		MinimumReaderVersion: MinimumMetadataReaderVersion,
		MinimumWriterVersion: MinimumMetadataWriterVersion,
		Record:               record,
	}
	if err := ValidateMetadataSchemaRecord(record); err != nil {
		out.Status = MetadataSchemaStatusMigration
		out.Reason = "schema_record_invalid"
		out.MigrationRequired = true
		out.Error = err.Error()
		return out
	}
	if record.SchemaVersion > CurrentMetadataSchemaVersion || record.MinReaderVersion > CurrentMetadataSchemaVersion {
		out.Status = MetadataSchemaStatusUnsupported
		out.Reason = "schema_version_future"
		out.UnsupportedFuture = true
		return out
	}
	if record.SchemaVersion < CurrentMetadataSchemaVersion || record.MinWriterVersion < MinimumMetadataWriterVersion {
		out.Status = MetadataSchemaStatusMigration
		out.Reason = "schema_version_behind"
		out.MigrationRequired = true
		return out
	}
	return out
}

func CheckMetadataSchema(ctx context.Context, repo Repository) MetadataSchemaPosture {
	out := MetadataSchemaPosture{
		Status:               MetadataSchemaStatusError,
		Reason:               "metadata_repository_missing",
		CurrentVersion:       CurrentMetadataSchemaVersion,
		MinimumReaderVersion: MinimumMetadataReaderVersion,
		MinimumWriterVersion: MinimumMetadataWriterVersion,
		Error:                "metadata repository is not configured",
	}
	if repo == nil {
		return out
	}
	record, err := repo.GetMetadataSchema(ctx)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			out.Status = MetadataSchemaStatusMigration
			out.Reason = "schema_record_missing"
			out.MigrationRequired = true
			out.Error = ""
			return out
		}
		out.Reason = "schema_record_unavailable"
		out.Error = err.Error()
		return out
	}
	return MetadataSchemaPostureForRecord(record)
}

func MetadataSchemaRecordFromRequest(req PutMetadataSchemaRequest, now time.Time, existing model.MetadataSchemaRecord) (model.MetadataSchemaRecord, error) {
	record := model.MetadataSchemaRecord{
		SchemaVersion:    req.SchemaVersion,
		MinReaderVersion: req.MinReaderVersion,
		MinWriterVersion: req.MinWriterVersion,
		UpdatedBy:        strings.TrimSpace(req.UpdatedBy),
		CreatedAt:        existing.CreatedAt,
		UpdatedAt:        now.UTC(),
	}
	if record.MinReaderVersion == 0 {
		record.MinReaderVersion = record.SchemaVersion
	}
	if record.MinWriterVersion == 0 {
		record.MinWriterVersion = record.SchemaVersion
	}
	if record.UpdatedBy == "" {
		record.UpdatedBy = "namros"
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = record.UpdatedAt
	}
	if err := ValidateMetadataSchemaRecord(record); err != nil {
		return model.MetadataSchemaRecord{}, err
	}
	return record, nil
}

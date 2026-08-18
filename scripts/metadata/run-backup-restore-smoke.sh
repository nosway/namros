#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
. "$REPO_ROOT/scripts/compat/common.sh"

GO="${GO:-go}"
NAMROS_KEEP_TMP="${NAMROS_KEEP_TMP:-0}"

require_cmd "$GO"
require_cmd jq

tmpdir="$(make_tmpdir)"
source_meta="$tmpdir/source-meta"
empty_target_meta="$tmpdir/empty-target-meta"
seed_json="$tmpdir/seed-export.json"
export_json="$tmpdir/metadata-export.json"
import_source_json="$tmpdir/import-source-only.json"
import_empty_target_json="$tmpdir/import-empty-target.json"
apply_target_meta="$tmpdir/apply-target-meta"
apply_json="$tmpdir/import-apply.json"
restored_export_json="$tmpdir/restored-export.json"
import_conflict_json="$tmpdir/import-conflict-target.json"

cleanup() {
	local status=$?
	if compat_should_keep_tmp "$status"; then
		log "kept tmpdir: $tmpdir"
	else
		rm -rf "$tmpdir"
	fi
	exit "$status"
}

trap cleanup EXIT

run_admin() {
	mkdir -p "$REPO_ROOT/.cache/go-build" "$REPO_ROOT/.cache/go-mod"
	GOCACHE="${GOCACHE:-$REPO_ROOT/.cache/go-build}" \
		GOMODCACHE="${GOMODCACHE:-$REPO_ROOT/.cache/go-mod}" \
		"$GO" run ./cmd/namros-admin "$@"
}

log "seed source metadata with a community-safe audit record"
cat >"$seed_json" <<'JSON'
{
  "schema_version": 1,
  "generated_at": "2026-07-06T00:00:00Z",
  "limit": 20,
  "metadata_schema": {
    "schema_version": 1,
    "min_reader_version": 1,
    "min_writer_version": 1,
    "updated_by": "backup-restore-smoke",
    "created_at": "2026-07-06T00:00:00Z",
    "updated_at": "2026-07-06T00:00:00Z"
  },
  "metadata_migration_operations": [
    {
      "operation_id": "metadata-migration-community-smoke-1",
      "target_schema_version": 1,
      "status": "succeeded",
      "dry_run": false,
      "apply": true,
      "owner_id": "backup-restore-smoke",
      "steps": [
        {
          "name": "list_index_repair",
          "status": "succeeded",
          "records_scanned": 1,
          "records_repaired": 0
        }
      ],
      "started_at": "2026-07-06T00:00:00Z",
      "finished_at": "2026-07-06T00:00:01Z",
      "created_at": "2026-07-06T00:00:02Z"
    }
  ],
  "audit_events": [
    {
      "event_id": "audit-community-smoke-1",
      "action": "admin_metadata_export",
      "event_hash": "hash-community-smoke-1",
      "created_at": "2026-07-06T00:00:00Z"
    }
  ],
  "volume_pools": [
    {
      "pool_id": "object-pool",
      "generation": 1,
      "durability_class": "replicated",
      "storage_class_ids": ["STANDARD"],
      "members": [
        {
          "volume_id": "18a00001",
          "data_endpoint": "sbs-data-a:9444",
          "state": "active",
          "weight": 1
        }
      ],
      "created_at": "2026-07-06T00:00:00Z",
      "updated_at": "2026-07-06T00:00:00Z"
    }
  ],
  "volume_drain_operations": [
    {
      "operation_id": "drain-community-smoke-1",
      "pool_id": "object-pool",
      "source_volume_id": "18a00001",
      "target_volume_id": "18a00002",
      "owner_id": "backup-restore-smoke",
      "status": "succeeded",
      "scanned": 1,
      "copied": 1,
      "started_at": "2026-07-06T00:01:00Z",
      "finished_at": "2026-07-06T00:01:01Z",
      "created_at": "2026-07-06T00:01:02Z"
    }
  ],
  "worker_leases": [
    {
      "lease_id": "gc/orphans",
      "worker_kind": "gc",
      "shard_id": "orphans",
      "owner_id": "backup-restore-smoke",
      "generation": 1,
      "cursor": "cursor-a",
      "acquired_at": "2026-07-06T00:02:00Z",
      "updated_at": "2026-07-06T00:02:01Z",
      "expires_at": "2026-07-06T00:05:00Z"
    }
  ],
  "worker_operations": [
    {
      "operation_id": "worker-op-community-smoke-1",
      "worker_kind": "gc",
      "shard_id": "orphans",
      "owner_id": "backup-restore-smoke",
      "lease_id": "gc/orphans",
      "status": "succeeded",
      "scanned": 1,
      "processed": 1,
      "started_at": "2026-07-06T00:03:00Z",
      "finished_at": "2026-07-06T00:03:01Z",
      "created_at": "2026-07-06T00:03:02Z"
    }
  ]
}
JSON
(
	cd "$REPO_ROOT"
	run_admin metadata-import \
		-input "$seed_json" \
		-metadata-backend pebble \
		-metadata-path "$source_meta" \
		-apply \
		-allow-experimental-apply
) >/dev/null

log "export operational metadata"
(
	cd "$REPO_ROOT"
	run_admin metadata-export \
		-metadata-backend pebble \
		-metadata-path "$source_meta" \
		-limit 20
) >"$export_json"
jq -e '
	.schema_version == 1
	and .metadata_schema.schema_version == 1
	and ((.kms_keys // []) | length) == 0
	and ((.dedupe_operations // []) | length) == 0
	and (.audit_events | length) == 1
	and (.metadata_migration_operations | length) == 1
	and (.volume_pools | length) == 1
	and (.volume_drain_operations | length) == 1
	and (.worker_leases | length) == 1
	and (.worker_operations | length) == 1
' "$export_json" >/dev/null

log "validate source-only import dry-run"
(
	cd "$REPO_ROOT"
	run_admin metadata-import \
		-input "$export_json"
) >"$import_source_json"
jq -e '
	.valid == true
	and .ready_for_apply == true
	and .apply_plan.status == "blocked"
	and .apply_plan.write_enabled == false
	and .apply_plan.apply_supported == true
	and .apply_plan.preserve_source_ids == true
	and .apply_plan.preserve_audit_hashes == true
	and (.actions[] | select(.collection == "metadata_schema") | .operation) == "upsert_schema_marker"
	and (.actions[] | select(.collection == "audit_events") | .operation) == "insert_preserve_source_id"
' "$import_source_json" >/dev/null

log "validate empty target preflight"
(
	cd "$REPO_ROOT"
	run_admin metadata-import \
		-input "$export_json" \
		-metadata-backend pebble \
		-metadata-path "$empty_target_meta"
) >"$import_empty_target_json"
jq -e '
	.target_checked == true
	and .target_empty == true
	and .ready_for_apply == true
	and .apply_plan.status == "ready"
	and .apply_plan.write_enabled == true
	and .apply_plan.apply_supported == true
	and (.apply_plan.gates[] | select(.name == "target_checked") | .status) == "passed"
	and (.apply_plan.gates[] | select(.name == "target_empty") | .status) == "passed"
	and (.apply_plan.gates[] | select(.name == "write_path") | .status) == "passed"
' "$import_empty_target_json" >/dev/null

log "apply metadata restore into empty target"
(
	cd "$REPO_ROOT"
	run_admin metadata-import \
		-input "$export_json" \
		-metadata-backend pebble \
		-metadata-path "$apply_target_meta" \
		-apply \
		-allow-experimental-apply
) >"$apply_json"
jq -e '
	.apply_requested == true
	and .dry_run == false
	and .apply_plan.status == "ready"
	and .apply_result.status == "succeeded"
	and .apply_result.write_enabled == true
	and .apply_result.records_planned == 7
	and .apply_result.records_written == 7
	and (.apply_result.collections[] | select(.collection == "metadata_schema") | .status) == "written"
	and (.apply_result.collections[] | select(.collection == "audit_events") | .status) == "written"
	and (.apply_result.collections[] | select(.collection == "volume_pools") | .status) == "written"
	and (.apply_result.collections[] | select(.collection == "worker_operations") | .status) == "written"
' "$apply_json" >/dev/null

log "verify restored target export"
(
	cd "$REPO_ROOT"
	run_admin metadata-export \
		-metadata-backend pebble \
		-metadata-path "$apply_target_meta" \
		-limit 20
) >"$restored_export_json"
jq -e --slurpfile source "$export_json" '
	((.dedupe_operations // []) | length) == 0
	and ((.kms_keys // []) | length) == 0
	and .metadata_schema.schema_version == 1
	and (.metadata_migration_operations | length) == 1
	and (.audit_events | length) == 1
	and .audit_events[0].event_id == $source[0].audit_events[0].event_id
	and .audit_events[0].event_hash == $source[0].audit_events[0].event_hash
	and (.volume_pools | length) == 1
	and (.volume_drain_operations | length) == 1
	and (.worker_leases | length) == 1
	and (.worker_operations | length) == 1
' "$restored_export_json" >/dev/null

log "validate non-empty target conflict preflight"
(
	cd "$REPO_ROOT"
	run_admin metadata-import \
		-input "$export_json" \
		-metadata-backend pebble \
		-metadata-path "$source_meta"
) >"$import_conflict_json"
jq -e '
	.target_checked == true
	and .target_empty == false
	and .ready_for_apply == false
	and (.conflicts[] | select(.reason == "target_not_empty"))
	and (.apply_plan.gates[] | select(.name == "target_empty") | .status) == "blocked"
' "$import_conflict_json" >/dev/null

log "metadata backup/restore smoke passed"

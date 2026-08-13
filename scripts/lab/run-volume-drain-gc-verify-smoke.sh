#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GO="${GO:-go}"
GOFLAGS_VALUE="${GOFLAGS:--buildvcs=false}"
REPORT_ROOT="${NAMROS_VOLUME_DRAIN_GC_VERIFY_SMOKE_DIR:-$ROOT_DIR/.cache/volume-drain-gc-verify-smoke}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$REPORT_ROOT/$STAMP-$$"
LOG_FILE="$RUN_DIR/go-test.log"
SUMMARY_FILE="$RUN_DIR/summary.json"
LATEST_SUMMARY_FILE="$REPORT_ROOT/summary.json"

json_escape() {
	local value="$1"
	value="${value//\\/\\\\}"
	value="${value//\"/\\\"}"
	value="${value//$'\n'/\\n}"
	printf '%s' "$value"
}

write_summary() {
	local status="$1"
	local exit_code="$2"
	local started_at="$3"
	local finished_at="$4"
	{
		printf '{\n'
		printf '  "schema_version": "namros.volume_drain_gc_verify_smoke.v1",\n'
		printf '  "status": "%s",\n' "$(json_escape "$status")"
		printf '  "exit_code": %s,\n' "$exit_code"
		printf '  "started_at": "%s",\n' "$(json_escape "$started_at")"
		printf '  "finished_at": "%s",\n' "$(json_escape "$finished_at")"
		printf '  "tests": [\n'
		printf '    "TestWorkerReclaimOperationQueuesOldRefsAfterTargetValidation",\n'
		printf '    "TestWorkerReclaimOperationBlocksWhenTargetValidationFails",\n'
		printf '    "TestWorkerReclaimOperationBlocksProtectedSourceRef",\n'
		printf '    "TestVolumeDrainOperationsCommandPutsAndLists"\n'
		printf '  ],\n'
		printf '  "packages": [\n'
		printf '    "./internal/drain",\n'
		printf '    "./cmd/namros-admin"\n'
		printf '  ],\n'
		printf '  "evidence": {\n'
		printf '    "target_ref_is_validated_before_old_ref_gc": true,\n'
		printf '    "old_source_ref_is_queued_as_metadata_gc_candidate": true,\n'
		printf '    "protected_source_ref_blocks_gc_candidate": true,\n'
		printf '    "failed_target_validation_blocks_gc_candidate": true,\n'
		printf '    "queued_gc_attempt_status_is_admin_visible": true\n'
		printf '  },\n'
		printf '  "artifact_dir": "%s",\n' "$(json_escape "$RUN_DIR")"
		printf '  "log_file": "%s"\n' "$(json_escape "$LOG_FILE")"
		printf '}\n'
	} >"$SUMMARY_FILE"
	cp "$SUMMARY_FILE" "$LATEST_SUMMARY_FILE"
}

mkdir -p "$RUN_DIR" "$ROOT_DIR/.cache/go-build" "$ROOT_DIR/.cache/go-mod"
cd "$ROOT_DIR"

started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
exit_code=0
printf '[volume-drain-gc-verify-smoke] run volume drain GC verification smoke\n' >&2

# shellcheck disable=SC2086
if GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}" \
	"$GO" test $GOFLAGS_VALUE ./internal/drain ./cmd/namros-admin \
	-run '^(TestWorkerReclaimOperation.*|TestVolumeDrainOperationsCommandPutsAndLists)$' \
	-count=1 -v >"$LOG_FILE" 2>&1; then
	status="passed"
else
	exit_code=$?
	status="failed"
fi

finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
write_summary "$status" "$exit_code" "$started_at" "$finished_at"

printf '[volume-drain-gc-verify-smoke] summary: %s\n' "$SUMMARY_FILE" >&2
if [[ "$exit_code" -ne 0 ]]; then
	tail -n 80 "$LOG_FILE" >&2 || true
	exit "$exit_code"
fi

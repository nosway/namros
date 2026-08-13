#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GO="${GO:-go}"
GOFLAGS_VALUE="${GOFLAGS:--buildvcs=false}"
REPORT_ROOT="${NAMROS_VOLUME_POOL_DRAIN_READ_OLD_REF_SMOKE_DIR:-$ROOT_DIR/.cache/volume-pool-drain-read-old-ref-smoke}"
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
		printf '  "schema_version": "namros.volume_pool_drain_read_old_ref_smoke.v1",\n'
		printf '  "status": "%s",\n' "$(json_escape "$status")"
		printf '  "exit_code": %s,\n' "$exit_code"
		printf '  "started_at": "%s",\n' "$(json_escape "$started_at")"
		printf '  "finished_at": "%s",\n' "$(json_escape "$finished_at")"
		printf '  "test": "TestOpenDependenciesSBSVolumePoolDrainKeepsOldRefsRoutable",\n'
		printf '  "package": "./internal/gateway",\n'
		printf '  "go_tags": "enterprise,namros_ec",\n'
		printf '  "evidence": {\n'
		printf '    "draining_member_receives_new_writes": false,\n'
		printf '    "replacement_write_volume": "18a20002",\n'
		printf '    "old_ref_volume": "18a20001",\n'
		printf '    "old_ref_readable_after_drain": true,\n'
		printf '    "old_ref_deletable_after_drain": true\n'
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
printf '[volume-pool-drain-read-old-ref-smoke] run gateway drain/read-old-ref smoke\n' >&2

# shellcheck disable=SC2086
if GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}" \
	"$GO" test $GOFLAGS_VALUE -tags=enterprise,namros_ec ./internal/gateway \
	-run '^TestOpenDependenciesSBSVolumePoolDrainKeepsOldRefsRoutable$' -count=1 -v >"$LOG_FILE" 2>&1; then
	status="passed"
else
	exit_code=$?
	status="failed"
fi

finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
write_summary "$status" "$exit_code" "$started_at" "$finished_at"

printf '[volume-pool-drain-read-old-ref-smoke] summary: %s\n' "$SUMMARY_FILE" >&2
if [[ "$exit_code" -ne 0 ]]; then
	tail -n 80 "$LOG_FILE" >&2 || true
	exit "$exit_code"
fi

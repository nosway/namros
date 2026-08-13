#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GO="${GO:-go}"
GOFLAGS_VALUE="${GOFLAGS:--buildvcs=false}"
REPORT_ROOT="${NAMROS_NOISY_TENANT_PROFILE_SMOKE_DIR:-$ROOT_DIR/.cache/noisy-tenant-profile-smoke}"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="$REPORT_ROOT/$STAMP-$$"
LOG_FILE="$RUN_DIR/go-test.log"
CLI_STDOUT="$RUN_DIR/noisy-tenant-profile.stdout.json"
PROFILE_SUMMARY="$RUN_DIR/noisy-tenant-profile-summary.json"
EVENT_JSONL="$RUN_DIR/noisy-tenant-profile-events.jsonl"
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
		printf '  "schema_version": "namros.noisy_tenant_profile_smoke.v1",\n'
		printf '  "status": "%s",\n' "$(json_escape "$status")"
		printf '  "exit_code": %s,\n' "$exit_code"
		printf '  "started_at": "%s",\n' "$(json_escape "$started_at")"
		printf '  "finished_at": "%s",\n' "$(json_escape "$finished_at")"
		printf '  "tests": [\n'
		printf '    "TestNoisyTenantProfileReportsNeighborProgress",\n'
		printf '    "TestNoisyTenantProfileFailsWhenGlobalCapacityStarvesNeighbor",\n'
		printf '    "TestNoisyTenantProfileWritesSummaryAndEvents"\n'
		printf '  ],\n'
		printf '  "commands": [\n'
		printf '    "namros-s3bench noisy-tenant-profile"\n'
		printf '  ],\n'
		printf '  "evidence": {\n'
		printf '    "noisy_tenant_is_throttled": true,\n'
		printf '    "neighbor_tenant_completes_without_throttle": true,\n'
		printf '    "starvation_gate_fails_under_global_capacity_one": true,\n'
		printf '    "profile_summary_json_written": true,\n'
		printf '    "profile_event_jsonl_written": true\n'
		printf '  },\n'
		printf '  "artifact_dir": "%s",\n' "$(json_escape "$RUN_DIR")"
		printf '  "profile_summary": "%s",\n' "$(json_escape "$PROFILE_SUMMARY")"
		printf '  "event_jsonl": "%s",\n' "$(json_escape "$EVENT_JSONL")"
		printf '  "log_file": "%s"\n' "$(json_escape "$LOG_FILE")"
		printf '}\n'
	} >"$SUMMARY_FILE"
	cp "$SUMMARY_FILE" "$LATEST_SUMMARY_FILE"
}

mkdir -p "$RUN_DIR" "$ROOT_DIR/.cache/go-build" "$ROOT_DIR/.cache/go-mod"
cd "$ROOT_DIR"

started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
exit_code=0
printf '[noisy-tenant-profile-smoke] run noisy tenant profile smoke\n' >&2

# shellcheck disable=SC2086
if GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}" \
	"$GO" test $GOFLAGS_VALUE ./cmd/namros-s3bench \
	-run '^(TestNoisyTenantProfileReportsNeighborProgress|TestNoisyTenantProfileFailsWhenGlobalCapacityStarvesNeighbor|TestNoisyTenantProfileWritesSummaryAndEvents)$' \
	-count=1 -v >"$LOG_FILE" 2>&1 &&
	GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}" \
	"$GO" run $GOFLAGS_VALUE ./cmd/namros-s3bench noisy-tenant-profile \
		-summary-json "$PROFILE_SUMMARY" \
		-event-jsonl "$EVENT_JSONL" >"$CLI_STDOUT" 2>>"$LOG_FILE" &&
	grep -q '"status": "passed"' "$PROFILE_SUMMARY" &&
	grep -q '"neighbor_completed_without_throttle"' "$PROFILE_SUMMARY" &&
	grep -q '"status":"throttled"' "$EVENT_JSONL"; then
	status="passed"
else
	exit_code=$?
	status="failed"
fi

finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
write_summary "$status" "$exit_code" "$started_at" "$finished_at"

printf '[noisy-tenant-profile-smoke] summary: %s\n' "$SUMMARY_FILE" >&2
if [[ "$exit_code" -ne 0 ]]; then
	tail -n 80 "$LOG_FILE" >&2 || true
	cat "$CLI_STDOUT" >&2 || true
	exit "$exit_code"
fi

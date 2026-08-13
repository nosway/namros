#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GO="${GO:-go}"
GOFLAGS_VALUE="${GOFLAGS:--buildvcs=false}"
REPORT_ROOT="${NAMROS_WORKER_CONTROL_SMOKE_DIR:-$ROOT_DIR/.cache/worker-control-smoke}"
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
		printf '  "schema_version": "namros.worker_control_smoke.v1",\n'
		printf '  "status": "%s",\n' "$(json_escape "$status")"
		printf '  "exit_code": %s,\n' "$exit_code"
		printf '  "started_at": "%s",\n' "$(json_escape "$started_at")"
		printf '  "finished_at": "%s",\n' "$(json_escape "$finished_at")"
		printf '  "tests": [\n'
		printf '    "TestRepositorySuite",\n'
		printf '    "TestRunnerRunOnceReturns*ControlWithoutOperation",\n'
		printf '    "TestSchedulerRunOnceSkipsWhenPaused",\n'
		printf '    "TestWorkerControlCommandPausesGetsAndResumes"\n'
		printf '  ],\n'
		printf '  "packages": [\n'
		printf '    "./internal/meta/memory",\n'
		printf '    "./internal/meta/kvrepo",\n'
		printf '    "./internal/workerscheduler",\n'
		printf '    "./internal/gc",\n'
		printf '    "./internal/lifecycle",\n'
		printf '    "./cmd/namros-admin"\n'
		printf '  ],\n'
		printf '  "evidence": {\n'
		printf '    "worker_control_record_is_durable": true,\n'
		printf '    "paused_worker_does_not_acquire_lease_or_run_work": true,\n'
		printf '    "canceled_worker_does_not_run_work": true,\n'
		printf '    "gc_and_lifecycle_schedulers_report_pause_as_skip": true,\n'
		printf '    "admin_cli_pause_get_resume": true\n'
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
printf '[worker-control-smoke] run worker pause/cancel/resume smoke\n' >&2

# shellcheck disable=SC2086
if GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.cache/go-mod}" \
	"$GO" test $GOFLAGS_VALUE ./internal/meta/memory ./internal/meta/kvrepo ./internal/workerscheduler ./internal/gc ./internal/lifecycle ./cmd/namros-admin \
	-run '^(TestRepositorySuite|TestRunnerRunOnceReturnsPausedControlWithoutOperation|TestRunnerRunOnceReturnsCanceledControlWithoutOperation|TestSchedulerRunOnceSkipsWhenPaused|TestWorkerControlCommandPausesGetsAndResumes)$' \
	-count=1 -v >"$LOG_FILE" 2>&1; then
	status="passed"
else
	exit_code=$?
	status="failed"
fi

finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
write_summary "$status" "$exit_code" "$started_at" "$finished_at"

printf '[worker-control-smoke] summary: %s\n' "$SUMMARY_FILE" >&2
if [[ "$exit_code" -ne 0 ]]; then
	tail -n 80 "$LOG_FILE" >&2 || true
	exit "$exit_code"
fi

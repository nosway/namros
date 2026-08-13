#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"
docker_bin="${DOCKER:-docker}"
compose_file="${CONTAINER_COMPOSE_FILE:-$repo_root/packaging/docker/compose.community.yml}"
env_file="${CONTAINER_ENV_FILE:-$repo_root/packaging/docker/.env}"
report_root="${NAMROS_COMMUNITY_FAILOVER_SMOKE_DIR:-$repo_root/.cache/container-community-failover-smoke}"
min_free_bytes="${NAMROS_COMMUNITY_FAILOVER_MIN_FREE_BYTES:-10737418240}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
run_dir="$report_root/$stamp-$$"
events_file="$run_dir/events.jsonl"
summary_file="$run_dir/summary.json"
latest_summary_file="$report_root/summary.json"
started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
status="running"
exit_code=0
restore_service=""

log() {
	printf '[community-failover] %s\n' "$*" >&2
}

skip() {
	log "SKIP: $*"
	exit 77
}

check_free_space() {
	if [ "${NAMROS_COMMUNITY_FAILOVER_SKIP_DISK_PREFLIGHT:-0}" = "1" ]; then
		return 0
	fi
	case "$min_free_bytes" in
		''|*[!0-9]*)
			log "invalid NAMROS_COMMUNITY_FAILOVER_MIN_FREE_BYTES=$min_free_bytes"
			exit 2
			;;
	esac
	min_free_kib=$(( (min_free_bytes + 1023) / 1024 ))
	free_kib="$(df -Pk "$repo_root" 2>/dev/null | awk 'NR == 2 { print $4 }')"
	case "$free_kib" in
		''|*[!0-9]*)
			log "could not determine free disk space for $repo_root"
			exit 2
			;;
	esac
	if [ "$free_kib" -lt "$min_free_kib" ]; then
		free_mib=$(( free_kib / 1024 ))
		min_free_mib=$(( min_free_kib / 1024 ))
		skip "insufficient free disk for container failover smoke: free=${free_mib}MiB required=${min_free_mib}MiB path=$repo_root"
	fi
}

json_escape() {
	value="$1"
	value="$(printf '%s' "$value" | sed 's/\\/\\\\/g; s/"/\\"/g')"
	printf '%s' "$value"
}

compose() {
	GIT_TERMINAL_PROMPT="${GIT_TERMINAL_PROMPT:-0}" "$docker_bin" compose --env-file "$env_file" -f "$compose_file" --profile community "$@"
}

if ! command -v "$docker_bin" >/dev/null 2>&1; then
	log "$docker_bin is required"
	exit 127
fi
if ! "$docker_bin" compose version >/dev/null 2>&1; then
	log "Docker Compose provider is required for $docker_bin; install docker compose, docker-compose, or podman-compose"
	exit 127
fi
check_free_space
if [ "${NAMROS_CONTAINER_SKIP_ENSURE_LOCAL_FILES:-0}" != "1" ] &&
	{ [ -z "${CONTAINER_ENV_FILE:-}" ] || [ "$env_file" = "$repo_root/packaging/docker/.env" ]; }; then
	sh "$repo_root/scripts/container/ensure-local-files.sh"
fi

mkdir -p "$run_dir"
: >"$events_file"

append_event() {
	event_name="$1"
	event_kind="$2"
	event_status="$3"
	event_exit_code="$4"
	event_duration="$5"
	event_log_file="$6"
	event_note="$7"
	printf '{"name":"%s","kind":"%s","status":"%s","exit_code":%s,"duration_seconds":%s,"log_file":"%s","note":"%s"}\n' \
		"$(json_escape "$event_name")" \
		"$(json_escape "$event_kind")" \
		"$(json_escape "$event_status")" \
		"$event_exit_code" \
		"$event_duration" \
		"$(json_escape "$event_log_file")" \
		"$(json_escape "$event_note")" >>"$events_file"
}

write_summary() {
	summary_status="$1"
	summary_exit_code="$2"
	finished_at="$3"
	{
		printf '{\n'
		printf '  "schema_version": "namros.container_community_failover_smoke.v1",\n'
		printf '  "status": "%s",\n' "$(json_escape "$summary_status")"
		printf '  "exit_code": %s,\n' "$summary_exit_code"
		printf '  "started_at": "%s",\n' "$(json_escape "$started_at")"
		printf '  "finished_at": "%s",\n' "$(json_escape "$finished_at")"
		printf '  "compose_file": "%s",\n' "$(json_escape "$compose_file")"
		printf '  "env_file": "%s",\n' "$(json_escape "$env_file")"
		printf '  "evidence": {\n'
		printf '    "quick_start_smoke": "namros-container-community-smoke",\n'
		printf '    "cross_gateway_put_get_list": true,\n'
		printf '    "load_balancer_smoke_during_disruption": true,\n'
		printf '    "gateway_failover_checked": true,\n'
		printf '    "sbs_data_failover_checked": true,\n'
		printf '    "sbs_service_failover_checked": true,\n'
		printf '    "recovery_timing_recorded": true,\n'
		printf '    "fixed_recovery_sla_claimed": false\n'
		printf '  },\n'
		printf '  "stopped_services": ["namros-gateway-b", "sbs-data-4", "sbs-service-2"],\n'
		printf '  "events": [\n'
		event_first=1
		while IFS= read -r event_line; do
			[ -n "$event_line" ] || continue
			if [ "$event_first" -eq 0 ]; then
				printf ',\n'
			fi
			printf '    %s' "$event_line"
			event_first=0
		done <"$events_file"
		if [ "$event_first" -eq 0 ]; then
			printf '\n'
		fi
		printf '  ],\n'
		printf '  "artifact_dir": "%s",\n' "$(json_escape "$run_dir")"
		printf '  "events_file": "%s"\n' "$(json_escape "$events_file")"
		printf '}\n'
	} >"$summary_file"
	cp "$summary_file" "$latest_summary_file"
}

finish() {
	rc=$?
	trap - EXIT
	if [ -n "$restore_service" ]; then
		log "restore $restore_service"
		compose start "$restore_service" >"$run_dir/restore-$restore_service.log" 2>&1 || true
		restore_service=""
	fi
	if [ "$status" = "running" ]; then
		if [ "$rc" -eq 0 ]; then
			status="passed"
			exit_code=0
		else
			status="failed"
			exit_code="$rc"
		fi
	fi
	write_summary "$status" "$exit_code" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	if [ "$status" = "passed" ]; then
		log "failover smoke passed: $summary_file"
	else
		log "failover smoke failed: $summary_file"
	fi
}

trap finish EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

run_compose_event() {
	event_name="$1"
	event_note="$2"
	shift 2
	event_log_file="$run_dir/$event_name.log"
	event_started_epoch="$(date +%s)"
	log "$event_name"
	if compose "$@" >"$event_log_file" 2>&1; then
		event_rc=0
		event_status="passed"
	else
		event_rc=$?
		event_status="failed"
	fi
	event_duration=$(( $(date +%s) - event_started_epoch ))
	append_event "$event_name" "compose" "$event_status" "$event_rc" "$event_duration" "$event_log_file" "$event_note"
	if [ "$event_rc" -ne 0 ]; then
		tail -n 80 "$event_log_file" >&2 || true
		return "$event_rc"
	fi
	return 0
}

run_smoke() {
	smoke_name="$1"
	smoke_mode="$2"
	smoke_note="$3"
	smoke_log_file="$run_dir/$smoke_name.log"
	smoke_started_epoch="$(date +%s)"
	log "$smoke_name mode=$smoke_mode"
	if compose run --rm -e "NAMROS_COMMUNITY_SMOKE_MODE=$smoke_mode" community-tools namros-container-community-smoke >"$smoke_log_file" 2>&1; then
		smoke_rc=0
		smoke_status="passed"
	else
		smoke_rc=$?
		smoke_status="failed"
	fi
	smoke_duration=$(( $(date +%s) - smoke_started_epoch ))
	append_event "$smoke_name" "smoke" "$smoke_status" "$smoke_rc" "$smoke_duration" "$smoke_log_file" "$smoke_note"
	if [ "$smoke_rc" -ne 0 ]; then
		tail -n 120 "$smoke_log_file" >&2 || true
		return "$smoke_rc"
	fi
	return 0
}

stop_and_smoke() {
	service="$1"
	run_compose_event "stop-$service" "stop one replica before disruptive smoke" stop "$service"
	restore_service="$service"
	run_smoke "during-$service-stopped" "lb" "load-balancer smoke while $service is stopped"
	run_compose_event "start-$service" "restart stopped replica" start "$service"
	restore_service=""
	run_smoke "post-restore-$service" "cross-gateway" "cross-gateway recovery smoke after $service restart"
}

run_smoke baseline full "cross-gateway and load-balancer smoke before disruption"
stop_and_smoke namros-gateway-b
stop_and_smoke sbs-data-4
stop_and_smoke sbs-service-2

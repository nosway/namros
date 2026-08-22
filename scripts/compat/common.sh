#!/usr/bin/env bash

set -euo pipefail

NAMROS_COMPAT_REPO_ROOT="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
GO="${GO:-go}"
NAMROS_ENDPOINT="${NAMROS_ENDPOINT:-http://127.0.0.1:9000}"
NAMROS_REGION="${NAMROS_REGION:-us-east-1}"
NAMROS_ACCESS_KEY_ID="${NAMROS_ACCESS_KEY_ID:-namrosroot}"
NAMROS_SECRET_ACCESS_KEY="${NAMROS_SECRET_ACCESS_KEY:-namrosrootsecret}"
NAMROS_BUCKET_PREFIX="${NAMROS_BUCKET_PREFIX:-namros-compat}"
NAMROS_KEEP_BUCKET="${NAMROS_KEEP_BUCKET:-0}"
NAMROS_TMPDIR="${NAMROS_TMPDIR:-}"

log() {
	printf '[compat] %s\n' "$*" >&2
}

die() {
	printf '[compat] ERROR: %s\n' "$*" >&2
	exit 1
}

compat_err_trap() {
	local status=$?
	printf '[compat] ERROR: command failed with exit %d: %s\n' "$status" "$BASH_COMMAND" >&2
	exit "$status"
}

trap compat_err_trap ERR

has_cmd() {
	command -v "$1" >/dev/null 2>&1
}

require_cmd() {
	has_cmd "$1" || die "$1 is required"
}

compat_bool_env() {
	local name="$1"
	local value="${!name:-}"
	[[ "$value" == "1" || "$value" == "true" || "$value" == "TRUE" || "$value" == "yes" || "$value" == "YES" ]]
}

compat_should_use_18node_sbs() {
	compat_bool_env NAMROS_COMPAT_USE_18NODE_SBS || compat_bool_env NAMROS_QA_ALLOW_18NODE_EXEC
}

compat_load_18node_lab_env() {
	local lab_common="$NAMROS_COMPAT_REPO_ROOT/scripts/lab/common.sh"
	[[ -f "$lab_common" ]] || die "NAMROS lab common helper not found: $lab_common"
	# shellcheck disable=SC1090
	source "$lab_common"
	lab_load_env "${NAMROS_18NODE_ENV_FILE:-$NAMROS_COMPAT_REPO_ROOT/scripts/lab/namros-18node-lab.env}"
}

compat_resolve_18node_sbs_service_endpoint() {
	compat_load_18node_lab_env
	lab_resolve_sbs_service_endpoint
}

compat_resolve_18node_sbs_data_http_endpoint() {
	compat_load_18node_lab_env
	if [[ -n "${NAMROS_18NODE_SBS_DATA_HTTP_ENDPOINT:-}" ]]; then
		printf '%s\n' "$NAMROS_18NODE_SBS_DATA_HTTP_ENDPOINT"
		return 0
	fi
	local data_endpoint="${NAMROS_SBS_DATA_ENDPOINT:-$NAMROS_18NODE_SBS_DATA_ENDPOINT}"
	local host="${data_endpoint%:*}"
	[[ -n "$host" && "$host" != "$data_endpoint" ]] || die "cannot derive SBS data HTTP endpoint from $data_endpoint"
	printf '%s:%s\n' "$host" "$NAMROS_18NODE_SBS_DATA_HTTP_PORT"
}

compat_ensure_18node_sbs_volume() {
	local volume_id="$1"
	compat_should_use_18node_sbs || return 0
	[[ -n "$volume_id" ]] || return 0

	require_cmd curl
	require_cmd jq
	compat_load_18node_lab_env

	local artifact_dir status_file status_err materialize_file materialize_err
	artifact_dir="$NAMROS_18NODE_LOCAL_ARTIFACT_ROOT/compat-sbs-volume"
	mkdir -p "$artifact_dir"
	status_file="$artifact_dir/volume-status-$volume_id.json"
	status_err="$artifact_dir/volume-status-$volume_id.err"
	materialize_file="$artifact_dir/materialize-volume-$volume_id.json"
	materialize_err="$artifact_dir/materialize-volume-$volume_id.err"

	if lab_run_sbsctl volume status --volume-id "$volume_id" --output json >"$status_file" 2>"$status_err"; then
		log "reuse 18node SBS volume: $volume_id"
	else
		if [[ "${NAMROS_COMPAT_CREATE_18NODE_SBS_VOLUME:-1}" != "1" ]]; then
			cat "$status_err" >&2 2>/dev/null || true
			die "18node SBS volume $volume_id is missing and NAMROS_COMPAT_CREATE_18NODE_SBS_VOLUME != 1"
		fi
		log "create 18node SBS volume: $volume_id"
		lab_run_sbsctl volume create \
			--volume-id "$volume_id" \
			--size "$NAMROS_18NODE_SBS_VOLUME_SIZE" \
			--block-size "$NAMROS_18NODE_SBS_BLOCK_SIZE" \
			--allocation-chunk-size "$NAMROS_18NODE_SBS_ALLOCATION_CHUNK_SIZE" \
			--allocation-page-size "$NAMROS_18NODE_SBS_ALLOCATION_PAGE_SIZE" \
			--replication-factor 3 \
			--reason "namros-compat-sbs-volume" >"$artifact_dir/volume-create-$volume_id.json"
		lab_run_sbsctl volume status --volume-id "$volume_id" --output json >"$status_file" 2>"$status_err"
	fi

	if [[ "${NAMROS_COMPAT_MATERIALIZE_18NODE_SBS_VOLUME:-$NAMROS_18NODE_MATERIALIZE_SBS_VOLUME}" != "1" ]]; then
		return 0
	fi

	local data_http_endpoint size_bytes block_size chunk_size_bytes page_bytes prefix url status body
	data_http_endpoint="$(compat_resolve_18node_sbs_data_http_endpoint)"
	size_bytes="$(jq -r '.volume.size_bytes // empty' "$status_file")"
	block_size="$(jq -r '.volume.block_size // empty' "$status_file")"
	chunk_size_bytes="$(jq -r '.volume.chunk_size_bytes // empty' "$status_file")"
	page_bytes="$(jq -r '.volume.extent_page_bytes // empty' "$status_file")"
	[[ "$size_bytes" =~ ^[0-9]+$ && "$size_bytes" != "0" ]] || size_bytes="$(lab_parse_size_bytes "$NAMROS_18NODE_SBS_VOLUME_SIZE")"
	[[ "$block_size" =~ ^[0-9]+$ && "$block_size" != "0" ]] || block_size="$(lab_parse_size_bytes "$NAMROS_18NODE_SBS_BLOCK_SIZE")"
	[[ "$chunk_size_bytes" =~ ^[0-9]+$ && "$chunk_size_bytes" != "0" ]] || chunk_size_bytes="$(lab_parse_size_bytes "$NAMROS_18NODE_SBS_ALLOCATION_CHUNK_SIZE")"
	[[ "$page_bytes" =~ ^[0-9]+$ && "$page_bytes" != "0" ]] || page_bytes="$(lab_parse_size_bytes "$NAMROS_18NODE_SBS_ALLOCATION_PAGE_SIZE")"
	prefix="namros-$volume_id"
	url="http://${data_http_endpoint}/debug/materialize-volume?volume_id=${volume_id}&size_bytes=${size_bytes}&block_size=${block_size}&prefix=${prefix}&allocation_chunk_size_bytes=${chunk_size_bytes}&allocation_page_bytes=${page_bytes}"
	log "materialize 18node SBS volume: http=$data_http_endpoint volume=$volume_id"
	status="$(curl -sS -o "$materialize_file" -w '%{http_code}' -X POST "$url" 2>"$materialize_err" || true)"
	if [[ "$status" != "200" ]]; then
		body="$(tr '\n' ' ' <"$materialize_file" 2>/dev/null | sed 's/[[:space:]]*$//' || true)"
		cat "$materialize_err" >&2 2>/dev/null || true
		die "materialize 18node SBS volume failed http_status=${status:-curl-failed} endpoint=http://$data_http_endpoint body=${body:-<empty>}"
	fi
	jq -e \
		--arg volume_id "$volume_id" \
		--argjson size_bytes "$size_bytes" \
		'.ok == true and .volume_id == $volume_id and .size_bytes == $size_bytes' \
		"$materialize_file" >/dev/null
}

compat_should_keep_tmp() {
	local status="$1"

	if [ "${NAMROS_KEEP_TMP:-0}" = "1" ]; then
		return 0
	fi
	if [ "$status" -ne 0 ] && [ "${NAMROS_KEEP_FAILED_TMP:-1}" = "1" ]; then
		return 0
	fi
	return 1
}

compat_dump_file_tail() {
	local path="$1"
	local lines="${2:-120}"
	local label="${3:-log}"

	if [ -f "$path" ]; then
		log "$label tail: $path"
		tail -n "$lines" "$path" >&2 || true
	else
		log "$label is missing: $path"
	fi
}

NAMROS_COMPAT_AUTOSTART_GATEWAY_PID=""
NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG=""

compat_endpoint_listen_addr() {
	local endpoint="$1"
	local listen

	case "$endpoint" in
	http://*)
		listen="${endpoint#http://}"
		;;
	*)
		die "NAMROS_COMPAT_AUTOSTART_GATEWAY supports http endpoints only: $endpoint"
		;;
	esac
	listen="${listen%%/*}"
	[ -n "$listen" ] || die "could not derive listen address from NAMROS_ENDPOINT=$endpoint"
	printf '%s\n' "$listen"
}

compat_autostart_gateway() {
	local tmpdir="$1"
	local listen gateway_bin deadline

	if [ "${NAMROS_COMPAT_AUTOSTART_GATEWAY:-0}" != "1" ]; then
		return 0
	fi
	if curl -fsS "$NAMROS_ENDPOINT/readyz" >/dev/null 2>&1; then
		log "reuse existing gateway at $NAMROS_ENDPOINT"
		return 0
	fi

	require_cmd "$GO"
	require_cmd curl

	listen="$(compat_endpoint_listen_addr "$NAMROS_ENDPOINT")"
	gateway_bin="$tmpdir/namros-gateway"
	NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG="$tmpdir/namros-gateway.log"

	log "build autostart gateway binary"
	mkdir -p "$NAMROS_COMPAT_REPO_ROOT/.cache/go-build" "$NAMROS_COMPAT_REPO_ROOT/.cache/go-mod"
	(
		cd "$NAMROS_COMPAT_REPO_ROOT"
		GOCACHE="${GOCACHE:-$NAMROS_COMPAT_REPO_ROOT/.cache/go-build}" \
			GOMODCACHE="${GOMODCACHE:-$NAMROS_COMPAT_REPO_ROOT/.cache/go-mod}" \
			"$GO" build -o "$gateway_bin" ./cmd/namros-gateway
	)

	log "start autostart gateway: endpoint=$NAMROS_ENDPOINT listen=$listen"
	"$gateway_bin" \
		-http-listen "$listen" \
		-region "$NAMROS_REGION" \
		-metadata-backend memory \
		-storage-backend memory \
		-root-access-key-id "$NAMROS_ACCESS_KEY_ID" \
		-root-secret-access-key "$NAMROS_SECRET_ACCESS_KEY" \
		>"$NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG" 2>&1 &
	NAMROS_COMPAT_AUTOSTART_GATEWAY_PID=$!

	deadline=$((SECONDS + ${NAMROS_COMPAT_GATEWAY_START_TIMEOUT_SECONDS:-30}))
	while [ "$SECONDS" -lt "$deadline" ]; do
		if ! kill -0 "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1; then
			wait "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1 || true
			compat_dump_file_tail "$NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG" 120 "autostart gateway log"
			die "autostart gateway exited before readiness"
		fi
		if curl -fsS "$NAMROS_ENDPOINT/readyz" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	compat_dump_file_tail "$NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG" 120 "autostart gateway log"
	die "autostart gateway did not become ready at $NAMROS_ENDPOINT"
}

compat_stop_autostart_gateway() {
	local status="${1:-0}"

	if [ -n "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" ] && kill -0 "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1; then
		kill "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1 || true
		sleep 1
		if kill -0 "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1; then
			kill -9 "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1 || true
		fi
		wait "$NAMROS_COMPAT_AUTOSTART_GATEWAY_PID" >/dev/null 2>&1 || true
	fi
	if [ "$status" -ne 0 ] && [ -n "$NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG" ]; then
		compat_dump_file_tail "$NAMROS_COMPAT_AUTOSTART_GATEWAY_LOG" 120 "autostart gateway log"
	fi
}

compat_bucket() {
	local tool="${1:-smoke}"
	local suffix

	suffix="$(date +%Y%m%d%H%M%S)-$$"
	printf '%s-%s-%s' "$NAMROS_BUCKET_PREFIX" "$tool" "$suffix" | tr '[:upper:]_' '[:lower:]-'
}

make_tmpdir() {
	if [ -n "$NAMROS_TMPDIR" ]; then
		mkdir -p "$NAMROS_TMPDIR"
		mktemp -d "$NAMROS_TMPDIR/namros-compat.XXXXXX"
	else
		mktemp -d "${TMPDIR:-/tmp}/namros-compat.XXXXXX"
	fi
}

sha256_file() {
	if has_cmd sha256sum; then
		sha256sum "$1" | awk '{print $1}'
	elif has_cmd shasum; then
		shasum -a 256 "$1" | awk '{print $1}'
	else
		die "sha256sum or shasum is required"
	fi
}

assert_file_equals() {
	local want="$1"
	local got="$2"
	local label="${3:-file content}"

	if ! cmp -s "$want" "$got"; then
		die "$label mismatch: want $(sha256_file "$want"), got $(sha256_file "$got")"
	fi
}

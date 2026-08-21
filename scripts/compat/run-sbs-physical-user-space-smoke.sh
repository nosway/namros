#!/usr/bin/env bash

set -euo pipefail

USER_NAMROS_ENDPOINT="${NAMROS_ENDPOINT:-}"

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

GO="${GO:-go}"
NAMROS_GATEWAY_LISTEN="${NAMROS_GATEWAY_LISTEN:-127.0.0.1:9000}"
NAMROS_GATEWAY_CLIENT_HOST="${NAMROS_GATEWAY_CLIENT_HOST:-127.0.0.1}"
NAMROS_GATEWAY_START_TIMEOUT_SECONDS="${NAMROS_GATEWAY_START_TIMEOUT_SECONDS:-30}"
NAMROS_METADATA_BACKEND="${NAMROS_METADATA_BACKEND:-tikv}"
NAMROS_TIKV_PD_ENDPOINTS="${NAMROS_TIKV_PD_ENDPOINTS:-${NAMROS_18NODE_TIKV_PD_ENDPOINTS:-127.0.0.1:2379}}"
NAMROS_TIKV_API_VERSION="${NAMROS_TIKV_API_VERSION:-${NAMROS_18NODE_TIKV_API_VERSION:-v1}}"
NAMROS_TIKV_KEYSPACE="${NAMROS_TIKV_KEYSPACE:-namros-compat-$(date +%Y%m%d%H%M%S)-$$}"
NAMROS_TIKV_TIMEOUT="${NAMROS_TIKV_TIMEOUT:-}"
NAMROS_TIKV_RETRY_ATTEMPTS="${NAMROS_TIKV_RETRY_ATTEMPTS:-}"
NAMROS_TIKV_RETRY_INITIAL_BACKOFF="${NAMROS_TIKV_RETRY_INITIAL_BACKOFF:-}"
NAMROS_TIKV_RETRY_MAX_BACKOFF="${NAMROS_TIKV_RETRY_MAX_BACKOFF:-}"
NAMROS_TIKV_TLS_CA="${NAMROS_TIKV_TLS_CA:-}"
NAMROS_TIKV_TLS_CERT="${NAMROS_TIKV_TLS_CERT:-}"
NAMROS_TIKV_TLS_KEY="${NAMROS_TIKV_TLS_KEY:-}"
NAMROS_METADATA_PATH="${NAMROS_METADATA_PATH:-}"
NAMROS_SBS_VOLUME_ID="${NAMROS_SBS_VOLUME_ID:-${NAMROS_18NODE_SBS_VOLUME_ID:-}}"
NAMROS_SBS_CHUNK_SIZE_BYTES="${NAMROS_SBS_CHUNK_SIZE_BYTES:-}"
NAMROS_SBS_GATEWAY_ID="${NAMROS_SBS_GATEWAY_ID:-namros-compat-gateway-$(date +%Y%m%d%H%M%S)-$$}"
NAMROS_SBS_ATTACHMENT_ID="${NAMROS_SBS_ATTACHMENT_ID:-att-$NAMROS_SBS_GATEWAY_ID}"
NAMROS_SBS_GENERATION="${NAMROS_SBS_GENERATION:-}"
NAMROS_SBS_VERIFY_READBACK="${NAMROS_SBS_VERIFY_READBACK:-true}"
NAMROS_GATEWAY_LOG="${NAMROS_GATEWAY_LOG:-}"
NAMROS_SBS_SERVICE_ENDPOINT="${NAMROS_SBS_SERVICE_ENDPOINT:-${NAMROS_18NODE_SBS_SERVICE_ENDPOINT:-}}"
NAMROS_SBS_DATA_ENDPOINT="${NAMROS_SBS_DATA_ENDPOINT:-${NAMROS_18NODE_SBS_DATA_ENDPOINT:-}}"

require_cmd "$GO"
require_cmd curl

if [ -z "$NAMROS_SBS_SERVICE_ENDPOINT" ] && compat_should_use_18node_sbs; then
	NAMROS_SBS_SERVICE_ENDPOINT="$(compat_resolve_18node_sbs_service_endpoint)"
fi

require_env() {
	local name="$1"
	if [ -z "${!name:-}" ]; then
		die "$name is required"
	fi
}

add_nonempty_flag() {
	local flag="$1"
	local value="$2"
	if [ -n "$value" ]; then
		gateway_args+=("$flag" "$value")
	fi
}

listen_port="${NAMROS_GATEWAY_LISTEN##*:}"
if [ -z "$USER_NAMROS_ENDPOINT" ]; then
	NAMROS_ENDPOINT="http://$NAMROS_GATEWAY_CLIENT_HOST:$listen_port"
fi
export NAMROS_ENDPOINT

require_env NAMROS_SBS_SERVICE_ENDPOINT
require_env NAMROS_SBS_DATA_ENDPOINT
compat_ensure_18node_sbs_volume "$NAMROS_SBS_VOLUME_ID"

tmpdir="$(make_tmpdir)"
gateway_pid=""

cleanup() {
	local status=$?
	if [ -n "$gateway_pid" ] && kill -0 "$gateway_pid" >/dev/null 2>&1; then
		kill "$gateway_pid" >/dev/null 2>&1 || true
		wait "$gateway_pid" >/dev/null 2>&1 || true
	fi
	if compat_should_keep_tmp "$status"; then
		log "kept tmpdir: $tmpdir"
	else
		rm -rf "$tmpdir"
	fi
	exit "$status"
}

trap cleanup EXIT

dump_gateway_log() {
	compat_dump_file_tail "$NAMROS_GATEWAY_LOG" "${NAMROS_GATEWAY_LOG_TAIL_LINES:-120}" "gateway log"
}

gateway_bin="$tmpdir/namros-gateway"
gateway_args=(
	"$gateway_bin"
	-listen "$NAMROS_GATEWAY_LISTEN"
	-region "$NAMROS_REGION"
	-metadata-backend "$NAMROS_METADATA_BACKEND"
	-storage-backend sbs-physical
	-sbs-service-endpoint "$NAMROS_SBS_SERVICE_ENDPOINT"
	-sbs-data-endpoint "$NAMROS_SBS_DATA_ENDPOINT"
	-sbs-gateway-id "$NAMROS_SBS_GATEWAY_ID"
	-sbs-verify-readback="$NAMROS_SBS_VERIFY_READBACK"
	-root-access-key-id "$NAMROS_ACCESS_KEY_ID"
	-root-secret-access-key "$NAMROS_SECRET_ACCESS_KEY"
)

case "$NAMROS_METADATA_BACKEND" in
memory)
	;;
pebble)
	if [ -z "$NAMROS_METADATA_PATH" ]; then
		NAMROS_METADATA_PATH="$tmpdir/meta"
	fi
	gateway_args+=(-metadata-path "$NAMROS_METADATA_PATH")
	;;
tikv)
	gateway_args+=(
		-tikv-pd-endpoints "$NAMROS_TIKV_PD_ENDPOINTS"
		-tikv-api-version "$NAMROS_TIKV_API_VERSION"
		-tikv-keyspace "$NAMROS_TIKV_KEYSPACE"
	)
	add_nonempty_flag -tikv-timeout "$NAMROS_TIKV_TIMEOUT"
	add_nonempty_flag -tikv-retry-attempts "$NAMROS_TIKV_RETRY_ATTEMPTS"
	add_nonempty_flag -tikv-retry-initial-backoff "$NAMROS_TIKV_RETRY_INITIAL_BACKOFF"
	add_nonempty_flag -tikv-retry-max-backoff "$NAMROS_TIKV_RETRY_MAX_BACKOFF"
	add_nonempty_flag -tikv-tls-ca "$NAMROS_TIKV_TLS_CA"
	add_nonempty_flag -tikv-tls-cert "$NAMROS_TIKV_TLS_CERT"
	add_nonempty_flag -tikv-tls-key "$NAMROS_TIKV_TLS_KEY"
	;;
*)
	die "unsupported NAMROS_METADATA_BACKEND for sbs-physical compat smoke: $NAMROS_METADATA_BACKEND"
	;;
esac

add_nonempty_flag -sbs-volume-id "$NAMROS_SBS_VOLUME_ID"
add_nonempty_flag -sbs-chunk-size-bytes "$NAMROS_SBS_CHUNK_SIZE_BYTES"
add_nonempty_flag -sbs-attachment-id "$NAMROS_SBS_ATTACHMENT_ID"
add_nonempty_flag -sbs-generation "$NAMROS_SBS_GENERATION"

if [ -z "$NAMROS_GATEWAY_LOG" ]; then
	NAMROS_GATEWAY_LOG="$tmpdir/namros-gateway.log"
fi

if curl -fsS "$NAMROS_ENDPOINT/readyz" >/dev/null 2>&1; then
	die "gateway endpoint is already ready before smoke start: $NAMROS_ENDPOINT; stop the existing gateway or set NAMROS_GATEWAY_LISTEN/NAMROS_ENDPOINT to a free port"
fi

log "start sbs-physical gateway: listen=$NAMROS_GATEWAY_LISTEN endpoint=$NAMROS_ENDPOINT metadata=$NAMROS_METADATA_BACKEND"
if [ "$NAMROS_METADATA_BACKEND" = "tikv" ]; then
	log "tikv metadata: pd=$NAMROS_TIKV_PD_ENDPOINTS api=$NAMROS_TIKV_API_VERSION keyspace=$NAMROS_TIKV_KEYSPACE"
fi
log "sbs physical endpoints: service=$NAMROS_SBS_SERVICE_ENDPOINT data=$NAMROS_SBS_DATA_ENDPOINT"

log "build namros-gateway: $gateway_bin"
"$GO" build -o "$gateway_bin" ./cmd/namros-gateway >"$NAMROS_GATEWAY_LOG" 2>&1

"${gateway_args[@]}" >>"$NAMROS_GATEWAY_LOG" 2>&1 &
gateway_pid=$!

deadline=$((SECONDS + NAMROS_GATEWAY_START_TIMEOUT_SECONDS))
while [ "$SECONDS" -lt "$deadline" ]; do
	if ! kill -0 "$gateway_pid" >/dev/null 2>&1; then
		wait "$gateway_pid" >/dev/null 2>&1 || true
		dump_gateway_log
		die "namros-gateway exited before readiness; see $NAMROS_GATEWAY_LOG"
	fi
	if curl -fsS "$NAMROS_ENDPOINT/readyz" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

if ! curl -fsS "$NAMROS_ENDPOINT/readyz" >/dev/null 2>&1; then
	dump_gateway_log
	die "namros-gateway did not become ready within ${NAMROS_GATEWAY_START_TIMEOUT_SECONDS}s; see $NAMROS_GATEWAY_LOG"
fi

log "gateway ready; run user-space compatibility smoke"
export NAMROS_COMPAT_AWS_CLI_READ_TIMEOUT="${NAMROS_COMPAT_AWS_CLI_READ_TIMEOUT:-300}"
export NAMROS_COMPAT_CLIENT_LARGE_OBJECT_MIB="${NAMROS_COMPAT_CLIENT_LARGE_OBJECT_MIB:-1}"
bash "$SCRIPT_DIR/run-user-space-smoke.sh"
log "sbs-physical user-space compatibility smoke passed"

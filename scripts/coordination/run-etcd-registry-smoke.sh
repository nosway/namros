#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
. "$REPO_ROOT/scripts/compat/common.sh"

GO="${GO:-go}"
NAMROS_ETCD_ENDPOINTS="${NAMROS_ETCD_ENDPOINTS:-${NAMROS_18NODE_ETCD_ENDPOINTS:-127.0.0.1:12379}}"
NAMROS_GATEWAY_LISTEN="${NAMROS_GATEWAY_LISTEN:-127.0.0.1:19090}"
NAMROS_GATEWAY_INSTANCE_ID="${NAMROS_GATEWAY_INSTANCE_ID:-namros-smoke-$(date +%Y%m%d%H%M%S)-$$}"
NAMROS_GATEWAY_REGISTRY_PREFIX="${NAMROS_GATEWAY_REGISTRY_PREFIX:-${NAMROS_18NODE_GATEWAY_REGISTRY_PREFIX:-/namros/gateway-smoke}}"
NAMROS_GATEWAY_LEASE_TTL="${NAMROS_GATEWAY_LEASE_TTL:-3s}"
NAMROS_GATEWAY_HEARTBEAT="${NAMROS_GATEWAY_HEARTBEAT:-1s}"
NAMROS_GATEWAY_START_TIMEOUT_SECONDS="${NAMROS_GATEWAY_START_TIMEOUT_SECONDS:-30}"
NAMROS_GATEWAY_LEASE_EXPIRE_TIMEOUT_SECONDS="${NAMROS_GATEWAY_LEASE_EXPIRE_TIMEOUT_SECONDS:-30}"
NAMROS_GATEWAY_LOG="${NAMROS_GATEWAY_LOG:-}"
NAMROS_KEEP_TMP="${NAMROS_KEEP_TMP:-0}"

require_cmd "$GO"
require_cmd curl
require_cmd etcdctl
require_cmd jq

tmpdir="$(make_tmpdir)"
gateway_bin="$tmpdir/namros-gateway"
gateway_pid=""
registry_key="${NAMROS_GATEWAY_REGISTRY_PREFIX%/}/$NAMROS_GATEWAY_INSTANCE_ID"
record_json="$tmpdir/registry-record.json"
record_value="$tmpdir/registry-record-value.json"

cleanup() {
	local status=$?
	if [ -n "$gateway_pid" ] && kill -0 "$gateway_pid" >/dev/null 2>&1; then
		kill "$gateway_pid" >/dev/null 2>&1 || true
		sleep 1
		if kill -0 "$gateway_pid" >/dev/null 2>&1; then
			kill -9 "$gateway_pid" >/dev/null 2>&1 || true
		fi
		wait "$gateway_pid" >/dev/null 2>&1 || true
	fi
	if [ "${NAMROS_KEEP_COORDINATION_KEY:-0}" != "1" ]; then
		etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" del "$registry_key" >/dev/null 2>&1 || true
	fi
	if [ "$NAMROS_KEEP_TMP" != "1" ]; then
		rm -rf "$tmpdir"
	else
		log "kept tmpdir: $tmpdir"
	fi
	exit "$status"
}

trap cleanup EXIT

log "build gateway binary"
mkdir -p "$REPO_ROOT/.cache/go-build" "$REPO_ROOT/.cache/go-mod"
GOCACHE="${GOCACHE:-$REPO_ROOT/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$REPO_ROOT/.cache/go-mod}" "$GO" build -o "$gateway_bin" ./cmd/namros-gateway

if [ -z "$NAMROS_GATEWAY_LOG" ]; then
	NAMROS_GATEWAY_LOG="$tmpdir/namros-gateway.log"
fi

log "clear previous smoke key: $registry_key"
etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" del "$registry_key" >/dev/null

log "start gateway with etcd registry: endpoint=$NAMROS_ETCD_ENDPOINTS key=$registry_key ttl=$NAMROS_GATEWAY_LEASE_TTL"
"$gateway_bin" \
	-listen "$NAMROS_GATEWAY_LISTEN" \
	-region "$NAMROS_REGION" \
	-metadata-backend memory \
	-storage-backend memory \
	-coordination-backend etcd \
	-etcd-endpoints "$NAMROS_ETCD_ENDPOINTS" \
	-gateway-instance-id "$NAMROS_GATEWAY_INSTANCE_ID" \
	-gateway-advertise-endpoint "$NAMROS_GATEWAY_LISTEN" \
	-gateway-registry-prefix "$NAMROS_GATEWAY_REGISTRY_PREFIX" \
	-gateway-lease-ttl "$NAMROS_GATEWAY_LEASE_TTL" \
	-gateway-heartbeat "$NAMROS_GATEWAY_HEARTBEAT" \
	> "$NAMROS_GATEWAY_LOG" 2>&1 &
gateway_pid=$!

deadline=$((SECONDS + NAMROS_GATEWAY_START_TIMEOUT_SECONDS))
while [ "$SECONDS" -lt "$deadline" ]; do
	if ! kill -0 "$gateway_pid" >/dev/null 2>&1; then
		wait "$gateway_pid" >/dev/null 2>&1 || true
		die "namros-gateway exited before readiness; see $NAMROS_GATEWAY_LOG"
	fi
	if curl -fsS "http://$NAMROS_GATEWAY_LISTEN/readyz" >/dev/null 2>&1; then
		break
	fi
	sleep 1
done

curl -fsS "http://$NAMROS_GATEWAY_LISTEN/readyz" >/dev/null || die "namros-gateway did not become ready; see $NAMROS_GATEWAY_LOG"

deadline=$((SECONDS + NAMROS_GATEWAY_START_TIMEOUT_SECONDS))
while [ "$SECONDS" -lt "$deadline" ]; do
	etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$registry_key" --write-out json > "$record_json"
	if jq -e '(.count // 0) == 1 and (.kvs[0].lease // 0) > 0' "$record_json" >/dev/null; then
		etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$registry_key" --print-value-only > "$record_value"
		if jq -e --arg id "$NAMROS_GATEWAY_INSTANCE_ID" '.instance_id == $id and .healthy == true and .ready == true and .status == "ready"' "$record_value" >/dev/null; then
			break
		fi
	fi
	sleep 1
done

etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$registry_key" --write-out json > "$record_json"
jq -e '(.count // 0) == 1 and (.kvs[0].lease // 0) > 0' "$record_json" >/dev/null || die "registry key was not created with a lease: $registry_key"
lease_id="$(jq -r '.kvs[0].lease' "$record_json")"
etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$registry_key" --print-value-only > "$record_value"
jq -e --arg id "$NAMROS_GATEWAY_INSTANCE_ID" '.instance_id == $id and .healthy == true and .ready == true and .status == "ready"' "$record_value" >/dev/null || die "registry value is not a ready gateway record"
if etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" lease timetolive "$lease_id" > "$tmpdir/lease-ttl.txt" 2>/dev/null; then
	log "registry lease ttl: $(tr '\n' ' ' < "$tmpdir/lease-ttl.txt")"
fi

log "registry key is present with lease; terminate gateway without revoke to verify TTL expiry"
kill -9 "$gateway_pid" >/dev/null 2>&1 || true
wait "$gateway_pid" >/dev/null 2>&1 || true
gateway_pid=""

deadline=$((SECONDS + NAMROS_GATEWAY_LEASE_EXPIRE_TIMEOUT_SECONDS))
while [ "$SECONDS" -lt "$deadline" ]; do
	etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$registry_key" --write-out json > "$record_json"
	if jq -e '(.count // 0) == 0' "$record_json" >/dev/null; then
		log "registry lease expired and key disappeared"
		log "etcd registry smoke passed"
		exit 0
	fi
	sleep 1
done

die "registry key still exists after lease expiry timeout: $registry_key"

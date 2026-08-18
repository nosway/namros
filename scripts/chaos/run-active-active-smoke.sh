#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
. "$REPO_ROOT/scripts/compat/common.sh"

GO="${GO:-go}"

if [ "${NAMROS_ACTIVE_ACTIVE_LOAD_18NODE_ENV:-1}" = "1" ] &&
	[ -z "${NAMROS_TIKV_PD_ENDPOINTS:-}" ] &&
	[ -z "${NAMROS_18NODE_TIKV_PD_ENDPOINTS:-}" ] &&
	[ -f "$REPO_ROOT/scripts/lab/namros-18node-lab.env" ]; then
	compat_load_18node_lab_env
	log "loaded 18-node lab env: ${NAMROS_18NODE_ENV_FILE:-scripts/lab/namros-18node-lab.env}"
fi

NAMROS_ACTIVE_ACTIVE_LISTEN_A="${NAMROS_ACTIVE_ACTIVE_LISTEN_A:-127.0.0.1:19100}"
NAMROS_ACTIVE_ACTIVE_LISTEN_B="${NAMROS_ACTIVE_ACTIVE_LISTEN_B:-127.0.0.1:19101}"
NAMROS_ACTIVE_ACTIVE_ENDPOINT_A="${NAMROS_ACTIVE_ACTIVE_ENDPOINT_A:-http://$NAMROS_ACTIVE_ACTIVE_LISTEN_A}"
NAMROS_ACTIVE_ACTIVE_ENDPOINT_B="${NAMROS_ACTIVE_ACTIVE_ENDPOINT_B:-http://$NAMROS_ACTIVE_ACTIVE_LISTEN_B}"
NAMROS_METADATA_BACKEND="${NAMROS_METADATA_BACKEND:-tikv}"
NAMROS_TIKV_PD_ENDPOINTS="${NAMROS_TIKV_PD_ENDPOINTS:-${NAMROS_18NODE_TIKV_PD_ENDPOINTS:-127.0.0.1:2379}}"
NAMROS_TIKV_API_VERSION="${NAMROS_TIKV_API_VERSION:-${NAMROS_18NODE_TIKV_API_VERSION:-v1}}"
NAMROS_TIKV_KEYSPACE="${NAMROS_TIKV_KEYSPACE:-namros-active-active-$(date +%Y%m%d%H%M%S)-$$}"
NAMROS_ETCD_ENDPOINTS="${NAMROS_ETCD_ENDPOINTS:-${NAMROS_18NODE_ETCD_ENDPOINTS:-127.0.0.1:12379}}"
NAMROS_GATEWAY_REGISTRY_PREFIX="${NAMROS_GATEWAY_REGISTRY_PREFIX:-${NAMROS_18NODE_GATEWAY_REGISTRY_PREFIX:-/namros/active-active-smoke}}"
NAMROS_GATEWAY_LEASE_TTL="${NAMROS_GATEWAY_LEASE_TTL:-5s}"
NAMROS_GATEWAY_HEARTBEAT="${NAMROS_GATEWAY_HEARTBEAT:-1s}"
NAMROS_GATEWAY_START_TIMEOUT_SECONDS="${NAMROS_GATEWAY_START_TIMEOUT_SECONDS:-30}"
NAMROS_GATEWAY_FAILOVER_TIMEOUT_SECONDS="${NAMROS_GATEWAY_FAILOVER_TIMEOUT_SECONDS:-30}"

require_cmd "$GO"
require_cmd aws
require_cmd curl
require_cmd etcdctl
require_cmd jq

if [ "$NAMROS_METADATA_BACKEND" != "tikv" ]; then
	die "active-active smoke requires shared TiKV metadata; got NAMROS_METADATA_BACKEND=$NAMROS_METADATA_BACKEND"
fi

check_etcd_ready() {
	if ! etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" endpoint health >/dev/null 2>&1; then
		die "etcd endpoint is not healthy: NAMROS_ETCD_ENDPOINTS=$NAMROS_ETCD_ENDPOINTS"
	fi
}

check_pd_tcp_ready() {
	local endpoints endpoint hostport host port
	endpoints="${NAMROS_TIKV_PD_ENDPOINTS//,/ }"
	for endpoint in $endpoints; do
		hostport="${endpoint#http://}"
		hostport="${hostport#https://}"
		hostport="${hostport%%/*}"
		host="${hostport%:*}"
		port="${hostport##*:}"
		if [ -z "$host" ] || [ -z "$port" ] || [ "$host" = "$port" ]; then
			die "invalid TiKV PD endpoint: $endpoint"
		fi
		if (exec 3<>"/dev/tcp/$host/$port") >/dev/null 2>&1; then
			exec 3>&- 3<&-
			return 0
		fi
	done
	die "no TiKV PD endpoint is reachable: NAMROS_TIKV_PD_ENDPOINTS=$NAMROS_TIKV_PD_ENDPOINTS"
}

check_etcd_ready
check_pd_tcp_ready

tmpdir="$(make_tmpdir)"
gateway_bin="$tmpdir/namros-gateway"
gateway_a_pid=""
gateway_b_pid=""
gateway_a_log="$tmpdir/namros-gateway-a.log"
gateway_b_log="$tmpdir/namros-gateway-b.log"
registry_key_a="${NAMROS_GATEWAY_REGISTRY_PREFIX%/}/namros-active-a-$$"
registry_key_b="${NAMROS_GATEWAY_REGISTRY_PREFIX%/}/namros-active-b-$$"
bucket="$(compat_bucket active-active)"
NAMROS_STORAGE_PATH="${NAMROS_STORAGE_PATH:-$tmpdir/segments}"

cleanup() {
	local status=$?
	for pid in "$gateway_a_pid" "$gateway_b_pid"; do
		if [ -n "$pid" ] && kill -0 "$pid" >/dev/null 2>&1; then
			kill "$pid" >/dev/null 2>&1 || true
			wait "$pid" >/dev/null 2>&1 || true
		fi
	done
	etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" del "$registry_key_a" >/dev/null 2>&1 || true
	etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" del "$registry_key_b" >/dev/null 2>&1 || true
	if compat_should_keep_tmp "$status"; then
		log "kept tmpdir: $tmpdir"
	else
		rm -rf "$tmpdir"
	fi
	exit "$status"
}

trap cleanup EXIT

aws_s3api() {
	local endpoint="$1"
	shift
	env \
		AWS_ACCESS_KEY_ID="$NAMROS_ACCESS_KEY_ID" \
		AWS_SECRET_ACCESS_KEY="$NAMROS_SECRET_ACCESS_KEY" \
		AWS_DEFAULT_REGION="$NAMROS_REGION" \
		AWS_CONFIG_FILE="$tmpdir/aws-config" \
		AWS_EC2_METADATA_DISABLED=true \
		aws --endpoint-url "$endpoint" --region "$NAMROS_REGION" s3api "$@"
}

cleanup_bucket() {
	local endpoint="$1"
	local versions
	local objects

	if ! aws_s3api "$endpoint" head-bucket --bucket "$bucket" >/dev/null 2>&1; then
		return 0
	fi

	versions="$(aws_s3api "$endpoint" list-object-versions --bucket "$bucket" --output json 2>/dev/null || printf '{}')"
	while IFS=$'\t' read -r key version_id; do
		[ -n "$key" ] || continue
		aws_s3api "$endpoint" delete-object --bucket "$bucket" --key "$key" --version-id "$version_id" >/dev/null || true
	done < <(printf '%s' "$versions" | jq -r '.Versions[]? | [.Key, .VersionId] | @tsv')
	while IFS=$'\t' read -r key version_id; do
		[ -n "$key" ] || continue
		aws_s3api "$endpoint" delete-object --bucket "$bucket" --key "$key" --version-id "$version_id" >/dev/null || true
	done < <(printf '%s' "$versions" | jq -r '.DeleteMarkers[]? | [.Key, .VersionId] | @tsv')

	objects="$(aws_s3api "$endpoint" list-objects-v2 --bucket "$bucket" --output json 2>/dev/null || printf '{}')"
	while IFS= read -r key; do
		[ -n "$key" ] || continue
		aws_s3api "$endpoint" delete-object --bucket "$bucket" --key "$key" >/dev/null || true
	done < <(printf '%s' "$objects" | jq -r '.Contents[]?.Key')

	aws_s3api "$endpoint" delete-bucket --bucket "$bucket" >/dev/null || true
}

wait_ready() {
	local label="$1"
	local pid="$2"
	local endpoint="$3"
	local log_file="$4"

	local deadline=$((SECONDS + NAMROS_GATEWAY_START_TIMEOUT_SECONDS))
	while [ "$SECONDS" -lt "$deadline" ]; do
		if ! kill -0 "$pid" >/dev/null 2>&1; then
			wait "$pid" >/dev/null 2>&1 || true
			compat_dump_file_tail "$log_file" "${NAMROS_GATEWAY_LOG_TAIL_LINES:-120}" "$label gateway log"
			die "$label gateway exited before readiness; see $log_file"
		fi
		if curl -fsS "$endpoint/readyz" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	compat_dump_file_tail "$log_file" "${NAMROS_GATEWAY_LOG_TAIL_LINES:-120}" "$label gateway log"
	die "$label gateway did not become ready within ${NAMROS_GATEWAY_START_TIMEOUT_SECONDS}s; see $log_file"
}

assert_registry_ready() {
	local key="$1"
	local instance_id="$2"
	local output="$tmpdir/$(basename "$key").json"

	local deadline=$((SECONDS + NAMROS_GATEWAY_START_TIMEOUT_SECONDS))
	while [ "$SECONDS" -lt "$deadline" ]; do
		etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$key" --print-value-only >"$output"
		if jq -e --arg id "$instance_id" '.instance_id == $id and .healthy == true and .ready == true and .status == "ready"' "$output" >/dev/null 2>&1; then
			return 0
		fi
		sleep 1
	done
	die "registry key did not become ready: $key"
}

wait_registry_gone() {
	local key="$1"
	local output="$tmpdir/registry-gone.json"

	local deadline=$((SECONDS + NAMROS_GATEWAY_FAILOVER_TIMEOUT_SECONDS))
	while [ "$SECONDS" -lt "$deadline" ]; do
		etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" get "$key" --write-out json >"$output"
		if jq -e '(.count // 0) == 0' "$output" >/dev/null; then
			return 0
		fi
		sleep 1
	done
	die "registry key still exists after failover timeout: $key"
}

start_gateway() {
	local label="$1"
	local listen="$2"
	local endpoint="$3"
	local instance_id="$4"
	local registry_key="$5"
	local log_file="$6"

	log "start gateway $label: listen=$listen endpoint=$endpoint registry_key=$registry_key"
	"$gateway_bin" \
		-listen "$listen" \
		-region "$NAMROS_REGION" \
		-metadata-backend "$NAMROS_METADATA_BACKEND" \
		-tikv-pd-endpoints "$NAMROS_TIKV_PD_ENDPOINTS" \
		-tikv-api-version "$NAMROS_TIKV_API_VERSION" \
		-tikv-keyspace "$NAMROS_TIKV_KEYSPACE" \
		-storage-backend local \
		-storage-path "$NAMROS_STORAGE_PATH" \
		-coordination-backend etcd \
		-etcd-endpoints "$NAMROS_ETCD_ENDPOINTS" \
		-gateway-instance-id "$instance_id" \
		-gateway-advertise-endpoint "$endpoint" \
		-gateway-registry-prefix "$NAMROS_GATEWAY_REGISTRY_PREFIX" \
		-gateway-lease-ttl "$NAMROS_GATEWAY_LEASE_TTL" \
		-gateway-heartbeat "$NAMROS_GATEWAY_HEARTBEAT" \
		-root-access-key-id "$NAMROS_ACCESS_KEY_ID" \
		-root-secret-access-key "$NAMROS_SECRET_ACCESS_KEY" \
		>"$log_file" 2>&1 &
	printf '%s\n' "$!"
}

if curl -fsS "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_A/readyz" >/dev/null 2>&1; then
	die "gateway A endpoint is already ready before smoke start: $NAMROS_ACTIVE_ACTIVE_ENDPOINT_A"
fi
if curl -fsS "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B/readyz" >/dev/null 2>&1; then
	die "gateway B endpoint is already ready before smoke start: $NAMROS_ACTIVE_ACTIVE_ENDPOINT_B"
fi

cat >"$tmpdir/aws-config" <<EOF
[default]
region = $NAMROS_REGION
s3 =
    addressing_style = path
    signature_version = s3v4
EOF

log "build namros-gateway: $gateway_bin"
mkdir -p "$REPO_ROOT/.cache/go-build" "$REPO_ROOT/.cache/go-mod"
GOCACHE="${GOCACHE:-$REPO_ROOT/.cache/go-build}" GOMODCACHE="${GOMODCACHE:-$REPO_ROOT/.cache/go-mod}" "$GO" build -o "$gateway_bin" ./cmd/namros-gateway

log "active-active smoke metadata: tikv_pd=$NAMROS_TIKV_PD_ENDPOINTS api=$NAMROS_TIKV_API_VERSION keyspace=$NAMROS_TIKV_KEYSPACE"
log "active-active smoke coordination: etcd=$NAMROS_ETCD_ENDPOINTS prefix=$NAMROS_GATEWAY_REGISTRY_PREFIX ttl=$NAMROS_GATEWAY_LEASE_TTL heartbeat=$NAMROS_GATEWAY_HEARTBEAT"
log "active-active smoke storage: local path=$NAMROS_STORAGE_PATH"

etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" del "$registry_key_a" >/dev/null 2>&1 || true
etcdctl --endpoints "$NAMROS_ETCD_ENDPOINTS" del "$registry_key_b" >/dev/null 2>&1 || true

gateway_a_pid="$(start_gateway A "$NAMROS_ACTIVE_ACTIVE_LISTEN_A" "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_A" "$(basename "$registry_key_a")" "$registry_key_a" "$gateway_a_log")"
gateway_b_pid="$(start_gateway B "$NAMROS_ACTIVE_ACTIVE_LISTEN_B" "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B" "$(basename "$registry_key_b")" "$registry_key_b" "$gateway_b_log")"

wait_ready A "$gateway_a_pid" "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_A" "$gateway_a_log"
wait_ready B "$gateway_b_pid" "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B" "$gateway_b_log"
assert_registry_ready "$registry_key_a" "$(basename "$registry_key_a")"
assert_registry_ready "$registry_key_b" "$(basename "$registry_key_b")"

small="$tmpdir/small.txt"
small_from_b="$tmpdir/small-from-b.txt"
after_failover="$tmpdir/after-failover.txt"
after_failover_from_b="$tmpdir/after-failover-from-b.txt"
printf 'active-active write from gateway A\n' >"$small"
printf 'write after gateway A stopped\n' >"$after_failover"

log "write bucket/object through gateway A"
aws_s3api "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_A" create-bucket --bucket "$bucket" >/dev/null
aws_s3api "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_A" put-object --bucket "$bucket" --key small.txt --body "$small" >/dev/null

log "read/list gateway A object through gateway B"
aws_s3api "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B" get-object --bucket "$bucket" --key small.txt "$small_from_b" >/dev/null
assert_file_equals "$small" "$small_from_b" "active-active cross-gateway read"
aws_s3api "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B" list-objects-v2 --bucket "$bucket" --prefix small --output json >"$tmpdir/list-small.json"
jq -e 'any(.Contents[]?; .Key == "small.txt")' "$tmpdir/list-small.json" >/dev/null

log "terminate gateway A and verify gateway B remains usable"
kill "$gateway_a_pid" >/dev/null 2>&1 || true
wait "$gateway_a_pid" >/dev/null 2>&1 || true
gateway_a_pid=""
wait_registry_gone "$registry_key_a"
curl -fsS "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B/readyz" >/dev/null
aws_s3api "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B" put-object --bucket "$bucket" --key after-failover.txt --body "$after_failover" >/dev/null
aws_s3api "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B" get-object --bucket "$bucket" --key after-failover.txt "$after_failover_from_b" >/dev/null
assert_file_equals "$after_failover" "$after_failover_from_b" "post-failover object"
assert_registry_ready "$registry_key_b" "$(basename "$registry_key_b")"

cleanup_bucket "$NAMROS_ACTIVE_ACTIVE_ENDPOINT_B"
log "active-active failover smoke passed"

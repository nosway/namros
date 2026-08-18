#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
DEFAULT_CONFIG="$ROOT_DIR/packaging/k8s/production-kind.env"

action="${1:-render}"
config_file="${2:-${NAMROS_K8S_CONFIG:-$DEFAULT_CONFIG}}"

log() {
	printf '[k8s-production] %s\n' "$*" >&2
}

die() {
	printf '[k8s-production] ERROR: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat >&2 <<'USAGE'
Usage: scripts/k8s/deploy-production.sh ACTION [CONFIG]

Actions:
  write-values       Generate Helm values from CONFIG
  render             Generate values and render Helm manifests
  deploy             helm upgrade --install into the current Kubernetes context
  delete             helm uninstall the release
  status             kubectl status view for the release namespace
  build-images       Build local NAMROS/NAMRBD images through Docker Compose
  kind-up            Create the kind cluster when it does not exist
  kind-load-images   Load configured local images into kind
  kind-deploy        kind-up, optionally build/load images, then deploy
  kind-down          Delete the configured kind cluster
USAGE
}

repo_path() {
	case "$1" in
	/*)
		printf '%s\n' "$1"
		;;
	*)
		printf '%s\n' "$ROOT_DIR/$1"
		;;
	esac
}

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		die "missing required command: $1"
	fi
}

is_true() {
	case "${1:-}" in
	1|true|TRUE|True|yes|YES|Yes|y|Y)
		return 0
		;;
	*)
		return 1
		;;
	esac
}

require_uint() {
	name="$1"
	value="$2"
	case "$value" in
	''|*[!0-9]*)
		die "$name must be a non-negative integer: $value"
		;;
	esac
}

require_positive() {
	name="$1"
	value="$2"
	require_uint "$name" "$value"
	if [ "$value" -lt 1 ]; then
		die "$name must be greater than zero: $value"
	fi
}

yaml_quote() {
	local value
	value="${1//\\/\\\\}"
	value="${value//\"/\\\"}"
	printf '"%s"' "$value"
}

trim_value() {
	printf '%s' "$1" | tr -d ' \t\r\n'
}

csv_value_at() {
	local list="$1"
	local wanted_index="$2"
	local fallback="$3"
	local index=0
	local selected=""
	local old_ifs raw value
	old_ifs="$IFS"
	IFS=,
	for raw in $list; do
		IFS="$old_ifs"
		value="$(trim_value "$raw")"
		if [ -n "$value" ]; then
			if [ "$index" -eq "$wanted_index" ]; then
				selected="$value"
				break
			fi
			index=$((index + 1))
		fi
		IFS=,
	done
	IFS="$old_ifs"
	if [ -z "$selected" ]; then
		selected="$fallback"
	fi
	printf '%s' "$selected"
}

volume_id_at() {
	local index="$1"
	local explicit="${NAMROS_K8S_SBS_VOLUME_IDS:-}"
	if [ -n "$explicit" ]; then
		csv_value_at "$explicit" "$index" ""
		return 0
	fi
	printf '%s%1x\n' "$NAMROS_K8S_SBS_VOLUME_ID_PREFIX" "$((index + 1))"
}

load_config() {
	config_file="$(repo_path "$config_file")"
	if [ ! -f "$config_file" ]; then
		die "configuration file not found: $config_file"
	fi
	# The config file is intentionally shell-style KEY=VALUE for portability.
	set -a
	# shellcheck source=/dev/null
	. "$config_file"
	set +a

	NAMROS_K8S_RELEASE="${NAMROS_K8S_RELEASE:-namros}"
	NAMROS_K8S_NAMESPACE="${NAMROS_K8S_NAMESPACE:-namros}"
	NAMROS_K8S_CHART="${NAMROS_K8S_CHART:-packaging/helm/namros-community}"
	NAMROS_K8S_KIND_CLUSTER="${NAMROS_K8S_KIND_CLUSTER:-namros-production}"
	NAMROS_K8S_KIND_CONFIG="${NAMROS_K8S_KIND_CONFIG:-packaging/k8s/kind-production.yaml}"
	NAMROS_K8S_WORK_DIR="${NAMROS_K8S_WORK_DIR:-$ROOT_DIR/.cache/k8s-production}"
	NAMROS_K8S_VALUES_OUT="${NAMROS_K8S_VALUES_OUT:-$NAMROS_K8S_WORK_DIR/values.generated.yaml}"
	NAMROS_K8S_MANIFEST_OUT="${NAMROS_K8S_MANIFEST_OUT:-$NAMROS_K8S_WORK_DIR/rendered.yaml}"
	NAMROS_K8S_DEPLOYMENT_PROFILE="${NAMROS_K8S_DEPLOYMENT_PROFILE:-production}"
	NAMROS_K8S_REGION="${NAMROS_K8S_REGION:-us-east-1}"
	NAMROS_K8S_METADATA_BACKEND="${NAMROS_K8S_METADATA_BACKEND:-tikv}"
	NAMROS_K8S_COORDINATION_BACKEND="${NAMROS_K8S_COORDINATION_BACKEND:-etcd}"
	NAMROS_K8S_EMBEDDED_TIKV="${NAMROS_K8S_EMBEDDED_TIKV:-true}"
	NAMROS_K8S_EMBEDDED_ETCD="${NAMROS_K8S_EMBEDDED_ETCD:-true}"
	NAMROS_K8S_GATEWAY_REPLICAS="${NAMROS_K8S_GATEWAY_REPLICAS:-2}"
	NAMROS_K8S_SBS_SERVICE_REPLICAS="${NAMROS_K8S_SBS_SERVICE_REPLICAS:-2}"
	NAMROS_K8S_SBS_DATA_REPLICAS="${NAMROS_K8S_SBS_DATA_REPLICAS:-5}"
	NAMROS_K8S_TIKV_REPLICAS="${NAMROS_K8S_TIKV_REPLICAS:-1}"
	NAMROS_K8S_PD_REPLICAS="${NAMROS_K8S_PD_REPLICAS:-1}"
	NAMROS_K8S_ETCD_REPLICAS="${NAMROS_K8S_ETCD_REPLICAS:-1}"
	NAMROS_K8S_GATEWAY_SERVICE_TYPE="${NAMROS_K8S_GATEWAY_SERVICE_TYPE:-NodePort}"
	NAMROS_K8S_GATEWAY_SERVICE_PORT="${NAMROS_K8S_GATEWAY_SERVICE_PORT:-9000}"
	NAMROS_K8S_GATEWAY_NODE_PORT="${NAMROS_K8S_GATEWAY_NODE_PORT:-30900}"
	NAMROS_K8S_IMAGE_TAG="${NAMROS_K8S_IMAGE_TAG:-local}"
	NAMROS_K8S_SBS_IMAGE_TAG="${NAMROS_K8S_SBS_IMAGE_TAG:-local}"
	NAMROS_K8S_GATEWAY_IMAGE="${NAMROS_K8S_GATEWAY_IMAGE:-namros-gateway}"
	NAMROS_K8S_TOOLS_IMAGE="${NAMROS_K8S_TOOLS_IMAGE:-namros-tools}"
	NAMROS_K8S_SBS_SERVICE_IMAGE="${NAMROS_K8S_SBS_SERVICE_IMAGE:-namros-sbs-service}"
	NAMROS_K8S_SBS_DATA_IMAGE="${NAMROS_K8S_SBS_DATA_IMAGE:-namros-sbs-data}"
	NAMROS_K8S_SBSCTL_IMAGE="${NAMROS_K8S_SBSCTL_IMAGE:-namros-sbsctl}"
	NAMROS_K8S_IMAGE_PULL_POLICY="${NAMROS_K8S_IMAGE_PULL_POLICY:-IfNotPresent}"
	NAMROS_K8S_ROOT_CREDENTIALS_CREATE="${NAMROS_K8S_ROOT_CREDENTIALS_CREATE:-true}"
	NAMROS_K8S_ROOT_CREDENTIALS_SECRET="${NAMROS_K8S_ROOT_CREDENTIALS_SECRET:-}"
	NAMROS_K8S_ROOT_ACCESS_KEY_ID="${NAMROS_K8S_ROOT_ACCESS_KEY_ID:-namrosroot}"
	NAMROS_K8S_ROOT_SECRET_ACCESS_KEY="${NAMROS_K8S_ROOT_SECRET_ACCESS_KEY:-namrosrootsecret}"
	NAMROS_K8S_CLUSTER_ID="${NAMROS_K8S_CLUSTER_ID:-namros-production}"
	NAMROS_K8S_SBS_CLUSTER_ID="${NAMROS_K8S_SBS_CLUSTER_ID:-namros-production-sbs}"
	NAMROS_K8S_SBS_VOLUME_POOL_ID="${NAMROS_K8S_SBS_VOLUME_POOL_ID:-production-pool}"
	NAMROS_K8S_SBS_VOLUME_POOL_GENERATION="${NAMROS_K8S_SBS_VOLUME_POOL_GENERATION:-0}"
	NAMROS_K8S_SBS_VOLUME_ID_PREFIX="${NAMROS_K8S_SBS_VOLUME_ID_PREFIX:-18d0000}"
	NAMROS_K8S_SBS_VOLUME_SIZE="${NAMROS_K8S_SBS_VOLUME_SIZE:-64M}"
	NAMROS_K8S_SBS_BLOCK_SIZE="${NAMROS_K8S_SBS_BLOCK_SIZE:-4K}"
	NAMROS_K8S_SBS_ALLOCATION_CHUNK_SIZE="${NAMROS_K8S_SBS_ALLOCATION_CHUNK_SIZE:-1M}"
	NAMROS_K8S_SBS_ALLOCATION_PAGE_SIZE="${NAMROS_K8S_SBS_ALLOCATION_PAGE_SIZE:-4M}"
	NAMROS_K8S_SBS_REPLICATION_FACTOR="${NAMROS_K8S_SBS_REPLICATION_FACTOR:-3}"
	NAMROS_K8S_SBS_CHUNK_SIZE_BYTES="${NAMROS_K8S_SBS_CHUNK_SIZE_BYTES:-1048576}"
	NAMROS_K8S_SBS_ATTACHMENT_ID="${NAMROS_K8S_SBS_ATTACHMENT_ID:-att-namros-production-kind-object-pool}"
	NAMROS_K8S_SBS_WRITER_GROUP_ID="${NAMROS_K8S_SBS_WRITER_GROUP_ID:-namros-production-kind-writers}"
	NAMROS_K8S_SBS_VOLUME_EPOCH="${NAMROS_K8S_SBS_VOLUME_EPOCH:-1}"
	NAMROS_K8S_SBS_ZONES="${NAMROS_K8S_SBS_ZONES:-zone-a,zone-b,zone-c}"
	NAMROS_K8S_KIND_BUILD_IMAGES="${NAMROS_K8S_KIND_BUILD_IMAGES:-true}"
	NAMROS_K8S_DEPLOY_TIMEOUT="${NAMROS_K8S_DEPLOY_TIMEOUT:-10m}"

	chart_dir="$(repo_path "$NAMROS_K8S_CHART")"
	kind_config="$(repo_path "$NAMROS_K8S_KIND_CONFIG")"
	values_out="$(repo_path "$NAMROS_K8S_VALUES_OUT")"
	manifest_out="$(repo_path "$NAMROS_K8S_MANIFEST_OUT")"
	work_dir="$(dirname -- "$values_out")"

	require_positive NAMROS_K8S_GATEWAY_REPLICAS "$NAMROS_K8S_GATEWAY_REPLICAS"
	require_positive NAMROS_K8S_SBS_SERVICE_REPLICAS "$NAMROS_K8S_SBS_SERVICE_REPLICAS"
	require_positive NAMROS_K8S_SBS_DATA_REPLICAS "$NAMROS_K8S_SBS_DATA_REPLICAS"
	require_positive NAMROS_K8S_TIKV_REPLICAS "$NAMROS_K8S_TIKV_REPLICAS"
	require_positive NAMROS_K8S_PD_REPLICAS "$NAMROS_K8S_PD_REPLICAS"
	require_positive NAMROS_K8S_ETCD_REPLICAS "$NAMROS_K8S_ETCD_REPLICAS"
	require_positive NAMROS_K8S_GATEWAY_SERVICE_PORT "$NAMROS_K8S_GATEWAY_SERVICE_PORT"
	if [ "$NAMROS_K8S_GATEWAY_SERVICE_TYPE" = "NodePort" ]; then
		require_positive NAMROS_K8S_GATEWAY_NODE_PORT "$NAMROS_K8S_GATEWAY_NODE_PORT"
	fi
}

write_values() {
	load_config
	mkdir -p "$work_dir"
	{
		printf 'deploymentProfile: %s\n' "$(yaml_quote "$NAMROS_K8S_DEPLOYMENT_PROFILE")"
		printf 'region: %s\n' "$(yaml_quote "$NAMROS_K8S_REGION")"
		printf '\nimages:\n'
		printf '  gateway:\n    repository: %s\n    tag: %s\n    pullPolicy: %s\n' "$(yaml_quote "$NAMROS_K8S_GATEWAY_IMAGE")" "$(yaml_quote "$NAMROS_K8S_IMAGE_TAG")" "$(yaml_quote "$NAMROS_K8S_IMAGE_PULL_POLICY")"
		printf '  tools:\n    repository: %s\n    tag: %s\n    pullPolicy: %s\n' "$(yaml_quote "$NAMROS_K8S_TOOLS_IMAGE")" "$(yaml_quote "$NAMROS_K8S_IMAGE_TAG")" "$(yaml_quote "$NAMROS_K8S_IMAGE_PULL_POLICY")"
		printf '  sbsService:\n    repository: %s\n    tag: %s\n    pullPolicy: %s\n' "$(yaml_quote "$NAMROS_K8S_SBS_SERVICE_IMAGE")" "$(yaml_quote "$NAMROS_K8S_SBS_IMAGE_TAG")" "$(yaml_quote "$NAMROS_K8S_IMAGE_PULL_POLICY")"
		printf '  sbsData:\n    repository: %s\n    tag: %s\n    pullPolicy: %s\n' "$(yaml_quote "$NAMROS_K8S_SBS_DATA_IMAGE")" "$(yaml_quote "$NAMROS_K8S_SBS_IMAGE_TAG")" "$(yaml_quote "$NAMROS_K8S_IMAGE_PULL_POLICY")"
		printf '  sbsctl:\n    repository: %s\n    tag: %s\n    pullPolicy: %s\n' "$(yaml_quote "$NAMROS_K8S_SBSCTL_IMAGE")" "$(yaml_quote "$NAMROS_K8S_SBS_IMAGE_TAG")" "$(yaml_quote "$NAMROS_K8S_IMAGE_PULL_POLICY")"
		printf '\nembedded:\n'
		printf '  etcd:\n    enabled: %s\n    replicas: %s\n' "$NAMROS_K8S_EMBEDDED_ETCD" "$NAMROS_K8S_ETCD_REPLICAS"
		printf '  tikv:\n    enabled: %s\n    pdReplicas: %s\n    tikvReplicas: %s\n' "$NAMROS_K8S_EMBEDDED_TIKV" "$NAMROS_K8S_PD_REPLICAS" "$NAMROS_K8S_TIKV_REPLICAS"
		printf '\nmetadata:\n  backend: %s\n  tikv:\n    pdEndpoints: %s\n' "$(yaml_quote "$NAMROS_K8S_METADATA_BACKEND")" "$(yaml_quote "${NAMROS_K8S_EXTERNAL_TIKV_PD_ENDPOINTS:-}")"
		printf '\ncoordination:\n  backend: %s\n  etcd:\n    endpoints: %s\n' "$(yaml_quote "$NAMROS_K8S_COORDINATION_BACKEND")" "$(yaml_quote "${NAMROS_K8S_EXTERNAL_ETCD_ENDPOINTS:-}")"
		printf '\nrootCredentials:\n'
		printf '  create: %s\n  existingSecret: %s\n  accessKeyId: %s\n  secretAccessKey: %s\n' "$NAMROS_K8S_ROOT_CREDENTIALS_CREATE" "$(yaml_quote "$NAMROS_K8S_ROOT_CREDENTIALS_SECRET")" "$(yaml_quote "$NAMROS_K8S_ROOT_ACCESS_KEY_ID")" "$(yaml_quote "$NAMROS_K8S_ROOT_SECRET_ACCESS_KEY")"
		printf '\ngateway:\n'
		printf '  replicas: %s\n  service:\n    type: %s\n    port: %s\n' "$NAMROS_K8S_GATEWAY_REPLICAS" "$(yaml_quote "$NAMROS_K8S_GATEWAY_SERVICE_TYPE")" "$NAMROS_K8S_GATEWAY_SERVICE_PORT"
		if [ "$NAMROS_K8S_GATEWAY_SERVICE_TYPE" = "NodePort" ]; then
			printf '    nodePort: %s\n' "$NAMROS_K8S_GATEWAY_NODE_PORT"
		fi
		printf '\nsbs:\n'
		printf '  clusterId: %s\n  sbsClusterId: %s\n' "$(yaml_quote "$NAMROS_K8S_CLUSTER_ID")" "$(yaml_quote "$NAMROS_K8S_SBS_CLUSTER_ID")"
		printf '  volumeIds:\n'
		for ((i = 0; i < NAMROS_K8S_SBS_DATA_REPLICAS; i++)); do
			volume_id="$(volume_id_at "$i")"
			if [ -z "$volume_id" ]; then
				die "missing volume id for SBS data index $i"
			fi
			printf '    - %s\n' "$(yaml_quote "$volume_id")"
		done
		printf '  volumePoolId: %s\n  volumePoolGeneration: %s\n' "$(yaml_quote "$NAMROS_K8S_SBS_VOLUME_POOL_ID")" "$NAMROS_K8S_SBS_VOLUME_POOL_GENERATION"
		printf '  volumeSize: %s\n  blockSize: %s\n  allocationChunkSize: %s\n  allocationPageSize: %s\n' "$(yaml_quote "$NAMROS_K8S_SBS_VOLUME_SIZE")" "$(yaml_quote "$NAMROS_K8S_SBS_BLOCK_SIZE")" "$(yaml_quote "$NAMROS_K8S_SBS_ALLOCATION_CHUNK_SIZE")" "$(yaml_quote "$NAMROS_K8S_SBS_ALLOCATION_PAGE_SIZE")"
		printf '  replicationFactor: %s\n  chunkSizeBytes: %s\n' "$NAMROS_K8S_SBS_REPLICATION_FACTOR" "$NAMROS_K8S_SBS_CHUNK_SIZE_BYTES"
		printf '  attachmentId: %s\n  writerGroupId: %s\n  volumeEpoch: %s\n' "$(yaml_quote "$NAMROS_K8S_SBS_ATTACHMENT_ID")" "$(yaml_quote "$NAMROS_K8S_SBS_WRITER_GROUP_ID")" "$NAMROS_K8S_SBS_VOLUME_EPOCH"
		printf '  service:\n    nodes:\n'
		for ((i = 1; i <= NAMROS_K8S_SBS_SERVICE_REPLICAS; i++)); do
			printf '      - id: %s\n' "$(yaml_quote "sbs-service-$i")"
		done
		printf '  data:\n    nodes:\n'
		for ((i = 1; i <= NAMROS_K8S_SBS_DATA_REPLICAS; i++)); do
			zone="$(csv_value_at "$NAMROS_K8S_SBS_ZONES" "$((i - 1))" "zone-a")"
			printf '      - id: %s\n        zone: %s\n' "$(yaml_quote "sbs-data-$i")" "$(yaml_quote "$zone")"
		done
	} >"$values_out"
	log "wrote values: $values_out"
}

render() {
	write_values
	require_cmd helm
	helm template "$NAMROS_K8S_RELEASE" "$chart_dir" \
		--namespace "$NAMROS_K8S_NAMESPACE" \
		-f "$values_out" >"$manifest_out"
	log "rendered manifests: $manifest_out"
}

deploy() {
	write_values
	require_cmd helm
	require_cmd kubectl
	kubectl create namespace "$NAMROS_K8S_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
	helm upgrade --install "$NAMROS_K8S_RELEASE" "$chart_dir" \
		--namespace "$NAMROS_K8S_NAMESPACE" \
		-f "$values_out" \
		--wait \
		--timeout "$NAMROS_K8S_DEPLOY_TIMEOUT"
}

delete_release() {
	load_config
	require_cmd helm
	helm uninstall "$NAMROS_K8S_RELEASE" --namespace "$NAMROS_K8S_NAMESPACE" || true
}

status() {
	load_config
	require_cmd kubectl
	kubectl -n "$NAMROS_K8S_NAMESPACE" get deploy,po,svc,job
}

build_images() {
	load_config
	require_cmd docker
	sh "$ROOT_DIR/scripts/container/ensure-local-files.sh"
	docker compose --env-file "$ROOT_DIR/packaging/docker/.env" \
		-f "$ROOT_DIR/packaging/docker/compose.sbs-quickstart.yml" \
		--profile sbs-quickstart \
		build \
		sbs-quickstart-gateway \
		sbs-quickstart-tools \
		sbs-quickstart-service \
		sbs-quickstart-data-1 \
		sbs-quickstart-bootstrap
}

kind_up() {
	load_config
	require_cmd kind
	if kind get clusters | grep -Fx "$NAMROS_K8S_KIND_CLUSTER" >/dev/null 2>&1; then
		log "kind cluster already exists: $NAMROS_K8S_KIND_CLUSTER"
		return 0
	fi
	kind create cluster --name "$NAMROS_K8S_KIND_CLUSTER" --config "$kind_config"
}

kind_load_images() {
	load_config
	require_cmd kind
	require_cmd docker
	local images=(
		"$NAMROS_K8S_GATEWAY_IMAGE:$NAMROS_K8S_IMAGE_TAG"
		"$NAMROS_K8S_TOOLS_IMAGE:$NAMROS_K8S_IMAGE_TAG"
		"$NAMROS_K8S_SBS_SERVICE_IMAGE:$NAMROS_K8S_SBS_IMAGE_TAG"
		"$NAMROS_K8S_SBS_DATA_IMAGE:$NAMROS_K8S_SBS_IMAGE_TAG"
		"$NAMROS_K8S_SBSCTL_IMAGE:$NAMROS_K8S_SBS_IMAGE_TAG"
	)
	for image in "${images[@]}"; do
		log "load image into kind: $image"
		kind load docker-image --name "$NAMROS_K8S_KIND_CLUSTER" "$image"
	done
}

kind_deploy() {
	load_config
	kind_up
	if is_true "$NAMROS_K8S_KIND_BUILD_IMAGES"; then
		build_images
	fi
	kind_load_images
	deploy
}

kind_down() {
	load_config
	require_cmd kind
	kind delete cluster --name "$NAMROS_K8S_KIND_CLUSTER"
}

case "$action" in
write-values)
	write_values
	;;
render)
	render
	;;
deploy)
	deploy
	;;
delete)
	delete_release
	;;
status)
	status
	;;
build-images)
	build_images
	;;
kind-up)
	kind_up
	;;
kind-load-images)
	kind_load_images
	;;
kind-deploy)
	kind_deploy
	;;
kind-down)
	kind_down
	;;
help|-h|--help)
	usage
	;;
*)
	usage
	die "unknown action: $action"
	;;
esac

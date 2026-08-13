#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"
chart_dir="$repo_root/packaging/helm/namros-community"

fail() {
	printf '[helm-chart-check] ERROR: %s\n' "$*" >&2
	exit 1
}

log() {
	printf '[helm-chart-check] %s\n' "$*" >&2
}

require_file() {
	path="$1"
	if [ ! -f "$repo_root/$path" ]; then
		fail "missing required file: $path"
	fi
}

require_pattern() {
	file="$1"
	pattern="$2"
	description="$3"
	if ! grep -Eq -- "$pattern" "$repo_root/$file"; then
		fail "$description missing in $file"
	fi
}

for path in \
	packaging/helm/namros-community/Chart.yaml \
	packaging/helm/namros-community/values.yaml \
	packaging/helm/namros-community/values.local.yaml \
	packaging/helm/namros-community/values.production.yaml \
	packaging/helm/namros-community/templates/_helpers.tpl \
	packaging/helm/namros-community/templates/gateway.yaml \
	packaging/helm/namros-community/templates/jobs.yaml \
	packaging/helm/namros-community/templates/sbs.yaml \
	packaging/helm/namros-community/templates/etcd.yaml \
	packaging/helm/namros-community/templates/tikv.yaml; do
	require_file "$path"
done

require_pattern packaging/helm/namros-community/Chart.yaml '^apiVersion: v2$' 'Helm v2 chart apiVersion'
require_pattern packaging/helm/namros-community/Chart.yaml '^name: namros-community$' 'chart name'
require_pattern packaging/helm/namros-community/values.yaml '^deploymentProfile: production$' 'default production deployment profile'
require_pattern packaging/helm/namros-community/values.yaml 'volumePoolId: object-pool' 'default SBS volume pool id'
require_pattern packaging/helm/namros-community/values.yaml 'writerGroupId: namros-community-writers' 'default SBS writer group'
require_pattern packaging/helm/namros-community/values.yaml 'volumeIds:' 'SBS volume list'
require_pattern packaging/helm/namros-community/values.local.yaml 'enabled: true' 'local embedded dependency profile'
require_pattern packaging/helm/namros-community/values.production.yaml 'enabled: false' 'production external dependency profile'
require_pattern packaging/helm/namros-community/values.production.yaml 'existingSecret: namros-root-credentials' 'production existing secret profile'
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'NAMROS_DEPLOYMENT_PROFILE' 'gateway deployment profile env'
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'NAMROS_GC_CANDIDATE_QUEUE' 'gateway metadata GC queue env'
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'NAMROS_SBS_VOLUME_POOL_ID' 'gateway volume-pool env'
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'NAMROS_SBS_ATTACHMENT_ID' 'gateway shared attachment env'
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'NAMROS_SBS_WRITER_GROUP_ID' 'gateway writer group env'
require_pattern packaging/helm/namros-community/templates/gateway.yaml '/readyz' 'gateway readiness probe'
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'NAMROS_SBS_DATA_ENDPOINT' 'gateway SBS data endpoint env'
require_pattern packaging/helm/namros-community/templates/gateway.yaml '\$data0\.id' 'gateway SBS data endpoint pins first data node service'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'NAMROS_COMMUNITY_SBS_DATA_ENDPOINTS' 'pool bootstrap SBS per-volume data endpoints env'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'namros-container-community-bootstrap' 'SBS bootstrap job'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'namros-container-volume-pool-bootstrap' 'volume-pool bootstrap job'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'helm.sh/hook-weight: "10"' 'SBS bootstrap hook order'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'helm.sh/hook-weight: "20"' 'pool bootstrap hook order'
require_pattern packaging/helm/namros-community/templates/sbs.yaml '--grpc-listen=0\.0\.0\.0:9444' 'SBS data 9444 listener'
require_pattern packaging/helm/namros-community/templates/sbs.yaml 'app.kubernetes.io/component: sbs-service' 'SBS service workload'
require_pattern packaging/helm/namros-community/templates/sbs.yaml 'app.kubernetes.io/component: sbs-data' 'SBS data workload'
require_pattern packaging/helm/namros-community/templates/_helpers.tpl 'metadata\.tikv\.pdEndpoints is required' 'production TiKV endpoint requirement'
require_pattern packaging/helm/namros-community/templates/_helpers.tpl 'coordination\.etcd\.endpoints is required' 'production etcd endpoint requirement'

if command -v helm >/dev/null 2>&1; then
	log "run helm lint"
	helm lint "$chart_dir"
	log "render local values"
	helm template namros "$chart_dir" -f "$chart_dir/values.local.yaml" >/dev/null
	log "render production values"
	helm template namros "$chart_dir" -f "$chart_dir/values.production.yaml" >/dev/null
else
	log "helm CLI not found; static chart contract checks passed"
fi

log "passed"

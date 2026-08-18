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
	packaging/helm/namros-community/templates/tikv.yaml \
	packaging/helm/namros-community/templates/servicemonitor.yaml \
	packaging/helm/namros-community/templates/podmonitor.yaml \
	packaging/helm/namros-community/templates/tests/gateway-smoke.yaml \
	packaging/k8s/production-kind.env \
	packaging/k8s/kind-production.yaml \
	scripts/k8s/deploy-production.sh; do
	require_file "$path"
done

require_pattern packaging/helm/namros-community/Chart.yaml '^apiVersion: v2$' 'Helm v2 chart apiVersion'
require_pattern packaging/helm/namros-community/Chart.yaml '^name: namros-community$' 'chart name'
require_pattern packaging/helm/namros-community/Chart.yaml '^home: https://github\.com/nosway/namros$' 'chart home'
require_pattern packaging/helm/namros-community/Chart.yaml '^icon: https://raw\.githubusercontent\.com/nosway/namros/main/docs-src/assets/namros-icon\.svg$' 'chart icon'
require_pattern packaging/helm/namros-community/Chart.yaml '^sources:$' 'chart sources'
require_pattern packaging/helm/namros-community/Chart.yaml '^maintainers:$' 'chart maintainers'
require_pattern packaging/helm/namros-community/Chart.yaml '^keywords:$' 'chart keywords'
require_pattern packaging/helm/namros-community/Chart.yaml 'artifacthub\.io/category: storage' 'Artifact Hub category annotation'
require_pattern docs-src/assets/namros-icon.svg '<svg' 'chart icon asset'
require_pattern packaging/helm/namros-community/values.yaml '^deploymentProfile: production$' 'default production deployment profile'
require_pattern packaging/helm/namros-community/values.yaml 'tikvReplicas: 1' 'default embedded TiKV replica count'
require_pattern packaging/helm/namros-community/values.yaml 'volumePoolId: object-pool' 'default SBS volume pool id'
require_pattern packaging/helm/namros-community/values.yaml 'writerGroupId: namros-community-writers' 'default SBS writer group'
require_pattern packaging/helm/namros-community/values.yaml 'volumeIds:' 'SBS volume list'
require_pattern packaging/helm/namros-community/values.yaml 'id: sbs-data-5' 'default fifth SBS data node'
require_pattern packaging/helm/namros-community/values.yaml '^monitoring:' 'monitoring values'
require_pattern packaging/helm/namros-community/values.yaml '^tests:' 'Helm test values'
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
require_pattern packaging/helm/namros-community/templates/gateway.yaml 'nodePort:' 'gateway NodePort support'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'NAMROS_COMMUNITY_SBS_DATA_ENDPOINTS' 'pool bootstrap SBS per-volume data endpoints env'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'NAMROS_COMMUNITY_SBS_DATA_NODE_IDS' 'SBS bootstrap dynamic data node ids env'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'namros\.sbsDataGRPCEndpoints' 'SBS bootstrap dynamic data endpoints helper'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'namros-container-community-bootstrap' 'SBS bootstrap job'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'namros-container-volume-pool-bootstrap' 'volume-pool bootstrap job'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'helm.sh/hook-weight: "10"' 'SBS bootstrap hook order'
require_pattern packaging/helm/namros-community/templates/jobs.yaml 'helm.sh/hook-weight: "20"' 'pool bootstrap hook order'
require_pattern packaging/helm/namros-community/templates/sbs.yaml '--grpc-listen=0\.0\.0\.0:9444' 'SBS data 9444 listener'
require_pattern packaging/helm/namros-community/templates/sbs.yaml 'app.kubernetes.io/component: sbs-service' 'SBS service workload'
require_pattern packaging/helm/namros-community/templates/sbs.yaml 'app.kubernetes.io/component: sbs-data' 'SBS data workload'
require_pattern packaging/helm/namros-community/templates/_helpers.tpl 'metadata\.tikv\.pdEndpoints is required' 'production TiKV endpoint requirement'
require_pattern packaging/helm/namros-community/templates/_helpers.tpl 'coordination\.etcd\.endpoints is required' 'production etcd endpoint requirement'
require_pattern packaging/helm/namros-community/templates/servicemonitor.yaml '^kind: ServiceMonitor$' 'ServiceMonitor template'
require_pattern packaging/helm/namros-community/templates/servicemonitor.yaml 'monitoring\.serviceMonitor\.enabled' 'ServiceMonitor enable guard'
require_pattern packaging/helm/namros-community/templates/podmonitor.yaml '^kind: PodMonitor$' 'PodMonitor template'
require_pattern packaging/helm/namros-community/templates/podmonitor.yaml 'monitoring\.podMonitor\.enabled' 'PodMonitor enable guard'
require_pattern packaging/helm/namros-community/templates/tests/gateway-smoke.yaml 'helm\.sh/hook": test' 'Helm test hook'
require_pattern packaging/helm/namros-community/templates/tests/gateway-smoke.yaml '/readyz' 'Helm gateway readiness smoke'
require_pattern packaging/k8s/production-kind.env '^NAMROS_K8S_GATEWAY_REPLICAS=2$' 'production-kind default gateway count'
require_pattern packaging/k8s/production-kind.env '^NAMROS_K8S_SBS_SERVICE_REPLICAS=2$' 'production-kind default SBS service count'
require_pattern packaging/k8s/production-kind.env '^NAMROS_K8S_SBS_DATA_REPLICAS=5$' 'production-kind default SBS data count'
require_pattern packaging/k8s/production-kind.env '^NAMROS_K8S_TIKV_REPLICAS=1$' 'production-kind default TiKV count'
require_pattern packaging/k8s/kind-production.yaml 'hostPort: 9000' 'kind gateway host port mapping'
require_pattern scripts/k8s/deploy-production.sh 'write-values' 'K8s production config writer action'
require_pattern scripts/k8s/deploy-production.sh 'kind-deploy' 'kind production deploy action'
require_pattern scripts/k8s/deploy-production.sh 'NAMROS_K8S_SBS_DATA_REPLICAS' 'K8s production SBS data replica config'

if ! bash -n "$repo_root/scripts/k8s/deploy-production.sh"; then
	fail "K8s production deploy script has a syntax error"
fi

k8s_check_dir="$repo_root/.cache/helm-chart-check/k8s-production"
NAMROS_K8S_WORK_DIR="$k8s_check_dir" "$repo_root/scripts/k8s/deploy-production.sh" write-values "$repo_root/packaging/k8s/production-kind.env" >/dev/null
require_pattern .cache/helm-chart-check/k8s-production/values.generated.yaml '^  replicas: 2$' 'generated K8s gateway replica count'
require_pattern .cache/helm-chart-check/k8s-production/values.generated.yaml 'id: "?sbs-service-2"?' 'generated K8s second SBS service'
require_pattern .cache/helm-chart-check/k8s-production/values.generated.yaml 'id: "?sbs-data-5"?' 'generated K8s fifth SBS data node'
require_pattern .cache/helm-chart-check/k8s-production/values.generated.yaml 'tikvReplicas: 1' 'generated K8s TiKV replica count'

if command -v helm >/dev/null 2>&1; then
	log "run helm lint"
	helm lint "$chart_dir"
	log "render local values"
	helm template namros "$chart_dir" -f "$chart_dir/values.local.yaml" >/dev/null
	log "render production values"
	helm template namros "$chart_dir" -f "$chart_dir/values.production.yaml" >/dev/null
	log "render monitoring templates"
	helm template namros "$chart_dir" \
		--set monitoring.serviceMonitor.enabled=true \
		--set monitoring.podMonitor.enabled=true >/dev/null
	log "render production-kind generated values"
	helm template namros "$chart_dir" -f "$k8s_check_dir/values.generated.yaml" >/dev/null
else
	if is_true "${NAMROS_HELM_REQUIRE_HELM:-false}"; then
		fail "helm CLI is required when NAMROS_HELM_REQUIRE_HELM=true"
	fi
	log "helm CLI not found; static chart contract checks passed"
fi

log "passed"

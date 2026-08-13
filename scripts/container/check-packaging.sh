#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"

fail() {
	printf '[container-packaging-check] ERROR: %s\n' "$*" >&2
	exit 1
}

require_pattern() {
	file="$1"
	pattern="$2"
	description="$3"
	if ! grep -Eq "$pattern" "$repo_root/$file"; then
		fail "$description missing in $file"
	fi
}

reject_pattern() {
	file="$1"
	pattern="$2"
	description="$3"
	if grep -Eq "$pattern" "$repo_root/$file"; then
		fail "$description found in $file"
	fi
}

require_executable() {
	file="$1"
	description="$2"
	if [ ! -x "$repo_root/$file" ]; then
		fail "$description is not executable: $file"
	fi
}

require_pattern packaging/docker/Dockerfile.gateway '^USER 65532:65532$' 'non-root runtime user'
require_pattern packaging/docker/Dockerfile.gateway '^HEALTHCHECK .*readyz' 'gateway Dockerfile healthcheck'
require_pattern packaging/docker/Dockerfile.sbs '^FROM sbs-runtime-base AS sbs-service$' 'SBS service runtime stage'
require_pattern packaging/docker/Dockerfile.sbs '^FROM sbs-runtime-base AS sbs-data$' 'SBS data runtime stage'
require_pattern packaging/docker/Dockerfile.sbs '^HEALTHCHECK .*9081/readyz' 'SBS service Dockerfile healthcheck'
require_pattern packaging/docker/Dockerfile.sbs '^HEALTHCHECK .*9082/readyz' 'SBS data Dockerfile healthcheck'
require_pattern packaging/docker/Dockerfile.tools 'namros-admin' 'tools image namros-admin binary'
require_pattern packaging/docker/Dockerfile.tools 'namros-ops-report' 'tools image namros-ops-report binary'
require_pattern packaging/docker/Dockerfile.tools 'namros-s3bench' 'tools image namros-s3bench binary'
require_pattern packaging/docker/Dockerfile.tools 'namros-container-community-smoke' 'tools image Community smoke helper'
require_pattern packaging/docker/Dockerfile.tools 'namros-container-volume-pool-bootstrap' 'tools image volume-pool bootstrap helper'
require_pattern packaging/docker/Dockerfile.tools '^USER 65532:65532$' 'tools non-root runtime user'
require_pattern packaging/docker/compose.community.yml 'namros-pool-bootstrap' 'Community compose volume-pool bootstrap service'
require_pattern packaging/docker/compose.community.yml 'namros-gateway-a' 'Community compose first gateway'
require_pattern packaging/docker/compose.community.yml 'namros-gateway-b' 'Community compose second gateway'
require_pattern packaging/docker/compose.community.yml 'nofile:' 'Community TiKV nofile ulimit config'
require_pattern packaging/docker/compose.yaml '^  sbs-data-lb:' 'SBS raw data load balancer service'
require_pattern packaging/docker/compose.yaml '.*--grpc-listen=0\.0\.0\.0:9444' 'SBS raw data 9444 listener'
require_pattern packaging/docker/compose.yaml 'NAMROS_COMMUNITY_SBS_DATA_ENDPOINT:-sbs-data-1:9444' 'Community NAMROS SBS default data endpoint'
require_pattern packaging/docker/compose.yaml 'NAMROS_COMMUNITY_SBS_DATA_ENDPOINTS:-sbs-data-1:9444,sbs-data-2:9444' 'Community NAMROS SBS per-volume data endpoints'
require_pattern packaging/docker/compose.yaml 'NAMROS_SBS_VOLUME_POOL_ID' 'gateway metadata volume-pool registry config'
require_pattern packaging/docker/compose.yaml 'NAMROS_COMMUNITY_SBS_VOLUME_POOL_GENERATION:-0' 'Community volume-pool bootstrap auto generation'
require_pattern packaging/docker/compose.yaml 'NAMROS_COMMUNITY_SBS_ATTACHMENT_ID:-att-namros-community-object-pool' 'Community shared SBS attachment default'
require_pattern packaging/docker/compose.yaml 'NAMROS_SBS_WRITER_GROUP_ID' 'gateway writer group config'
require_pattern packaging/docker/compose.yaml 'NAMROS_SBS_VOLUME_EPOCH' 'gateway volume epoch config'
require_pattern packaging/docker/compose.yaml 'NAMROS_GC_CANDIDATE_QUEUE: metadata' 'production metadata GC queue config'
require_pattern packaging/docker/compose.yaml 'nofile:' 'TiKV nofile ulimit config'
require_pattern packaging/docker/compose.yaml 'soft: 200000' 'TiKV nofile soft limit'
require_pattern packaging/docker/compose.yaml 'hard: 200000' 'TiKV nofile hard limit'
require_pattern packaging/docker/compose.yaml 'condition: service_started' 'SBS admin load balancer does not block on active/standby readyz'
require_pattern packaging/docker/haproxy/sbs-admin.cfg 'option httpchk GET /readyz' 'SBS admin load balancer readiness check'
require_pattern packaging/docker/haproxy/sbs-admin.cfg 'check port 9081' 'SBS admin load balancer checks HTTP readiness port'
require_pattern packaging/docker/compose.yaml 'namros-pool-bootstrap' 'Compose volume-pool bootstrap service'
require_pattern packaging/docker/compose.yaml 'namros-container-volume-pool-bootstrap:/usr/local/bin/namros-container-volume-pool-bootstrap:ro' 'Compose volume-pool bootstrap helper bind mount'
require_pattern packaging/docker/compose.yaml 'service_completed_successfully' 'gateway waits for volume-pool bootstrap completion'
require_pattern packaging/docker/compose.yaml 'namros-gateway-a' 'first Community gateway'
require_pattern packaging/docker/compose.yaml 'namros-gateway-b' 'second Community gateway'
require_pattern packaging/docker/bin/namros-container-community-bootstrap 'NAMROS_COMMUNITY_SBS_VOLUME_IDS' 'multi-volume SBS bootstrap'
require_pattern packaging/docker/bin/namros-container-community-bootstrap 'wait_either_http "\$sbs_service_1_ready_url" sbs-service-1 "\$sbs_service_2_ready_url" sbs-service-2' 'SBS bootstrap accepts active replica readiness'
require_pattern packaging/docker/bin/namros-container-community-bootstrap 'NAMROS_COMMUNITY_SBS_ADMIN_LB_SETTLE_SECONDS' 'SBS admin load balancer convergence guard'
require_pattern packaging/docker/bin/namros-container-community-bootstrap '/debug/materialize-volume' 'SBS bootstrap materializes data-node volumes'
require_pattern packaging/docker/bin/namros-container-community-bootstrap 'binary_size_bytes' 'SBS bootstrap converts materialize volume sizes'
require_executable packaging/docker/bin/namros-container-community-bootstrap 'Community SBS bootstrap helper'
require_pattern packaging/docker/bin/namros-container-community-smoke 'run_cross_gateway_smoke' 'Community cross-gateway smoke'
require_pattern packaging/docker/bin/namros-container-community-smoke 'namros-container-local-smoke' 'Community load-balancer smoke'
require_pattern packaging/docker/bin/namros-container-volume-pool-bootstrap 'volume-pool-put' 'metadata volume-pool bootstrap'
require_pattern packaging/docker/bin/namros-container-volume-pool-bootstrap 'GENERATION:-0' 'volume-pool bootstrap uses auto generation by default'
require_pattern packaging/docker/bin/namros-container-volume-pool-bootstrap 'DATA_ENDPOINT:-sbs-data-1:9444' 'volume-pool bootstrap uses SBS raw data endpoint by default'
require_pattern packaging/docker/bin/namros-container-volume-pool-bootstrap 'NAMROS_COMMUNITY_SBS_DATA_ENDPOINTS' 'volume-pool bootstrap supports per-volume SBS data endpoints'
require_pattern packaging/docker/bin/namros-container-volume-pool-bootstrap 'endpoint_at' 'volume-pool bootstrap maps volumes to data endpoints'
require_pattern scripts/container/community-failover-smoke.sh 'summary\.json' 'Community failover summary artifact'
require_pattern scripts/container/community-failover-smoke.sh 'sbs-service-2' 'Community failover SBS service disruption'
require_pattern scripts/container/community-failover-smoke.sh 'run_smoke "during-\$service-stopped" "lb"' 'Community failover load-balancer smoke mode'
require_pattern scripts/container/community-failover-smoke.sh 'ensure-local-files\.sh' 'Community failover prepares local Compose files'
require_pattern scripts/container/community-failover-smoke.sh 'GIT_TERMINAL_PROMPT' 'Community failover disables interactive Git credential prompts'
require_pattern scripts/container/community-failover-smoke.sh 'NAMROS_COMMUNITY_FAILOVER_MIN_FREE_BYTES' 'Community failover disk preflight threshold'
require_pattern scripts/container/community-failover-smoke.sh 'exit 77' 'Community failover skip exit contract'
require_pattern scripts/release/check-production-scale-readiness.sh 'container smoke preflight skipped' 'Production readiness handles container preflight skip'
require_pattern scripts/container/ensure-local-files.sh 'NAMROS_LOCAL_NAMRBD_CONTEXT' 'Local NAMRBD context override'
require_pattern scripts/container/ensure-local-files.sh 'NAMROS_CONTAINER_USE_SLIM_NAMRBD_CONTEXT' 'Slim local NAMRBD context toggle'
require_pattern scripts/container/ensure-local-files.sh 'git -C "\$source_dir" ls-files -co --exclude-standard' 'Slim NAMRBD context excludes ignored cache files'
require_pattern scripts/container/ensure-local-files.sh '\.cache/namrbd-contexts' 'Slim NAMRBD context cache path'
require_pattern scripts/container/ensure-local-files.sh 'namrbd-context\.dockerignore' 'Slim NAMRBD context dockerignore template'
require_pattern scripts/container/ensure-local-files.sh '\.dockerignore' 'Slim NAMRBD context dockerignore generation'
require_pattern scripts/container/ensure-local-files.sh 'using local NAMRBD build context' 'Local NAMRBD context diagnostic'
require_pattern scripts/container/ensure-local-files.sh 'chmod 700 "\$secrets_dir"' 'Local secret directory access guard'
require_pattern scripts/container/ensure-local-files.sh 'chmod 444 "\$access_key_file" "\$secret_key_file"' 'Rootless non-root container secret read mode'
require_pattern packaging/docker/namrbd-context.dockerignore '^\.cache/' 'NAMRBD context excludes cache directories'
require_pattern packaging/docker/namrbd-context.dockerignore '^bin/' 'NAMRBD context excludes local binaries'
require_pattern packaging/docker/namrbd-context.dockerignore '^kernel/module/\*\.ko' 'NAMRBD context excludes kernel build artifacts'
require_pattern packaging/docker/.env.example '^NAMROS_NAMRBD_CONTEXT=https://github.com/nosway/namrbd\.git#v[0-9]' 'pinned NAMRBD context default'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_VOLUME_ID=[0-9a-f]{8}$' 'Community SBS primary volume id default'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_VOLUME_IDS=[0-9a-f]{8},[0-9a-f]{8}$' 'Community SBS volume list default'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_VOLUME_POOL_ID=' 'Community volume-pool id default'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_VOLUME_POOL_GENERATION=0$' 'Community volume-pool auto generation default'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_DATA_ENDPOINT=sbs-data-1:9444$' 'Community SBS default data endpoint'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_DATA_ENDPOINTS=sbs-data-1:9444,sbs-data-2:9444$' 'Community SBS per-volume data endpoint defaults'
require_pattern packaging/docker/.env.example '^NAMROS_COMMUNITY_SBS_ATTACHMENT_ID=att-namros-community-object-pool$' 'Community shared SBS attachment id default'
require_pattern packaging/helm/namros-community/values.yaml '    - 18a00001' 'Helm Community primary SBS volume id default'
require_pattern packaging/helm/namros-community/values.yaml 'attachmentId: att-namros-community-object-pool' 'Helm Community shared SBS attachment id default'
require_pattern packaging/helm/namros-community/values.production.yaml '    - 18b00001' 'Helm production SBS volume id example'
require_pattern packaging/helm/namros-community/values.production.yaml 'attachmentId: att-namros-production-object-pool' 'Helm production SBS attachment id example'
reject_pattern packaging/docker/.env.example 'namrbd\.git#main' 'floating NAMRBD context'
reject_pattern packaging/docker/compose.yaml 'namrbd\.git#main' 'floating NAMRBD context'
reject_pattern packaging/docker/compose.yaml '\$\{[^}]*\$\{' 'nested Compose variable default'

printf '[container-packaging-check] passed\n'

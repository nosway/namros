#!/usr/bin/env bash

set -u

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GO="${GO:-go}"
GOFLAGS_VALUE="${GOFLAGS:--buildvcs=false}"
REPORT_DIR="${NAMROS_PRODUCTION_SCALE_CHECK_DIR:-$ROOT_DIR/.cache/production-scale-check}"
SUMMARY_FILE="$REPORT_DIR/summary.txt"

fail=0
pass_count=0
fail_count=0
skip_count=0

mkdir -p "$REPORT_DIR" "$ROOT_DIR/.cache/go-build"
: >"$SUMMARY_FILE"

log() {
	printf '[production-scale-check] %s\n' "$*"
}

record() {
	local status="$1"
	local name="$2"
	local detail="${3:-}"
	printf '%s\t%s\t%s\n' "$status" "$name" "$detail" >>"$SUMMARY_FILE"
	case "$status" in
	PASS) pass_count=$((pass_count + 1)) ;;
	FAIL)
		fail_count=$((fail_count + 1))
		fail=1
		;;
	SKIP) skip_count=$((skip_count + 1)) ;;
	esac
	log "$status $name${detail:+: $detail}"
}

have_cmd() {
	command -v "$1" >/dev/null 2>&1
}

go_env_args() {
	printf 'GOCACHE=%s\n' "${GOCACHE:-$ROOT_DIR/.cache/go-build}"
	if [ -n "${GOMODCACHE:-}" ]; then
		printf 'GOMODCACHE=%s\n' "$GOMODCACHE"
	fi
}

run_required() {
	local name="$1"
	shift
	log "RUN $name"
	if "$@"; then
		record PASS "$name"
	else
		record FAIL "$name"
	fi
}

run_go_test() {
	local name="$1"
	shift
	local -a env_args=()
	while IFS= read -r env_arg; do
		env_args+=("$env_arg")
	done < <(go_env_args)
	run_required "$name" env "${env_args[@]}" "$GO" test $GOFLAGS_VALUE "$@"
}

run_metadata_list_gate() {
	local name="$1"
	local object_count="$2"
	local page_size="$3"
	local gate_dir="$REPORT_DIR/$name-$(date -u +%Y%m%dT%H%M%SZ)-$$"
	local -a env_args=()
	while IFS= read -r env_arg; do
		env_args+=("$env_arg")
	done < <(go_env_args)
	mkdir -p "$gate_dir"
	log "RUN $name"
	if env "${env_args[@]}" "$GO" run $GOFLAGS_VALUE ./cmd/namros-s3bench metadata-list-index \
		-metadata-backend pebble \
		-metadata-path "$gate_dir/meta" \
		-bucket "production-scale-list-$object_count" \
		-object-count "$object_count" \
		-page-size "$page_size" \
		-summary-json "$gate_dir/summary.json" \
		-page-jsonl "$gate_dir/pages.jsonl" >"$gate_dir/stdout.json"; then
		record PASS "$name" "$gate_dir/summary.json"
	else
		record FAIL "$name" "$gate_dir/summary.json"
	fi
}

run_metadata_scale_budget_gate() {
	local name="$1"
	local gate_dir="$REPORT_DIR/$name-$(date -u +%Y%m%dT%H%M%SZ)-$$"
	local -a env_args=()
	while IFS= read -r env_arg; do
		env_args+=("$env_arg")
	done < <(go_env_args)
	local -a budget_args=(
		metadata-scale-budget
		-release-gate
		-part-count "${NAMROS_PRODUCTION_SCALE_METADATA_PART_COUNT:-10000}"
		-segment-ref-count "${NAMROS_PRODUCTION_SCALE_METADATA_SEGMENT_REF_COUNT:-0}"
		-protected-ref-count "${NAMROS_PRODUCTION_SCALE_METADATA_PROTECTED_REF_COUNT:-0}"
		-gc-candidate-count "${NAMROS_PRODUCTION_SCALE_METADATA_GC_CANDIDATE_COUNT:-0}"
		-chunks-per-segment "${NAMROS_PRODUCTION_SCALE_METADATA_CHUNKS_PER_SEGMENT:-1}"
		-value-budget-bytes "${NAMROS_PRODUCTION_SCALE_METADATA_VALUE_BUDGET_BYTES:-16777216}"
		-complete-txn-budget-bytes "${NAMROS_PRODUCTION_SCALE_METADATA_COMPLETE_TXN_BUDGET_BYTES:-67108864}"
		-include-list-index-write-bytes="${NAMROS_PRODUCTION_SCALE_METADATA_INCLUDE_LIST_INDEX_BYTES:-true}"
		-include-protected-ref-write-bytes="${NAMROS_PRODUCTION_SCALE_METADATA_INCLUDE_PROTECTED_REF_BYTES:-true}"
		-include-gc-candidate-write-bytes="${NAMROS_PRODUCTION_SCALE_METADATA_INCLUDE_GC_CANDIDATE_BYTES:-true}"
	)
	if [ "${NAMROS_PRODUCTION_SCALE_METADATA_FAIL_ON_WATCH:-0}" = "1" ]; then
		budget_args+=(-fail-on-watch)
	fi
	mkdir -p "$gate_dir"
	log "RUN $name"
	if env "${env_args[@]}" "$GO" run $GOFLAGS_VALUE ./cmd/namros-admin "${budget_args[@]}" >"$gate_dir/metadata-scale-budget.json"; then
		record PASS "$name" "$gate_dir/metadata-scale-budget.json"
	else
		record FAIL "$name" "$gate_dir/metadata-scale-budget.json"
	fi
}

run_optional_script() {
	local name="$1"
	local env_name="$2"
	local script_path="$3"
	shift 3
	local -a required_cmds=("$@")
	if [ "${!env_name:-0}" != "1" ]; then
		record SKIP "$name" "set $env_name=1 to run this external smoke"
		return
	fi
	if [ ! -f "$script_path" ]; then
		record SKIP "$name" "script is not present in this checkout: $script_path"
		return
	fi
	local missing=()
	local cmd
	for cmd in "${required_cmds[@]}"; do
		if ! have_cmd "$cmd"; then
			missing+=("$cmd")
		fi
	done
	if [ "${#missing[@]}" -gt 0 ]; then
		record SKIP "$name" "missing external command(s): ${missing[*]}"
		return
	fi
	run_required "$name" bash "$script_path"
}

run_optional_container_compose_script() {
	local name="$1"
	local env_name="$2"
	local script_path="$3"
	local docker_bin="${DOCKER:-docker}"
	if [ "${!env_name:-0}" != "1" ]; then
		record SKIP "$name" "set $env_name=1 to run this external smoke"
		return
	fi
	if [ ! -f "$script_path" ]; then
		record SKIP "$name" "script is not present in this checkout: $script_path"
		return
	fi
	if ! have_cmd "$docker_bin"; then
		record SKIP "$name" "missing external command: $docker_bin"
		return
	fi
	if ! "$docker_bin" compose version >/dev/null 2>&1; then
		record SKIP "$name" "missing Docker Compose provider for $docker_bin; install docker compose, docker-compose, or podman-compose"
		return
	fi
	log "RUN $name"
	if sh "$script_path"; then
		record PASS "$name"
	else
		rc=$?
		if [ "$rc" -eq 77 ]; then
			record SKIP "$name" "container smoke preflight skipped; see script output"
		else
			record FAIL "$name"
		fi
	fi
}

cd "$ROOT_DIR" || exit 1

if ! have_cmd "$GO"; then
	record FAIL "go-toolchain" "missing command: $GO"
else
	run_go_test "config-production-profile" ./internal/config -run 'Test(ParseDeploymentProfile|ValidateDeploymentProfile|ParseProductionProfile)'
		run_go_test "metadata-production-readiness" ./internal/adminstatus ./cmd/namros-admin -run 'Test(BuildProductionReadiness|StatusCommandReportsProductionReadiness|VolumePoolPutCommand)'
		run_go_test "metadata-volume-pool-registry" ./internal/meta ./internal/meta/memory ./internal/meta/kvrepo -run 'Test'
		run_go_test "storage-volume-pool-routing" ./internal/storage/volumepool -run 'Test'
		if [ -f scripts/lab/run-volume-pool-runtime-add-member-smoke.sh ]; then
			run_required "volume-pool-runtime-add-member-smoke" bash scripts/lab/run-volume-pool-runtime-add-member-smoke.sh
		else
			record SKIP "volume-pool-runtime-add-member-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-volume-pool-drain-read-old-ref-smoke.sh ]; then
			run_required "volume-pool-drain-read-old-ref-smoke" bash scripts/lab/run-volume-pool-drain-read-old-ref-smoke.sh
		else
			record SKIP "volume-pool-drain-read-old-ref-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-sbs-session-refcount-open-smoke.sh ]; then
			run_required "sbs-session-refcount-open-smoke" bash scripts/lab/run-sbs-session-refcount-open-smoke.sh
		else
			record SKIP "sbs-session-refcount-open-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-sbs-session-close-guard-smoke.sh ]; then
			run_required "sbs-session-close-guard-smoke" bash scripts/lab/run-sbs-session-close-guard-smoke.sh
		else
			record SKIP "sbs-session-close-guard-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-sbs-session-fence-smoke.sh ]; then
			run_required "sbs-session-fence-smoke" bash scripts/lab/run-sbs-session-fence-smoke.sh
		else
			record SKIP "sbs-session-fence-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-gateway-drain-state-smoke.sh ]; then
			run_required "gateway-drain-state-smoke" bash scripts/lab/run-gateway-drain-state-smoke.sh
		else
			record SKIP "gateway-drain-state-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-lab-bridge-production-guard-smoke.sh ]; then
			run_required "lab-bridge-production-guard-smoke" bash scripts/lab/run-lab-bridge-production-guard-smoke.sh
		else
			record SKIP "lab-bridge-production-guard-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-production-gc-queue-guard-smoke.sh ]; then
			run_required "production-gc-queue-guard-smoke" bash scripts/lab/run-production-gc-queue-guard-smoke.sh
		else
			record SKIP "production-gc-queue-guard-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-lifecycle-worker-runner-smoke.sh ]; then
			run_required "lifecycle-worker-runner-smoke" bash scripts/lab/run-lifecycle-worker-runner-smoke.sh
		else
			record SKIP "lifecycle-worker-runner-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-worker-control-smoke.sh ]; then
			run_required "worker-control-smoke" bash scripts/lab/run-worker-control-smoke.sh
		else
			record SKIP "worker-control-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-worker-budget-smoke.sh ]; then
			run_required "worker-budget-smoke" bash scripts/lab/run-worker-budget-smoke.sh
		else
			record SKIP "worker-budget-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-volume-drain-operation-model-smoke.sh ]; then
			run_required "volume-drain-operation-model-smoke" bash scripts/lab/run-volume-drain-operation-model-smoke.sh
		else
			record SKIP "volume-drain-operation-model-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-volume-drain-copy-publish-smoke.sh ]; then
			run_required "volume-drain-copy-publish-smoke" bash scripts/lab/run-volume-drain-copy-publish-smoke.sh
		else
			record SKIP "volume-drain-copy-publish-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-volume-drain-gc-verify-smoke.sh ]; then
			run_required "volume-drain-gc-verify-smoke" bash scripts/lab/run-volume-drain-gc-verify-smoke.sh
		else
			record SKIP "volume-drain-gc-verify-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-volume-drain-worker-failover-smoke.sh ]; then
			run_required "volume-drain-worker-failover-smoke" bash scripts/lab/run-volume-drain-worker-failover-smoke.sh
		else
			record SKIP "volume-drain-worker-failover-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-tenant-quota-records-smoke.sh ]; then
			run_required "tenant-quota-records-smoke" bash scripts/lab/run-tenant-quota-records-smoke.sh
		else
			record SKIP "tenant-quota-records-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-tenant-usage-reconcile-smoke.sh ]; then
			run_required "tenant-usage-reconcile-smoke" bash scripts/lab/run-tenant-usage-reconcile-smoke.sh
		else
			record SKIP "tenant-usage-reconcile-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-tenant-active-upload-quota-smoke.sh ]; then
			run_required "tenant-active-upload-quota-smoke" bash scripts/lab/run-tenant-active-upload-quota-smoke.sh
		else
			record SKIP "tenant-active-upload-quota-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-gateway-request-limiter-smoke.sh ]; then
			run_required "gateway-request-limiter-smoke" bash scripts/lab/run-gateway-request-limiter-smoke.sh
		else
			record SKIP "gateway-request-limiter-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-gateway-bandwidth-hooks-smoke.sh ]; then
			run_required "gateway-bandwidth-hooks-smoke" bash scripts/lab/run-gateway-bandwidth-hooks-smoke.sh
		else
			record SKIP "gateway-bandwidth-hooks-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-noisy-tenant-profile-smoke.sh ]; then
			run_required "noisy-tenant-profile-smoke" bash scripts/lab/run-noisy-tenant-profile-smoke.sh
		else
			record SKIP "noisy-tenant-profile-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-admission-observability-smoke.sh ]; then
			run_required "admission-observability-smoke" bash scripts/lab/run-admission-observability-smoke.sh
		else
			record SKIP "admission-observability-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-gateway-latency-error-metrics-smoke.sh ]; then
			run_required "gateway-latency-error-metrics-smoke" bash scripts/lab/run-gateway-latency-error-metrics-smoke.sh
		else
			record SKIP "gateway-latency-error-metrics-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-tikv-retry-metrics-smoke.sh ]; then
			run_required "tikv-retry-metrics-smoke" bash scripts/lab/run-tikv-retry-metrics-smoke.sh
		else
			record SKIP "tikv-retry-metrics-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-worker-backlog-metrics-smoke.sh ]; then
			run_required "worker-backlog-metrics-smoke" bash scripts/lab/run-worker-backlog-metrics-smoke.sh
		else
			record SKIP "worker-backlog-metrics-smoke" "script is not present in this checkout"
		fi
		if [ -f scripts/ops/check-observability-assets.sh ]; then
			run_required "observability-assets-check" bash scripts/ops/check-observability-assets.sh
		else
			record SKIP "observability-assets-check" "script is not present in this checkout"
		fi
		if [ -f scripts/lab/run-incident-drill-smoke.sh ]; then
			run_required "incident-drill-smoke" bash scripts/lab/run-incident-drill-smoke.sh
		else
			record SKIP "incident-drill-smoke" "script is not present in this checkout"
		fi
		run_metadata_scale_budget_gate "metadata-scale-budget"
		run_metadata_list_gate "metadata-list-index-benchmark" "${NAMROS_PRODUCTION_SCALE_LIST_OBJECT_COUNT:-1000}" "${NAMROS_PRODUCTION_SCALE_LIST_PAGE_SIZE:-100}"
	if [ "${NAMROS_PRODUCTION_SCALE_RUN_MILLION_KEY_LIST:-0}" = "1" ]; then
		run_metadata_list_gate "metadata-million-key-list-benchmark" "${NAMROS_PRODUCTION_SCALE_MILLION_LIST_OBJECT_COUNT:-1000000}" "${NAMROS_PRODUCTION_SCALE_MILLION_LIST_PAGE_SIZE:-1000}"
	else
		record SKIP "metadata-million-key-list-benchmark" "set NAMROS_PRODUCTION_SCALE_RUN_MILLION_KEY_LIST=1 to run the full million-key local gate"
	fi
fi

if [ -f scripts/release/check-community-source.sh ]; then
	run_required "source-boundary" env NAMROS_COMMUNITY_CHECK_SKIP_GO_TEST=true scripts/release/check-community-source.sh
else
	record SKIP "source-boundary" "script is not present in this checkout"
fi

if [ -f scripts/container/check-packaging.sh ]; then
	run_required "container-packaging-check" sh scripts/container/check-packaging.sh
else
	record SKIP "container-packaging-check" "script is not present in this checkout"
fi
if [ -f scripts/release/check-helm-chart.sh ]; then
	run_required "helm-chart-check" sh scripts/release/check-helm-chart.sh
else
	record SKIP "helm-chart-check" "script is not present in this checkout"
fi

for script in \
	scripts/container/check-packaging.sh \
	scripts/lab/run-18node-sbs-smoke.sh \
	scripts/lab/run-18node-sbs-multigateway-smoke.sh \
	scripts/lab/run-18node-sbs-4gateway-ec-volume-pool-smoke.sh \
	scripts/lab/run-18node-sbs-4gateway-rolling-overlap-smoke.sh \
	scripts/lab/run-18node-sbs-4gateway-partial-volume-loss-drill.sh \
	scripts/lab/run-volume-pool-runtime-add-member-smoke.sh \
	scripts/lab/run-volume-pool-drain-read-old-ref-smoke.sh \
	scripts/lab/run-sbs-session-refcount-open-smoke.sh \
	scripts/lab/run-sbs-session-close-guard-smoke.sh \
	scripts/lab/run-sbs-session-fence-smoke.sh \
	scripts/lab/run-gateway-drain-state-smoke.sh \
	scripts/lab/run-lab-bridge-production-guard-smoke.sh \
	scripts/lab/run-production-gc-queue-guard-smoke.sh \
	scripts/lab/run-lifecycle-worker-runner-smoke.sh \
	scripts/lab/run-worker-control-smoke.sh \
	scripts/lab/run-worker-budget-smoke.sh \
	scripts/lab/run-volume-drain-operation-model-smoke.sh \
	scripts/lab/run-volume-drain-copy-publish-smoke.sh \
	scripts/lab/run-volume-drain-gc-verify-smoke.sh \
	scripts/lab/run-volume-drain-worker-failover-smoke.sh \
	scripts/lab/run-tenant-quota-records-smoke.sh \
	scripts/lab/run-tenant-usage-reconcile-smoke.sh \
	scripts/lab/run-tenant-active-upload-quota-smoke.sh \
	scripts/lab/run-gateway-request-limiter-smoke.sh \
	scripts/lab/run-gateway-bandwidth-hooks-smoke.sh \
	scripts/lab/run-noisy-tenant-profile-smoke.sh \
	scripts/lab/run-admission-observability-smoke.sh \
	scripts/lab/run-gateway-latency-error-metrics-smoke.sh \
	scripts/lab/run-tikv-retry-metrics-smoke.sh \
	scripts/lab/run-worker-backlog-metrics-smoke.sh \
	scripts/ops/check-observability-assets.sh \
	scripts/ops/check-incident-bundle.sh \
	scripts/chaos/adopt-multi-node-soak-baseline.sh \
	scripts/lab/run-incident-drill-smoke.sh \
	scripts/lab/run-18node-s3-perf-baseline.sh; do
	if [ -f "$script" ]; then
		run_required "syntax-${script}" bash -n "$script"
	else
		record SKIP "syntax-${script}" "script is not present in this checkout"
	fi
done

run_optional_script "active-active-smoke" NAMROS_PRODUCTION_SCALE_RUN_ACTIVE_ACTIVE scripts/chaos/run-active-active-smoke.sh aws curl etcdctl jq
run_optional_script "metadata-backup-restore-smoke" NAMROS_PRODUCTION_SCALE_RUN_BACKUP_RESTORE scripts/metadata/run-backup-restore-smoke.sh jq
run_optional_script "ec-volume-pool-failover-smoke" NAMROS_PRODUCTION_SCALE_RUN_EC_VOLUME_POOL scripts/lab/run-18node-sbs-4gateway-ec-volume-pool-smoke.sh jq curl aws etcdctl ssh scp go dd cmp git rg
run_optional_container_compose_script "container-community-failover-smoke" NAMROS_PRODUCTION_SCALE_RUN_CONTAINER scripts/container/community-failover-smoke.sh

log "summary: pass=$pass_count fail=$fail_count skip=$skip_count file=$SUMMARY_FILE"
if [ "$fail" -ne 0 ]; then
	exit 1
fi

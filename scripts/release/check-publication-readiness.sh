#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
ALLOWLIST_FILE="$SCRIPT_DIR/community-source-allowlist.txt"
EXCLUDES_FILE="$SCRIPT_DIR/community-source-excludes.txt"
OVERLAYS_DIR="$SCRIPT_DIR/community-source-overlays"
OVERLAYS_REL="scripts/release/community-source-overlays"
RG="${RG:-rg}"
RG_CHECK_EXCLUDES=(
	--glob '!scripts/release/check-publication-readiness.sh'
	--glob '!scripts/release/check-community-source.sh'
	--glob '!scripts/release/export-community-source.sh'
)

cd "$ROOT_DIR"
require_readme="${NAMROS_PUBLICATION_READINESS_REQUIRE_README:-true}"

fail=0

log() {
	printf '[publication-check] %s\n' "$*"
}

error() {
	printf '[publication-check] ERROR: %s\n' "$*" >&2
	fail=1
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		error "missing required command: $cmd"
	fi
}

require_file() {
	local path="$1"
	if [ ! -f "$path" ]; then
		error "missing required file: $path"
	fi
}

is_false() {
	case "${1:-}" in
	0|false|FALSE|False|no|NO|No|n|N)
		return 0
		;;
	*)
		return 1
		;;
	esac
}

check_absent() {
	local description="$1"
	local pattern="$2"
	shift 2
	local output
	output="$("$RG" -n "${RG_CHECK_EXCLUDES[@]}" -- "$pattern" "$@" 2>/dev/null || true)"
	if [ -n "$output" ]; then
		error "$description"
		printf '%s\n' "$output" >&2
	fi
}

check_absent_in_files() {
	local description="$1"
	local pattern="$2"
	shift 2
	local output
	if [ "$#" -eq 0 ]; then
		return 0
	fi
	output="$("$RG" -n "${RG_CHECK_EXCLUDES[@]}" -- "$pattern" "$@" 2>/dev/null || true)"
	if [ -n "$output" ]; then
		error "$description"
		printf '%s\n' "$output" >&2
	fi
}

trim_manifest_line() {
	local value="$1"
	value="${value%%#*}"
	value="${value#"${value%%[![:space:]]*}"}"
	value="${value%"${value##*[![:space:]]}"}"
	printf '%s' "$value"
}

is_allowlisted() {
	local path="$1"
	local include
	if [ "${#allowlist[@]}" -eq 0 ]; then
		return 1
	fi
	for include in "${allowlist[@]}"; do
		case "$include" in
		*/)
			if [[ "$path" == "$include"* ]]; then
				return 0
			fi
			;;
		*)
			if [ "$path" = "$include" ]; then
				return 0
			fi
			;;
		esac
	done
	return 1
}

is_excluded() {
	local path="$1"
	local exclude
	if [ "${#excludes[@]}" -eq 0 ]; then
		return 1
	fi
	for exclude in "${excludes[@]}"; do
		if [ "$path" = "$exclude" ]; then
			return 0
		fi
	done
	return 1
}

is_overlaid() {
	local path="$1"
	local overlay
	if [ "${#overlaid_paths[@]}" -eq 0 ]; then
		return 1
	fi
	for overlay in "${overlaid_paths[@]}"; do
		if [ "$path" = "$overlay" ]; then
			return 0
		fi
	done
	return 1
}

load_manifest() {
	local file="$1"
	local dest_name="$2"
	local line path
	while IFS= read -r line || [ -n "$line" ]; do
		path="$(trim_manifest_line "$line")"
		if [ -n "$path" ]; then
			eval "$dest_name+=(\"\$path\")"
		fi
	done <"$file"
}

collect_public_files() {
	local path
	while IFS= read -r path; do
		if is_allowlisted "$path" && ! is_excluded "$path" && ! is_overlaid "$path"; then
			printf '%s\n' "$path"
		fi
	done < <(git ls-files)
}

check_local_namrbd_replace() {
	local replace_hits unexpected_hits
	replace_hits="$("$RG" -n '^replace[[:space:]]+github\.com/nosway/namrbd[[:space:]]+=>[[:space:]]+\.\./NAMRBD[[:space:]]*$' go.mod 2>/dev/null || true)"
	unexpected_hits="$("$RG" -n 'replace[[:space:]]+.*namrbd[[:space:]]*=>|\.\./NAMRBD' go.mod 2>/dev/null || true)"
	if [ -n "$unexpected_hits" ] && [ "$unexpected_hits" != "$replace_hits" ]; then
		error "go.mod may only use the temporary local NAMRBD replace stripped by export-community"
		printf '%s\n' "$unexpected_hits" >&2
	fi
	if [ -n "$replace_hits" ]; then
		log "allow temporary local NAMRBD replace in development go.mod; export strips it"
	fi
}

require_cmd git
require_cmd bash
require_cmd "$RG"
require_file "$ALLOWLIST_FILE"
require_file "$EXCLUDES_FILE"

allowlist=()
excludes=()
overlaid_paths=()
public_files=()
public_files_without_go_mod=()
public_doc_sources=()

load_manifest "$ALLOWLIST_FILE" allowlist
load_manifest "$EXCLUDES_FILE" excludes

if [ -d "$OVERLAYS_DIR" ]; then
	while IFS= read -r path; do
		overlaid_paths+=("${path#"$OVERLAYS_REL"/}")
	done < <(git ls-files "$OVERLAYS_REL")
fi

while IFS= read -r path; do
	public_files+=("$path")
	case "$path" in
	go.mod|scripts/release/*)
		;;
	*)
		public_files_without_go_mod+=("$path")
		;;
	esac
	case "$path" in
	docs-src/*.md|docs-src/*/*.md|docs-src/*/*/*.md|docs-src/*/*/*/*.md|docs-src/*/*/*/*/*.md)
		public_doc_sources+=("$path")
		;;
	esac
done < <(collect_public_files)

if [ "${#public_files[@]}" -eq 0 ]; then
	error "no public files found from git ls-files; run publication readiness from a Git checkout"
fi

if [ -d "$OVERLAYS_DIR" ]; then
	while IFS= read -r path; do
		public_files+=("$path")
		public_files_without_go_mod+=("$path")
	done < <(git ls-files "$OVERLAYS_REL")
fi

log "verify required public repository files"
for path in \
	LICENSE \
	NOTICE \
	CHANGELOG.md \
	CONTRIBUTING.md \
	CODE_OF_CONDUCT.md \
	SECURITY.md \
	.github/workflows/community.yml \
	.github/workflows/docs-pages.yml \
	.github/workflows/release.yml \
	.github/dependabot.yml \
	.github/pull_request_template.md \
	.github/ISSUE_TEMPLATE/bug_report.md \
	.github/ISSUE_TEMPLATE/feature_request.md \
	packaging/helm/namros-community/Chart.yaml \
	packaging/helm/namros-community/values.yaml \
	packaging/helm/namros-community/values.local.yaml \
	packaging/helm/namros-community/values.production.yaml \
	packaging/helm/namros-community/templates/servicemonitor.yaml \
	packaging/helm/namros-community/templates/podmonitor.yaml \
	packaging/helm/namros-community/templates/tests/gateway-smoke.yaml \
	docs-src/assets/namros-icon.svg \
	packaging/docker/compose.sbs-quickstart.yml \
	packaging/docker/bin/namros-container-sbs-quickstart-bootstrap \
	packaging/k8s/production-kind.env \
	packaging/k8s/kind-production.yaml \
	scripts/compat/run-public-s3-compat-smoke.sh \
	scripts/docs/check-html-docs.sh \
	scripts/k8s/deploy-production.sh \
	scripts/release/check-helm-chart.sh \
	scripts/release/write-release-artifact-metadata.sh; do
	require_file "$path"
done

if is_false "$require_readme"; then
	log "skip README.md requirement by configuration"
else
	require_file README.md
fi

log "verify Docs workflow keeps push CI independent from GitHub Pages setup"
if ! "$RG" -n -F 'make docs-render-check' .github/workflows/docs-pages.yml >/dev/null; then
	error "Docs workflow must render-check docs-src"
fi
if ! "$RG" -n -F 'deploy_pages:' .github/workflows/docs-pages.yml >/dev/null; then
	error "Docs workflow must require explicit Pages deployment input"
fi
if ! "$RG" -n -F "github.event_name == 'workflow_dispatch' && inputs.deploy_pages" .github/workflows/docs-pages.yml >/dev/null; then
	error "Docs workflow must deploy Pages only from explicit workflow_dispatch"
fi
if ! "$RG" -n -F 'actions/configure-pages' .github/workflows/docs-pages.yml >/dev/null; then
	error "Docs workflow must keep a manual configure-pages path"
fi
if ! "$RG" -n -F 'actions/deploy-pages' .github/workflows/docs-pages.yml >/dev/null; then
	error "Docs workflow must keep a manual Pages deploy path"
fi

log "verify Community CI runs public compatibility and real Helm checks"
if ! "$RG" -n -F 'make compat-public-s3' .github/workflows/community.yml >/dev/null; then
	error "Community CI must run strict public S3 compatibility smoke"
fi
for pattern in \
	'awscli' \
	'dl.min.io/client/mc' \
	'rclone' \
	'NAMROS_COMPAT_AUTOSTART_GATEWAY' \
	'NAMROS_COMPAT_MC_LARGE_OBJECT_MIB' \
	'NAMROS_COMPAT_RCLONE_UPLOAD_CUTOFF'; do
	if ! "$RG" -n "$pattern" .github/workflows/community.yml >/dev/null; then
		error "Community CI S3 compatibility job is missing required pattern: $pattern"
	fi
done
if ! "$RG" -n -F 'NAMROS_HELM_REQUIRE_HELM=true make helm-chart-check' .github/workflows/community.yml >/dev/null; then
	error "Community CI must run helm-chart-check with Helm CLI required"
fi

log "verify release workflow publishes tagged artifacts"
for pattern in \
	'push:' \
	'v\*\.\*\.\*' \
	'make build-community' \
	'NAMROS_VERSION' \
	'NAMROS_COMMIT' \
	'NAMROS_BUILD_DATE' \
	'helm package' \
	'docker/build-push-action' \
	'write-release-artifact-metadata\.sh' \
	'NAMROS_RELEASE_GENERATE_SBOM=1' \
	'checksums\.sha256' \
	'softprops/action-gh-release'; do
	if ! "$RG" -n "$pattern" .github/workflows/release.yml >/dev/null; then
		error "release workflow is missing required contract pattern: $pattern"
	fi
done

log "verify binaries are stamped with release version metadata"
for file in Makefile scripts/release/community-source-overlays/Makefile; do
	if ! "$RG" -n '^VERSION_PACKAGE \?= github\.com/nosway/namros/internal/version$' "$file" >/dev/null; then
		error "$file must define the internal/version linker package"
	fi
	if ! "$RG" -n 'GO_LDFLAGS_VERSION .*\.Version=' "$file" >/dev/null; then
		error "$file must stamp internal/version.Version with linker flags"
	fi
	if ! "$RG" -n -- '-ldflags' "$file" >/dev/null; then
		error "$file build rules must pass Go linker flags"
	fi
done
for file in packaging/docker/Dockerfile.gateway packaging/docker/Dockerfile.tools; do
	if ! "$RG" -n 'internal/version\.Version' "$file" >/dev/null; then
		error "$file must stamp internal/version.Version"
	fi
	if ! "$RG" -n -- '-ldflags' "$file" >/dev/null; then
		error "$file build rules must pass Go linker flags"
	fi
done

log "verify public Go module paths"
if ! "$RG" -n '^module github\.com/nosway/namros$' go.mod >/dev/null; then
	error "go.mod module path must be github.com/nosway/namros"
fi
if ! "$RG" -n 'github\.com/nosway/namrbd[[:space:]]+v' go.mod >/dev/null; then
	error "go.mod must depend on github.com/nosway/namrbd"
fi
check_local_namrbd_replace
if [ "${#public_files_without_go_mod[@]}" -gt 0 ]; then
	check_absent_in_files "local NAMRBD replace leaked outside go.mod/release tooling" '\.\./NAMRBD|replace[[:space:]]+.*namrbd[[:space:]]*=>' "${public_files_without_go_mod[@]}"
	check_absent_in_files "runtime edition switch leaked into public files" 'NAMROS_EDITION|StringVar\(&cfg\.Edition|Var\(&cfg\.Edition|"-edition"' "${public_files_without_go_mod[@]}"
	check_absent_in_files "private lab host, QA suite, or planning reference leaked into public files" '\b(dev001|u[0-9][0-9])\b|qa-suite|namros-community-publication-plan|namros-implementation-plan|namros-production-scale-design-and-implementation|namros-container-deployment-implementation-plan|namros-enterprise-source-overlay' "${public_files_without_go_mod[@]}"
fi
if [ "${#public_files[@]}" -gt 0 ]; then
	check_absent_in_files "old GitHub account placeholder leaked" 'github\.com/twkim' "${public_files[@]}"
fi

log "verify public docs do not contain personal local paths"
if [ "${#public_files[@]}" -gt 0 ]; then
	check_absent_in_files "personal macOS path leaked" '/Users/[A-Za-z0-9_.-]+' "${public_files[@]}"
	check_absent_in_files "personal Linux path leaked" '/home/[A-Za-z0-9_.-]+' "${public_files[@]}"
fi

log "verify public HTML docs do not depend on private Markdown notes"
if [ "${#public_doc_sources[@]}" -gt 0 ]; then
	check_absent_in_files "public documentation links into the private docs/ tree" '\]\((\.\./)*docs/' "${public_doc_sources[@]}"
fi
for path in "${public_files[@]}"; do
	case "$path" in
	docs/*.md|docs/*/*.md|docs/*/*/*.md)
		error "private Markdown docs must not be part of the public source export: $path"
		;;
	esac
done

log "verify public docs reference public Makefile targets"
public_makefile="Makefile"
if [ -f "$OVERLAYS_DIR/Makefile" ]; then
	public_makefile="$OVERLAYS_DIR/Makefile"
fi
if ! NAMROS_DOCS_PUBLIC_MAKEFILE="$public_makefile" bash scripts/docs/check-html-docs.sh; then
	fail=1
fi

log "verify release artifact metadata tooling"
if ! bash -n scripts/release/check-helm-chart.sh; then
	error "Helm chart check script has a syntax error"
fi
if ! bash -n scripts/release/write-release-artifact-metadata.sh; then
	error "release artifact metadata script has a syntax error"
fi
if ! "$RG" -n 'packaging/helm/namros-community' scripts/release/check-helm-chart.sh >/dev/null; then
	error "Helm chart check must validate packaging/helm/namros-community"
fi
for pattern in \
	'^home:' \
	'^icon:' \
	'^sources:' \
	'^maintainers:' \
	'^keywords:' \
	'artifacthub\.io/category'; do
	if ! "$RG" -n "$pattern" packaging/helm/namros-community/Chart.yaml >/dev/null; then
		error "Helm chart metadata is missing required pattern: $pattern"
	fi
done
for pattern in \
	'kind: ServiceMonitor' \
	'kind: PodMonitor' \
	'helm\.sh/hook": test' \
	'NAMROS_HELM_REQUIRE_HELM'; do
	if ! "$RG" -n "$pattern" scripts/release/check-helm-chart.sh packaging/helm/namros-community/templates >/dev/null; then
		error "Helm chart checks/templates are missing required pattern: $pattern"
	fi
done
for pattern in \
	'namros\.release_artifact_metadata\.v1' \
	'checksums\.sha256' \
	'go-modules\.txt' \
	'provenance\.json' \
	'sbom-status\.json' \
	'NAMROS_RELEASE_GENERATE_SBOM'; do
	if ! "$RG" -n "$pattern" scripts/release/write-release-artifact-metadata.sh >/dev/null; then
		error "release artifact metadata script is missing required contract pattern: $pattern"
	fi
done
if ! "$RG" -n 'write-release-artifact-metadata\.sh' scripts/release/export-community-source.sh >/dev/null; then
	error "community source export must emit release artifact metadata"
fi

log "verify generated/local artifacts are not tracked"
tracked_junk="$(git ls-files | "$RG" '(^|/)\.DS_Store$|^\.cache/|^dist/|^\.namros/|^tmp/|\.log$' || true)"
if [ -n "$tracked_junk" ]; then
	error "generated or local artifacts are tracked"
	printf '%s\n' "$tracked_junk" >&2
fi

log "verify release hygiene files"
if ! "$RG" -n '^/dist/$' .gitignore >/dev/null; then
	error ".gitignore must ignore /dist/"
fi
if ! "$RG" -n '^/\.namros/$' .gitignore >/dev/null; then
	error ".gitignore must ignore /.namros/"
fi
if ! "$RG" -n '^\.DS_Store$' .gitignore >/dev/null; then
	error ".gitignore must ignore .DS_Store"
fi

if [ "$fail" -ne 0 ]; then
	exit 1
fi

log "passed"

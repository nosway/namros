#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
ALLOWLIST_FILE="$SCRIPT_DIR/community-source-allowlist.txt"
EXCLUDES_FILE="$SCRIPT_DIR/community-source-excludes.txt"
OVERLAY_DIR="$SCRIPT_DIR/community-source-overlays"
GO="${GO:-go}"
RG="${RG:-rg}"
RG_CHECK_EXCLUDES=(
	--glob '!scripts/release/check-community-source.sh'
	--glob '!scripts/release/check-enterprise-build-source.sh'
	--glob '!scripts/release/check-publication-readiness.sh'
)

cd "$ROOT_DIR"

fail=0

log() {
	printf '[community-check] %s\n' "$*"
}

error() {
	printf '[community-check] ERROR: %s\n' "$*" >&2
	fail=1
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		error "missing required command: $cmd"
	fi
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

trim_manifest_line() {
	local value="$1"
	value="${value%%#*}"
	value="${value#"${value%%[![:space:]]*}"}"
	value="${value%"${value##*[![:space:]]}"}"
	printf '%s' "$value"
}

check_allowlist_manifest() {
	local line path tracked
	while IFS= read -r line || [ -n "$line" ]; do
		path="$(trim_manifest_line "$line")"
		if [ -z "$path" ]; then
			continue
		fi
		case "$path" in
		docs/|scripts/|qa-suite/|dist/|bin/|.cache/|.namros/)
			error "community source allowlist is too broad for private/generated tree: $path"
			continue
			;;
		esac
		case "$path" in
		*/)
			tracked="$(git ls-files -- "$path" 2>/dev/null || true)"
			if [ -z "$tracked" ]; then
				error "community source allowlist directory has no tracked files: $path"
			fi
			;;
		*)
			if ! git ls-files --error-unmatch "$path" >/dev/null 2>&1; then
				error "community source allowlist file is not tracked: $path"
			fi
			;;
		esac
	done <"$ALLOWLIST_FILE"
}

check_exclude_manifest() {
	local line path
	while IFS= read -r line || [ -n "$line" ]; do
		path="$(trim_manifest_line "$line")"
		if [ -z "$path" ]; then
			continue
		fi
		if ! git ls-files --error-unmatch "$path" >/dev/null 2>&1; then
			error "community source exclude path is not tracked: $path"
		fi
	done <"$EXCLUDES_FILE"
}

prepare_community_test_tree() {
	local work_dir out_dir path overlay_path rel namrbd_dir
	work_dir="$ROOT_DIR/.cache/community-source-check"
	out_dir="$work_dir/namros-community"
	rm -rf "$work_dir"
	mkdir -p "$out_dir"
	while IFS= read -r path; do
		if ! is_allowlisted "$path" || is_excluded "$path"; then
			continue
		fi
		if [ ! -f "$path" ]; then
			continue
		fi
		mkdir -p "$out_dir/$(dirname -- "$path")"
		cp -p "$path" "$out_dir/$path"
	done < <(git ls-files)
	if [ -d "$OVERLAY_DIR" ]; then
		while IFS= read -r -d '' overlay_path; do
			rel="${overlay_path#"$OVERLAY_DIR/"}"
			mkdir -p "$out_dir/$(dirname -- "$rel")"
			cp -p "$overlay_path" "$out_dir/$rel"
		done < <(find "$OVERLAY_DIR" -type f -print0 | sort -z)
	fi
	namrbd_dir="$(cd -- "$ROOT_DIR/../NAMRBD" 2>/dev/null && pwd || true)"
	if [ -n "$namrbd_dir" ]; then
		ln -s "$namrbd_dir" "$work_dir/NAMRBD"
	fi
	printf '%s\n' "$out_dir"
}

check_local_namrbd_replace() {
	local replace_hits unexpected_hits
	replace_hits="$("$RG" -n '^replace[[:space:]]+github\.com/nosway/namrbd[[:space:]]+=>[[:space:]]+\.\./NAMRBD[[:space:]]*$' go.mod 2>/dev/null || true)"
	unexpected_hits="$("$RG" -n 'replace[[:space:]]+.*namrbd[[:space:]]*=>|\.\./NAMRBD' go.mod 2>/dev/null || true)"
	if [ -n "$unexpected_hits" ] && [ "$unexpected_hits" != "$replace_hits" ]; then
		error "go.mod may only use the temporary local NAMRBD replace: replace github.com/nosway/namrbd => ../NAMRBD"
		printf '%s\n' "$unexpected_hits" >&2
	fi
	if [ -n "$replace_hits" ]; then
		log "allow temporary local NAMRBD replace until github.com/nosway/namrbd is published"
	fi
}

check_public_doc_make_targets() {
	local public_makefile="Makefile"
	if [ -f "$OVERLAY_DIR/Makefile" ]; then
		public_makefile="$OVERLAY_DIR/Makefile"
	fi
	NAMROS_DOCS_PUBLIC_MAKEFILE="$public_makefile" bash scripts/docs/check-html-docs.sh
}

require_cmd git
require_cmd "$RG"
require_cmd "$GO"
require_cmd bash

allowlist=()
excludes=()
load_manifest "$ALLOWLIST_FILE" allowlist
load_manifest "$EXCLUDES_FILE" excludes

if [ "$fail" -ne 0 ]; then
	exit 1
fi

log "verify public source has no runtime Enterprise switch"
check_absent "runtime edition environment switch leaked" 'NAMROS_EDITION' cmd internal scripts Makefile
check_absent "public -edition flag leaked" 'StringVar\(&cfg\.Edition|Var\(&cfg\.Edition|"-edition"' cmd internal scripts Makefile
check_absent "public admin edition injection hook leaked" 'edition string|edition:[[:space:]]*edition\.Enterprise|cfg\.Edition[[:space:]]=' cmd/namros-admin
check_absent "public admin dispatch still calls enterprise command bodies" 'return c\.run(Dedupe|KMS|Compliance)' cmd/namros-admin/main.go
check_absent "public admin enterprise command bodies leaked" 'func \(c adminCommand\) run(Dedupe|KMS|Compliance)' cmd/namros-admin/main.go
check_absent "public admin enterprise helper implementations leaked" 'func (buildCompliance|buildAccess|filterAuditEventsByTimeRange|attachAccessHistory|buildEncryptionKeyEvidence|parseTimeSourceEvidenceConfig|buildTimeSourceEvidence|complianceEvidence|buildComplianceDiscoveryManifest|verifyAuditChain|auditEventHash|runDedupeScrubOnce|enrichDedupeScrubOutput|parseKMSKeyState|parsePolicyLegalHold|kmsKeyAuditDetails|dedupeAckAuditDetails|dedupeRepairAuditDetails|dedupeScrubAuditDetails|complianceProfileAttachAuditDetails|complianceEvidenceAuditDetails)' cmd/namros-admin/main.go
check_absent "public source imports private compliance implementation" '(github\.com/nosway/namros/)?internal/compliance' cmd internal
check_absent "public admin imports private dedupe implementation" '(github\.com/nosway/namros/)?internal/dedupe' cmd/namros-admin
check_absent "public enterprise build tag leaked" '//go:build .*enterprise|// \+build .*enterprise|namros_enterprise' cmd internal scripts Makefile

log "verify public Go module paths"
if ! "$RG" -n '^module github\.com/nosway/namros$' go.mod >/dev/null; then
	error "go.mod module path must be github.com/nosway/namros"
fi
if ! "$RG" -n 'github\.com/nosway/namrbd[[:space:]]+v' go.mod >/dev/null; then
	error "go.mod must depend on github.com/nosway/namrbd"
fi
check_local_namrbd_replace
check_absent "local NAMRBD replace leaked outside go.mod/release tooling" '\.\./NAMRBD|replace[[:space:]]+.*namrbd[[:space:]]*=>' Makefile cmd internal scripts/chaos scripts/compat scripts/coordination scripts/docs scripts/lab scripts/metadata scripts/ops scripts/perf
check_absent "short NAMRBD import path leaked" '"namrbd/' cmd internal scripts/release/community-source-overlays
check_absent "short NAMROS internal import path leaked" '"namros/internal/' cmd internal scripts/release/community-source-overlays

log "verify Community identity file"
if ! "$RG" -n '^const current = Community$' internal/edition/current_community.go >/dev/null; then
	error "internal/edition/current_community.go must fix current edition to Community"
fi

log "verify edition build targets keep Enterprise overlay private"
if ! "$RG" -n '^build-community:' Makefile >/dev/null; then
	error "Makefile must provide build-community"
fi
if ! "$RG" -n '^build-enterprise:' Makefile >/dev/null; then
	error "Makefile must provide build-enterprise"
fi
if ! "$RG" -n '^check-edition-boundary:' Makefile >/dev/null; then
	error "Makefile must provide check-edition-boundary"
fi
if ! "$RG" -n '^check-community-export:' Makefile >/dev/null; then
	error "Makefile must provide check-community-export"
fi
if ! "$RG" -n '^export-community:' Makefile >/dev/null; then
	error "Makefile must provide export-community"
fi
if ! "$RG" -n '^test-community-export:' Makefile >/dev/null; then
	error "Makefile must provide test-community-export"
fi
if ! "$RG" -n '^VERSION_PACKAGE \?= github\.com/nosway/namros/internal/version$' Makefile >/dev/null; then
	error "Makefile must define the internal/version linker package"
fi
if ! "$RG" -n 'GO_LDFLAGS_VERSION .*\.Version=' Makefile >/dev/null; then
	error "Makefile must stamp internal/version.Version with linker flags"
fi
if ! "$RG" -n -- '-ldflags' Makefile >/dev/null; then
	error "Makefile build rules must pass Go linker flags"
fi
log "verify public docs reference public Makefile targets"
if ! check_public_doc_make_targets; then
	fail=1
fi
if scripts/release/check-enterprise-build-source.sh "$ROOT_DIR" >/dev/null 2>&1; then
	error "public Community tree must not be accepted as an Enterprise build source"
fi

log "verify source export exclude manifest"
check_allowlist_manifest
check_exclude_manifest

if [ "$fail" -ne 0 ]; then
	exit 1
fi

if [ "${NAMROS_COMMUNITY_CHECK_SKIP_GO_TEST:-false}" != "true" ]; then
	log "run Community gate package tests"
	mkdir -p "$ROOT_DIR/.cache/go-build"
	IFS=' ' read -r -a packages <<<"${NAMROS_COMMUNITY_CHECK_PACKAGES:-./internal/edition ./internal/config ./cmd/namros-admin ./cmd/namros-mcp ./internal/mcpops ./internal/gateway}"
	test_root="$(prepare_community_test_tree)"
	(
		cd "$test_root"
		GOCACHE="${GOCACHE:-$ROOT_DIR/.cache/go-build}" "$GO" test "${packages[@]}"
	)
fi

log "passed"

#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
ALLOWLIST_FILE="$SCRIPT_DIR/community-source-allowlist.txt"
EXCLUDES_FILE="$SCRIPT_DIR/community-source-excludes.txt"
OVERLAY_DIR="$SCRIPT_DIR/community-source-overlays"
STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
EXPORT_ROOT="${NAMROS_COMMUNITY_EXPORT_ROOT:-$ROOT_DIR/dist/community-source-$STAMP}"
EXPORT_DIR="${NAMROS_COMMUNITY_EXPORT_DIR:-$EXPORT_ROOT/namros-community}"
TARBALL="${NAMROS_COMMUNITY_EXPORT_TARBALL:-$ROOT_DIR/dist/namros-community-$STAMP.tar.gz}"
go_mod_rewrite="none"
doc_path_rewrite="none (docs-src ships verbatim)"

cd "$ROOT_DIR"

log() {
	printf '[community-export] %s\n' "$*"
}

die() {
	printf '[community-export] ERROR: %s\n' "$*" >&2
	exit 1
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		die "missing required command: $cmd"
	fi
}

trim_manifest_line() {
	local value="$1"
	value="${value%%#*}"
	value="${value#"${value%%[![:space:]]*}"}"
	value="${value%"${value##*[![:space:]]}"}"
	printf '%s' "$value"
}

is_excluded() {
	local path="$1"
	local exclude
	for exclude in "${excludes[@]}"; do
		if [ "$path" = "$exclude" ]; then
			return 0
		fi
	done
	return 1
}

is_allowlisted() {
	local path="$1"
	local include
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

export_path() {
	printf '%s' "$1"
}

write_manifest() {
	local manifest="$EXPORT_DIR/COMMUNITY-SOURCE-MANIFEST.txt"
	local commit dirty
	commit="$(git rev-parse --short HEAD 2>/dev/null || printf unknown)"
	dirty="$(git status --short 2>/dev/null || true)"
	if [ -n "$dirty" ]; then
		dirty=true
	else
		dirty=false
	fi
	{
		printf 'NAMROS Community Source Export\n'
		printf 'schema_version: 1\n'
		printf 'generated_at: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
		printf 'git_commit: %s\n' "$commit"
		printf 'git_dirty: %s\n' "$dirty"
		printf 'edition_identity: community\n'
		printf 'allowlist_manifest: scripts/release/community-source-allowlist.txt\n'
		printf 'exclude_manifest: scripts/release/community-source-excludes.txt\n'
		printf 'go_mod_rewrite: %s\n' "$go_mod_rewrite"
		printf 'doc_path_rewrite: %s\n' "$doc_path_rewrite"
		printf 'namrbd_dependency: github.com/nosway/namrbd\n'
		printf '\nIncluded allowlist entries:\n'
		if [ "${#allowlist[@]}" -eq 0 ]; then
			printf '%s\n' '- none'
		else
			for include in "${allowlist[@]}"; do
				printf -- '- %s\n' "$include"
			done
		fi
		printf '\nExcluded tracked path count: %s\n' "${#excludes[@]}"
		printf '\nOverlay paths:\n'
		if [ "${#overlays[@]}" -eq 0 ]; then
			printf '%s\n' '- none'
		else
			for overlay in "${overlays[@]}"; do
				printf -- '- %s\n' "$overlay"
			done
		fi
	} >"$manifest"
}

strip_local_namrbd_replace() {
	local go_mod="$EXPORT_DIR/go.mod"
	local tmp
	if [ ! -f "$go_mod" ]; then
		return 0
	fi
	if ! grep -Eq '^replace[[:space:]]+github\.com/nosway/namrbd[[:space:]]+=>[[:space:]]+\.\./NAMRBD[[:space:]]*$' "$go_mod"; then
		return 0
	fi
	log "remove temporary local NAMRBD replace from exported go.mod"
	tmp="$(mktemp "${TMPDIR:-/tmp}/namros-go-mod.XXXXXX")"
	awk '
		$1 == "replace" && $2 == "github.com/nosway/namrbd" && $3 == "=>" && $4 == "../NAMRBD" { next }
		{ print }
	' "$go_mod" >"$tmp"
	mv "$tmp" "$go_mod"
	go_mod_rewrite="removed_local_namrbd_replace"
}

rewrite_export_paths() {
	:
}

verify_export_layout() {
	# The manual set is built from docs-src and served from GitHub Pages.
	# Shipping rendered HTML would reintroduce a copy that can drift.
	if [ -e "$EXPORT_DIR/docs" ]; then
		die "exported tree must not ship rendered docs/; publish docs-src through Pages"
	fi
	if [ ! -f "$EXPORT_DIR/docs-src/index.md" ]; then
		die "exported documentation source missing: docs-src/index.md"
	fi
	if [ ! -f "$EXPORT_DIR/mkdocs.yml" ]; then
		die "exported documentation config missing: mkdocs.yml"
	fi
}

write_release_metadata() {
	local metadata_dir="$EXPORT_ROOT/release-metadata"
	log "write release artifact metadata $metadata_dir"
	NAMROS_RELEASE_ARTIFACT_DIR="$metadata_dir" \
		NAMROS_RELEASE_ARTIFACTS="$TARBALL" \
		NAMROS_RELEASE_REQUIRE_ARTIFACTS=1 \
		NAMROS_RELEASE_EDITION=community \
		NAMROS_RELEASE_SOURCE_DIR="$EXPORT_DIR" \
		NAMROS_RELEASE_SKIP_GO_LIST=1 \
		"$SCRIPT_DIR/write-release-artifact-metadata.sh"
}

require_cmd git
require_cmd perl
require_cmd tar

if [ "${NAMROS_COMMUNITY_EXPORT_SKIP_CHECK:-false}" != "true" ]; then
	"$SCRIPT_DIR/check-community-source.sh"
fi

if [ -e "$EXPORT_DIR" ]; then
	die "export directory already exists: $EXPORT_DIR"
fi

mkdir -p "$EXPORT_DIR"
mkdir -p "$(dirname -- "$TARBALL")"

allowlist=()
while IFS= read -r line || [ -n "$line" ]; do
	path="$(trim_manifest_line "$line")"
	if [ -n "$path" ]; then
		allowlist+=("$path")
	fi
done <"$ALLOWLIST_FILE"

excludes=()
while IFS= read -r line || [ -n "$line" ]; do
	path="$(trim_manifest_line "$line")"
	if [ -n "$path" ]; then
		excludes+=("$path")
	fi
done <"$EXCLUDES_FILE"

overlays=()

log "copy tracked Community source files to $EXPORT_DIR"
while IFS= read -r path; do
	dst_path="$(export_path "$path")"
	if ! is_allowlisted "$path"; then
		continue
	fi
	if is_excluded "$path"; then
		continue
	fi
	if [ ! -f "$path" ]; then
		continue
	fi
	mkdir -p "$EXPORT_DIR/$(dirname -- "$dst_path")"
	cp -p "$path" "$EXPORT_DIR/$dst_path"
done < <(git ls-files)

if [ -d "$OVERLAY_DIR" ]; then
	log "apply Community source overlays from $OVERLAY_DIR"
	while IFS= read -r -d '' overlay_path; do
		rel="${overlay_path#"$OVERLAY_DIR/"}"
		overlays+=("$rel")
		mkdir -p "$EXPORT_DIR/$(dirname -- "$rel")"
		cp -p "$overlay_path" "$EXPORT_DIR/$rel"
	done < <(find "$OVERLAY_DIR" -type f -print0 | sort -z)
fi

strip_local_namrbd_replace
rewrite_export_paths
verify_export_layout
write_manifest

log "create tarball $TARBALL"
tar -czf "$TARBALL" -C "$(dirname -- "$EXPORT_DIR")" "$(basename -- "$EXPORT_DIR")"
write_release_metadata

log "source directory: $EXPORT_DIR"
log "tarball: $TARBALL"
log "release metadata: $EXPORT_ROOT/release-metadata/release-metadata.json"

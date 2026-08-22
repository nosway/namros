#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
RG="${RG:-rg}"
GREP="${GREP:-grep}"
AWK="${AWK:-awk}"
MAKEFILE="${NAMROS_DOCS_PUBLIC_MAKEFILE:-$ROOT_DIR/Makefile}"

cd "$ROOT_DIR"

fail=0
target_file=""

cleanup() {
	if [ -n "$target_file" ]; then
		rm -f "$target_file"
	fi
}

trap cleanup EXIT

log() {
	printf '[html-docs-check] %s\n' "$*"
}

error() {
	printf '[html-docs-check] ERROR: %s\n' "$*" >&2
	fail=1
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		error "missing required command: $cmd"
	fi
}

search_doc_lines() {
	local pattern="$1"
	shift
	if command -v "$RG" >/dev/null 2>&1; then
		"$RG" -n "$pattern" "$@" 2>/dev/null || true
	else
		"$GREP" -RInE -- "$pattern" "$@" 2>/dev/null || true
	fi
}

search_doc_matches() {
	local pattern="$1"
	shift
	if command -v "$RG" >/dev/null 2>&1; then
		"$RG" -n -o "$pattern" "$@" 2>/dev/null || true
	else
		"$GREP" -RInEo -- "$pattern" "$@" 2>/dev/null || true
	fi
}

add_existing_root() {
	local path="$1"
	if [ -e "$path" ]; then
		doc_roots+=("$path")
	fi
}

require_cmd "$GREP"
require_cmd "$AWK"

doc_roots=()
add_existing_root README.md
add_existing_root CONTRIBUTING.md
add_existing_root SECURITY.md
add_existing_root CODE_OF_CONDUCT.md
add_existing_root docs-src
add_existing_root .github

if [ "${#doc_roots[@]}" -eq 0 ]; then
	error "no documentation roots found to scan"
fi

if [ ! -f "$MAKEFILE" ]; then
	error "public Makefile not found: $MAKEFILE"
else
	target_file="$(mktemp "${TMPDIR:-/tmp}/namros-doc-make-targets.XXXXXX")"
	"$AWK" -F: '/^[A-Za-z0-9_.%\/-]+[[:space:]]*:/ { target=$1; sub(/[[:space:]].*/, "", target); print target }' "$MAKEFILE" | sort -u >"$target_file"
fi

if [ "$fail" -ne 0 ]; then
	exit 1
fi

log "scan stale CLI flags and environment names"
stale_cli_pattern='(^|[^[:alnum:]_-])-coordination-endpoints|(^|[^[:alnum:]_-])-metadata-endpoints|(^|[^[:alnum:]_-])-metadata-keyspace|(^|[^[:alnum:]_-])-pd-endpoints|(^|[^[:alnum:]_-])-sbs-admin-endpoints?|(^|[^[:alnum:]_-])--admin-http-endpoint|(^|[^[:alnum:]_])NAMROS_SBS_ADMIN_ENDPOINT|(^|[^[:alnum:]_])SBS_ADMIN_ENDPOINTS|(^|[^[:alnum:]_])ETCD_ENDPOINTS=|(^|[^[:alnum:]_])ETCD_ROOT='
stale_hits="$(search_doc_lines "$stale_cli_pattern" "${doc_roots[@]}")"
if [ -n "$stale_hits" ]; then
	error "stale CLI flag or environment variable in public documentation"
	printf '%s\n' "$stale_hits" >&2
fi

log "verify documented make targets exist in public Makefile"
make_ref_pattern='make[[:space:]]+[A-Za-z0-9_.]+-[A-Za-z0-9_.-]+'
while IFS= read -r hit; do
	file="${hit%%:*}"
	rest="${hit#*:}"
	line="${rest%%:*}"
	match="${rest#*:}"
	target="$(printf '%s\n' "$match" | "$AWK" '{ print $2 }')"
	if [ -z "$target" ]; then
		continue
	fi
	if ! grep -Fxq "$target" "$target_file"; then
		error "public docs reference missing Makefile target: $file:$line: make $target"
	fi
done < <(search_doc_matches "$make_ref_pattern" "${doc_roots[@]}")

if [ "$fail" -ne 0 ]; then
	exit 1
fi

log "passed"

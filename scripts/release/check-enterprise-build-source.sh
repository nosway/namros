#!/usr/bin/env bash

set -euo pipefail

REPO_DIR="${1:-$(pwd)}"
GO="${GO:-go}"
RG="${RG:-rg}"
RG_CHECK_EXCLUDES=(
	--glob '!scripts/release/check-enterprise-build-source.sh'
	--glob '!scripts/release/check-community-source.sh'
	--glob '!scripts/release/check-publication-readiness.sh'
)

log() {
	printf '[enterprise-build-check] %s\n' "$*"
}

die() {
	printf '[enterprise-build-check] ERROR: %s\n' "$*" >&2
	exit 1
}

require_cmd() {
	local cmd="$1"
	if ! command -v "$cmd" >/dev/null 2>&1; then
		die "missing required command: $cmd"
	fi
}

require_cmd git
require_cmd "$GO"
require_cmd "$RG"

if [ ! -d "$REPO_DIR" ]; then
	die "enterprise source directory not found: $REPO_DIR"
fi
if [ ! -f "$REPO_DIR/go.mod" ]; then
	die "go.mod not found in enterprise source directory: $REPO_DIR"
fi

REPO_DIR="$(cd -- "$REPO_DIR" && pwd)"
cd "$REPO_DIR"

if ! "$RG" -n '^module github\.com/nosway/namros$' go.mod >/dev/null; then
	die "enterprise source must use module github.com/nosway/namros"
fi

identity_hits="$("$RG" -n '^const current = Enterprise$' internal/edition 2>/dev/null || true)"
if [ -z "$identity_hits" ]; then
	die "enterprise source must provide internal/edition current identity as Enterprise; use a private overlay checkout via NAMROS_ENTERPRISE_REPO"
fi

community_identity_hits="$("$RG" -n '^const current = Community$' internal/edition 2>/dev/null || true)"
if [ -n "$community_identity_hits" ]; then
	die "enterprise source still contains Community current identity; replace the Community identity file in the private overlay"
fi

if "$RG" -n "${RG_CHECK_EXCLUDES[@]}" -- 'NAMROS_EDITION|"-edition"|StringVar\(&cfg\.Edition|Var\(&cfg\.Edition' cmd internal scripts Makefile >/dev/null 2>&1; then
	die "enterprise source must not expose a runtime edition switch"
fi

log "enterprise source accepted: $REPO_DIR"

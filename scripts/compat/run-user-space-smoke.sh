#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

ran=0
failed=0

run_if_ready() {
	local label="$1"
	local script="$2"
	shift 2

	for cmd in "$@"; do
		if ! has_cmd "$cmd"; then
			log "skip $label: $cmd is not installed"
			return 0
		fi
	done

	ran=1
	log "run $label"
	if ! bash "$SCRIPT_DIR/$script"; then
		failed=1
	fi
}

run_if_ready "AWS CLI" "awscli-smoke.sh" aws jq curl
run_if_ready "MinIO client" "mc-smoke.sh" mc aws jq curl
run_if_ready "rclone" "rclone-smoke.sh" rclone aws jq curl

if [ "$ran" -eq 0 ]; then
	die "no supported user-space clients were found; install awscli+jq+curl, mc, or rclone"
fi
if [ "$failed" -ne 0 ]; then
	die "one or more compatibility smoke tests failed"
fi

log "user-space compatibility smoke passed"

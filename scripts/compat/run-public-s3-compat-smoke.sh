#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

for cmd in aws jq curl mc rclone; do
	require_cmd "$cmd"
done

export NAMROS_COMPAT_AUTOSTART_GATEWAY="${NAMROS_COMPAT_AUTOSTART_GATEWAY:-1}"
export NAMROS_COMPAT_CLIENT_LARGE_OBJECT_MIB="${NAMROS_COMPAT_CLIENT_LARGE_OBJECT_MIB:-8}"
export NAMROS_COMPAT_MC_LARGE_OBJECT_MIB="${NAMROS_COMPAT_MC_LARGE_OBJECT_MIB:-70}"
export NAMROS_COMPAT_RCLONE_LARGE_OBJECT_MIB="${NAMROS_COMPAT_RCLONE_LARGE_OBJECT_MIB:-2}"
export NAMROS_COMPAT_RCLONE_UPLOAD_CUTOFF="${NAMROS_COMPAT_RCLONE_UPLOAD_CUTOFF:-1Mi}"

log "public S3 compatibility smoke: aws-cli + mc + rclone"
log "coverage: bucket, object, multipart-sized object, versioned object writes"

bash "$SCRIPT_DIR/awscli-smoke.sh"
bash "$SCRIPT_DIR/mc-smoke.sh"
bash "$SCRIPT_DIR/rclone-smoke.sh"

log "public S3 compatibility smoke passed"

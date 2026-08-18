#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

require_cmd rclone
require_cmd aws
require_cmd jq
require_cmd curl

bucket="$(compat_bucket rclone)"
tmpdir="$(make_tmpdir)"
compat_autostart_gateway "$tmpdir"
remote=":s3:$bucket"

aws_s3api() {
	env \
		AWS_ACCESS_KEY_ID="$NAMROS_ACCESS_KEY_ID" \
		AWS_SECRET_ACCESS_KEY="$NAMROS_SECRET_ACCESS_KEY" \
		AWS_DEFAULT_REGION="$NAMROS_REGION" \
		AWS_CONFIG_FILE="$tmpdir/aws-config" \
		AWS_EC2_METADATA_DISABLED=true \
		aws --endpoint-url "$NAMROS_ENDPOINT" --region "$NAMROS_REGION" s3api "$@"
}

cleanup_bucket() {
	local versions
	local objects

	if ! aws_s3api head-bucket --bucket "$bucket" >/dev/null 2>&1; then
		return 0
	fi
	versions="$(aws_s3api list-object-versions --bucket "$bucket" --output json 2>/dev/null || printf '{}')"
	while IFS=$'\t' read -r key version_id; do
		[ -n "$key" ] || continue
		aws_s3api delete-object --bucket "$bucket" --key "$key" --version-id "$version_id" >/dev/null || true
	done < <(printf '%s' "$versions" | jq -r '.Versions[]? | [.Key, .VersionId] | @tsv')
	while IFS=$'\t' read -r key version_id; do
		[ -n "$key" ] || continue
		aws_s3api delete-object --bucket "$bucket" --key "$key" --version-id "$version_id" >/dev/null || true
	done < <(printf '%s' "$versions" | jq -r '.DeleteMarkers[]? | [.Key, .VersionId] | @tsv')

	objects="$(aws_s3api list-objects-v2 --bucket "$bucket" --output json 2>/dev/null || printf '{}')"
	while IFS= read -r key; do
		[ -n "$key" ] || continue
		aws_s3api delete-object --bucket "$bucket" --key "$key" >/dev/null || true
	done < <(printf '%s' "$objects" | jq -r '.Contents[]?.Key')
	aws_s3api delete-bucket --bucket "$bucket" >/dev/null || true
}

rclone_cmd() {
	rclone \
		--config "$tmpdir/rclone.conf" \
		--s3-provider Other \
		--s3-env-auth=false \
		--s3-access-key-id "$NAMROS_ACCESS_KEY_ID" \
		--s3-secret-access-key "$NAMROS_SECRET_ACCESS_KEY" \
		--s3-region "$NAMROS_REGION" \
		--s3-location-constraint "$NAMROS_REGION" \
		--s3-endpoint "$NAMROS_ENDPOINT" \
		--s3-force-path-style=true \
		--s3-no-check-bucket=true \
		--s3-upload-cutoff "${NAMROS_COMPAT_RCLONE_UPLOAD_CUTOFF:-5Mi}" \
		--s3-upload-concurrency 1 \
		--transfers 1 \
		--checkers 1 \
		"$@"
}

cleanup() {
	local status=$?

	if [ "$NAMROS_KEEP_BUCKET" = "1" ]; then
		log "leaving bucket in place: $bucket"
	else
		cleanup_bucket
	fi
	compat_stop_autostart_gateway "$status"
	rm -rf "$tmpdir"
	exit "$status"
}
trap cleanup EXIT

log "rclone smoke: endpoint=$NAMROS_ENDPOINT bucket=$bucket"

cat >"$tmpdir/aws-config" <<EOF
[default]
region = $NAMROS_REGION
s3 =
    addressing_style = path
    signature_version = s3v4
EOF
: >"$tmpdir/rclone.conf"

aws_s3api create-bucket --bucket "$bucket" >/dev/null

small="$tmpdir/small.txt"
small_download="$tmpdir/small.downloaded.txt"
printf 'hello from rclone\n' >"$small"

log "copy/list/read"
rclone_cmd copyto "$small" "$remote/small.txt"
rclone_cmd lsf "$remote" | grep -q '^small.txt$' || die "rclone lsf did not show small.txt"
rclone_cmd copyto "$remote/small.txt" "$small_download"
assert_file_equals "$small" "$small_download" "rclone small object"

log "move/delete"
rclone_cmd moveto "$remote/small.txt" "$remote/moved.txt"
rclone_cmd lsf "$remote" | grep -q '^moved.txt$' || die "rclone lsf did not show moved.txt"
rclone_cmd deletefile "$remote/moved.txt"

large_mib="${NAMROS_COMPAT_RCLONE_LARGE_OBJECT_MIB:-${NAMROS_COMPAT_CLIENT_LARGE_OBJECT_MIB:-8}}"
[[ "$large_mib" =~ ^[1-9][0-9]*$ ]] || die "NAMROS_COMPAT_RCLONE_LARGE_OBJECT_MIB must be a positive integer"
log "large object copy (${large_mib}MiB)"
large="$tmpdir/large.bin"
dd if=/dev/urandom of="$large" bs=1048576 count="$large_mib" >/dev/null 2>&1
rclone_cmd copyto "$large" "$remote/large.bin"
rclone_cmd copyto "$remote/large.bin" "$tmpdir/large.downloaded.bin"
assert_file_equals "$large" "$tmpdir/large.downloaded.bin" "rclone large object"

log "versioned object writes"
aws_s3api put-bucket-versioning --bucket "$bucket" --versioning-configuration Status=Enabled >/dev/null
printf 'rclone version one\n' >"$tmpdir/version-one.txt"
printf 'rclone version two\n' >"$tmpdir/version-two.txt"
rclone_cmd copyto "$tmpdir/version-one.txt" "$remote/versioned.txt"
rclone_cmd copyto "$tmpdir/version-two.txt" "$remote/versioned.txt"
aws_s3api list-object-versions --bucket "$bucket" --prefix versioned.txt --output json >"$tmpdir/versions.json"
jq -e '[.Versions[]? | select(.Key == "versioned.txt")] | length >= 2' "$tmpdir/versions.json" >/dev/null
rclone_cmd copyto "$remote/versioned.txt" "$tmpdir/version-latest.downloaded.txt"
assert_file_equals "$tmpdir/version-two.txt" "$tmpdir/version-latest.downloaded.txt" "rclone latest version"

log "rclone smoke passed"

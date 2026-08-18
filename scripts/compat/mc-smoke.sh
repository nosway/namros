#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

require_cmd mc
require_cmd aws
require_cmd jq
require_cmd curl

bucket="$(compat_bucket mc)"
tmpdir="$(make_tmpdir)"
compat_autostart_gateway "$tmpdir"
alias_name="namros"

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

mc_cmd() {
	mc --config-dir "$tmpdir/mc" "$@"
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

log "mc smoke: endpoint=$NAMROS_ENDPOINT bucket=$bucket"

cat >"$tmpdir/aws-config" <<EOF
[default]
region = $NAMROS_REGION
s3 =
    addressing_style = path
    signature_version = s3v4
EOF

mc_cmd alias set "$alias_name" "$NAMROS_ENDPOINT" "$NAMROS_ACCESS_KEY_ID" "$NAMROS_SECRET_ACCESS_KEY" --api S3v4 --path on >/dev/null
mc_cmd mb "$alias_name/$bucket" >/dev/null

small="$tmpdir/small.txt"
small_download="$tmpdir/small.downloaded.txt"
printf 'hello from mc\n' >"$small"

log "copy/cat/stat/list"
mc_cmd cp "$small" "$alias_name/$bucket/small.txt" >/dev/null
mc_cmd stat "$alias_name/$bucket/small.txt" >/dev/null
mc_cmd cat "$alias_name/$bucket/small.txt" >"$small_download"
assert_file_equals "$small" "$small_download" "mc small object"
mc_cmd ls "$alias_name/$bucket" | grep -q 'small.txt' || die "mc ls did not show small.txt"

log "server-side move"
mc_cmd cp "$alias_name/$bucket/small.txt" "$alias_name/$bucket/copy.txt" >/dev/null
mc_cmd mv "$alias_name/$bucket/copy.txt" "$alias_name/$bucket/moved.txt" >/dev/null
mc_cmd stat "$alias_name/$bucket/moved.txt" >/dev/null

large_mib="${NAMROS_COMPAT_MC_LARGE_OBJECT_MIB:-${NAMROS_COMPAT_CLIENT_LARGE_OBJECT_MIB:-8}}"
[[ "$large_mib" =~ ^[1-9][0-9]*$ ]] || die "NAMROS_COMPAT_MC_LARGE_OBJECT_MIB must be a positive integer"
log "large object copy (${large_mib}MiB)"
large="$tmpdir/large.bin"
dd if=/dev/urandom of="$large" bs=1048576 count="$large_mib" >/dev/null 2>&1
mc_cmd cp "$large" "$alias_name/$bucket/large.bin" >/dev/null
mc_cmd cp "$alias_name/$bucket/large.bin" "$tmpdir/large.downloaded.bin" >/dev/null
assert_file_equals "$large" "$tmpdir/large.downloaded.bin" "mc large object"

log "versioned object writes"
aws_s3api put-bucket-versioning --bucket "$bucket" --versioning-configuration Status=Enabled >/dev/null
printf 'mc version one\n' >"$tmpdir/version-one.txt"
printf 'mc version two\n' >"$tmpdir/version-two.txt"
mc_cmd cp "$tmpdir/version-one.txt" "$alias_name/$bucket/versioned.txt" >/dev/null
mc_cmd cp "$tmpdir/version-two.txt" "$alias_name/$bucket/versioned.txt" >/dev/null
aws_s3api list-object-versions --bucket "$bucket" --prefix versioned.txt --output json >"$tmpdir/versions.json"
jq -e '[.Versions[]? | select(.Key == "versioned.txt")] | length >= 2' "$tmpdir/versions.json" >/dev/null
mc_cmd cp "$alias_name/$bucket/versioned.txt" "$tmpdir/version-latest.downloaded.txt" >/dev/null
assert_file_equals "$tmpdir/version-two.txt" "$tmpdir/version-latest.downloaded.txt" "mc latest version"

log "mc smoke passed"

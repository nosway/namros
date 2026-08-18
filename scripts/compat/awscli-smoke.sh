#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
. "$SCRIPT_DIR/common.sh"

require_cmd aws
require_cmd jq
require_cmd curl

NAMROS_COMPAT_AWS_CLI_CONNECT_TIMEOUT="${NAMROS_COMPAT_AWS_CLI_CONNECT_TIMEOUT:-60}"
NAMROS_COMPAT_AWS_CLI_READ_TIMEOUT="${NAMROS_COMPAT_AWS_CLI_READ_TIMEOUT:-60}"

aws_s3api() {
	env \
		AWS_ACCESS_KEY_ID="$NAMROS_ACCESS_KEY_ID" \
		AWS_SECRET_ACCESS_KEY="$NAMROS_SECRET_ACCESS_KEY" \
		AWS_DEFAULT_REGION="$NAMROS_REGION" \
		AWS_CONFIG_FILE="$tmpdir/aws-config" \
		AWS_EC2_METADATA_DISABLED=true \
		aws \
		--cli-connect-timeout "$NAMROS_COMPAT_AWS_CLI_CONNECT_TIMEOUT" \
		--cli-read-timeout "$NAMROS_COMPAT_AWS_CLI_READ_TIMEOUT" \
		--endpoint-url "$NAMROS_ENDPOINT" \
		--region "$NAMROS_REGION" \
		s3api "$@"
}

aws_s3() {
	env \
		AWS_ACCESS_KEY_ID="$NAMROS_ACCESS_KEY_ID" \
		AWS_SECRET_ACCESS_KEY="$NAMROS_SECRET_ACCESS_KEY" \
		AWS_DEFAULT_REGION="$NAMROS_REGION" \
		AWS_CONFIG_FILE="$tmpdir/aws-config" \
		AWS_EC2_METADATA_DISABLED=true \
		aws \
		--cli-connect-timeout "$NAMROS_COMPAT_AWS_CLI_CONNECT_TIMEOUT" \
		--cli-read-timeout "$NAMROS_COMPAT_AWS_CLI_READ_TIMEOUT" \
		--endpoint-url "$NAMROS_ENDPOINT" \
		--region "$NAMROS_REGION" \
		s3 "$@"
}

cleanup_bucket() {
	local bucket="$1"
	local versions
	local objects

	if ! aws_s3api head-bucket --bucket "$bucket" >/dev/null 2>&1; then
		return 0
	fi

	versions="$(aws_s3api list-object-versions --bucket "$bucket" --output json 2>/dev/null || printf '{}')"
	while IFS=$'\t' read -r key version_id; do
		[ -n "$key" ] || continue
		if [ "${NAMROS_COMPAT_ENABLE_OBJECT_LOCK:-0}" = "1" ]; then
			aws_s3api delete-object --bucket "$bucket" --key "$key" --version-id "$version_id" --bypass-governance-retention >/dev/null || true
		else
			aws_s3api delete-object --bucket "$bucket" --key "$key" --version-id "$version_id" >/dev/null || true
		fi
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

write_complete_multipart_json() {
	local path="$1"
	local etag1="$2"
	local etag2="$3"

	jq -n \
		--arg etag1 "$etag1" \
		--arg etag2 "$etag2" \
		'{Parts: [{ETag: $etag1, PartNumber: 1}, {ETag: $etag2, PartNumber: 2}]}' >"$path"
}

future_rfc3339() {
	local days="${1:-2}"

	if date -u -d "+$days days" '+%Y-%m-%dT%H:%M:%SZ' >/dev/null 2>&1; then
		date -u -d "+$days days" '+%Y-%m-%dT%H:%M:%SZ'
		return 0
	fi
	if date -u -v+"$days"d '+%Y-%m-%dT%H:%M:%SZ' >/dev/null 2>&1; then
		date -u -v+"$days"d '+%Y-%m-%dT%H:%M:%SZ'
		return 0
	fi
	printf '2035-01-02T03:04:05Z\n'
}

assert_retention_json() {
	local path="$1"
	local mode="$2"
	local retain_until="$3"

	jq -e \
		--arg mode "$mode" \
		--arg retain_until "$retain_until" \
		'
		def normalize_timestamp:
			tostring
			| sub("\\.[0-9]+Z$"; "Z")
			| sub("\\.[0-9]+\\+00:00$"; "Z")
			| sub("\\+00:00$"; "Z");
		.Retention.Mode == $mode
		and (.Retention.RetainUntilDate | normalize_timestamp) == ($retain_until | normalize_timestamp)
		' "$path" >/dev/null
}

bucket="$(compat_bucket awscli)"
tmpdir="$(make_tmpdir)"
compat_autostart_gateway "$tmpdir"

cat >"$tmpdir/aws-config" <<EOF
[default]
region = $NAMROS_REGION
s3 =
    addressing_style = path
    signature_version = s3v4
EOF

cleanup() {
	local status=$?

	if [ "$NAMROS_KEEP_BUCKET" = "1" ]; then
		log "leaving bucket in place: $bucket"
	else
		cleanup_bucket "$bucket"
	fi
	compat_stop_autostart_gateway "$status"
	rm -rf "$tmpdir"
	exit "$status"
}
trap cleanup EXIT

log "AWS CLI smoke: endpoint=$NAMROS_ENDPOINT bucket=$bucket region=$NAMROS_REGION"

if [ "${NAMROS_COMPAT_ENABLE_OBJECT_LOCK:-0}" = "1" ]; then
	aws_s3api create-bucket --bucket "$bucket" --object-lock-enabled-for-bucket >/dev/null
else
	aws_s3api create-bucket --bucket "$bucket" >/dev/null
fi

small="$tmpdir/small.txt"
small_download="$tmpdir/small.downloaded.txt"
range_download="$tmpdir/small.range.txt"
printf 'hello namros compatibility\n' >"$small"

log "put/get/head/range/list"
aws_s3api put-object \
	--bucket "$bucket" \
	--key small.txt \
	--body "$small" \
	--metadata color=blue \
	--acl private \
	--storage-class STANDARD_IA >/dev/null
aws_s3api head-object --bucket "$bucket" --key small.txt --output json >"$tmpdir/head-small.json"
jq -e '.Metadata | to_entries | any(.[]; (.key | ascii_downcase) == "color" and .value == "blue")' "$tmpdir/head-small.json" >/dev/null
jq -e '.StorageClass == "STANDARD_IA"' "$tmpdir/head-small.json" >/dev/null
aws_s3api get-object --bucket "$bucket" --key small.txt "$small_download" >/dev/null
assert_file_equals "$small" "$small_download" "small object"
aws_s3api get-object --bucket "$bucket" --key small.txt --range bytes=0-4 "$range_download" >/dev/null
[ "$(cat "$range_download")" = "hello" ] || die "range GET returned unexpected content"
aws_s3api list-objects-v2 --bucket "$bucket" --prefix small --output json >"$tmpdir/list-v2.json"
jq -e 'any(.Contents[]?; .Key == "small.txt")' "$tmpdir/list-v2.json" >/dev/null
aws_s3api list-objects --bucket "$bucket" --prefix small --output json >"$tmpdir/list-v1.json"
jq -e 'any(.Contents[]?; .Key == "small.txt")' "$tmpdir/list-v1.json" >/dev/null

log "copy/self-copy metadata replace/tagging"
aws_s3api copy-object --bucket "$bucket" --key copy.txt --copy-source "$bucket/small.txt" >/dev/null
aws_s3api copy-object \
	--bucket "$bucket" \
	--key copy.txt \
	--copy-source "$bucket/copy.txt" \
	--metadata-directive REPLACE \
	--metadata color=green \
	--storage-class ONEZONE_IA >/dev/null
aws_s3api head-object --bucket "$bucket" --key copy.txt --output json >"$tmpdir/head-copy.json"
jq -e '.Metadata | to_entries | any(.[]; (.key | ascii_downcase) == "color" and .value == "green")' "$tmpdir/head-copy.json" >/dev/null
jq -e '.StorageClass == "ONEZONE_IA"' "$tmpdir/head-copy.json" >/dev/null
aws_s3api put-object-tagging \
	--bucket "$bucket" \
	--key copy.txt \
	--tagging 'TagSet=[{Key=color,Value=green},{Key=tool,Value=awscli}]' >/dev/null
aws_s3api get-object-tagging --bucket "$bucket" --key copy.txt --output json >"$tmpdir/tags.json"
jq -e 'any(.TagSet[]?; .Key == "tool" and .Value == "awscli")' "$tmpdir/tags.json" >/dev/null
aws_s3api delete-object-tagging --bucket "$bucket" --key copy.txt >/dev/null

log "bucket versioning/delete marker/list versions"
aws_s3api put-bucket-versioning --bucket "$bucket" --versioning-configuration Status=Enabled >/dev/null
printf 'version one\n' >"$tmpdir/version-one.txt"
printf 'version two\n' >"$tmpdir/version-two.txt"
v1="$(aws_s3api put-object --bucket "$bucket" --key versioned.txt --body "$tmpdir/version-one.txt" --output json | jq -r '.VersionId // empty')"
v2="$(aws_s3api put-object --bucket "$bucket" --key versioned.txt --body "$tmpdir/version-two.txt" --output json | jq -r '.VersionId // empty')"
[ -n "$v1" ] || die "first version id was empty"
[ -n "$v2" ] || die "second version id was empty"
aws_s3api delete-object --bucket "$bucket" --key versioned.txt --output json >"$tmpdir/delete-marker.json"
jq -e '.DeleteMarker == true' "$tmpdir/delete-marker.json" >/dev/null
aws_s3api list-object-versions --bucket "$bucket" --prefix versioned.txt --output json >"$tmpdir/versions.json"
jq -e 'any(.Versions[]?; .Key == "versioned.txt") and any(.DeleteMarkers[]?; .Key == "versioned.txt")' "$tmpdir/versions.json" >/dev/null
aws_s3api get-object --bucket "$bucket" --key versioned.txt --version-id "$v1" "$tmpdir/version-one.downloaded.txt" >/dev/null
assert_file_equals "$tmpdir/version-one.txt" "$tmpdir/version-one.downloaded.txt" "versioned object"

if [ "${NAMROS_COMPAT_ENABLE_OBJECT_LOCK:-0}" = "1" ]; then
	log "object lock retention/legal-hold"
	aws_s3api put-object-lock-configuration \
		--bucket "$bucket" \
		--object-lock-configuration 'ObjectLockEnabled=Enabled,Rule={DefaultRetention={Mode=GOVERNANCE,Days=1}}' >/dev/null
	aws_s3api get-object-lock-configuration --bucket "$bucket" --output json >"$tmpdir/object-lock-config.json"
	jq -e '.ObjectLockConfiguration.ObjectLockEnabled == "Enabled" and .ObjectLockConfiguration.Rule.DefaultRetention.Mode == "GOVERNANCE"' "$tmpdir/object-lock-config.json" >/dev/null

	retain_until="$(future_rfc3339 2)"
	updated_retain_until="$(future_rfc3339 3)"
	printf 'locked by governance\n' >"$tmpdir/locked.txt"
	locked_version="$(aws_s3api put-object \
		--bucket "$bucket" \
		--key locked.txt \
		--body "$tmpdir/locked.txt" \
		--object-lock-mode GOVERNANCE \
		--object-lock-retain-until-date "$retain_until" \
		--object-lock-legal-hold-status OFF \
		--output json | jq -r '.VersionId // empty')"
	[ -n "$locked_version" ] || die "locked object version id was empty"
	aws_s3api get-object-retention --bucket "$bucket" --key locked.txt --version-id "$locked_version" --output json >"$tmpdir/locked-retention.json"
	assert_retention_json "$tmpdir/locked-retention.json" GOVERNANCE "$retain_until"
	aws_s3api get-object-legal-hold --bucket "$bucket" --key locked.txt --version-id "$locked_version" --output json >"$tmpdir/locked-legal-hold.json"
	jq -e '.LegalHold.Status == "OFF"' "$tmpdir/locked-legal-hold.json" >/dev/null
	if aws_s3api delete-object --bucket "$bucket" --key locked.txt --version-id "$locked_version" >/dev/null 2>"$tmpdir/locked-delete.err"; then
		die "locked governance delete without bypass unexpectedly succeeded"
	fi
	aws_s3api put-object-retention \
		--bucket "$bucket" \
		--key locked.txt \
		--version-id "$locked_version" \
		--retention "Mode=GOVERNANCE,RetainUntilDate=$updated_retain_until" \
		--bypass-governance-retention >/dev/null
	aws_s3api get-object-retention --bucket "$bucket" --key locked.txt --version-id "$locked_version" --output json >"$tmpdir/locked-retention-updated.json"
	assert_retention_json "$tmpdir/locked-retention-updated.json" GOVERNANCE "$updated_retain_until"
	aws_s3api put-object-legal-hold \
		--bucket "$bucket" \
		--key locked.txt \
		--version-id "$locked_version" \
		--legal-hold Status=ON >/dev/null
	if aws_s3api delete-object --bucket "$bucket" --key locked.txt --version-id "$locked_version" --bypass-governance-retention >/dev/null 2>"$tmpdir/locked-legal-hold-delete.err"; then
		die "locked legal-hold delete unexpectedly succeeded"
	fi
	aws_s3api put-object-legal-hold \
		--bucket "$bucket" \
		--key locked.txt \
		--version-id "$locked_version" \
		--legal-hold Status=OFF >/dev/null
	aws_s3api delete-object --bucket "$bucket" --key locked.txt --version-id "$locked_version" --bypass-governance-retention >/dev/null

	printf 'default retention\n' >"$tmpdir/default-locked.txt"
	default_locked_version="$(aws_s3api put-object --bucket "$bucket" --key default-locked.txt --body "$tmpdir/default-locked.txt" --output json | jq -r '.VersionId // empty')"
	[ -n "$default_locked_version" ] || die "default locked object version id was empty"
	aws_s3api head-object --bucket "$bucket" --key default-locked.txt --output json >"$tmpdir/head-default-locked.json"
	jq -e '.ObjectLockMode == "GOVERNANCE" and (.ObjectLockRetainUntilDate // "") != ""' "$tmpdir/head-default-locked.json" >/dev/null

	copy_locked_retain_until="$(future_rfc3339 4)"
	aws_s3api copy-object \
		--bucket "$bucket" \
		--key copied-locked.txt \
		--copy-source "$bucket/default-locked.txt" \
		--object-lock-mode GOVERNANCE \
		--object-lock-retain-until-date "$copy_locked_retain_until" \
		--object-lock-legal-hold-status OFF >/dev/null
	aws_s3api head-object --bucket "$bucket" --key copied-locked.txt --output json >"$tmpdir/head-copied-locked.json"
	jq -e \
		--arg retain_until "$copy_locked_retain_until" \
		'
		.ObjectLockMode == "GOVERNANCE"
		and (.ObjectLockRetainUntilDate // "" | sub("\\.[0-9]+Z$"; "Z")) == ($retain_until | sub("\\.[0-9]+Z$"; "Z"))
		and .ObjectLockLegalHoldStatus == "OFF"
		' "$tmpdir/head-copied-locked.json" >/dev/null
else
	log "skip object lock retention/legal-hold: NAMROS_COMPAT_ENABLE_OBJECT_LOCK=0"
fi

log "CORS config and preflight"
cat >"$tmpdir/cors.json" <<'JSON'
{
  "CORSRules": [
    {
      "AllowedOrigins": ["http://example.test"],
      "AllowedMethods": ["GET", "PUT"],
      "AllowedHeaders": ["*"],
      "ExposeHeaders": ["ETag"],
      "MaxAgeSeconds": 60
    }
  ]
}
JSON
aws_s3api put-bucket-cors --bucket "$bucket" --cors-configuration "file://$tmpdir/cors.json" >/dev/null
aws_s3api get-bucket-cors --bucket "$bucket" --output json >"$tmpdir/get-cors.json"
jq -e 'any(.CORSRules[]?; any(.AllowedOrigins[]?; . == "http://example.test"))' "$tmpdir/get-cors.json" >/dev/null
preflight_status="$(
	curl -sS -o "$tmpdir/preflight.body" -D "$tmpdir/preflight.headers" -w '%{http_code}' \
		-X OPTIONS \
		-H 'Origin: http://example.test' \
		-H 'Access-Control-Request-Method: PUT' \
		-H 'Access-Control-Request-Headers: x-amz-meta-color' \
		"$NAMROS_ENDPOINT/$bucket/small.txt"
)"
case "$preflight_status" in
	200|204) ;;
	*) die "CORS preflight returned HTTP $preflight_status" ;;
esac
grep -qi '^Access-Control-Allow-Origin: http://example.test' "$tmpdir/preflight.headers" || die "CORS preflight missing allow-origin"

log "presigned GET"
presigned_url="$(aws_s3 presign "s3://$bucket/small.txt" --expires-in 60)"
curl -fsS "$presigned_url" -o "$tmpdir/presigned.downloaded.txt"
assert_file_equals "$small" "$tmpdir/presigned.downloaded.txt" "presigned GET"

log "multipart upload/list/abort"
large="$tmpdir/large.bin"
part1="$tmpdir/large.part1.bin"
part2="$tmpdir/large.part2.bin"
dd if=/dev/urandom of="$large" bs=1048576 count=6 >/dev/null 2>&1
dd if="$large" of="$part1" bs=1048576 count=5 >/dev/null 2>&1
dd if="$large" of="$part2" bs=1048576 skip=5 count=1 >/dev/null 2>&1
upload_id="$(aws_s3api create-multipart-upload --bucket "$bucket" --key large.bin --storage-class GLACIER_IR --output json | jq -r '.UploadId')"
etag1="$(aws_s3api upload-part --bucket "$bucket" --key large.bin --upload-id "$upload_id" --part-number 1 --body "$part1" --output json | jq -r '.ETag')"
etag2="$(aws_s3api upload-part --bucket "$bucket" --key large.bin --upload-id "$upload_id" --part-number 2 --body "$part2" --output json | jq -r '.ETag')"
write_complete_multipart_json "$tmpdir/complete-mpu.json" "$etag1" "$etag2"
aws_s3api complete-multipart-upload \
	--bucket "$bucket" \
	--key large.bin \
	--upload-id "$upload_id" \
	--multipart-upload "file://$tmpdir/complete-mpu.json" >/dev/null
aws_s3api get-object --bucket "$bucket" --key large.bin "$tmpdir/large.downloaded.bin" >/dev/null
assert_file_equals "$large" "$tmpdir/large.downloaded.bin" "multipart object"
aws_s3api head-object --bucket "$bucket" --key large.bin --output json >"$tmpdir/head-large.json"
jq -e '.StorageClass == "GLACIER_IR"' "$tmpdir/head-large.json" >/dev/null

aborted_upload_id="$(aws_s3api create-multipart-upload --bucket "$bucket" --key abandoned.bin --output json | jq -r '.UploadId')"
aws_s3api list-multipart-uploads --bucket "$bucket" --output json >"$tmpdir/uploads.json"
jq -e --arg upload_id "$aborted_upload_id" 'any(.Uploads[]?; .UploadId == $upload_id)' "$tmpdir/uploads.json" >/dev/null
aws_s3api abort-multipart-upload --bucket "$bucket" --key abandoned.bin --upload-id "$aborted_upload_id" >/dev/null

log "AWS CLI smoke passed"

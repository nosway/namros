#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_ROOT="${NAMROS_COMPAT_REPORT_DIR:-$ROOT_DIR/compat-reports/$(date -u +%Y%m%dT%H%M%SZ)}"
TARGETS="${NAMROS_COMPAT_REPORT_TARGETS:-compat-user-space smoke-etcd-registry compat-sbs-cluster-ec smoke-active-active smoke-metadata-backup-restore s3fs-linux}"
MAKE="${MAKE:-make}"
LEGACY_S3FS_TARGET='s3fs-u''11'

mkdir -p "$REPORT_ROOT"

json_escape() {
	local value="$1"
	value="${value//\\/\\\\}"
	value="${value//\"/\\\"}"
	value="${value//$'\n'/\\n}"
	value="${value//$'\r'/}"
	printf '"%s"' "$value"
}

status_label() {
	case "$1" in
	0) printf "passed" ;;
	skipped) printf "skipped" ;;
	*) printf "failed" ;;
	esac
}

write_targets_json() {
	local first_target=1
	local target

	printf '['
	for target in $TARGETS; do
		if [ "$first_target" != "1" ]; then
			printf ', '
		fi
		printf '%s' "$(json_escape "$target")"
		first_target=0
	done
	printf ']'
}

write_entry_json() {
	local first="$1"
	local name="$2"
	local status="$3"
	local exit_code="$4"
	local duration="$5"
	local log_file="$6"
	local note="$7"
	if [ "$first" != "1" ]; then
		printf ',\n' >>"$REPORT_ROOT/compat-matrix.json"
	fi
	printf '    {' >>"$REPORT_ROOT/compat-matrix.json"
	printf '"name":%s,' "$(json_escape "$name")" >>"$REPORT_ROOT/compat-matrix.json"
	printf '"status":%s,' "$(json_escape "$status")" >>"$REPORT_ROOT/compat-matrix.json"
	printf '"exit_code":%s,' "$(json_escape "$exit_code")" >>"$REPORT_ROOT/compat-matrix.json"
	printf '"duration_seconds":%s,' "$duration" >>"$REPORT_ROOT/compat-matrix.json"
	printf '"log":%s,' "$(json_escape "$log_file")" >>"$REPORT_ROOT/compat-matrix.json"
	printf '"note":%s' "$(json_escape "$note")" >>"$REPORT_ROOT/compat-matrix.json"
	printf '}' >>"$REPORT_ROOT/compat-matrix.json"
}

generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
{
	printf '{\n'
	printf '  "generated_at": %s,\n' "$(json_escape "$generated_at")"
	printf '  "targets": '
	write_targets_json
	printf ',\n'
	printf '  "entries": [\n'
} >"$REPORT_ROOT/compat-matrix.json"

{
	printf '# NAMROS Compatibility Matrix\n\n'
	printf '%s\n' "- Generated at: \`$generated_at\`"
	printf '%s\n\n' "- Report directory: \`$REPORT_ROOT\`"
	printf '%s\n\n' "- Targets: \`$TARGETS\`"
	printf '| Target | Status | Exit | Duration | Log | Note |\n'
	printf '| --- | --- | ---: | ---: | --- | --- |\n'
} >"$REPORT_ROOT/compat-matrix.md"

first=1
failed=0

for target in $TARGETS; do
	log_file="$target.log"
	log_path="$REPORT_ROOT/$log_file"
	note=""
	start="$(date +%s)"
	exit_code=0
	if [ "$target" = "s3fs-linux" ] || [ "$target" = "$LEGACY_S3FS_TARGET" ]; then
		status="${NAMROS_COMPAT_S3FS_STATUS:-${NAMROS_COMPAT_S3FS_U11_STATUS:-skipped}}"
		note="${NAMROS_COMPAT_S3FS_NOTE:-${NAMROS_COMPAT_S3FS_U11_NOTE:-manual Linux s3fs-fuse result; set NAMROS_COMPAT_S3FS_STATUS=passed|failed and NAMROS_COMPAT_S3FS_NOTE to record it}}"
		printf '%s\n' "$note" >"$log_path"
		exit_code="manual"
		duration=0
		if [ "$status" = "failed" ]; then
			failed=1
		fi
	else
		set +e
		"$MAKE" -C "$ROOT_DIR" "$target" >"$log_path" 2>&1
		exit_code=$?
		set -e
		end="$(date +%s)"
		duration=$((end - start))
		status="$(status_label "$exit_code")"
		note="make $target"
		if [ "$exit_code" -ne 0 ]; then
			failed=1
		fi
	fi

	write_entry_json "$first" "$target" "$status" "$exit_code" "$duration" "$log_file" "$note"
	first=0
	printf '| `%s` | `%s` | `%s` | `%ss` | `%s` | %s |\n' "$target" "$status" "$exit_code" "$duration" "$log_file" "$note" >>"$REPORT_ROOT/compat-matrix.md"
done

{
	printf '\n  ]\n'
	printf '}\n'
} >>"$REPORT_ROOT/compat-matrix.json"

printf '[compat] report: %s\n' "$REPORT_ROOT/compat-matrix.md"
printf '[compat] report-json: %s\n' "$REPORT_ROOT/compat-matrix.json"

if [ "$failed" -ne 0 ]; then
	exit 1
fi

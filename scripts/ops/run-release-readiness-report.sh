#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
REPORT_ROOT="${NAMROS_RELEASE_REPORT_DIR:-$ROOT_DIR/release-reports/$(date -u +%Y%m%dT%H%M%SZ)}"
TARGETS="${NAMROS_RELEASE_TARGETS:-test smoke-metadata-backup-restore}"
MAKE="${MAKE:-make}"

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
	*) printf "failed" ;;
	esac
}

git_value() {
	local args="$1"
	if command -v git >/dev/null 2>&1; then
		(cd "$ROOT_DIR" && git $args) 2>/dev/null || printf "unknown"
	else
		printf "unknown"
	fi
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
	if [ "$first" != "1" ]; then
		printf ',\n' >>"$REPORT_ROOT/release-readiness.json"
	fi
	printf '    {' >>"$REPORT_ROOT/release-readiness.json"
	printf '"name":%s,' "$(json_escape "$name")" >>"$REPORT_ROOT/release-readiness.json"
	printf '"status":%s,' "$(json_escape "$status")" >>"$REPORT_ROOT/release-readiness.json"
	printf '"exit_code":%s,' "$(json_escape "$exit_code")" >>"$REPORT_ROOT/release-readiness.json"
	printf '"duration_seconds":%s,' "$duration" >>"$REPORT_ROOT/release-readiness.json"
	printf '"log":%s' "$(json_escape "$log_file")" >>"$REPORT_ROOT/release-readiness.json"
	printf '}' >>"$REPORT_ROOT/release-readiness.json"
}

generated_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
git_commit="$(git_value "rev-parse --short HEAD")"
git_dirty="$(git_value "status --short")"
if [ "$git_dirty" = "unknown" ] || [ -z "$git_dirty" ]; then
	git_dirty="false"
else
	git_dirty="true"
fi

{
	printf '{\n'
	printf '  "schema_version": 1,\n'
	printf '  "generated_at": %s,\n' "$(json_escape "$generated_at")"
	printf '  "git_commit": %s,\n' "$(json_escape "$git_commit")"
	printf '  "git_dirty": %s,\n' "$git_dirty"
	printf '  "targets": '
	write_targets_json
	printf ',\n'
	printf '  "entries": [\n'
} >"$REPORT_ROOT/release-readiness.json"

{
	printf '# NAMROS Release Readiness\n\n'
	printf '%s\n' "- Generated at: \`$generated_at\`"
	printf '%s\n' "- Git commit: \`$git_commit\`"
	printf '%s\n' "- Git dirty: \`$git_dirty\`"
	printf '%s\n\n' "- Report directory: \`$REPORT_ROOT\`"
	printf '%s\n\n' "- Targets: \`$TARGETS\`"
	printf '| Target | Status | Exit | Duration | Log |\n'
	printf '| --- | --- | ---: | ---: | --- |\n'
} >"$REPORT_ROOT/release-readiness.md"

first=1
failed=0

for target in $TARGETS; do
	log_file="$target.log"
	log_path="$REPORT_ROOT/$log_file"
	start="$(date +%s)"
	set +e
	"$MAKE" -C "$ROOT_DIR" "$target" >"$log_path" 2>&1
	exit_code=$?
	set -e
	end="$(date +%s)"
	duration=$((end - start))
	status="$(status_label "$exit_code")"
	if [ "$exit_code" -ne 0 ]; then
		failed=1
	fi

	write_entry_json "$first" "$target" "$status" "$exit_code" "$duration" "$log_file"
	first=0
	printf '| `%s` | `%s` | `%s` | `%ss` | `%s` |\n' "$target" "$status" "$exit_code" "$duration" "$log_file" >>"$REPORT_ROOT/release-readiness.md"
done

{
	printf '\n  ]\n'
	printf '}\n'
} >>"$REPORT_ROOT/release-readiness.json"

printf '[release] report: %s\n' "$REPORT_ROOT/release-readiness.md"
printf '[release] report-json: %s\n' "$REPORT_ROOT/release-readiness.json"

if [ "$failed" -ne 0 ]; then
	exit 1
fi

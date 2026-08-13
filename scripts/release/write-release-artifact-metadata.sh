#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "$SCRIPT_DIR/../.." && pwd)"
GO="${GO:-go}"
OUT_DIR="${NAMROS_RELEASE_ARTIFACT_DIR:-$ROOT_DIR/dist/release-metadata}"
SOURCE_DIR="${NAMROS_RELEASE_SOURCE_DIR:-$ROOT_DIR}"
EDITION="${NAMROS_RELEASE_EDITION:-community}"
VERSION="${NAMROS_RELEASE_VERSION:-}"
GENERATE_SBOM="${NAMROS_RELEASE_GENERATE_SBOM:-0}"
REQUIRE_ARTIFACTS="${NAMROS_RELEASE_REQUIRE_ARTIFACTS:-0}"
SKIP_GO_LIST="${NAMROS_RELEASE_SKIP_GO_LIST:-0}"

log() {
	printf '[release-metadata] %s\n' "$*" >&2
}

die() {
	printf '[release-metadata] ERROR: %s\n' "$*" >&2
	exit 1
}

json_escape() {
	local value="$1"
	value="${value//\\/\\\\}"
	value="${value//\"/\\\"}"
	value="${value//$'\n'/\\n}"
	printf '%s' "$value"
}

json_string() {
	printf '"%s"' "$(json_escape "$1")"
}

relpath() {
	local path="$1"
	local out_parent
	out_parent="$(cd -- "$(dirname -- "$OUT_DIR")" 2>/dev/null && pwd || true)"
	case "$path" in
	"$ROOT_DIR"/*)
		printf '%s' "${path#"$ROOT_DIR/"}"
		;;
	"$out_parent"/*)
		printf '%s' "${path#"$out_parent/"}"
		;;
	*)
		basename -- "$path"
		;;
	esac
}

abs_path() {
	local path="$1"
	if [[ "$path" = /* ]]; then
		printf '%s' "$path"
	else
		printf '%s/%s' "$ROOT_DIR" "$path"
	fi
}

sha256_file() {
	local path="$1"
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$path" | awk '{print $1}'
		return 0
	fi
	if command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$path" | awk '{print $1}'
		return 0
	fi
	die "sha256sum or shasum is required"
}

file_size_bytes() {
	local path="$1"
	wc -c <"$path" | tr -d '[:space:]'
}

git_value() {
	local fallback="$1"
	shift
	if git -C "$ROOT_DIR" "$@" 2>/dev/null; then
		return 0
	fi
	printf '%s\n' "$fallback"
}

bool_dirty() {
	if [ -n "$(git -C "$ROOT_DIR" status --short 2>/dev/null || true)" ]; then
		printf 'true'
	else
		printf 'false'
	fi
}

truthy() {
	case "${1:-}" in
	1|true|TRUE|yes|YES)
		return 0
		;;
	*)
		return 1
		;;
	esac
}

discover_artifacts() {
	local path
	if [ -n "${NAMROS_RELEASE_ARTIFACTS:-}" ]; then
		# shellcheck disable=SC2206
		artifacts=($NAMROS_RELEASE_ARTIFACTS)
		return 0
	fi
	if [ -d "$ROOT_DIR/dist" ]; then
		while IFS= read -r path; do
			artifacts+=("$path")
		done < <(find "$ROOT_DIR/dist" -maxdepth 1 -type f \( -name '*.tar.gz' -o -name '*.tgz' -o -name '*.zip' -o -name '*.sbom' -o -name '*.json' \) | sort)
	fi
}

write_go_modules() {
	local go_mod="$SOURCE_DIR/go.mod"
	local go_sum="$SOURCE_DIR/go.sum"
	go_modules_status="generated"
	go_modules_file="$OUT_DIR/go-modules.txt"
	go_modules_error_file="$OUT_DIR/go-list-error.txt"
	go_mod_release_file="$OUT_DIR/go.mod.release"
	go_sum_release_file="$OUT_DIR/go.sum.release"

	if [ -f "$go_mod" ]; then
		cp "$go_mod" "$go_mod_release_file"
	else
		: >"$go_mod_release_file"
	fi
	if [ -f "$go_sum" ]; then
		cp "$go_sum" "$go_sum_release_file"
	else
		: >"$go_sum_release_file"
	fi

	if truthy "$SKIP_GO_LIST"; then
		go_modules_status="fallback_go_mod"
	elif command -v "$GO" >/dev/null 2>&1 && [ -f "$go_mod" ]; then
		if (cd "$SOURCE_DIR" && "$GO" list -m all >"$go_modules_file" 2>"$go_modules_error_file"); then
			rm -f "$go_modules_error_file"
		else
			go_modules_status="fallback_go_mod"
		fi
	else
		go_modules_status="fallback_go_mod"
	fi

	if [ "$go_modules_status" = "fallback_go_mod" ]; then
		{
			awk '$1 == "module" { print $2; exit }' "$go_mod" 2>/dev/null || true
			awk '
				$1 == "require" && NF >= 3 { print $2 " " $3; next }
				$1 == "require" && $2 == "(" { in_require = 1; next }
				in_require && $1 == ")" { in_require = 0; next }
				in_require && NF >= 2 { print $1 " " $2 }
			' "$go_mod" 2>/dev/null || true
		} >"$go_modules_file"
	fi
	go_modules_count="$(grep -cv '^[[:space:]]*$' "$go_modules_file" || true)"
}

write_namrbd_contexts() {
	namrbd_contexts_file="$OUT_DIR/namrbd-contexts.txt"
	: >"$namrbd_contexts_file"
	for file in "$ROOT_DIR/packaging/docker/.env.example" "$ROOT_DIR/packaging/docker/compose.yaml" "$ROOT_DIR/packaging/docker/compose.community.yml"; do
		if [ -f "$file" ]; then
			grep -Eo 'https://github\.com/nosway/namrbd\.git#[A-Za-z0-9._/-]+' "$file" >>"$namrbd_contexts_file" || true
		fi
	done
	if [ -f "$SOURCE_DIR/go.mod" ]; then
		awk '$1 == "github.com/nosway/namrbd" { print "go-module " $1 " " $2 }' "$SOURCE_DIR/go.mod" >>"$namrbd_contexts_file" || true
	fi
	sort -u "$namrbd_contexts_file" -o "$namrbd_contexts_file"
	namrbd_context_count="$(grep -cv '^[[:space:]]*$' "$namrbd_contexts_file" || true)"
}

write_string_array() {
	local file="$1"
	local indent="$2"
	local first=1
	local line
	while IFS= read -r line; do
		[ -n "$line" ] || continue
		if [ "$first" -eq 0 ]; then
			printf ',\n'
		fi
		printf '%s' "$indent"
		json_string "$line"
		first=0
	done <"$file"
	if [ "$first" -eq 0 ]; then
		printf '\n'
	fi
}

write_object_array() {
	local file="$1"
	local indent="$2"
	local first=1
	local line
	while IFS= read -r line; do
		[ -n "$line" ] || continue
		if [ "$first" -eq 0 ]; then
			printf ',\n'
		fi
		printf '%s%s' "$indent" "$line"
		first=0
	done <"$file"
	if [ "$first" -eq 0 ]; then
		printf '\n'
	fi
}

write_sbom_status() {
	sbom_status_file="$OUT_DIR/sbom-status.json"
	sbom_file="$OUT_DIR/sbom.spdx.json"
	sbom_status="not_generated"
	sbom_reason="set NAMROS_RELEASE_GENERATE_SBOM=1 and install syft to generate an SPDX JSON SBOM"
	if truthy "$GENERATE_SBOM"; then
		if command -v syft >/dev/null 2>&1; then
			if syft "dir:$SOURCE_DIR" -o spdx-json >"$sbom_file"; then
				sbom_status="generated"
				sbom_reason="generated by syft"
			else
				sbom_status="failed"
				sbom_reason="syft exited with an error"
				rm -f "$sbom_file"
			fi
		else
			sbom_status="tool_missing"
			sbom_reason="syft is not installed"
		fi
	fi
	{
		printf '{\n'
		printf '  "schema_version": "namros.release_sbom_status.v1",\n'
		printf '  "status": "%s",\n' "$(json_escape "$sbom_status")"
		printf '  "reason": "%s",\n' "$(json_escape "$sbom_reason")"
		printf '  "requested": %s,\n' "$(truthy "$GENERATE_SBOM" && printf true || printf false)"
		if [ -f "$sbom_file" ]; then
			printf '  "sbom_file": "%s"\n' "$(json_escape "$(relpath "$sbom_file")")"
		else
			printf '  "sbom_file": null\n'
		fi
		printf '}\n'
	} >"$sbom_status_file"
}

mkdir -p "$OUT_DIR"
cd "$ROOT_DIR"

if [ -z "$VERSION" ]; then
	VERSION="$(git_value unknown describe --tags --always --dirty)"
fi
REVISION="$(git_value unknown rev-parse HEAD)"
SHORT_REVISION="$(git_value unknown rev-parse --short HEAD)"
DIRTY="$(bool_dirty)"
GENERATED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
CHECKSUMS_FILE="$OUT_DIR/checksums.sha256"
ARTIFACTS_JSONL="$OUT_DIR/artifacts.jsonl"
IMAGE_REFS_FILE="$OUT_DIR/image-refs.txt"
PROVENANCE_FILE="$OUT_DIR/provenance.json"
METADATA_FILE="$OUT_DIR/release-metadata.json"

artifacts=()
discover_artifacts
if [ "${#artifacts[@]}" -eq 0 ] && truthy "$REQUIRE_ARTIFACTS"; then
	die "no release artifacts were provided; set NAMROS_RELEASE_ARTIFACTS"
fi

: >"$CHECKSUMS_FILE"
: >"$ARTIFACTS_JSONL"
for artifact in "${artifacts[@]}"; do
	artifact_abs="$(abs_path "$artifact")"
	if [ ! -f "$artifact_abs" ]; then
		die "release artifact does not exist: $artifact"
	fi
	artifact_rel="$(relpath "$artifact_abs")"
	artifact_sha="$(sha256_file "$artifact_abs")"
	artifact_size="$(file_size_bytes "$artifact_abs")"
	printf '%s  %s\n' "$artifact_sha" "$artifact_rel" >>"$CHECKSUMS_FILE"
	printf '{"path":"%s","sha256":"%s","size_bytes":%s}\n' \
		"$(json_escape "$artifact_rel")" \
		"$(json_escape "$artifact_sha")" \
		"$artifact_size" >>"$ARTIFACTS_JSONL"
done

: >"$IMAGE_REFS_FILE"
if [ -n "${NAMROS_RELEASE_IMAGE_REFS:-}" ]; then
	# shellcheck disable=SC2206
	image_refs=($NAMROS_RELEASE_IMAGE_REFS)
	for image_ref in "${image_refs[@]}"; do
		printf '%s\n' "$image_ref" >>"$IMAGE_REFS_FILE"
	done
fi

write_go_modules
write_namrbd_contexts
write_sbom_status

{
	printf '{\n'
	printf '  "schema_version": "namros.release_provenance.v1",\n'
	printf '  "generated_at": "%s",\n' "$(json_escape "$GENERATED_AT")"
	printf '  "version": "%s",\n' "$(json_escape "$VERSION")"
	printf '  "revision": "%s",\n' "$(json_escape "$REVISION")"
	printf '  "short_revision": "%s",\n' "$(json_escape "$SHORT_REVISION")"
	printf '  "dirty": %s,\n' "$DIRTY"
	printf '  "edition": "%s",\n' "$(json_escape "$EDITION")"
	printf '  "source_dir": "%s",\n' "$(json_escape "$(relpath "$(abs_path "$SOURCE_DIR")")")"
	printf '  "metadata_script": "scripts/release/write-release-artifact-metadata.sh"\n'
	printf '}\n'
} >"$PROVENANCE_FILE"

{
	printf '{\n'
	printf '  "schema_version": "namros.release_artifact_metadata.v1",\n'
	printf '  "generated_at": "%s",\n' "$(json_escape "$GENERATED_AT")"
	printf '  "version": "%s",\n' "$(json_escape "$VERSION")"
	printf '  "revision": "%s",\n' "$(json_escape "$REVISION")"
	printf '  "short_revision": "%s",\n' "$(json_escape "$SHORT_REVISION")"
	printf '  "dirty": %s,\n' "$DIRTY"
	printf '  "edition": "%s",\n' "$(json_escape "$EDITION")"
	printf '  "artifact_count": %s,\n' "${#artifacts[@]}"
	printf '  "artifacts": [\n'
	write_object_array "$ARTIFACTS_JSONL" "    "
	printf '  ],\n'
	printf '  "checksums_file": "%s",\n' "$(json_escape "$(relpath "$CHECKSUMS_FILE")")"
	printf '  "image_refs": [\n'
	write_string_array "$IMAGE_REFS_FILE" "    "
	printf '  ],\n'
	printf '  "dependency_pins": {\n'
	printf '    "go_mod_file": "%s",\n' "$(json_escape "$(relpath "$go_mod_release_file")")"
	printf '    "go_sum_file": "%s",\n' "$(json_escape "$(relpath "$go_sum_release_file")")"
	printf '    "go_modules_file": "%s",\n' "$(json_escape "$(relpath "$go_modules_file")")"
	printf '    "go_modules_status": "%s",\n' "$(json_escape "$go_modules_status")"
	printf '    "go_modules_count": %s,\n' "$go_modules_count"
	printf '    "namrbd_contexts_file": "%s",\n' "$(json_escape "$(relpath "$namrbd_contexts_file")")"
	printf '    "namrbd_context_count": %s\n' "$namrbd_context_count"
	printf '  },\n'
	printf '  "sbom": {\n'
	printf '    "status_file": "%s",\n' "$(json_escape "$(relpath "$sbom_status_file")")"
	printf '    "status": "%s"\n' "$(json_escape "$sbom_status")"
	printf '  },\n'
	printf '  "provenance_file": "%s"\n' "$(json_escape "$(relpath "$PROVENANCE_FILE")")"
	printf '}\n'
} >"$METADATA_FILE"

log "metadata: $METADATA_FILE"
log "checksums: $CHECKSUMS_FILE"
log "dependency pins: $go_modules_file"

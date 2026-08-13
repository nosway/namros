#!/usr/bin/env sh
set -eu

script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
repo_root="$(CDPATH= cd -- "$script_dir/../.." && pwd)"
docker_dir="$repo_root/packaging/docker"
env_file="$docker_dir/.env"
env_example="$docker_dir/.env.example"
namrbd_context_dockerignore="$docker_dir/namrbd-context.dockerignore"
secrets_dir="$docker_dir/secrets"
access_key_file="$secrets_dir/root-access-key-id"
secret_key_file="$secrets_dir/root-secret-access-key"

write_env_value() {
	key="$1"
	value="$2"
	tmp_env_file="$env_file.tmp.$$"
	awk -v key="$key" -v value="$value" '
		BEGIN { written = 0 }
		index($0, key "=") == 1 {
			print key "=" value
			written = 1
			next
		}
		{ print }
		END {
			if (written == 0) {
				print key "=" value
			}
		}
	' "$env_file" >"$tmp_env_file"
	mv "$tmp_env_file" "$env_file"
}

prepare_slim_namrbd_context() {
	source_dir="$1"
	if [ "${NAMROS_CONTAINER_USE_SLIM_NAMRBD_CONTEXT:-1}" = "0" ]; then
		printf '%s\n' "$source_dir"
		return 0
	fi
	if ! command -v git >/dev/null 2>&1 ||
		! git -C "$source_dir" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		printf '%s\n' "$source_dir"
		return 0
	fi

	context_root="${NAMROS_NAMRBD_CONTEXT_CACHE_DIR:-$repo_root/.cache/namrbd-contexts}"
	context_dir="$context_root/local"
	tmp_context_dir="$context_root/local.tmp.$$"
	file_list="$context_root/local-files.$$"
	mkdir -p "$context_root"
	rm -rf "$tmp_context_dir"
	mkdir -p "$tmp_context_dir"

	if ! git -C "$source_dir" ls-files -co --exclude-standard >"$file_list"; then
		rm -rf "$tmp_context_dir" "$file_list"
		printf '%s\n' "$source_dir"
		return 0
	fi
	if [ ! -s "$file_list" ]; then
		rm -rf "$tmp_context_dir" "$file_list"
		printf '%s\n' "$source_dir"
		return 0
	fi
	if ! (cd "$source_dir" && tar -cf - -T "$file_list") | (cd "$tmp_context_dir" && tar -xf -); then
		rm -rf "$tmp_context_dir" "$file_list"
		printf '%s\n' "$source_dir"
		return 0
	fi
	rm -f "$file_list"
	if [ -f "$namrbd_context_dockerignore" ]; then
		cp "$namrbd_context_dockerignore" "$tmp_context_dir/.dockerignore"
	else
		printf '.git/\n.cache/\nbin/\ntmp/\n*.log\n*.test\n' >"$tmp_context_dir/.dockerignore"
	fi
	rm -rf "$context_dir"
	mv "$tmp_context_dir" "$context_dir"
	printf '[container] prepared slim local NAMRBD build context: %s (source: %s)\n' "$context_dir" "$source_dir" >&2
	printf '%s\n' "$context_dir"
}

mkdir -p "$secrets_dir"
chmod 700 "$secrets_dir"

if [ ! -f "$env_file" ]; then
	cp "$env_example" "$env_file"
	printf '[container] created %s from .env.example\n' "$env_file" >&2
fi

appended_env_defaults=0
while IFS= read -r line; do
	case "$line" in
		[A-Za-z_]*=*)
			key="${line%%=*}"
			if ! grep -q "^$key=" "$env_file"; then
				if [ "$appended_env_defaults" -eq 0 ]; then
					printf '\n# Added defaults from .env.example by scripts/container/ensure-local-files.sh\n' >>"$env_file"
					appended_env_defaults=1
				fi
				printf '%s\n' "$line" >>"$env_file"
			fi
			;;
	esac
done <"$env_example"
if [ "$appended_env_defaults" -eq 1 ]; then
	printf '[container] appended missing defaults to %s\n' "$env_file" >&2
fi

default_namrbd_context="$(sed -n 's/^NAMROS_NAMRBD_CONTEXT=//p' "$env_example" | tail -n 1)"
current_namrbd_context="$(sed -n 's/^NAMROS_NAMRBD_CONTEXT=//p' "$env_file" | tail -n 1 || true)"
local_namrbd_context="${NAMROS_LOCAL_NAMRBD_CONTEXT:-}"
if [ -z "$local_namrbd_context" ]; then
	namrbd_dirname="NAMRBD"
	repo_parent="$(CDPATH= cd -- "$repo_root/.." && pwd -P)"
	if [ -d "$repo_parent/$namrbd_dirname" ]; then
		local_namrbd_context="$(CDPATH= cd -- "$repo_parent/$namrbd_dirname" && pwd -P)"
	elif [ -n "${HOME:-}" ] && [ -d "$HOME/$namrbd_dirname" ]; then
		local_namrbd_context="$(CDPATH= cd -- "$HOME/$namrbd_dirname" && pwd -P)"
	fi
fi
if [ -n "$local_namrbd_context" ] &&
	{ [ -z "$current_namrbd_context" ] ||
		[ -n "${NAMROS_LOCAL_NAMRBD_CONTEXT:-}" ] ||
		[ "$current_namrbd_context" = "$default_namrbd_context" ] ||
		[ "$current_namrbd_context" = "$local_namrbd_context" ] ||
		case "$current_namrbd_context" in
			"$repo_root/.cache/namrbd-contexts/"*) true ;;
			*) false ;;
		esac; }; then
	prepared_namrbd_context="$(prepare_slim_namrbd_context "$local_namrbd_context")"
	write_env_value NAMROS_NAMRBD_CONTEXT "$prepared_namrbd_context"
	if [ "$prepared_namrbd_context" = "$local_namrbd_context" ]; then
		printf '[container] using local NAMRBD build context: %s\n' "$local_namrbd_context" >&2
	else
		printf '[container] using slim local NAMRBD build context: %s\n' "$prepared_namrbd_context" >&2
	fi
fi

if [ ! -f "$access_key_file" ]; then
	umask 077
	printf 'namrosroot\n' >"$access_key_file"
	printf '[container] created development access key file: %s\n' "$access_key_file" >&2
fi

if [ ! -f "$secret_key_file" ]; then
	umask 077
	printf 'namrosrootsecret\n' >"$secret_key_file"
	printf '[container] created development secret key file: %s\n' "$secret_key_file" >&2
fi

chmod 444 "$access_key_file" "$secret_key_file"

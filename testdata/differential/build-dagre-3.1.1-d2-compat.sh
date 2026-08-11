#!/bin/sh

set -eu

upstream_commit=c3ed0802cd98de74c21cff1f754689ebbb0f8dae
upstream_url=${DAGRE_UPSTREAM_URL:-https://github.com/dagrejs/dagre.git}
expected_lock_sha256=9f5e1e7a40667dcffc12e35ea5d4db96f346dadf943e97c1ecc2b4dc21afbb2d
expected_patch_sha256=d510c474e1f291c38c14c276f6bc498dbbd0dc7132e3b9694e31b47650356d19
expected_dist_sha256=9b91fccee8e70a74299cf47eaf8100c46a900fb1f334a11424ff3682c1019585
expected_oracle_sha256=8e34c25ed53dbccca2fa206780b0b46974b285c74e0cd7b34d0d1fafa5506cab

script_dir=$(CDPATH= cd -P "$(dirname "$0")" && pwd)
patch_path=$script_dir/dagre-3.1.1-d2-compat.patch

fail() {
	printf '%s\n' "$*" >&2
	exit 1
}

[ "$#" -eq 1 ] || fail "usage: $0 OUTPUT.cjs"
output_path=$1

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | while IFS=' ' read -r hash _; do printf '%s\n' "$hash"; done
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | while IFS=' ' read -r hash _; do printf '%s\n' "$hash"; done
	else
		fail "sha256sum or shasum is required"
	fi
}

for command_name in git install mktemp node npm; do
	command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

actual_patch_sha256=$(sha256_file "$patch_path")
[ "$actual_patch_sha256" = "$expected_patch_sha256" ] ||
	fail "patch SHA-256 is $actual_patch_sha256, want $expected_patch_sha256"

temp_base=${TMPDIR:-/tmp}
temp_base=${temp_base%/}
temp_dir=$(mktemp -d "$temp_base/dagro-dagre-compat.XXXXXX")
case "$temp_dir" in
	"$temp_base"/dagro-dagre-compat.*) ;;
	*) fail "refusing unexpected temporary directory: $temp_dir" ;;
esac

cleanup() {
	case "$temp_dir" in
		"$temp_base"/dagro-dagre-compat.*) rm -rf -- "$temp_dir" ;;
	esac
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

source_dir=$temp_dir/dagre
git init --quiet "$source_dir"
git -C "$source_dir" fetch --quiet --depth=1 --no-tags "$upstream_url" "$upstream_commit"
git -C "$source_dir" checkout --quiet --detach FETCH_HEAD

actual_commit=$(git -C "$source_dir" rev-parse HEAD)
[ "$actual_commit" = "$upstream_commit" ] ||
	fail "upstream commit is $actual_commit, want $upstream_commit"
[ -z "$(git -C "$source_dir" status --short)" ] || fail "upstream checkout is not clean"

actual_lock_sha256=$(sha256_file "$source_dir/package-lock.json")
[ "$actual_lock_sha256" = "$expected_lock_sha256" ] ||
	fail "upstream package-lock.json SHA-256 is $actual_lock_sha256, want $expected_lock_sha256"

git -C "$source_dir" apply --check "$patch_path"
git -C "$source_dir" apply "$patch_path"

(
	cd "$source_dir"
	npm ci --ignore-scripts --no-audit --no-fund
	npm run build
)

esbuild_version=$(cd "$source_dir" && node -p 'require("./node_modules/esbuild/package.json").version')
tsx_version=$(cd "$source_dir" && node -p 'require("./node_modules/tsx/package.json").version')
typescript_version=$(cd "$source_dir" && node -p 'require("./node_modules/typescript/package.json").version')
graphlib_version=$(cd "$source_dir" && node -p 'require("./node_modules/@dagrejs/graphlib/package.json").version')
[ "$esbuild_version" = 0.27.3 ] || fail "esbuild version is $esbuild_version, want 0.27.3"
[ "$tsx_version" = 4.21.0 ] || fail "tsx version is $tsx_version, want 4.21.0"
[ "$typescript_version" = 5.9.3 ] || fail "TypeScript version is $typescript_version, want 5.9.3"
[ "$graphlib_version" = 4.0.5 ] || fail "Graphlib version is $graphlib_version, want 4.0.5"

dist_path=$source_dir/dist/dagre.js
actual_dist_sha256=$(sha256_file "$dist_path")
[ "$actual_dist_sha256" = "$expected_dist_sha256" ] ||
	fail "patched dist/dagre.js SHA-256 is $actual_dist_sha256, want $expected_dist_sha256"

generated_path=$temp_dir/dagre-3.1.1-d2-compat.cjs
{
	cat "$source_dir/dist/dagre.js.LEGAL.txt"
	while IFS= read -r line || [ -n "$line" ]; do
		case "$line" in
			'/*! For license information please see dagre.js.LEGAL.txt */'|'//# sourceMappingURL=dagre.js.map') ;;
			*) printf '%s\n' "$line" ;;
		esac
	done < "$dist_path"
	printf 'module.exports = dagre;\n'
} > "$generated_path"

actual_oracle_sha256=$(sha256_file "$generated_path")
[ "$actual_oracle_sha256" = "$expected_oracle_sha256" ] ||
	fail "compatibility oracle SHA-256 is $actual_oracle_sha256, want $expected_oracle_sha256"

install -m 0644 "$generated_path" "$output_path"

printf 'Dagre commit: %s\n' "$actual_commit"
printf 'Node: %s\n' "$(node --version)"
printf 'npm: %s\n' "$(npm --version)"
printf 'esbuild: %s\n' "$esbuild_version"
printf 'tsx: %s\n' "$tsx_version"
printf 'TypeScript: %s\n' "$typescript_version"
printf 'Graphlib: %s\n' "$graphlib_version"
printf 'package-lock.json SHA-256: %s\n' "$actual_lock_sha256"
printf 'compatibility patch SHA-256: %s\n' "$actual_patch_sha256"
printf 'patched dist/dagre.js SHA-256: %s\n' "$actual_dist_sha256"
printf 'compatibility oracle SHA-256: %s\n' "$actual_oracle_sha256"
printf 'Wrote: %s\n' "$output_path"

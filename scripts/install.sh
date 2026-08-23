#!/usr/bin/env bash

set -euo pipefail

repo="waldemarsson/sluss"
ref="${SLUSS_REF:-main}"
install_dir="${SLUSS_INSTALL_DIR:-${HOME}/.local/bin}"
base_url="${SLUSS_BASE_URL:-https://raw.githubusercontent.com/${repo}/${ref}}"

fail() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"

tmp_file="$(mktemp)"
trap 'rm -f "$tmp_file"' EXIT

printf 'Downloading sluss from %s...\n' "$ref"
curl -fsSL "${base_url}/scripts/sluss" -o "$tmp_file"

# Reject HTML error pages and unrelated content before replacing an installation.
IFS= read -r first_line < "$tmp_file"
[[ "$first_line" == '#!/usr/bin/env bash' ]] || fail "downloaded file is not the sluss script"
bash -n "$tmp_file" || fail "downloaded sluss script has invalid syntax"

mkdir -p "$install_dir"
install -m 0755 "$tmp_file" "$install_dir/.sluss.tmp"
mv "$install_dir/.sluss.tmp" "$install_dir/sluss"

printf 'Installed sluss to %s/sluss\n' "$install_dir"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
	printf 'Add this directory to Fish PATH with:\n'
	printf '  fish_add_path %s\n' "$install_dir"
fi

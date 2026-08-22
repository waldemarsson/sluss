#!/usr/bin/env bash

set -euo pipefail

repo="waldemarsson/sluss"
version="${SLUSS_VERSION:-latest}"
install_dir="${SLUSS_INSTALL_DIR:-${HOME}/.local/bin}"

fail() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		fail "sha256sum or shasum is required"
	fi
}

if ! command -v curl >/dev/null 2>&1; then
	fail "curl is required"
fi
if ! command -v tar >/dev/null 2>&1; then
	fail "tar is required"
fi

case "$(uname -s)/$(uname -m)" in
	Darwin/arm64) platform="darwin_arm64" ;;
	Linux/x86_64) platform="linux_amd64" ;;
	*) fail "unsupported platform: $(uname -s)/$(uname -m)" ;;
esac

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

if [[ "$version" == "latest" ]]; then
	latest_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${repo}/releases/latest")"
	version="${latest_url##*/}"
	if [[ "$version" != v* ]]; then
		fail "could not determine the latest release version"
	fi
fi

base_url="https://github.com/${repo}/releases/download/${version}"
archive="sluss_${version}_${platform}.tar.gz"

printf 'Downloading sluss %s for %s...\n' "$version" "$platform"
curl -fsSL "${base_url}/${archive}" -o "$tmp_dir/sluss.tar.gz"
curl -fsSL "${base_url}/checksums.txt" -o "$tmp_dir/checksums.txt"

expected_checksum="$(awk -v archive="$archive" '$2 == archive { print $1 }' "$tmp_dir/checksums.txt")"
if [[ -z "$expected_checksum" ]]; then
	fail "release checksum not found for ${archive}"
fi

actual_checksum="$(sha256_file "$tmp_dir/sluss.tar.gz")"
if [[ "$actual_checksum" != "$expected_checksum" ]]; then
	fail "checksum mismatch for ${archive}"
fi

tar -xzf "$tmp_dir/sluss.tar.gz" -C "$tmp_dir"
mkdir -p "$install_dir"
install -m 0755 "$tmp_dir/sluss" "$install_dir/.sluss.tmp"
mv "$install_dir/.sluss.tmp" "$install_dir/sluss"

printf 'Installed sluss %s to %s/sluss\n' "$version" "$install_dir"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
	printf 'Add sluss to PATH for the current shell:\n'
	printf "  export PATH=\"%s:\$PATH\"\n" "$install_dir"
fi

if ! command -v sbx >/dev/null 2>&1; then
	printf 'Docker Sandboxes is also required: https://docs.docker.com/ai/sandboxes/install/\n'
fi

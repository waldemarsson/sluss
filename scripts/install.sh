#!/usr/bin/env bash

set -euo pipefail

repo="waldemarsson/sluss"
version="${SLUSS_VERSION:-main}"
install_dir="${SLUSS_INSTALL_DIR:-${HOME}/.local/bin}"

fail() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

if ! command -v curl >/dev/null 2>&1; then
	fail "curl is required"
fi
if ! command -v go >/dev/null 2>&1; then
	fail "Go 1.23 or later is required to install sluss from source"
fi

go_version="$(go env GOVERSION)"
go_version="${go_version#go}"
go_major="${go_version%%.*}"
go_minor="${go_version#*.}"
go_minor="${go_minor%%.*}"
if ((go_major < 1 || (go_major == 1 && go_minor < 23))); then
	fail "Go 1.23 or later is required (found $(go env GOVERSION))"
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

archive_url="https://github.com/${repo}/archive/${version}.tar.gz"
printf 'Downloading sluss %s...\n' "$version"
curl -fsSL "$archive_url" | tar -xz -C "$tmp_dir" --strip-components=1

mkdir -p "$install_dir"
(
	cd "$tmp_dir"
	CGO_ENABLED=0 go build \
		-ldflags "-X main.version=${version}" \
		-o "${install_dir}/sluss" \
		./cmd/sluss
)

printf 'Installed sluss %s to %s/sluss\n' "$version" "$install_dir"
if [[ ":${PATH}:" != *":${install_dir}:"* ]]; then
	printf 'Add %s to PATH to run sluss without its full path.\n' "$install_dir"
fi

if ! command -v sbx >/dev/null 2>&1; then
	printf 'Docker Sandboxes is also required: https://docs.docker.com/ai/sandboxes/install/\n'
fi

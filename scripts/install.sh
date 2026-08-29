#!/bin/sh
# Install or upgrade sluss from a GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/install.sh | sh
#
# POSIX sh on purpose: this is piped into whatever shell the reader has.

set -eu

repo="waldemarsson/sluss"
version="${SLUSS_VERSION:-latest}"
install_dir="${SLUSS_INSTALL_DIR:-${HOME}/.local/bin}"
base_url="${SLUSS_BASE_URL:-https://github.com/${repo}/releases}"

fail() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v tar >/dev/null 2>&1 || fail "tar is required"

# The download is checked before anything is replaced, so a checksum tool is a hard
# requirement rather than a nicety. Linux ships sha256sum, macOS ships shasum.
if command -v sha256sum >/dev/null 2>&1; then
	checksum() { sha256sum "$1" | cut -d ' ' -f 1; }
elif command -v shasum >/dev/null 2>&1; then
	checksum() { shasum -a 256 "$1" | cut -d ' ' -f 1; }
else
	fail "sha256sum or shasum is required to verify the download"
fi

os="$(uname -s)"
arch="$(uname -m)"
case "$os" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) fail "unsupported operating system: $os (sluss releases cover macOS and Linux)" ;;
esac
case "$arch" in
	arm64 | aarch64) arch=arm64 ;;
	x86_64 | amd64) arch=amd64 ;;
	*) fail "unsupported architecture: $arch (sluss releases cover arm64 and amd64)" ;;
esac

# Asset names carry no version, so "latest/download/<name>" resolves without asking
# the GitHub API and without inheriting its rate limit.
asset="sluss_${os}_${arch}.tar.gz"
if [ "$version" = latest ]; then
	download="${base_url}/latest/download"
else
	download="${base_url}/download/${version}"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

printf 'Downloading %s (%s)...\n' "$asset" "$version"
curl -fsSL "${download}/${asset}" -o "${tmp}/${asset}" ||
	fail "could not download ${download}/${asset}"
curl -fsSL "${download}/checksums.txt" -o "${tmp}/checksums.txt" ||
	fail "could not download ${download}/checksums.txt"

expected="$(awk -v name="$asset" '$2 == name || $2 == "*" name { print $1; exit }' "${tmp}/checksums.txt")"
[ -n "$expected" ] || fail "checksums.txt does not list ${asset}"
actual="$(checksum "${tmp}/${asset}")"
# A captive portal or proxy can answer 200 with HTML, which curl -f will not reject.
[ "$expected" = "$actual" ] ||
	fail "checksum mismatch for ${asset}; the download is not the released archive, so nothing was installed"

tar -xzf "${tmp}/${asset}" -C "$tmp" || fail "the download is not a valid archive"
[ -f "${tmp}/sluss" ] || fail "the archive contains no sluss binary"
chmod 0755 "${tmp}/sluss"
"${tmp}/sluss" version >/dev/null 2>&1 || fail "the downloaded sluss does not run on this machine"

mkdir -p "$install_dir"
# Copy first, then rename within the destination directory: rename is atomic there,
# so an interrupted install leaves either the old sluss or the new one.
cp "${tmp}/sluss" "${install_dir}/.sluss.new"
chmod 0755 "${install_dir}/.sluss.new"
mv "${install_dir}/.sluss.new" "${install_dir}/sluss"

printf 'Installed sluss to %s/sluss\n' "$install_dir"
case ":${PATH}:" in
	*":${install_dir}:"*) ;;
	*)
		printf '\n%s is not on your PATH. Add it, for example:\n' "$install_dir"
		# shellcheck disable=SC2016 # $PATH is printed for the reader's shell, not expanded here.
		printf '  bash/zsh:  echo '\''export PATH="%s:$PATH"'\'' >> ~/.profile\n' "$install_dir"
		printf '  fish:      fish_add_path %s\n' "$install_dir"
		;;
esac

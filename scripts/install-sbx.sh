#!/usr/bin/env bash

set -euo pipefail

fail() {
	printf 'error: %s\n' "$1" >&2
	exit 1
}

install_macos() {
	if [[ "$(uname -m)" != "arm64" ]]; then
		fail "Docker Sandboxes requires Apple silicon on macOS"
	fi

	local macos_version
	macos_version="$(sw_vers -productVersion)"
	local macos_major="${macos_version%%.*}"
	if ((macos_major < 14)); then
		fail "Docker Sandboxes requires macOS 14 or later (found ${macos_version})"
	fi

	if ! command -v brew >/dev/null 2>&1; then
		fail "Homebrew is required; install it from https://brew.sh"
	fi

	# Current Homebrew versions require explicit trust for third-party casks.
	if brew help trust >/dev/null 2>&1; then
		brew trust docker/tap
	fi
	brew install docker/tap/sbx
}

install_ubuntu() {
	# /etc/os-release is the canonical source of OS identity.
	# shellcheck disable=SC1091
	source /etc/os-release
	if [[ "${ID:-}" != "ubuntu" ]]; then
		fail "Docker supports sbx on Ubuntu, not Ubuntu derivatives (found ${ID:-unknown})"
	fi

	local ubuntu_major="${VERSION_ID%%.*}"
	if ((ubuntu_major < 24)); then
		fail "Docker Sandboxes requires Ubuntu 24.04 or later (found ${VERSION_ID})"
	fi

	case "$(uname -m)" in
		x86_64 | aarch64 | arm64) ;;
		*) fail "unsupported Linux architecture: $(uname -m)" ;;
	esac

	if [[ ! -e /dev/kvm ]]; then
		fail "/dev/kvm is missing; enable KVM or nested virtualization first"
	fi
	if ! command -v curl >/dev/null 2>&1; then
		fail "curl is required to configure Docker's apt repository"
	fi
	if ! command -v sudo >/dev/null 2>&1; then
		fail "sudo is required to install docker-sbx"
	fi

	curl -fsSL https://get.docker.com | sudo REPO_ONLY=1 sh
	sudo apt-get install -y docker-sbx

	local current_user
	current_user="$(id -un)"
	if [[ " $(id -nG "$current_user") " != *" kvm "* ]]; then
		sudo usermod -aG kvm "$current_user"
		printf '\nAdded %s to the kvm group. Sign out and back in before running sbx.\n' "$current_user"
	fi
}

if ((EUID == 0)); then
	fail "run this script as your normal user, not with sudo"
fi

if command -v sbx >/dev/null 2>&1; then
	printf 'sbx is already installed: '
	sbx version
else
	case "$(uname -s)" in
		Darwin) install_macos ;;
		Linux) install_ubuntu ;;
		*) fail "unsupported operating system: $(uname -s)" ;;
	esac
	sbx version
fi

cat <<'EOF'

Docker Sandboxes is installed.

Next:
  sbx login
  sbx daemon start --detach
  sbx policy init balanced
  sbx diagnose

The login opens Docker's device authorization flow. Policy initialization is a
security decision, so this script does not make it automatically.
EOF

#!/bin/sh
# Remove an installed sluss binary.
#
#   curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/main/scripts/uninstall.sh | sh
#
# This removes the binary and nothing else: configuration, worktrees, branches and
# sandboxes all outlive it, and are listed rather than deleted.

set -eu

install_dir="${SLUSS_INSTALL_DIR:-${HOME}/.local/bin}"
target="${install_dir}/sluss"

if [ ! -e "$target" ]; then
	# Not where we install, but possibly somewhere else on PATH — say where, rather
	# than claiming nothing is installed.
	found="$(command -v sluss 2>/dev/null || true)"
	if [ -n "$found" ]; then
		printf 'sluss is not installed in %s, but %s is on your PATH.\n' "$install_dir" "$found"
		printf 'Remove that one with:\n  rm %s\n' "$found"
		printf 'Or re-run with: SLUSS_INSTALL_DIR=%s\n' "$(dirname "$found")"
		exit 0
	fi
	printf 'sluss is not installed in %s; nothing to do.\n' "$install_dir"
	exit 0
fi

rm -f "$target"
printf 'Removed %s\n' "$target"

config="${XDG_CONFIG_HOME:-${HOME}/.config}/sluss/config.json"
printf '\nLeft in place, on purpose:\n'
if [ -f "$config" ]; then
	printf '  configuration  %s\n' "$config"
fi
printf '  worktrees      whatever worktreeRoot points at; remove them with "git worktree remove"\n'
printf '  sandboxes      still known to sbx; list them with "sbx --app-name SCOPE ls"\n'

# .devcontainer/Dockerfile
# Ubuntu 24.04 Noble — Go, Node.js, Claude Code, and Copilot CLI via devcontainer features.
#
# Note: sbx is NOT installed here and cannot be. It needs hardware virtualisation and its
# own host daemon, so every sbx-touching command (docs/SPIKE.md, `sluss doctor`, and any
# `sluss env` run against a real sandbox) happens on the host, not in this container.
# This container is for writing and building the Go code.
FROM mcr.microsoft.com/devcontainers/base:noble

# ── Agent tooling ────────────────────────────────────────────────────────────
# Fast CLI utilities used by AI coding agents and dev workflows.
#   ripgrep : fast recursive search (rg)
#   fd-find : fast file finder (exposed as `fd`)
#   bat     : syntax-highlighting cat (exposed as `bat`)
#   jq      : structured-data (JSON) query
#   tree    : directory tree view
#   gh      : GitHub CLI (PR/issue creation), pulled straight from Ubuntu noble's
#             universe repo — no need for GitHub's own apt repo/keyring on this release.
# Note: on Debian/Ubuntu apt installs fd as `fdfind` and bat as `batcat` —
# the symlinks below expose them under their canonical names.
# ast-grep (npm) is installed in postCreate.sh, after the Node feature is on PATH.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ripgrep \
        fd-find \
        bat \
        jq \
        tree \
        gh \
    && ln -s "$(which fdfind)" /usr/local/bin/fd \
    && ln -s "$(which batcat)" /usr/local/bin/bat \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# ── Playwright / Chromium runtime libraries ──────────────────────────────────
# The dashboard's browser tests drive a real headless Chromium. The browser *binary* is a
# per-user download handled in postCreate.sh, but its shared libraries are system packages
# needing root — only possible here, at build time: postCreate.sh drops the vscode NOPASSWD
# sudo rule as its last step, so nothing can apt-install once the container is running.
# Without these, a test run fails at browser launch rather than on an assertion.
#
# This is the Ubuntu 24.04 (noble) set that `playwright install-deps` resolves to; the `t64`
# suffixes are noble's 64-bit-time_t package renames and are not interchangeable with the
# jammy names. To refresh after a Playwright major upgrade, run
# `npx playwright install-deps --dry-run chromium` and reconcile.
# Kept in step with the homehub and babytabs devcontainers.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        libasound2t64 \
        libatk-bridge2.0-0t64 \
        libatk1.0-0t64 \
        libatspi2.0-0t64 \
        libcairo2 \
        libcups2t64 \
        libdbus-1-3 \
        libgbm1 \
        libglib2.0-0t64 \
        libnspr4 \
        libnss3 \
        libpango-1.0-0 \
        libx11-6 \
        libxcb1 \
        libxcomposite1 \
        libxdamage1 \
        libxext6 \
        libxfixes3 \
        libxkbcommon0 \
        libxrandr2 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# ── micro editor (external prompt editing via Ctrl+G) ────────────────────────
# Pinned release, downloaded directly from GitHub and SHA256-verified — no `curl | bash`,
# so the build fails loudly on a tampered or failed download instead of running remote code.
ARG MICRO_VERSION=2.0.15
# Checksums come from the release's published .tar.gz.sha files on GitHub.
ARG MICRO_SHA256_AMD64=dfa1b6ae53e4e0b063b54224fd2b6b0a3c3159ea09d042a3a8f5cd001844d44c
ARG MICRO_SHA256_ARM64=5ca127857bf5500be3879f1a70b27556e737a49da04a1be5334de9e8e8781ad9
RUN set -eux; \
    # 1. Pick the release asset + its checksum for this image's architecture.
    arch="$(dpkg --print-architecture)"; \
    case "${arch}" in \
        amd64) asset="linux64";     sha256="${MICRO_SHA256_AMD64}" ;; \
        arm64) asset="linux-arm64"; sha256="${MICRO_SHA256_ARM64}" ;; \
        *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;; \
    esac; \
    # 2. Download the pinned tarball.
    url="https://github.com/micro-editor/micro/releases/download/v${MICRO_VERSION}/micro-${MICRO_VERSION}-${asset}.tar.gz"; \
    curl -fsSL "${url}" -o /tmp/micro.tar.gz; \
    # 3. Verify its integrity before doing anything with it.
    echo "${sha256}  /tmp/micro.tar.gz" | sha256sum -c -; \
    # 4. Install the binary and clean up.
    tar -xzf /tmp/micro.tar.gz -C /tmp; \
    mv "/tmp/micro-${MICRO_VERSION}/micro" /usr/local/bin/micro; \
    rm -rf /tmp/micro.tar.gz "/tmp/micro-${MICRO_VERSION}"; \
    micro -version

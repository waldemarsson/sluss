#!/usr/bin/env bash
# .devcontainer/postCreate.sh
set -euo pipefail

echo ""
echo "🔧 Running post-create setup..."
echo ""

# ── Prepare Copilot bind-mount directories ────────────────────────────────────
for dir in "$HOME/.copilot/skills" "$HOME/.copilot/agents" "$HOME/.copilot/extensions" "$HOME/.copilot/workflow" \
    "$HOME/.copilot/installed-plugins" "$HOME/.copilot/plugin-data" "$HOME/.copilot/instructions" \
    "$HOME/.copilot/prompts" "$HOME/.copilot/commands" "$HOME/.copilot/hooks" \
    "$HOME/.agents" "$HOME/.agents/skills"; do
    mkdir -p "$dir"
done
for dir in "$HOME/.copilot/skills" "$HOME/.copilot/agents" "$HOME/.copilot/extensions" "$HOME/.copilot/workflow" \
    "$HOME/.copilot/installed-plugins" "$HOME/.copilot/plugin-data" "$HOME/.copilot/instructions" \
    "$HOME/.copilot/prompts" "$HOME/.copilot/commands" "$HOME/.copilot/hooks" "$HOME/.agents/skills"; do
    if [ ! -w "$dir" ]; then
        echo "⚠️  $dir is not writable by $(whoami); fix host ownership instead of changing from inside the container"
    fi
done
echo "✅ Copilot directories ready"
# (No Claude directory prep needed — ~/.claude is bind-mounted as a whole.)

# ── Set yolo aliases for Claude and CoPilot ───────────────────────────────────

grep -q 'yolopilot' "$HOME/.bashrc" 2>/dev/null \
    || echo "alias yolopilot='copilot --yolo --experimental'" >> "$HOME/.bashrc"


grep -q 'yoloclaude' "$HOME/.bashrc" 2>/dev/null \
    || echo "alias yoloclaude='claude --dangerously-skip-permissions'" >> "$HOME/.bashrc"


# ── Fix volume ownership ──────────────────────────────────────────────────────
# .claude and .copilot's bind-mounted subdirs (skills, agents, etc.) are direct host
# binds — never chown -R paths containing those, it would rewrite ownership on your
# actual host files. Only fix the .copilot directory entry itself (Docker may have
# auto-created it as root before the container started) and the true named volumes.
echo "🔑 Fixing volume ownership..."
sudo mkdir -p \
    "$HOME/go/pkg/mod" \
    "$HOME/.cache/go-build" \
    "$HOME/.cache/ms-playwright" \
    "$HOME/.npm" \
    "$HOME/.copilot"
sudo chown vscode:vscode "$HOME/.copilot"
sudo chown -R vscode:vscode \
    "$HOME/go" \
    "$HOME/.cache/go-build" \
    "$HOME/.cache/ms-playwright" \
    "$HOME/.npm" \
    2>/dev/null || true
echo "   ✅ Done"

# ── Git credentials via PAT (no SSH key mounted) ──────────────────────────────
# GH_TOKEN (containerEnv, see devcontainer.json) is a fine-grained token scoped to just this
# repo, also picked up directly by the gh CLI. The helper reads it from the env at push/fetch
# time — never written to disk.
echo "🔐 Configuring git credential helper..."
git config --global credential.helper '!f() { echo username=x-access-token; echo password="$GH_TOKEN"; }; f'
echo "   ✅ Done"

# ── Migrate Claude Code to native installer ───────────────────────────────────
echo "🤖 Installing Claude Code (native)..."
claude install 2>/dev/null || true
echo "   ✅ Done"

# ── ast-grep (npm) ─────────────────────────────────────────────────────────────
# Installed here rather than the Dockerfile since it needs Node on PATH,
# which is only guaranteed once the Node devcontainer feature has run.
echo "🌳 Installing ast-grep..."
npm install -g @ast-grep/cli
echo "   ✅ Done"

# ── go-task (Taskfile runner) ─────────────────────────────────────────────────
# Installed with `go install` rather than a pinned tarball in the Dockerfile: Go is only
# on PATH once its devcontainer feature has run, and this avoids carrying release
# checksums that would need bumping on every go-task upgrade.
echo "🧰 Installing go-task..."
go install github.com/go-task/task/v3/cmd/task@latest
echo "   ✅ Done"

# ── Go modules ────────────────────────────────────────────────────────────────
# Resolve the workspace from this script so the host's checkout path does not matter.
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
echo "📦 Downloading Go modules..."
if [ -f "$repo_root/go.mod" ]; then
    (cd "$repo_root" && go mod download)
    echo "   ✅ Done"
else
    echo "   ⚠️  No go.mod yet, skipping"
fi

# ── Dashboard dependencies (M4) ───────────────────────────────────────────────
# The Svelte dashboard does not exist until milestone M4; skip quietly until it does.
if [ -f "$repo_root/web/dashboard/package.json" ]; then
    echo "📦 Installing dashboard dependencies..."
    (cd "$repo_root/web/dashboard" && npm ci)
    echo "   ✅ Done"

    # ── Playwright browser ──────────────────────────────────────────────────────
    # Chromium only. Just the browser binary here: its system libraries are baked into
    # the image (see .devcontainer/Dockerfile), so this needs no root and cannot strand
    # the NOPASSWD-sudo removal below. Lands in the ms-playwright volume, so a rebuild
    # does not re-download it. Same split as homehub and babytabs.
    echo "🎭 Installing Playwright Chromium..."
    (cd "$repo_root/web/dashboard" && npx playwright install chromium)
    echo "   ✅ Done"
fi

# ── Drop NOPASSWD sudo rule ───────────────────────────────────────────────────
# All privileged setup is complete. Removing this means AI agents in auto-approve
# mode cannot escalate to root.
echo "🔒 Hardening: removing NOPASSWD sudo rule..."
sudo rm -f /etc/sudoers.d/vscode
echo "   ✅ sudo disabled for vscode user"

# ── Verification ─────────────────────────────────────────────────────────────
echo ""
echo "══════════════════════════════════════════════════════════"
echo "  Environment Verification"
echo "══════════════════════════════════════════════════════════"
echo "  Go             : $(go version 2>/dev/null || echo 'NOT FOUND')"
echo "  go-task        : $(task --version 2>/dev/null || echo 'NOT FOUND')"
echo "  Node.js        : $(node --version 2>/dev/null || echo 'NOT FOUND')"
echo "  npm            : $(npm --version 2>/dev/null || echo 'NOT FOUND')"
echo "  Claude Code    : $(claude --version 2>/dev/null || echo 'NOT FOUND')"
echo "  Copilot CLI    : $(copilot --version 2>/dev/null || echo 'run: copilot')"
echo "  ripgrep        : $(rg --version 2>/dev/null | head -n1 || echo 'NOT FOUND')"
echo "  fd             : $(fd --version 2>/dev/null || echo 'NOT FOUND')"
echo "  bat            : $(bat --version 2>/dev/null || echo 'NOT FOUND')"
echo "  jq             : $(jq --version 2>/dev/null || echo 'NOT FOUND')"
echo "  tree           : $(tree --version 2>/dev/null | head -n1 || echo 'NOT FOUND')"
echo "  micro          : $(micro -version 2>/dev/null | head -n1 || echo 'NOT FOUND')"
echo "  ast-grep       : $(sg --version 2>/dev/null || echo 'NOT FOUND')"
echo "  Chromium       : $(ls -d "$HOME/.cache/ms-playwright"/chromium-* 2>/dev/null | head -n1 | xargs -r basename || echo 'NOT FOUND')"
echo "  sbx            : host-only (needs hardware virt) — see docs/SPIKE.md"
echo "  Copilot skills : $(ls "$HOME/.copilot/skills" 2>/dev/null | wc -l) file(s)"
echo "  Copilot agents : $(ls "$HOME/.copilot/agents" 2>/dev/null | wc -l) file(s)"
echo "  Copilot exts   : $(ls "$HOME/.copilot/extensions" 2>/dev/null | wc -l) file(s)"
echo "  Copilot wkflow : $(ls "$HOME/.copilot/workflow" 2>/dev/null | wc -l) file(s)"
echo "  Copilot plugins: $(ls "$HOME/.copilot/installed-plugins" 2>/dev/null | wc -l) file(s)"
echo "  Claude agents  : $(ls "$HOME/.claude/agents" 2>/dev/null | wc -l) file(s)"
echo "  Claude commands: $(ls "$HOME/.claude/commands" 2>/dev/null | wc -l) file(s)"
echo "  Claude plugins : $(ls "$HOME/.claude/plugins/marketplaces" 2>/dev/null | wc -l) marketplace(s)"
echo "══════════════════════════════════════════════════════════"
echo ""
echo "📋 Quick Start"
echo "  Build:         task build"
echo "  Check:         task check       # gofmt + go vet + go test"
echo "  Dashboard:     task test:web"
echo "  Cross-compile: task build:linux # NAS target (M5)"
echo "  Claude Code:   claude"
echo "  Copilot CLI:   copilot auth login"
echo ""
echo "⚠️  sbx does not run in this container (hardware virtualisation)."
echo "   Run docs/SPIKE.md and any real 'sluss env' command on the host Mac."
echo ""

# Phase 0 — Verify before writing code

Seven assumptions. Each is under an hour. Two are load-bearing: **1 and 3**. Do those first.

If 1 or 3 comes back false, stop and revise `SPEC.md` before writing anything.

Record answers inline in this file as you go.

---

## Setup

Tested with `sbx v0.39.0` on Ubuntu 24.04 x86_64 with nested KVM.

```bash
# Install sbx by following https://docs.docker.com/ai/sandboxes/install/
sbx version
sbx login
sbx setup
sbx create opencode --name spike ~/src/mercurius
```

Use a scoped daemon so experiments don't pollute your real state:

```bash
export APP=sluss-spike
sbx --app-name $APP login
sbx --app-name $APP policy init balanced
# clean up any time with:
sbx --app-name $APP reset --force
```

---

## 1. Is the workspace mount bidirectional? ⚠️ LOAD-BEARING

**Why it matters:** the entire worktree model in SPEC.md §6 depends on the host seeing sandbox changes live.

```bash
sbx exec spike -- sh -c 'echo hello > /home/agent/workspace/SPIKE_TEST'
ls ~/src/mercurius/SPIKE_TEST          # does it exist on the host?
echo world >> ~/src/mercurius/SPIKE_TEST
sbx exec spike -- cat /home/agent/workspace/SPIKE_TEST   # does the sandbox see it?
```

Also check the exact workspace path inside the sandbox — docs suggest `/home/agent/workspace`, verify.

**If false:** worktrees become pointless. Switch to `--clone` plus an explicit sync step (`sbx cp`, or push to a branch). Rewrite §6.

**Answer:**

**True with an additional mount.** Plain workspace files synchronized in both directions,
and the sandbox used the same workspace path as the host. Mounting only a linked worktree
failed because its `.git` file pointed to the main repository outside the mounted
workspace. Passing the main repository's `.git` directory as a second writable workspace
made `git status`, staging, and committing work, with the commit immediately visible on
the host. Two concurrent sandboxes also accepted the same `.git` mount while using
different worktrees and branches. `SPEC.md` section 6 must require this second mount;
it gives each sandbox access to all Git objects and refs in that repository.

---

## 2. Is `--kit` repeatable?

**Why it matters:** determines whether a profile kit can compose with a repo kit, or whether composition must happen entirely in `.sbxenv.yaml`.

```bash
sbx create --help | grep -A3 kit
sbx --app-name $APP create --kit ./kit-a --kit ./kit-b opencode /tmp/probe
```

**If false:** one kit per sandbox. Profile identity goes in the kit; repo specifics go in `.sbxenv.yaml`.

**Answer:**

---

## 3. Is SSE through a reverse proxy acceptable? ⚠️ LOAD-BEARING

**Why it matters:** the browser-chat premise is the main reason this project exists. There are public reports of OpenCode's web UI feeling much slower than the TUI behind a proxy.

Minimal test — ~30 lines of Go, throwaway:

```go
package main

import (
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	target, _ := url.Parse("http://127.0.0.1:14096")
	p := &httputil.ReverseProxy{
		Rewrite:       func(r *httputil.ProxyRequest) { r.SetURL(target) },
		FlushInterval: -1,
	}
	http.ListenAndServe(":8420", p)
}
```

Then compare, on the same prompt:
- `sbx tui` — how fast do tokens appear?
- direct to the published port in a browser
- through the proxy

**If the proxy is materially worse than direct:** something else is buffering. Check for gzip middleware, `Content-Length` being set, or a missing `X-Accel-Buffering: no`.

**If direct is already bad:** it's OpenCode's web UI, not your proxy. The premise fails. Fall back to `sbx tui` — at which point very little of sluss remains, and that is a legitimate outcome.

**Answer:**

---

## 4. How does `sbx ports` work?

```bash
sbx ports --help
sbx --app-name $APP ports spike --publish 14096:4096
curl -v http://127.0.0.1:14096
```

Need to confirm: can you choose the host port, or is one assigned? Is it per-sandbox persistent across stop/start?

**If ports are assigned rather than chosen:** sluss reads the assignment after creation instead of allocating, and the routing table becomes dynamic.

**Answer:**

With v0.39.0, the host port can be chosen using
`sbx ports <sandbox> --publish <host>:<sandbox>`. The fixed mapping was restored after
the sandbox stopped and started, although `sbx ports --json` returned no active mappings
while it was stopped.

---

## 5. Does `--app-name` actually isolate the secret store?

**Why it matters:** this is the difference between profiles being a real boundary and being a naming convention.

```bash
sbx --app-name profile-a login
sbx --app-name profile-a secret set TEST_TOKEN
sbx --app-name profile-b secret ls        # is TEST_TOKEN visible?
```

Also check whether sandbox names collide across app-names, and whether a second login is genuinely required.

**If leaky:** profiles degrade to convention. Still useful — the microVM does the heavy security lifting — but say so plainly in the docs rather than implying a guarantee.

**Answer:**

---

## 6. Does nested virtualization work in a TrueNAS SCALE VM?

Only blocks Phase 4. Check before planning it.

```bash
# on the SCALE host
grep -E 'vmx|svm' /proc/cpuinfo | head -1
# inside a test guest
grep -E 'vmx|svm' /proc/cpuinfo | head -1
ls -l /dev/kvm
```

Also confirm current sbx platform support — reports from April 2026 said macOS Apple Silicon, Windows 11 x86_64, or Linux Ubuntu 22.04+ with KVM. Verify that's still accurate.

**If false:** Mac-only. Drop Phase 4. Not a disaster.

**Answer:**

---

## 7. Does sbx accept JSON in a `.sbxenv.yaml` file?

**Why it matters:** if yes, sluss needs no YAML library at all — `encoding/json` writes the overlay.

```bash
cat > /tmp/probe/.sbxenv.yaml <<'EOF'
{"name": "probe", "workspace": {"path": "/tmp/probe"}}
EOF
sbx --app-name $APP env create /tmp/probe
```

Also test the deep-merge with mixed formats: a YAML base file and a JSON overlay, and confirm later-overrides-earlier holds.

**If false:** emit the overlay from a small text template. Thirty lines, still no YAML parser needed.

**Answer:**

---

## Cleanup

```bash
sbx --app-name $APP reset --force
sbx rm -f spike
rm -f ~/src/mercurius/SPIKE_TEST
```

---

## Decision

After answering all seven, write one paragraph here: build, revise, or abandon. Be honest — a "no" now costs an afternoon, a "no" in six weeks costs a project.

**Verdict:**

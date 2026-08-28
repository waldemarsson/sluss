# Phase 0 — Verify before writing code

Eight assumptions. Each is under an hour. Three are load-bearing: **1, 3 and 8**. Do those first.

If 1 or 3 comes back false, stop and revise `SPEC.md` before writing anything. Assumption 8 was
added after the GUI was built (D13) and decides whether the default access mode survives; a "no"
there is a spec revision too, not a workaround.

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

## Observed CLI surface

Recorded 2026-08-25 from a macOS host while building `scripts/sluss`. **The sbx version was
not captured**, so treat these as true of that host on that date and re-check before relying
on them. `SPIKE-RUNBOOK.md` asks for the real syntax to be written down; this is that.

`sbx --help` lists: `completion cp create daemon diagnose env exec help kit login logout ls
mcp policy ports prune reset rm run secret setup skills stop template tui version`. Its only
flags are `-D/--debug` and `-h/--help`.

Two commands work but are **absent from that list**: `sbx inspect SANDBOX_NAME [--json]`
("agent, kits, state, auth mode, workspace, network policy, secrets, published ports, active
sessions") and `sbx settings {get,list,set,unset}`. `--app-name` is likewise accepted but
undocumented there. Do not treat `sbx --help` as the full surface.

`sbx ls` is documented as listing "all sandboxes with their agent, status, published ports,
and workspace". The flag is `--json`; **`--format json` is rejected** as an unknown flag. The
shape, which `scripts/sluss` parses to recover a sandbox's agent:

```json
{ "sandboxes": [ { "name": "pergola", "id": "50fe619b-…", "agent": "opencode",
    "status": "running",
    "ports": [ { "host_ip": "127.0.0.1", "host_port": 49155,
                 "sandbox_port": 4096, "protocol": "tcp" } ],
    "workspaces": [ "/Users/marwal/src/worktrees/pergola/pergola",
                    "/Users/marwal/code/private/pergola/.git" ] } ] }
```

`agent` follows `name` within each object, and a stopped sandbox simply omits `ports` —
consistent with assumption 4's note that no mappings are reported while stopped. `sluss`
depends on both the key names and that ordering, so this block is load-bearing for
`scripts/sluss`, not just reference material.

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

**Partial — the flag is real; the secret store is still untested.** Probed on the macOS host
while building `scripts/sluss`, not as a deliberate run of this assumption, so treat it as
evidence rather than a verdict.

`--app-name` is accepted as a top-level flag (`sbx --app-name personal settings get …`
succeeded rather than erroring on an unknown flag) even though it appears **nowhere** in
`sbx --help`'s flag list — see the CLI surface notes above. Invoking a previously unused
app-name started a *separate* `sandboxd` daemon and ran the first-run setup/import banner
before answering, and `sbx settings --help` states the daemon owns settings — so at minimum
the daemon, and therefore settings, are per-scope.

Still to do for a real answer to this assumption: the `secret set` / `secret ls` cross-scope
check as written above, whether sandbox names collide across app-names, and whether a second
`login` is genuinely required.

**What now depends on it:** the secrets pane assumes `secret ls` prints names (one per line, first
field) and that `secret set NAME` reads the value from stdin — argv would expose it in `ps`.
`secret rm NAME` is assumed for deletion. All three assumptions live in `internal/sbx/sbx.go` and
nowhere else, so a wrong answer costs one file. sluss already keys everything by
`(scope, name)`, so a name collision across app-names cannot confuse it either way.

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

**True.** Verified 2026-08-28 against the real target — the `opencode` Incus QEMU VM on the
TrueNAS box, not a throwaway guest. AMD host, and nesting is on at the host's kernel module:

```
# TrueNAS host
$ sudo cat /sys/module/kvm_amd/parameters/nested
1

# inside the opencode VM
$ grep -oE 'vmx|svm' /proc/cpuinfo | head -1
svm
$ ls -l /dev/kvm
crw-rw---- 1 root kvm 10, 232 Aug 22 13:57 /dev/kvm
```

So the guest sees AMD-V and has a KVM device, which is what sbx needs on Linux. The NAS
deployment is not blocked by virtualization.

One thing this does **not** yet establish, and it is cheap: **`/dev/kvm` is `root:kvm` mode
0660.** Whichever user runs sbx must be in the `kvm` group, and the VM's `agent` user is not in
it by default — `homelab`'s `cloud-init.yaml` adds `agent` to `docker` and nothing else. Expect
the same class of failure as that file's documented gotchas: a permission error that reads like
a virtualization fault. Check with `groups agent`, fix with `usermod -aG kvm agent` and record
it in `cloud-init.yaml`.

VM sizing and how many sandboxes fit is an operator concern, deliberately not a sluss design
input — the fleet view assumes no maximum.

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

## 8. Does OpenCode Web work under a base path? ⚠️ LOAD-BEARING

**Why it matters:** D13 makes `path` mode the default — `https://<host>/s/<scope>/<name>/` — because
it needs no DNS, no wildcard certificate and no reverse proxy at all. D7 records OpenCode's web UI
as assuming root. If that assumption holds, the zero-infrastructure deployment does not work for
OpenCode and `host` mode becomes mandatory.

Run `slussd serve` with `access: path` against a real OpenCode sandbox, open
`http://127.0.0.1:8420/s/<scope>/<name>/` and check, in order:

- does `index.html` load at all, or does it 404 on its own assets?
- do the asset URLs resolve under the prefix, or are they absolute to `/`?
- does the SSE/token stream connect, or does it request `/event` at the root?

**If false:** `path` mode still serves everything else; OpenCode alone needs `host` mode. Say so in
`SPEC.md` §7, make `host` the documented default for OpenCode, and note that wildcard DNS returns
as a requirement — already satisfied on the NAS target, no longer free elsewhere.

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

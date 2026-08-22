# M0 spike — host runbook

Step-by-step for running `SPIKE.md`'s seven assumptions **on the Mac**, outside the
devcontainer. sbx needs hardware virtualisation and its own daemon, so none of this
works in the container (`AGENTS.md` § *Where things run*).

`SPIKE.md` is where the *answers* live. This file is how you get them.

> **Every `sbx` command below is unverified.** They're copied from `SPIKE.md` or inferred
> from Docker's docs; sbx is experimental and its CLI is explicitly allowed to change (D1).
> Run `--help` before each block and **record the real syntax you used** — the corrected
> commands are part of what the spike produces, not a detail.

**Rule from `SPIKE.md`: if assumption 1 or 3 comes back false, stop.** Don't run the rest,
don't write code — revise `SPEC.md` first.

---

## 0. Prerequisites

- [ ] Open **Terminal.app on macOS** (not the VS Code container terminal). Sanity check:
      `uname -s` prints `Darwin`. If it prints `Linux`, you're in the container.
- [ ] Docker Desktop is running.
- [ ] `cd /Users/marwal/code/private/sluss` — the repo is bind-mounted at the same path on
      both sides, so host and container see identical files. Logs you write here are
      readable from the container session.
- [ ] The spike proxy binary exists: `ls -l dist/spike-proxy`. If missing, run
      `task spike:proxy` **in the container** — it cross-compiles a static `darwin/arm64`
      binary, so the Mac needs no Go toolchain.

Everything below assumes you're in the repo root. Each block ends with a `tee` into
`spike/out/` (gitignored) so nothing has to be copy-pasted back by hand.

```bash
mkdir -p spike/out
export APP=sluss-spike     # scoped daemon — keeps experiments out of your real state
```

---

## 1. Install sbx

```bash
brew install docker/tap/sbx
sbx version
sbx --app-name $APP login
sbx --app-name $APP setup
```

- [ ] `sbx version` prints a version. **Write it into `SPIKE.md`** — every answer below is
      only true for this version.
- [ ] Note whether `--app-name` had to come before or after the subcommand; assumption 5
      depends on that placement being real.

```bash
{ sbx version; sbx --help; sbx create --help; sbx ports --help; } 2>&1 | tee spike/out/01-cli.log
```

---

## 2. Throwaway repo for the workspace tests

`SPIKE.md` uses `~/src/mercurius`. Use a scratch repo instead — assumption 1 writes a file
into the workspace, and a throwaway means cleanup can't touch real work.

```bash
mkdir -p ~/src/sluss-spike-repo && cd ~/src/sluss-spike-repo
git init -q && echo "# spike" > README.md && git add -A && git commit -qm init
cd /Users/marwal/code/private/sluss
```

- [ ] Repo exists at `~/src/sluss-spike-repo`.

---

## 3. Assumption 1 — is the workspace mount bidirectional? ⚠️ LOAD-BEARING

The whole worktree model in `SPEC.md` §6 rests on this.

```bash
sbx --app-name $APP create opencode --name spike ~/src/sluss-spike-repo 2>&1 | tee spike/out/03a-create.log
```

- [ ] Record the **actual** create syntax that worked (flag order, whether the path is
      positional, whether `--name` exists).

Find the real workspace path inside the sandbox before testing — the docs say
`/home/agent/workspace`, verify it:

```bash
sbx --app-name $APP exec spike -- sh -c 'pwd; ls -a; echo ---; ls -a /home/agent/workspace' 2>&1 | tee spike/out/03b-path.log
```

- [ ] Exact workspace path inside the sandbox: `________________`

Now both directions, sandbox → host first:

```bash
{
  sbx --app-name $APP exec spike -- sh -c 'echo hello > /home/agent/workspace/SPIKE_TEST'
  echo "--- host sees it? ---"
  cat ~/src/sluss-spike-repo/SPIKE_TEST
  echo "--- host writes ---"
  echo world >> ~/src/sluss-spike-repo/SPIKE_TEST
  echo "--- sandbox sees it? ---"
  sbx --app-name $APP exec spike -- cat /home/agent/workspace/SPIKE_TEST
} 2>&1 | tee spike/out/03c-mount.log
```

- [ ] Host shows `hello` → sandbox-to-host works.
- [ ] Sandbox shows `hello` *and* `world` → host-to-sandbox works.
- [ ] Bonus, since worktrees are the real use case: does the sandbox see a **git worktree**
      dir (a `.git` *file* pointing elsewhere), not just a plain directory? If `git status`
      inside the sandbox fails on a worktree, that's assumption 1 failing in the only shape
      that matters.

**Both directions true → continue. Either false → STOP.** Worktrees become pointless;
`SPEC.md` §6 needs rewriting around an explicit sync step.

**Answer → `SPIKE.md` § 1.**

---

## 4. Assumption 4 (partial) — get a port before you can proxy

`SPIKE.md` lists this fourth, but assumption 3 can't run without an address to point at.
Do the publish half now, the persistence half in step 6.

```bash
{ sbx --app-name $APP ports --help; sbx --app-name $APP ports publish spike 4096:14096; } 2>&1 | tee spike/out/04a-ports.log
curl -sv http://127.0.0.1:14096 2>&1 | head -20 | tee spike/out/04b-curl.log
```

- [ ] Host port sluss can actually reach: `http://127.0.0.1:________`
- [ ] Did **you choose** the host port, or was one **assigned**? `________________`
      (Assigned means the routing table becomes dynamic — `internal/proxy` reads after
      create instead of allocating. It's an open question in `ROADMAP.md`.)

---

## 5. Assumption 3 — is SSE through a reverse proxy acceptable? ⚠️ LOAD-BEARING

The browser-chat premise is the main reason this project exists.

**5a. Baseline the TUI first** — you can't judge "materially worse" without it.

```bash
sbx --app-name $APP tui spike
```

- [ ] Send a prompt long enough to stream for a few seconds. How do tokens appear —
      smooth, or clumped? `________________`

**5b. Direct in the browser.** Open `http://127.0.0.1:<port from step 4>`, same prompt.

- [ ] Direct feels: `________________`
- [ ] **If direct is already bad, stop here.** That's OpenCode's web UI, not the proxy —
      the premise fails and the fallback is `sbx tui`, at which point little of sluss
      remains. A legitimate outcome; record it honestly.

**5c. Through the proxy.**

```bash
./dist/spike-proxy -target http://127.0.0.1:<port> -listen 127.0.0.1:8420
```

Open `http://127.0.0.1:8420`, same prompt again.

- [ ] Proxied feels: `________________`
- [ ] Verify the response is genuinely streamed, not buffered:

```bash
curl -N -sv http://127.0.0.1:8420/<sse-endpoint> 2>&1 | head -40 | tee spike/out/05-sse.log
```

Look for `Content-Type: text/event-stream`, **no** `Content-Length`, and events arriving
over time rather than in one burst.

- [ ] If proxied is materially worse than direct, something is buffering: check for gzip
      middleware, a `Content-Length` on the streamed response, or a missing
      `X-Accel-Buffering: no`. `FlushInterval: -1` is already set in the spike proxy.

**Answer → `SPIKE.md` § 3.** This one deserves a sentence of prose, not a yes/no —
"acceptable" is a judgement call and future-you needs the reasoning.

---

## 6. The remaining four

Only worth running if 1 and 3 both passed.

### Assumption 2 — is `--kit` repeatable?

```bash
mkdir -p /tmp/probe /tmp/kit-a /tmp/kit-b
{ sbx create --help | grep -A3 -i kit
  sbx --app-name $APP create --kit /tmp/kit-a --kit /tmp/kit-b opencode /tmp/probe; } 2>&1 | tee spike/out/06-kit.log
```

- [ ] Two `--kit` flags accepted, or last-one-wins / error? `________________`
      (False → one kit per sandbox: profile identity in the kit, repo specifics in
      `.sbxenv.yaml`.)

### Assumption 4 (rest) — port persistence

```bash
{ sbx --app-name $APP stop spike; sbx --app-name $APP start spike; sbx --app-name $APP ports ls spike; } 2>&1 | tee spike/out/07-port-persist.log
```

- [ ] Same host port after stop/start? `________________`
      (No → the routing table must refresh on every sandbox start.)

### Assumption 5 — does `--app-name` isolate the secret store?

This is the difference between profiles being a real boundary and a naming convention.

```bash
{ sbx --app-name profile-a login
  sbx --app-name profile-a secret set TEST_TOKEN
  echo "--- can profile-b see it? ---"
  sbx --app-name profile-b secret ls; } 2>&1 | tee spike/out/08-appname.log
```

- [ ] Is `TEST_TOKEN` visible from `profile-b`? `________________`
- [ ] Do sandbox **names** collide across app-names? `________________`
- [ ] Was a second `login` genuinely required? `________________`
      (Leaky → profiles degrade to convention. Still useful, but the docs must say so
      plainly rather than implying a guarantee.)

### Assumption 7 — does sbx accept JSON in `.sbxenv.yaml`?

If yes, sluss needs no YAML library at all (D6).

```bash
cat > /tmp/probe/.sbxenv.yaml <<'EOF'
{"name": "probe", "workspace": {"path": "/tmp/probe"}}
EOF
sbx --app-name $APP env create /tmp/probe 2>&1 | tee spike/out/09-json.log
```

- [ ] JSON accepted? `________________`
- [ ] Deep-merge with mixed formats — YAML base + JSON overlay — does later-overrides-earlier
      hold? `________________`
      (False → emit the overlay from a ~30-line text template. Still no YAML parser.)

---

## 7. Assumption 6 — nested virtualisation on TrueNAS

**Skip unless M5 (Linux/NAS) is still wanted.** Runs on the SCALE host, not the Mac.

```bash
grep -E 'vmx|svm' /proc/cpuinfo | head -1   # on the SCALE host
grep -E 'vmx|svm' /proc/cpuinfo | head -1   # inside a test guest
ls -l /dev/kvm                               # inside the guest
```

- [ ] Also re-confirm sbx's current platform support. April 2026 reports said macOS Apple
      Silicon, Windows 11 x86_64, or Linux Ubuntu 22.04+ with KVM — verify that still holds.
- [ ] False → Mac-only, drop M5. Not a disaster.

---

## 8. Cleanup

```bash
sbx --app-name $APP reset --force
sbx --app-name profile-a reset --force
sbx --app-name profile-b reset --force
rm -rf ~/src/sluss-spike-repo /tmp/probe /tmp/kit-a /tmp/kit-b
```

- [ ] `sbx ls` (no `--app-name`) shows your real sandboxes untouched.

---

## 9. Write it up

- [ ] Every `**Answer:**` in `SPIKE.md` filled in — including the ones that were awkward.
- [ ] Corrected sbx command syntax recorded wherever it differed from this runbook.
- [ ] sbx version recorded at the top of `SPIKE.md`.
- [ ] The **Verdict** paragraph written: build, revise, or abandon. That paragraph is M0's
      only exit criterion (`ROADMAP.md`).
- [ ] Two `ROADMAP.md` open questions now have evidence: port allocation (step 4/6) and
      whether the routing table must be dynamic.
- [ ] Delete `spike/proxy/` and the `spike:proxy` task once the verdict is written — both
      are throwaway.

Back in the container, the raw logs are at `spike/out/*.log`. Point the agent at them and
it can draft the `SPIKE.md` answers from the actual output rather than your recollection.

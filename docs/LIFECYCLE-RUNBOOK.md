# Verifying the ported lifecycle against a real sbx

Step-by-step for checking that `internal/lifecycle` behaves against a **real** `sbx` the way
`scripts/sluss` did. Run this **on the Mac**, outside the devcontainer: sbx needs hardware
virtualisation and its own daemon, so none of it works in the container (`AGENTS.md`
§ *Where things run*).

**Why this exists.** The lifecycle port (D23) is covered by tests that stub sbx with a fake on
`PATH` and drive real git repositories. That proves sluss sends the argv it means to send. It
cannot prove sbx accepts that argv, because no test here is allowed to invoke a real sandbox.
Two `SPIKE.md` assumptions are still open, and this is the first run of the Go implementation
against the thing it delegates to.

> **If `sbx create` rejects sluss's argument order, stop and report it.** The order in
> `internal/sbx/lifecycle.go` was reproduced from the retired shell script deliberately, on the
> assumption that sbx cares. If sbx disagrees, that changes the spec — don't work around it in
> code (`AGENTS.md`).

---

## 0. Prerequisites

- [ ] **Terminal.app on macOS**, not the container terminal. Check: `uname -s` prints `Darwin`.
- [ ] `sbx` installed and logged in: `sbx version`, then `sbx --app-name personal ls`.
- [ ] A sluss binary. Either install the release candidate:
      ```sh
      curl -fsSL https://raw.githubusercontent.com/waldemarsson/sluss/single-binary-sluss/scripts/install.sh |
        SLUSS_VERSION=v0.2.0-rc1 sh
      ```
      or build one in the container with `task build` and use `./dist/sluss`.
- [ ] A throwaway git checkout to create sandboxes from, and a scratch worktree root:
      ```sh
      export SLUSS_APP_NAME=personal
      export SLUSS_WORKTREE_ROOT="$(mktemp -d)/worktrees"
      git init -q -b main /tmp/slusscheck && cd /tmp/slusscheck
      git commit -q --allow-empty -m seed
      ```

Record what actually happened at each step, including the real syntax if sbx wanted something
else. The corrections are the output of this runbook, not a detail.

---

## 1. Create — the argv, the mounts, the unwind

```sh
sluss start probe --cpus 2
```

- [ ] It printed `Created probe on agent/probe at … using opencode`.
- [ ] `sbx --app-name personal ls` shows `probe`.
- [ ] The **branch and worktree** exist: `git branch --list 'agent/*'`, `ls "$SLUSS_WORKTREE_ROOT"`.
- [ ] Port 4096 was auto-published: `sbx --app-name personal ports probe` shows a host mapping.
- [ ] **Both workspaces mounted**: inside the sandbox, the worktree is writable *and* git works —
      `sluss exec probe git status` succeeds. This is SPIKE assumption 1 and D4; if git fails
      inside the sandbox, the `.git` mount is wrong.

Then the unwind, which no stub can prove:

```sh
sluss start probe        # again — must resume, not recreate
sluss start bad-name-that-already-exists   # or force a create failure some other way
```

- [ ] The second `start probe` did **not** create a second sandbox — it ran `sbx exec probe true`.
- [ ] After a *failed* create, no orphan worktree or branch is left:
      `ls "$SLUSS_WORKTREE_ROOT"` and `git branch --list 'agent/*'`.

## 2. Attach — the part with the most sbx surface

```sh
sluss attach probe
```

- [ ] It printed `OpenCode Web port:` and a mapping, and the URL opens in a browser.
- [ ] Running it **again** does not start a second server (the PID file guard).
- [ ] With a claude sandbox: `sluss start c1 --agent claude && sluss attach c1` — check that
      `sbx settings set claude.remoteControl true` is accepted, and that the session appears at
      `claude.ai/code`.
- [ ] With a copilot sandbox: `sluss start g1 --agent copilot && sluss attach g1` — `--remote`
      accepted, session visible on GitHub.
- [ ] The agent is read back from sbx, not from the environment: `SLUSS_AGENT=opencode sluss attach c1`
      must still attach *claude*.

## 3. Destroy — the refusals, against real sandboxes

```sh
echo scratch > "$SLUSS_WORKTREE_ROOT/slusscheck/probe/notes.txt"
sluss destroy probe
```

- [ ] Refused: `agent/probe has uncommitted changes; commit them or use --force`, exit 1.
- [ ] **The sandbox still exists** — `sbx --app-name personal ls` still lists `probe`. The refusal
      must come before sbx is touched.
- [ ] Commit inside the worktree, then `sluss destroy probe` refuses again for unmerged commits,
      naming `main`.
- [ ] `sluss destroy probe --force` removes the sandbox, the worktree and the branch — and leaves
      every *other* sandbox alone.

## 4. Serve — the dashboard against a live fleet

```sh
sluss doctor
sluss serve
```

- [ ] `doctor` reports ok for sbx, each scope, git and the worktree root, and did **not** create
      the worktree root as a side effect.
- [ ] The fleet at `http://127.0.0.1:8420` shows the real sandboxes, their branches, and dirty /
      unmerged state.
- [ ] An opencode sandbox's UI loads through `http://127.0.0.1:8420/s/personal/<name>/`
      (**SPIKE assumption 8** — does OpenCode Web serve under a base path? If not, that is a spec
      revision, not a workaround).
- [ ] SSE feels live, not buffered (**SPIKE assumption 3**, the premise of the whole project).
- [ ] Create, start, stop and destroy from the browser behave exactly as the commands did, and a
      destroy refusal reaches the page unchanged.

## 5. Cleanup

```sh
sluss destroy probe --force 2>/dev/null
sbx --app-name personal ls          # nothing of this run left
rm -rf /tmp/slusscheck "$SLUSS_WORKTREE_ROOT"
```

## 6. Write it up

Record the answers in `docs/SPIKE.md` for assumptions 3 and 8, and anything sbx wanted spelled
differently in `docs/DECISIONS.md`. If sbx's actual argv differs from what
`internal/sbx/lifecycle.go` builds, that is a finding about the port, not a bug to patch quietly.

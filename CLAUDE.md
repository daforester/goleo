@AGENTS.md

## Claude Code

### SPIKES.md is NOT auto-loaded — read it before touching these paths

`SPIKES.md` is the durable record of the de-risking spikes behind the current
architecture, and it is deliberately **not** imported here: at ~83k characters it cost
~20k tokens of every session's context to carry a log most sessions never consult.

**Read it first — do not re-derive its findings — before changing any of:**

- the **desktop webview** (`runtime/webview_glaze*.go`, the `crgimenes/glaze` dependency,
  the `daforester/glaze` fork, anything cgo-related). The cgo-free thesis, the permission
  auto-grant, and why the fork exists are all evidenced there.
- **native IPC**, **scheme assets** (`goleo://`), **multi-window**, **tray**, or the
  **native menu bar** — each has hardware-verified findings and known failure modes.
- the **mobile build path** (`gomobile bind`, vendoring, the Android/iOS shells). In
  particular the gomobile **naming rule** for iOS is not guessable from the source.
- **store packaging or submission** — see also `docs/store-submission.md`, which is the
  cold-start handoff for the Play/Apple/Microsoft account work.

`AGENTS.md` (imported above) carries the *conclusions* those spikes reached; `SPIKES.md`
carries the *evidence* and the failure modes. If you are about to conclude that something
in the architecture is unnecessary or could be simpler, the reason it is that way is very
likely recorded there.

### The rest of the map

`AGENTS.md` is deliberately kept small enough to load every session. The detail behind it lives
in sub-docs it links to, each with a "read this before touching X" trigger:

| Doc | Read before changing |
|---|---|
| `docs/agents/webview.md` | `runtime/webview*.go`, the glaze dependency/fork, window modes, multi-window |
| `docs/agents/host-features.md` | a `runtime/<feature>/` package, a mobile provider, generated entry points |
| `docs/agents/external-binaries.md` | anything that shells out to a third-party binary, or a feature you assume works on Linux |
| `docs/agents/desktop-subsystems.md` | windowing/lifecycle, native IPC, scheme assets, OS integration, bundling |
| `docs/store-submission.md` | anything touching the Play / Apple / Microsoft developer accounts |
| `SPIKES.md` | see above — the evidence behind the architecture |
| `docs/history.md` | nothing; it is dated background, not guidance |

**`AGENTS.md` carries the invariants; the sub-docs carry the detail.** If you are about to
conclude that some piece of the architecture is unnecessary, check the relevant sub-doc first —
the reason it is that way is usually recorded, and several of these constraints exist because the
obvious simpler approach was tried and failed on real hardware.

### Keep the docs in sync

Keep the project docs in sync with the code as architecture and behavior change —
not only AGENTS.md and SPIKES.md, but the human-facing docs that drift with them:
README.md, docs/comparison.md, docs/roadmap.md, and the docs/guide/ pages.

When a change lands — a dependency swap, a backend migration, a removed build flag
or dependency, a version bump — grep the whole doc set for the old state and update
**every** mention, not just the two files named above. (The cgo-free / webview_go
churn left stale claims in README, comparison, roadmap, and the guide precisely
because only AGENTS/SPIKES were being maintained.)

SPIKES.md, docs/history.md and docs/roadmap.md are **dated logs**: append new findings
and mark superseded entries as history (with a brief currency note) rather than rewriting
them. The other docs describe the *current* state — those should read as present tense
with no stale claims.

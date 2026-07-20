@AGENTS.md

@SPIKES.md

## Claude Code

Keep the project docs in sync with the code as architecture and behavior change —
not only AGENTS.md and SPIKES.md (the primary AI-context files, imported above),
but the human-facing docs that drift with them: README.md, docs/comparison.md,
docs/roadmap.md, and the docs/guide/ pages.

When a change lands — a dependency swap, a backend migration, a removed build flag
or dependency, a version bump — grep the whole doc set for the old state and update
**every** mention, not just the two files named above. (This session's cgo-free /
webview_go churn left stale claims in README, comparison, roadmap, and the guide
precisely because only AGENTS/SPIKES were being maintained.)

SPIKES.md and docs/roadmap.md are **dated logs**: append new findings and mark
superseded entries as history (with a brief currency note) rather than rewriting
them. The other docs describe the *current* state — those should read as present
tense with no stale claims.
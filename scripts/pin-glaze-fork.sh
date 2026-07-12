#!/usr/bin/env bash
# Pin the glaze webview backend to a fork you control — defense-in-depth for a
# pre-1.0, single-maintainer dependency (insulates against upstream deletion and
# lets you patch it). The version is already pinned + hashed in go.sum, so this
# is optional; use it if you want to own the source.
#
# One-time manual step first: fork github.com/crgimenes/glaze on GitHub (the
# fork copies its tags, incl. v0.0.31). Then:
#   scripts/pin-glaze-fork.sh github.com/<you>/glaze            # defaults to v0.0.31
#   scripts/pin-glaze-fork.sh github.com/<you>/glaze v0.0.31
#
# Repoints both the root module and the vendored cli/npm/goleo copy. Review the
# go.mod/go.sum changes and commit. Undo: `go mod edit -dropreplace github.com/crgimenes/glaze`.
set -euo pipefail
FORK="${1:?usage: pin-glaze-fork.sh <fork-module-path> [version]}"
VER="${2:-v0.0.31}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
for mod in "$ROOT" "$ROOT/cli/npm/goleo"; do
  ( cd "$mod" && go mod edit -replace "github.com/crgimenes/glaze=${FORK}@${VER}" && go mod tidy && go mod vendor )
done
echo "Pinned glaze -> ${FORK}@${VER} in root + cli/npm/goleo. Review go.mod/go.sum and commit."

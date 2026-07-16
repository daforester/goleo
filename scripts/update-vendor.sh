#!/usr/bin/env bash
# Refresh the committed vendor/ tree for the root module. Third-party deps are
# vendored so builds never break if an upstream repo disappears (glaze in
# particular is pre-1.0 / single-maintainer). The cli/npm/goleo bundle is a
# generated copy of the root (cli/npm/copy-source.js), so it needs no separate
# vendoring — it inherits this vendor/ at build/publish time.
#
# Usage:
#   scripts/update-vendor.sh                          # re-vendor current go.mod
#   scripts/update-vendor.sh github.com/crgimenes/glaze@v0.0.32   # bump one dep, then re-vendor
#   scripts/update-vendor.sh -u ./...                 # update all deps to latest, then re-vendor
#
# Any arguments are passed to `go get`.
# After it finishes, review `git status -- vendor go.mod go.sum` and commit.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODS=("$ROOT")

if [ "$#" -gt 0 ]; then
  echo ">> go get $* (in each module that requires the dependency)"
  for mod in "${MODS[@]}"; do
    if ( cd "$mod" && go get "$@" ); then
      echo "   updated: $mod"
    else
      echo "   skipped: $mod (dependency not required there, or go get failed)"
    fi
  done
fi

for mod in "${MODS[@]}"; do
  echo ">> refreshing vendor: $mod"
  ( cd "$mod" && go mod tidy && go mod vendor && go mod verify ) || { echo "FAILED in $mod"; exit 1; }
done

echo "Done. Review 'git status -- vendor go.mod go.sum' and commit."

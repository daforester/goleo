#!/usr/bin/env bash
# Refresh the committed vendor/ trees for both product modules (root +
# cli/npm/goleo). Third-party deps are vendored so builds never break if an
# upstream repo disappears (glaze in particular is pre-1.0 / single-maintainer).
#
# Usage:
#   scripts/update-vendor.sh                          # re-vendor current go.mod (both modules)
#   scripts/update-vendor.sh github.com/crgimenes/glaze@v0.0.32   # bump one dep, then re-vendor
#   scripts/update-vendor.sh -u ./...                 # update all deps to latest, then re-vendor
#
# Any arguments are passed to `go get` in each module that has the dependency.
# After it finishes, review `git status -- vendor cli/npm/goleo/vendor go.mod go.sum`
# and commit.
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MODS=("$ROOT" "$ROOT/cli/npm/goleo")

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

echo "Done. Review 'git status -- vendor cli/npm/goleo/vendor go.mod go.sum' and commit."

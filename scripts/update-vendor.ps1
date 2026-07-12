#!/usr/bin/env pwsh
# Refresh the committed vendor/ trees for both product modules (root +
# cli/npm/goleo). Third-party deps are vendored so builds never break if an
# upstream repo disappears (glaze in particular is pre-1.0 / single-maintainer).
#
# Usage:
#   scripts/update-vendor.ps1                                       # re-vendor current go.mod
#   scripts/update-vendor.ps1 github.com/crgimenes/glaze@v0.0.32    # bump one dep, then re-vendor
#   scripts/update-vendor.ps1 -u ./...                              # update all deps, then re-vendor
#
# Any arguments are passed to `go get` in each module that has the dependency.
# Afterwards review `git status -- vendor cli/npm/goleo/vendor go.mod go.sum` and commit.
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$mods = @($root, (Join-Path $root "cli/npm/goleo"))

if ($args.Count -gt 0) {
  Write-Host ">> go get $($args -join ' ') (in each module that requires the dependency)"
  foreach ($mod in $mods) {
    Push-Location $mod
    try { & go get @args; Write-Host "   updated: $mod" }
    catch { Write-Host "   skipped: $mod (dependency not required there, or go get failed)" }
    finally { Pop-Location }
  }
}

foreach ($mod in $mods) {
  Write-Host ">> refreshing vendor: $mod"
  Push-Location $mod
  try { & go mod tidy; & go mod vendor; & go mod verify }
  finally { Pop-Location }
}

Write-Host "Done. Review 'git status -- vendor cli/npm/goleo/vendor go.mod go.sum' and commit." -ForegroundColor Green

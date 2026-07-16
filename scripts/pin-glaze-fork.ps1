#!/usr/bin/env pwsh
# Pin the glaze webview backend to a fork you control - defense-in-depth for a
# pre-1.0, single-maintainer dependency (insulates against upstream deletion and
# lets you patch it). The version is already pinned + hashed in go.sum, so this
# is optional; use it if you want to own the source.
#
# One-time manual step first: fork github.com/crgimenes/glaze on GitHub (the
# fork copies its tags, incl. v0.0.31). Then:
#   scripts/pin-glaze-fork.ps1 github.com/<you>/glaze          # defaults to v0.0.31
#   scripts/pin-glaze-fork.ps1 github.com/<you>/glaze v0.0.31
#
# Repoints the root module. Review the go.mod/go.sum changes and commit. To undo:
# `go mod edit -dropreplace github.com/crgimenes/glaze`. (The cli/npm/goleo bundle
# is generated from the root by cli/npm/copy-source.js, so it inherits the pin.)
param(
  [Parameter(Mandatory = $true)] [string]$Fork,
  [string]$Version = "v0.0.31"
)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
Push-Location $root
try {
  & go mod edit -replace "github.com/crgimenes/glaze=$Fork@$Version"
  & go mod tidy
  & go mod vendor
} finally { Pop-Location }
Write-Host "Pinned glaze -> $Fork@$Version in the root module. Review go.mod/go.sum and commit." -ForegroundColor Green

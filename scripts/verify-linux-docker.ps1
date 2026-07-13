#!/usr/bin/env pwsh
# Run the cgo-free glaze webview smokes on real Linux/WebKitGTK locally, via
# Docker (Docker Desktop WSL2 backend) — the same checks as the ubuntu job in
# .github/workflows/glaze-verify.yml, without CI. Each smoke builds
# CGO_ENABLED=0 and runs headless under xvfb, wrapped in a hard `timeout` so a
# GUI hang can't wedge the run. Usage: scripts/verify-linux-docker.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$image = "goleo-linux-verify"

Write-Host ">> building $image"
docker build -f "$root\scripts\linux-verify.Dockerfile" -t $image "$root\scripts"
if ($LASTEXITCODE -ne 0) { exit 1 }

function Run-Smoke($name, $sub, $target) {
  Write-Host ">> $name"
  docker run --rm -v "$root\$($sub -replace '/','\'):/work" $image bash -c `
    "CGO_ENABLED=0 go build -o /tmp/bin $target && timeout 60 xvfb-run -a /tmp/bin"
  return $LASTEXITCODE
}

$rc = 0
if ((Run-Smoke "webview round-trip" "spikes/glaze-webview" "./verify") -ne 0) { $rc = 1 }
if ((Run-Smoke "multi-window (2 windows, 1 loop)" "spikes/glaze-multiwindow" ".") -ne 0) { $rc = 1 }

if ($rc -eq 0) { Write-Host "ALL LINUX SMOKES PASSED" -ForegroundColor Green }
else { Write-Host "SOME LINUX SMOKES FAILED" -ForegroundColor Red }
exit $rc

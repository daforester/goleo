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

$script:rc = 0
function Run-Smoke($name, $sub, $target) {
  Write-Host ">> $name"
  # Let docker's output stream to the console; check $LASTEXITCODE directly (do
  # NOT return it — a function's return value merges with its stdout in
  # PowerShell, which would swallow the RESULT lines and break the check).
  docker run --rm -v "$root\$($sub -replace '/','\'):/work" $image bash -c `
    "CGO_ENABLED=0 go build -o /tmp/bin $target && timeout 60 xvfb-run -a /tmp/bin"
  if ($LASTEXITCODE -ne 0) { $script:rc = 1 }
}

Run-Smoke "webview round-trip" "spikes/glaze-webview" "./verify"
Run-Smoke "multi-window (2 windows, 1 loop)" "spikes/glaze-multiwindow" "."

# Runtime-level: a real goleo app (native IPC + permission shim +
# mainLoopWindowManager). Mounts the repo root so the spike's replace directive
# resolves the runtime from source.
Write-Host ">> runtime stack (native IPC + perm shim + in-proc 2nd window)"
docker run --rm -v "${root}:/work" -w /work/spikes/glaze-runtime-verify $image bash -c `
  "CGO_ENABLED=0 go build -o /tmp/bin . && timeout 60 xvfb-run -a /tmp/bin"
if ($LASTEXITCODE -ne 0) { $script:rc = 1 }

Write-Host ">> system tray (native tray, cgo-free)"
docker run --rm -v "${root}:/work" -w /work/spikes/glaze-tray-verify $image bash -c `
  "CGO_ENABLED=0 go build -o /tmp/bin . && timeout 30 xvfb-run -a /tmp/bin"
if ($LASTEXITCODE -ne 0) { $script:rc = 1 }

Write-Host ">> native menu bar (GTK3, cgo-free)"
docker run --rm -v "${root}:/work" -w /work/spikes/glaze-menu-verify $image bash -c `
  "CGO_ENABLED=0 go build -o /tmp/bin . && timeout 40 xvfb-run -a /tmp/bin"
if ($LASTEXITCODE -ne 0) { $script:rc = 1 }

# GTK4 / webkitgtk-6.0: exercises menu_linux.go's GMenu/GtkPopoverMenuBar path.
Write-Host ">> native menu bar (GTK4 / webkitgtk-6.0)"
docker build -q -f "$root\scripts\linux-verify-gtk4.Dockerfile" -t goleo-linux-verify-gtk4 "$root\scripts" | Out-Null
docker run --rm -e WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 -v "${root}:/work" -w /work/spikes/glaze-menu-verify goleo-linux-verify-gtk4 bash -c `
  "CGO_ENABLED=0 go build -o /tmp/bin . && timeout 40 dbus-run-session -- xvfb-run -a /tmp/bin"
if ($LASTEXITCODE -ne 0) { $script:rc = 1 }

$rc = $script:rc

if ($rc -eq 0) { Write-Host "ALL LINUX SMOKES PASSED" -ForegroundColor Green }
else { Write-Host "SOME LINUX SMOKES FAILED" -ForegroundColor Red }
exit $rc

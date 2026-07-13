#!/usr/bin/env bash
# Run the cgo-free glaze webview smokes on real Linux/WebKitGTK locally, via
# Docker (works with the Docker Desktop WSL2 backend on Windows) — the same
# checks as .github/workflows/glaze-verify.yml's ubuntu job, without CI.
#
# Each smoke builds CGO_ENABLED=0 and runs headless under xvfb, wrapped in a hard
# `timeout` so a GUI hang can't wedge the run. Usage: scripts/verify-linux-docker.sh
set -uo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="goleo-linux-verify"

echo ">> building $IMAGE"
docker build -f "$ROOT/scripts/linux-verify.Dockerfile" -t "$IMAGE" "$ROOT/scripts" || exit 1

run() { # name  mount-subdir  build-target
  echo ">> $1"
  docker run --rm -v "$ROOT/$2:/work" "$IMAGE" bash -c \
    "CGO_ENABLED=0 go build -o /tmp/bin $3 && timeout 60 xvfb-run -a /tmp/bin"
}

rc=0
run "webview round-trip" "spikes/glaze-webview" "./verify" || rc=1
run "multi-window (2 windows, 1 loop)" "spikes/glaze-multiwindow" "." || rc=1

# Runtime-level: a real goleo app (native IPC + permission shim +
# mainLoopWindowManager). Mounts the repo root so the spike's replace directive
# resolves the runtime from source.
echo ">> runtime stack (native IPC + perm shim + in-proc 2nd window)"
docker run --rm -v "$ROOT:/work" -w /work/spikes/glaze-runtime-verify "$IMAGE" bash -c \
  "CGO_ENABLED=0 go build -o /tmp/bin . && timeout 60 xvfb-run -a /tmp/bin" || rc=1

echo ">> system tray (native tray, cgo-free)"
docker run --rm -v "$ROOT:/work" -w /work/spikes/glaze-tray-verify "$IMAGE" bash -c \
  "CGO_ENABLED=0 go build -o /tmp/bin . && timeout 30 xvfb-run -a /tmp/bin" || rc=1

[ "$rc" = 0 ] && echo "ALL LINUX SMOKES PASSED" || echo "SOME LINUX SMOKES FAILED"
exit $rc

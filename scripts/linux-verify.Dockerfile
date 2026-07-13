# Headless Linux verification image for the cgo-free glaze webview backend.
# Reproduces the .github/workflows/glaze-verify.yml ubuntu job locally (Docker +
# WSL2), so the Linux GUI smokes can be run without GitHub Actions. See
# scripts/verify-linux-docker.{ps1,sh}.
FROM golang:1.26-bookworm

# Runtime libs glaze dlopens on Linux (GTK3 + WebKitGTK 4.1) + a virtual display.
# xauth is required by xvfb-run.
RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb \
      xauth \
      libgtk-3-0 \
      libglib2.0-0 \
      libwebkit2gtk-4.1-0 \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /work

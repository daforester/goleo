# Headless GTK4 / webkitgtk-6.0 verification image — glaze selects the GTK4
# backend when webkitgtk-6.0 is present, exercising menu_linux.go's GTK4 path
# (GMenu + GtkPopoverMenuBar). WebKitGTK 6.0 enforces a bubblewrap sandbox and
# needs a session bus, so runs use `dbus-run-session` +
# WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1 (see scripts/verify-linux-docker).
FROM golang:1.26-trixie

RUN apt-get update && apt-get install -y --no-install-recommends \
      xvfb xauth dbus dbus-x11 \
      libgtk-4-1 \
      libglib2.0-0 \
      libwebkitgtk-6.0-4 \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

WORKDIR /work

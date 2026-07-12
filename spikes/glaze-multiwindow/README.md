# Spike — macOS/Linux in-process multi-window via glaze (2026-07-13)

**Question:** can goleo host multiple in-process windows on macOS? The Windows
path (`inProcWindowManager`) gives each window its own `LockOSThread` goroutine
running its own `Run()` — but **AppKit is main-thread-only**, so a second NSApp
loop on another thread is impossible. macOS needs the *single-loop master*
model: one `[NSApp run]` on the main thread owns all windows.

**Finding: glaze already supports this.** Its darwin backend shares one
`NSApplication`, and the second `NewWindow()` skips the launch handshake
(`getAndSetIsFirstInstance()` → false) and just creates another `NSWindow`,
incrementing `windowCount`; the app terminates only when the last window closes
(`decWindowCount() <= 0`). Linux (GTK, also main-thread-only) works the same way.

So the model is: **create the primary window + `Run()` the loop on the main
thread; open additional windows by `Dispatch`-ing `glaze.New()` onto that same
thread (never call `Run()` on them).**

`main.go` proves the *dynamic* case goleo needs: it opens window 2 **after** the
primary loop is already running (from a `Dispatch` triggered once window 1 has
round-tripped), serves both over a loopback origin, and confirms **both** windows
complete a JS→Go round-trip. Cross-compiles cgo-free; runs on `macos-14` +
`ubuntu-latest` (xvfb) via `.github/workflows/glaze-verify.yml`.

## goleo integration design (next step, after this passes on hardware)

Add a **third** `windowSpawner` for macOS in-process (the Windows one won't port):

- **Main-thread handle:** `runWebview` registers the primary `*WebviewWindow`
  with the manager (like `setNativeWin`), giving it a `Dispatch` onto the single
  loop.
- **Open(opts):** marshal `NewWebviewWindow(...)` onto the main thread via
  `primary.Dispatch`, wait on a channel for the created window, track it, return
  its id. (`glaze.New` on the main thread creates the window under the shared
  NSApp; no per-window `Run()`.)
- **Close(id):** `Dispatch(win.Destroy())` — glaze decrements `windowCount` and
  closes just that window; the app keeps running while others remain.
- **CloseAll / lifecycle:** closing every window drives `windowCount` to 0 →
  glaze terminates → the primary's `Run()` returns → normal `shutdown()`.
  `ExitOnClose` and `Quit()` funnel through the existing lifecycle.

Selection: on macOS, `Config.InProcessWindows` picks this manager; Windows keeps
`inProcWindowManager`; everything else stays multi-process.

## Run it

```
cd spikes/glaze-multiwindow
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o /dev/null .   # compile check
./glazemw    # on a real Mac/Linux desktop (Linux: xvfb-run -a ./glazemw)
```

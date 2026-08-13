/// <reference path="./init.d.ts" />
// init.js — Goleo startup script.
//
// Runs inside the Go backend (embedded JS engine) before any window is shown. Two
// jobs: it decides how many windows to open and how each is configured, and any
// function it defines can be called from Go — so app logic you want to change
// without a rebuild can live here.
//
//   // in init.js
//   function priceOrder(o) { return o.qty * o.unit * 1.2 }
//
//   // in backend/app/app.go
//   total, err := a.JS().Call(ctx, "priceOrder", order)
//
// Calls are serialised onto one goroutine (the JS engine is not thread-safe), take
// a context so a runaway script cannot wedge the app, and cross as JSON. A JS throw
// comes back as a Go error.
//
// THE COMPLETE API available TO this script. These three globals are everything the
// engine defines — there is nothing else in scope here.
//
//   getConfig()
//     -> { title, width, height, devMode, devServer, port, url }
//        Values from runtime.Config, plus the resolved port and the app's own URL.
//
//   createWindow(opts)
//     -> true if a native window was created; false in browser mode (goleo dev,
//        goleo emulate, mobile), where there is no native window to create.
//        opts, with the default each falls back to:
//          title      Config.Title        width      Config.Width
//          height     Config.Height       minWidth   0 (no minimum)
//          minHeight  0 (no minimum)      center     true
//          devTools   Config.DevMode      url        the app's own URL
//
//   console.log / console.info / console.warn / console.error
//
// There is NO bridge object here, and no way to call goleo:* commands from this
// file. Those are the FRONTEND's API: import them from @goleo/bridge in
// frontend/src, where every call is checked against the Policy ACL. For backend
// logic, write Go — register a handler with a.Bridge().Handle(...) in
// backend/app/app.go.
//
// Delete this file (and its embed line in main.go) to fall back to the
// built-in window setup from runtime.Config.

const config = getConfig()

createWindow({
  title: config.title,
  width: config.width,
  height: config.height,
  center: true,
})

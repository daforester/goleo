# 7. RPC in depth

Goleo's frontend↔backend RPC is one model — `{id, method, args}` request →
`{id, result | error}` response, plus one-way events — carried over whichever
transport is available. You write handlers in Go and call them from JS; the plumbing
is uniform.

## Backend: handlers

```go
app.Bridge().Handle("users:get", func(ctx context.Context, args json.RawMessage) (any, error) {
    var q struct{ ID string }
    if err := json.Unmarshal(args, &q); err != nil {
        return nil, err                    // → rejected promise on the frontend
    }
    u, err := db.FindUser(q.ID)
    if err != nil {
        return nil, err
    }
    return u, nil                          // → JSON-resolved on the frontend
})
```

- **Naming**: use a `namespace:verb` convention (`users:get`, `files:save`). Goleo's
  own commands use the `goleo:` prefix.
- **Args**: a single JSON value (`json.RawMessage`); unmarshal into a struct.
- **Return**: any JSON-serializable value, or `(nil, error)`.
- **Context**: `ctx` is cancelled on shutdown; honor it for long work.
- Handlers may run on a background goroutine — don't touch UI-thread-only state
  directly; use events or `Dispatch` where needed.

## Frontend: invoke

```ts
import { invoke } from '@goleo/bridge'

try {
  const user = await invoke('users:get', { id: '42' })
} catch (e) {
  // the Go error surfaces here
}
```

Generate typed overloads so `invoke('users:get', …)` is checked and autocompleted:
```bash
npm run goleo:types      # writes frontend/src/goleo.d.ts
```

## Events (push)

Backend → frontend:
```go
app.Emit("download:progress", map[string]any{"pct": 40})
```
```ts
const off = on('download:progress', ({ pct }) => setProgress(pct))
// off() to unsubscribe
```

Frontend → backend (fire-and-forget, no reply):
```ts
// @goleo/bridge sends one-way messages that map to app.On(...) handlers
```
```go
app.On("telemetry:event", func(ctx context.Context, data json.RawMessage) { /* ... */ })
```

## Transports (chosen automatically)

`initBridge()` selects, in order:

1. **Native in-process IPC** (`Config.NativeIPC`) — the desktop webview's own
   message channel (a bound Go function for JS→Go, evaluated JS for Go→JS). No
   socket, no port. Lowest latency; used by the primary + in-process windows.
2. **WebSocket** — persistent, bidirectional. The default and the backbone for
   child-process windows, browser/PWA, and mobile. Supports server push.
3. **HTTP POST** (`/api/invoke`) — fallback when WebSocket is unavailable. No push.

All three carry the same envelope and funnel through the same
`Bridge.HandleRequest`, so your handlers and the security policy behave identically
regardless of transport. The frontend falls back down the list transparently.

## Portless + secure: native IPC + scheme assets

For a desktop app that opens **no TCP port at all** while keeping a *secure
context* (so `localStorage`, `crypto.subtle`, `getUserMedia`, and history routing
work):

```go
goleo.Config{
    NativeIPC:    true,   // RPC over the in-process channel (no WS port)
    SchemeAssets: true,   // serve the UI from goleo:// (no asset-server port)
    // AssetScheme: "goleo",  // optional; the origin's scheme
}
```

- RPC goes over native IPC; the UI loads from a **custom secure origin**
  (`goleo://…` on macOS/Linux; a secure `https://<scheme>.localhost` virtual host on
  Windows — transparent to your code, which uses relative URLs).
- The loopback HTTP/WS server stays up as a fallback for other transports.

This is production-friendly: smaller attack surface, no port to bind or collide on.

## Security: the capability policy

Lock down which commands the frontend may call with a `Policy` (deny-by-default
when set, enforced centrally in `Bridge.HandleRequest` for **every** transport):

```go
app.SetPolicy(&goleo.Policy{
    Allow: []string{"users:*", "files:read", "goleo:clipboard*"},
})
```

- `prefix*` wildcards are supported; core-safe commands are always allowed.
- `SetPolicy` takes a **pointer** (`*Policy`).
- The production server also binds loopback-only, checks the WS origin allow-list,
  and injects a per-launch token into `index.html`. Native IPC sidesteps the socket
  surface entirely while the policy still applies.
- `HTTPHosts` and `ShellPrograms` are **reserved** — there is no http or shell
  plugin yet, so they gate nothing today.

## Filesystem scope

The `fs` plugin is confined by default. It may reach:

1. the app's own data directory (always),
2. anything listed in `Policy.FSRoots`,
3. any path the user picked in a native dialog this session — so the ordinary
   "user chooses a file, app opens it" flow needs no configuration.

```go
app.SetPolicy(&goleo.Policy{
    Allow:   []string{"goleo:fs*"},
    FSRoots: []string{"/srv/shared-data"},   // widens the scope
})
```

- **Writes and deletes** outside the scope are refused. An unrecoverable
  `RemoveAll` gets no compatibility window.
- **Reads** outside the scope currently succeed but log a one-time deprecation
  warning naming the path. They will become errors.
- **System locations** (`C:\Windows`, `%ProgramFiles%`, `/usr`, `/etc`, …) are
  refused for writes in *every* mode.
- Symlinks are resolved before the check, so a link inside a root cannot point out
  of it.

Need the whole disk (a file manager, a dev tool)? Opt out explicitly:

```go
runtime.New(runtime.Config{FSScope: runtime.FSScopeUnrestricted})
```

or set `GOLEO_FS_UNRESTRICTED=1`.

---

Next: [Native menus →](08-menus.md)

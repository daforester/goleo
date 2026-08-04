# Migrating between goleo versions

Only changes that can break a working app are listed here. goleo is pre-1.0, so
breaking changes do happen — each one below says how to tell whether it affects
you and what to do.

---

## 0.9.0 — filesystem access is confined by default

**Affects:** apps that call `runtime.RegisterFS` (which `RegisterDesktopFeatures`
does, so both scaffolds enable it) **and** pass absolute paths from outside the
app's own data directory.

### What changed

`runtime/fs` previously rejected only *relative* traversal (`../x`). Every
absolute path was allowed, so anything running in the webview — including an XSS,
a compromised npm/CDN dependency, or a third-party analytics script — could read
`~/.ssh/id_rsa` or hand a home directory to `os.RemoveAll` via `goleo:fsDelete`.
`Policy.FSRoots` looked like the mitigation but had no call sites at all.

The `fs` plugin is now confined to:

1. the app's own data directory (`os.UserConfigDir()/<AppID or Title>`),
2. anything in `Policy.FSRoots`,
3. any path the user picked in a native file dialog during this session.

| Operation | Outside the scope |
|---|---|
| write, delete | **refused** — no compatibility window for `os.RemoveAll` |
| read | allowed, logs a one-time deprecation warning; **will become an error** |
| write to a system location (`C:\Windows`, `/usr`, `/etc`, …) | **refused in every mode** |

Symlinks are resolved before the check, so a link inside an allowed root cannot
point out of it.

### Do I need to change anything?

Run your app and exercise its file features. If it works with no
`goleo: DEPRECATED — read outside the allowed filesystem roots` lines in the
output, you are unaffected.

- **Reading a file the user picked** — no change needed. Dialog results are
  granted automatically.
- **Reading/writing your own app data** — no change needed.
- **A fixed directory outside app data** (a shared data dir, a sibling project):
  add it to the policy.

  ```go
  app.SetPolicy(&runtime.Policy{
      Allow:   []string{"goleo:fs*"},
      FSRoots: []string{"/srv/shared-data"},
  })
  ```

  `FSRoots` widens the scope; it does not replace the app data directory.

- **You genuinely need the whole disk** (a file manager, a dev tool):

  ```go
  runtime.New(runtime.Config{FSScope: runtime.FSScopeUnrestricted})
  ```

  or set `GOLEO_FS_UNRESTRICTED=1` in the environment. The system-location
  deny-list still applies to writes.

### What is already in scope for you

- **`appDataDir()` brings its own directory into scope.** Calling
  `goleo:fsAppDataDir` / `appDataDir("my-app")` registers the directory it returns,
  so you can write there immediately without touching `Policy.FSRoots`. Vending a
  path and then refusing writes to it would be incoherent — and it broke the
  scaffolded demo, whose FileSystem page does exactly this.

  One new restriction: `appName` becomes a path element, so it may not contain a
  path separator or `..`. `appDataDir("../../etc")` used to return `/etc`; it now
  errors. Without that guard, granting the result would have handed out an
  arbitrary directory to anything running in the webview.

- **`homeDir()` grants nothing.** It answers "where is home", which is
  informational. Granting it would hand back the whole user profile and defeat the
  confinement, so writing under it still needs `Policy.FSRoots` or a dialog-picked
  path.

- **Relative paths resolve against the backend's working directory**, not your app
  data directory, and are therefore usually out of scope. Previously a relative path
  silently wrote into the project directory. Resolve `appDataDir()` first and join
  onto it.

### Also in this change

- `Policy.FSRoots` is now actually enforced (it previously did nothing).
- Error messages are preserved end-to-end. `@goleo/bridge`'s `fs` wrappers used to
  replace every failure with `"<op> requires the Go backend"`, which would have
  masked the confinement message entirely. They now rethrow the backend's own error
  — which names the offending path and the three ways to allow it — and only claim
  the backend is missing when it genuinely is.
- `Policy.HTTPHosts` and `Policy.ShellPrograms` are documented as **reserved** —
  goleo has no http or shell plugin, so they gate nothing today. They are still
  accepted so a policy written now keeps working when those plugins land.
- `Policy.AllowsFSPath` is retained as a raw helper but is **not** the enforcement
  path; its "empty list means unconstrained" rule is the opposite of the default
  the plugin needs. Use the scope model, not this helper, to decide access.

---

## 0.9.0 — `@goleo/bridge` reports failures instead of inventing values

**Affects:** any code that relied on a bridge call *succeeding* when it had actually
failed. If your callers already `try/catch` (or `.catch()`), you are unaffected.

Several wrappers converted a failure into a value the caller could not distinguish
from a real result. Each now throws when the backend is present and only falls back
when there genuinely is no backend.

| Call | Was, on failure | Now |
|---|---|---|
| `showMessage()` | returned `'OK'` | throws; with no backend, asks via `confirm`/`alert` |
| `saveFile()`, `selectFolder()` | returned `null` (= "user cancelled") | throws; `null` still means cancel |
| `showPrompt()` | silently opened a browser prompt | throws; browser prompt only with no backend |
| `openFile()`, `openFiles()` | **never settled** if the picker was cancelled | resolves `null` / `[]` |
| `readTextFile()` etc. | `"<op> requires the Go backend"` | the backend's real error |
| `storeGet()`/`storeSet()` etc. | silently switched to `localStorage` | throws; `localStorage` only with no backend |
| `bleConnect()` | resolved having done nothing | throws |

The one to look at is **`showMessage`**. It used to return `'OK'` when the dialog
failed, so a caller asking "Delete all data?" proceeded with the deletion. If you
wrote code assuming it always resolves, it can now reject — which is the point.

Two additions rather than changes:

- **`reconnect()`** — there was previously no way out of local-only mode. If the
  initial connect timed out (3s default) or the retries were spent, every non-core
  method threw "backend not connected" for the rest of the session, even after the
  backend came up. Wire it to a retry affordance or to `bridge:reconnectFailed`.
  Retries now also back off exponentially with jitter instead of a fixed 3s.
- **`Capabilities.menu`** — the Go side has always returned it; the TypeScript type
  omitted it. `menuSupported()` now shares the cached, no-backend-safe path with
  `isWindowingSupported()`/`isTraySupported()` instead of throwing where they
  return `false`.

### Binary files cross the bridge as base64

`readBinaryFile()`/`writeBinaryFile()` were corrupting any payload that was not
plain ASCII, in both directions. They now use base64 on both ends, which is what
`encoding/json` already does for a Go `[]byte`.

You only need to act if you call `goleo:fsReadBinaryFile` / `goleo:fsWriteBinaryFile`
through `invoke()` **directly** rather than through the wrappers — in which case
`data` is now base64 in both directions. The wrappers still take and return a
`Uint8Array`, so their callers change nothing.

---

## 0.9.0 — each app gets its own key/value store

**Affects:** apps using `RegisterStore` / `@goleo/bridge`'s `storeGet`/`storeSet`.
**No action needed** — existing data is migrated automatically. Read on only if you
care what happens.

Every goleo app on a machine previously shared **one** `store.json`, under
`<UserConfigDir>/goleo-app/`. Two different goleo apps read and clobbered each
other's keys. The store now lives under the app's own name
(`Config.AppID`, falling back to `Config.Title`).

On first run at the new path, the old shared store is **copied** into it, so nothing
is lost. It is a copy rather than a move because other apps on the machine may still
be reading the legacy file. An app that already has its own store is never
overwritten.

Two consequences worth knowing:

- **You may inherit another app's keys once.** If several of your goleo apps shared
  the legacy store, each one adopts a copy of the whole thing on first run. Delete
  any keys that do not belong to that app, or clear them with `storeClear()`.
- **The legacy file is left behind.** Once every app on the machine has migrated,
  `<UserConfigDir>/goleo-app/store.json` can be deleted by hand.

Also fixed here: writes now `fsync` before the rename (a crash could previously
leave a zero-length store where a valid one had been), and each write uses a unique
temp file — the old fixed `store.json.tmp` meant two writers, such as a second
instance racing the primary, shared one temp path and could interleave into a
corrupt store.

---

## 0.9.0 — `goleo:openURL` only opens web schemes

**Affects:** apps that pass anything other than `http`, `https`, `mailto` or `tel`
to `openURL` / `goleo:openURL`.

`openURL` handed any string to the OS handler, so `file://`, UNC paths and paths
to executables all went through — arbitrary execution from anything running in the
webview. It is now restricted to `http`/`https`/`mailto`/`tel` plus the app's own
`Config.URLScheme` (registered automatically by `App.Run`).

To open a different scheme deliberately, register it:

```go
runtime.AllowURLScheme("myproto")
```

Opening a local file is intentionally not supported through `openURL` — use the
`fs` plugin (within its scope) or a native dialog.

---

## 0.9.0 — dev-mode bridge rejects public origins

**Affects:** development setups served from a public hostname (a tunnel such as
ngrok, or a hosted preview) — not local Vite, not `goleo emulate android`, not LAN
device testing.

In dev mode the bridge accepted **any** `Origin`, so any page the user happened to
visit could open `ws://127.0.0.1:<port>/ws`, invoke every registered method
(including the filesystem plugin) and read the replies. Dev now allows loopback,
private-network and link-local origins on any port — which covers Vite on any
port, the Android emulator's `10.0.2.2`, and real-device testing over the LAN —
and rejects public origins with an explanatory log line.

If you serve your dev frontend from a public hostname:

```bash
GOLEO_DEV_ALLOWED_ORIGINS=https://my-tunnel.example.com goleo dev
```

Comma-separate multiple origins. Production behaviour is unchanged.

---

## 0.9.0 — a malformed `goleo.json` now fails the build

**Affects:** projects whose `goleo.json` does not parse, or has a key of the wrong
type (`"version": 2.0` instead of `"2.0"`).

Every loader used to swallow parse errors and fall back to defaults, so a trailing
comma produced a *successful* build of an app named "Goleo App" with identifier
`com.goleo.app` and version `0.1.0`. Malformed config is now reported:

```
goleo.json is not valid JSON: invalid character ',' looking for beginning of object key string
```

Fix the JSON. Unknown/extra keys are still accepted.

Three previously-ignored keys now take effect —
`mobile.android.min_sdk`, `mobile.ios.deployment_target` and
`mobile.ios.bundle_identifier`. If you had set them expecting them to work, they
now do; if you set them to something wrong, the build will change accordingly. The
iOS bundle identifier previously fell back to the *Android* `package_name`, and
still does when unset.

---

## 0.9.0 — existing projects must update their `@goleo/bridge` pin

**Affects:** every project scaffolded by `goleo new` before this release. Check with:

```
grep '@goleo/bridge' frontend/package.json
```

If it says `"^0.2.1"`, you are affected. `goleo dev` and `goleo build` now warn
about it too.

**What was wrong.** The generated `frontend/package.json` hardcoded
`"@goleo/bridge": "^0.2.1"`. A caret on a `0.x` version locks the *minor*, so that
range resolves to **0.2.9** — it never picks up 0.3 or later. The Go side had
already been changed to pin the CLI's own version, so a new project got a v0.8.x
runtime alongside a bridge six minors old.

That is a skew across a wire contract, not just an old dependency, and it fails
quietly:

- **binary file I/O was broken.** `writeBinaryFile` in 0.2.x sends
  `TextDecoder` output where the current runtime expects base64, and
  `readBinaryFile` expects a shape the runtime no longer returns.
- **filesystem errors were misattributed.** Confinement errors surfaced as
  `"… requires the Go backend"` instead of the real message naming the path.
- Missing since 0.2.x: `reconnect()` (a slow backend stranded the app in
  local-only mode for the whole session), `showMessage` throwing rather than
  reporting a fake `'OK'`, per-call localStorage fallbacks in `store`, and the
  cached-capabilities fix that made `openWindow` throw "not supported" forever.

Nobody hit this while working on goleo itself because `goleo new` npm-links a local
bridge checkout over the dependency — the development path masked it and only
end users got the stale pin.

**Fix — in your project:**

```
cd frontend && npm install @goleo/bridge@<your goleo version>
```

Use the version `goleo version` reports, so the bridge and runtime stay in
lockstep. New projects now get this automatically: the pin is injected from the
CLI's own version, exactly like the `go.mod` require.

---

## 0.9.0 — `goleo build` on Windows now produces `app.exe`

**Affects:** Windows only, and only scripts that referred to the built file by name.

`goleo build` (the default `current` target) wrote a binary with **no extension** —
`app`, not `app.exe`. Windows will not execute that: double-clicking does nothing
and `Start-Process .\app` fails with "the system cannot find all the information
required". Only the explicit cross-target `goleo build windows` was correct, because
the `current` entry in the target table took the host's `GOOS` while hardcoding an
empty extension.

**What to do.** Nothing, unless a script, installer input, or CI step referenced
`app` literally on Windows — those need `app.exe`. `-o` is unaffected: an explicit
`-o myapp` becomes `myapp.exe`, and `-o myapp.exe` is left alone (no doubling).

If you had worked around this by renaming the output yourself, drop the rename.

---

## Unreleased — `goleo:share` only accepts web URLs

**Affects:** apps calling `runtime.RegisterShare` (the demo scaffold does) that passed
`goleo:share` anything other than an `http`/`https`/`mailto`/`tel` URL or their own
registered `Config.URLScheme`.

On desktop, share hands `data.URL` to the OS default handler — `rundll32
url.dll,FileProtocolHandler` on Windows, `open` on macOS, `xdg-open` on Linux. It did
so without validating the URL, so a `file://` URL, a UNC path, or a bare path to an
executable was **arbitrary execution from any script in the webview**:

```js
invoke('goleo:share', { url: 'file:///C:/Windows/System32/calc.exe' })
```

`goleo:openURL` was given a scheme allow-list for exactly this reason. Share reaches
the same handlers and was missed — it was written from the same mechanism but not the
same guard. Both now go through one validator, so a third caller cannot diverge again.

**What to do.** Nothing for ordinary use: sharing a link still works. If you were
relying on share to open a local file, use the filesystem plugin, or register your
own scheme via `Config.URLScheme` and handle it in-app.

---

## Unreleased — Linux notifications and the Windows prompt handle text more faithfully

**Affects:** Linux desktop notifications, and multi-line messages in
`showPrompt` on Windows. No API change.

`goleo:notify` is a default builtin, so its title and body come from the frontend.
On Linux they were passed to `notify-send` as bare positional arguments, and
notify-send uses GLib option parsing — so a title beginning with `--` was read as an
option rather than as the summary. `--help` or `--version` made notify-send print and
exit 0, meaning **the notification silently never appeared while the call reported
success**, and `--icon=` / `--hint=` / `--expire-time=` let a frontend restyle or
suppress notifications the app believed it controlled. Arguments are now passed after
a `--` terminator. No shell was involved, so this was flag confusion, not RCE.

Separately, `showPrompt` on Windows built its PowerShell script by replacing newlines
with a backtick-`n` escape. That escape only means anything in a *double*-quoted
PowerShell string; the values are interpolated into *single*-quoted literals, where it
is taken literally — so every newline in a title or message reached the dialog as the
two characters `` `n ``. Newlines are now preserved, and `
` normalises to one
line break instead of two.

**What to do.** Nothing. If you had worked around the Windows prompt by pre-formatting
messages without newlines, you can stop.


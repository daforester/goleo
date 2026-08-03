# Migrating between goleo versions

Only changes that can break a working app are listed here. goleo is pre-1.0, so
breaking changes do happen — each one below says how to tell whether it affects
you and what to do.

---

## Unreleased — filesystem access is confined by default

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

## Unreleased — `goleo:openURL` only opens web schemes

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

## Unreleased — dev-mode bridge rejects public origins

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

## Unreleased — a malformed `goleo.json` now fails the build

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

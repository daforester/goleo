# Draft reply to the maintainer review (PR: custom URL-scheme handlers)

Pushed as `7d4faa7` on `upstream-scheme`. Post the block between the markers.

<!-- ===================== POST FROM HERE ===================== -->

Thanks for the thorough review — all of it made sense. Pushed in `7d4faa7`;
CI is green from here (build + vet + golangci-lint per GOOS, gofmt, and the
scheme tests). Point by point:

**Windows: AddRef the stored environment.** Done. `handlerInvoke` now `AddRef`s
the environment before storing it and `Destroy` `Release`s it, matching the
controller/webview2 references. Added `iEnvironment.AddRef/Release` for it.

**Linux: `g_memdup2` guarded lookup.** Done, and I went one step past "bail
gracefully": rather than skip the scheme when `g_memdup2` is missing, I resolve
it with `Dlsym` and **fall back to `g_memdup`** (the older `guint`-length form)
on GLib < 2.68, so custom schemes keep working on Debian 11 / Ubuntu 20.04
instead of being disabled there. It only errors if neither symbol exists (which
shouldn't happen). Happy to reduce this to a plain skip-with-error if you'd
rather not lean on the deprecated `g_memdup` — but it seemed worth keeping the
feature alive on the older LTS distros you named.

**macOS: no nil `NSError`.** Done. The not-found path now builds a real
`NSError(NSURLErrorDomain, NSURLErrorFileDoesNotExist)` for `didFailWithError:`.

**macOS: the two leaks.** Done. The `NSHTTPURLResponse` is now autoreleased
(it was `alloc`/`init`'d and leaked once per request), and the
`WKURLSchemeHandler` delegates are tracked and removed from the instance
registry in `Destroy`, so the engine is no longer pinned after teardown.

**One canonical URL on every platform.** Done. `serveScheme` on Windows now
reconstructs the original `<scheme>://<authority>/path` form before invoking
the handler, so `SchemeRequest.URL` is identical to the macOS/Linux backends
(`app://home/index.html`, not `https://app.localhost/index.html`). The
authority the app navigated with is recorded in `rewriteSchemeURL` (the vhost
origin has nowhere to carry it) and restored at request time; if a scheme is
somehow served before any navigation, it falls back to the scheme name so the
URL is still well-formed `scheme://`. I kept the existing
`https://<scheme>.localhost` vhost + filter (already verified on real WebView2)
rather than encoding the authority into a subdomain, which would have changed
the filter and needed a fresh hardware pass. And the fragment is now preserved
on `Navigate` — it was being dropped, which broke hash/path routing.

**Linux: fail loudly.** Done. `registerSchemes` returns an error when a scheme
can't be registered (missing library/handle/symbol, nil web context, nil
security manager) and `NewWithOptions` propagates it and tears down the
half-built window, instead of silently leaving `app://` pages unable to load.

**Docs and example.** Added a "Custom URL schemes" section to the README (the
purpose, the secure-context table, the Windows vhost note) and a runnable
`examples/scheme` program that serves an embedded page over `app://` and
reports `isSecureContext`, a `localStorage` round-trip, a `crypto.subtle`
digest, and a sub-resource load. Extended `webview2_scheme_windows_test.go`
with fragment, reconstruction, fallback, and rewrite→reconstruct round-trip
cases.

**CI.** Thanks for approving the run — everything the matrix checks passes
locally here (all three OSes at compile/lint/logic level; I don't have physical
macOS/Linux to drive the GUI tests, so the runners are the source of truth for
the on-hardware pass).

<!-- ===================== END POST ===================== -->

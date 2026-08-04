package notify

// notifySendArgs builds the argv for notify-send.
//
// Kept free of build constraints and of exec so the property below can be tested on
// any host, rather than only on a Linux box with libnotify and a session bus.
//
// The property: the frontend-supplied summary and body must never be parsed as
// options. notify-send uses GLib option parsing, so a title beginning with `--`
// becomes a flag — and `goleo:notify` is a default builtin, so any script in the
// webview can set it. `--help`/`--version` make notify-send print and exit 0, so the
// notification silently never appears while the call reports success; `--icon=`,
// `--hint=` and `--expire-time=` let a frontend restyle or suppress notifications the
// app believes it controls. `--` ends option parsing, so everything after it is
// positional whatever it contains.
func notifySendArgs(title, body string) []string {
	return []string{"--", title, body}
}

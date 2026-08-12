# @goleo/cli

The command-line tool for [Goleo](https://github.com/daforester/goleo) — build
cross-platform **desktop and mobile** apps with a Go backend and any web frontend.

## Install

```bash
npm install -g @goleo/cli
```

This installs the `goleo` command. The matching native binary for your OS/CPU is
delivered automatically as an optional dependency
(`@goleo/cli-<os>-<arch>`) — no build step, no download at install time.

> Requires the [Go toolchain](https://go.dev/dl/) on your machine to *build* apps
> (the `goleo` CLI orchestrates `go build`, Vite, gomobile, etc.). Go is not
> needed just to install the CLI.

## Usage

```bash
goleo new my-app            # scaffold a project
cd my-app
cd frontend && npm install && cd ..
goleo dev                   # dev mode (Go + Vite HMR, native window)
goleo build                 # standalone binary for this OS
goleo build windows         # cross-compile
goleo build --bundle        # native installer (NSIS / .dmg / .deb+.rpm)
goleo build android         # installable APK
```

Run `goleo --help` for the full command list, or see the
[Developer Guide](https://github.com/daforester/goleo/tree/master/docs/guide).

## How the binary is delivered

`@goleo/cli` ships a thin Node launcher (`bin/goleo.js`) and lists one
prebuilt-binary package per platform in `optionalDependencies`
(`@goleo/cli-darwin-arm64`, `@goleo/cli-linux-x64`, `@goleo/cli-win32-x64`, …).
npm installs only the one matching your `os`/`cpu`, and the launcher execs it.
If no platform package is present, the launcher prints how to build from source
(`go install github.com/daforester/goleo/cli/goleo@latest`).

## License

MIT

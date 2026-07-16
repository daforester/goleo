# create-goleo-app

Scaffold a new [Goleo](https://github.com/daforester/goleo) project — a
cross-platform **desktop and mobile** app with a Go backend and a web frontend.

## Usage

```bash
npm create goleo-app@latest my-app
# or
npx create-goleo-app my-app
```

Then:

```bash
cd my-app
cd frontend && npm install && cd ..
npx goleo dev       # start development
npx goleo build     # build for the current platform
```

This creates a ready-to-run project: a Go backend (`backend/app/app.go`), a Vue +
Vite frontend wired to `@goleo/bridge`, and the `goleo:*` npm scripts for dev,
build, bundling installers, and mobile.

See the [Developer Guide](https://github.com/daforester/goleo/tree/master/docs/guide)
for what to do next.

## License

MIT

#!/usr/bin/env node

// Keeps @goleo/cli's optionalDependencies pinned to its own version, without
// cross-compiling (build-platform-packages.js does the same sync, but only as
// a side effect of a full binary build, and only inside the release workflow
// — that mutation is never committed back). Left unsynced, a plain `npm
// version X --workspaces` bump only touches the top-level "version" field, so
// the committed optionalDependencies silently freeze at whatever version was
// last synced (this happened for real: they sat at 0.3.0 through fifteen
// releases, 0.4.0 through 0.8.4, before this script existed). That's harmless
// for end users — `npm install -g @goleo/cli` installs from the registry,
// where the published package.json has the correct synced versions — but it
// corrupts any local development checkout of this repo: `npm install` at the
// root resolves cli/npm as a workspace member and reads ITS committed
// optionalDependencies verbatim, silently installing an ancient platform
// binary alongside a current-version @goleo/cli wrapper. Wired as this
// package's "version" npm lifecycle script, so `npm version` (used by
// RELEASING.md's release step) keeps this in sync automatically.

import { readFileSync, writeFileSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const pkgPath = resolve(__dirname, 'package.json')
const pkg = JSON.parse(readFileSync(pkgPath, 'utf8'))

for (const name of Object.keys(pkg.optionalDependencies || {})) {
  pkg.optionalDependencies[name] = pkg.version
}

writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + '\n')
console.log(`Synced @goleo/cli optionalDependencies to ${pkg.version}`)

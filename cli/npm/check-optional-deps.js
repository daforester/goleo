#!/usr/bin/env node

// Refuses to publish @goleo/cli unless every optionalDependency is pinned to this
// exact version.
//
// This exists because the committed package.json deliberately carries "latest" for
// the six @goleo/cli-<os>-<arch> packages, and build-platform-packages.js re-stamps
// them to the exact version just before publishing. That arrangement fixes a real
// problem (see below) but it moves a correctness requirement from "committed state" to
// "a step that must run first", so it needs a guard that fails closed.
//
// Why nothing is committed: the exact version cannot be. A release bumps
// cli/npm/package.json to X.Y.Z and commits, but @goleo/cli-<os>-<arch>@X.Y.Z does not
// exist on the registry until the release workflow publishes it *afterwards*. npm
// cannot write lockfile entries for a version it cannot resolve, and because these are
// OPTIONAL dependencies it drops them silently instead of failing — so the committed
// tree ended up declaring six packages with zero lockfile entries, and `npm ci` (which
// requires package.json and package-lock.json to agree exactly) refused on a fresh
// clone with "Missing: @goleo/cli-darwin-arm64@X.Y.Z from lock file".
//
// The first fix for that committed "latest" instead, which always resolves. It worked,
// but traded one problem for a quieter one: "latest" is a FLOATING range and the
// lockfile records whatever it resolved to once. That snapshot then never moves. It sat
// at 0.9.1 while the tree was at 0.10.12, so every `npm install` in this repo — which
// scripts/setup.* runs — pulled a months-old published binary into node_modules, where
// bin/goleo.js found it before the developer's own build and refused to run.
//
// So now the committed tree declares NONE, and build-platform-packages.js adds all six
// at the exact version just before publishing (it replaces the whole object, so it never
// needed them pre-declared). Nothing floats, nothing is pinned stale, and `npm ci` has
// nothing to reconcile. What must not happen is @goleo/cli reaching the registry with
// them missing or floating — end users would get no binary, or a drifting one that
// bin/goleo.js's version guard then refuses to run. Hence this check, which fails closed
// on both.

import { readFileSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const pkg = JSON.parse(readFileSync(resolve(__dirname, 'package.json'), 'utf8'))

const deps = pkg.optionalDependencies || {}
const names = Object.keys(deps)

if (names.length === 0) {
  console.error('check-optional-deps: @goleo/cli declares no optionalDependencies — the platform binaries would not be installable')
  process.exit(1)
}

const wrong = names.filter((n) => deps[n] !== pkg.version)
if (wrong.length > 0) {
  console.error(
    `check-optional-deps: refusing to publish @goleo/cli@${pkg.version}.\n` +
      `These optionalDependencies are not pinned to ${pkg.version}:\n` +
      wrong.map((n) => `  ${n}: ${deps[n]}`).join('\n') +
      `\n\nRun \`node cli/npm/build-platform-packages.js\` first — it builds the six\n` +
      `platform packages and stamps these to the exact version. \`npm run publish:cli\`\n` +
      `and the release workflow both do this; publishing @goleo/cli on its own does not.`
  )
  process.exit(1)
}

console.log(`check-optional-deps: all ${names.length} platform packages pinned to ${pkg.version}`)

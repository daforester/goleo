#!/usr/bin/env node

// Copies the Go source and bridge npm package into the npm package bundle.
// These are needed at runtime by the goleo binary to create the replace directive
// and link the @goleo/bridge npm package.

import { cpSync, existsSync, rmSync } from 'fs'
import { resolve, dirname, basename } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PROJECT_ROOT = resolve(__dirname, '..', '..')
const GOLEO_DIR = resolve(__dirname, 'goleo')

// Clean previous bundle
if (existsSync(GOLEO_DIR)) {
  rmSync(GOLEO_DIR, { recursive: true })
}

// Copy Go source (runtime package + go.mod)
cpSync(resolve(PROJECT_ROOT, 'runtime'), resolve(GOLEO_DIR, 'runtime'), { recursive: true })
cpSync(resolve(PROJECT_ROOT, 'go.mod'), resolve(GOLEO_DIR, 'go.mod'))
if (existsSync(resolve(PROJECT_ROOT, 'go.sum'))) {
  cpSync(resolve(PROJECT_ROOT, 'go.sum'), resolve(GOLEO_DIR, 'go.sum'))
}

// Copy the vendored dependencies (including the pinned github.com/crgimenes/glaze
// fork) so the bundled goleo module is self-contained and builds without fetching
// third-party code from the network.
if (existsSync(resolve(PROJECT_ROOT, 'vendor'))) {
  cpSync(resolve(PROJECT_ROOT, 'vendor'), resolve(GOLEO_DIR, 'vendor'), { recursive: true })
}

// Copy bridge npm package (for npm link @goleo/bridge).
//
// Filtered, because this whole tree is published inside @goleo/cli (see the
// package's "files": ["bin/goleo.js", "goleo", ...]) and a plain recursive copy
// takes whatever happens to be sitting in bridge/ at publish time:
//
//   node_modules — currently ~1KB only because npm hoists workspace deps to the
//     repo root. Anyone who runs `npm install` inside bridge/ standalone would
//     silently add tens of MB to every published CLI.
//   *.test.ts / vitest.config.ts — test scaffolding is not something users need,
//     and the bundled package.json's "test" script would fail confusingly there
//     since vitest is not installed in that context. tsconfig.json already
//     excludes tests from dist/ for the same reason; this is the other half.
cpSync(resolve(PROJECT_ROOT, 'bridge'), resolve(GOLEO_DIR, 'bridge'), {
  recursive: true,
  filter: (src) => {
    const name = basename(src)
    if (name === 'node_modules') return false
    if (name.endsWith('.test.ts') || name === 'vitest.config.ts') return false
    return true
  },
})

console.log('[goleo] Go source and bridge bundled in:', GOLEO_DIR)

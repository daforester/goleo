#!/usr/bin/env node

// Copies the Go source and bridge npm package into the npm package bundle.
// These are needed at runtime by the goleo binary to create the replace directive
// and link the @goleo/bridge npm package.

import { cpSync, existsSync, rmSync } from 'fs'
import { resolve, dirname } from 'path'
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

// Copy bridge npm package (for npm link @goleo/bridge)
cpSync(resolve(PROJECT_ROOT, 'bridge'), resolve(GOLEO_DIR, 'bridge'), { recursive: true })

console.log('[goleo] Go source and bridge bundled in:', GOLEO_DIR)

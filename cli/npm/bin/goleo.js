#!/usr/bin/env node

// Launcher for the `goleo` CLI. The native binary is delivered as an
// os/cpu-specific optional dependency (@goleo/cli-<platform>-<arch>) — npm
// installs only the matching one — and this script execs it, forwarding args,
// stdio, and the exit code. No download or build happens at install time.

import { spawnSync } from 'child_process'
import { existsSync } from 'fs'
import { createRequire } from 'module'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const require = createRequire(import.meta.url)
const __dirname = dirname(fileURLToPath(import.meta.url))
const { platform, arch } = process
const binaryName = platform === 'win32' ? 'goleo.exe' : 'goleo'
const pkgName = `@goleo/cli-${platform}-${arch}`

function findBinary() {
  // 1. The os/cpu-specific package installed via optionalDependencies. Resolved
  //    through Node so it works regardless of hoisting / install layout.
  try {
    return require.resolve(`${pkgName}/${binaryName}`)
  } catch {}

  // 2. Local development build at the repo root (created by scripts/setup.* when
  //    the package is `npm link`ed from a clone).
  const localBinary = resolve(__dirname, '..', '..', '..', binaryName)
  if (existsSync(localBinary)) return localBinary

  // 3. A binary bundled next to this launcher (manual placement).
  const bundled = resolve(__dirname, binaryName)
  if (existsSync(bundled)) return bundled

  return null
}

const binary = findBinary()
if (!binary) {
  console.error(`[goleo] no prebuilt binary found for ${platform}-${arch}.`)
  console.error(`[goleo] the platform package (${pkgName}) was not installed.`)
  console.error('[goleo] reinstall without skipping optional deps, or build from source:')
  console.error('  go install github.com/daforester/goleo/cli/goleo@latest')
  process.exit(1)
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' })
if (result.error) {
  console.error(`[goleo] failed to run binary: ${result.error.message}`)
  process.exit(1)
}
process.exit(result.status ?? 1)

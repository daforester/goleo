#!/usr/bin/env node

import { spawnSync } from 'child_process'
import { existsSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const platform = process.platform
const arch = process.arch

const binaryName = platform === 'win32' ? 'goleo.exe' : 'goleo'

// Look for binary: local build, platform-specific package, or PATH
function findBinary() {
  // 1. Local development build
  const localBinary = resolve(__dirname, '..', '..', '..', binaryName)
  if (existsSync(localBinary)) return localBinary

  // 2. Platform-specific npm package
  const pkgName = `@goleo/cli-${platform}-${arch}`
  try {
    const pkgPath = resolve(__dirname, '..', 'node_modules', pkgName, binaryName)
    if (existsSync(pkgPath)) return pkgPath
  } catch {}

  // 3. Next to this script (shipped with package)
  const bundledBinary = resolve(__dirname, binaryName)
  if (existsSync(bundledBinary)) return bundledBinary

  return null
}

const binary = findBinary()
if (!binary) {
  console.error(`[goleo] binary not found for ${platform}-${arch}`)
  console.error('[goleo] install the matching platform package:')
  console.error(`  npm install @goleo/cli-${platform}-${arch}`)
  process.exit(1)
}

const args = process.argv.slice(2)
const result = spawnSync(binary, args, { stdio: 'inherit' })
process.exit(result.status ?? 1)

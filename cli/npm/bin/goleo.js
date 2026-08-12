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

// Tracks which of the three lookup strategies findBinary() used, so the
// version-mismatch check below only applies to the optionalDependency case
// (a local dev build or a manually bundled binary has no version to compare).
let resolvedFrom = null

function findBinary() {
  // 1. A local development build in the goleo repo itself (scripts/setup.* builds it
  //    at the repo root and `npm link`s this package). This is checked FIRST, and the
  //    go.mod test is what makes that safe.
  //
  //    Why first: cli/npm declares the platform packages as optionalDependencies, so a
  //    workspace `npm install` — which scripts/setup.* runs — pulls one into the repo's
  //    own node_modules at whatever version package-lock.json pins. That pin lags the
  //    working tree (it sat at 0.9.1 while the tree was 0.10.12), so resolving the
  //    platform package first meant a dev's freshly built binary was shadowed by a
  //    months-old published one, and the version guard below then refused to run at all.
  //
  //    Requiring go.mod alongside the binary keeps this from ever firing for an end
  //    user: a published install has no go.mod three levels up from bin/, so this falls
  //    straight through to the platform package.
  const repoRoot = resolve(__dirname, '..', '..', '..')
  const localBinary = resolve(repoRoot, binaryName)
  if (existsSync(localBinary) && existsSync(resolve(repoRoot, 'go.mod'))) {
    resolvedFrom = 'local-dev-build'
    return localBinary
  }

  // 2. The os/cpu-specific package installed via optionalDependencies — the end-user
  //    path. Resolved through Node so it works regardless of hoisting / install layout.
  try {
    const binary = require.resolve(`${pkgName}/${binaryName}`)
    resolvedFrom = 'platform-package'
    return binary
  } catch {}

  // 3. A binary bundled next to this launcher (manual placement).
  const bundled = resolve(__dirname, binaryName)
  if (existsSync(bundled)) {
    resolvedFrom = 'bundled'
    return bundled
  }

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

// Guard against a stale optionalDependency: npm can leave the os/cpu-specific
// platform package (an independently-versioned real dependency) behind an
// older release while @goleo/cli's own package.json — and `npm list` — already
// report the new version (seen in practice with workspace-linked/partial
// installs). That silently runs an old native binary against a project
// vendored by a newer CLI, which corrupts go.mod: the old binary's
// ensureGoleoRequire happily re-pins the require to ITS OWN (older) version,
// since that old tag always resolves successfully, leaving go.mod and the
// committed vendor/ disagreeing ("inconsistent vendoring").
if (resolvedFrom === 'platform-package') {
  try {
    const cliVersion = require('../package.json').version
    const platformVersion = require(`${pkgName}/package.json`).version
    if (cliVersion !== platformVersion && !process.env.GOLEO_ALLOW_VERSION_MISMATCH) {
      console.error(`[goleo] version mismatch: @goleo/cli is ${cliVersion} but the installed`)
      console.error(`[goleo] native binary (${pkgName}) is ${platformVersion}.`)
      console.error('[goleo] this usually means a partial/stale npm install left the platform')
      console.error('[goleo] package behind. Reinstall to fix it:')
      console.error(`  npm install -g @goleo/cli@${cliVersion}`)
      console.error('[goleo] In the goleo repo itself, reinstalling does NOT help: the lockfile')
      console.error('[goleo] pins the platform package, so npm install restores the same old')
      console.error('[goleo] binary. Build locally instead and let it take precedence:')
      console.error('[goleo]   scripts/setup.ps1   (or setup.sh)')
      console.error('[goleo] Set GOLEO_ALLOW_VERSION_MISMATCH=1 to run anyway.')
      process.exit(1)
    }
  } catch {}
}

const result = spawnSync(binary, process.argv.slice(2), { stdio: 'inherit' })
if (result.error) {
  console.error(`[goleo] failed to run binary: ${result.error.message}`)
  process.exit(1)
}
process.exit(result.status ?? 1)

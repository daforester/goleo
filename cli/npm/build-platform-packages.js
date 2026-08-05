#!/usr/bin/env node

// Builds the per-platform binary packages for @goleo/cli.
//
// For each supported OS/CPU it cross-compiles the Go CLI (CGO_ENABLED=0, so this
// runs from any one machine) and writes cli/npm/packages/cli-<os>-<arch>/ with:
//   - package.json  (name @goleo/cli-<os>-<arch>, os/cpu fields, matching version)
//   - the goleo[.exe] binary
//   - README.md
//
// It also re-stamps @goleo/cli's own optionalDependencies to the current version
// so the main package and its platform packages always publish in lockstep.
//
// Run before publishing (the release workflow does this):  node build-platform-packages.js

import { execSync } from 'child_process'
import { mkdirSync, rmSync, writeFileSync, chmodSync, readFileSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PROJECT_ROOT = resolve(__dirname, '..', '..')
const PACKAGES_DIR = resolve(__dirname, 'packages')
const mainPkgPath = resolve(__dirname, 'package.json')
const mainPkg = JSON.parse(readFileSync(mainPkgPath, 'utf8'))
const VERSION = mainPkg.version

// node process.platform / process.arch  <->  GOOS / GOARCH
const targets = [
  { os: 'darwin', cpu: 'arm64', goos: 'darwin', goarch: 'arm64', ext: '' },
  { os: 'darwin', cpu: 'x64', goos: 'darwin', goarch: 'amd64', ext: '' },
  { os: 'linux', cpu: 'arm64', goos: 'linux', goarch: 'arm64', ext: '' },
  { os: 'linux', cpu: 'x64', goos: 'linux', goarch: 'amd64', ext: '' },
  { os: 'win32', cpu: 'arm64', goos: 'windows', goarch: 'arm64', ext: '.exe' },
  { os: 'win32', cpu: 'x64', goos: 'windows', goarch: 'amd64', ext: '.exe' },
]

rmSync(PACKAGES_DIR, { recursive: true, force: true })
mkdirSync(PACKAGES_DIR, { recursive: true })

const optionalDependencies = {}

for (const t of targets) {
  const pkgName = `@goleo/cli-${t.os}-${t.cpu}`
  const dir = resolve(PACKAGES_DIR, `cli-${t.os}-${t.cpu}`)
  const binaryName = `goleo${t.ext}`
  mkdirSync(dir, { recursive: true })

  console.log(`Building ${pkgName} (${t.goos}/${t.goarch})...`)
  execSync(`go build -ldflags "-s -w -X github.com/daforester/goleo/cli/cmd.Version=${VERSION}" -o ${JSON.stringify(resolve(dir, binaryName))} ./cli/goleo/`, {
    cwd: PROJECT_ROOT,
    env: { ...process.env, GOOS: t.goos, GOARCH: t.goarch, CGO_ENABLED: '0' },
    stdio: 'inherit',
  })
  if (t.ext === '') {
    try { chmodSync(resolve(dir, binaryName), 0o755) } catch {}
  }

  const pkg = {
    name: pkgName,
    version: VERSION,
    description: `Prebuilt goleo CLI binary for ${t.os} ${t.cpu}.`,
    license: 'MIT',
    homepage: 'https://github.com/daforester/goleo#readme',
    repository: {
      type: 'git',
      url: 'git+https://github.com/daforester/goleo.git',
      directory: 'cli/npm',
    },
    os: [t.os],
    cpu: [t.cpu],
    // No "exports" field on purpose: @goleo/cli's launcher resolves the binary
    // via require.resolve('<pkg>/goleo[.exe]'), which needs unrestricted subpaths.
    files: [binaryName, 'README.md'],
    publishConfig: { access: 'public' },
  }
  writeFileSync(resolve(dir, 'package.json'), JSON.stringify(pkg, null, 2) + '\n')
  writeFileSync(
    resolve(dir, 'README.md'),
    `# ${pkgName}\n\nPrebuilt \`goleo\` CLI binary for **${t.os} ${t.cpu}**.\n\n` +
      'This is an internal platform package for [`@goleo/cli`](https://www.npmjs.com/package/@goleo/cli); ' +
      'install that instead:\n\n```bash\nnpm install -g @goleo/cli\n```\n',
  )

  optionalDependencies[pkgName] = VERSION
  console.log(`  -> ${dir}`)
}

// Keep @goleo/cli's optionalDependencies locked to this version + platform set.
mainPkg.optionalDependencies = optionalDependencies
writeFileSync(mainPkgPath, JSON.stringify(mainPkg, null, 2) + '\n')

console.log(`\nBuilt ${targets.length} platform packages at ${VERSION} in ${PACKAGES_DIR}`)
console.log('Synced @goleo/cli optionalDependencies to', VERSION)

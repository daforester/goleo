// Builds @goleo/bridge and copies its output next to the spike's page, so the
// webview loads the ACTUAL package build rather than a copy that can drift.
//
// No bundler is involved on purpose. If `tsc` ever emits imports a browser cannot
// resolve (it used to: extensionless './bridge'), the page fails to load and this
// spike catches it — which is a real defect in the published package, not a
// problem with the test harness.
import { execSync } from 'node:child_process'
import { cpSync, existsSync, mkdirSync, readdirSync, rmSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const bridge = join(here, '..', '..', 'bridge')
const dest = join(here, 'frontend', 'dist', 'bridge')

// Make sure the bridge's own toolchain is present before asking it to build.
//
// This is not hypothetical tidiness: the first CI run of this spike passed on
// ubuntu-latest and failed on windows-latest and macos-14 with "'tsc' is not
// recognized". The Ubuntu runner image happens to ship a global typescript, the
// others do not — so the step was quietly depending on a runner-provided global.
// Installing here fixes both jobs and means a fresh clone can run the spike
// without knowing to install the bridge's devDependencies first.
if (!existsSync(join(bridge, 'node_modules', 'typescript'))) {
  console.log('installing @goleo/bridge devDependencies…')
  // bridge is an npm workspace of the repo root, and `npm ci` from inside a
  // workspace directory does not always accept that; fall back the way ci.yml does.
  try {
    execSync('npm ci', { cwd: bridge, stdio: 'inherit' })
  } catch {
    execSync('npm install', { cwd: bridge, stdio: 'inherit' })
  }
}

console.log('building @goleo/bridge…')
execSync('npm run build', { cwd: bridge, stdio: 'inherit' })

const src = join(bridge, 'dist')
if (!existsSync(join(src, 'index.js'))) {
  console.error('no bridge/dist/index.js — the build produced nothing')
  process.exit(1)
}

rmSync(dest, { recursive: true, force: true })
mkdirSync(dest, { recursive: true })
// Only the runtime JS. Source maps and .d.ts files are irrelevant to the page and
// the .map references would 404 noisily over the custom scheme.
for (const f of readdirSync(src).filter((f) => f.endsWith('.js'))) {
  cpSync(join(src, f), join(dest, f))
}

console.log(`copied ${readdirSync(dest).length} modules -> frontend/dist/bridge/`)

// Rebuild the binary, ALWAYS, as part of preparing.
//
// main.go embeds frontend/dist with //go:embed, so the page is baked into the
// executable at compile time. Copying new JS here without recompiling leaves the
// old page inside the old binary and the run silently verifies stale code. That
// mistake is very easy to make — it fooled the author three times while
// mutation-testing this spike, each time looking like a pass. Doing the Go build
// here makes it structurally impossible rather than a thing to remember.
console.log('building the spike (the page is //go:embed-ed, so this must follow the copy)…')
execSync('go build -o bridge-e2e-verify' + (process.platform === 'win32' ? '.exe' : '') + ' .', {
  cwd: here,
  stdio: 'inherit',
})
console.log('ready — run ./bridge-e2e-verify')

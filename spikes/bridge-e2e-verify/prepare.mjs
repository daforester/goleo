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

#!/usr/bin/env node

// Publishes every generated per-platform binary package (cli/npm/packages/*).
// Run `node build-platform-packages.js` first. Publish these BEFORE @goleo/cli
// so its optionalDependencies resolve for installers. Honors npm auth from the
// environment (NODE_AUTH_TOKEN / ~/.npmrc). Pass --dry-run to preview.

import { readdirSync, existsSync } from 'fs'
import { execSync } from 'child_process'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const dir = resolve(__dirname, 'packages')
const dryRun = process.argv.includes('--dry-run')

if (!existsSync(dir)) {
  console.error('[goleo] cli/npm/packages not found — run: node build-platform-packages.js')
  process.exit(1)
}

const pkgs = readdirSync(dir, { withFileTypes: true }).filter((d) => d.isDirectory())
if (pkgs.length === 0) {
  console.error('[goleo] no platform packages found in', dir)
  process.exit(1)
}

for (const d of pkgs) {
  const p = resolve(dir, d.name)
  console.log(`[goleo] publishing ${d.name}${dryRun ? ' (dry-run)' : ''}...`)
  execSync(`npm publish --access public${dryRun ? ' --dry-run' : ''}`, { cwd: p, stdio: 'inherit' })
}

console.log(`[goleo] published ${pkgs.length} platform packages.`)

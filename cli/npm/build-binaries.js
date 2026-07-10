#!/usr/bin/env node

// Pre-publish script: builds Go binaries for all platforms
// Run manually before publishing: node build-binaries.js

import { execSync } from 'child_process'
import { copyFileSync, existsSync, mkdirSync, chmodSync } from 'fs'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PROJECT_ROOT = resolve(__dirname, '..', '..')
const OUTPUT_DIR = resolve(__dirname, 'prebuilt')

const targets = [
  { goos: 'windows', goarch: 'amd64', ext: '.exe' },
  { goos: 'windows', goarch: 'arm64', ext: '.exe' },
  { goos: 'linux', goarch: 'amd64', ext: '' },
  { goos: 'linux', goarch: 'arm64', ext: '' },
  { goos: 'darwin', goarch: 'amd64', ext: '' },
  { goos: 'darwin', goarch: 'arm64', ext: '' },
]

mkdirSync(OUTPUT_DIR, { recursive: true })

for (const t of targets) {
  const binaryName = `goleo${t.ext}`
  const outName = `${binaryName}-${t.goos}-${t.goarch}`

  console.log(`Building ${t.goos}/${t.goarch}...`)
  execSync('go build -o ' + resolve(OUTPUT_DIR, outName) + ' ./cli/goleo/', {
    cwd: PROJECT_ROOT,
    env: { ...process.env, GOOS: t.goos, GOARCH: t.goarch, CGO_ENABLED: '0' },
    stdio: 'inherit',
  })

  const outputPath = resolve(OUTPUT_DIR, outName)
  if (existsSync(outputPath) && t.goos !== 'windows') {
    try { chmodSync(outputPath, 0o755) } catch {}
  }
  console.log(`  -> ${outName}`)
}

console.log('\nAll binaries built in:', OUTPUT_DIR)

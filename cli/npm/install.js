#!/usr/bin/env node

import { createWriteStream, existsSync, mkdirSync, chmodSync, unlinkSync } from 'fs'
import { get } from 'https'
import { execSync } from 'child_process'
import { resolve, dirname } from 'path'
import { fileURLToPath } from 'url'
import { platform, arch } from 'process'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PKG_VERSION = process.env.npm_package_version || '0.1.0'
const REPO = 'daforester/goleo'
const BIN_DIR = resolve(__dirname, 'bin')

const platformMap = {
  'win32-x64':    { goos: 'windows', goarch: 'amd64', ext: '.exe' },
  'win32-arm64':  { goos: 'windows', goarch: 'arm64', ext: '.exe' },
  'linux-x64':    { goos: 'linux',   goarch: 'amd64', ext: '' },
  'linux-arm64':  { goos: 'linux',   goarch: 'arm64', ext: '' },
  'darwin-x64':   { goos: 'darwin',  goarch: 'amd64', ext: '' },
  'darwin-arm64': { goos: 'darwin',  goarch: 'arm64', ext: '' },
}

const key = `${platform}-${arch}`
const target = platformMap[key]
const binaryName = target ? `goleo${target.ext}` : 'goleo'
const outputPath = resolve(BIN_DIR, binaryName)

if (existsSync(outputPath)) {
  process.exit(0)
}

mkdirSync(BIN_DIR, { recursive: true })

async function download() {
  if (!target) throw new Error(`unsupported platform: ${key}`)

  const downloadUrl = `https://github.com/${REPO}/releases/download/v${PKG_VERSION}/goleo-${target.goos}-${target.goarch}`
  console.log(`[goleo] downloading binary for ${key}...`)

  await new Promise((resolve, reject) => {
    const file = createWriteStream(outputPath)
    const request = get(downloadUrl, (response) => {
      const url = (response.statusCode >= 300 && response.statusCode < 400 && response.headers.location)
        ? response.headers.location
        : null

      if (url) {
        get(url, (r) => {
          if (r.statusCode !== 200) {
            reject(new Error(`HTTP ${r.statusCode}`))
            return
          }
          r.pipe(file)
          file.on('finish', () => { file.close(); resolve() })
        }).on('error', reject)
        return
      }

      if (response.statusCode !== 200) {
        reject(new Error(`HTTP ${response.statusCode}`))
        return
      }
      response.pipe(file)
      file.on('finish', () => { file.close(); resolve() })
    })
    request.on('error', reject)
    request.setTimeout(30000, () => { request.destroy(); reject(new Error('timeout')) })
  })

  try { chmodSync(outputPath, 0o755) } catch {}
  console.log(`[goleo] binary downloaded: ${outputPath}`)
}

function buildFromSource() {
  console.log(`[goleo] compiling from Go source...`)

  const modulePath = 'github.com/daforester/goleo/cli/goleo'
  execSync(`go install ${modulePath}@v${PKG_VERSION}`, { stdio: 'inherit' })

  const goBin = execSync('go env GOPATH', { encoding: 'utf8' }).trim()
  const goosBin = target ? `goleo${target.ext}` : 'goleo'
  const sourceBinary = resolve(goBin, 'bin', goosBin)

  if (!existsSync(sourceBinary)) {
    throw new Error('binary not found after go install')
  }

  copyFileSync(sourceBinary, outputPath)
  try { chmodSync(outputPath, 0o755) } catch {}
  console.log(`[goleo] compiled binary: ${outputPath}`)
}

async function main() {
  // Attempt 1: download pre-built binary
  try {
    await download()
    return
  } catch (err) {
    console.warn(`[goleo] download failed: ${err.message}`)
  }

  // Attempt 2: compile from Go source
  try {
    buildFromSource()
    return
  } catch (err) {
    console.warn(`[goleo] source build failed: ${err.message}`)
  }

  console.error('[goleo] could not obtain CLI binary. Install manually:')
  console.error('  go install github.com/daforester/goleo/cli/goleo@latest')
  process.exit(1)
}

main()

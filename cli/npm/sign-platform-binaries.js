#!/usr/bin/env node

// Authenticode-signs the Windows platform binaries produced by
// build-platform-packages.js.
//
// Unsigned Windows executables get flagged. Not hypothetically: Bitdefender
// quarantined @goleo/cli-win32-x64's binary straight out of a user's
// node_modules, leaving `goleo` reporting "no prebuilt binary found". An
// Authenticode signature from a real code-signing certificate is what stops
// reputation-based heuristics treating a fresh Go binary as suspicious.
//
// Signing happens here rather than through runtime/signing.go's signWindows
// because that path shells out to signtool.exe, which only exists on Windows,
// and the release workflow cross-compiles every target from one Linux runner.
// osslsigncode produces the same Authenticode signature on Linux.
//
// No certificate configured => this is a no-op with a notice, so the release
// still succeeds. Set both to enable it:
//
//   WINDOWS_CERT_BASE64    base64 of the .pfx/.p12 code-signing certificate
//   WINDOWS_CERT_PASSWORD  its password
//
// See RELEASING.md.

import { execFileSync } from 'child_process'
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'fs'
import { tmpdir } from 'os'
import { dirname, resolve } from 'path'
import { fileURLToPath } from 'url'

const __dirname = dirname(fileURLToPath(import.meta.url))
const PACKAGES_DIR = resolve(__dirname, 'packages')
const VERSION = JSON.parse(readFileSync(resolve(__dirname, 'package.json'), 'utf8')).version

// Both Windows targets built by build-platform-packages.js.
const WINDOWS_BINARIES = ['cli-win32-x64', 'cli-win32-arm64'].map((d) =>
  resolve(PACKAGES_DIR, d, 'goleo.exe'),
)

// RFC 3161 timestamping, so signatures stay valid after the certificate
// expires. Without it every released binary silently goes untrusted on the
// cert's expiry date.
const TIMESTAMP_URL = 'http://timestamp.digicert.com'

const certB64 = process.env.WINDOWS_CERT_BASE64
const certPass = process.env.WINDOWS_CERT_PASSWORD

if (!certB64 || !certPass) {
  console.log('  Windows code signing skipped: WINDOWS_CERT_BASE64 / WINDOWS_CERT_PASSWORD not set.')
  console.log('  The release continues unsigned — see RELEASING.md to enable signing.')
  process.exit(0)
}

let workDir
try {
  workDir = mkdtempSync(resolve(tmpdir(), 'goleo-sign-'))
  const certPath = resolve(workDir, 'cert.pfx')
  const passPath = resolve(workDir, 'pass.txt')
  // The password goes in a file read by -readpass rather than on the command
  // line, where it would be visible to anything that can list processes.
  writeFileSync(certPath, Buffer.from(certB64, 'base64'), { mode: 0o600 })
  writeFileSync(passPath, certPass, { mode: 0o600 })

  for (const binary of WINDOWS_BINARIES) {
    if (!existsSync(binary)) {
      throw new Error(`expected binary not found: ${binary} (run build-platform-packages.js first)`)
    }
    // osslsigncode cannot sign in place.
    const signed = `${binary}.signed`
    console.log(`  Signing ${binary}...`)
    execFileSync(
      'osslsigncode',
      [
        'sign',
        '-pkcs12', certPath,
        '-readpass', passPath,
        '-n', 'Goleo CLI',
        '-i', 'https://github.com/daforester/goleo',
        '-t', TIMESTAMP_URL,
        '-h', 'sha256',
        '-in', binary,
        '-out', signed,
      ],
      { stdio: 'inherit' },
    )
    rmSync(binary)
    execFileSync('mv', [signed, binary])
  }

  console.log(`  Signed ${WINDOWS_BINARIES.length} Windows binaries for v${VERSION}.`)
} finally {
  // Remove the certificate and password even if signing threw.
  if (workDir) rmSync(workDir, { recursive: true, force: true })
}

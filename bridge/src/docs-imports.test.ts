import { readdirSync, readFileSync, statSync } from 'node:fs'
import { join, relative } from 'node:path'

import { describe, expect, it } from 'vitest'

import * as bridge from './index'

// Every name the docs import from '@goleo/bridge' must actually be exported.
//
// The docs shipped several that were not. README and the host-features guide imported
// `readText`/`writeText` (exported as `clipboardReadText`/`clipboardWriteText`),
// `connect`/`disconnect` (exported as `bleConnect`/`bleDisconnect`) and
// `subscribe`/`unsubscribe`/`getSubscription` (exported as `push*`). The internal names
// exist in the source modules, which is how they got written down, but index.ts renames
// them on the way out — so copying any of those samples gives you an immediate
// "does not provide an export named" error.
//
// Fixing the four instances does not stop the fifth, hence this: it reads the actual
// markdown and checks it against the actual exports, so a sample and the API cannot
// drift apart again.

const REPO_ROOT = join(__dirname, '..', '..')

function markdownFiles(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    // Skip vendored and generated trees — their markdown is not ours to police.
    if (['node_modules', '.git', 'vendor', 'dist', 'pkg', 'spikes'].includes(entry)) continue
    const full = join(dir, entry)
    const st = statSync(full)
    if (st.isDirectory()) markdownFiles(full, out)
    else if (entry.endsWith('.md')) out.push(full)
  }
  return out
}

/** Named imports from '@goleo/bridge' in a markdown file, with the line for reporting. */
function bridgeImports(md: string): Array<{ names: string[]; line: string }> {
  const out: Array<{ names: string[]; line: string }> = []
  // `import { a, b as c } from '@goleo/bridge'` — the form every sample uses.
  const re = /import\s*\{([^}]*)\}\s*from\s*['"]@goleo\/bridge['"]/g
  let m: RegExpExecArray | null
  while ((m = re.exec(md)) !== null) {
    const names = m[1]
      .split(',')
      .map((n) => n.trim())
      // `type Foo` and `a as b`: the imported name is the first token.
      .map((n) => n.replace(/^type\s+/, '').split(/\s+as\s+/)[0].trim())
      .filter((n) => n.length > 0)
    out.push({ names, line: m[0].replace(/\s+/g, ' ') })
  }
  return out
}

describe('documented @goleo/bridge imports', () => {
  const files = markdownFiles(REPO_ROOT)

  it('finds markdown to check, so a silent zero-file pass is impossible', () => {
    expect(files.length).toBeGreaterThan(5)
    const withImports = files.filter((f) => bridgeImports(readFileSync(f, 'utf8')).length > 0)
    expect(withImports.length).toBeGreaterThan(0)
  })

  it('only imports names the package actually exports', () => {
    // Types are erased at runtime, so they cannot be checked against the module object.
    // They are listed here rather than skipped silently, so an unknown type name still
    // shows up as a failure to think about.
    const knownTypes = new Set([
      'OSInfo', 'PlatformInfo', 'BatteryInfo', 'BLEDevice', 'FileEntry', 'NFCTag',
      'Position', 'SensorReading', 'ShareData', 'UpdateInfo', 'MenuItem',
      'WindowOptions', 'Capabilities', 'BridgeConfig', 'DialogFilter',
      'MessageBoxOptions', 'PromptOptions', 'PushSubscription', 'StoreValue',
    ])
    const exported = new Set(Object.keys(bridge))
    const problems: string[] = []

    for (const file of files) {
      const md = readFileSync(file, 'utf8')
      for (const { names, line } of bridgeImports(md)) {
        for (const name of names) {
          if (exported.has(name) || knownTypes.has(name)) continue
          problems.push(`${relative(REPO_ROOT, file)}: "${name}" is not exported — in: ${line}`)
        }
      }
    }

    expect(problems, problems.join('\n')).toEqual([])
  })
})

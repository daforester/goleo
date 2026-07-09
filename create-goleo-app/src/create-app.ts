import { execSync } from 'child_process'
import {
  existsSync,
  mkdirSync,
  readdirSync,
  readFileSync,
  writeFileSync,
} from 'fs'
import { dirname, join, relative, resolve } from 'path'
import { fileURLToPath } from 'url'

// Placeholder replaced with the project name throughout the template files.
const TOKEN = /__GOLEO_APP_NAME__/g

// Template files whose on-disk name must change when scaffolded. npm will not
// publish a file literally named `.gitignore`, so it lives as `gitignore`.
const RENAME: Record<string, string> = {
  gitignore: '.gitignore',
}

export async function createApp(projectName: string): Promise<void> {
  const projectDir = resolve(process.cwd(), projectName)

  if (existsSync(projectDir)) {
    throw new Error(`Directory ${projectName} already exists`)
  }

  console.log(`\n  Creating Goleo project: ${projectName}\n`)

  const templateDir = resolve(
    dirname(fileURLToPath(import.meta.url)),
    '..',
    'template',
  )
  if (!existsSync(templateDir)) {
    throw new Error(`template directory not found at ${templateDir}`)
  }

  mkdirSync(projectDir, { recursive: true })
  copyTemplate(templateDir, projectDir, projectName, projectDir)

  // Install dependencies
  const frontendDir = join(projectDir, 'frontend')
  console.log('\n  Installing frontend dependencies...')

  try {
    execSync('npm install', { cwd: frontendDir, stdio: 'inherit' })
  } catch {
    // npm install failed (likely @goleo/bridge not published yet).
    // Try linking from global if available.
    console.log('  npm install failed. Checking for local @goleo/bridge...')
    try {
      execSync('npm link @goleo/bridge', { cwd: frontendDir, stdio: 'inherit' })
      console.log('  @goleo/bridge linked locally — retrying install...')
      execSync('npm install', { cwd: frontendDir, stdio: 'inherit' })
    } catch {
      console.log(`  Warning: could not install frontend dependencies.`)
      console.log(`  Run manually:`)
      console.log(`    cd ${projectName}/frontend`)
      console.log(`    npm link @goleo/bridge`)
      console.log(`    npm install`)
    }
  }

  console.log('\n  Project created successfully!\n')
  console.log('  Next steps:')
  console.log(`    cd ${projectName}`)
  console.log(`    cd frontend && npm link @goleo/bridge && npm install && cd ..`)
  console.log('    goleo dev')
  console.log('    goleo build')
  console.log('    goleo emulate android\n')
}

// copyTemplate recursively copies srcDir into destDir, replacing the app-name
// token in every file and applying the RENAME map (e.g. gitignore -> .gitignore).
function copyTemplate(
  srcDir: string,
  destDir: string,
  appName: string,
  projectRoot: string,
): void {
  for (const entry of readdirSync(srcDir, { withFileTypes: true })) {
    const srcPath = join(srcDir, entry.name)
    const destPath = join(destDir, RENAME[entry.name] ?? entry.name)

    if (entry.isDirectory()) {
      mkdirSync(destPath, { recursive: true })
      copyTemplate(srcPath, destPath, appName, projectRoot)
    } else {
      const content = readFileSync(srcPath, 'utf-8').replace(TOKEN, appName)
      writeFileSync(destPath, content, 'utf-8')
      console.log(`  created ${relative(projectRoot, destPath)}`)
    }
  }
}

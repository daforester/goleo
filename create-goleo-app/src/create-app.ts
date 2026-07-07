import { execSync } from 'child_process'
import { existsSync, mkdirSync, writeFileSync } from 'fs'
import { join, resolve } from 'path'

interface ProjectFiles {
  [relativePath: string]: string
}

export async function createApp(projectName: string): Promise<void> {
  const projectDir = resolve(process.cwd(), projectName)

  if (existsSync(projectDir)) {
    throw new Error(`Directory ${projectName} already exists`)
  }

  console.log(`\n  Creating Goleo project: ${projectName}\n`)

  // Create directory structure
  const dirs = [
    'backend',
    'frontend/src',
    'frontend/public',
  ]

  for (const dir of dirs) {
    mkdirSync(join(projectDir, dir), { recursive: true })
    console.log(`  created ${projectName}/${dir}/`)
  }

  // Write all project files
  const files: ProjectFiles = getProjectFiles(projectName)
  for (const [relPath, content] of Object.entries(files)) {
    const fullPath = join(projectDir, relPath)
    writeFileSync(fullPath, content, 'utf-8')
    console.log(`  created ${projectName}/${relPath}`)
  }

  // Install dependencies
  console.log('\n  Installing frontend dependencies...')
  try {
    execSync('npm install', {
      cwd: join(projectDir, 'frontend'),
      stdio: 'inherit',
    })
  } catch {
    console.log('  Warning: npm install failed. Run manually: cd frontend && npm install')
  }

  console.log('\n  Project created successfully!\n')
  console.log('  Next steps:')
  console.log(`    cd ${projectName}`)
  console.log('    goleo dev')
  console.log('    goleo build\n')
}

function getProjectFiles(name: string): ProjectFiles {
  return {
    'package.json': JSON.stringify({
      name,
      private: true,
      scripts: {
        'goleo:dev': 'goleo dev',
        'goleo:build': 'goleo build',
        'goleo:build-windows': 'goleo build windows',
        'goleo:build-linux': 'goleo build linux',
        'goleo:build-darwin': 'goleo build darwin',
        'goleo:build-android': 'goleo build android',
        'goleo:build-ios': 'goleo build ios',
      },
    }, null, 2) + '\n',

    'goleo.json': JSON.stringify({
      version: '0.1.0',
      app_name: name,
      frontend: {
        directory: 'frontend',
        build_command: 'npm run build',
        dev_command: 'npm run dev',
        dist_dir: 'dist',
      },
      backend: {
        directory: 'backend',
        main_file: 'main.go',
      },
      mobile: {
        android: {
          min_sdk: 24,
          package_name: `com.${name}.app`,
        },
        ios: {
          deployment_target: '14.0',
          bundle_identifier: `com.${name}.app`,
        },
      },
    }, null, 2) + '\n',

    'backend/go.mod': `module goleo/${name}\n\ngo 1.26\n\nrequire github.com/daforester/goleo v0.1.0\n`,

    'backend/main.go': `package main

import (
	"context"
	"embed"
	"log"

	"goleo/${name}/backend/commands"
	"github.com/daforester/goleo/runtime"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	app := runtime.New(runtime.Config{
		Title:    "${name}",
		Width:    1024,
		Height:   768,
		EmbedFS:  frontendFS,
		OnStartup: func(ctx context.Context) {
			log.Println("${name} starting up...")
			runtime.RegisterBuiltins(app.Bridge())
			commands.Register(app.Bridge())
		},
		OnShutdown: func(ctx context.Context) {
			log.Println("${name} shutting down...")
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
`,

    'backend/commands.go': `package commands

import (
	"context"
	"encoding/json"

	"github.com/daforester/goleo/runtime"
)

func Register(b *runtime.Bridge) {
	b.Handle("greet", func(ctx context.Context, args json.RawMessage) (any, error) {
		var params map[string]string
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, err
		}
		name := params["name"]
		if name == "" {
			name = "World"
		}
		return map[string]string{
			"message": "Hello, " + name + "! From Go backend.",
		}, nil
	})

	b.Handle("getVersion", func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]string{
			"version": "1.0.0",
		}, nil
	})
}
`,

    'frontend/package.json': JSON.stringify({
      name: `${name}-frontend`,
      private: true,
      version: '0.0.0',
      type: 'module',
      scripts: {
        dev: 'vite',
        build: 'vite build',
        preview: 'vite preview',
      },
      dependencies: {
        vue: '^3.4.0',
        '@goleo/bridge': '^0.1.0',
      },
      devDependencies: {
        '@vitejs/plugin-vue': '^5.0.0',
        typescript: '^5.3.0',
        vite: '^5.0.0',
        'vue-tsc': '^1.8.0',
      },
    }, null, 2) + '\n',

    'frontend/index.html': `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>${name}</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
`,

    'frontend/vite.config.ts': `import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

export default defineConfig({
  plugins: [vue()],
  server: {
    proxy: {
      '/api': 'http://localhost:9842',
      '/ws': {
        target: 'ws://localhost:9842',
        ws: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
`,

    'frontend/tsconfig.json': JSON.stringify({
      compilerOptions: {
        target: 'ES2020',
        module: 'ESNext',
        lib: ['ES2020', 'DOM', 'DOM.Iterable'],
        skipLibCheck: true,
        moduleResolution: 'bundler',
        allowImportingTsExtensions: true,
        resolveJsonModule: true,
        isolatedModules: true,
        noEmit: true,
        jsx: 'preserve',
        strict: true,
        noUnusedLocals: true,
        noUnusedParameters: true,
        noFallthroughCasesInSwitch: true,
      },
      include: ['src/**/*.ts', 'src/**/*.tsx', 'src/**/*.vue', 'env.d.ts'],
      references: [{ path: './tsconfig.node.json' }],
    }, null, 2) + '\n',

    'frontend/env.d.ts': `/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
`,

    'frontend/src/main.ts': `import { createApp } from 'vue'
import { initBridge } from '@goleo/bridge'
import App from './App.vue'
import './style.css'

async function main() {
  await initBridge()

  const app = createApp(App)
  app.mount('#app')
}

main()
`,

    'frontend/src/App.vue': `<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { invoke, getOSInfo, on } from '@goleo/bridge'

const message = ref('Loading...')
const osInfo = ref('')
const backendMessage = ref('')

onMounted(async () => {
  try {
    const info = await getOSInfo()
    osInfo.value = JSON.stringify(info, null, 2)

    const result = await invoke('greet', { name: 'Goleo' })
    backendMessage.value = result.message
  } catch (err) {
    message.value = 'Error: ' + err
  }
})

on('backend:ready', () => {
  console.log('Backend is ready')
})
</script>

<template>
  <div class="container">
    <h1>{{ message }}</h1>
    <p>OS Info:</p>
    <pre>{{ osInfo }}</pre>
    <p>Backend says: <strong>{{ backendMessage }}</strong></p>
  </div>
</template>
`,

    'frontend/src/style.css': `* {
  margin: 0;
  padding: 0;
  box-sizing: border-box;
}

body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen,
    Ubuntu, Cantarell, sans-serif;
  background-color: #f5f5f5;
  color: #333;
}

.container {
  max-width: 800px;
  margin: 0 auto;
  padding: 2rem;
}

h1 {
  font-size: 2rem;
  margin-bottom: 1rem;
  color: #2c3e50;
}

pre {
  background: #eee;
  padding: 0.5rem;
  border-radius: 4px;
  overflow-x: auto;
}
`,
  }
}

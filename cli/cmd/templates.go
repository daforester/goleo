package cmd

var tmplMainGo = `package main

import (
	"context"
	"embed"
	"log"

	"{{.ModuleName}}/backend/commands"
	"github.com/daforester/goleo/runtime"
)

//go:embed frontend/dist/*
var frontendFS embed.FS

func main() {
	app := runtime.New(runtime.Config{
		Title:  "{{.Name}}",
		Width:  1024,
		Height: 768,
		EmbedFS: frontendFS,
		OnStartup: func(ctx context.Context) {
			log.Println("{{.Name}} starting up...")
			commands.Register(app.Bridge())
			runtime.RegisterBuiltins(app.Bridge())
		},
		OnShutdown: func(ctx context.Context) {
			log.Println("{{.Name}} shutting down...")
		},
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
`

var tmplCommandsGo = `package commands

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
`

var tmplGoMod = `module {{.ModuleName}}

go 1.26

require github.com/daforester/goleo v0.1.0
`

var tmplFrontendPackageJSON = `{
  "name": "{{.Name}}-frontend",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "vue-tsc && vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "@goleo/bridge": "^0.1.0",
    "vue": "^3.4.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0",
    "vue-tsc": "^1.8.0"
  }
}
`

var tmplIndexHTML = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <title>{{.Name}}</title>
  </head>
  <body>
    <div id="app"></div>
    <script type="module" src="/src/main.ts"></script>
  </body>
</html>
`

var tmplViteConfig = `import { defineConfig } from 'vite'
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
`

var tmplTsconfig = `{
  "compilerOptions": {
    "target": "ES2020",
    "useDefineForExpose": true,
    "module": "ESNext",
    "lib": ["ES2020", "DOM", "DOM.Iterable"],
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "resolveJsonModule": true,
    "isolatedModules": true,
    "noEmit": true,
    "jsx": "preserve",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true
  },
  "include": ["src/**/*.ts", "src/**/*.tsx", "src/**/*.vue", "env.d.ts"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
`

var tmplEnvDTS = `/// <reference types="vite/client" />

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  const component: DefineComponent<{}, {}, any>
  export default component
}
`

var tmplMainTS = `import { createApp } from 'vue'
import { initBridge } from '@goleo/bridge'
import App from './App.vue'
import './style.css'

async function main() {
  await initBridge()

  const app = createApp(App)
  app.mount('#app')
}

main()
`

var tmplAppVue = `<script setup lang="ts">
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
    <p>OS Info: <pre>{{ osInfo }}</pre></p>
    <p>Backend says: <strong>{{ backendMessage }}</strong></p>
  </div>
</template>
`

var tmplStyleCSS = `* {
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
`

var tmplRootPackageJSON = `{
  "name": "{{.Name}}",
  "private": true,
  "scripts": {
    "goleo:dev": "goleo dev",
    "goleo:build": "goleo build",
    "goleo:build-windows": "goleo build windows",
    "goleo:build-linux": "goleo build linux",
    "goleo:build-darwin": "goleo build darwin",
    "goleo:build-android": "goleo build android",
    "goleo:build-ios": "goleo build ios"
  },
  "devDependencies": {
    "goleo": "^0.1.0"
  }
}
`

var tmplGoleoJSON = `{
  "version": "0.1.0",
  "app_name": "{{.Name}}",
  "frontend": {
    "directory": "frontend",
    "build_command": "npm run build",
    "dev_command": "npm run dev",
    "dist_dir": "dist"
  },
  "backend": {
    "directory": "backend",
    "main_file": "main.go"
  },
  "mobile": {
    "android": {
      "min_sdk": 24,
      "package_name": "com.{{.Name}}.app"
    },
    "ios": {
      "deployment_target": "14.0",
      "bundle_identifier": "com.{{.Name}}.app"
    }
  }
}
`

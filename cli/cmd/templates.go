package cmd

var tmplMainGo = `package main

import (
	"context"
	"embed"
	"log"
	"os"

	"{{.ModuleName}}/backend/commands"
	"github.com/daforester/goleo/runtime"
)

// Embedded application assets: the built frontend plus the startup script.
// goleo build copies frontend/dist here before compiling.
// If you delete init.js, also remove its embed line below — the app then
// falls back to the window settings in runtime.Config.
//
//go:embed all:frontend/dist
//go:embed init.js
var appFS embed.FS

func main() {
	devMode := os.Getenv("GOLEO_DEV") == "true"

	var app *runtime.App
	app = runtime.New(runtime.Config{
		Title:      "{{.Name}}",
		Width:      1024,
		Height:     768,
		DevMode:    devMode,
		Port:       9842,
		WindowMode: runtime.WindowModeWebview,
		EmbedFS:    appFS,
		// InitJS: "init.js", // custom startup script path (default: init.js, then backend/init.js)
		OnStartup: func(ctx context.Context) {
			log.Println("{{.Name}} starting up...")
			runtime.RegisterBuiltins(app.Bridge())
			commands.Register(app.Bridge())
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

var tmplInitJS = `// init.js — Goleo startup script.
//
// Runs inside the Go backend (embedded JS engine) before any window is
// shown, giving you full control over window creation. Available API:
//
//   getConfig()       -> { title, width, height, devMode, devServer, port, url }
//   createWindow(opts) - opts: title, width, height, minWidth, minHeight,
//                        center, devTools, url (defaults to the app's own URL)
//   console.log/info/warn/error
//
// Delete this file (and its embed line in main.go) to fall back to the
// built-in window setup from runtime.Config.

const config = getConfig()

createWindow({
  title: config.title,
  width: config.width,
  height: config.height,
  center: true,
})
`

var tmplBackendCommandsGo = `package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"time"

	goleoruntime "github.com/daforester/goleo/runtime"
)

func Register(b *goleoruntime.Bridge) {
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
			"message": fmt.Sprintf("Hello, %s! From Go backend at %s.", name, time.Now().Format(time.RFC3339)),
		}, nil
	})

	b.Handle("systemInfo", func(ctx context.Context, args json.RawMessage) (any, error) {
		return map[string]any{
			"goVersion":  runtime.Version(),
			"os":         runtime.GOOS,
			"arch":       runtime.GOARCH,
			"cpus":       runtime.NumCPU(),
			"goroutines": runtime.NumGoroutine(),
		}, nil
	})

	b.Handle("add", func(ctx context.Context, args json.RawMessage) (any, error) {
		var params map[string]float64
		if err := json.Unmarshal(args, &params); err != nil {
			return nil, fmt.Errorf("invalid args: need 'a' and 'b' numbers")
		}
		return map[string]float64{
			"result": params["a"] + params["b"],
		}, nil
	})

	b.Handle("countdown", func(ctx context.Context, args json.RawMessage) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(3 * time.Second):
			return map[string]string{
				"message": "3 seconds have passed! Async works.",
			}, nil
		}
	})

	b.Handle("notify", func(ctx context.Context, args json.RawMessage) (any, error) {
		var params map[string]string
		if err := json.Unmarshal(args, &params); err != nil {
			params = map[string]string{"title": "Notification", "message": string(args)}
		}
		if params["title"] == "" {
			params["title"] = "Goleo"
		}
		if params["message"] == "" {
			params["message"] = "Hello from Go!"
		}
		// Deliver through the OS notification service: toast on Windows,
		// Notification Center on macOS, libnotify on Linux, and the native
		// shell on Android/iOS.
		if err := goleoruntime.Notify(params["title"], params["message"]); err != nil {
			return nil, err
		}
		b.Emit("notification:show", params)
		return map[string]string{"status": "sent"}, nil
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
    "build": "vite build",
    "preview": "vite preview"
  },
  "dependencies": {
    "@goleo/bridge": "^0.1.0",
    "vue": "^3.4.0"
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^5.0.0",
    "typescript": "^5.3.0",
    "vite": "^5.0.0"
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

var tmplTsconfigNode = `{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true
  },
  "include": ["vite.config.ts"]
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
import { invoke, getOSInfo, on, isPermissionGranted, requestPermission } from '@goleo/bridge'

const message = ref('Loading...')
const osInfo = ref('')
const backendMessage = ref('')
const notifyStatus = ref('')
const lastNotification = ref('')
const permStatus = ref('')

onMounted(async () => {
  try {
    const info = await getOSInfo()
    osInfo.value = JSON.stringify(info, null, 2)

    const result = await invoke('greet', { name: 'Goleo' })
    backendMessage.value = result.message

    permStatus.value = (await isPermissionGranted()) ? 'granted' : 'default'
  } catch (err) {
    message.value = 'Error: ' + err
  }
})

on('backend:ready', () => {
  console.log('Backend is ready')
})

on('notification:show', (data: any) => {
  lastNotification.value = data.message || ''
})

async function requestNotifyPermission() {
  permStatus.value = 'requesting'
  const result = await requestPermission()
  permStatus.value = result
}

async function sendNotification() {
  if (!(await isPermissionGranted())) {
    const result = await requestPermission()
    permStatus.value = result
    if (result !== 'granted') {
      notifyStatus.value = 'Permission denied'
      return
    }
  }
  notifyStatus.value = 'Sending...'
  try {
    await invoke('notify', {
      title: 'Hello from Goleo!',
      message: 'This notification was triggered from the Go backend.',
    })
    notifyStatus.value = 'Notification sent!'
  } catch (err) {
    notifyStatus.value = 'Error: ' + err
  }
}
</script>

<template>
  <div class="container">
    <h1>{{"{{"}} message }}</h1>
    <p>OS Info:</p>
    <pre>{{"{{"}} osInfo }}</pre>
    <p>Backend says: <strong>{{"{{"}} backendMessage }}</strong></p>

    <hr />
    <h2>Notifications Demo</h2>
    <p>Permission: <strong>{{"{{"}} permStatus }}</strong></p>
    <button @click="requestNotifyPermission" class="btn">Request Permission</button>
    <button @click="sendNotification" class="btn">Send System Notification</button>
    <p class="status">{{"{{"}} notifyStatus }}</p>
    <p class="status">Last event: {{"{{"}} lastNotification }}</p>
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

h2 {
  font-size: 1.5rem;
  margin: 1.5rem 0 0.5rem;
  color: #2c3e50;
}

pre {
  background: #eee;
  padding: 0.5rem;
  border-radius: 4px;
  overflow-x: auto;
}

hr {
  border: none;
  border-top: 1px solid #ddd;
  margin: 1.5rem 0;
}

.btn {
  display: inline-block;
  padding: 0.6rem 1.2rem;
  background: #42b883;
  color: #fff;
  border: none;
  border-radius: 6px;
  font-size: 1rem;
  cursor: pointer;
}

.btn:hover {
  background: #38a476;
}

.status {
  margin-top: 0.5rem;
  font-size: 0.9rem;
  color: #666;
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
    "goleo:build-ios": "goleo build ios",
    "goleo:emulate": "goleo emulate android",
    "goleo:emulate-ios": "goleo emulate ios"
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
  "mobile": {
    "android": {
      "min_sdk": 24,
      "package_name": "com.goleo.app"
    },
    "ios": {
      "deployment_target": "14.0",
      "bundle_identifier": "com.goleo.app"
    }
  }
}
`

var tmplMobileGo = `//go:build mobilebuild && !goleodev

package gomobile

import (
	"context"
	"embed"

	"{{.ModuleName}}/backend/commands"
	"github.com/daforester/goleo/runtime"
)

//go:embed all:frontend/dist
var frontendFS embed.FS

var app *runtime.App

func StartServer() int {
	app = runtime.New(runtime.Config{
		Title:      "{{.Name}}",
		Width:      1024,
		Height:     768,
		DevMode:    false,
		WindowMode: runtime.WindowModeBrowser,
		EmbedFS:    frontendFS,
		OnStartup: func(ctx context.Context) {
			runtime.RegisterBuiltins(app.Bridge())
			commands.Register(app.Bridge())
		},
	})
	port, err := app.StartServer()
	if err != nil {
		return 0
	}
	return port
}

func StopServer() {
	if app != nil {
		app.Stop()
	}
}
`

var tmplMobileNotifierGo = `//go:build mobilebuild

package gomobile

import "github.com/daforester/goleo/runtime"

// Notifier is implemented by the native shell (Android Activity / iOS
// AppDelegate) to deliver system notifications. gomobile generates
// bindings for this interface.
type Notifier interface {
	Show(title string, body string)
	PermissionGranted() bool
	RequestPermission() string
}

// SetNotifier registers the native notification backend. The shell must
// call this at startup so runtime.Notify can reach the OS notification
// service.
func SetNotifier(n Notifier) {
	if n == nil {
		runtime.SetNativeNotifier(nil)
		return
	}
	runtime.SetNativeNotifier(&notifierAdapter{n: n})
}

type notifierAdapter struct{ n Notifier }

func (a *notifierAdapter) Show(title, body string) error {
	a.n.Show(title, body)
	return nil
}

func (a *notifierAdapter) PermissionGranted() bool { return a.n.PermissionGranted() }

func (a *notifierAdapter) RequestPermission() string { return a.n.RequestPermission() }
`

var tmplMobileDevGo = `//go:build mobilebuild && goleodev

package gomobile

import (
	"context"

	"{{.ModuleName}}/backend/commands"
	"github.com/daforester/goleo/runtime"
)

var app *runtime.App

func StartServer() int {
	app = runtime.New(runtime.Config{
		Title:      "{{.Name}} (dev)",
		Width:      1024,
		Height:     768,
		DevMode:    true,
		WindowMode: runtime.WindowModeBrowser,
		EmbedFS:    nil,
		OnStartup: func(ctx context.Context) {
			runtime.RegisterBuiltins(app.Bridge())
			commands.Register(app.Bridge())
		},
	})
	port, err := app.StartServer()
	if err != nil {
		return 0
	}
	return port
}

func StopServer() {
	if app != nil {
		app.Stop()
	}
}
`

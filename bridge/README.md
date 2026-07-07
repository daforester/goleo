# @goleo/bridge

The frontend bridge library for [Goleo](https://github.com/daforester/goleo) applications.

Provides communication between your web frontend and Go backend.

## Installation

```bash
npm install @goleo/bridge
```

## Usage

```typescript
import { initBridge, invoke, on, getOSInfo } from '@goleo/bridge'

// Initialize the bridge (connects to Go backend)
await initBridge()

// Call Go backend functions
const result = await invoke('greet', { name: 'World' })
console.log(result) // { message: "Hello, World! From Go backend." }

// Listen for backend events
const unsubscribe = on('backend:ready', (data) => {
  console.log('Backend ready:', data)
})

// Use built-in functions
const osInfo = await getOSInfo()
console.log('OS:', osInfo.name, osInfo.arch)

// Cleanup
unsubscribe()
```

## API

- `initBridge(config?)` - Initialize connection to Go backend
- `invoke(method, args?)` - Call a Go backend function
- `on(event, callback)` - Listen for backend events (returns unsubscribe function)
- `off(event, callback)` - Remove event listener
- `getOSInfo()` - Get OS information
- `getPlatformInfo()` - Get platform information
- `getArch()` - Get CPU architecture
- `getEnv(key)` - Get environment variable
- `openURL(url)` - Open URL in default browser
- `disconnect()` - Disconnect from backend
- `isConnected()` - Check connection status

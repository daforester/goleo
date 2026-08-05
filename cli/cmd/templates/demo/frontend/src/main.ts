import { createApp } from 'vue'
import { initBridge } from '@goleo/bridge'
import App from './App.vue'
import './style.css'

async function main() {
  const isPWA = import.meta.env.VITE_GOLEO_PLATFORM === 'pwa'
  await initBridge({ backend: !isPWA })

  if (isPWA && 'serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js')
  }

  const app = createApp(App)
  app.mount('#app')
}

main()

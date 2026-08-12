<script setup lang="ts">
import { computed, defineAsyncComponent } from 'vue'
import { path, navigate } from './router'
import DemosMenu from './demos/DemosMenu.vue'
import { findDemo } from './demos/registry'

// Tiny view resolver driven by the hash router. This is deliberately small so
// you can rip out the demo browser and drop in your own component: just render
// your root component instead of the <template>s below.
const view = computed(() => {
  const p = path.value
  if (p === '/demos') return { kind: 'menu' as const }
  if (p.startsWith('/demos/')) {
    const demo = findDemo(p.slice('/demos/'.length))
    return demo ? { kind: 'demo' as const, demo } : { kind: 'notfound' as const }
  }
  return { kind: 'home' as const }
})

const DemoComponent = computed(() =>
  view.value.kind === 'demo' ? defineAsyncComponent(view.value.demo.load) : null,
)
</script>

<template>
  <header class="topbar">
    <a class="brand" href="#/"><span class="logo">🦁</span> __GOLEO_APP_NAME__</a>
    <a class="btn btn-ghost" href="#/demos">Demos</a>
  </header>

  <main>
    <!-- Landing / opening page -->
    <section v-if="view.kind === 'home'" class="hero">
      <span class="badge-pill">Built with Goleo</span>
      <h1>__GOLEO_APP_NAME__</h1>
      <p>
        A cross-platform app powered by Go and web technology — one codebase
        that ships to desktop, mobile, and the web.
      </p>
      <div class="hero-actions">
        <a class="btn btn-primary" href="#/demos">Explore the demos →</a>
        <a
          class="btn btn-ghost"
          href="https://github.com/daforester/goleo"
          target="_blank"
          rel="noreferrer"
        >Documentation</a>
      </div>

      <div class="wrap feature-strip">
        <div class="feature">
          <h3>🧩 One codebase</h3>
          <p>Write your UI once with Vue; run it as a desktop, mobile, or PWA app.</p>
        </div>
        <div class="feature">
          <h3>⚡ Go backend</h3>
          <p>Call typed Go functions and stream live events over the bridge.</p>
        </div>
        <div class="feature">
          <h3>📱 Native features</h3>
          <p>Camera, geolocation, notifications, sensors and more — see the demos.</p>
        </div>
      </div>
    </section>

    <!-- Demo menu -->
    <DemosMenu v-else-if="view.kind === 'menu'" />

    <!-- A single demo -->
    <div v-else-if="view.kind === 'demo' && DemoComponent" class="wrap">
      <component :is="DemoComponent" />
    </div>

    <!-- Unknown demo id -->
    <div v-else class="wrap">
      <button class="demo-back" @click="navigate('/demos')">&larr; All demos</button>
      <p class="muted" style="margin-top: 1rem">That demo doesn’t exist.</p>
    </div>
  </main>

  <footer class="footer">
    Edit <code>frontend/src/App.vue</code> to make this landing page your own.
  </footer>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { navigate } from '../router'
import { findDemo } from './registry'
import {
  detectPlatform,
  PLATFORM_LABELS,
  PLATFORM_ORDER,
  SUPPORT_NOTE,
} from './support'
import type { PlatformKey } from './support'

// DemoFrame renders the shared chrome for a demo: a back link, the title,
// per-platform support badges (with the current platform highlighted) and a
// support note. The demo's own controls go in the default slot. Metadata is
// read from the registry by `id`, so there is a single source of truth.
const props = defineProps<{ id: string }>()
const meta = findDemo(props.id)
const current = ref<PlatformKey | null>(null)

onMounted(async () => {
  current.value = await detectPlatform()
})
</script>

<template>
  <div class="demo-frame" v-if="meta">
    <button class="demo-back" @click="navigate('/demos')">&larr; All demos</button>

    <div class="demo-frame-head">
      <h1><span>{{ meta.icon }}</span> {{ meta.title }}</h1>
      <p>{{ meta.description }}</p>
      <div class="badges">
        <span
          v-for="p in PLATFORM_ORDER"
          :key="p"
          class="badge"
          :class="['badge--' + meta.support[p], { 'badge--current': current === p }]"
        >
          <span class="dot"></span>{{ PLATFORM_LABELS[p] }}
        </span>
      </div>
    </div>

    <div v-if="current" class="platform-note">
      <strong>{{ PLATFORM_LABELS[current] }}:</strong>
      {{ SUPPORT_NOTE[meta.support[current]] }}
    </div>

    <slot />
  </div>
</template>

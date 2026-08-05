<script setup lang="ts">
import { ref } from 'vue'
import { getCurrentPosition } from '@goleo/bridge'
import type { Position } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const pos = ref<Position | null>(null)
const loading = ref(false)
const err = ref('')

async function locate() {
  err.value = ''
  loading.value = true
  pos.value = null
  try {
    pos.value = await getCurrentPosition({ enableHighAccuracy: true, timeout: 10000 })
  } catch (e) {
    err.value = String(e)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <DemoFrame id="geolocation">
    <div class="panel">
      <button class="btn btn-primary" @click="locate" :disabled="loading">
        {{ loading ? 'Locating…' : 'Get current position' }}
      </button>
      <div v-if="pos" class="result">
        Latitude: {{ pos.latitude }}<br />
        Longitude: {{ pos.longitude }}<br />
        <template v-if="pos.accuracy != null">Accuracy: ±{{ Math.round(pos.accuracy) }} m<br /></template>
        <a
          :href="'https://www.openstreetmap.org/?mlat=' + pos.latitude + '&mlon=' + pos.longitude + '#map=15/' + pos.latitude + '/' + pos.longitude"
          target="_blank"
          rel="noreferrer"
        >View on OpenStreetMap ↗</a>
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        On desktop Linux there is no portable OS location source, so the call
        falls back to the webview's browser geolocation (which may be unavailable).
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

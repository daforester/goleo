<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { getBatteryInfo } from '@goleo/bridge'
import type { BatteryInfo } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const info = ref<BatteryInfo | null>(null)
const err = ref('')

async function refresh() {
  err.value = ''
  try {
    info.value = await getBatteryInfo()
  } catch (e) {
    err.value = String(e)
  }
}

onMounted(refresh)
</script>

<template>
  <DemoFrame id="battery">
    <div class="panel">
      <button class="btn btn-primary" @click="refresh">Refresh</button>
      <div v-if="info" class="result">
        Level: {{ Math.round(info.level * 100) }}%<br />
        Charging: {{ info.charging ? 'yes' : 'no' }}
        <template v-if="info.chargingTime"><br />Time to full: {{ Math.round(info.chargingTime / 60) }} min</template>
        <template v-if="info.dischargingTime"><br />Time to empty: {{ Math.round(info.dischargingTime / 60) }} min</template>
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        In PWA/web mode this uses the browser Battery Status API, which only some
        browsers (mainly Chromium) implement.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

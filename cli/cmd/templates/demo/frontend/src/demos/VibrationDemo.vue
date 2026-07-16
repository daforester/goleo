<script setup lang="ts">
import { ref } from 'vue'
import { vibrate } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const status = ref('')
const err = ref('')

async function buzz(pattern: number | number[], label: string) {
  err.value = ''
  status.value = ''
  try {
    await vibrate(pattern)
    status.value = label + ' — if your device has a vibration motor, you felt it.'
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="vibration">
    <div class="panel">
      <div class="row">
        <button class="btn btn-primary" @click="buzz(200, 'Short buzz')">Short buzz</button>
        <button class="btn" @click="buzz([100, 50, 100, 50, 300], 'Pattern')">Pattern</button>
      </div>
      <p class="muted" v-if="status">{{ status }}</p>
      <p class="muted" style="margin-top: 0.75rem">
        Desktops have no vibration motor. In the browser this uses
        <code>navigator.vibrate</code>, which is effectively Android-Chrome only.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { wakeLockRequest, wakeLockRelease } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const active = ref(false)
const err = ref('')

async function acquire() {
  err.value = ''
  try {
    await wakeLockRequest('screen')
    active.value = true
  } catch (e) {
    err.value = String(e)
  }
}

async function release() {
  err.value = ''
  try {
    await wakeLockRelease()
    active.value = false
  } catch (e) {
    err.value = String(e)
  }
}

onBeforeUnmount(() => {
  if (active.value) wakeLockRelease().catch(() => {})
})
</script>

<template>
  <DemoFrame id="wakelock">
    <div class="panel">
      <p>Status: <strong>{{ active ? 'holding wake lock' : 'released' }}</strong></p>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn btn-primary" @click="acquire" :disabled="active">Keep screen awake</button>
        <button class="btn" @click="release" :disabled="!active">Release</button>
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        In PWA/web mode this uses the Screen Wake Lock API, which requires a
        secure (HTTPS) context and releases automatically if the tab is hidden.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

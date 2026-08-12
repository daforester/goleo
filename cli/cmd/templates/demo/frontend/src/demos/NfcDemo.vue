<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { startScan, stopScan, nfcWrite, on } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const scanning = ref(false)
const text = ref('Hello from Goleo')
const status = ref('')
const lastTag = ref('')
const err = ref('')
let offTag: (() => void) | null = null

onMounted(() => {
  // Native backends (e.g. the libnfc desktop scanner) push scanned tags here.
  offTag = on('nfc:tag', (d: any) => {
    lastTag.value = d?.uid ? 'UID: ' + d.uid : JSON.stringify(d)
  })
})
onBeforeUnmount(() => {
  offTag?.()
  if (scanning.value) stopScan().catch(() => {})
})

async function scan() {
  err.value = ''
  try {
    await startScan()
    scanning.value = true
    status.value = 'Scanning… tap an NFC tag against the reader/device.'
  } catch (e) {
    err.value = String(e)
  }
}

async function stop() {
  err.value = ''
  try {
    await stopScan()
  } finally {
    scanning.value = false
    status.value = ''
  }
}

async function write() {
  err.value = ''
  status.value = ''
  try {
    await nfcWrite({ records: [{ type: 'text', mediaType: 'text/plain', data: text.value }] })
    status.value = 'Wrote a text record — hold a writable tag to the device.'
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="nfc">
    <div class="panel">
      <h2>Scan</h2>
      <div class="row">
        <button class="btn btn-primary" @click="scan" :disabled="scanning">Start scan</button>
        <button class="btn" @click="stop" :disabled="!scanning">Stop</button>
      </div>
      <p class="muted" v-if="status">{{ status }}</p>
      <div class="result" v-if="lastTag">Last tag → {{ lastTag }}</div>
      <p class="muted" style="margin-top: 0.5rem">
        On the Linux desktop, native scanning uses libnfc (build with
        <code>-tags goleo_libnfc</code> and a supported reader). Detected tags
        arrive as <code>nfc:tag</code> events, shown above.
      </p>
    </div>

    <div class="panel">
      <h2>Write a text tag</h2>
      <div class="row">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="text" />
        <button class="btn" @click="write">Write</button>
      </div>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

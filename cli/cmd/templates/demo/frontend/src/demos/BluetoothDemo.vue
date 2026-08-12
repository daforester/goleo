<script setup lang="ts">
import { ref } from 'vue'
import { requestDevice, bleConnect, bleDisconnect } from '@goleo/bridge'
import type { BLEDevice } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const device = ref<BLEDevice | null>(null)
const connected = ref(false)
const err = ref('')

async function pick() {
  err.value = ''
  try {
    device.value = await requestDevice({ acceptAllDevices: true })
    connected.value = false
  } catch (e) {
    err.value = String(e)
  }
}

async function connect() {
  err.value = ''
  if (!device.value) return
  try {
    await bleConnect(device.value.id)
    connected.value = true
  } catch (e) {
    err.value = String(e)
  }
}

async function disconnect() {
  err.value = ''
  if (!device.value) return
  try {
    await bleDisconnect(device.value.id)
    connected.value = false
  } catch (e) {
    err.value = String(e)
  }
}
</script>

<template>
  <DemoFrame id="bluetooth">
    <div class="panel">
      <div class="row">
        <button class="btn btn-primary" @click="pick">Choose a device</button>
        <button class="btn" @click="connect" :disabled="!device || connected">Connect</button>
        <button class="btn" @click="disconnect" :disabled="!device || !connected">Disconnect</button>
      </div>
      <div class="result" v-if="device">
        {{ device.name }} <span class="muted">({{ device.id }})</span><br />
        {{ connected ? 'Connected' : 'Not connected' }}
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        In PWA/web mode this uses Web Bluetooth, which is only available in
        Chromium browsers over HTTPS.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

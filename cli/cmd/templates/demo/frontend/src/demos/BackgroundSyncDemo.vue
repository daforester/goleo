<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import {
  registerSync,
  isBackgroundPermissionGranted,
  requestBackgroundPermission,
  on,
} from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const granted = ref(false)
const tag = ref('goleo-demo-sync')
const status = ref('')
const err = ref('')
const lastSync = ref('')

// Fires when a registered task actually runs (Android WorkManager /
// iOS BGTaskScheduler — see MainActivity.java's GoleoBackground).
const offSync = on('goleo:backgroundSync', (d: any) => {
  lastSync.value = 'Ran at ' + new Date().toLocaleTimeString() + ' — tag "' + (d?.tag ?? '') + '"'
})
onBeforeUnmount(offSync)

async function refresh() {
  err.value = ''
  try {
    granted.value = await isBackgroundPermissionGranted()
  } catch (e) {
    err.value = String(e)
  }
}

async function ask() {
  err.value = ''
  try {
    await requestBackgroundPermission()
    await refresh()
  } catch (e) {
    err.value = String(e)
  }
}

async function register() {
  err.value = ''
  status.value = ''
  try {
    await registerSync(tag.value)
    status.value = 'Registered background sync task "' + tag.value + '".'
  } catch (e) {
    err.value = String(e)
  }
}

onMounted(refresh)
</script>

<template>
  <DemoFrame id="background">
    <div class="panel">
      <p>Permission: <strong>{{ granted ? 'granted' : 'not granted' }}</strong></p>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn" @click="ask">Request permission</button>
      </div>
    </div>

    <div class="panel">
      <h2>Register a sync task</h2>
      <div class="row">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="tag" />
        <button class="btn btn-primary" @click="register">Register</button>
      </div>
      <p class="muted" v-if="status">{{ status }}</p>
      <p class="muted" v-if="lastSync" style="margin-top: 0.5rem">{{ lastSync }}</p>
      <p class="muted" style="margin-top: 0.5rem">
        On Android this schedules a real WorkManager task that runs once
        connectivity is available — even if the app isn't in the foreground.
        In PWA/web mode it uses the Background Sync API (Chromium only) via
        the service worker instead. Desktops keep the process running, so
        background scheduling doesn't apply.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

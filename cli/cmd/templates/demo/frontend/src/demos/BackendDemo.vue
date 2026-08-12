<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { invoke, on, isConnected, sendEvent } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const connected = ref(isConnected())
const greetName = ref('Goleo')
const greetOut = ref('')
const sysOut = ref('')
const a = ref(2)
const b = ref(3)
const sumOut = ref<number | null>(null)
const countdownOut = ref('')
const heartbeat = ref('')
const logText = ref('Hello from the frontend!')
const logStatus = ref('')
const err = ref('')

let offHeartbeat: (() => void) | null = null
onMounted(() => {
  offHeartbeat = on('heartbeat', (d) => {
    heartbeat.value = JSON.stringify(d)
  })
})
onBeforeUnmount(() => offHeartbeat?.())

async function greet() {
  err.value = ''
  try {
    const r = await invoke<{ message: string }>('greet', { name: greetName.value })
    greetOut.value = r.message
  } catch (e) {
    err.value = String(e)
  }
}

async function systemInfo() {
  err.value = ''
  try {
    sysOut.value = JSON.stringify(await invoke('systemInfo'), null, 2)
  } catch (e) {
    err.value = String(e)
  }
}

async function addNums() {
  err.value = ''
  try {
    const r = await invoke<{ result: number }>('add', { a: a.value, b: b.value })
    sumOut.value = r.result
  } catch (e) {
    err.value = String(e)
  }
}

async function countdown() {
  err.value = ''
  countdownOut.value = 'Waiting 3s…'
  try {
    const r = await invoke<{ message: string }>('countdown')
    countdownOut.value = r.message
  } catch (e) {
    err.value = String(e)
  }
}

function sendLog() {
  sendEvent('app:log', { text: logText.value })
  logStatus.value = "Sent! Look for [app:log] in the `goleo dev` console."
}
</script>

<template>
  <DemoFrame id="backend">
    <div class="panel" v-if="!connected">
      <p class="muted">
        No Go backend is connected (PWA/web mode), so these calls will fail.
        Run <code>goleo dev</code> to use the backend.
      </p>
    </div>

    <div class="panel">
      <h2>greet(name)</h2>
      <div class="row">
        <input class="input" v-model="greetName" />
        <button class="btn btn-primary" @click="greet">Invoke greet</button>
      </div>
      <div class="result" v-if="greetOut">{{ greetOut }}</div>
    </div>

    <div class="panel">
      <h2>systemInfo()</h2>
      <button class="btn btn-primary" @click="systemInfo">Get system info</button>
      <pre v-if="sysOut" style="margin-top: 0.75rem">{{ sysOut }}</pre>
    </div>

    <div class="panel">
      <h2>add(a, b) &amp; async countdown()</h2>
      <div class="row">
        <input class="input" type="number" v-model.number="a" />
        <span>+</span>
        <input class="input" type="number" v-model.number="b" />
        <button class="btn btn-primary" @click="addNums">=</button>
        <strong v-if="sumOut !== null">{{ sumOut }}</strong>
      </div>
      <div class="row" style="margin-top: 0.75rem">
        <button class="btn" @click="countdown">Start 3s async countdown</button>
        <span class="muted">{{ countdownOut }}</span>
      </div>
    </div>

    <div class="panel">
      <h2>Live events</h2>
      <p class="muted">The backend emits a <code>heartbeat</code> event every 5s:</p>
      <div class="result">{{ heartbeat || 'Waiting for heartbeat…' }}</div>
      <div class="row" style="margin-top: 0.75rem">
        <input class="input" style="flex: 1; min-width: 12rem" v-model="logText" />
        <button class="btn" @click="sendLog">sendEvent('app:log')</button>
      </div>
      <p class="muted" v-if="logStatus">{{ logStatus }}</p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

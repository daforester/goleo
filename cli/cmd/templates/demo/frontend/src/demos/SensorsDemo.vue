<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { startBrowserSensor, startNativeSensor } from '@goleo/bridge'
import type { SensorData } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const type = ref<'accelerometer' | 'gyroscope' | 'magnetometer'>('accelerometer')
const reading = ref<SensorData | null>(null)
const running = ref(false)
const err = ref('')
let stop: (() => void) | null = null

async function start() {
  err.value = ''
  stopSensor()
  const onReading = (d: SensorData) => { reading.value = d }
  try {
    // Native platform sensor manager (Android SensorManager / iOS
    // CoreMotion), if a shell registered one.
    stop = await startNativeSensor(type.value, onReading)
    running.value = true
    return
  } catch {
    // Fall through to the browser Generic Sensor API.
  }
  try {
    stop = startBrowserSensor(type.value, onReading)
    running.value = true
  } catch (e) {
    err.value = String(e)
    running.value = false
  }
}

function stopSensor() {
  stop?.()
  stop = null
  running.value = false
}

onBeforeUnmount(stopSensor)
</script>

<template>
  <DemoFrame id="sensors">
    <div class="panel">
      <div class="row">
        <select class="input" v-model="type" :disabled="running">
          <option value="accelerometer">accelerometer</option>
          <option value="gyroscope">gyroscope</option>
          <option value="magnetometer">magnetometer</option>
        </select>
        <button class="btn btn-primary" @click="start" :disabled="running">Start</button>
        <button class="btn" @click="stopSensor" :disabled="!running">Stop</button>
      </div>
      <div class="result" v-if="reading">
        x: {{ reading.x.toFixed(3) }}<br />
        y: {{ reading.y.toFixed(3) }}<br />
        z: {{ reading.z.toFixed(3) }}
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        This uses the browser Generic Sensor API (secure context; effectively
        Android-Chrome). On native mobile builds, motion data is delivered
        through the platform sensor manager rather than this browser API.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

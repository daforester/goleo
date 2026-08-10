<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { invoke } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

// Recording is done entirely in the WebView: getUserMedia + MediaRecorder work on every
// platform goleo targets, so there is no Go-side capture to call. What the Go backend adds
// is the PERMISSION state, which a web page cannot ask about without starting a capture —
// on mobile that is a native API (goleo:microphonePermission / …RequestPermission).

const permission = ref('unknown')
const recording = ref(false)
const clipUrl = ref('')
const seconds = ref(0)
const err = ref('')

let recorder: MediaRecorder | null = null
let stream: MediaStream | null = null
let chunks: Blob[] = []
let ticker: ReturnType<typeof setInterval> | null = null

async function checkPermission() {
  err.value = ''
  try {
    const r = await invoke<{ granted: boolean }>('goleo:microphonePermission')
    permission.value = r.granted ? 'granted' : 'not granted'
  } catch (e) {
    // Desktop has no OS-level mic permission to query — getUserMedia's own prompt is the
    // permission model there — so ErrUnsupported is the expected answer, not a failure.
    permission.value = 'n/a on this platform (the browser prompt decides)'
  }
}

async function requestPermission() {
  err.value = ''
  try {
    const r = await invoke<{ status: string }>('goleo:microphoneRequestPermission')
    // "default" means the OS prompt is on screen and has not been answered yet.
    permission.value = r.status === 'default' ? 'prompt shown — check again' : r.status
  } catch (e) {
    err.value = String(e)
  }
}

// Mirrors CameraDemo.vue's describeError. The distinction matters more here than the raw
// DOMException does: "denied" and "no device" look identical from the demo but need opposite
// responses, and on an emulator the usual cause is neither — the emulator zeroes out audio
// input unless it was started with -allow-host-audio (goleo passes it; an emulator started
// from Android Studio needs Extended Controls -> Microphone).
function describeError(e: unknown): string {
  const name = (e as { name?: string })?.name
  if (name === 'NotAllowedError' || name === 'SecurityError') {
    return `${e} — microphone permission was denied.`
  }
  if (name === 'NotFoundError' || name === 'OverconstrainedError') {
    return `${e} — no microphone was found. On an emulator, audio input is off unless the emulator was started with host audio enabled; on a real device, check that a mic is present.`
  }
  if (name === 'NotReadableError' || name === 'AbortError') {
    return `${e} — a microphone exists but could not be opened. It may be in use by another app, or the emulator is zeroing out audio input.`
  }
  return String(e)
}

async function start() {
  err.value = ''
  clipUrl.value = ''
  if (!navigator.mediaDevices?.getUserMedia) {
    err.value = 'Recording needs getUserMedia, which is not available here.'
    return
  }
  if (typeof MediaRecorder === 'undefined') {
    err.value = 'Recording needs MediaRecorder, which this WebView does not provide.'
    return
  }
  try {
    stream = await navigator.mediaDevices.getUserMedia({ audio: true, video: false })
  } catch (e) {
    err.value = describeError(e)
    return
  }
  chunks = []
  recorder = new MediaRecorder(stream)
  recorder.ondataavailable = (ev) => {
    if (ev.data.size > 0) chunks.push(ev.data)
  }
  recorder.onstop = () => {
    // Type comes from the recorder, not a guess: WebKit produces mp4 and Chromium webm,
    // and hardcoding either gives a clip the <audio> element refuses to play.
    const blob = new Blob(chunks, { type: recorder?.mimeType || 'audio/webm' })
    if (clipUrl.value) URL.revokeObjectURL(clipUrl.value)
    clipUrl.value = URL.createObjectURL(blob)
  }
  recorder.start()
  recording.value = true
  seconds.value = 0
  ticker = setInterval(() => { seconds.value += 1 }, 1000)
}

function stop() {
  recording.value = false
  if (ticker) { clearInterval(ticker); ticker = null }
  recorder?.stop()
  // Releasing the tracks is what turns the OS recording indicator off. Leaving them live
  // looks to the user like the app is still listening.
  stream?.getTracks().forEach((t) => t.stop())
  stream = null
}

onBeforeUnmount(() => {
  if (recording.value) stop()
  if (clipUrl.value) URL.revokeObjectURL(clipUrl.value)
})
</script>

<template>
  <DemoFrame id="microphone">
    <div class="panel">
      <p>Permission: <strong>{{ permission }}</strong></p>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn" @click="checkPermission">Check permission</button>
        <button class="btn" @click="requestPermission">Request permission</button>
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        “Request permission” asks the OS directly, without recording — the quickest way to
        confirm the microphone prompt appears. On desktop there is no OS-level microphone
        permission to query; the browser’s own getUserMedia prompt is the permission model.
      </p>
    </div>

    <div class="panel" style="margin-top: 1rem">
      <p>
        Recorder:
        <strong>{{ recording ? `recording… ${seconds}s` : 'idle' }}</strong>
      </p>
      <div class="row" style="margin-top: 0.5rem">
        <button class="btn btn-primary" @click="start" :disabled="recording">Record</button>
        <button class="btn" @click="stop" :disabled="!recording">Stop</button>
      </div>

      <div v-if="clipUrl" style="margin-top: 0.75rem">
        <audio :src="clipUrl" controls style="width: 100%"></audio>
        <p class="muted" style="margin-top: 0.5rem">
          Play it back — hearing your own voice is the only proof the microphone actually
          captured rather than merely being permitted.
        </p>
      </div>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

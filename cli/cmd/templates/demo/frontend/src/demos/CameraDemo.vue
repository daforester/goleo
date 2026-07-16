<script setup lang="ts">
import { onBeforeUnmount, ref } from 'vue'
import { capturePhoto } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const err = ref('')

// ── Still photo (via the bridge: native V4L2 on Linux, getUserMedia elsewhere)
const photo = ref('')
const busy = ref(false)

function toSrc(data: string, format: string): string {
  return data.startsWith('data:') ? data : 'data:image/' + format + ';base64,' + data
}

async function capture() {
  err.value = ''
  busy.value = true
  photo.value = ''
  try {
    const p = await capturePhoto({ width: 640, height: 480 })
    photo.value = toSrc(p.data, p.format)
  } catch (e) {
    err.value = String(e)
  } finally {
    busy.value = false
  }
}

// ── Live video preview (frontend-only: getUserMedia -> <video>)
const videoEl = ref<HTMLVideoElement | null>(null)
const streaming = ref(false)
let stream: MediaStream | null = null

async function startPreview() {
  err.value = ''
  if (!navigator.mediaDevices?.getUserMedia) {
    err.value = 'Live preview needs getUserMedia, which is not available here.'
    return
  }
  try {
    // `video: true` (no size constraints) avoids OverconstrainedError.
    stream = await navigator.mediaDevices.getUserMedia({ video: true, audio: false })
    if (videoEl.value) {
      videoEl.value.srcObject = stream
      await videoEl.value.play()
    }
    streaming.value = true
  } catch (e) {
    err.value = String(e)
    stopPreview()
  }
}

function stopPreview() {
  stream?.getTracks().forEach((t) => t.stop())
  stream = null
  if (videoEl.value) videoEl.value.srcObject = null
  streaming.value = false
}

onBeforeUnmount(stopPreview)
</script>

<template>
  <DemoFrame id="camera">
    <div class="panel">
      <h2>Live preview</h2>
      <div class="row">
        <button class="btn btn-primary" @click="startPreview" :disabled="streaming">Start camera</button>
        <button class="btn" @click="stopPreview" :disabled="!streaming">Stop</button>
      </div>
      <!-- kept in the DOM (v-show) so the ref is available before playing -->
      <video
        ref="videoEl"
        v-show="streaming"
        autoplay
        muted
        playsinline
        style="margin-top: 0.75rem; max-width: 100%; border-radius: 8px; background: #000"
      ></video>
      <p class="muted" style="margin-top: 0.75rem">
        Streams straight from the OS camera into a <code>&lt;video&gt;</code>
        element. Works on the Linux desktop webview (permission auto-granted),
        Windows (WebView2), and browsers/PWA; macOS needs a camera usage string.
      </p>
    </div>

    <div class="panel">
      <h2>Capture a still photo</h2>
      <button class="btn btn-primary" @click="capture" :disabled="busy">
        {{ busy ? 'Capturing…' : 'Capture photo' }}
      </button>
      <div v-if="photo" style="margin-top: 0.75rem">
        <img :src="photo" alt="captured" style="max-width: 100%; border-radius: 8px" />
      </div>
      <p class="muted" style="margin-top: 0.75rem">
        On Linux this grabs a frame natively via V4L2 in Go; macOS/Windows use
        the webview's getUserMedia.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

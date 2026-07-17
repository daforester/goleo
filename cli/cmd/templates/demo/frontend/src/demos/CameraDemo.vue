<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { capturePhoto } from '@goleo/bridge'
import DemoFrame from './DemoFrame.vue'

const err = ref('')

// ── Still photo (via the bridge: native V4L2 on Linux, getUserMedia elsewhere)
const photo = ref('')
const busy = ref(false)

function toSrc(data: string, format: string): string {
  return data.startsWith('data:') ? data : 'data:image/' + format + ';base64,' + data
}

function grabFrame(video: HTMLVideoElement): string {
  const canvas = document.createElement('canvas')
  canvas.width = video.videoWidth
  canvas.height = video.videoHeight
  canvas.getContext('2d')!.drawImage(video, 0, 0, canvas.width, canvas.height)
  return canvas.toDataURL('image/jpeg')
}

async function capture() {
  err.value = ''
  busy.value = true
  photo.value = ''
  try {
    if (streaming.value && videoEl.value?.videoWidth) {
      // A preview is live on the selected camera — grab that frame directly.
      // This respects the source picker and avoids opening the (possibly busy)
      // device a second time.
      photo.value = grabFrame(videoEl.value)
    } else {
      // No preview: capture via the bridge (native V4L2 on Linux; getUserMedia
      // elsewhere), passing the selected camera so it applies here too.
      const p = await capturePhoto({ width: 640, height: 480, deviceId: selectedId.value || undefined })
      photo.value = toSrc(p.data, p.format)
    }
  } catch (e) {
    err.value = describeError(e)
  } finally {
    busy.value = false
  }
}

// ── Live video preview (frontend-only: getUserMedia -> <video>)
const videoEl = ref<HTMLVideoElement | null>(null)
const streaming = ref(false)
let stream: MediaStream | null = null

// Camera selection
const cameras = ref<MediaDeviceInfo[]>([])
const selectedId = ref('')

async function refreshCameras() {
  if (!navigator.mediaDevices?.enumerateDevices) return
  try {
    const devices = await navigator.mediaDevices.enumerateDevices()
    cameras.value = devices.filter((d) => d.kind === 'videoinput')
    // Keep a valid selection; default to the first camera.
    if (!cameras.value.some((c) => c.deviceId === selectedId.value)) {
      selectedId.value = cameras.value[0]?.deviceId ?? ''
    }
  } catch {
    // enumerateDevices can fail before any permission grant — ignore.
  }
}

function cameraLabel(d: MediaDeviceInfo, i: number): string {
  // Labels are only populated after a getUserMedia grant.
  return d.label || `Camera ${i + 1}`
}

function describeError(e: unknown): string {
  const name = (e as { name?: string })?.name
  if (name === 'AbortError' || name === 'NotReadableError') {
    return `${e} — the camera couldn't start. It may be in use by another app, or the selected source is unavailable. Close other apps using the camera, or pick a different camera above and retry.`
  }
  if (name === 'NotFoundError' || name === 'OverconstrainedError') {
    return `${e} — the selected camera isn't available. Pick a different one above.`
  }
  if (name === 'NotAllowedError') {
    return `${e} — camera permission was denied.`
  }
  return String(e)
}

async function startPreview() {
  err.value = ''
  if (!navigator.mediaDevices?.getUserMedia) {
    err.value = 'Live preview needs getUserMedia, which is not available here.'
    return
  }
  // Release any existing source first — reusing a busy device is a common cause
  // of "AbortError: Timeout starting video source" on Windows/WebView2.
  stopPreview()
  try {
    const video: MediaTrackConstraints | boolean = selectedId.value
      ? { deviceId: { exact: selectedId.value } }
      : true
    stream = await navigator.mediaDevices.getUserMedia({ video, audio: false })
    if (videoEl.value) {
      videoEl.value.srcObject = stream
      await videoEl.value.play()
    }
    streaming.value = true
    // Now that a grant exists, device labels are readable — refresh the picker.
    await refreshCameras()
  } catch (e) {
    err.value = describeError(e)
    stopPreview()
  }
}

function stopPreview() {
  stream?.getTracks().forEach((t) => t.stop())
  stream = null
  if (videoEl.value) videoEl.value.srcObject = null
  streaming.value = false
}

// Switch camera live: restart the stream on the newly selected device.
async function onSelectCamera() {
  if (streaming.value) await startPreview()
}

onMounted(() => {
  refreshCameras()
  navigator.mediaDevices?.addEventListener?.('devicechange', refreshCameras)
})
onBeforeUnmount(() => {
  navigator.mediaDevices?.removeEventListener?.('devicechange', refreshCameras)
  stopPreview()
})
</script>

<template>
  <DemoFrame id="camera">
    <div class="panel">
      <h2>Live preview</h2>

      <div class="row" v-if="cameras.length">
        <label for="cam" class="muted">Camera:</label>
        <select id="cam" v-model="selectedId" @change="onSelectCamera">
          <option v-for="(c, i) in cameras" :key="c.deviceId || i" :value="c.deviceId">
            {{ cameraLabel(c, i) }}
          </option>
        </select>
      </div>

      <div class="row" style="margin-top: 0.5rem">
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
        element. Pick a source above if you have more than one. Device names
        appear once you've granted camera access. Works on the Linux desktop
        webview (permission auto-granted), Windows (WebView2), and browsers/PWA;
        macOS needs a camera usage string.
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
        Uses the camera selected above. If the live preview is running, the still
        is grabbed straight from it; otherwise it's captured via the bridge —
        natively via V4L2 in Go on Linux, or getUserMedia on macOS/Windows.
      </p>
    </div>

    <div class="result result--error" v-if="err">{{ err }}</div>
  </DemoFrame>
</template>

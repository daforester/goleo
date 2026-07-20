import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface PhotoData {
  data: string
  format: string
}

// getUserMedia can hang indefinitely when the platform WebView surfaces a
// permission prompt that can't be answered in an embedded context — notably
// WebView2 on Windows, where there is no host auto-grant, so the promise never
// settles. Bound it so the fallback rejects cleanly instead of hanging. If the
// stream resolves after we've already given up, stop its tracks so the camera
// is not left running.
const DEFAULT_GET_USER_MEDIA_TIMEOUT_MS = 10000

function getUserMediaWithTimeout(
  constraints: MediaStreamConstraints,
  timeoutMs: number,
): Promise<MediaStream> {
  let timedOut = false
  let timer: ReturnType<typeof setTimeout>
  const media = navigator.mediaDevices.getUserMedia(constraints)
  media
    .then((stream) => {
      if (timedOut) {
        stream.getTracks().forEach((t) => t.stop())
      }
    })
    .catch(() => {
      // Swallow a late rejection on this side-channel; the race below already
      // surfaced the error (or the timeout) to the caller.
    })
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(() => {
      timedOut = true
      reject(
        new Error(
          `getUserMedia timed out after ${timeoutMs}ms (permission prompt may be unanswerable in this webview)`,
        ),
      )
    }, timeoutMs)
  })
  return Promise.race([media, timeout]).finally(() => clearTimeout(timer))
}

export async function capturePhoto(options?: {
  width?: number
  height?: number
  deviceId?: string
  timeoutMs?: number
}): Promise<PhotoData> {
  try {
    return await bridge().invoke<PhotoData>('goleo:cameraCapturePhoto', options as Record<string, unknown>)
  } catch {
    // Fallback: use getUserMedia + canvas.
    if (!navigator.mediaDevices?.getUserMedia) {
      throw new Error('camera not available')
    }
    // Pass width/height as *ideal* hints, not exact constraints — a plain
    // { width, height } is treated as a hard constraint by some engines
    // (notably WebKitGTK) and throws OverconstrainedError when the camera
    // can't produce that exact size. `ideal` lets it pick the closest mode.
    const video: MediaTrackConstraints =
      options?.width || options?.height
        ? {
            width: options?.width ? { ideal: options.width } : undefined,
            height: options?.height ? { ideal: options.height } : undefined,
          }
        : {}
    // Honor an explicit camera selection when given.
    if (options?.deviceId) {
      video.deviceId = { exact: options.deviceId }
    }
    const stream = await getUserMediaWithTimeout(
      { video },
      options?.timeoutMs ?? DEFAULT_GET_USER_MEDIA_TIMEOUT_MS,
    )
    const el = document.createElement('video')
    el.srcObject = stream
    el.muted = true
    el.setAttribute('playsinline', '')
    try {
      await el.play()
      // Ensure the frame dimensions are known before drawing.
      if (!el.videoWidth) {
        await new Promise<void>((resolve) => {
          el.onloadedmetadata = () => resolve()
        })
      }
      const canvas = document.createElement('canvas')
      canvas.width = el.videoWidth || options?.width || 640
      canvas.height = el.videoHeight || options?.height || 480
      canvas.getContext('2d')!.drawImage(el, 0, 0, canvas.width, canvas.height)
      return { data: canvas.toDataURL('image/jpeg'), format: 'jpeg' }
    } finally {
      stream.getTracks().forEach((t) => t.stop())
    }
  }
}

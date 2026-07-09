import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface PhotoData {
  data: string
  format: string
}

export async function capturePhoto(options?: { width?: number; height?: number }): Promise<PhotoData> {
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
    const stream = await navigator.mediaDevices.getUserMedia({ video })
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

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
    // Fallback: use getUserMedia + canvas
    const stream = await navigator.mediaDevices.getUserMedia({
      video: { width: options?.width ?? 640, height: options?.height ?? 480 },
    })
    const video = document.createElement('video')
    video.srcObject = stream
    await video.play()
    const canvas = document.createElement('canvas')
    canvas.width = video.videoWidth
    canvas.height = video.videoHeight
    canvas.getContext('2d')!.drawImage(video, 0, 0)
    stream.getTracks().forEach((t) => t.stop())
    return { data: canvas.toDataURL('image/jpeg'), format: 'jpeg' }
  }
}

import { getBridge } from './bridge'

const bridge = () => getBridge()

export async function vibrate(pattern?: number | number[]): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:vibrate', { pattern: Array.isArray(pattern) ? pattern : [pattern ?? 200] })
  } catch {
    if (typeof navigator !== 'undefined' && 'vibrate' in navigator) {
      navigator.vibrate(pattern ?? 200)
      return
    }
    throw new Error('vibration API not available')
  }
}

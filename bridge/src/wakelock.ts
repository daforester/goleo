import { getBridge } from './bridge'

const bridge = () => getBridge()

export async function wakeLockRequest(type?: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:wakeLockRequest', { type: type ?? 'screen' })
  } catch {
    if (typeof navigator !== 'undefined' && 'wakeLock' in navigator) {
      await (navigator as any).wakeLock.request(type ?? 'screen')
      return
    }
    throw new Error('wakeLock API not available')
  }
}

export async function wakeLockRelease(): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:wakeLockRelease')
  } catch {
    // Browser WakeLockSentinel auto-releases on page visibility change
  }
}

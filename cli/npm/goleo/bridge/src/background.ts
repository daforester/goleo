import { getBridge } from './bridge'

const bridge = () => getBridge()

export async function registerSync(tag: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:backgroundRegisterSync', { tag })
  } catch {
    if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
      const reg = await navigator.serviceWorker.ready
      await (reg as any).sync.register(tag)
      return
    }
    throw new Error('background sync not available')
  }
}

export async function isPermissionGranted(): Promise<boolean> {
  try {
    return await bridge().invoke<boolean>('goleo:backgroundPermissionGranted')
  } catch {
    return typeof navigator !== 'undefined' && 'serviceWorker' in navigator
  }
}

export async function requestPermission(): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:backgroundRequestPermission')
  } catch {
    // In browsers, permission is granted by default for service workers
  }
}

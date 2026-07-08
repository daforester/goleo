import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface PushSubscriptionData {
  endpoint: string
  keys: Record<string, string>
}

export async function subscribe(serverKey?: string): Promise<PushSubscriptionData> {
  try {
    return await bridge().invoke<PushSubscriptionData>('goleo:pushSubscribe', { serverKey })
  } catch {
    if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator && 'PushManager' in window) {
      const reg = await navigator.serviceWorker.ready
      const sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: serverKey ? urlBase64ToUint8Array(serverKey) as unknown as BufferSource : undefined,
      })
      return {
        endpoint: sub.endpoint,
        keys: Object.fromEntries((await sub.getKey('p256dh') && await sub.getKey('auth')) ? [
          ['p256dh', btoa(String.fromCharCode(...new Uint8Array(await sub.getKey('p256dh')!)))],
          ['auth', btoa(String.fromCharCode(...new Uint8Array(await sub.getKey('auth')!)))],
        ] : []),
      }
    }
    throw new Error('Push API not available')
  }
}

export async function unsubscribe(): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:pushUnsubscribe')
  } catch {
    if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
      const reg = await navigator.serviceWorker.ready
      const sub = await reg.pushManager.getSubscription()
      await sub?.unsubscribe()
    }
  }
}

export async function getSubscription(): Promise<PushSubscriptionData | null> {
  try {
    return await bridge().invoke<PushSubscriptionData | null>('goleo:pushGetSubscription')
  } catch {
    if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
      const reg = await navigator.serviceWorker.ready
      const sub = await reg.pushManager.getSubscription()
      if (!sub) return null
      return {
        endpoint: sub.endpoint,
        keys: Object.fromEntries((await sub.getKey('p256dh') && await sub.getKey('auth')) ? [
          ['p256dh', btoa(String.fromCharCode(...new Uint8Array(await sub.getKey('p256dh')!)))],
          ['auth', btoa(String.fromCharCode(...new Uint8Array(await sub.getKey('auth')!)))],
        ] : []),
      }
    }
    return null
  }
}

function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = '='.repeat((4 - base64String.length % 4) % 4)
  const base64 = (base64String + padding).replace(/-/g, '+').replace(/_/g, '/')
  const rawData = atob(base64)
  return Uint8Array.from(rawData.split('').map((c) => c.charCodeAt(0)))
}

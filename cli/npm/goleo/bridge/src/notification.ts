import { getBridge } from './bridge'

export interface NotificationOptions {
  title: string
  body?: string
}

export type NotificationPermission = 'granted' | 'denied' | 'default'

function supportsBrowserNotification(): boolean {
  return typeof window !== 'undefined' && 'Notification' in window
}

export async function sendNotification(options: NotificationOptions | string): Promise<void> {
  const opts = typeof options === 'string' ? { title: options } : options
  try {
    await getBridge().invoke<void>('goleo:notify', { title: opts.title, body: opts.body ?? '' })
  } catch {
    if (supportsBrowserNotification() && window.Notification.permission === 'granted') {
      new window.Notification(opts.title, { body: opts.body })
    }
  }
}

export async function isPermissionGranted(): Promise<boolean> {
  try {
    return await getBridge().invoke<boolean>('goleo:notificationPermissionGranted')
  } catch {
    if (supportsBrowserNotification()) {
      return window.Notification.permission === 'granted'
    }
    return false
  }
}

export async function requestPermission(): Promise<NotificationPermission> {
  try {
    return await getBridge().invoke<NotificationPermission>('goleo:requestNotificationPermission')
  } catch {
    if (supportsBrowserNotification()) {
      return window.Notification.requestPermission()
    }
    return 'denied'
  }
}

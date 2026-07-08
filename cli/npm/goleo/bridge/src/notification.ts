import { invoke } from './index'

export interface NotificationOptions {
  /** Notification title. */
  title: string
  /** Optional notification body text. */
  body?: string
}

export type NotificationPermission = 'granted' | 'denied' | 'default'

/**
 * Show a native system notification.
 *
 * The notification is posted by the Goleo core: toast notifications on
 * Windows, Notification Center on macOS, libnotify on Linux, and the
 * platform notification service on Android/iOS (via the native shell).
 */
export async function sendNotification(options: NotificationOptions | string): Promise<void> {
  const opts = typeof options === 'string' ? { title: options } : options
  return invoke<void>('goleo:notify', { title: opts.title, body: opts.body ?? '' })
}

/** Whether the app currently has permission to post notifications. */
export async function isPermissionGranted(): Promise<boolean> {
  return invoke<boolean>('goleo:notificationPermissionGranted')
}

/**
 * Request permission to post notifications. On desktop this resolves to
 * "granted" immediately; on Android 13+ / iOS it triggers the system
 * permission dialog and returns the current state.
 */
export async function requestPermission(): Promise<NotificationPermission> {
  return invoke<NotificationPermission>('goleo:requestNotificationPermission')
}

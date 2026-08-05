import type { Component } from 'vue'
import type { PlatformSupport } from './support'

// A single demo entry. `load` is a lazy import so a demo's code is only fetched
// when opened, and removing a demo is as simple as deleting its .vue file and
// its one entry in the `demos` array below.
export interface Demo {
  id: string
  title: string
  icon: string
  description: string
  support: PlatformSupport
  load: () => Promise<{ default: Component }>
}

// ── The demo registry ──────────────────────────────────────────────────────
// To REMOVE a demo: delete its `.vue` file and delete its object here.
// To ADD a demo: create `MyDemo.vue`, then add an entry here.
export const demos: Demo[] = [
  {
    id: 'backend',
    title: 'Backend & Events',
    icon: '🔌',
    description: 'Call Go functions and stream live events over the bridge.',
    support: { desktop: 'yes', android: 'yes', ios: 'yes', pwa: 'no' },
    load: () => import('./BackendDemo.vue'),
  },
  {
    id: 'notifications',
    title: 'Notifications',
    icon: '🔔',
    description: 'Show native (or browser) notifications with permission handling.',
    support: { desktop: 'yes', android: 'yes', ios: 'yes', pwa: 'yes' },
    load: () => import('./NotificationsDemo.vue'),
  },
  {
    id: 'clipboard',
    title: 'Clipboard',
    icon: '📋',
    description: 'Read from and write to the system clipboard.',
    support: { desktop: 'yes', android: 'yes', ios: 'yes', pwa: 'yes' },
    load: () => import('./ClipboardDemo.vue'),
  },
  {
    id: 'dialogs',
    title: 'Dialogs',
    icon: '🗂️',
    description: 'Native file open/save, folder pickers, message boxes and prompts.',
    support: { desktop: 'yes', android: 'partial', ios: 'partial', pwa: 'partial' },
    load: () => import('./DialogsDemo.vue'),
  },
  {
    id: 'filesystem',
    title: 'File System',
    icon: '📁',
    description: 'Read, write, list and delete files in app-scoped directories.',
    support: { desktop: 'yes', android: 'yes', ios: 'partial', pwa: 'no' },
    load: () => import('./FileSystemDemo.vue'),
  },
  {
    id: 'geolocation',
    title: 'Geolocation',
    icon: '📍',
    description: 'Get the current position from the OS or the browser.',
    support: { desktop: 'partial', android: 'yes', ios: 'yes', pwa: 'yes' },
    load: () => import('./GeolocationDemo.vue'),
  },
  {
    id: 'battery',
    title: 'Battery',
    icon: '🔋',
    description: 'Read battery level and charging state.',
    support: { desktop: 'yes', android: 'yes', ios: 'yes', pwa: 'partial' },
    load: () => import('./BatteryDemo.vue'),
  },
  {
    id: 'vibration',
    title: 'Vibration',
    icon: '📳',
    description: 'Trigger haptic feedback with a pattern.',
    support: { desktop: 'no', android: 'yes', ios: 'partial', pwa: 'partial' },
    load: () => import('./VibrationDemo.vue'),
  },
  {
    id: 'wakelock',
    title: 'Wake Lock',
    icon: '☕',
    description: 'Keep the screen awake while the app is in the foreground.',
    support: { desktop: 'yes', android: 'yes', ios: 'yes', pwa: 'yes' },
    load: () => import('./WakeLockDemo.vue'),
  },
  {
    id: 'sensors',
    title: 'Motion Sensors',
    icon: '🧭',
    description: 'Stream accelerometer / gyroscope / magnetometer readings.',
    support: { desktop: 'no', android: 'yes', ios: 'partial', pwa: 'partial' },
    load: () => import('./SensorsDemo.vue'),
  },
  {
    id: 'camera',
    title: 'Camera',
    icon: '📷',
    description: 'Live camera preview and still-photo capture.',
    support: { desktop: 'partial', android: 'yes', ios: 'yes', pwa: 'yes' },
    load: () => import('./CameraDemo.vue'),
  },
  {
    id: 'bluetooth',
    title: 'Bluetooth LE',
    icon: '📶',
    description: 'Discover and connect to a Bluetooth Low Energy device.',
    support: { desktop: 'no', android: 'yes', ios: 'no', pwa: 'partial' },
    load: () => import('./BluetoothDemo.vue'),
  },
  {
    id: 'nfc',
    title: 'NFC',
    icon: '📇',
    description: 'Scan and write NFC tags.',
    support: { desktop: 'partial', android: 'yes', ios: 'no', pwa: 'partial' },
    load: () => import('./NfcDemo.vue'),
  },
  {
    id: 'push',
    title: 'Push Notifications',
    icon: '📨',
    description: 'Subscribe to remote push notifications.',
    support: { desktop: 'no', android: 'no', ios: 'no', pwa: 'partial' },
    load: () => import('./PushDemo.vue'),
  },
  {
    id: 'background',
    title: 'Background Sync',
    icon: '🔄',
    description: 'Register a task to run when connectivity returns.',
    support: { desktop: 'no', android: 'yes', ios: 'partial', pwa: 'partial' },
    load: () => import('./BackgroundSyncDemo.vue'),
  },
]

export function findDemo(id: string): Demo | undefined {
  return demos.find((d) => d.id === id)
}

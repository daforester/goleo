export { Bridge, getBridge } from './bridge.js'
export {
  sendNotification,
  isPermissionGranted,
  requestPermission,
} from './notification.js'
export type { NotificationOptions, NotificationPermission } from './notification.js'
export {
  readText as clipboardReadText,
  writeText as clipboardWriteText,
} from './clipboard.js'
export {
  openFile,
  openFiles,
  saveFile,
  selectFolder,
  showMessage,
  showPrompt,
} from './dialogs.js'
export type { FileFilter, FileDialogOptions, MessageBoxOptions, PromptOptions } from './dialogs.js'
export {
  readTextFile,
  writeTextFile,
  readBinaryFile,
  writeBinaryFile,
  listDir,
  deleteFile,
  appDataDir,
  homeDir,
} from './fs.js'
export type { FileEntry } from './fs.js'
export {
  getCurrentPosition,
} from './geolocation.js'
export type { Position, PositionOptions } from './geolocation.js'
export {
  getBatteryInfo,
} from './battery.js'
export type { BatteryInfo } from './battery.js'
export {
  wakeLockRequest,
  wakeLockRelease,
} from './wakelock.js'
export {
  vibrate,
} from './vibration.js'
export {
  startSensor,
  stopSensor,
  startBrowserSensor,
  startNativeSensor,
} from './sensors.js'
export type { SensorData } from './sensors.js'
export {
  capturePhoto,
} from './camera.js'
export type { PhotoData } from './camera.js'
export {
  requestDevice,
  connect as bleConnect,
  disconnect as bleDisconnect,
} from './bluetooth.js'
export type { BLEDevice } from './bluetooth.js'
export {
  startScan,
  stopScan,
  write as nfcWrite,
} from './nfc.js'
export type { NFCRecord, NFCMessage } from './nfc.js'
export {
  registerSync,
  isPermissionGranted as isBackgroundPermissionGranted,
  requestPermission as requestBackgroundPermission,
} from './background.js'
export {
  subscribe as pushSubscribe,
  unsubscribe as pushUnsubscribe,
  getSubscription as pushGetSubscription,
} from './push.js'
export type { PushSubscriptionData } from './push.js'
export {
  share,
} from './share.js'
export type { ShareData } from './share.js'
export {
  storeGet,
  storeSet,
  storeDelete,
  storeKeys,
  storeClear,
} from './store.js'
export {
  checkForUpdate,
  applyUpdate,
  onUpdateProgress,
} from './updater.js'
export {
  enableAutostart,
  disableAutostart,
  isAutostartEnabled,
} from './autostart.js'
export {
  getInitialURL,
  onDeepLink,
} from './deeplink.js'
export type { UpdateInfo, UpdateProgress } from './updater.js'
export {
  openWindow,
  closeWindow,
  listWindows,
  quitApp,
  getCapabilities,
  isWindowingSupported,
  isTraySupported,
} from './window.js'
export type { WindowOptions, Capabilities } from './window.js'
export { setMenu, onMenu, menuSupported } from './menu.js'
export type { MenuItemSpec } from './menu.js'
export type {
  OSInfo,
  PlatformInfo,
  InvokeRequest,
  InvokeResponse,
  EventMessage,
  InvokeHandler,
  EventCallback,
  BridgeConfig,
} from './types.js'

import { getBridge } from './bridge.js'
import type { BridgeConfig, OSInfo, PlatformInfo } from './types.js'

let initialized = false

export async function initBridge(config?: BridgeConfig): Promise<void> {
  if (initialized) return
  const bridge = getBridge(config)
  await bridge.connect()
  initialized = true
}

export async function invoke<T = unknown>(method: string, args?: Record<string, unknown>): Promise<T> {
  const bridge = getBridge()
  return bridge.invoke<T>(method, args)
}

export function on(event: string, callback: (data: unknown) => void): () => void {
  const bridge = getBridge()
  return bridge.on(event, callback)
}

export function off(event: string, callback: (data: unknown) => void): void {
  const bridge = getBridge()
  return bridge.off(event, callback)
}

export async function getOSInfo(): Promise<OSInfo> {
  return invoke<OSInfo>('goleo:getOS')
}

export async function getPlatformInfo(): Promise<PlatformInfo> {
  return invoke<PlatformInfo>('goleo:getPlatform')
}

export async function getArch(): Promise<string> {
  return invoke<string>('goleo:getArch')
}

export async function getEnv(key: string): Promise<string> {
  return invoke<string>('goleo:getEnv', { key })
}

export async function openURL(url: string): Promise<void> {
  await invoke<void>('goleo:openURL', { url })
}

export function disconnect(): void {
  const bridge = getBridge()
  bridge.disconnect()
  initialized = false
}

export function isConnected(): boolean {
  const bridge = getBridge()
  return bridge.isConnected()
}

/**
 * True when the bridge is using the desktop webview's in-process channel
 * (Config.NativeIPC) rather than a WebSocket or HTTP POST.
 */
export function isNative(): boolean {
  const bridge = getBridge()
  return bridge.isNative()
}

/**
 * Reconnect to the Go backend, resetting the retry counter.
 *
 * There was previously no way back from local-only mode: if the initial connect
 * timed out (default 3s) or the retries were exhausted, the bridge stayed local for
 * the rest of the session and every non-core method threw "backend not connected" —
 * even once the backend was up. A slow-starting backend permanently disabled the
 * app. Wire this to a retry button, or to the `bridge:reconnectFailed` event.
 */
export async function reconnect(): Promise<void> {
  const bridge = getBridge()
  await bridge.reconnect()
}

export function sendEvent(event: string, data?: Record<string, unknown>): void {
  const bridge = getBridge()
  bridge.sendEvent(event, data)
}

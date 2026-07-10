export { Bridge, getBridge } from './bridge'
export {
  sendNotification,
  isPermissionGranted,
  requestPermission,
} from './notification'
export type { NotificationOptions, NotificationPermission } from './notification'
export {
  readText as clipboardReadText,
  writeText as clipboardWriteText,
} from './clipboard'
export {
  openFile,
  openFiles,
  saveFile,
  selectFolder,
  showMessage,
  showPrompt,
} from './dialogs'
export type { FileFilter, FileDialogOptions, MessageBoxOptions, PromptOptions } from './dialogs'
export {
  readTextFile,
  writeTextFile,
  readBinaryFile,
  writeBinaryFile,
  listDir,
  deleteFile,
  appDataDir,
  homeDir,
} from './fs'
export type { FileEntry } from './fs'
export {
  getCurrentPosition,
} from './geolocation'
export type { Position, PositionOptions } from './geolocation'
export {
  getBatteryInfo,
} from './battery'
export type { BatteryInfo } from './battery'
export {
  wakeLockRequest,
  wakeLockRelease,
} from './wakelock'
export {
  vibrate,
} from './vibration'
export {
  startSensor,
  stopSensor,
  startBrowserSensor,
  startNativeSensor,
} from './sensors'
export type { SensorData } from './sensors'
export {
  capturePhoto,
} from './camera'
export type { PhotoData } from './camera'
export {
  requestDevice,
  connect as bleConnect,
  disconnect as bleDisconnect,
} from './bluetooth'
export type { BLEDevice } from './bluetooth'
export {
  startScan,
  stopScan,
  write as nfcWrite,
} from './nfc'
export type { NFCRecord, NFCMessage } from './nfc'
export {
  registerSync,
  isPermissionGranted as isBackgroundPermissionGranted,
  requestPermission as requestBackgroundPermission,
} from './background'
export {
  subscribe as pushSubscribe,
  unsubscribe as pushUnsubscribe,
  getSubscription as pushGetSubscription,
} from './push'
export type { PushSubscriptionData } from './push'
export {
  share,
} from './share'
export type { ShareData } from './share'
export {
  storeGet,
  storeSet,
  storeDelete,
  storeKeys,
  storeClear,
} from './store'
export {
  checkForUpdate,
  applyUpdate,
  onUpdateProgress,
} from './updater'
export {
  enableAutostart,
  disableAutostart,
  isAutostartEnabled,
} from './autostart'
export {
  getInitialURL,
  onDeepLink,
} from './deeplink'
export type { UpdateInfo, UpdateProgress } from './updater'
export {
  openWindow,
  closeWindow,
  listWindows,
  quitApp,
  getCapabilities,
  isWindowingSupported,
  isTraySupported,
} from './window'
export type { WindowOptions, Capabilities } from './window'
export type {
  OSInfo,
  PlatformInfo,
  InvokeRequest,
  InvokeResponse,
  EventMessage,
  InvokeHandler,
  EventCallback,
  BridgeConfig,
} from './types'

import { getBridge } from './bridge'
import type { BridgeConfig, OSInfo, PlatformInfo } from './types'

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

export function sendEvent(event: string, data?: Record<string, unknown>): void {
  const bridge = getBridge()
  bridge.sendEvent(event, data)
}

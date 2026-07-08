export { Bridge, getBridge } from './bridge'
export {
  sendNotification,
  isPermissionGranted,
  requestPermission,
} from './notification'
export type { NotificationOptions, NotificationPermission } from './notification'
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
  return invoke<void>('goleo:openURL', { url })
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

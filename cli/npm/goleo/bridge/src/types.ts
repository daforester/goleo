export interface OSInfo {
  os: string
  arch: string
  name: string
  version?: string
}

export interface PlatformInfo {
  platform: string
  isMobile: boolean
  isDesktop: boolean
  isBrowser: boolean
}

export interface InvokeRequest {
  id: string
  method: string
  args?: Record<string, unknown>
}

export interface InvokeResponse {
  id: string
  result?: unknown
  error?: string
}

export interface EventMessage {
  event: string
  data?: unknown
}

export type InvokeHandler = (args?: Record<string, unknown>) => Promise<unknown>

export type EventCallback = (data: unknown) => void

export interface BridgeConfig {
  serverUrl?: string
  wsUrl?: string
  autoReconnect?: boolean
  reconnectInterval?: number
  maxReconnectAttempts?: number
}

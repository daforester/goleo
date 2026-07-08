import type { BridgeConfig, InvokeRequest, InvokeResponse, EventMessage, EventCallback } from './types'

class Bridge {
  private ws: WebSocket | null = null
  private httpUrl: string
  private wsUrl: string
  private pending = new Map<string, { resolve: (v: unknown) => void; reject: (e: Error) => void }>()
  private eventHandlers = new Map<string, Set<EventCallback>>()
  private localHandlers = new Map<string, (args?: Record<string, unknown>) => Promise<unknown>>()
  private requestId = 0
  private connected = false
  private ready = false
  private readyPromise: Promise<void>
  private readyResolve!: () => void
  private reconnectAttempts = 0
  private config: Required<BridgeConfig>

  constructor(config: BridgeConfig = {}) {
    this.config = {
      serverUrl: config.serverUrl || 'http://localhost:9842',
      wsUrl: config.wsUrl || 'ws://localhost:9842/ws',
      backend: config.backend ?? true,
      autoReconnect: config.autoReconnect ?? true,
      reconnectInterval: config.reconnectInterval ?? 3000,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 3,
      connectionTimeout: config.connectionTimeout ?? 3000,
    }
    this.httpUrl = this.config.serverUrl
    this.wsUrl = this.config.wsUrl
    this.readyPromise = new Promise((resolve) => { this.readyResolve = resolve })
  }

  handleLocal(method: string, handler: (args?: Record<string, unknown>) => Promise<unknown>): void {
    this.localHandlers.set(method, handler)
  }

  async connect(): Promise<void> {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) return

    if (!this.config.backend) {
      this.enterLocalMode()
      return
    }

    return new Promise((resolve) => {
      try {
        this.ws = new WebSocket(this.wsUrl)
      } catch {
        this.enterLocalMode()
        resolve()
        return
      }

      const timeout = setTimeout(() => {
        if (!this.connected) {
          this.ws?.close()
          this.enterLocalMode()
          resolve()
        }
      }, this.config.connectionTimeout)

      this.ws.onopen = () => {
        clearTimeout(timeout)
        this.connected = true
        this.reconnectAttempts = 0
        this.ready = true
        this.readyResolve()
        this.emitEvent('bridge:connected', {})
        resolve()
      }

      this.ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data)
          this.handleMessage(msg)
        } catch (err) {
          console.error('[goleo] failed to parse message:', err)
        }
      }

      this.ws.onclose = () => {
        this.connected = false
        this.emitEvent('bridge:disconnected', {})
        if (this.config.autoReconnect && this.ready) {
          this.attemptReconnect()
        }
      }

      this.ws.onerror = () => {
        if (!this.connected) {
          clearTimeout(timeout)
          this.ws?.close()
          this.enterLocalMode()
          resolve()
        }
      }
    })
  }

  private enterLocalMode(): void {
    this.connected = false
    this.ready = true
    this.readyResolve()
    this.registerLocalHandlers()
    this.emitEvent('bridge:disconnected', {})
    console.log('[goleo] running in local-only mode (no backend detected)')
  }

  private registerLocalHandlers(): void {
    this.handleLocal('goleo:notify', async (args) => {
      const { title, body, message } = (args || {}) as { title?: string; body?: string; message?: string }
      if (typeof window !== 'undefined' && 'Notification' in window && window.Notification.permission === 'granted') {
        new window.Notification(title || '', { body: body || message || '' })
      }
    })

    this.handleLocal('goleo:notificationPermissionGranted', async () => {
      if (typeof window !== 'undefined' && 'Notification' in window) {
        return window.Notification.permission === 'granted'
      }
      return false
    })

    this.handleLocal('goleo:requestNotificationPermission', async () => {
      if (typeof window !== 'undefined' && 'Notification' in window) {
        return window.Notification.requestPermission()
      }
      return 'denied' as NotificationPermission
    })

    this.handleLocal('goleo:getOS', async () => {
      const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
      const platform = typeof navigator !== 'undefined' ? navigator.platform : ''
      return {
        os: platform || 'web',
        arch: '',
        name: ua,
        version: '',
      }
    })

    this.handleLocal('goleo:getPlatform', async () => {
      const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
      const isMobile = /Android|iPhone|iPad|iPod|webOS/i.test(ua)
      return {
        platform: isMobile ? 'mobile' : 'web',
        isMobile,
        isDesktop: !isMobile && typeof window !== 'undefined',
        isBrowser: typeof window !== 'undefined',
      }
    })

    this.handleLocal('goleo:getArch', async () => '')

    this.handleLocal('goleo:getEnv', async () => '')

    this.handleLocal('goleo:openURL', async (args) => {
      const { url } = (args || {}) as { url?: string }
      if (url && typeof window !== 'undefined') {
        window.open(url, '_blank')
      }
    })
  }

  private attemptReconnect(): void {
    if (this.reconnectAttempts >= this.config.maxReconnectAttempts) {
      console.error('[goleo] max reconnection attempts reached')
      this.emitEvent('bridge:reconnectFailed', {})
      return
    }

    this.reconnectAttempts++
    console.log(`[goleo] reconnecting (attempt ${this.reconnectAttempts}/${this.config.maxReconnectAttempts})...`)
    this.emitEvent('bridge:reconnecting', { attempt: this.reconnectAttempts })

    setTimeout(() => {
      this.connect().catch(() => {})
    }, this.config.reconnectInterval)
  }

  private handleMessage(msg: { type: string; data?: unknown }): void {
    switch (msg.type) {
      case 'invokeResult': {
        const data = msg.data as InvokeResponse
        const pending = this.pending.get(data.id)
        if (pending) {
          this.pending.delete(data.id)
          if (data.error) {
            pending.reject(new Error(data.error))
          } else {
            pending.resolve(data.result)
          }
        }
        break
      }

      case 'event': {
        const eventMsg = msg.data as EventMessage
        if (eventMsg?.event) {
          this.emitEvent(eventMsg.event, eventMsg.data)
        }
        break
      }

      case 'pong':
        break

      default:
        console.warn('[goleo] unknown message type:', msg.type)
    }
  }

  async invoke<T = unknown>(method: string, args?: Record<string, unknown>): Promise<T> {
    await this.readyPromise

    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return this.invokeWS<T>(method, args)
    }

    if (this.connected) {
      return this.invokeHTTP<T>(method, args)
    }

    const localHandler = this.localHandlers.get(method)
    if (localHandler) {
      return localHandler(args) as Promise<T>
    }

    throw new Error(`backend not connected: cannot invoke "${method}"`)
  }

  private invokeWS<T>(method: string, args?: Record<string, unknown>): Promise<T> {
    return new Promise((resolve, reject) => {
      const id = String(++this.requestId)
      const request: InvokeRequest = { id, method, args }

      this.pending.set(id, {
        resolve: (v) => resolve(v as T),
        reject,
      })

      this.ws!.send(JSON.stringify({
        type: 'invoke',
        data: request,
      }))

      setTimeout(() => {
        if (this.pending.has(id)) {
          this.pending.delete(id)
          reject(new Error(`invoke timeout: ${method}`))
        }
      }, 30000)
    })
  }

  private async invokeHTTP<T>(method: string, args?: Record<string, unknown>): Promise<T> {
    const id = String(++this.requestId)
    const request: InvokeRequest = { id, method, args }

    const response = await fetch(`${this.httpUrl}/api/invoke`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(request),
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}: ${response.statusText}`)
    }

    const data: InvokeResponse = await response.json()
    if (data.error) {
      throw new Error(data.error)
    }

    return data.result as T
  }

  on(event: string, callback: EventCallback): () => void {
    if (!this.eventHandlers.has(event)) {
      this.eventHandlers.set(event, new Set())
    }
    this.eventHandlers.get(event)!.add(callback)

    return () => {
      this.eventHandlers.get(event)?.delete(callback)
    }
  }

  off(event: string, callback: EventCallback): void {
    this.eventHandlers.get(event)?.delete(callback)
  }

  private emitEvent(event: string, data: unknown): void {
    const handlers = this.eventHandlers.get(event)
    if (handlers) {
      handlers.forEach((callback) => {
        try {
          callback(data)
        } catch (err) {
          console.error(`[goleo] error in event handler for "${event}":`, err)
        }
      })
    }
  }

  disconnect(): void {
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.connected = false
    this.ready = false
  }

  sendEvent(event: string, data?: Record<string, unknown>): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({
        type: 'event',
        data: { event, data },
      }))
    }
  }

  isConnected(): boolean {
    return this.connected
  }

  isReady(): boolean {
    return this.ready
  }
}

let bridge: Bridge | null = null

function getBridge(config?: BridgeConfig): Bridge {
  if (!bridge) {
    bridge = new Bridge(config)
  }
  return bridge
}

export { Bridge, getBridge }

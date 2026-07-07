import type { BridgeConfig, InvokeRequest, InvokeResponse, EventMessage, EventCallback } from './types'

class Bridge {
  private ws: WebSocket | null = null
  private httpUrl: string
  private wsUrl: string
  private pending = new Map<string, { resolve: (v: unknown) => void; reject: (e: Error) => void }>()
  private eventHandlers = new Map<string, Set<EventCallback>>()
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
      autoReconnect: config.autoReconnect ?? true,
      reconnectInterval: config.reconnectInterval ?? 3000,
      maxReconnectAttempts: config.maxReconnectAttempts ?? 10,
    }
    this.httpUrl = this.config.serverUrl
    this.wsUrl = this.config.wsUrl
    this.readyPromise = new Promise((resolve) => { this.readyResolve = resolve })
  }

  async connect(): Promise<void> {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) return

    return new Promise((resolve, reject) => {
      try {
        this.ws = new WebSocket(this.wsUrl)
      } catch (err) {
        reject(err)
        return
      }

      this.ws.onopen = () => {
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
        if (this.config.autoReconnect) {
          this.attemptReconnect()
        }
      }

      this.ws.onerror = (err) => {
        if (!this.connected) {
          reject(err)
        }
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

    return this.invokeHTTP<T>(method, args)
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

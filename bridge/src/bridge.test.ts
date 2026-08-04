import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { Bridge } from './index'

// Every other suite mocks ./bridge away, so the Bridge class itself — including
// everything Phase 3 added to it (reconnect, rejectPending, the backoff) — had no
// coverage at all. These drive the real class against a controllable socket.

type Listener = ((ev?: unknown) => void) | null

class FakeSocket {
  static instances: FakeSocket[] = []
  static OPEN = 1
  static CLOSED = 3

  readyState = 0
  onopen: Listener = null
  onclose: Listener = null
  onerror: Listener = null
  onmessage: ((ev: { data: string }) => void) | null = null
  sent: string[] = []

  constructor(public url: string) {
    FakeSocket.instances.push(this)
  }

  send(data: string) {
    this.sent.push(data)
  }

  close() {
    this.readyState = FakeSocket.CLOSED
    this.onclose?.()
  }

  // Test helpers.
  open() {
    this.readyState = FakeSocket.OPEN
    this.onopen?.()
  }
  deliver(msg: unknown) {
    this.onmessage?.({ data: JSON.stringify(msg) })
  }
  static latest() {
    return FakeSocket.instances[FakeSocket.instances.length - 1]
  }
  static reset() {
    FakeSocket.instances = []
  }
}

// invoke() awaits the bridge's readiness promise before it touches the socket, so
// the frame is not sent synchronously. Let the microtask queue drain first.
const flush = () => new Promise((r) => setTimeout(r, 0))

// Connect a bridge and return it together with ITS socket. Re-querying
// FakeSocket.latest() later is unsafe: a lingering async connect from an earlier
// test can push a newer instance and the assertions then read the wrong socket.
async function connected(overrides = {}) {
  const b = newBridge(overrides)
  const connecting = b.connect()
  const sock = FakeSocket.latest()
  sock.open()
  await connecting
  return { b, sock }
}

function newBridge(overrides = {}) {
  return new Bridge({
    wsUrl: 'ws://localhost:0/ws',
    connectionTimeout: 50,
    reconnectInterval: 10,
    maxReconnectAttempts: 2,
    ...overrides,
  })
}

beforeEach(() => {
  FakeSocket.reset()
  vi.stubGlobal('WebSocket', FakeSocket as unknown as typeof WebSocket)
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('in-flight invokes on disconnect', () => {
  // On close, pending invokes were simply abandoned. Each then sat until its own
  // 30s timeout and reported "invoke timeout" — a misleading message for a dropped
  // connection, and a 30s stall in the UI for something already known.
  it('rejects pending invokes immediately, with the real reason', async () => {
    const { b, sock } = await connected()

    const inflight = b.invoke('slow:call')
    await flush()
    // The request went out and has not been answered.
    expect(sock.sent.length).toBe(1)

    sock.close()

    await expect(inflight).rejects.toThrow(/connection closed/i)
  })

  it('does not leave the pending map populated after a close', async () => {
    const { b, sock } = await connected()

    const a = b.invoke('one')
    const c = b.invoke('two')
    await flush()
    sock.close()

    await expect(a).rejects.toThrow()
    await expect(c).rejects.toThrow()
  })
})

describe('reconnect()', () => {
  // Once the initial connect timed out or the retries were spent, the bridge stayed
  // in local-only mode for the rest of the session — every non-core method threw
  // "backend not connected" even after the backend came up. There was no way back.
  it('recovers a bridge that fell into local-only mode', async () => {
    vi.useFakeTimers()
    const b = newBridge({ autoReconnect: false })

    const first = b.connect()
    // Never open the socket: let the connection timeout fire.
    await vi.advanceTimersByTimeAsync(60)
    await first
    expect(b.isConnected()).toBe(false)

    vi.useRealTimers()

    // The backend is up now. Before reconnect() existed this was unrecoverable.
    const again = b.reconnect()
    await Promise.resolve()
    FakeSocket.latest().open()
    await again

    expect(b.isConnected()).toBe(true)
  })

  it('opens a fresh socket rather than reusing the dead one', async () => {
    vi.useFakeTimers()
    const b = newBridge({ autoReconnect: false })
    const first = b.connect()
    await vi.advanceTimersByTimeAsync(60)
    await first
    const beforeCount = FakeSocket.instances.length
    vi.useRealTimers()

    const again = b.reconnect()
    await Promise.resolve()
    FakeSocket.latest().open()
    await again

    expect(FakeSocket.instances.length).toBeGreaterThan(beforeCount)
  })
})

describe('reconnect backoff', () => {
  // A fixed 3s retry means every client of a restarting backend reconnects in
  // lockstep, and three evenly-spaced attempts give a slow starter no more time
  // than a fast one.
  it('grows the delay between attempts and jitters it', async () => {
    const b = newBridge({ reconnectInterval: 1000, maxReconnectAttempts: 5 })
    const delay = (b as unknown as { reconnectDelay: () => number }).reconnectDelay.bind(b)
    const attempts = b as unknown as { reconnectAttempts: number }

    const sample = (n: number) => {
      attempts.reconnectAttempts = n
      return Array.from({ length: 40 }, delay)
    }

    const first = sample(1)
    const third = sample(3)

    // Jittered to 50-100% of the backed-off value, so never a fixed number.
    expect(new Set(first).size).toBeGreaterThan(1)
    expect(Math.min(...first)).toBeGreaterThanOrEqual(500)
    expect(Math.max(...first)).toBeLessThanOrEqual(1000)

    // Attempt 3 backs off to 4x the base before jitter.
    expect(Math.max(...third)).toBeGreaterThan(Math.max(...first))
    expect(Math.max(...third)).toBeLessThanOrEqual(4000)

    // And it is capped, so a long outage does not schedule a retry hours away.
    attempts.reconnectAttempts = 20
    expect(delay()).toBeLessThanOrEqual(8000)
  })
})

describe('invoke wire format', () => {
  // The Go side decodes {id, method, args} out of a {type, data} envelope
  // (runtime/websocket.go). If this shape drifts, every call fails at runtime while
  // both sides' own unit tests still pass.
  it('sends the envelope the Go bridge parses', async () => {
    const { b, sock } = await connected()

    void b.invoke('goleo:fsReadTextFile', { path: '/x' })
    await flush()

    const frame = JSON.parse(sock.sent[0])
    expect(frame.type).toBe('invoke')
    expect(frame.data.method).toBe('goleo:fsReadTextFile')
    expect(frame.data.args).toEqual({ path: '/x' })
    expect(typeof frame.data.id).toBe('string')
  })

  it('resolves the matching request when a result arrives', async () => {
    const { b, sock } = await connected()

    const call = b.invoke<string>('echo')
    await flush()
    const id = JSON.parse(sock.sent[0]).data.id
    sock.deliver({ type: 'invokeResult', data: { id, result: 'hi' } })

    await expect(call).resolves.toBe('hi')
  })

  it('rejects with the backend error text, which the fs/dialog wrappers rely on', async () => {
    const { b, sock } = await connected()

    const call = b.invoke('goleo:fsWriteTextFile')
    await flush()
    const id = JSON.parse(sock.sent[0]).data.id
    sock.deliver({
      type: 'invokeResult',
      data: { id, error: 'fs: "/etc/x" is outside the allowed roots' },
    })

    await expect(call).rejects.toThrow(/outside the allowed roots/)
  })
})

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const invoke = vi.fn()

vi.mock('./index', () => ({
  invoke: (...args: unknown[]) => invoke(...args),
}))

// capsCache is module-level state that survives between tests, so each test needs a
// fresh module instance — otherwise one test's cached answer silently satisfies the
// next and the caching assertions prove nothing.
async function freshWindow() {
  vi.resetModules()
  return import('./window')
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('capability caching', () => {
  // The fallback used to be memoised: if the first query happened before the
  // backend connected — easy, since nothing orders these — the all-false answer was
  // cached for the whole session and openWindow threw "not supported" forever, even
  // on a desktop that supports it. Only a real answer may be cached.
  it('retries after a failure instead of caching the fallback', async () => {
    const { isWindowingSupported } = await freshWindow()

    invoke.mockRejectedValueOnce(new Error('not connected yet'))
    await expect(isWindowingSupported()).resolves.toBe(false)

    // Backend is up now — the next query must actually ask again.
    invoke.mockResolvedValueOnce({ windowing: true, tray: true, menu: true })
    await expect(isWindowingSupported()).resolves.toBe(true)
    expect(invoke).toHaveBeenCalledTimes(2)
  })

  it('caches a successful answer for the session', async () => {
    const { getCapabilities } = await freshWindow()

    invoke.mockResolvedValue({ windowing: true, tray: false, menu: true })
    await getCapabilities()
    await getCapabilities()
    await getCapabilities()
    expect(invoke).toHaveBeenCalledTimes(1)
  })

  it('exposes menu, which the Go side has always returned', async () => {
    const { getCapabilities } = await freshWindow()

    invoke.mockResolvedValue({ windowing: true, tray: true, menu: true })
    await expect(getCapabilities()).resolves.toHaveProperty('menu', true)
  })
})

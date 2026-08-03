import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const invoke = vi.fn()
const isConnected = vi.fn()

vi.mock('./bridge', () => ({
  getBridge: () => ({ invoke, isConnected }),
}))

const { storeGet, storeSet, storeDelete, storeKeys } = await import('./store')

beforeEach(() => {
  localStorage.clear()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('localStorage fallback', () => {
  // The old code fell back to localStorage on ANY error, which silently split the
  // store in two: a transient backend failure (or a forgotten RegisterStore) sent
  // that one write to localStorage while everything else kept using the Go store,
  // so reads and writes landed in different places with nothing to indicate it.
  it('does not divert a write to localStorage when the backend is present', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('store: disk full'))

    await expect(storeSet('token', 'abc')).rejects.toThrow('disk full')
    expect(localStorage.getItem('goleo:store:token')).toBeNull()
  })

  it('does not read from localStorage when the backend is present', async () => {
    localStorage.setItem('goleo:store:token', JSON.stringify('stale-local'))
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('store: transient failure'))

    await expect(storeGet('token')).rejects.toThrow('transient failure')
  })

  it.each([
    ['storeDelete', () => storeDelete('k')],
    ['storeKeys', () => storeKeys()],
  ])('%s surfaces the backend error rather than falling back', async (_n, call) => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('store: boom'))
    await expect(call()).rejects.toThrow('boom')
  })

  it('uses localStorage when there genuinely is no backend', async () => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))

    await storeSet('token', 'abc')
    expect(localStorage.getItem('goleo:store:token')).toBe(JSON.stringify('abc'))
    await expect(storeGet('token')).resolves.toBe('abc')
    await expect(storeKeys()).resolves.toEqual(['token'])

    await storeDelete('token')
    await expect(storeGet('token')).resolves.toBeUndefined()
  })
})

describe('backend path', () => {
  it('returns undefined for a key the backend does not have', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue({ value: null, found: false })
    await expect(storeGet('missing')).resolves.toBeUndefined()
  })

  it('returns the stored value when found', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue({ value: { a: 1 }, found: true })
    await expect(storeGet('k')).resolves.toEqual({ a: 1 })
  })
})

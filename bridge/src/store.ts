import { getBridge } from './bridge.js'

const bridge = () => getBridge()

// Prefix for the localStorage fallback (PWA / no-backend mode).
const LS_PREFIX = 'goleo:store:'

function ls(): Storage | undefined {
  return typeof localStorage !== 'undefined' ? localStorage : undefined
}

// backendPresent separates "the backend rejected this" from "there is no backend".
// Asked of the bridge instance directly rather than importing isConnected from
// ./index, which re-exports this module (circular import).
function backendPresent(): boolean {
  try {
    return bridge().isConnected()
  } catch {
    return false
  }
}

// useFallback decides whether a failure may be answered with localStorage.
//
// It may not, if a backend is present. The old code fell back on ANY error, which
// silently split the store in two: a transient backend failure (or a developer
// forgetting RegisterStore) sent that write to localStorage while every other
// operation kept using the Go store, so reads and writes landed in different places
// with nothing to indicate it. Persistence that quietly changes location is worse
// than persistence that fails.
function useFallback(op: string, e: unknown): never | void {
  if (backendPresent()) {
    throw e instanceof Error ? e : new Error(`${op}: ${String(e)}`)
  }
}

/** Read a value from the persistent key/value store. Returns undefined if absent. */
export async function storeGet<T = unknown>(key: string): Promise<T | undefined> {
  try {
    const res = await bridge().invoke<{ value: T | null; found: boolean }>('goleo:storeGet', { key })
    return res.found ? (res.value as T) : undefined
  } catch (e) {
    useFallback('store', e)
    const s = ls()
    if (s) {
      const raw = s.getItem(LS_PREFIX + key)
      return raw === null ? undefined : (JSON.parse(raw) as T)
    }
    throw new Error('store not available')
  }
}

/** Write a value to the persistent key/value store. */
export async function storeSet(key: string, value: unknown): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:storeSet', { key, value })
  } catch (e) {
    useFallback('store', e)
    const s = ls()
    if (s) {
      s.setItem(LS_PREFIX + key, JSON.stringify(value))
      return
    }
    throw new Error('store not available')
  }
}

/** Delete a key from the store. */
export async function storeDelete(key: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:storeDelete', { key })
  } catch (e) {
    useFallback('store', e)
    const s = ls()
    if (s) {
      s.removeItem(LS_PREFIX + key)
      return
    }
    throw new Error('store not available')
  }
}

/** List all keys in the store. */
export async function storeKeys(): Promise<string[]> {
  try {
    const res = await bridge().invoke<{ keys: string[] }>('goleo:storeKeys')
    return res.keys ?? []
  } catch (e) {
    useFallback('store', e)
    const s = ls()
    if (s) {
      const keys: string[] = []
      for (let i = 0; i < s.length; i++) {
        const k = s.key(i)
        if (k && k.startsWith(LS_PREFIX)) keys.push(k.slice(LS_PREFIX.length))
      }
      return keys
    }
    throw new Error('store not available')
  }
}

/** Remove all keys from the store. */
export async function storeClear(): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:storeClear')
  } catch (e) {
    useFallback('store', e)
    const s = ls()
    if (s) {
      for (const k of await storeKeys()) s.removeItem(LS_PREFIX + k)
      return
    }
    throw new Error('store not available')
  }
}

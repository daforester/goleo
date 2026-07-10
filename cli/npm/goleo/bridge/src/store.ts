import { getBridge } from './bridge'

const bridge = () => getBridge()

// Prefix for the localStorage fallback (PWA / no-backend mode).
const LS_PREFIX = 'goleo:store:'

function ls(): Storage | undefined {
  return typeof localStorage !== 'undefined' ? localStorage : undefined
}

/** Read a value from the persistent key/value store. Returns undefined if absent. */
export async function storeGet<T = unknown>(key: string): Promise<T | undefined> {
  try {
    const res = await bridge().invoke<{ value: T | null; found: boolean }>('goleo:storeGet', { key })
    return res.found ? (res.value as T) : undefined
  } catch {
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
  } catch {
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
  } catch {
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
  } catch {
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
  } catch {
    const s = ls()
    if (s) {
      for (const k of await storeKeys()) s.removeItem(LS_PREFIX + k)
      return
    }
    throw new Error('store not available')
  }
}

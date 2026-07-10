import { invoke } from './index'

/** Options for opening an additional native window. */
export interface WindowOptions {
  title?: string
  width?: number
  height?: number
  /** Load this URL verbatim. */
  url?: string
  /** Load the app's own server root plus this path (e.g. "/settings"). */
  path?: string
}

/** Desktop capabilities the running platform supports. */
export interface Capabilities {
  /** Additional native windows can be opened (desktop only). */
  windowing: boolean
  /** A system tray icon is available (desktop only). */
  tray: boolean
}

let capsCache: Promise<Capabilities> | undefined

/**
 * Query which desktop capabilities the running platform supports. The result
 * is cached for the session. Falls back to "nothing supported" if the backend
 * is unavailable (e.g. pure PWA with no Go process).
 */
export function getCapabilities(): Promise<Capabilities> {
  if (!capsCache) {
    capsCache = invoke<Capabilities>('goleo:capabilities').catch(
      () => ({ windowing: false, tray: false }),
    )
  }
  return capsCache
}

/** Whether additional native windows can be opened on this platform. */
export async function isWindowingSupported(): Promise<boolean> {
  return (await getCapabilities()).windowing
}

/** Whether a system tray is available on this platform. */
export async function isTraySupported(): Promise<boolean> {
  return (await getCapabilities()).tray
}

/**
 * Open an additional native window (desktop only). Rejects with a clear error
 * on platforms without windowing (mobile/PWA) rather than failing obscurely.
 * Resolves to the new window's id.
 */
export async function openWindow(opts: WindowOptions = {}): Promise<number> {
  if (!(await isWindowingSupported())) {
    throw new Error('goleo: windowing is not supported on this platform')
  }
  const res = await invoke<{ id: number }>('goleo:windowOpen', opts as Record<string, unknown>)
  return res.id
}

/** Close a window previously opened with {@link openWindow}. */
export async function closeWindow(id: number): Promise<void> {
  if (!(await isWindowingSupported())) {
    throw new Error('goleo: windowing is not supported on this platform')
  }
  await invoke<void>('goleo:windowClose', { id })
}

/**
 * List the ids of all currently open managed windows. Returns an empty array
 * (rather than throwing) on platforms without windowing.
 */
export async function listWindows(): Promise<number[]> {
  if (!(await isWindowingSupported())) return []
  const res = await invoke<{ ids: number[] }>('goleo:windowList')
  return res.ids
}

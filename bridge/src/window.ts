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
  /** Quit the whole app when this window closes (default false). */
  exitOnClose?: boolean
}

/** Request a graceful app shutdown (desktop). */
export async function quitApp(): Promise<void> {
  await invoke<void>('goleo:quit')
}

/** Desktop capabilities the running platform supports. */
export interface Capabilities {
  /** Additional native windows can be opened (desktop only). */
  windowing: boolean
  /** A system tray icon is available (desktop only). */
  tray: boolean
  /** A native menu bar can be installed (desktop only). */
  menu: boolean
}

let capsCache: Promise<Capabilities> | undefined

const NO_CAPABILITIES: Capabilities = { windowing: false, tray: false, menu: false }

/**
 * Query which desktop capabilities the running platform supports. A successful
 * answer is cached for the session; a failure is NOT.
 *
 * Caching the failure was a real trap: if the first call happened before the
 * backend connected — easy, since nothing orders these — the all-false fallback was
 * memoised for the whole session, and openWindow then threw "not supported" forever
 * even on a desktop that supports it. Only a real answer is worth remembering.
 */
export function getCapabilities(): Promise<Capabilities> {
  if (!capsCache) {
    capsCache = invoke<Capabilities>('goleo:capabilities').catch(() => {
      capsCache = undefined // let the next call try again
      return NO_CAPABILITIES
    })
  }
  return capsCache
}

/** Whether a native menu bar can be installed on this platform. */
export async function isMenuSupported(): Promise<boolean> {
  return (await getCapabilities()).menu
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

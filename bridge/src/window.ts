import { invoke } from './index.js'

/**
 * A window's decorations and behaviour.
 *
 * Every property is optional and OMITTING ONE IS NOT THE SAME AS PASSING FALSE: an
 * omitted property keeps the app's Config.Chrome value, and failing that the OS default —
 * which for all four is the on/true-ish one (windows are resizable and decorated). Pass
 * false explicitly to turn one off.
 */
export interface WindowChrome {
  /** Whether the user can resize the window. */
  resizable?: boolean
  /** Keep the window above other applications' windows. */
  alwaysOnTop?: boolean
  /** Open fullscreen. Maximizes rather than going borderless on Windows. */
  fullscreen?: boolean
  /** Title bar and border. false gives a frameless window. */
  decorations?: boolean
}

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
  /** Decorations and behaviour; each property falls back to the app's Config.Chrome. */
  chrome?: WindowChrome
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

// Note: there is deliberately no isMenuSupported here. menu.ts has exported
// menuSupported() since before this file grew capability helpers, and adding a
// second public name for the same question would be worse than the naming
// inconsistency. menuSupported() now delegates to getCapabilities().

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

import { getBridge } from './bridge.js'
import { getCapabilities } from './window.js'

/** One entry in a native menu (see App.SetMenu / Config.Menu on the Go side). */
export interface MenuItemSpec {
  /** Stable id; a leaf item with an id fires a `menu:<id>` event when clicked. */
  id?: string
  label?: string
  /**
   * Standard action wired to the OS (so Cmd/Ctrl shortcuts work): one of
   * quit, copy, paste, cut, selectAll, undo, redo, minimize, close.
   */
  role?: string
  /** e.g. "cmd+q", "cmd+shift+z" (macOS accelerators). */
  accelerator?: string
  separator?: boolean
  submenu?: MenuItemSpec[]
}

/**
 * Install the application menu bar natively. Resolves on all platforms with a
 * native menu bar (macOS/Windows/Linux); rejects with an ErrUnsupported-style
 * error elsewhere (PWA/mobile) — catch it and render an in-page menu instead.
 *
 * Handle clicks with onMenu(id, cb) (or bridge.on(`menu:${id}`, cb)).
 */
export async function setMenu(items: MenuItemSpec[]): Promise<void> {
  await getBridge().invoke('goleo:setMenu', { items })
}

/** Subscribe to clicks of the menu item with the given id. Returns unsubscribe. */
export function onMenu(id: string, cb: () => void): () => void {
  return getBridge().on(`menu:${id}`, () => cb())
}

/**
 * Whether the running platform has a native menu bar (from goleo:capabilities).
 *
 * Delegates to getCapabilities() rather than invoking directly, so it shares the
 * same caching and the same "no backend means not supported" behaviour as
 * isWindowingSupported/isTraySupported. Invoking directly meant this one threw
 * where its two siblings returned false, and re-queried the backend on every call.
 */
export async function menuSupported(): Promise<boolean> {
  return (await getCapabilities()).menu
}

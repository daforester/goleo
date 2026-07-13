import { getBridge } from './bridge'

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

/** Whether the running platform has a native menu bar (from goleo:capabilities). */
export async function menuSupported(): Promise<boolean> {
  const caps = await getBridge().invoke<Record<string, boolean>>('goleo:capabilities')
  return !!caps?.menu
}

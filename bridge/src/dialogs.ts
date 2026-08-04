import { getBridge } from './bridge.js'

const bridge = () => getBridge()

// backendPresent distinguishes "the backend rejected this" from "there is no
// backend". Every fallback below is only legitimate in the second case; treating a
// real failure as a fallback trigger is what made a failed dialog look like a user
// decision. Asked of the bridge instance directly rather than importing
// isConnected from ./index, which re-exports this module (circular import).
function backendPresent(): boolean {
  try {
    return bridge().isConnected()
  } catch {
    return false
  }
}

// dialogError rethrows the backend's own error when it is present, so a failing
// dialog surfaces as a failure instead of being silently converted into a value the
// caller cannot distinguish from a real user choice.
function dialogError(op: string, e: unknown): Error {
  if (e instanceof Error) return e
  return new Error(`${op}: ${String(e)}`)
}

export interface FileFilter {
  name: string
  patterns: string[]
}

export interface FileDialogOptions {
  title?: string
  defaultPath?: string
  filters?: FileFilter[]
  multiple?: boolean
}

export interface MessageBoxOptions {
  title?: string
  message: string
  icon?: 'info' | 'warning' | 'error' | 'question'
  buttons?: string[]
}

export interface PromptOptions {
  title?: string
  message: string
  defaultValue?: string
}

// pickFilesViaInput is the browser fallback for the file dialogs.
//
// IMPORTANT: it resolves file NAMES, not paths — a browser cannot expose a
// filesystem path. The Go-backed fs plugin takes paths, so a name from here cannot
// be handed to readTextFile(). Treat these values as display labels only; if you
// need to read the contents in a browser context, use the File objects directly.
//
// It also has to settle on cancellation. Dismissing a file picker fires no
// `change` event, so the original code's promise simply never resolved and the
// caller hung forever. Modern browsers fire `cancel` on the input; where they do
// not, a window-focus check after the dialog closes catches it.
function pickFilesViaInput(multiple: boolean): Promise<string[]> {
  const input = document.createElement('input')
  input.type = 'file'
  input.multiple = multiple

  return new Promise((resolve) => {
    let settled = false
    const finish = (names: string[]) => {
      if (settled) return
      settled = true
      window.removeEventListener('focus', onFocus)
      resolve(names)
    }
    const onFocus = () => {
      // Focus returning to the window means the picker closed. Give the change
      // event a moment to arrive first; if it does not, the user cancelled.
      setTimeout(() => {
        if (!input.files || input.files.length === 0) finish([])
      }, 300)
    }

    input.onchange = () => {
      finish(input.files ? Array.from(input.files).map((f) => f.name) : [])
    }
    input.oncancel = () => finish([])
    window.addEventListener('focus', onFocus)
    input.click()
  })
}

export async function openFile(options?: FileDialogOptions): Promise<string | null> {
  try {
    const result = await bridge().invoke<string[]>('goleo:dialogOpenFile', (options ?? {}) as Record<string, unknown>)
    if (!result || result.length === 0) return null
    return result[0]
  } catch (e) {
    if (backendPresent()) {
      throw dialogError('openFile', e)
    }
    const names = await pickFilesViaInput(options?.multiple ?? false)
    return names.length > 0 ? names[0] : null
  }
}

export async function openFiles(options?: FileDialogOptions): Promise<string[]> {
  try {
    return await bridge().invoke<string[]>('goleo:dialogOpenFile', { ...(options ?? {}), multiple: true } as Record<string, unknown>)
  } catch (e) {
    if (backendPresent()) {
      throw dialogError('openFiles', e)
    }
    return pickFilesViaInput(true)
  }
}

export async function saveFile(options?: FileDialogOptions): Promise<string | null> {
  try {
    return await bridge().invoke<string | null>('goleo:dialogSaveFile', (options ?? {}) as Record<string, unknown>)
  } catch (e) {
    // null is the caller's signal for "the user cancelled". Returning it on a
    // FAILURE told the caller the user had declined, hiding the error entirely.
    // There is no browser equivalent for choosing a save path or a directory, so
    // when there is no backend this is genuinely unsupported — say so.
    if (backendPresent()) {
      throw dialogError('saveFile', e)
    }
    throw new Error('saveFile requires the Go backend')
  }
}

export async function selectFolder(options?: FileDialogOptions): Promise<string | null> {
  try {
    return await bridge().invoke<string | null>('goleo:dialogSelectFolder', (options ?? {}) as Record<string, unknown>)
  } catch (e) {
    // null is the caller's signal for "the user cancelled". Returning it on a
    // FAILURE told the caller the user had declined, hiding the error entirely.
    // There is no browser equivalent for choosing a save path or a directory, so
    // when there is no backend this is genuinely unsupported — say so.
    if (backendPresent()) {
      throw dialogError('selectFolder', e)
    }
    throw new Error('selectFolder requires the Go backend')
  }
}

export async function showMessage(options: MessageBoxOptions): Promise<string> {
  try {
    const result = await bridge().invoke<{ button: string }>('goleo:dialogShowMessage', options as unknown as Record<string, unknown>)
    return result.button
  } catch (e) {
    // NEVER invent 'OK' on a failure. This is the most dangerous shape of the
    // swallow-and-substitute pattern: a confirmation dialog ("Delete all data?")
    // that failed for any reason — backend not connected, RegisterDialogs not
    // called, policy denial, zenity missing — returned exactly what a user
    // clicking OK returns, so the caller went ahead with the destructive action.
    if (backendPresent()) {
      throw dialogError('showMessage', e)
    }
    // No backend (browser/PWA): ask the user for real via the browser's own
    // dialog. A returned value must always reflect an actual decision.
    const buttons = options.buttons ?? []
    if (buttons.length >= 2) {
      const accepted = globalThis.confirm?.(options.message)
      if (accepted === undefined) {
        throw new Error('showMessage requires the Go backend or a browser confirm dialog')
      }
      return accepted ? buttons[0] : buttons[1]
    }
    if (typeof globalThis.alert !== 'function') {
      throw new Error('showMessage requires the Go backend or a browser alert dialog')
    }
    globalThis.alert(options.message)
    return buttons[0] ?? 'OK'
  }
}

export async function showPrompt(options: PromptOptions): Promise<string | null> {
  try {
    return await bridge().invoke<string | null>('goleo:dialogShowPrompt', options as unknown as Record<string, unknown>)
  } catch (e) {
    // A browser prompt is a real user decision, so it is a legitimate fallback —
    // but only when there is no backend. With a backend present, a failure must
    // not be answered by silently opening a different dialog.
    if (backendPresent()) {
      throw dialogError('showPrompt', e)
    }
    if (typeof globalThis.prompt !== 'function') {
      throw new Error('showPrompt requires the Go backend or a browser prompt dialog')
    }
    return globalThis.prompt(options.message) ?? null
  }
}

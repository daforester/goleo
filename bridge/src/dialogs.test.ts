import { afterEach, describe, expect, it, vi } from 'vitest'

const invoke = vi.fn()
const isConnected = vi.fn()

vi.mock('./bridge', () => ({
  getBridge: () => ({ invoke, isConnected }),
}))

const { showMessage, showPrompt, saveFile, selectFolder, openFile, openFiles } = await import('./dialogs')

afterEach(() => {
  // restoreAllMocks, not clearAllMocks: clear only resets call history, leaving
  // document.createElement spies installed. A later test that captured "the real"
  // createElement would then capture the previous test's mock and recurse until the
  // stack blew.
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('showMessage', () => {
  // The most dangerous swallow in the codebase: `catch { return 'OK' }` meant a
  // confirmation dialog that FAILED returned exactly what a user clicking OK
  // returns. A caller asking "Delete all data?" then proceeded with the deletion.
  it('never returns OK when the dialog fails and a backend is present', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('dialogs: zenity not found'))

    await expect(
      showMessage({ message: 'Delete all data?', buttons: ['Yes', 'No'] }),
    ).rejects.toThrow('zenity not found')
  })

  it('reflects a real user decision via confirm when there is no backend', async () => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))

    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false))
    // Declining must map to the SECOND button, not the first.
    await expect(
      showMessage({ message: 'Delete all data?', buttons: ['Yes', 'No'] }),
    ).resolves.toBe('No')

    vi.stubGlobal('confirm', vi.fn().mockReturnValue(true))
    await expect(
      showMessage({ message: 'Delete all data?', buttons: ['Yes', 'No'] }),
    ).resolves.toBe('Yes')
  })

  it('uses alert for a single-button message and returns that button', async () => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))
    const alert = vi.fn()
    vi.stubGlobal('alert', alert)

    await expect(showMessage({ message: 'Saved', buttons: ['Fine'] })).resolves.toBe('Fine')
    expect(alert).toHaveBeenCalledWith('Saved')
  })

  it('returns the backend button on success', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue({ button: 'Cancel' })
    await expect(showMessage({ message: 'x', buttons: ['OK', 'Cancel'] })).resolves.toBe('Cancel')
  })
})

describe('saveFile / selectFolder', () => {
  // null is the caller's signal for "the user cancelled". Returning it on a failure
  // told the caller the user had declined, hiding the error completely.
  it.each([
    ['saveFile', () => saveFile({ title: 'Save' })],
    ['selectFolder', () => selectFolder()],
  ])('%s throws rather than returning null on failure', async (_n, call) => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('dialog backend exploded'))
    await expect(call()).rejects.toThrow('dialog backend exploded')
  })

  it.each([
    ['saveFile', () => saveFile({ title: 'Save' })],
    ['selectFolder', () => selectFolder()],
  ])('%s reports unsupported when there is no backend', async (name, call) => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))
    await expect(call()).rejects.toThrow(`${name} requires the Go backend`)
  })

  it('still returns null when the user genuinely cancels', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue(null)
    await expect(saveFile()).resolves.toBeNull()
  })
})

describe('showPrompt', () => {
  it('does not silently open a browser prompt when the backend failed', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('prompt backend failed'))
    const prompt = vi.fn().mockReturnValue('typed')
    vi.stubGlobal('prompt', prompt)

    await expect(showPrompt({ message: 'Name?' })).rejects.toThrow('prompt backend failed')
    expect(prompt).not.toHaveBeenCalled()
  })

  it('falls back to the browser prompt when there is no backend', async () => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))
    vi.stubGlobal('prompt', vi.fn().mockReturnValue('typed'))
    await expect(showPrompt({ message: 'Name?' })).resolves.toBe('typed')
  })
})

describe('openFile fallback', () => {
  // Dismissing a file picker fires no `change` event, so the original promise never
  // settled and the caller hung forever. It must resolve on cancellation.
  it('settles when the picker is cancelled', async () => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))

    // Simulate a cancelled picker: click() fires `cancel`, never `change`.
    const realCreate = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag) as HTMLInputElement
      if (tag === 'input') {
        el.click = () => {
          setTimeout(() => el.oncancel?.(new Event('cancel')), 0)
        }
      }
      return el
    })

    await expect(openFile()).resolves.toBeNull()
  })

  it('resolves the chosen names when the picker returns files', async () => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('not connected'))

    const realCreate = document.createElement.bind(document)
    vi.spyOn(document, 'createElement').mockImplementation((tag: string) => {
      const el = realCreate(tag) as HTMLInputElement
      if (tag === 'input') {
        el.click = () => {
          Object.defineProperty(el, 'files', {
            value: [new File(['x'], 'chosen.txt')],
            configurable: true,
          })
          setTimeout(() => el.onchange?.(new Event('change')), 0)
        }
      }
      return el
    })

    await expect(openFiles()).resolves.toEqual(['chosen.txt'])
  })

  it('throws instead of opening a picker when the backend itself failed', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error('dialogOpenFile denied by policy'))
    const create = vi.spyOn(document, 'createElement')

    await expect(openFile()).rejects.toThrow('denied by policy')
    expect(create).not.toHaveBeenCalled()
  })
})

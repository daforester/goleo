import { afterEach, describe, expect, it, vi } from 'vitest'

// The module under test resolves the bridge lazily through getBridge(), so stub
// that rather than standing up a real WebSocket.
const invoke = vi.fn()
const isConnected = vi.fn()

vi.mock('./bridge', () => ({
  getBridge: () => ({
    invoke,
    isConnected,
  }),
}))

const { readTextFile, writeTextFile, listDir, deleteFile, appDataDir, homeDir } = await import('./fs')

afterEach(() => {
  vi.clearAllMocks()
})

describe('error reporting', () => {
  // Every fs wrapper used to do `catch { throw new Error('<op> requires the Go
  // backend') }`, which is wrong whenever the backend IS connected. It became
  // actively harmful once the filesystem plugin gained scope confinement: the Go
  // side returns an actionable refusal naming the offending path and the three ways
  // to allow it, and this layer replaced all of it with a message pointing at a
  // missing backend. That is the wrong problem entirely.
  const confinement =
    'fs: "C:\\\\Users\\\\me\\\\secret.txt" is outside the allowed roots. ' +
    'Add it with Policy.FSRoots, let the user pick it via a native dialog, ' +
    'or set Config.FSScope = FSScopeUnrestricted'

  const cases: Array<[string, () => Promise<unknown>]> = [
    ['readTextFile', () => readTextFile('/x')],
    ['writeTextFile', () => writeTextFile('/x', 'c')],
    ['listDir', () => listDir('/x')],
    ['deleteFile', () => deleteFile('/x')],
    ['appDataDir', () => appDataDir('app')],
    ['homeDir', () => homeDir()],
  ]

  it.each(cases)('%s preserves the backend error when connected', async (_name, call) => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue(new Error(confinement))

    await expect(call()).rejects.toThrow(confinement)
    // And specifically must NOT have been replaced by the misleading message.
    await expect(call()).rejects.not.toThrow(/requires the Go backend/)
  })

  it.each(cases)('%s reports a missing backend only when truly disconnected', async (name, call) => {
    isConnected.mockReturnValue(false)
    invoke.mockRejectedValue(new Error('connection refused'))

    await expect(call()).rejects.toThrow(`${name} requires the Go backend`)
  })

  it('wraps a non-Error rejection without losing its text', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockRejectedValue('plain string failure')

    await expect(readTextFile('/x')).rejects.toThrow(/plain string failure/)
  })
})

describe('happy paths', () => {
  it('readTextFile returns the backend value', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue('file contents')
    await expect(readTextFile('/x')).resolves.toBe('file contents')
    expect(invoke).toHaveBeenCalledWith('goleo:fsReadTextFile', { path: '/x' })
  })

  it('appDataDir defaults its appName', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue('/cfg/goleo')
    await appDataDir()
    expect(invoke).toHaveBeenCalledWith('goleo:fsAppDataDir', { appName: 'goleo' })
  })

  it('appDataDir passes an explicit appName through', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue('/cfg/goleo-demo')
    await appDataDir('goleo-demo')
    expect(invoke).toHaveBeenCalledWith('goleo:fsAppDataDir', { appName: 'goleo-demo' })
  })
})

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

const { readTextFile, writeTextFile, listDir, deleteFile, appDataDir, homeDir, readBinaryFile, writeBinaryFile } = await import('./fs')

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

describe('binary encoding', () => {
  // Both directions used TextEncoder/TextDecoder, which mangles anything that is
  // not valid UTF-8. The wire format is base64 — what encoding/json already uses
  // for a Go []byte — so these assert the exact bytes survive.
  const png = new Uint8Array([
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
    0xff, 0xfe, 0xfd, 0x00, 0x01, 0x80, 0xc0, 0xc1,
  ])
  const pngB64 = 'iVBORw0KGgr//v0AAYDAwQ=='

  it('writeBinaryFile sends base64, not TextDecoder output', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue(undefined)

    await writeBinaryFile('/blob.bin', png)

    expect(invoke).toHaveBeenCalledWith('goleo:fsWriteBinaryFile', {
      path: '/blob.bin',
      data: pngB64,
    })
  })

  it('readBinaryFile decodes base64 back to the exact bytes', async () => {
    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue({ data: pngB64 })

    const out = await readBinaryFile('/blob.bin')
    expect(Array.from(out)).toEqual(Array.from(png))
  })

  it('round-trips every byte value 0-255 without loss', async () => {
    const all = new Uint8Array(256)
    for (let i = 0; i < 256; i++) all[i] = i

    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue(undefined)
    await writeBinaryFile('/all.bin', all)
    const sent = invoke.mock.calls[0][1] as { data: string }

    invoke.mockResolvedValue({ data: sent.data })
    const back = await readBinaryFile('/all.bin')
    expect(Array.from(back)).toEqual(Array.from(all))
  })

  it('handles a payload larger than the fromCharCode argument limit', async () => {
    // String.fromCharCode(...bytes) blows the stack somewhere around a few hundred
    // KB — an ordinary file size — so the encoder chunks.
    const big = new Uint8Array(200_000).map((_, i) => i % 256)

    isConnected.mockReturnValue(true)
    invoke.mockResolvedValue(undefined)
    await expect(writeBinaryFile('/big.bin', big)).resolves.toBeUndefined()

    const sent = invoke.mock.calls[0][1] as { data: string }
    invoke.mockResolvedValue({ data: sent.data })
    const back = await readBinaryFile('/big.bin')
    expect(back.length).toBe(big.length)
    expect(back[0]).toBe(big[0])
    expect(back[back.length - 1]).toBe(big[big.length - 1])
  })
})

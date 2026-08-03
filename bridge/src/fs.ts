import { getBridge } from './bridge'

const bridge = () => getBridge()

// Preserve the backend's own error message instead of replacing every failure with
// "requires the Go backend".
//
// That blanket message is wrong whenever the backend IS connected, and it became
// actively harmful when the filesystem plugin gained scope confinement: the Go side
// returns an actionable message naming the offending path and the three ways to
// allow it, and this layer was throwing all of that away. A developer hitting the
// new confinement was told the backend was missing, which sends them to entirely
// the wrong place.
//
// Only claim the backend is absent when it actually is.
function fsError(op: string, e: unknown): Error {
  // Ask the bridge instance directly rather than importing isConnected from
  // ./index — index re-exports this module, so that would be a circular import.
  let connected = false
  try {
    connected = bridge().isConnected()
  } catch {
    connected = false
  }
  if (!connected) {
    return new Error(`${op} requires the Go backend`)
  }
  if (e instanceof Error) return e
  return new Error(`${op}: ${String(e)}`)
}


export interface FileEntry {
  name: string
  path: string
  isDir: boolean
  size: number
  modTime: string
}

export async function readTextFile(path: string): Promise<string> {
  try {
    return await bridge().invoke<string>('goleo:fsReadTextFile', { path })
  } catch (e) {
    throw fsError('readTextFile', e)
  }
}

export async function writeTextFile(path: string, content: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:fsWriteTextFile', { path, content })
  } catch (e) {
    throw fsError('writeTextFile', e)
  }
}

export async function readBinaryFile(path: string): Promise<Uint8Array> {
  try {
    const result = await bridge().invoke<{ data: string }>('goleo:fsReadBinaryFile', { path })
    const encoder = new TextEncoder()
    return encoder.encode(result.data)
  } catch (e) {
    throw fsError('readBinaryFile', e)
  }
}

export async function writeBinaryFile(path: string, data: Uint8Array): Promise<void> {
  try {
    const decoder = new TextDecoder()
    await bridge().invoke<void>('goleo:fsWriteBinaryFile', { path, data: decoder.decode(data) })
  } catch (e) {
    throw fsError('writeBinaryFile', e)
  }
}

export async function listDir(path: string): Promise<FileEntry[]> {
  try {
    return await bridge().invoke<FileEntry[]>('goleo:fsListDir', { path })
  } catch (e) {
    throw fsError('listDir', e)
  }
}

export async function deleteFile(path: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:fsDelete', { path })
  } catch (e) {
    throw fsError('deleteFile', e)
  }
}

export async function appDataDir(appName?: string): Promise<string> {
  try {
    return await bridge().invoke<string>('goleo:fsAppDataDir', { appName: appName ?? 'goleo' })
  } catch (e) {
    throw fsError('appDataDir', e)
  }
}

export async function homeDir(): Promise<string> {
  try {
    return await bridge().invoke<string>('goleo:fsHomeDir')
  } catch (e) {
    throw fsError('homeDir', e)
  }
}

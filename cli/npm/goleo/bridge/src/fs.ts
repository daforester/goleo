import { getBridge } from './bridge'

const bridge = () => getBridge()

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
  } catch {
    throw new Error('readTextFile requires the Go backend')
  }
}

export async function writeTextFile(path: string, content: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:fsWriteTextFile', { path, content })
  } catch {
    throw new Error('writeTextFile requires the Go backend')
  }
}

export async function readBinaryFile(path: string): Promise<Uint8Array> {
  try {
    const result = await bridge().invoke<{ data: string }>('goleo:fsReadBinaryFile', { path })
    const encoder = new TextEncoder()
    return encoder.encode(result.data)
  } catch {
    throw new Error('readBinaryFile requires the Go backend')
  }
}

export async function writeBinaryFile(path: string, data: Uint8Array): Promise<void> {
  try {
    const decoder = new TextDecoder()
    await bridge().invoke<void>('goleo:fsWriteBinaryFile', { path, data: decoder.decode(data) })
  } catch {
    throw new Error('writeBinaryFile requires the Go backend')
  }
}

export async function listDir(path: string): Promise<FileEntry[]> {
  try {
    return await bridge().invoke<FileEntry[]>('goleo:fsListDir', { path })
  } catch {
    throw new Error('listDir requires the Go backend')
  }
}

export async function deleteFile(path: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:fsDelete', { path })
  } catch {
    throw new Error('deleteFile requires the Go backend')
  }
}

export async function appDataDir(appName?: string): Promise<string> {
  try {
    return await bridge().invoke<string>('goleo:fsAppDataDir', { appName: appName ?? 'goleo' })
  } catch {
    throw new Error('appDataDir requires the Go backend')
  }
}

export async function homeDir(): Promise<string> {
  try {
    return await bridge().invoke<string>('goleo:fsHomeDir')
  } catch {
    throw new Error('homeDir requires the Go backend')
  }
}

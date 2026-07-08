import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface NFCRecord {
  type: string
  mediaType: string
  data: string
}

export interface NFCMessage {
  records: NFCRecord[]
}

export async function startScan(): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:nfcStartScan')
  } catch {
    if (typeof navigator !== 'undefined' && 'nfc' in navigator) {
      const reader = new (navigator as any).NDEFReader()
      await reader.scan()
      return
    }
    throw new Error('NFC API not available')
  }
}

export async function stopScan(): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:nfcStopScan')
  } catch {
    // no-op
  }
}

export async function write(message: NFCMessage): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:nfcWrite', message as unknown as Record<string, unknown>)
  } catch {
    throw new Error('NFC write requires Go backend')
  }
}

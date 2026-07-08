import { getBridge } from './bridge'

export async function readText(): Promise<string> {
  const bridge = getBridge()
  try {
    const result = await bridge.invoke<{ text: string }>('goleo:clipboardReadText')
    return result.text
  } catch {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      return navigator.clipboard.readText()
    }
    throw new Error('clipboard not available')
  }
}

export async function writeText(text: string): Promise<void> {
  const bridge = getBridge()
  try {
    await bridge.invoke<void>('goleo:clipboardWriteText', { text })
  } catch {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      await navigator.clipboard.writeText(text)
      return
    }
    throw new Error('clipboard not available')
  }
}

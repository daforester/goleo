import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface ShareData {
  title?: string
  text?: string
  url?: string
}

/**
 * Open the native share sheet. Prefers the Go backend (native share on mobile,
 * best-effort URL hand-off on desktop); falls back to the Web Share API — the
 * real share sheet inside WKWebView / Chromium WebViews — then to copying the
 * URL/text to the clipboard.
 */
export async function share(data: ShareData): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:share', data as Record<string, unknown>)
  } catch {
    const nav = typeof navigator !== 'undefined' ? (navigator as any) : undefined
    if (nav && typeof nav.share === 'function') {
      await nav.share(data)
      return
    }
    const text = data.url || data.text || ''
    if (text && nav && nav.clipboard) {
      await nav.clipboard.writeText(text)
      return
    }
    throw new Error('share API not available')
  }
}

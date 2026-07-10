import { getBridge } from './bridge'

const bridge = () => getBridge()

/**
 * The URL the app was launched with (e.g. from a myapp:// deep link), or "" if
 * none. Read this once on startup; subsequent deep links arrive via
 * {@link onDeepLink}.
 */
export async function getInitialURL(): Promise<string> {
  try {
    const res = await bridge().invoke<{ url: string }>('goleo:initialURL')
    return res.url
  } catch {
    return ''
  }
}

/**
 * Subscribe to deep links delivered while the app is already running (a second
 * launch is forwarded to this instance). Returns an unsubscribe function.
 */
export function onDeepLink(cb: (url: string) => void): () => void {
  return bridge().on('app:openURL', (d) => {
    const url = (d as { url?: string })?.url
    if (url) cb(url)
  })
}

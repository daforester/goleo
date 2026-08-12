import { getPlatformInfo, isConnected } from '@goleo/bridge'

// Support level of a feature on a given platform.
export type Support = 'yes' | 'partial' | 'no'
export type PlatformKey = 'desktop' | 'android' | 'ios' | 'pwa'

export interface PlatformSupport {
  desktop: Support
  android: Support
  ios: Support
  pwa: Support
}

export const PLATFORM_ORDER: PlatformKey[] = ['desktop', 'android', 'ios', 'pwa']

export const PLATFORM_LABELS: Record<PlatformKey, string> = {
  desktop: 'Desktop',
  android: 'Android',
  ios: 'iOS',
  pwa: 'PWA / Web',
}

export const SUPPORT_NOTE: Record<Support, string> = {
  yes: 'Supported on this platform.',
  partial: 'Partially supported — some capabilities may be missing or require extra setup. Try it and read any message below.',
  no: 'Not supported on this platform. The controls below will explain why.',
}

let cached: PlatformKey | null = null

// detectPlatform figures out which of the four target platforms the app is
// currently running on, so demos can highlight the relevant support badge.
export async function detectPlatform(): Promise<PlatformKey> {
  if (cached) return cached
  // A PWA build is tagged at build time by `goleo build pwa` / `goleo dev pwa`.
  if (import.meta.env.VITE_GOLEO_PLATFORM === 'pwa') {
    cached = 'pwa'
    return cached
  }
  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
  try {
    const info = await getPlatformInfo()
    if (info.isMobile) {
      cached = /iPhone|iPad|iPod/i.test(ua) ? 'ios' : 'android'
      return cached
    }
    // A desktop build talks to the Go backend; a plain browser with no backend
    // is treated as PWA/web.
    cached = isConnected() ? 'desktop' : 'pwa'
    return cached
  } catch {
    cached = 'pwa'
    return cached
  }
}

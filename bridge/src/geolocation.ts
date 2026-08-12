export interface Position {
  latitude: number
  longitude: number
  accuracy?: number
}

export interface PositionOptions {
  enableHighAccuracy?: boolean
  timeout?: number
  maximumAge?: number
}

function supportsBrowserGeolocation(): boolean {
  return typeof navigator !== 'undefined' && 'geolocation' in navigator
}

/**
 * Geolocation is a pure web feature on every platform — there is no Go-side
 * implementation to try first, so this calls navigator.geolocation directly.
 *
 * It used to `invoke('goleo:geolocationGetCurrentPosition')` and fall back to the
 * browser on failure. That handler no longer exists: of six platforms only Windows
 * ever had a native path, and it launched a PowerShell subprocess per call. The
 * webview reaches the same OS APIs itself, with the real permission UI.
 *
 * The app must still call `runtime.RegisterGeolocation` in Go. It registers no
 * handler, but it is what declares Android's ACCESS_FINE_LOCATION and iOS's
 * NSLocationWhenInUseUsageDescription — without which the WebView's own
 * geolocation request is denied.
 *
 * Rejects rather than inventing a position: a plausible-looking wrong coordinate
 * is worse than a failure the caller can handle.
 *
 * Only works while the page is alive and foregrounded. Background location is not
 * reachable through the web API.
 */
export function getCurrentPosition(options?: PositionOptions): Promise<Position> {
  return new Promise((resolve, reject) => {
    if (!supportsBrowserGeolocation()) {
      reject(new Error('geolocation not available'))
      return
    }
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        resolve({
          latitude: pos.coords.latitude,
          longitude: pos.coords.longitude,
          accuracy: pos.coords.accuracy,
        })
      },
      (err) => reject(err),
      {
        enableHighAccuracy: options?.enableHighAccuracy ?? false,
        timeout: options?.timeout ?? 10000,
        maximumAge: options?.maximumAge ?? 0,
      },
    )
  })
}

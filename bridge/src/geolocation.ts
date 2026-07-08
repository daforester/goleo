import { getBridge } from './bridge'

const bridge = () => getBridge()

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

function browserGetCurrentPosition(options?: PositionOptions): Promise<Position> {
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

export async function getCurrentPosition(options?: PositionOptions): Promise<Position> {
  try {
    return await bridge().invoke<Position>('goleo:geolocationGetCurrentPosition', options as Record<string, unknown>)
  } catch {
    return browserGetCurrentPosition(options)
  }
}

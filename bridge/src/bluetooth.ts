import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface BLEDevice {
  id: string
  name: string
  rssi?: number
}

let gattServer: any | null = null

export async function requestDevice(filters?: Record<string, unknown>): Promise<BLEDevice> {
  try {
    return await bridge().invoke<BLEDevice>('goleo:bleRequestDevice', filters)
  } catch {
    if (typeof navigator !== 'undefined' && 'bluetooth' in navigator) {
      const device = await (navigator as any).bluetooth.requestDevice(filters ?? { acceptAllDevices: true })
      return { id: device.id, name: device.name ?? 'Unknown', rssi: 0 }
    }
    throw new Error('Bluetooth API not available')
  }
}

export async function connect(deviceId: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:bleConnect', { deviceId })
  } catch (e) {
    if (typeof navigator !== 'undefined' && 'bluetooth' in navigator) {
      // NOTE: deviceId is an opaque id, not a name. Matching it as a name is
      // wrong, and re-prompting with a picker is not a reconnect — but Web
      // Bluetooth offers no way to reconnect by id without a prior permission
      // grant, so this remains the closest available behaviour. Callers should not
      // assume the device they get back is the one they asked for.
      const device = await (navigator as any).bluetooth.requestDevice({ filters: [{ name: deviceId }] })
      gattServer = await device.gatt.connect()
      return
    }
    // Falling through here used to RESOLVE, reporting a connection that was never
    // made: no backend, no Web Bluetooth, and the caller was told it succeeded.
    throw e instanceof Error ? e : new Error(`bleConnect: ${String(e)}`)
  }
}

export async function disconnect(deviceId: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:bleDisconnect', { deviceId })
  } catch {
    gattServer?.disconnect()
    gattServer = null
  }
}

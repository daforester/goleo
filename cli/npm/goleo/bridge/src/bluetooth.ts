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
  } catch {
    if (typeof navigator !== 'undefined' && 'bluetooth' in navigator) {
      const device = await (navigator as any).bluetooth.requestDevice({ filters: [{ name: deviceId }] })
      gattServer = await device.gatt.connect()
    }
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

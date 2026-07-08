import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface BatteryInfo {
  level: number
  charging: boolean
  chargingTime?: number
  dischargingTime?: number
}

export async function getBatteryInfo(): Promise<BatteryInfo> {
  try {
    return await bridge().invoke<BatteryInfo>('goleo:batteryGetInfo')
  } catch {
    if (typeof navigator !== 'undefined' && 'getBattery' in navigator) {
      const b = await (navigator as any).getBattery()
      return {
        level: b.level,
        charging: b.charging,
        chargingTime: b.chargingTime,
        dischargingTime: b.dischargingTime,
      }
    }
    throw new Error('battery API not available')
  }
}

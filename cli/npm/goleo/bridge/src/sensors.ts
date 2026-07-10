import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface SensorData {
  type: string
  x: number
  y: number
  z: number
  timestamp: number
}

export async function startSensor(type: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:sensorStart', { type })
  } catch {
    // Browser Sensor API is available in secure contexts only
    throw new Error('sensor start requires Go backend or secure context')
  }
}

export async function stopSensor(type: string): Promise<void> {
  try {
    await bridge().invoke<void>('goleo:sensorStop', { type })
  } catch {
    // no-op
  }
}

// Native sensor reading: arms the platform sensor manager (Android
// SensorManager / iOS CoreMotion, see MainActivity.java's GoleoSensors) and
// delivers readings as goleo:sensorReading bridge events. Throws if there's
// no native provider registered — callers should catch and fall back to
// startBrowserSensor.
export async function startNativeSensor(
  type: string,
  callback: (data: SensorData) => void,
): Promise<() => void> {
  await startSensor(type)
  const handler = (data: unknown) => {
    const reading = data as SensorData
    if (reading.type === type) callback(reading)
  }
  bridge().on('goleo:sensorReading', handler)
  return () => {
    bridge().off('goleo:sensorReading', handler)
    stopSensor(type).catch(() => {})
  }
}

// Browser-side sensor reading (only works in secure contexts)
export function startBrowserSensor(type: string, callback: (data: SensorData) => void): () => void {
  if (typeof window === 'undefined') {
    throw new Error('sensors not available outside browser')
  }
  const SensorMap: Record<string, new (opts: { frequency: number }) => any> = {
    accelerometer: (window as any).Accelerometer,
    gyroscope: (window as any).Gyroscope,
    magnetometer: (window as any).Magnetometer,
  }
  const SensorClass = SensorMap[type]
  if (!SensorClass) throw new Error(`unsupported sensor: ${type}`)
  const sensor = new SensorClass({ frequency: 60 })
  sensor.addEventListener('reading', () => {
    callback({
      type,
      x: sensor.x ?? 0,
      y: sensor.y ?? 0,
      z: sensor.z ?? 0,
      timestamp: Date.now(),
    })
  })
  sensor.start()
  return () => sensor.stop()
}

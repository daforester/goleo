import { invoke } from './index'

/** Register the app to launch on login (desktop only). */
export async function enableAutostart(): Promise<void> {
  await invoke<void>('goleo:autostartEnable')
}

/** Remove the launch-on-login entry. */
export async function disableAutostart(): Promise<void> {
  await invoke<void>('goleo:autostartDisable')
}

/** Whether the app is registered to launch on login. */
export async function isAutostartEnabled(): Promise<boolean> {
  const res = await invoke<{ enabled: boolean }>('goleo:autostartIsEnabled')
  return res.enabled
}

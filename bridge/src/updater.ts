import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface UpdateInfo {
  available: boolean
  version?: string
  notes?: string
}

export interface UpdateProgress {
  done: number
  total: number
}

/** Check whether a newer signed release is available for this platform. */
export async function checkForUpdate(): Promise<UpdateInfo> {
  return bridge().invoke<UpdateInfo>('goleo:updaterCheck')
}

/**
 * Download and apply the latest update, then relaunch. On success the process
 * is replaced, so this call does not return normally. Subscribe with
 * onUpdateProgress first to show a progress bar.
 */
export async function applyUpdate(): Promise<void> {
  await bridge().invoke<void>('goleo:updaterApply')
}

/** Subscribe to download progress; returns an unsubscribe function. */
export function onUpdateProgress(cb: (p: UpdateProgress) => void): () => void {
  return bridge().on('updater:progress', (d) => cb(d as UpdateProgress))
}

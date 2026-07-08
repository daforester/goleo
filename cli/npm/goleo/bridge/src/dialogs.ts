import { getBridge } from './bridge'

const bridge = () => getBridge()

export interface FileFilter {
  name: string
  patterns: string[]
}

export interface FileDialogOptions {
  title?: string
  defaultPath?: string
  filters?: FileFilter[]
  multiple?: boolean
}

export interface MessageBoxOptions {
  title?: string
  message: string
  icon?: 'info' | 'warning' | 'error' | 'question'
  buttons?: string[]
}

export interface PromptOptions {
  title?: string
  message: string
  defaultValue?: string
}

export async function openFile(options?: FileDialogOptions): Promise<string | null> {
  try {
    const result = await bridge().invoke<string[]>('goleo:dialogOpenFile', (options ?? {}) as Record<string, unknown>)
    if (!result || result.length === 0) return null
    return result[0]
  } catch {
    const input = document.createElement('input')
    input.type = 'file'
    if (options?.multiple) input.multiple = true
    return new Promise((resolve) => {
      input.onchange = () => {
        if (input.files && input.files.length > 0) {
          resolve(input.files[0].name)
        } else {
          resolve(null)
        }
      }
      input.click()
    })
  }
}

export async function openFiles(options?: FileDialogOptions): Promise<string[]> {
  try {
    return await bridge().invoke<string[]>('goleo:dialogOpenFile', { ...(options ?? {}), multiple: true } as Record<string, unknown>)
  } catch {
    const input = document.createElement('input')
    input.type = 'file'
    input.multiple = true
    return new Promise((resolve) => {
      input.onchange = () => {
        if (input.files) {
          resolve(Array.from(input.files).map((f) => f.name))
        } else {
          resolve([])
        }
      }
      input.click()
    })
  }
}

export async function saveFile(options?: FileDialogOptions): Promise<string | null> {
  try {
    return await bridge().invoke<string | null>('goleo:dialogSaveFile', (options ?? {}) as Record<string, unknown>)
  } catch {
    return null
  }
}

export async function selectFolder(options?: FileDialogOptions): Promise<string | null> {
  try {
    return await bridge().invoke<string | null>('goleo:dialogSelectFolder', (options ?? {}) as Record<string, unknown>)
  } catch {
    return null
  }
}

export async function showMessage(options: MessageBoxOptions): Promise<string> {
  try {
    const result = await bridge().invoke<{ button: string }>('goleo:dialogShowMessage', options as unknown as Record<string, unknown>)
    return result.button
  } catch {
    return 'OK'
  }
}

export async function showPrompt(options: PromptOptions): Promise<string | null> {
  try {
    return await bridge().invoke<string | null>('goleo:dialogShowPrompt', options as unknown as Record<string, unknown>)
  } catch {
    return prompt(options.message) ?? null
  }
}

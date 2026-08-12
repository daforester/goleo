// init.js — Goleo startup script.
//
// Runs inside the Go backend (embedded JS engine) before any window is
// shown, giving you full control over window creation. Available API:
//
//   getConfig()       -> { title, width, height, devMode, devServer, port, url }
//   createWindow(opts) - opts: title, width, height, minWidth, minHeight,
//                        center, devTools, url (defaults to the app's own URL)
//   console.log/info/warn/error
//
// Available bridge commands (call via bridge.invoke("goleo:xxx", { ... })):
//
//   Core:
//     goleo:getOS                          -> OSInfo
//     goleo:getPlatform                    -> PlatformInfo
//     goleo:getArch                        -> string
//     goleo:getEnv({ key })                -> string
//     goleo:openURL({ url })               -> void
//     goleo:notify({ title, body? })       -> void
//     goleo:showMessage({ title, message }) -> void
//
//   Clipboard:
//     goleo:clipboardReadText              -> { text }
//     goleo:clipboardWriteText({ text })   -> void
//
//   Dialogs:
//     goleo:dialogOpenFile({ ... })        -> string[]
//     goleo:dialogSaveFile({ ... })        -> string
//     goleo:dialogSelectFolder({ ... })    -> string
//     goleo:dialogShowMessage({ ... })     -> { button }
//     goleo:dialogShowPrompt({ ... })      -> string
//
//   File System:
//     goleo:fsReadTextFile({ path })       -> string
//     goleo:fsWriteTextFile({ path, content }) -> void
//     goleo:fsReadBinaryFile({ path })     -> { data }
//     goleo:fsWriteBinaryFile({ path, data }) -> void
//     goleo:fsListDir({ path })            -> FileEntry[]
//     goleo:fsDelete({ path })             -> void
//     goleo:fsAppDataDir({ appName? })     -> string
//     goleo:fsHomeDir                      -> string
//
//   Geolocation:
//     goleo:geolocationGetCurrentPosition({ ... }) -> Position
//
// Delete this file (and its embed line in main.go) to fall back to the
// built-in window setup from runtime.Config.

const config = getConfig()

createWindow({
  title: config.title,
  width: config.width,
  height: config.height,
  center: true,
})

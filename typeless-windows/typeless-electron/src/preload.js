const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('api', {
  onState:      (cb) => ipcRenderer.on('state', (_, s) => cb(s)),
  onSetText:    (cb) => ipcRenderer.on('set-text', (_, t) => cb(t)),
  getConfig:    ()      => ipcRenderer.invoke('get-config'),
  saveConfig:   (cfg)   => ipcRenderer.invoke('save-config', cfg),
  getHistory:   ()      => ipcRenderer.invoke('get-history'),
  clearHistory: ()      => ipcRenderer.invoke('clear-history'),
  copyText:     (text)  => ipcRenderer.invoke('copy-text', text),
  testOllama:   (cfg)   => ipcRenderer.invoke('test-ollama', cfg),
})

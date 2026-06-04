const { app, BrowserWindow, globalShortcut, ipcMain, Tray, Menu, clipboard, shell } = require('electron')
const path = require('path')
const fs = require('fs')
const os = require('os')
const { spawn, execSync } = require('child_process')
const { uIOhook, UiohookKey } = require('uiohook-napi')
const { Ollama } = require('ollama')

// ---------------------------------------------------------------------------
// Rutas
// ---------------------------------------------------------------------------
// En desarrollo apunta a C:\typeless, en producción al directorio del exe
const APP_DIR = app.isPackaged
  ? path.dirname(app.getPath('exe'))
  : path.join(__dirname, '..')
const DATA_DIR  = path.join(app.getPath('userData'))
const CFG_FILE  = path.join(DATA_DIR, 'config.json')
const HIST_FILE = path.join(DATA_DIR, 'history.json')

const defaultConfig = {
  whisperExe:   path.join(APP_DIR, 'whisper-cli.exe'),
  whisperModel: path.join(APP_DIR, 'ggml-small.bin'),
  csvPath:      path.join(APP_DIR, 'palabras.csv'),
  keywordReply: 'ok',
  ollamaModel:  'llama3',
  ollamaHost:   'http://127.0.0.1:11434',
  useOllama:    true,
  maxPasteLen:  400,
  language:     'es',
}

function loadConfig() {
  if (!fs.existsSync(CFG_FILE)) return { ...defaultConfig }
  try { return { ...defaultConfig, ...JSON.parse(fs.readFileSync(CFG_FILE, 'utf8')) } }
  catch { return { ...defaultConfig } }
}
function saveConfig(cfg) {
  if (!fs.existsSync(DATA_DIR)) fs.mkdirSync(DATA_DIR, { recursive: true })
  fs.writeFileSync(CFG_FILE, JSON.stringify(cfg, null, 2))
}

function loadHistory() {
  if (!fs.existsSync(HIST_FILE)) return []
  try { return JSON.parse(fs.readFileSync(HIST_FILE, 'utf8')) }
  catch { return [] }
}
function appendHistory(entry) {
  const hist = loadHistory()
  hist.unshift({ ...entry, ts: new Date().toISOString() })
  const trimmed = hist.slice(0, 200)
  if (!fs.existsSync(DATA_DIR)) fs.mkdirSync(DATA_DIR, { recursive: true })
  fs.writeFileSync(HIST_FILE, JSON.stringify(trimmed, null, 2))
}

function loadKeywords(csvPath) {
  if (!fs.existsSync(csvPath)) return []
  const lines = fs.readFileSync(csvPath, 'utf8').split('\n')
  return lines.map(l => l.split(',')[0].trim().toLowerCase()).filter(Boolean)
}

// ---------------------------------------------------------------------------
// Ventanas
// ---------------------------------------------------------------------------
let floatingWin = null
let copyWin     = null
let settingsWin = null
let historyWin  = null
let tray        = null

function createFloatingWindow() {
  floatingWin = new BrowserWindow({
    width: 320,
    height: 80,
    frame: false,
    transparent: true,
    alwaysOnTop: true,
    skipTaskbar: true,
    resizable: false,
    focusable: false,
    show: false,
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
    },
  })
  floatingWin.loadFile(path.join(__dirname, '..', 'renderer', 'floating.html'))
  floatingWin.setAlwaysOnTop(true, 'screen-saver')
}

function showFloating(state) {
  if (!floatingWin) return
  floatingWin.webContents.send('state', state)
  if (!floatingWin.isVisible()) {
    // Posicionar en esquina inferior derecha
    const { screen } = require('electron')
    const disp = screen.getPrimaryDisplay()
    const { width, height } = disp.workAreaSize
    floatingWin.setPosition(width - 340, height - 100)
    floatingWin.showInactive()
  }
}

function hideFloating() {
  if (floatingWin && floatingWin.isVisible()) floatingWin.hide()
}

function showCopyWindow(text) {
  if (copyWin && !copyWin.isDestroyed()) {
    copyWin.webContents.send('set-text', text)
    copyWin.show()
    copyWin.focus()
    return
  }
  copyWin = new BrowserWindow({
    width: 520,
    height: 300,
    frame: true,
    alwaysOnTop: true,
    resizable: true,
    title: 'Typeless — Copiar texto',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
    },
  })
  copyWin.loadFile(path.join(__dirname, '..', 'renderer', 'copy.html'))
  copyWin.once('ready-to-show', () => {
    copyWin.webContents.send('set-text', text)
    copyWin.show()
    copyWin.focus()
  })
  copyWin.on('closed', () => { copyWin = null })
}

function openSettings() {
  if (settingsWin && !settingsWin.isDestroyed()) { settingsWin.focus(); return }
  settingsWin = new BrowserWindow({
    width: 500,
    height: 520,
    title: 'Typeless — Configuración',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
    },
  })
  settingsWin.loadFile(path.join(__dirname, '..', 'renderer', 'settings.html'))
  settingsWin.on('closed', () => { settingsWin = null })
}

function openHistory() {
  if (historyWin && !historyWin.isDestroyed()) { historyWin.focus(); return }
  historyWin = new BrowserWindow({
    width: 600,
    height: 500,
    title: 'Typeless — Historial',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
    },
  })
  historyWin.loadFile(path.join(__dirname, '..', 'renderer', 'history.html'))
  historyWin.on('closed', () => { historyWin = null })
}

// ---------------------------------------------------------------------------
// Audio — grabación via ffmpeg
// ---------------------------------------------------------------------------
let ffmpegProc = null
let tmpWav     = path.join(os.tmpdir(), 'typeless_rec.wav')

function getAudioDevice(ffmpegPath) {
  try {
    const out = require('child_process').execSync(
      `"${ffmpegPath}" -list_devices true -f dshow -i dummy 2>&1`,
      { encoding: 'utf8', timeout: 5000, windowsHide: true }
    )
    const match = out.match(/"([^"]+)"\s*\(audio\)/)
    if (match) {
      console.log('[audio] Micrófono detectado:', match[1])
      return match[1]
    }
  } catch (e) {
    const out = e.stdout || e.stderr || e.message || ''
    const match = out.match(/"([^"]+)"\s*\(audio\)/)
    if (match) {
      console.log('[audio] Micrófono detectado:', match[1])
      return match[1]
    }
  }
  console.log('[audio] Usando dispositivo por defecto')
  return null
}

function startRecording() {
  const ffmpegPath = findFile(['ffmpeg.exe'], APP_DIR) || 'ffmpeg'
  const device = getAudioDevice(ffmpegPath)
  const input = device ? `audio=${device}` : 'audio='
  console.log('[audio] Grabando desde:', input)
  ffmpegProc = spawn(ffmpegPath, [
    '-y',
    '-f', 'dshow',
    '-i', input,
    '-ar', '16000',
    '-ac', '1',
    '-acodec', 'pcm_s16le',
    tmpWav,
  ], { windowsHide: true })
  ffmpegProc.stderr.on('data', d => console.log('[ffmpeg]', d.toString().trim()))
}

function stopRecording() {
  return new Promise(resolve => {
    if (!ffmpegProc) { resolve(false); return }
    ffmpegProc.stdin.write('q')
    ffmpegProc.on('close', () => {
      ffmpegProc = null
      const exists = fs.existsSync(tmpWav) && fs.statSync(tmpWav).size > 4096
      resolve(exists)
    })
    setTimeout(() => {
      if (ffmpegProc) { ffmpegProc.kill('SIGTERM'); ffmpegProc = null }
      resolve(fs.existsSync(tmpWav) && fs.statSync(tmpWav).size > 4096)
    }, 2000)
  })
}

// ---------------------------------------------------------------------------
// Transcripción — whisper.cpp
// ---------------------------------------------------------------------------
function transcribe(cfg) {
  return new Promise((resolve, reject) => {
    const outBase = tmpWav.replace('.wav', '')
    const args = [
      '--model', cfg.whisperModel,
      '--language', cfg.language,
      '--no-timestamps',
      '--output-txt',
      '--output-file', outBase,
      tmpWav,
    ]
    const proc = spawn(cfg.whisperExe, args, { windowsHide: true })
    proc.on('close', code => {
      const txtFile = outBase + '.txt'
      if (!fs.existsSync(txtFile)) { reject(new Error('whisper no generó salida')); return }
      const text = fs.readFileSync(txtFile, 'utf8').trim()
      fs.unlinkSync(txtFile)
      resolve(text)
    })
    proc.on('error', reject)
  })
}

// ---------------------------------------------------------------------------
// Corrección con Ollama
// ---------------------------------------------------------------------------
async function correctWithOllama(text, cfg) {
  try {
    const ollama = new Ollama({ host: cfg.ollamaHost })
    const res = await ollama.chat({
      model: cfg.ollamaModel,
      messages: [{
        role: 'user',
        content: `Corregí la gramática, puntuación y ortografía del siguiente texto dictado por voz en español.
Devolvé SOLO el texto corregido, sin explicaciones ni comillas.
Texto: ${text}`,
      }],
    })
    return res.message.content.trim()
  } catch (e) {
    console.error('[ollama error]', e.message)
    return text // si falla Ollama, devolver texto original
  }
}

// ---------------------------------------------------------------------------
// Pegar texto — clipboard + Ctrl+V via PowerShell
// ---------------------------------------------------------------------------
function pasteText(text) {
  clipboard.writeText(text)
  setTimeout(() => {
    try {
      execSync(`powershell -WindowStyle Hidden -Command "Add-Type -AssemblyName System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait('^v')"`,
        { windowsHide: true, timeout: 3000 })
    } catch (e) {
      console.error('[paste error]', e.message)
    }
  }, 80)
}

// ---------------------------------------------------------------------------
// Detectar palabra clave
// ---------------------------------------------------------------------------
function detectKeyword(text, keywords) {
  const lower = text.toLowerCase()
  return keywords.find(k => lower.includes(k)) || null
}

// ---------------------------------------------------------------------------
// Buscar archivo
// ---------------------------------------------------------------------------
function findFile(names, dir) {
  for (const name of names) {
    const p = path.join(dir, name)
    if (fs.existsSync(p)) return p
  }
  return null
}

// ---------------------------------------------------------------------------
// Pipeline principal
// ---------------------------------------------------------------------------
let isRecording = false

async function handleRelease() {
  if (!isRecording) return
  isRecording = false
  showFloating({ status: 'processing', text: 'Transcribiendo...' })

  const cfg = loadConfig()
  const hasAudio = await stopRecording()
  if (!hasAudio) { hideFloating(); return }

  let text
  try {
    text = await transcribe(cfg)
  } catch (e) {
    showFloating({ status: 'error', text: 'Error al transcribir' })
    setTimeout(hideFloating, 2000)
    return
  }

  if (!text) { hideFloating(); return }

  const keywords = loadKeywords(cfg.csvPath)
  const kw = detectKeyword(text, keywords)
  if (kw) {
    text = cfg.keywordReply
    showFloating({ status: 'keyword', text: `Clave: "${kw}"` })
  } else {
    if (cfg.useOllama) {
      showFloating({ status: 'processing', text: 'Corrigiendo...' })
      text = await correctWithOllama(text, cfg)
    }
    showFloating({ status: 'done', text })
  }

  appendHistory({ text, keyword: kw || null })

  setTimeout(() => {
    hideFloating()
    if (text.length > cfg.maxPasteLen) {
      showCopyWindow(text)
    } else {
      pasteText(text)
    }
  }, 800)
}

// ---------------------------------------------------------------------------
// App init
// ---------------------------------------------------------------------------
app.whenReady().then(() => {
  if (!fs.existsSync(DATA_DIR)) fs.mkdirSync(DATA_DIR, { recursive: true })

  createFloatingWindow()

  // Bandeja del sistema — ícono embebido en base64 (16x16 azul)
  const { nativeImage } = require('electron')
  const ICON_B64 = 'iVBORw0KGgoAAAANSUhEUgAAABAAAAAQCAYAAAAf8/9hAAAABmJLR0QA/wD/AP+gvaeTAAAAN0lEQVQ4jWNgGAWkAkYGBob/VMBQBwMDA8P/oXYBGRkZ/Q+1CxgYGBiG2gVkZGT0f6hdQBUAABCOBAVqamPKAAAAAElFTkSuQmCC'
  const trayIcon = nativeImage.createFromDataURL('data:image/png;base64,' + ICON_B64)
  tray = new Tray(trayIcon)
  tray.setToolTip('Typeless ES')
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: 'Historial',       click: openHistory },
    { label: 'Configuración',   click: openSettings },
    { type: 'separator' },
    { label: 'Salir',           click: () => app.quit() },
  ]))

  // Hotkey global: Numpad 5 — toggle
  console.log('[uiohook] Iniciando listener de teclado...')
  console.log('[uiohook] Numpad5 keycode esperado:', UiohookKey.Numpad5)
  uIOhook.on('keydown', e => {
    console.log('[keydown] keycode:', e.keycode)
    if (e.keycode !== UiohookKey.Numpad5) return
    if (!isRecording) {
      isRecording = true
      startRecording()
      showFloating({ status: 'recording', text: 'Escuchando...' })
    } else {
      handleRelease()
    }
  })
  try {
    uIOhook.start()
    console.log('[uiohook] OK — presioná Numpad 5')
  } catch (e) {
    console.error('[uiohook] ERROR:', e.message)
  }

  // IPC handlers
  ipcMain.handle('get-config',  () => loadConfig())
  ipcMain.handle('save-config', (_, cfg) => { saveConfig(cfg); return true })
  ipcMain.handle('get-history', () => loadHistory())
  ipcMain.handle('clear-history', () => { fs.writeFileSync(HIST_FILE, '[]'); return true })
  ipcMain.handle('copy-text', (_, text) => { clipboard.writeText(text); return true })
  ipcMain.handle('test-ollama', async (_, cfg) => {
    try {
      const ol = new Ollama({ host: cfg.ollamaHost })
      const res = await ol.list()
      return { ok: true, models: res.models.map(m => m.name) }
    } catch (e) {
      return { ok: false, error: e.message }
    }
  })
})

app.on('window-all-closed', e => e.preventDefault()) // seguir en bandeja

app.on('before-quit', () => {
  uIOhook.stop()
})

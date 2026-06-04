package main

import (
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Windows API — grabación de audio con winmm.dll (sin CGO extra)
// ---------------------------------------------------------------------------

var (
	winmm                  = syscall.NewLazyDLL("winmm.dll")
	waveInOpen             = winmm.NewProc("waveInOpen")
	waveInPrepareHeader    = winmm.NewProc("waveInPrepareHeader")
	waveInAddBuffer        = winmm.NewProc("waveInAddBuffer")
	waveInStart            = winmm.NewProc("waveInStart")
	waveInStop             = winmm.NewProc("waveInStop")
	waveInUnprepareHeader  = winmm.NewProc("waveInUnprepareHeader")
	waveInClose            = winmm.NewProc("waveInClose")

	user32         = syscall.NewLazyDLL("user32.dll")
	getAsyncKeyState = user32.NewProc("GetAsyncKeyState")
	sendInput      = user32.NewProc("SendInput")
	vkKeyScanA     = user32.NewProc("VkKeyScanA")

	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	setClipboard   = syscall.NewLazyDLL("user32.dll")
	openClipboard  = setClipboard.NewProc("OpenClipboard")
	emptyClipboard = setClipboard.NewProc("EmptyClipboard")
	setClipData    = setClipboard.NewProc("SetClipboardData")
	closeClipboard = setClipboard.NewProc("CloseClipboard")
	globalAlloc    = kernel32.NewProc("GlobalAlloc")
	globalLock     = kernel32.NewProc("GlobalLock")
	globalUnlock   = kernel32.NewProc("GlobalUnlock")
)

// WAVEFORMATEX para PCM 16kHz mono 16bit
type WAVEFORMATEX struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

// WAVEHDR
type WAVEHDR struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

const (
	WAVE_FORMAT_PCM  = 1
	WAVE_MAPPER      = 0xFFFFFFFF
	WHDR_DONE        = 0x00000001
	VK_RMENU         = 0xA5 // Alt derecho
	GMEM_MOVEABLE    = 0x0002
	CF_UNICODETEXT   = 13
)

// ---------------------------------------------------------------------------
// Grabación
// ---------------------------------------------------------------------------

const (
	sampleRate = 16000
	bufSeconds = 60 // máximo 60 segundos de grabación
	bufSize    = sampleRate * 2 * bufSeconds // 16bit = 2 bytes/sample
)

type Grabador struct {
	handle uintptr
	hdr    WAVEHDR
	buf    []byte
	fmt    WAVEFORMATEX
}

func nuevoGrabador() (*Grabador, error) {
	g := &Grabador{}
	g.fmt = WAVEFORMATEX{
		FormatTag:      WAVE_FORMAT_PCM,
		Channels:       1,
		SamplesPerSec:  sampleRate,
		AvgBytesPerSec: sampleRate * 2,
		BlockAlign:     2,
		BitsPerSample:  16,
	}
	g.buf = make([]byte, bufSize)

	ret, _, _ := waveInOpen.Call(
		uintptr(unsafe.Pointer(&g.handle)),
		WAVE_MAPPER,
		uintptr(unsafe.Pointer(&g.fmt)),
		0, 0, 0,
	)
	if ret != 0 {
		return nil, fmt.Errorf("waveInOpen falló: %d", ret)
	}
	return g, nil
}

func (g *Grabador) iniciar() error {
	g.hdr = WAVEHDR{
		Data:         uintptr(unsafe.Pointer(&g.buf[0])),
		BufferLength: uint32(len(g.buf)),
	}
	ret, _, _ := waveInPrepareHeader.Call(g.handle, uintptr(unsafe.Pointer(&g.hdr)), unsafe.Sizeof(g.hdr))
	if ret != 0 {
		return fmt.Errorf("prepareHeader falló: %d", ret)
	}
	ret, _, _ = waveInAddBuffer.Call(g.handle, uintptr(unsafe.Pointer(&g.hdr)), unsafe.Sizeof(g.hdr))
	if ret != 0 {
		return fmt.Errorf("addBuffer falló: %d", ret)
	}
	ret, _, _ = waveInStart.Call(g.handle)
	if ret != 0 {
		return fmt.Errorf("waveInStart falló: %d", ret)
	}
	return nil
}

func (g *Grabador) detener() ([]byte, float64) {
	waveInStop.Call(g.handle)
	time.Sleep(50 * time.Millisecond)
	waveInUnprepareHeader.Call(g.handle, uintptr(unsafe.Pointer(&g.hdr)), unsafe.Sizeof(g.hdr))
	grabbed := g.hdr.BytesRecorded
	if grabbed == 0 {
		return nil, 0
	}
	data := make([]byte, grabbed)
	copy(data, g.buf[:grabbed])
	duracion := float64(grabbed) / float64(sampleRate*2)
	return data, duracion
}

func (g *Grabador) cerrar() {
	waveInClose.Call(g.handle)
}

// ---------------------------------------------------------------------------
// Guardar WAV
// ---------------------------------------------------------------------------

func guardarWAV(ruta string, pcm []byte, sampleRate int) error {
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer f.Close()

	dataLen := uint32(len(pcm))
	w := bufio.NewWriter(f)

	// RIFF header
	w.WriteString("RIFF")
	binary.Write(w, binary.LittleEndian, uint32(36+dataLen))
	w.WriteString("WAVE")
	// fmt chunk
	w.WriteString("fmt ")
	binary.Write(w, binary.LittleEndian, uint32(16))
	binary.Write(w, binary.LittleEndian, uint16(1))        // PCM
	binary.Write(w, binary.LittleEndian, uint16(1))        // mono
	binary.Write(w, binary.LittleEndian, uint32(sampleRate))
	binary.Write(w, binary.LittleEndian, uint32(sampleRate*2))
	binary.Write(w, binary.LittleEndian, uint16(2))
	binary.Write(w, binary.LittleEndian, uint16(16))
	// data chunk
	w.WriteString("data")
	binary.Write(w, binary.LittleEndian, dataLen)
	w.Write(pcm)
	return w.Flush()
}

// ---------------------------------------------------------------------------
// Silencio — descartar grabaciones casi silenciosas
// ---------------------------------------------------------------------------

func rmsAudio(pcm []byte) float64 {
	if len(pcm) < 2 {
		return 0
	}
	var suma float64
	n := len(pcm) / 2
	for i := 0; i < n; i++ {
		s := int16(binary.LittleEndian.Uint16(pcm[i*2:]))
		suma += float64(s) * float64(s)
	}
	return math.Sqrt(suma / float64(n))
}

// ---------------------------------------------------------------------------
// Transcripción con whisper.cpp
// ---------------------------------------------------------------------------

func transcribir(wavPath, whisperExe, modeloPath string) (string, error) {
	outTxt := wavPath + ".txt"
	defer os.Remove(outTxt)

	cmd := exec.Command(
		whisperExe,
		"--model", modeloPath,
		"--language", "es",
		"--no-timestamps",
		"--output-txt",
		"--output-file", strings.TrimSuffix(wavPath, ".wav"),
		wavPath,
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper error: %v\n%s", err, out)
	}

	data, err := os.ReadFile(outTxt)
	if err != nil {
		return "", fmt.Errorf("no se encontró la salida de whisper: %v", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// ---------------------------------------------------------------------------
// Portapapeles y pegado
// ---------------------------------------------------------------------------

func copiarAlPortapapeles(texto string) error {
	utf16, err := syscall.UTF16FromString(texto)
	if err != nil {
		return err
	}
	size := uintptr(len(utf16) * 2)

	hMem, _, _ := globalAlloc.Call(GMEM_MOVEABLE, size)
	if hMem == 0 {
		return fmt.Errorf("GlobalAlloc falló")
	}
	ptr, _, _ := globalLock.Call(hMem)
	if ptr == 0 {
		return fmt.Errorf("GlobalLock falló")
	}
	for i, v := range utf16 {
		*(*uint16)(unsafe.Pointer(ptr + uintptr(i)*2)) = v
	}
	globalUnlock.Call(hMem)

	openClipboard.Call(0)
	emptyClipboard.Call()
	setClipData.Call(CF_UNICODETEXT, hMem)
	closeClipboard.Call()
	return nil
}

// INPUT para SendInput
type KEYBDINPUT struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}
type INPUT struct {
	Type uint32
	Ki   KEYBDINPUT
	_    [8]byte // padding
}

const (
	INPUT_KEYBOARD    = 1
	KEYEVENTF_KEYUP   = 0x0002
	VK_CONTROL        = 0x11
	VK_V              = 0x56
)

func pegarCtrlV() {
	inputs := []INPUT{
		{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{Vk: VK_CONTROL}},
		{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{Vk: VK_V}},
		{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{Vk: VK_V, Flags: KEYEVENTF_KEYUP}},
		{Type: INPUT_KEYBOARD, Ki: KEYBDINPUT{Vk: VK_CONTROL, Flags: KEYEVENTF_KEYUP}},
	}
	sendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
}

func pegarTexto(texto string) {
	if err := copiarAlPortapapeles(texto); err != nil {
		fmt.Println("[error portapapeles]", err)
		return
	}
	time.Sleep(50 * time.Millisecond)
	pegarCtrlV()
}

// ---------------------------------------------------------------------------
// CSV de palabras clave
// ---------------------------------------------------------------------------

func cargarCSV(ruta string) []string {
	var palabras []string
	f, err := os.Open(ruta)
	if err != nil {
		return palabras
	}
	defer f.Close()
	r := csv.NewReader(f)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(rec) == 0 {
			continue
		}
		w := strings.TrimSpace(strings.ToLower(rec[0]))
		if w != "" {
			palabras = append(palabras, w)
		}
	}
	fmt.Printf("[csv] %d palabras clave cargadas desde '%s'\n", len(palabras), ruta)
	return palabras
}

func detectarClave(texto string, claves []string) string {
	lower := strings.ToLower(texto)
	for _, c := range claves {
		if strings.Contains(lower, c) {
			return c
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Tecla Alt derecho
// ---------------------------------------------------------------------------

func altDerechoPresionado() bool {
	ret, _, _ := getAsyncKeyState.Call(VK_RMENU)
	return ret&0x8000 != 0
}

// ---------------------------------------------------------------------------
// Buscar whisper.exe y modelo automáticamente
// ---------------------------------------------------------------------------

func buscarArchivo(nombres []string) string {
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	for _, nombre := range nombres {
		ruta := filepath.Join(dir, nombre)
		if _, err := os.Stat(ruta); err == nil {
			return ruta
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	fmt.Println("=== Typeless para Windows 10 — Dictado en español ===")
	fmt.Println("Mantené presionado ALT DERECHO para hablar. Soltá para transcribir.")
	fmt.Println("Ctrl+C para salir.\n")

	// Buscar whisper-cli.exe o main.exe de whisper.cpp
	whisperExe := buscarArchivo([]string{"whisper-cli.exe", "main.exe", "whisper.exe"})
	if whisperExe == "" {
		fmt.Println("[error] No se encontró whisper-cli.exe en la misma carpeta.")
		fmt.Println("  Descargá whisper.cpp desde: https://github.com/ggerganov/whisper.cpp/releases")
		fmt.Println("  Copiá whisper-cli.exe y el modelo .bin en la misma carpeta que typeless.exe")
		os.Exit(1)
	}
	fmt.Println("[ok] whisper encontrado:", whisperExe)

	// Buscar modelo .bin
	modeloPath := buscarArchivo([]string{
		"ggml-small.bin", "ggml-base.bin", "ggml-medium.bin",
		"ggml-small-q5_1.bin", "ggml-base-q5_1.bin",
		"models/ggml-small.bin", "models/ggml-base.bin",
	})
	if modeloPath == "" {
		fmt.Println("[error] No se encontró ningún modelo .bin de whisper.cpp")
		fmt.Println("  Descargá el modelo desde: https://huggingface.co/ggerganov/whisper.cpp")
		fmt.Println("  Recomendado para español: ggml-small.bin (~244 MB)")
		os.Exit(1)
	}
	fmt.Println("[ok] modelo encontrado:", modeloPath)

	// CSV de palabras clave
	csvPath := buscarArchivo([]string{"palabras.csv", "keywords.csv"})
	var claves []string
	if csvPath != "" {
		claves = cargarCSV(csvPath)
	} else {
		fmt.Println("[aviso] No se encontró palabras.csv — no habrá detección de palabras clave")
	}

	respuesta := "ok"
	if len(os.Args) > 1 {
		respuesta = os.Args[1]
	}

	// Grabador
	grabador, err := nuevoGrabador()
	if err != nil {
		fmt.Println("[error] No se pudo abrir el micrófono:", err)
		os.Exit(1)
	}
	defer grabador.cerrar()

	tmpDir := os.TempDir()
	grabando := false

	for {
		if altDerechoPresionado() {
			if !grabando {
				grabando = true
				fmt.Print("[grabando...] ")
				if err := grabador.iniciar(); err != nil {
					fmt.Println("error:", err)
					grabando = false
				}
			}
		} else {
			if grabando {
				grabando = false
				pcm, dur := grabador.detener()
				fmt.Printf("%.1fs\n", dur)

				if dur < 0.5 {
					fmt.Println("[aviso] Muy corto, ignorado.")
					continue
				}
				if rmsAudio(pcm) < 200 {
					fmt.Println("[aviso] Silencio detectado, ignorado.")
					continue
				}

				wavPath := filepath.Join(tmpDir, "typeless_tmp.wav")
				if err := guardarWAV(wavPath, pcm, sampleRate); err != nil {
					fmt.Println("[error] No se pudo guardar WAV:", err)
					continue
				}

				fmt.Print("[transcribiendo...] ")
				texto, err := transcribir(wavPath, whisperExe, modeloPath)
				os.Remove(wavPath)
				if err != nil {
					fmt.Println("error:", err)
					continue
				}
				fmt.Println(texto)

				clave := detectarClave(texto, claves)
				if clave != "" {
					fmt.Printf("[clave '%s'] → pegando '%s'\n", clave, respuesta)
					pegarTexto(respuesta)
				} else {
					pegarTexto(texto)
				}

				// Reiniciar grabador para la próxima grabación
				grabador.cerrar()
				grabador, err = nuevoGrabador()
				if err != nil {
					fmt.Println("[error] No se pudo reiniciar el micrófono:", err)
					os.Exit(1)
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
}

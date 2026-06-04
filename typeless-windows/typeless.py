"""
Typeless clone para Windows 10 — dictado por voz en español.
Mantené presionada la tecla ALT DERECHO para grabar.
Al soltar, transcribe con Whisper y pega el texto donde esté el cursor.
Si detecta una palabra clave del CSV, responde "ok" en su lugar.

Requisitos:
    pip install openai-whisper pyaudio keyboard pyperclip pyautogui sounddevice numpy

Uso:
    python typeless.py
    python typeless.py --modelo medium    (más preciso, más lento)
    python typeless.py --csv palabras.csv (tus palabras clave)
"""

import argparse
import csv
import queue
import sys
import tempfile
import threading
import time
import wave
from pathlib import Path

import keyboard
import numpy as np
import pyaudio
import pyautogui
import pyperclip
import whisper

# ---------------------------------------------------------------------------
# Configuración de audio
# ---------------------------------------------------------------------------
SAMPLE_RATE = 16_000   # Whisper espera 16kHz
CHANNELS    = 1
FORMAT      = pyaudio.paInt16
CHUNK       = 1024

# ---------------------------------------------------------------------------
# Carga de palabras clave desde CSV
# ---------------------------------------------------------------------------

def cargar_palabras_clave(ruta_csv: str) -> list[str]:
    """
    Lee el CSV y devuelve una lista de palabras/frases en minúsculas.
    El CSV puede tener una columna (una palabra por fila) o varias columnas;
    en ese caso toma la primera columna. Acepta archivos con o sin encabezado.
    """
    palabras: list[str] = []
    ruta = Path(ruta_csv)
    if not ruta.exists():
        print(f"[aviso] No se encontró el CSV '{ruta_csv}'. Se usará lista vacía.")
        return palabras
    with open(ruta, newline="", encoding="utf-8-sig") as f:
        reader = csv.reader(f)
        for fila in reader:
            if fila:
                palabra = fila[0].strip().lower()
                if palabra:
                    palabras.append(palabra)
    print(f"[info] {len(palabras)} palabras clave cargadas desde '{ruta_csv}'.")
    return palabras


def detectar_palabra_clave(texto: str, palabras_clave: list[str]) -> str | None:
    """Devuelve la primera palabra clave encontrada en el texto, o None."""
    texto_lower = texto.lower()
    for palabra in palabras_clave:
        if palabra in texto_lower:
            return palabra
    return None


# ---------------------------------------------------------------------------
# Grabación de audio
# ---------------------------------------------------------------------------

class Grabador:
    def __init__(self):
        self._pa        = pyaudio.PyAudio()
        self._frames: list[bytes] = []
        self._grabando  = False
        self._stream    = None
        self._lock      = threading.Lock()

    def iniciar(self):
        with self._lock:
            if self._grabando:
                return
            self._frames   = []
            self._grabando = True
            self._stream   = self._pa.open(
                format=FORMAT,
                channels=CHANNELS,
                rate=SAMPLE_RATE,
                input=True,
                frames_per_buffer=CHUNK,
                stream_callback=self._callback,
            )
            self._stream.start_stream()
        print("[grabando...]")

    def _callback(self, data, frame_count, time_info, status):
        if self._grabando:
            self._frames.append(data)
        return (None, pyaudio.paContinue)

    def detener(self) -> np.ndarray | None:
        with self._lock:
            if not self._grabando:
                return None
            self._grabando = False
            self._stream.stop_stream()
            self._stream.close()
            self._stream = None

        if not self._frames:
            return None

        # Convertir bytes → float32 normalizado que espera Whisper
        raw   = b"".join(self._frames)
        audio = np.frombuffer(raw, dtype=np.int16).astype(np.float32) / 32768.0
        print(f"[audio capturado] {len(audio)/SAMPLE_RATE:.1f}s")
        return audio

    def cerrar(self):
        self._pa.terminate()


# ---------------------------------------------------------------------------
# Transcripción con Whisper
# ---------------------------------------------------------------------------

def transcribir(audio: np.ndarray, modelo) -> str:
    resultado = modelo.transcribe(
        audio,
        language="es",          # forzar español
        task="transcribe",
        fp16=False,             # False = compatible con CPU
    )
    return resultado["text"].strip()


# ---------------------------------------------------------------------------
# Inserción de texto en el cursor activo
# ---------------------------------------------------------------------------

def pegar_texto(texto: str):
    """Copia al portapapeles y pega con Ctrl+V en la ventana activa."""
    if not texto:
        return
    pyperclip.copy(texto)
    time.sleep(0.05)
    pyautogui.hotkey("ctrl", "v")


# ---------------------------------------------------------------------------
# Loop principal
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Typeless clone para Windows 10")
    parser.add_argument(
        "--modelo",
        default="small",
        choices=["tiny", "base", "small", "medium", "large"],
        help="Modelo Whisper (default: small ~244MB, buena relación calidad/velocidad)",
    )
    parser.add_argument(
        "--csv",
        default="palabras.csv",
        help="Ruta al CSV con palabras clave (default: palabras.csv)",
    )
    parser.add_argument(
        "--tecla",
        default="right alt",
        help="Tecla push-to-talk (default: 'right alt')",
    )
    parser.add_argument(
        "--respuesta",
        default="ok",
        help="Texto a pegar cuando se detecta una palabra clave (default: 'ok')",
    )
    args = parser.parse_args()

    print(f"[inicio] Cargando modelo Whisper '{args.modelo}'...")
    modelo = whisper.load_model(args.modelo)
    print("[inicio] Modelo listo.")

    palabras_clave = cargar_palabras_clave(args.csv)

    grabador   = Grabador()
    presionada = False

    print(f"\nMantené presionada [{args.tecla.upper()}] para dictar. Ctrl+C para salir.\n")

    try:
        while True:
            # Detectar pulsación de tecla push-to-talk
            if keyboard.is_pressed(args.tecla):
                if not presionada:
                    presionada = True
                    grabador.iniciar()
            else:
                if presionada:
                    presionada = False
                    audio = grabador.detener()
                    if audio is not None and len(audio) / SAMPLE_RATE > 0.3:
                        print("[transcribiendo...]")
                        texto = transcribir(audio, modelo)
                        print(f"[texto] {texto}")

                        clave = detectar_palabra_clave(texto, palabras_clave)
                        if clave:
                            print(f"[clave detectada] '{clave}' → pegando '{args.respuesta}'")
                            pegar_texto(args.respuesta)
                        else:
                            pegar_texto(texto)
                    else:
                        print("[aviso] Audio muy corto, ignorado.")

            time.sleep(0.02)

    except KeyboardInterrupt:
        print("\n[saliendo]")
    finally:
        grabador.cerrar()


if __name__ == "__main__":
    main()

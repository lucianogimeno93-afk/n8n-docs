# Typeless para Windows 10 — Dictado en español

Clon gratuito de Typeless. Usa Whisper de OpenAI corriendo **100% local** (sin internet, sin costo).

---

## Instalación (una sola vez)

### 1. Instalar Python 3.10 o superior
Descargarlo de https://www.python.org/downloads/ — marcar "Add Python to PATH".

### 2. Instalar FFmpeg (necesario para Whisper)
```
winget install ffmpeg
```
O descargarlo de https://ffmpeg.org/download.html y agregarlo al PATH.

### 3. Instalar dependencias Python
Abrir una terminal (cmd o PowerShell) en esta carpeta y ejecutar:
```
pip install -r requirements.txt
```

> Si `pyaudio` falla, instalar con:
> ```
> pip install pipwin
> pipwin install pyaudio
> ```

---

## Uso

```
python typeless.py
```

**Mantené presionada la tecla ALT DERECHO** mientras hablás. Al soltar, el texto aparece donde esté el cursor.

### Opciones

| Opción | Descripción | Default |
|--------|-------------|---------|
| `--modelo` | `tiny` / `base` / `small` / `medium` / `large` | `small` |
| `--csv` | Ruta al CSV con palabras clave | `palabras.csv` |
| `--tecla` | Tecla push-to-talk | `right alt` |
| `--respuesta` | Texto al detectar palabra clave | `ok` |

### Ejemplos

```bash
# Usar modelo más preciso
python typeless.py --modelo medium

# Cambiar la tecla a F9
python typeless.py --tecla f9

# Usar tu propio CSV de palabras
python typeless.py --csv mis_palabras.csv

# Cuando detecte una palabra clave, responder "Entendido"
python typeless.py --respuesta "Entendido"
```

---

## Palabras clave (CSV)

El archivo `palabras.csv` tiene una palabra o frase por línea:

```
Sergio Mendoza
Ana María
hola
gracias
```

Si decís algo que contenga una de esas frases, en lugar de pegar lo que dijiste, pega la `--respuesta` (por defecto `ok`).

---

## Modelos Whisper — tamaño y velocidad

| Modelo | Tamaño | Velocidad | Precisión |
|--------|--------|-----------|-----------|
| tiny   | 75 MB  | muy rápido | básica |
| base   | 142 MB | rápido     | buena  |
| small  | 244 MB | normal     | muy buena ✓ recomendado |
| medium | 769 MB | lento      | excelente |
| large  | 1.5 GB | muy lento  | máxima |

El modelo se descarga automáticamente la primera vez que ejecutás el script.

---

## Español rioplatense y mexicano

Whisper entiende ambas variantes sin configuración extra. El idioma está fijado en `es` para evitar que detecte otro idioma.

# Typeless para Windows 10 — Dictado en español

Clon gratuito de Typeless. **No necesita Python ni instalador.**  
Usa Whisper corriendo **100% local** (sin internet, sin costo mensual).

---

## Archivos necesarios en la misma carpeta

```
typeless.exe          ← este programa
whisper-cli.exe       ← de whisper.cpp (ver abajo)
ggml-small.bin        ← modelo de español (ver abajo)
palabras.csv          ← tus palabras clave
```

---

## Paso 1 — Descargar whisper-cli.exe

1. Ir a: https://github.com/ggerganov/whisper.cpp/releases
2. Bajar el archivo `whisper-bin-x64.zip` (la versión más reciente)
3. Descomprimirlo y copiar `whisper-cli.exe` a la misma carpeta que `typeless.exe`

---

## Paso 2 — Descargar el modelo de español

Recomendado: **ggml-small.bin** (244 MB, buena precisión en español rioplatense y mexicano)

Descargarlo desde:
https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin

Copiarlo a la misma carpeta que `typeless.exe`.

> Si querés más precisión: `ggml-medium.bin` (769 MB)  
> Si querés más velocidad: `ggml-base.bin` (142 MB)

---

## Paso 3 — Editar palabras.csv

Una palabra o frase por línea. Si el programa detecta esa frase en lo que dijiste, pega `ok` en lugar del texto:

```
Sergio Mendoza
Ana María
```

---

## Paso 4 — Ejecutar

Doble clic en `typeless.exe`.

Se abre una ventana de consola que muestra el estado.  
**Mantené presionado ALT DERECHO** mientras hablás.  
Al soltar, el texto aparece donde esté el cursor (Word, Chrome, WhatsApp Web, etc.).

---

## Cambiar la respuesta al detectar una palabra clave

Por defecto responde `ok`. Para cambiarlo, ejecutalo desde cmd:

```
typeless.exe "Entendido"
```

---

## Estructura de la carpeta final

```
📁 typeless\
  ├── typeless.exe
  ├── whisper-cli.exe
  ├── ggml-small.bin
  └── palabras.csv
```

---

## Español rioplatense y mexicano

Whisper entiende ambas variantes sin configuración extra.  
El idioma está fijado en `es` para evitar que detecte otro idioma.

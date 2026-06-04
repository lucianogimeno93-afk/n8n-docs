@echo off
title Instalador de Typeless ES
echo ============================================
echo  Typeless ES - Instalador
echo ============================================
echo.

:: Verificar si Node.js ya esta instalado
where node >nul 2>nul
if %errorlevel% equ 0 (
    echo [OK] Node.js ya esta instalado.
    goto :instalar_deps
)

echo [1/3] Descargando Node.js...
powershell -Command "Invoke-WebRequest -Uri 'https://nodejs.org/dist/v20.18.0/node-v20.18.0-x64.msi' -OutFile 'node_setup.msi'"
echo [1/3] Instalando Node.js...
msiexec /i node_setup.msi /quiet /norestart
del node_setup.msi
:: Recargar PATH
call refreshenv 2>nul
set "PATH=%PATH%;%ProgramFiles%\nodejs"

:instalar_deps
echo [2/3] Instalando dependencias...
call npm install
if %errorlevel% neq 0 (
    echo ERROR al instalar dependencias. Revisa tu conexion a internet.
    pause
    exit /b 1
)

echo [3/3] Creando acceso directo en el escritorio...
powershell -Command "$ws = New-Object -ComObject WScript.Shell; $s = $ws.CreateShortcut([Environment]::GetFolderPath('Desktop') + '\Typeless ES.lnk'); $s.TargetPath = '%~dp0iniciar.bat'; $s.WorkingDirectory = '%~dp0'; $s.IconLocation = 'shell32.dll,23'; $s.Save()"

:: Crear bat de inicio
echo @echo off > iniciar.bat
echo cd /d "%%~dp0" >> iniciar.bat
echo npx electron . >> iniciar.bat

echo.
echo ============================================
echo  Instalacion completa!
echo  Usa el acceso directo en el escritorio
echo  o ejecuta iniciar.bat
echo ============================================
echo.
pause

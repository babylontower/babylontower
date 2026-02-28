@echo off
REM Babylon Tower - Launch Instance 1 (Alice) - Batch File
REM This script launches the first instance for manual two-instance testing
REM
REM Usage:
REM   scripts\test\launch-instance1.bat [data_dir]
REM
REM Arguments:
REM   data_dir - Optional custom data directory (default: .\test-data\instance1)

setlocal EnableDelayedExpansion

REM Get script directory and project root
set "SCRIPT_DIR=%~dp0"
set "PROJECT_ROOT=%SCRIPT_DIR%.."

REM Set default data directory
if "%1"=="" (
    set "DATA_DIR=%PROJECT_ROOT%\test-data\instance1"
) else (
    set "DATA_DIR=%1"
)

REM Detect platform
set "PLATFORM=windows"

REM Find binary
set "BINARY="
set "PLATFORM_BINARY=%PROJECT_ROOT%\bin\platform\%PLATFORM%\messenger.exe"
set "STANDARD_BINARY=%PROJECT_ROOT%\bin\messenger.exe"

if exist "%PLATFORM_BINARY%" (
    set "BINARY=%PLATFORM_BINARY%"
    echo [INFO] Using platform binary: %BINARY%
) else if exist "%STANDARD_BINARY%" (
    set "BINARY=%STANDARD_BINARY%"
    echo [INFO] Using standard binary: %BINARY%
) else (
    echo [INFO] Binary not found. Building for Windows...
    cd /d "%PROJECT_ROOT%"
    make build-windows
    if exist "%PLATFORM_BINARY%" (
        set "BINARY=%PLATFORM_BINARY%"
    ) else (
        set "BINARY=%STANDARD_BINARY%"
    )
)

REM Setup data directory
echo [INFO] Setting up data directory: %DATA_DIR%
echo [INFO] Using binary: %BINARY%
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"

echo.
echo ╔═══════════════════════════════════════════════════════════╗
echo ║        Babylon Tower - Instance 1 (Alice)                 ║
echo ╚═══════════════════════════════════════════════════════════╝
echo.
echo [INFO] Starting Instance 1 (Alice)...
echo.
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo Instance 1 (Alice)
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo.
echo Next steps:
echo   1. Wait for the instance to start and generate identity
echo   2. Run '/myid' to get your public key
echo   3. Share your public key with Instance 2 (Bob)
echo   4. Add Bob as contact: /add ^<bob_public_key^> Bob
echo   5. Start chat: /chat 1
echo.
echo To launch Instance 2 (Bob), run in another terminal:
echo   scripts\test\launch-instance2.bat
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo.

REM Set HOME environment variable for this process and child processes
REM This is CRITICAL for multi-instance isolation on Windows
set "HOME=%DATA_DIR%"
set "USERPROFILE=%DATA_DIR%"

echo [INFO] HOME set to: %HOME%
echo [INFO] USERPROFILE set to: %USERPROFILE%

REM Run the messenger
cd /d "%PROJECT_ROOT%"
"%BINARY%"

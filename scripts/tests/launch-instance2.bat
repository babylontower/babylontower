@echo off
REM Babylon Tower - Launch Instance 2 (Bob) - Windows
REM This script launches the second instance for manual two-instance testing
REM
REM Usage:
REM   launch-instance2.bat [data_dir]
REM
REM Arguments:
REM   data_dir - Optional custom data directory (default: .\test-data\instance2)

setlocal EnableDelayedExpansion

REM Get script directory and project root
set "SCRIPT_DIR=%~dp0"
set "PROJECT_ROOT=%SCRIPT_DIR%..\.."

REM Set default data directory
set "DEFAULT_DATA_DIR=%PROJECT_ROOT%\test-data\instance2"
set "DATA_DIR=%DEFAULT_DATA_DIR%"

REM Check if custom data directory was provided
if not "%~1"=="" set "DATA_DIR=%~1"

REM Set binary path (Windows build from make build-all)
set "BINARY=%PROJECT_ROOT%\bin\platform\windows\messenger.exe"

REM Check if binary exists
if not exist "%BINARY%" (
    echo [ERROR] Binary not found: %BINARY%
    echo Please run 'make build-all' or 'make build-windows' first.
    exit /b 1
)

REM Create data directory if it doesn't exist
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"

REM Display banner
echo.
echo ╔═══════════════════════════════════════════════════════════╗
echo ║        Babylon Tower - Instance 2 (Bob)                   ║
echo ╚═══════════════════════════════════════════════════════════╝
echo.
echo [INFO] Starting Instance 2 (Bob)...
echo [INFO] Data directory: %DATA_DIR%
echo [INFO] Using binary: %BINARY%
echo.
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo Instance 2 (Bob)
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo.
echo Next steps:
echo   1. Wait for the instance to start and generate identity
echo   2. Run '/myid' to get your public key
echo   3. Run '/listen' on Instance 1 to get its multiaddr
echo   4. Connect to Instance 1: /connect ^<alice_multiaddr^>
echo   5. Share your public key with Instance 1 (Alice)
echo   6. Add Alice as contact: /add ^<alice_public_key^> Alice
echo   7. Start chat: /chat 1
echo.
echo NOTE: Windows Firewall may block peer discovery. If peers don't
echo       connect automatically, use /connect with the multiaddr.
echo.
echo To launch Instance 1 (Alice), run in another terminal:
echo   scripts\tests\launch-instance1.bat
echo ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
echo.

REM Set USERPROFILE to use custom data directory
REM Babylon Tower stores identity in %%USERPROFILE%%\.babylontower
REM Note: On Windows, Go's os.UserHomeDir() uses USERPROFILE, not HOME
set "USERPROFILE=%DATA_DIR%"

REM Run the messenger
cd /d "%PROJECT_ROOT%"
"%BINARY%"

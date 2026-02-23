# Babylon Tower - Launch Instance 2 (Bob) - PowerShell
# This script launches the second instance for manual two-instance testing
#
# Usage:
#   .\scripts\test\launch-instance2.ps1 [data_dir]
#
# Arguments:
#   data_dir - Optional custom data directory (default: .\test-data\instance2)

param(
    [string]$DataDir = ""
)

$ErrorActionPreference = "Stop"

# Get script directory and project root
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent (Split-Path -Parent $ScriptDir)

# Set default data directory
if ($DataDir -eq "") {
    $DataDir = Join-Path $ProjectRoot "test-data\instance2"
}

# Colors
function Write-Info { Write-Host "[INFO] $args" -ForegroundColor Blue }
function Write-Success { Write-Host "[SUCCESS] $args" -ForegroundColor Green }

function Show-Banner {
    Write-Host ""
    Write-Host "╔═══════════════════════════════════════════════════════════╗"
    Write-Host "║        Babylon Tower - Instance 2 (Bob)                   ║"
    Write-Host "╚═══════════════════════════════════════════════════════════╝"
    Write-Host ""
}

# Detect platform
$Platform = "windows"

# Find binary
$Binary = ""
$PlatformBinary = Join-Path $ProjectRoot "bin\platform\$Platform\messenger.exe"
$StandardBinary = Join-Path $ProjectRoot "bin\messenger.exe"

if (Test-Path $PlatformBinary) {
    $Binary = $PlatformBinary
    Write-Info "Using platform binary: $Binary"
} elseif (Test-Path $StandardBinary) {
    $Binary = $StandardBinary
    Write-Info "Using standard binary: $Binary"
} else {
    Write-Info "Binary not found. Building for Windows..."
    Push-Location $ProjectRoot
    make build-windows
    Pop-Location
    
    if (Test-Path $PlatformBinary) {
        $Binary = $PlatformBinary
    } else {
        $Binary = $StandardBinary
    }
}

# Setup data directory
Write-Info "Setting up data directory: $DataDir"
Write-Info "Using binary: $Binary"
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null

Show-Banner

Write-Info "Starting Instance 2 (Bob)..."
Write-Host ""
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
Write-Host "Instance 2 (Bob)"
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
Write-Host ""
Write-Host "Next steps:"
Write-Host "  1. Wait for the instance to start and generate identity"
Write-Host "  2. Run '/myid' to get your public key"
Write-Host "  3. Share your public key with Instance 1 (Alice)"
Write-Host "  4. Add Alice as contact: /add <alice_public_key> Alice"
Write-Host "  5. Start chat: /chat 1"
Write-Host ""
Write-Host "To launch Instance 1 (Alice), run in another terminal:"
Write-Host "  .\scripts\test\launch-instance1.ps1"
Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
Write-Host ""

# Set HOME environment variable for this process and child processes
# This is CRITICAL for multi-instance isolation on Windows
$env:HOME = $DataDir
$env:USERPROFILE = $DataDir

Write-Info "HOME set to: $env:HOME"
Write-Info "USERPROFILE set to: $env:USERPROFILE"

# Run the messenger
Push-Location $ProjectRoot
& $Binary

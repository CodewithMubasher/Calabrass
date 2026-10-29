<# 
.SYNOPSIS
    Builds Calabrass completely and copies the installer to Desktop.

.DESCRIPTION
    Runs the full build pipeline:
    1. Frontend (pnpm install + tsc + vite build)
    2. Backend + Wails desktop app (go build via wails)
    3. MCP servers (excel-mcp, win-control-mcp)
    4. Inno Setup installer
    5. Copies Calabrass-setup-v<version>-x64.exe to Desktop

.NOTES
    Run from the repo root:  .\build-all.ps1
    Requires: Go, pnpm, Wails v2.16.0, Inno Setup 6
#>

param(
    [switch]$SkipFrontend,
    [switch]$SkipBackend,
    [switch]$SkipMcp,
    [switch]$SkipInstaller,
    [switch]$NoCopy
)

$ErrorActionPreference = "Stop"
$repoRoot = (Get-Location).Path
$desktop = [Environment]::GetFolderPath("Desktop")
$outDir = Join-Path $repoRoot "installer\out"
$issPath = Join-Path $repoRoot "installer\Calabrass.iss"
$versionMatch = Select-String -Path $issPath -Pattern '^\s*#define AppVersion "(.+)"' | Select-Object -First 1
if (-not $versionMatch) {
    Write-Error "Could not read AppVersion out of $issPath"
    exit 1
}
$setupName = "Calabrass-setup-v$($versionMatch.Matches.Groups[1].Value)-x64.exe"
$setupSrc = Join-Path $outDir $setupName
$setupDst = Join-Path $desktop $setupName

function Write-Step($msg) {
    Write-Host "`n==== $msg ====" -ForegroundColor Cyan
}

function Check-Command($name, $cmd) {
    if (-not (Get-Command $cmd -ErrorAction SilentlyContinue)) {
        Write-Error "Required command not found: $name ($cmd)"
        exit 1
    }
}

function Find-ISCC() {
    # Common install locations
    $candidates = @(
        "${env:LOCALAPPDATA}\Programs\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles}\Inno Setup 6\ISCC.exe",
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "ISCC"  # if in PATH
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) { return $c }
        if (Get-Command $c -ErrorAction SilentlyContinue) { return $c }
    }
    return $null
}

Write-Step "Environment check"
Check-Command "Go" "go"
Check-Command "pnpm" "pnpm"
Check-Command "Wails" "wails"
$ISCC = Find-ISCC
if (-not $ISCC) {
    Write-Error "Inno Setup (ISCC) not found. Install it or add to PATH."
    exit 1
}
Write-Host "  ISCC:    $ISCC"

$goVer = (go version)
$pnpmVer = (pnpm --version)
$wailsVer = (wails version 2>&1 | Select-Object -First 1)
Write-Host "Go:      $goVer"
Write-Host "pnpm:    $pnpmVer"
Write-Host "Wails:   $wailsVer"

if (-not $SkipFrontend) {
    Write-Step "Frontend: install deps"
    Push-Location (Join-Path $repoRoot "Frontend")
    pnpm install --frozen-lockfile
    Pop-Location

    Write-Step "Frontend: typecheck + build"
    Push-Location (Join-Path $repoRoot "Frontend")
    pnpm exec tsc -b
    pnpm run build:desktop
    Pop-Location

    # The background host serves the web interface itself, so the page is built
    # for the web (same-origin /api) and compiled into the host.
    Write-Step "Frontend: web build for the background host"
    Push-Location (Join-Path $repoRoot "Frontend")
    pnpm run build
    Pop-Location
    $webDist = Join-Path $repoRoot "Backend\internal\webui\dist"
    if (Test-Path $webDist) { Remove-Item $webDist -Recurse -Force }
    New-Item -ItemType Directory -Path $webDist | Out-Null
    Copy-Item (Join-Path $repoRoot "Frontend\dist\*") $webDist -Recurse -Force
}

if (-not $SkipBackend) {
    Write-Step "Backend + Desktop (Wails build)"
    Push-Location (Join-Path $repoRoot "Backend\desktop")
    wails build -clean
    Pop-Location

    # The background host: the same backend, as its own program, so scheduled
    # tasks keep running after the window closes and the terminal and the web
    # page can use it too. The installer puts it beside Calabrass.exe.
    Write-Step "Background host (calabrass-backend.exe)"
    Push-Location (Join-Path $repoRoot "Backend")
    $hostVersion = $versionMatch.Matches.Groups[1].Value
    go build -trimpath -ldflags "-s -w -X github.com/CodewithMubasher/Calabrass/backend/internal/version.Version=$hostVersion" `
        -o (Join-Path $repoRoot "Backend\desktop\build\bin\calabrass-backend.exe") .\cmd\server
    Pop-Location
}

if (-not $SkipMcp) {
    Write-Step "MCP: excel-mcp"
    Push-Location (Join-Path $repoRoot "MCP\excel")
    go build -ldflags="-H=windowsgui" -o "$repoRoot\MCP\excel\excel-mcp.exe" .
    Pop-Location

    Write-Step "MCP: win-control-mcp"
    Push-Location (Join-Path $repoRoot "MCP\win-control")
    go build -ldflags="-H=windowsgui" -o "$repoRoot\MCP\win-control\win-control-mcp.exe" .
    Pop-Location
}

if (-not $SkipInstaller) {
    Write-Step "Installer: Inno Setup"
    $iss = $issPath
    if (-not (Test-Path $iss)) { Write-Error "Missing $iss"; exit 1 }
    & $ISCC $iss
}

if (Test-Path $setupSrc) {
    $bytes = [IO.File]::ReadAllBytes($setupSrc)
    $sha = [BitConverter]::ToString((New-Object System.Security.Cryptography.SHA256Managed).ComputeHash($bytes)).Replace("-","").ToLower()
    $sizeMB = "{0:N2}" -f ($bytes.Length / 1MB)

    Write-Step "Result"
    Write-Host "  File:     $setupName"
    Write-Host "  Size:     $sizeMB MB"
    Write-Host "  SHA256:   $sha"
    Write-Host "  At:       $setupSrc"

    if (-not $NoCopy) {
        Write-Step "Copying to Desktop"
        Copy-Item $setupSrc $setupDst -Force
        Write-Host "  Copied to: $setupDst" -ForegroundColor Green
    }
} else {
    Write-Error "Installer not found at $setupSrc"
    exit 1
}

Write-Host "`nDone." -ForegroundColor Green
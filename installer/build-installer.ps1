#Requires -Version 5.1
<#
.SYNOPSIS
    Builds Calabrass from source and packs it into the Windows installer.

.DESCRIPTION
    Produces installer\out\Calabrass-setup-v1.0-x64.exe out of the sources
    alone, in the same three steps a release needs:

        1. the desktop app   — Frontend + Go, through Wails
        2. the MCP servers   — the ones the installer is allowed to ship
        3. the installer     — the two above, wrapped by Inno Setup

    The version comes from installer\Calabrass.iss. Bump it there.

    Running this touches only ordinary build output: Frontend\node_modules,
    Frontend\dist, Backend\desktop\build\bin, MCP\*\*-mcp.exe and
    installer\out.

.PARAMETER SkipApp
    Reuse the Calabrass.exe that is already in Backend\desktop\build\bin
    instead of rebuilding it. For when only the installer script changed.

.PARAMETER SkipMcp
    Reuse the MCP servers that are already built.

.EXAMPLE
    .\build-installer.ps1

.EXAMPLE
    .\build-installer.ps1 -SkipApp -SkipMcp
#>
[CmdletBinding()]
param(
    [switch] $SkipApp,
    [switch] $SkipMcp
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$RepoRoot    = Split-Path -Parent $PSScriptRoot
$DesktopDir  = Join-Path $RepoRoot 'Backend\desktop'
$AppExe      = Join-Path $DesktopDir 'build\bin\Calabrass.exe'
$Iss         = Join-Path $PSScriptRoot 'Calabrass.iss'
$OutputDir   = Join-Path $PSScriptRoot 'out'

$McpServers = @(
    @{ Name = 'excel';       Dir = 'MCP\excel';       Exe = 'excel-mcp.exe' }
    @{ Name = 'win-control'; Dir = 'MCP\win-control'; Exe = 'win-control-mcp.exe' }
)

function Write-Step {
    param([string] $Message)
    Write-Host ''
    Write-Host "==> $Message" -ForegroundColor Cyan
}

function Find-Tool {
    param([string] $Name, [string[]] $Fallbacks = @())
    $command = Get-Command $Name -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
    foreach ($candidate in $Fallbacks) {
        if (Test-Path -LiteralPath $candidate) { return $candidate }
    }
    throw "Cannot find '$Name'. Install it, then run this again."
}

function Invoke-Tool {
    param(
        [string]   $What,
        [string]   $Exe,
        [string[]] $Arguments,
        [string]   $WorkingDirectory = $RepoRoot
    )
    Write-Step $What
    Push-Location -LiteralPath $WorkingDirectory
    try {
        $LASTEXITCODE = 0
        & $Exe @Arguments
        if ($LASTEXITCODE -ne 0) { throw "Step failed: $What (exit $LASTEXITCODE)." }
    }
    finally {
        Pop-Location
    }
}

# A running Calabrass holds its own executable open, and Windows refuses to let
# Setup replace a file in use. Asking it to close first is what makes an
# upgrade-over-an-old-install work without a reboot.
function Stop-RunningApp {
    $running = @(Get-Process -Name 'Calabrass' -ErrorAction SilentlyContinue)
    if ($running.Count -eq 0) { return }

    Write-Step 'Closing the running Calabrass'
    foreach ($process in $running) {
        Write-Host "    pid $($process.Id)"
        try {
            if (-not $process.CloseMainWindow()) { $process.Kill() }
            elseif (-not $process.WaitForExit(5000)) { $process.Kill() }
        }
        catch {
            # It exited between the check and the ask, which is the good case.
        }
    }
    Start-Sleep -Milliseconds 500
}

$go       = Find-Tool 'go'
$pnpm     = Find-Tool 'pnpm'
$wails    = Find-Tool 'wails'
$inno     = Find-Tool 'ISCC' @(
    (Join-Path $env:LOCALAPPDATA 'Programs\Inno Setup 6\ISCC.exe')
    'C:\Program Files (x86)\Inno Setup 6\ISCC.exe'
    'C:\Program Files\Inno Setup 6\ISCC.exe'
)

$versionMatch = Select-String -Path $Iss -Pattern '^\s*#define AppVersion "(.+)"' | Select-Object -First 1
if (-not $versionMatch) { throw "Could not read AppVersion out of $Iss." }
$version = $versionMatch.Matches.Groups[1].Value
$installer = Join-Path $OutputDir "Calabrass-setup-v$version-x64.exe"

Write-Host "Calabrass $version installer" -ForegroundColor White
Write-Host "    repo      $RepoRoot"
Write-Host "    go        $go"
Write-Host "    pnpm      $pnpm"
Write-Host "    wails     $wails"
Write-Host "    Inno      $inno"

Stop-RunningApp

# 1. The desktop app. Wails drives the frontend itself: it runs pnpm install and
#    the desktop build from wails.json, then compiles the Go shell with the
#    frontend's output embedded in it.
if ($SkipApp) {
    if (-not (Test-Path -LiteralPath $AppExe)) {
        throw "-SkipApp was given but $AppExe does not exist."
    }
    Write-Step "Reusing $AppExe"
}
else {
    Invoke-Tool -What 'Building the desktop app (frontend + Go, via Wails)' `
                -Exe $wails -Arguments @('build', '-clean') -WorkingDirectory $DesktopDir
}

if (-not (Test-Path -LiteralPath $AppExe)) { throw "Wails did not produce $AppExe." }
Write-Host ("    {0:N1} MB  {1}" -f ((Get-Item -LiteralPath $AppExe).Length / 1MB), $AppExe)

# 2. The MCP servers. They are separate Go modules, built into the folders the
#    installer copies from. Anything not listed here does not ship.
#
#    -H=windowsgui matters and is not cosmetic. A Windows program built without
#    it is a console program, and Windows gives every console program a console
#    window of its own — the app is a GUI program and has none to lend it. These
#    servers are started by the app as child processes, and every one of those
#    starts would otherwise put a blank black window with the server's path in
#    its title over the top of the app. The MCP client that starts them goes
#    through a library that offers no way to ask for the window to be hidden, so
#    the server itself has to be built without one. Speaking MCP over the pipes
#    it is handed needs no console.
if ($SkipMcp) {
    Write-Step 'Reusing the built MCP servers'
}
foreach ($server in $McpServers) {
    $serverDir = Join-Path $RepoRoot $server.Dir
    $serverExe = Join-Path $serverDir $server.Exe

    if (-not $SkipMcp) {
        $env:CGO_ENABLED = '0'
        Invoke-Tool -What "Building the $($server.Name) MCP server" `
                    -Exe $go -Arguments @('build', '-trimpath', '-ldflags', '-s -w -H=windowsgui', '-o', $server.Exe, '.') `
                    -WorkingDirectory $serverDir
    }

    if (-not (Test-Path -LiteralPath $serverExe)) {
        throw "No $($server.Exe) in $serverDir."
    }
    Write-Host ("    {0:N1} MB  {1}" -f ((Get-Item -LiteralPath $serverExe).Length / 1MB), $serverExe)
}

# 3. The installer.
Invoke-Tool -What "Packing $([System.IO.Path]::GetFileName($installer))" `
            -Exe $inno -Arguments @($Iss) -WorkingDirectory $PSScriptRoot

if (-not (Test-Path -LiteralPath $installer)) { throw "Inno did not produce $installer." }

Write-Host ''
Write-Host "Built $installer" -ForegroundColor Green
Write-Host ("      {0:N1} MB" -f ((Get-Item -LiteralPath $installer).Length / 1MB))

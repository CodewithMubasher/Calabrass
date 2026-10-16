<#
.SYNOPSIS
  Run the Calabrass backend (Go) and frontend (Vite) together.

.DESCRIPTION
  Starts both dev servers as child processes, streams their output into this
  console, and shuts both down when you press Ctrl+C or when either process
  exits on its own.

.EXAMPLE
  .\dev.ps1
#>
[CmdletBinding()]
param(
  [switch]$SkipInstall
)

$ErrorActionPreference = "Stop"

$root = $PSScriptRoot
$backendDir = Join-Path $root "Backend"
$frontendDir = Join-Path $root "Frontend"

function Write-Step([string]$Message) {
  Write-Host "==> $Message" -ForegroundColor Cyan
}

function Write-Warn([string]$Message) {
  Write-Host "==> $Message" -ForegroundColor Yellow
}

# Start-Process needs a real executable. Resolving explicitly avoids picking up
# the PowerShell shim (pnpm.ps1) instead of pnpm.cmd.
function Resolve-CommandPath([string]$Name) {
  foreach ($candidate in @("$Name.cmd", "$Name.exe", $Name)) {
    $command = Get-Command $candidate -ErrorAction SilentlyContinue
    if ($command) { return $command.Source }
  }
  return $Name
}

# --- sanity checks ----------------------------------------------------------

if (-not (Test-Path $backendDir)) { throw "Backend directory not found: $backendDir" }
if (-not (Test-Path $frontendDir)) { throw "Frontend directory not found: $frontendDir" }

if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  throw "The 'go' command was not found on PATH."
}
if (-not (Get-Command pnpm -ErrorAction SilentlyContinue)) {
  throw "The 'pnpm' command was not found on PATH."
}

$goExe = Resolve-CommandPath "go"
$pnpmExe = Resolve-CommandPath "pnpm"

if (-not (Test-Path (Join-Path $backendDir ".env"))) {
  Write-Warn "Backend/.env is missing. Copy Backend/.env.example and add your API keys."
}

if (-not (Test-Path (Join-Path $frontendDir "node_modules")) -and -not $SkipInstall) {
  Write-Step "Installing frontend dependencies (first run)"
  Push-Location $frontendDir
  try { pnpm install } finally { Pop-Location }
}

# --- process helpers --------------------------------------------------------

$script:shuttingDown = $false
$script:children = @()

function Start-DevProcess {
  param(
    [Parameter(Mandatory)][string]$Name,
    [Parameter(Mandatory)][string]$FilePath,
    [Parameter(Mandatory)][string[]]$ArgumentList,
    [Parameter(Mandatory)][string]$WorkingDirectory,
    [Parameter(Mandatory)][System.ConsoleColor]$Color
  )

  Write-Step "Starting $Name"
  $proc = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList `
    -WorkingDirectory $WorkingDirectory -NoNewWindow -PassThru

  $script:children += [pscustomobject]@{ Name = $Name; Process = $proc; Color = $Color }
  return $proc
}

function Stop-DevProcesses {
  foreach ($child in $script:children) {
    $proc = $child.Process
    if ($null -eq $proc) { continue }
    try { $proc.Refresh() } catch { continue }
    if ($proc.HasExited) { continue }

    Write-Host "==> Stopping $($child.Name) (PID $($proc.Id))" -ForegroundColor $child.Color
    # /T kills the whole tree: `go run` and `pnpm` both spawn children.
    & taskkill.exe /PID $proc.Id /T /F 2>&1 | Out-Null
  }
}

# Ctrl+C should run our cleanup instead of hard-killing this shell.
$cancelHandler = [System.ConsoleCancelEventHandler] {
  param($sender, $eventArgs)
  $eventArgs.Cancel = $true
  $script:shuttingDown = $true
}
[Console]::add_CancelKeyPress($cancelHandler)

try {
  # The backend resolves Backend/.env relative to its working directory.
  Start-DevProcess -Name "backend" -FilePath $goExe `
    -ArgumentList @("run", "./cmd/server") `
    -WorkingDirectory $backendDir -Color DarkYellow | Out-Null

  Start-DevProcess -Name "frontend" -FilePath $pnpmExe `
    -ArgumentList @("dev") `
    -WorkingDirectory $frontendDir -Color DarkCyan | Out-Null

  Write-Host ""
  Write-Host "  Backend  : http://127.0.0.1:8787/api/health" -ForegroundColor DarkYellow
  Write-Host "  Frontend : http://localhost:5173" -ForegroundColor DarkCyan
  Write-Host "  Press Ctrl+C to stop both." -ForegroundColor Gray
  Write-Host ""

  while (-not $script:shuttingDown) {
    Start-Sleep -Milliseconds 300

    foreach ($child in $script:children) {
      $proc = $child.Process
      if ($null -eq $proc) { continue }
      try { $proc.Refresh() } catch { continue }
      if ($proc.HasExited) {
        Write-Warn "$($child.Name) exited with code $($proc.ExitCode); shutting the rest down."
        $script:shuttingDown = $true
        break
      }
    }
  }
}
finally {
  [Console]::remove_CancelKeyPress($cancelHandler)
  Stop-DevProcesses
  Write-Step "All dev processes stopped."
}

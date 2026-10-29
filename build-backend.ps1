# Build script for Calabrass backend binaries (PowerShell)
# Creates standalone executables for 6 target platforms

param(
    [string]$SourceDir = "./Backend",
    [string]$BuildDir = "./build/backend"
)

Write-Host "Building Calabrass backend binaries..."

# Define target platforms
$platforms = @(
    "linux/amd64",
    "linux/arm64", 
    "darwin/amd64",
    "darwin/arm64",
    "windows/amd64",
    "windows/arm64"
)

# Create build directory
if (!(Test-Path $BuildDir)) {
    New-Item -ItemType Directory -Path $BuildDir | Out-Null
}

# Check if we're in the right directory
if (!(Test-Path "$SourceDir/go.mod")) {
    Write-Error "Backend source not found. Run this script from the repository root."
    exit 1
}

# Build for each platform
foreach ($platform in $platforms) {
    $parts = $platform.Split('/')
    $goos = $parts[0]
    $goarch = $parts[1]
    
    # Determine platform-specific naming
    switch ($goos) {
        "linux" { $platform_name = "linux" }
        "darwin" { $platform_name = "darwin" }
        "windows" { $platform_name = "win32" }
    }
    
    # Output filename
    $output_name = "calabrass-backend"
    if ($goos -eq "windows") {
        $output_name += ".exe"
    }
    
    # Output directory
    $output_dir = "$BuildDir/calabrass-$platform_name-$goarch"
    if (!(Test-Path $output_dir)) {
        New-Item -ItemType Directory -Path $output_dir | Out-Null
    }
    
    Write-Host "Building $platform_name/$goarch..."
    
    # Set environment variables
    $env:GOOS = $goos
    $env:GOARCH = $goarch
    $env:CGO_ENABLED = "0"
    
    # Change to Backend directory and build
    Push-Location $SourceDir
    try {
        $build_cmd = "go build -trimpath -buildvcs=false -ldflags '-s -w' -o `"$output_dir/$output_name`" `".\cmd\server`""
        Invoke-Expression $build_cmd
        
        if ($LASTEXITCODE -ne 0) {
            Write-Error "Build failed for $platform_name/$goarch"
            exit 1
        }
    }
    finally {
        Pop-Location
    }
    
    Write-Host "Built: $output_dir/$output_name"
}

Write-Host "Backend build complete!"
Write-Host "Binaries are in: $BuildDir"

# List built files
Write-Host ""
Write-Host "Built binaries:"
Get-ChildItem -Path $BuildDir -Name "calabrass-backend*" -Recurse | ForEach-Object { Write-Host "  $_" }
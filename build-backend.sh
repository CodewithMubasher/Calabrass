#!/bin/bash

# Build script for Calabrass backend binaries
# Creates standalone executables for 6 target platforms

set -e

echo "Building Calabrass backend binaries..."

# Define target platforms
platforms=(
    "linux/amd64"
    "linux/arm64"
    "darwin/amd64"
    "darwin/arm64"
    "windows/amd64"
    "windows/arm64"
)

# Build directory
BUILD_DIR="./build/backend"
mkdir -p "$BUILD_DIR"

# Source directory
SOURCE_DIR="./Backend"

# Check if we're in the right directory
if [ ! -f "$SOURCE_DIR/go.mod" ]; then
    echo "Error: Backend source not found. Run this script from the repository root."
    exit 1
fi

# Build for each platform
for platform in "${platforms[@]}"; do
    IFS='/' read -r goos goarch <<< "$platform"
    
    # Determine platform-specific naming
    case $goos in
        linux) platform_name="linux" ;;
        darwin) platform_name="darwin" ;;
        windows) platform_name="win32" ;;
    esac
    
    # Output filename
    output_name="calabrass-backend"
    if [ "$goos" = "windows" ]; then
        output_name="$output_name.exe"
    fi
    
    # Output directory
    output_dir="$BUILD_DIR/calabrass-${platform_name}-${goarch}"
    mkdir -p "$output_dir"
    
    echo "Building $platform_name/$goarch..."
    
    # Set environment variables
    export GOOS="$goos"
    export GOARCH="$goarch"
    export CGO_ENABLED="0"  # SQLite is pure Go, no CGO needed
    
    # Build with release flags
    go build -trimpath -buildvcs=false -ldflags "-s -w" \
        -o "$output_dir/$output_name" \
        "$SOURCE_DIR/cmd/server"
    
    # Make Unix executables executable
    if [ "$goos" != "windows" ]; then
        chmod +x "$output_dir/$output_name"
    fi
    
    echo "Built: $output_dir/$output_name"
done

echo "Backend build complete!"
echo "Binaries are in: $BUILD_DIR"

# List built files
echo ""
echo "Built binaries:"
find "$BUILD_DIR" -name "calabrass-backend*" -type f
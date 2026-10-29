# calabrass

Terminal client for [Calabrass](https://github.com/CodewithMubasher/Calabrass) — the open source personal AI agent.

## Installation

```bash
npm install -g calabrass
```

Then run it by name:

```bash
cal
```

Or without installing:

```bash
npx -p calabrass cal
```

The package bundles the prebuilt binaries for every supported platform
(Linux, macOS and Windows on x64 and arm64), so `cal` works immediately — no
download step and no install script.

## Usage

```bash
cal [flags]

Flags:
  -addr string    Backend address (default: $CALABRASS_ADDR, saved config, or 127.0.0.1:8787)
  -c, -continue   Continue the latest session of this folder
```

The folder you start `cal` in is the workspace the agent works in. Every new
session is bound to it, and `/cd` moves the current session elsewhere.

The `cal` command automatically manages the backend for you:

```bash
cal
```

## How it works

`bin/cal.js` is a smart launcher that:

1. **Detects your platform** (Linux, macOS, Windows) and architecture (x64, arm64)
2. **Checks if the backend is already running** on `127.0.0.1:8787`
3. **Starts the bundled backend** if needed, with proper error handling and retries
4. **Waits for the backend to be ready** (health check on `/api/health`)
5. **Launches the TUI** connected to the running backend
6. **Manages the process lifecycle** - cleans up the backend when the TUI exits (if it started it)

The backend and TUI are both bundled in the npm package, so you don't need to:
- Install Go
- Clone the repository
- Run `go run ./cmd/server`
- Open a second terminal

## Backend Management

The CLI automatically handles backend startup:

- **Reuse existing backend**: If a backend is already running, it connects to it
- **Start bundled backend**: If no backend is running, it starts the bundled binary
- **Health checks**: Waits for the backend to be ready before launching the TUI
- **Process cleanup**: Kills the backend when the TUI exits (if it started it)
- **Port conflicts**: Handles cases where another service is using port 8787

## Troubleshooting

If you encounter issues:

1. **Backend startup fails**: Check if another service is using port 8787
2. **Missing binaries**: Try reinstalling: `npm install -g calabrass --force`
3. **Permission issues**: Make sure the binaries have execute permissions (Unix)

## Development

If you're developing from source:

```bash
# Build both TUI and backend binaries
./build-backend.ps1  # Windows
./build-backend.sh   # Unix-like

# Copy binaries to npm package
Copy-Item -Path "build/backend/calabrass-*" -Destination "npm-wrapper/binaries" -Recurse -Force

# Test the package
cd npm-wrapper
node scripts/prepare.js 1.0.6
npm pack --dry-run
```

First run: type `/connect`, pick a provider, add its API key. Then chat.

## How it works

`bin/cal.js` is a small shim. It picks `binaries/calabrass-<os>-<arch>/`
for your machine, hands it the terminal, and mirrors its exit code. The TUI
itself is written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Publishing

CLI releases use their own tag namespace, `cli-v*`, so they never create or
touch a desktop release:

1. Bump the version in `package.json`
2. Push a tag: `git tag cli-v1.0.6 && git push origin cli-v1.0.6`
3. The workflow builds all six targets, bundles them into `binaries/`, and
   publishes `calabrass` (skipping the publish if that version already exists)

Requires an `NPM_TOKEN` repository secret.

To try the packaging locally (after dropping binaries into `binaries/`):

```bash
node scripts/prepare.js 1.0.5
npm pack
```

## License

MIT

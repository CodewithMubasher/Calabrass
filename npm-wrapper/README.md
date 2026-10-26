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

This package ships no binary itself. The `calabrass-<os>-<arch>` package for
your platform is installed alongside it (an optional dependency), so there is
no download step and no install script.

## Usage

```bash
cal [flags]

Flags:
  -addr string    Backend address (default: $CALABRASS_ADDR, saved config, or 127.0.0.1:8787)
  -c, -continue   Continue the latest session of this folder
```

The folder you start `cal` in is the workspace the agent works in. Every new
session is bound to it, and `/cd` moves the current session elsewhere.

The TUI needs the Calabrass backend running:

```bash
cal
```

If the backend is not up, the first screen says so and offers `r` to retry.
Start it with the desktop app, or from a source checkout:

```bash
cd Backend && go run ./cmd/server
```

First run: type `/connect`, pick a provider, add its API key. Then chat.

## How it works

`bin/cal.js` is a small shim. It resolves the binary from the platform package
npm installed (`calabrass-linux-x64`, `calabrass-darwin-arm64`, …), hands it the
terminal, and mirrors its exit code. The TUI itself is written in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea).

## Publishing

CLI releases use their own tag namespace, `cli-v*`, so they never create or
touch a desktop release:

1. Bump the version in `package.json`
2. Push a tag: `git tag cli-v1.0.5 && git push origin cli-v1.0.5`
3. The workflow builds every platform, packs one npm package per platform,
   and publishes them plus `calabrass`

Requires an `NPM_TOKEN` repository secret.

To try the packaging locally:

```bash
node scripts/prepare.js 1.0.5
npm pack
```

## License

MIT

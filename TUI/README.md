# Calabrass TUI

Terminal client for the Calabrass backend. It talks to the same HTTP API as the
web and desktop apps (`/api/*`), so sessions, providers and MCP servers are
shared with them. Nothing in the UI is simulated.

## Run

```
# 1. start the backend (or open the desktop app)
cd Backend && go run ./cmd/server

# 2. start the TUI from the folder you want the agent to work in
cd TUI && go run .
```

Flags: `-addr host:port` (also `CALABRASS_ADDR`), `-c` to continue the latest
session of the current folder.

First run: type `/connect`, pick a provider, add its API key. Then just chat.

### Account (no login screen yet)

The backend requires a session, so the TUI signs in silently:
1. a saved token (`<user config dir>/Calabrass/tui.json`), else
2. `CALABRASS_EMAIL` + `CALABRASS_PASSWORD` to use your existing account, else
3. it creates a local account `tui-xxxx@calabrass.local` and remembers it.

Use option 2 if you want the TUI to show the same chats as the web app.

## Keys and commands

`enter` send, `alt+enter` new line, `esc` interrupt, `ctrl+c` interrupt/quit,
`ctrl+p` palette, `ctrl+o` open/close all tool rows (or click one),
`ctrl+r` right panel, `pgup/pgdn` scroll.

`/models /connect /continue /new /mcps /cd /rename /retry /answer /export /help /exit`

## Layout

- `internal/api` – HTTP + SSE client (tested against a fake SSE server)
- `internal/session` – silent sign-in
- `internal/settings` – token and last model on disk (0600)
- `turn.go`, `trace_parts.go` – streams a turn and turns the backend's trace into timeline rows
- `update.go`, `commands.go`, `app.go` – events, slash commands, backend calls
- the remaining files are the existing UI (timeline, dialogs, markdown, ...)

Tests: `go test ./...` (drives the real Update loop against a fake backend).

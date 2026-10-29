# Calabrass — Stage 1 + Stage 2 changes

Extract the zip over the repository root (paths are relative to it). 80 files: 42 edited, 38 new.
Delete these five obsolete files from `npm-wrapper/` (they grep the old launcher's source; replaced by `npm-wrapper/test/launcher.test.js`):
`test-cli.js`, `test-cli-commands.js`, `test-parse.js`, `test-race-fix.js`, `test-regression.js`.

## Stage 1 — truthful and safe
| Item | Where |
|---|---|
| Permission tiers 0–4, written down with defaults | `Backend/internal/permission/{tiers,classify,policy}.go`, `docs/PERMISSIONS.md` |
| Approval for tier 2+ in interactive chat (web dialog, TUI prompt, any face can answer) | `permission/broker.go`, `httpapi/approvals.go`, `Frontend/.../approval-{store,dialog}`, `TUI/turn.go` |
| Gate in front of **every** tool, fails closed when a turn has no session | `permission/wrap.go`, `httpapi/tools.go` |
| Schedules get a human-written scope; the model can't set it; crew workers inherit the session | `store/schedules.go`, `httpapi/schedules.go`, `scheduler.go`, `crew.go`, `schedule-dialog.tsx` |
| Refuse non-loopback bind without pairing; no signup from other machines; Origin/Host checks | `internal/netguard`, `config/config.go`, `httpapi/server.go` |
| README + in-app claims corrected; plain-language privacy statement | `README.md`, `PRIVACY.md` |
| Command endpoint was hardcoded `cmd /C` | `httpapi/workspace_git.go` |
| Quota file overwritten on every launch | `Backend/desktop/main.go` (`installQuotas`) |
| API keys never sent back to the page (write-only) | `httpapi/providers.go`, `provider-dialog.tsx`, `providers.tsx` |
| Found while testing: shell timeout didn't kill child processes on Linux/macOS (30s hang) | `agent/shell.go`, `proc/` |
| Found while testing: SSE writer raced between goroutines / wrote after handler returned | `httpapi/server.go` |

## Stage 2 — one brain
| Item | Where |
|---|---|
| Single host per machine; faces attach as guests; `host.json` discovery | `internal/host`, `cmd/server/main.go` (`--daemon`, `status`, `stop`) |
| CLI attaches instead of owning; never kills the host; `cal status` / `cal stop`; `-addr` honoured | `npm-wrapper/lib/launcher.js`, `bin/cal.js` |
| Desktop attaches / starts a background host / falls back to in-process | `Backend/desktop/main.go` |
| Hidden CLI account replaced by machine-owner sign-in (`POST /api/auth/local`, proved by host secret) | `auth/auth.go`, `httpapi/hostapi.go`, `store/users.go`, `TUI/internal/{hostfile,session}` |
| Backend serves the web UI (embedded) | `internal/webui`, `app/app.go`, build scripts, `release.yml` |
| Background mode so schedules run after the window closes | desktop spawns `calabrass-backend.exe --daemon`; `installer/Calabrass.iss`, `build-all.ps1` |
| Provider keys encrypted at rest under an OS-held master key; old plaintext rows migrated on start | `internal/secrets` (DPAPI / Keychain / Secret Service / key file), `store/store.go` |
| CI for backend (Linux+Windows), TUI, launcher, frontend | `.github/workflows/ci.yml` |

## Verified
- `go vet` + `go test -race ./internal/... ./cmd/... ./desktop` — all packages pass (Linux, real SQLite via a test shim).
- Cross-builds of the host: linux/darwin/windows × amd64/arm64. `GOOS=windows go vet` on desktop, secrets (DPAPI), proc.
- Real server binary, end to end: loopback-only refusal, open bind refused without pairing, daemon start, second host defers, owner sign-in, wrong secret/Origin/Host refused, secret-gated `stop`, embedded web UI served, `host.json` is 0600.
- TUI non-UI packages + launcher tests pass (`node --test`: 8/8). Frontend type-checks apart from the missing `components/ui` (below).

## Not verified — please check on your machines
1. **TUI main package** (`app.go panel.go turn.go update.go`): syntax-checked only. Its UI deps (`charm.land/*`) weren't downloadable here, so it was not compiled. The edits are small and follow existing patterns; run `go build ./... && go test ./...` in `TUI/`.
2. **Desktop app** was compiled for Windows (`GOOS=windows go vet`) but not run. Try: open it with nothing running → it should start a background host; close the window → asked once whether to keep it running.
3. **OS key stores** (DPAPI, Keychain, `secret-tool`) compile for their targets; only the key-file fallback was exercised.
4. **Frontend**: `Frontend/src/components/ui/` is missing from the zip you sent, so those imports couldn't resolve; my files type-check otherwise. Not run in a browser. `pnpm-lock.yaml` is also absent but the build scripts use `--frozen-lockfile`.
5. Installer / `release.yml` / `ci.yml` changes are untested (no Inno Setup or GitHub here).

## Known limits (deliberate, documented)
- Commands run with your permissions; the gate asks, it is not an OS sandbox. Tier classification is text-based and cautious, not perfect.
- "Always allow" answers live in memory (reset on host restart). No run-ledger UI yet; decisions are logged.
- No tray icon (Wails v2 has none). The background host is a separate process: reopen the app any time, `calabrass-backend stop` (Start menu) ends it.
- An existing `cal` user keeps their old separate account (so history stays visible); delete `tui.json` to switch to the owner account.
- A browser can't send the pairing header on EventSource streams, so remote *web* use isn't wired; remote CLI is (`CALABRASS_PAIRING_CODE`).

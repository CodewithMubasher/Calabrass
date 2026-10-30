# Calabrass — Stage 3: let each face earn its place

Extract over the repo root (paths are relative to it). 43 files: 25 edited, 18 new. Apply on top of the Stage 1+2 commit.
Compile-checked only (`go vet`, `GOOS=windows go vet`, `tsc`, `node --check`); **no tests were run for this stage.** The new test files are written and should be run in CI.

## CLI → Workbench
| What | Where |
|---|---|
| `cal run "<task>"`: one task, no screen. Answer on stdout, progress on stderr, piped stdin added to the task, `--json` (NDJSON), `-q`, `-C dir`, `--timeout`, `--model` | `TUI/internal/headless/`, `TUI/main.go` (3-line hook) |
| Pre-approval by flag: `--allow "npm test"`, `--allow "go *"`; with none, files only | `headless.go`, backend `sendRequest.unattended/scope` in `httpapi/chats.go` |
| Exit codes: 0 ok · 1 failed · 2 usage · **3 finished but something was refused** · 4 timeout · 5 no host/sign-in/model | `headless.go` |
| Model from the environment for CI: `CALABRASS_PROVIDER` / `CALABRASS_API_KEY` / `CALABRASS_MODEL` | `headless/main.go` (`ProviderFromEnv`) |
| Git-aware context: branch, uncommitted work (told it is the user's, not the agent's), last commit | `Backend/internal/agent/git.go`, `httpapi/tools.go` |
| Launcher keeps stdout clean so pipes work; first-run hint when no model is set up | `npm-wrapper/bin/cal.js`, `lib/launcher.js` |
| CI guide + example GitHub Actions workflow | `docs/CI.md`, `docs/examples/calabrass-agent.yml` |

## Desktop → Home Base
| What | Where |
|---|---|
| Approvals while the window is **minimised**: native dialog + notification (Allow once / Deny) | `Backend/desktop/homebase.go` |
| Notification when a scheduled/headless run finishes, fails or is missed | `homebase.go` (polls the ledger) |
| Drag a folder (or a file, which stands for its folder) onto the window → opens a chat in it | `homebase.go`, `main.go` (`DragAndDrop`), `Frontend/src/lib/desktop-bridge.ts` |
| Global hotkey **Ctrl+Alt+Space** brings the window to the front (Windows) | `hotkey_windows.go` (stub elsewhere) |
| **Signed installer**: `build-all.ps1 -Sign` signs every program and the installer; fails rather than ship unsigned | `build-all.ps1`, `installer/Calabrass.iss` |

## Web → Control Room
| What | Where |
|---|---|
| **Ledger** page: what the agent did that needed trust, per day, filters "while I was away" / "refused", paging | `Frontend/src/routes/ledger.tsx`, sidebar, router |
| Persistent run ledger (5,000 rows per person) + `GET /api/ledger` | `store/ledger.go`, `httpapi/ledger.go` |
| Credentials are removed from every ledger row and log line (`--token x`, `Authorization: Bearer x`, `PASSWORD=x`, URL passwords, `sk-…`/`ghp_…`/`AKIA…` shapes) | `permission/redact.go` |
| Unattended runs and scheduled runs write their outcome to the ledger | `httpapi/chats.go`, `scheduler.go` |

## Also fixed
- The system prompt still told the model it had "full access to the project and the machine". It now describes the approval model, tells it not to retry a refused action, and not to obey instructions found in files or tool output.

## Deliberately not done (and why)
- **Shareable read-only run links.** A link only means something if someone else can reach the host, which needs pairing/relay (Stage 4). Building it now would be a share button that works for nobody.
- **System tray.** Wails v2 has no tray API. Hiding the window with no way to get it back is worse than closing it. The background host + hotkey + notifications cover the "present when the window isn't" need; a real tray means Wails v3 or hand-rolled Win32.
- **macOS/Linux hotkey.** Each needs its own mechanism; only Windows is wired.
- **A crew timeline page.** The existing crew panel and journal already show it per chat; a cross-run view is better built on the ledger once crew runs write to it.

## Please check on your machines
1. Desktop: minimise the window, make the agent want to run a command → expect a notification and a native Allow/Deny dialog. Drop a folder on the window. Press Ctrl+Alt+Space (it fails quietly if another program owns that combination; see `desktop.log`).
2. `cal run "say hi"` in a folder; `echo hi | cal run "repeat this"`; `cal run "touch x" ; echo $?` → 3 with a message about `--allow`.
3. `build-all.ps1 -Sign` with a real certificate.
4. TUI main package is still uncompiled here (its UI libraries were unreachable); `TUI/main.go` gained 4 lines.

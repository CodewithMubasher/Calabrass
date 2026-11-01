# Calabrass — Stage 4 (CLI): disconnect hardening + reaching another machine

Extract over the repo root, on top of Stage 3. 47 files. **Verified this time**: the full backend suite under `-race` (Linux, plus `GOOS=windows go vet`), the TUI suite, the launcher tests (21), `tsc`, and a real end-to-end run of the server binary with the CLI commands.
(The TUI's UI libraries are now compiled here via GitHub mirrors, which also verified the Stage 2/3 TUI edits that had only been syntax-checked.)

## Part 1 — "backend connects/disconnects and it leaks into the CLI and breaks everything"

What was actually wrong, found by reading the code:

| Problem | Effect | Fix |
|---|---|---|
| Transport errors shown raw (`Post "http://127.0.0.1:8787/api/…": dial tcp … connectex …`) | URLs, ports and OS text on screen | `api.ConnError` + `api.Explain`: "can't reach the Calabrass host", one plain line |
| The 30-second schedule poll kept failing and every failure was **appended to the conversation** as an error message | A flood of permanent error lines while the host was down | One calm footer line; polling stops while down |
| A reply cut off when the host died ended with a clean close and no `done` event, and was **shown as a finished answer** | Silent truncation | A stream that ends without `done`/`error` is `ConnLost`; what arrived is kept, `/retry` offered |
| No recovery at all (boot screen needed a manual `r`; nothing retried; outdated hint) | "Breaks everything" until restart | Automatic reconnect with 1/2/4/8/10 s pauses; re-signs in; reloads; says "Reconnected" once |
| Host that died stayed dead | Person had to fix it | From the 2nd try the UI **starts the host again** (local host only, only the host's own program) |
| A stalled connection hung forever | Frozen screen | Idle watchdog: host now sends heartbeats; 3 missed beats = "host stopped responding". **Armed only when the host promises heartbeats**, so older hosts are never cut off during a long command |
| Text from a model, tool, error page or server went straight to the terminal | Escape sequences could move the cursor, set the title, write the **clipboard (OSC 52)**, or hide a command with bidi tricks | `termsafe.Clean` at every boundary (events, chats, messages, traces, approvals, API errors, schedules) |
| Unbounded line reads, unbounded replies | Memory exhaustion by a broken or hostile server | 16 MiB line cap, 64 MiB reply cap |
| Turn context never released | Leak | Cancelled when the turn ends |
| `cal run`: all failures were exit 5 | Pipelines couldn't tell "never connected" from "died mid-run" | New **exit 6**: host lost mid-run (files may be partly changed; safe to re-run) |

Also fixed while in there:
- **Regression from Stage 1**: the TUI still expected `apiKey` in the provider list, which the host stopped sending. Provider status and the edit form were wrong. Now `hasKey`/`keyHint`; the key field is write-only.
- `cal run` printed each tool call while still `pending`; the real lifecycle is pending → running → ok/error.
- A `tea.Batch` I'd added on every turn event (harmless to behaviour, now only when an approval prompt opens).

## Part 2 — Phase 4 "Reach", from the CLI

Replaces the single shared `CALABRASS_PAIRING_CODE` with real **device pairing**.

```
host machine:   CALABRASS_ALLOW_REMOTE=1 …   then   cal pair       → K7QD-M2XH (once, 10 min)
other machine:  cal connect 192.168.1.20:8787 K7QD-M2XH
host machine:   cal devices      cal revoke "Ada's laptop"          → signed out at once
device:         cal disconnect   (cal --local = use own host once)
```

- Backend: `devices` and `pairing_codes` tables; sessions gain `device_id`; `POST /api/pairing/start|claim`, `GET /api/devices`, `DELETE /api/devices/{id}`; network guard now lets a remote machine in **only** with a paired-device session (or to claim a code). Pairing management is machine-only. Remote password sign-in, signup and owner sign-in are refused.
- Codes: ~38 bits, hashed at rest, single-use (atomic: of 20 simultaneous claims exactly one wins), 10-minute expiry, max 3 unused per person, every wrong code gives the same answer, rate-limited (5/min per address, 40/min overall).
- Device sessions last 90 days and die with the device. Pairing and removal go in the ledger.
- `cal connect` **refuses a public address** (the connection is plain HTTP) unless `--insecure`, warns on a LAN address, and treats Tailscale/loopback as safe. This fired during testing on the sandbox's documentation-range IP, which is the behaviour you want.
- Device token lives in `remote.json` (0600) and is passed to the TUI by environment; it is never written to TUI settings. A removed device gets "no longer paired… pair again", not an endless reconnect loop. `cal status` on a device says whether the host still accepts it.
- Docs: `docs/REMOTE.md` (design, limits, SSH-tunnel recipe), README, `cal --help`.

## Deliberately not done
- **Encryption.** It is plain HTTP; the docs and `cal connect` say so loudly and point to SSH tunnels/VPN. TLS or a relay is its own piece of work and should not be rushed.
- **Devices page in the web UI / the mobile app.** Device management is CLI-only for now; the ledger shows pairing events.
- **Pairing from the desktop app.** Same API; no UI yet.

## Please check
1. `go build ./... && go test ./...` in `TUI/` — one test fails (`TestFullAgentTurnRendersReasoningToolsEditsAndAnswer`). It **fails identically on your original, untouched source** in this environment; I did not cause it and did not change it. Worth a look on your machine.
2. Kill the host while `cal` is open: footer should say "Host disconnected · reconnecting", and the host should come back by itself.
3. Pair a real second machine over your LAN.

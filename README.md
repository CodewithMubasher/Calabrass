# Calabrass AI

A local-first AI chat app: React + Vite frontend, Go + SQLite backend, streaming
replies over Server-Sent Events.

```
Calabrass/
├─ Backend/     Go API, SQLite persistence, provider streaming (GoAI SDK)
├─ Frontend/    React 19 + Vite + Tailwind v4 + Base UI
├─ dev.ps1      Runs both dev servers together
└─ dev.cmd      Same, for cmd.exe / double-click
```

## Run everything with one command

```powershell
.\dev.ps1
```

or, if your PowerShell execution policy blocks scripts:

```cmd
dev.cmd
```

Both start in the foreground and stream their logs into the same console.
**Ctrl+C stops both.** If either process exits on its own, the other is shut
down too.

| Service  | URL                            |
| -------- | ------------------------------ |
| Frontend | http://localhost:5173          |
| Backend  | http://127.0.0.1:8787/api      |

The frontend calls the backend through the Vite dev proxy (`/api` → `:8787`), so
there is no CORS setup to worry about.

## Setup

1. Copy `Backend/.env.example` to `Backend/.env` and add provider keys:

   ```
   GOOGLE_GENERATIVE_AI_API_KEY=...
   GROQ_API_KEY=...
   CALABRASS_PROVIDER_PRIORITY=groq,google
   ```

   `Backend/.env` is gitignored. `CALABRASS_PROVIDER_PRIORITY` decides which
   provider is offered first and used as the fallback — Groq is the default
   because it is the verified-working provider.

2. Install frontend dependencies:

   ```powershell
   cd Frontend; pnpm install
   ```

3. Run `.\dev.ps1`.

## API

| Method | Path                          | Purpose                             |
| ------ | ----------------------------- | ----------------------------------- |
| GET    | `/api/health`                 | Liveness + configured providers     |
| GET    | `/api/models`                 | Live model catalog (`?refresh=1`)   |
| GET    | `/api/chats`                  | List chats, newest first            |
| POST   | `/api/chats`                  | Create a chat                       |
| GET    | `/api/chats/{id}`             | One chat with its messages          |
| PATCH  | `/api/chats/{id}`             | Rename / change model               |
| DELETE | `/api/chats/{id}`             | Delete a chat                       |
| POST   | `/api/chats/{id}/messages`    | Send a turn, stream the reply (SSE) |

The message endpoint replies with a Server-Sent Event stream:

```
event: chat      -> the chat summary (auto-title, selected model)
event: message   -> the persisted user message
event: token     -> one text delta of the reply (repeated)
event: done      -> the persisted assistant message + token usage
event: error     -> provider/stream failure
```

A partial reply is still persisted if generation fails or the client
disconnects mid-stream.

## Notes

- Models are listed live from each provider, so the picker always reflects what
  your keys can actually reach.
- Chats and messages live in SQLite at `Backend/data/calabrass.db`.

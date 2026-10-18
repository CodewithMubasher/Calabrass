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

## Accounts

The backend creates the original `admin@cal.com` / `Zorin1122` account on first
start, so existing installs keep working. Use **Sign up** on the login page to
create another account. Every account is fully isolated: each one has its own
chats, messages, providers, workspaces, connectors, agents, schedules and
usage. Rows written before accounts existed belong to the admin account.

Passwords are stored as bcrypt hashes in SQLite and a sign-in lasts 30 days via
a bearer token; the database only stores the token's SHA-256. Every API
request must present that token, and the backend scopes all reads and writes to
the account it belongs to. The API is bound to the loopback interface and
remains reachable by local processes, so the account gate decides what the app
shows rather than what the machine allows.

## Desktop app (Wails)

The desktop shell is in `Backend/desktop`. It runs the same Go backend on
`127.0.0.1:8787` and embeds the same React frontend, so the whole app — chat,
Cal Live, workspaces, agents, connectors, schedules, providers, usage and
settings — is available in a native window.

Inside the repository it deliberately uses `Backend/.env` and
`Backend/data/calabrass.db`, so the desktop and browser builds share chats and
providers. The built `.exe` finds the repository when it is launched from
anywhere inside it; copied elsewhere, it falls back to
`%APPDATA%\Calabrass` for its database and workspaces.

```powershell
# once
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

# live development, with Vite hot reload
cd Backend\desktop
wails dev

# production build
wails build
# -> Backend\desktop\build\bin\Calabrass.exe
```

The window icon comes from `Frontend/public/vite.svg`, rendered into
`Backend/desktop/build/appicon.png` and `build/windows/icon.ico`.

If the browser dev server (`dev.ps1`) is already running, the desktop app
reuses its backend instead of starting a second one, so only one process owns
the database and the port.

## API

| Method | Path                          | Purpose                             |
| ------ | ----------------------------- | ----------------------------------- |
| GET    | `/api/health`                 | Liveness + configured providers     |
| POST   | `/api/auth/signup`            | Create an account, return a session |
| POST   | `/api/auth/login`             | Check a password, return a session  |
| GET    | `/api/auth/me`                | Resolve an `Authorization` token    |
| POST   | `/api/auth/logout`            | Revoke an `Authorization` token     |
| GET    | `/api/models`                 | Live model catalog (`?refresh=1`)   |
| GET    | `/api/providers`              | Providers + on/off state            |
| POST   | `/api/providers`              | Add a custom provider               |
| PUT    | `/api/providers/{id}`         | Edit a provider, or turn it on/off  |
| DELETE | `/api/providers/{id}`         | Remove a custom provider            |
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
event: reasoning -> one delta of the model's thinking (repeated, optional)
event: token     -> one text delta of the reply (repeated)
event: done      -> the persisted assistant message + token usage
event: error     -> provider/stream failure
```

A partial reply is still persisted if generation fails or the client
disconnects mid-stream.

## Notes

- Models are listed live from each provider, so the picker always reflects what
  your keys can actually reach.
- Turning a provider off on the Providers page stores the choice in SQLite and
  removes that provider from `/api/models`, so its models leave the composer's
  picker and are refused by the send path.
- Providers are rows in SQLite. A built-in (Google, Groq) keeps its key in
  `Backend/.env` unless the Providers page saves an override; a custom provider
  you add carries its own base URL and key, and is called through GoAI's
  OpenAI-compatible client. Because the API is bound to the loopback interface
  and is for a single local user, `/api/providers` returns the effective key so
  the detail dialog can show and edit it.
- Chats and messages live in SQLite at `Backend/data/calabrass.db`.
- The model's thinking is streamed on its own channel, never mixed into the
  answer, and stored alongside the message. The UI renders it with the shadcn
  [Trace](https://ui.shadcn.com/docs/components/trace) component as a
  "Thinking → Thought for Ns" row that opens while the model thinks and closes
  when it is done (`Frontend/src/components/reasoning-trace.tsx`).

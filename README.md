<div align="center">

  <img src="logo-preview.png" alt="Calabrass Logo" width="380" />

  <p><strong>The open source personal AI agent.</strong></p>

  [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
  ![Go](https://img.shields.io/badge/Go-1.23-00ADD8?logo=go&logoColor=white)
  ![React](https://img.shields.io/badge/React-19-61DAFB?logo=react&logoColor=black)
  ![TypeScript](https://img.shields.io/badge/TypeScript-5-3178C6?logo=typescript&logoColor=white)
  ![SQLite](https://img.shields.io/badge/SQLite-pure--Go-003B57?logo=sqlite&logoColor=white)
  ![Wails](https://img.shields.io/badge/Wails-v2-FF6B6B?logo=wails&logoColor=white)
  ![Tailwind](https://img.shields.io/badge/Tailwind-v4-06B6D4?logo=tailwindcss&logoColor=white)

  [Screenshots](#screenshots) · [Features](#features) · [Quick start](#quick-start) · [Configuration](#configuration) · [Architecture](#architecture) · [Providers](#providers---bring-your-own-keys) · [Tools](#built-in-tools) · [MCP](#mcp-model-context-protocol-host) · [Roadmap](#roadmap)

</div>

---

Calabrass is a **local-first, single-user AI assistant** that runs entirely on your machine. It combines a chat interface, an agent that can use tools (file system, web, desktop automation), a workspace-aware file context, and a Model Context Protocol (MCP) host — all in one native Windows app. No cloud account required, no data leaves your computer.

## 📸 Screenshots

<p align="center">
  <img src="homescreen.png" alt="Calabrass home screen" width="100%"/>
</p>

---

## ✨ Features

### 💬 Chat & Composer
- Streaming replies over Server-Sent Events — tokens appear as they are generated.
- **Reasoning / thinking** streamed on its own channel, rendered inline with a clean trace UI.
- **Model picker** shows live models from every provider you configure — always up to date.
- Auto-generated chat titles from the first user message.
- Per-chat model selection; switch mid-conversation.

## 🛠️ Built-in Tools

### 🤖 Agent & Tools
- Built-in tools: `read_file`, `write_file`, `list_files`, `glob`, `grep`, `run_shell`, `open_app`, `web_search`, `fetch_url`.
- **Agent loop** plans, calls tools, and iterates until the task is done.
- **Workspaces** — point the agent at a folder and it reads/writes files there with full context.
- **Schedules** — cron-style tasks that run agents on a timer.

## 🔌 MCP Connectors

### 🔌 MCP (Model Context Protocol) Host
- Starts and manages MCP servers as child processes.
- **Bundled servers ship with the installer** (opt-in, off by default):
  - `excel-mcp` — create, read, and modify Excel workbooks.
  - `win-control-mcp` — window management, keyboard/mouse automation, screenshots.
- Add your own MCP servers by path; they appear in the Connectors page, one click to enable.

## 🔑 Providers — Bring Your Own Keys
- **No built-in providers, no keys in the repo.** Add any OpenAI-compatible endpoint (Groq, OpenAI, Together, local vLLM/Ollama, etc.) or Google Gemini (native adapter).
- Provider rows live in your account's SQLite database — fully isolated per user.
- Turn providers on/off without deleting them; models disappear from the picker instantly.
- Groq endpoint automatically gets `max_completion_tokens` and the reasoning-content fix.
- Gemini endpoint uses the native Generative Language API.

### 🗣️ Cal Live (Voice)
- Real-time speech-to-text via Mistral or Groq Whisper.
- Push-to-talk or voice-activity detection.
- Transcripts land in the chat composer, ready to send.

### 💾 Data Ownership & Privacy
- **Everything yours stays in `%LOCALAPPDATA%\Calabrass`** (Windows) or the platform config folder:
  - `calabrass.db` — chats, messages, providers, connectors, agents, schedules, usage.
  - Workspace folders the agent creates.
  - `desktop.log` — structured JSONL logs.
- **Nothing user-made is ever written into the source checkout.** Clone, clean, update, delete the repo — your data is untouched.
- API bound to `127.0.0.1:8787`; no network exposure.

### 🖥️ Native Windows Installer
- Per-user install — **no UAC prompt**.
- Wizard: welcome → MIT licence → install folder → Start Menu folder → optional desktop shortcut + bundled MCP servers → ready → install → finish with "Launch Calabrass" tickbox.
- WebView2 runtime check; offers Microsoft download page if missing.
- Uninstall asks before touching `%LOCALAPPDATA%\Calabrass` (keeps it by default).
- `excel-mcp.exe` and `win-control-mcp.exe` installed beside the app, built as GUI subsystems (`-H=windowsgui`) — **no blank console windows**.
- SHA256: `767b9ebadd99f0b6e66076cdf52b492119c65d46533bab26f102899b7eb99ff8`

---

## 🚀 Quick Start

### Option A — Install the built app (recommended)
1. Download **`Calabrass-setup-v1.0-x64.exe`** from [Releases](https://github.com/CodewithMubasher/Calabrass/releases).
2. Run it. Windows SmartScreen may warn "Windows protected your PC" (not code-signed yet) → **More info → Run anyway**.
3. On first launch, click **Sign up**, create your account, then open **Providers** and add at least one provider (base URL + API key).
4. Start chatting.

### Option B — Run from source (development)
```powershell
# 1. Frontend deps
cd Frontend; pnpm install

# 2. Run both servers together (Ctrl+C stops both)
.\dev.ps1
```
- Frontend: http://localhost:5173
- Backend API: http://127.0.0.1:8787/api

### Desktop development (Wails)
```powershell
# once
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0

# live dev with Vite HMR
cd Backend\desktop
wails dev

# production build
wails build
# -> Backend\desktop\build\bin\Calabrass.exe
```

---

## 🏗️ Architecture

## 📁 Repository Structure

```
Calabrass/
├─ Backend/              Go API, SQLite, provider streaming (GoAI SDK)
│  ├─ internal/ai/       Provider registry, model catalog, GoAI adapters
│  ├─ internal/httpapi/  REST + SSE endpoints, auth, connectors, voice
│  ├─ internal/store/    SQLite schema, migrations, queries
│  ├─ internal/connectors/  MCP server pool, bundled server registry
│  └─ desktop/           Wails v2 shell (Go + embedded React)
├─ Frontend/             React 19 + Vite + Tailwind v4 + Base UI
├─ MCP/                  MCP server sources (excel, win-control)
├─ installer/            Inno Setup script + build script
├─ dev.ps1 / dev.cmd     One-command dev stack
└─ LICENSE               MIT
```

---

## 🔧 Configuration

No `.env` required for providers — add them in the UI.  
Optional backend settings via `%LOCALAPPDATA%\Calabrass\.env` (see `Backend/.env.example`):

```
CALABRASS_ADDR=127.0.0.1:8787          # API listen address
CALABRASS_DB=calabrass.db               # DB filename (inside data folder)
MISTRAL_API_KEY=...                     # For Cal Live (optional)
CALABRASS_VOICE_PROVIDER=mistral        # or "groq"
CALABRASS_VOICE_MODEL=voxtral-mini-latest
CALABRASS_SYSTEM_PROMPT=...             # Global system prompt override
CALABRASS_WORKSPACES_DIR=workspace      # Subfolder inside data folder
CALABRASS_QUOTAS=model_free_tier_quotas.md  # Free-tier limits file
```

---

## 🧱 Tech Stack

| Layer | Technology |
|-------|------------|
| Frontend | React 19, Vite, Tailwind CSS v4, Base UI, TanStack Router, TanStack Query |
| Backend | Go 1.23, SQLite (modernc.org/sqlite), GoAI SDK, Server-Sent Events |
| Desktop | Wails v2 (native WebView2 window) |
| MCP | Official Go MCP SDK, stdio transport |
| Auth | bcrypt (cost 12), 30-day bearer tokens, SHA-256 token storage |
| Logging | slog JSONL to `%LOCALAPPDATA%\Calabrass\desktop.log` |

---

## 🗺️ Roadmap

- [ ] Code-signed installer (remove SmartScreen warning)
- [ ] macOS / Linux builds (Wails supports them)
- [ ] Plugin system for custom tools
- [ ] Shared workspace collaboration (opt-in, local network)
- [ ] Agent memory / long-term context across sessions
- [ ] More bundled MCP servers (browser, database, git)

---

## 📄 License

MIT — see [LICENSE](LICENSE).  
Copyright (c) 2026 Mubasher Chaudhary.

---

## 🙏 Acknowledgements

- [GoAI](https://github.com/zendev-sh/goai) — unified provider interface
- [Wails](https://wails.io/) — Go + WebView2 desktop apps
- [MCP](https://modelcontextprotocol.io/) — Model Context Protocol
- [Base UI](https://base-ui.com/) — unstyled, accessible React components
- [shadcn/ui](https://ui.shadcn.com/) — Trace component for reasoning display
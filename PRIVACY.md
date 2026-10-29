# Privacy

Calabrass runs on your computer. This page says exactly what stays there and what does not.

## What stays on your machine

- Your chats, files, workspaces, schedules, usage history and settings, in Calabrass's data folder:
  - Windows: `%LOCALAPPDATA%\Calabrass`
  - macOS: `~/Library/Application Support/Calabrass`
  - Linux: `~/.config/Calabrass`
- Your model-provider API keys. They are **encrypted at rest** under a master key your operating system holds (Windows DPAPI, the macOS Keychain, or the Linux Secret Service; an owner-only key file where none is available), and the web page never receives them back. Copying `calabrass.db` somewhere does not copy your keys with it.
- There is **no Calabrass account in the cloud**, and Calabrass sends **no telemetry or analytics**. Nothing in the source phones home.
- The API listens only on `127.0.0.1`. The host refuses to listen on any other address unless you set a pairing code (see "Other devices" below).

## What leaves your machine, and when

Calabrass is an agent, so it has to talk to a model. These are the cases, and they are all things you set up:

| What | Where it goes | When |
|---|---|---|
| The text of your conversation, the files and command output the agent reads, and tool results | **The model provider you added** (OpenAI, Groq, Gemini, a local server, ...) | Every time you send a message or the agent takes a step. If your provider is a server on your own machine (Ollama, vLLM), this stays on your machine too. |
| Voice audio from Cal Live | **Mistral or Groq**, whichever you configured, for transcription | Only while you use Cal Live |
| Whatever a connector does | **Whatever service the connector talks to**: Gmail goes to Google, a web search goes to a search engine | Only when you add a connector and the agent uses it |

Two consequences worth knowing:

- Anything the agent can read in your project, it may send to your model provider as part of working on it. Do not point it at a folder holding things you would not paste into that provider's chat window.
- A file or web page the agent reads can contain instructions written by someone else. That is why the agent asks before running commands or acting on the outside world (see [permissions](docs/PERMISSIONS.md)).

## Other devices

By default nothing but this machine can reach Calabrass. You can allow other devices by starting the host with `CALABRASS_PAIRING_CODE` set to a secret of at least 16 characters. Those devices must present the code on every request, can never create accounts, and still need to sign in. Treat the code like a password: with it and a sign-in, a device can use the agent.

## Your control

- **Delete your data:** close Calabrass (`cal stop`) and delete the data folder.
- **Reset what the agent is allowed to do without asking:** restart the host (`cal stop`, then start it again). Remembered approvals live in memory only.
- **See what ran:** the host log (`host.log`, `desktop.log` in the data folder) records every command the agent asked to run, how it was decided, and who or what decided it.

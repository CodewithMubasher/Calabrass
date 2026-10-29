# How permissions work

The agent can run commands on your computer. Reaching the API is as good as reaching a shell, so Calabrass puts a gate in front of **every** tool the agent uses, built-in or from a connector, and decides by what an action *does*, not by what the tool is called.

## The five tiers

| Tier | What it is | Examples | Interactive chat | Unattended (schedules, no one to ask) |
|---|---|---|---|---|
| **0 Observe** | Look, inside the project | `read_file`, `list_dir`, `grep`, `git status`, `ls` | Runs | Runs |
| **1 Change files** | Create or edit files inside the project | `write_file`, `edit_file` | Runs | Runs |
| **2 Run a command** | Execute something | `npm test`, `go build`, a connector action | **Asks.** You may remember the answer. | Only commands listed in the task's scope |
| **3 Reach outside** | Leave the project, or do something hard to undo | `rm -rf`, `sudo`, paths outside the project, `curl ... \| sh`, installing software | **Asks every time** | **Never** |
| **4 Act on the outside world** | Send, publish, push, delete remotely, spend | `git push`, `npm publish`, `ssh`, `kubectl apply`, a connector's `send_email` | **Asks every time** | **Never** |

A command the classifier does not recognise is tier 2, never tier 0. A chained command (`a && b`, pipes, redirection) takes the highest tier of its parts, and is never remembered as a pattern: what you approved would not be what runs next time.

## Answering an approval

When the agent needs a yes, you are asked, in whichever window you are using. You can answer from any open Calabrass window; the first answer wins.

- **Allow once** — this one action.
- **Always allow "npm install" here** — that program and sub-command, in this project, until the host restarts. Tier 2 only.
- **Allow for this session** — everything up to tier 2 for the rest of this conversation (including a crew's workers). Anything riskier still asks.
- **Deny** — the agent is told it was refused, not to retry, and to carry on with what it can.

An approval nobody answers within ten minutes counts as a refusal.

## Unattended runs

A scheduled task has no one to ask, so it runs inside a **scope you write when you create it**: it may read and edit files in its project, and run only the commands you list (`npm test`, or `go *` for any go command). Anything else is refused and reported, never guessed at. Nothing unattended can ever reach tier 3 or 4.

**The scope is set by you, never by the model.** A task the agent books for you gets the default scope (files only, no commands); you can widen it on the Schedules page.

## What this does *not* do

Be honest about the limits:

- **Commands run with your permissions.** The gate decides whether to *ask*; it is not an operating-system sandbox. Once you allow a command, it can do whatever that command can do as you. The file tools are confined to the project folder; commands are not.
- The classifier works from the text of the command. It is deliberately cautious, but a determined or obfuscated command could look more harmless than it is. If a request looks wrong, deny it.
- "Always allow" answers are remembered in memory only; they reset when the host restarts.
- Voice and the crew's direct-delegate calls raise approvals that any open window can answer; if none is open, they time out and are refused.

## For developers

The rules live in `Backend/internal/permission` (tiers, the command classifier, the approval broker, and the tool gate). A turn is built under a `Session` that is either interactive or unattended; a turn built with no session is treated as unattended with the default scope, so a missed wiring fails closed. Approvals are served at `GET /api/approvals`, `GET /api/approvals/stream`, and `POST /api/approvals/{id}`.

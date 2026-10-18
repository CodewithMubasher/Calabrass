# Gmail MCP Server

A local MCP server for Gmail, built like the Excel MCP — a self-contained Go binary that speaks MCP over stdio and uses Google's official Gmail API via OAuth2.

## Features

- **search_emails** - Search with Gmail query syntax (`from:`, `subject:`, `is:unread`, `has:attachment`, etc.)
- **read_email** - Read full email content by ID
- **send_email** - Send new emails
- **create_draft** - Create draft emails
- **list_drafts** - List drafts
- **modify_labels** - Add/remove labels (mark read/unread, archive, star, etc.)
- **list_labels** - List all Gmail labels

## Quick Start

### 1. Create Google Cloud Project & OAuth Credentials

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project (or select existing)
3. Enable **Gmail API**: APIs & Services → Library → Gmail API → Enable
4. Create OAuth 2.0 Credentials:
   - APIs & Services → Credentials → Create Credentials → OAuth client ID
   - Application type: **Desktop app**
   - Name: "Calabrass Gmail MCP"
   - Copy **Client ID** and **Client Secret**

### 2. Configure Credentials

**Option A: Environment Variables**
```bash
set GMAIL_CLIENT_ID=your-client-id.apps.googleusercontent.com
set GMAIL_CLIENT_SECRET=your-client-secret
```

**Option B: Config File**
Create `~/.calabrass/gmail_config.json`:
```json
{
  "client_id": "your-client-id.apps.googleusercontent.com",
  "client_secret": "your-client-secret"
}
```

### 3. Build

```bash
cd MCP/gmail
go build -o gmail-mcp.exe .
```

### 4. Test Run

```bash
gmail-mcp.exe
```
First run opens browser for Google OAuth authorization. Token saved to `~/.calabrass/gmail_token.json`.

## Add to Calabrass

1. Open **Connectors** page in Calabrass
2. Click **"New connector"**
3. **Name**: `Gmail`
4. **Command**: `C:\path\to\gmail-mcp.exe` (or just `gmail-mcp.exe` if in PATH)
5. **Enable** ✓
6. **Save**

## Use with Agents

Edit an agent (or create new):
- **MCP connector** dropdown → select `Gmail`
- Agent now has Gmail tools available

## Example Agent Prompt

```
You are a Gmail assistant. You can search, read, send emails, manage drafts and labels.
When the user asks about emails, use search_emails with appropriate queries.
When they want to send, use send_email.
Always confirm before sending.
```

## Configuration Files

| File | Default Location | Purpose |
|------|------------------|---------|
| Token | `~/.calabrass/gmail_token.json` | OAuth2 access/refresh tokens (auto-created) |
| Config | `~/.calabrass/gmail_config.json` | Client ID/Secret (optional, can use env vars) |

## Command Line Flags

```bash
gmail-mcp.exe -token /custom/path/token.json -config /custom/path/config.json
```

## Scopes Used

- `gmail.readonly` - Read emails, labels, drafts
- `gmail.send` - Send emails
- `gmail.modify` - Modify labels (mark read/unread, archive, etc.)
- `gmail.compose` - Create drafts

## Troubleshooting

**"Gmail credentials not configured"**
- Set `GMAIL_CLIENT_ID` and `GMAIL_CLIENT_SECRET` env vars, OR
- Create `~/.calabrass/gmail_config.json` with client_id/client_secret

**"redirect_uri_mismatch"**
- In Google Cloud Console, add `http://localhost:8080/oauth2callback` to authorized redirect URIs

**Token expired / auth failed**
- Delete `~/.calabrass/gmail_token.json` and restart — it will re-authenticate

**Build fails: missing dependencies**
```bash
cd MCP/gmail
go mod tidy
go build
```
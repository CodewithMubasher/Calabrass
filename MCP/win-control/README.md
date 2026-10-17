# win-control-mcp

A small MCP server that lets a model put things in front of the person it is
helping: a song, a web search, a page.

It speaks MCP over **stdio**, so it needs no port, no install and no
configuration. A client starts it as a child process, and the protocol goes out
on stdout while everything else goes to stderr.

## Tools

| Tool | What it does |
| --- | --- |
| `play_music(name)` | Opens YouTube's search results for `name` in the default browser. |
| `search_web(query, engine?)` | Opens a search for `query` — `google` (default), `duckduckgo` or `bing`. |
| `open_url(url)` | Opens a web address. Only `http`/`https` — never `file:` or a shell. |

`open_app(name)` is next: it needs keyboard injection, which is its own piece of
work.

## Build

```
cd MCP/win-control
go build -o win-control-mcp.exe .
```

## Run it by hand

The protocol is one JSON message per line on stdin, so it can be driven from a
shell. `CALABRASS_WIN_CONTROL_DRY_RUN=1` makes it *report* what it would open
instead of opening it — the safe way to watch it work:

```powershell
$env:CALABRASS_WIN_CONTROL_DRY_RUN = "1"
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}',
'{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"play_music","arguments":{"name":"whysokai terrible time to meet"}}}' |
  .\win-control-mcp.exe
```

## Why it is written by hand

The protocol this needs is `initialize`, `tools/list`, `tools/call` and `ping`,
which is a few hundred lines of `encoding/json`. Written that way it has **no
dependencies**, no cgo, and a binary that starts in milliseconds — which is what
a server spawned per turn wants.

Above all it controls the **protocol version**. Clients are strict about the
version they are answered with, so this server echoes back the version it was
asked for rather than announcing a newer one of its own. `internal/protocol`
holds the reasoning in full.

## Layout

```
main.go                     the stdio entry point
internal/protocol/          JSON-RPC over stdio, the tool registry, argument checks
internal/tools/             the three tools: what they promise
internal/desktop/           how it is done on Windows (ShellExecute), and the dry run
```

## Registering it in Calabrass

The Connectors page stores an MCP server as a name and a command line. Add a
connector named `win-control` whose command is the path to
`win-control-mcp.exe`, enable it, and the tools above become available to the
agent — in chat and in Cal Live voice mode.

# excel-mcp

A local MCP server that lets a model **make and change Microsoft Excel
workbooks**: a budget, a report, a list, a chart — a file the person can open in
Excel when the conversation is over.

It speaks MCP over **stdio**, so it needs no port, no install and no Office
behind it. A client starts it as a child process; the protocol goes out on
stdout and everything else goes to stderr. Excel files are read and written with
[`github.com/xuri/excelize`](https://github.com/xuri/excelize), so the server is
one self-contained binary.

Every tool call **opens the workbook, makes its change and saves it again**, so
the file on disk is the truth at every moment. That is also why the server
answers one message at a time — two calls racing on the same file would save
over each other.

## Tools

Making and finding workbooks:

| Tool | What it does |
| --- | --- |
| `create_workbook(path, sheets?, overwrite?, title?, creator?)` | Makes a new `.xlsx`, naming its sheets in order. Refuses to replace an existing file unless `overwrite` is true. |
| `list_workbooks(directory?, recursive?)` | Lists the workbooks in a folder, with size and modified time. |
| `get_workbook_info(path)` | Describes a workbook: every sheet, its used range, hidden sheets, tables, merges, frozen panes, dropdowns, conditional rules and file properties. |

Sheets:

| Tool | What it does |
| --- | --- |
| `add_sheet(path, name)` | Adds an empty sheet at the end. |
| `rename_sheet(path, sheet, new_name)` | Renames a sheet; formulas follow on their own. |
| `delete_sheet(path, sheet)` | Removes a sheet. |
| `copy_sheet(path, source, name)` | Copies a sheet, values and all. |
| `set_sheet_visibility(path, sheet, state)` | Shows or hides a sheet. |

Reading and writing:

| Tool | What it does |
| --- | --- |
| `read_range(path, sheet?, range?, raw?, formulas?, trim?)` | Reads a range (or the whole sheet) as rows of values; can also return the formulas. Formula results are worked out by the server, since a workbook it made has never been opened by Excel. |
| `write_range(path, sheet?, start?, values, autofit?)` | Writes a block of values from a starting cell. Text starting with `=` is written as a formula; an apostrophe keeps a leading `=` as text. |
| `append_rows(path, sheet?, values)` | Adds rows under a sheet's existing data, lined up with the columns above. |
| `write_table(path, sheet?, start?, headers?, rows?, table?, style?, autofit?)` | Writes a header row and data in one call, as a real Excel table (filter buttons, a name) by default. |
| `clear_range(path, sheet?, range, what?)` | Empties a range — `contents` keeps formatting, `all` clears it too. |
| `insert_rows` / `delete_rows` / `insert_columns` / `delete_columns` | Make room in a sheet or take it away. |

Styling and rules:

| Tool | What it does |
| --- | --- |
| `format_range(path, sheet?, range, style)` | Bold/italic, font and size, text and fill colour, number format, alignment, wrapping, rotation, borders. `number_format` takes a preset (`currency`, `percent`, `date`, `integer`, `decimal`…) or a raw Excel format code. |
| `set_column_width(path, sheet?, columns, width)` | Sets column width in Excel's units (`0` hides). |
| `set_row_height(path, sheet?, rows, height)` | Sets row height in points (`0` hides). |
| `merge_cells(path, sheet?, range, unmerge?, center?)` | Joins a range into one cell for a title. |
| `freeze_panes(path, sheet?, cell?, unfreeze?)` | Holds rows/columns still while the rest scrolls (`A2` holds the header row). |
| `set_auto_filter(path, sheet?, range)` | Puts a filter button on a range's first row. |
| `conditional_format(path, sheet?, range, rule, value?, value2?, fill_color?, font_color?, bold?, mid_color?, bar_color?, stop_if_true?)` | Colours cells by their values, plus `color_scale` and `data_bar`. |
| `data_validation(path, sheet?, range, kind, values?, formula1?, formula2?, operator?, allow_blank?, title?, message?, input_title?, input_message?)` | Limits what a cell accepts; a `list` gives a dropdown. |

Charts and output:

| Tool | What it does |
| --- | --- |
| `add_chart(path, sheet, source, type, title?, position?, width?, height?, has_header?, has_categories?, show_legend?)` | Draws a chart from a range: `column`, `bar`, `line`, `area`, `pie`, `doughnut`, `radar`, `scatter` and their stacked variants. Linked to the cells it reads. |
| `search_cells(path, query, sheet?, match_case?, whole_cell?, max_results?)` | Finds cells holding a piece of text, across one sheet or the whole workbook. |
| `export_csv(path, sheet?, output?, raw?)` | Writes a sheet out as `.csv`/`.tsv`. `raw` (on by default) writes stored numbers (`4.5`), not the displayed text (`$4.50`). |

## Build

```
cd MCP/excel
go build -o excel-mcp.exe .
```

Cross-compilation from another platform works too: `GOOS=windows GOARCH=amd64
go build -o excel-mcp.exe .`

## Where files land

Relative paths are taken from one folder, and absolute paths must stay inside it
unless the server is told otherwise:

- the `-root <folder>` flag, or
- `CALABRASS_EXCEL_ROOT`, or
- `Documents\Calabrass` under the user's home, or
- the current directory, in that order.

`CALABRASS_EXCEL_ALLOW_OUTSIDE=1` lifts the rule that an absolute path must stay
inside that folder. Only `.xlsx` is written: an `.xlsm` is refused on purpose,
because re-saving one through the library would silently drop its macros.

## Run it by hand

The protocol is one JSON message per line on stdin, so it can be driven from a
shell:

```powershell
'{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}',
'{"jsonrpc":"2.0","method":"notifications/initialized"}',
'{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"create_workbook","arguments":{"path":"demo.xlsx","sheets":["Report"]}}}',
'{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_table","arguments":{"path":"demo.xlsx","sheet":"Report","headers":["Region","Sales"],"rows":[["North",1200],["South",980]]}}}' |
  .\excel-mcp.exe -root "$env:TEMP\excel-demo"
```

`demo.xlsx` appears in the folder `-root` names, and can be opened in Excel.

## Layout

```
main.go                     the stdio entry point, the -root flag
internal/protocol/          JSON-RPC over stdio, the tool registry, argument checks
internal/workbook/          paths and the root boundary, opening/saving, ranges and
                            cells, and the conversions between a model's words and
                            the library's (styles, number formats)
internal/tools/             the 28 tools: what each promises and how it keeps it
```

## Why it is written by hand

The protocol this needs is `initialize`, `tools/list`, `tools/call` and `ping` —
a few hundred lines of `encoding/json`. Written that way, the server's only
dependency is the spreadsheet library itself, not a transport, and it keeps
control of the **protocol version**: clients are strict about the version they
are answered with, so this server echoes back the version it was asked for
rather than announcing a newer one of its own. `internal/protocol` holds the
reasoning in full, copied from `win-control` and extended.

## Registering it in Calabrass

The Connectors page stores an MCP server as a name and a command line. Add a
connector named `excel` whose command is the path to `excel-mcp.exe`:

```
"C:\Users\<you>\Desktop\Calabrass\MCP\excel\excel-mcp.exe"
```

The quotes are needed because the project path contains a space. To choose a
folder other than `Documents\Calabrass`, pass `-root`:

```
"C:\Users\<you>\Desktop\Calabrass\MCP\excel\excel-mcp.exe" -root "D:\Spreadsheets"
```

Enable it, and the tools above become available to the agent — in chat and in
Cal Live voice mode.

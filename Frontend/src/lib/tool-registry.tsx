import type { IconSvgElement } from "@hugeicons/react"
import {
  BotIcon,
  File02Icon,
  FileAddIcon,
  FileEditIcon,
  Folder01Icon,
  FolderAddIcon,
  MessageQuestionIcon,
  Search01Icon,
  TerminalIcon,
  Wrench01Icon,
} from "@hugeicons/core-free-icons"

import { connectorIcon } from "@/lib/connector-icons"
import type { TracePart } from "@/types/chat"

/**
 * How a tool reads in the timeline: the icon on its node, the label beside it,
 * and the snippet that says what the call was about.
 *
 * Labels are present tense while a call is in flight ("Reading file") and past
 * tense once it lands ("Read file"), so a running timeline reads as a sequence
 * of things happening.
 */
export interface ToolRenderer {
  icon: IconSvgElement
  /** The label for a call that has landed. */
  label: string
  getLabel?: (part: TracePart) => string
  /** The file, pattern or command the call was about, shown dimmed after the label. */
  getFileName?: (part: TracePart) => string | undefined
}

/** Waiting and running both count as live: the call has not landed yet. */
export function isCallLive(part: TracePart): boolean {
  return (
    part.status !== "ok" && part.status !== "error" && part.status !== "skipped"
  )
}

/** A call that failed, whether the status says so or only the error does. */
export function isCallFailed(part: TracePart): boolean {
  return part.status === "error" || part.error !== undefined
}

/** The call's arguments, parsed. Malformed arguments simply yield nothing. */
function args(part: TracePart): Record<string, unknown> {
  if (!part.input || part.input.trim() === "") return {}
  try {
    const parsed: unknown = JSON.parse(part.input)
    if (
      parsed !== null &&
      typeof parsed === "object" &&
      !Array.isArray(parsed)
    ) {
      return parsed as Record<string, unknown>
    }
  } catch {
    // The raw payload still reaches the reader through the node's detail panel.
  }
  return {}
}

function str(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined
}

/**
 * The last segment of a path, which is what a folded row has room for. Both
 * separators are handled, since the model writes paths either way.
 */
function baseName(path: string | undefined): string | undefined {
  if (path === undefined) return undefined
  const segments = path.split(/[\\/]/)
  return segments[segments.length - 1] || path
}

/**
 * The worker a delegation went to. Once the report lands it names the worker
 * itself; while the call is still running, the id is all there is, so it is
 * turned back into words ("builtin-frontend-developer" -> "Frontend developer").
 */
function delegateWorker(part: TracePart): string | undefined {
  if (part.output) {
    try {
      const report = JSON.parse(part.output) as { workerName?: unknown }
      const name = str(report.workerName)
      if (name !== undefined) return name
    } catch {
      // A failed call may carry prose instead of a report; fall through.
    }
  }
  const id = str(args(part).workerId)
  if (id === undefined) return undefined
  const words = id.replace(/^builtin-/, "").replace(/[-_]+/g, " ").trim()
  return words.charAt(0).toUpperCase() + words.slice(1)
}

const toolRenderers: Record<string, ToolRenderer> = {
  read_file: {
    icon: File02Icon,
    label: "Read file",
    getLabel: (part) => (isCallLive(part) ? "Reading file" : "Read file"),
    getFileName: (part) => baseName(str(args(part).path)),
  },
  write_file: {
    icon: FileAddIcon,
    label: "Wrote file",
    getLabel: (part) => (isCallLive(part) ? "Writing file" : "Wrote file"),
    getFileName: (part) => baseName(str(args(part).path)),
  },
  edit_file: {
    icon: FileEditIcon,
    label: "Edited file",
    getLabel: (part) => (isCallLive(part) ? "Editing file" : "Edited file"),
    getFileName: (part) => baseName(str(args(part).path)),
  },
  create_dir: {
    icon: FolderAddIcon,
    label: "Created folder",
    getLabel: (part) =>
      isCallLive(part) ? "Creating folder" : "Created folder",
    getFileName: (part) => baseName(str(args(part).path)),
  },
  list_dir: {
    icon: Folder01Icon,
    label: "Listed folder",
    getLabel: (part) => (isCallLive(part) ? "Listing folder" : "Listed folder"),
    getFileName: (part) => baseName(str(args(part).path)) ?? ".",
  },
  grep: {
    icon: Search01Icon,
    label: "Searched",
    getLabel: (part) => (isCallLive(part) ? "Searching" : "Searched"),
    getFileName: (part) => {
      const pattern = str(args(part).pattern)
      if (pattern === undefined) return undefined
      const include = str(args(part).include)
      return include === undefined ? pattern : `${pattern} in ${include}`
    },
  },
  glob: {
    icon: Search01Icon,
    label: "Found files",
    getLabel: (part) => (isCallLive(part) ? "Finding files" : "Found files"),
    getFileName: (part) => str(args(part).pattern),
  },
  run_command: {
    icon: TerminalIcon,
    label: "Ran command",
    getLabel: (part) => (isCallLive(part) ? "Running command" : "Ran command"),
    getFileName: (part) => str(args(part).command),
  },
  ask_questions: {
    icon: MessageQuestionIcon,
    label: "Asked questions",
    getLabel: (part) =>
      isCallLive(part) ? "Asking questions" : "Asked questions",
    getFileName: (part) => {
      const asked = args(part).questions
      if (!Array.isArray(asked)) return undefined
      const count = asked.length
      return count === 1 ? "1 question" : `${count} questions`
    },
  },
  delegate_task: {
    icon: BotIcon,
    label: "Delegated task",
    getLabel: (part) =>
      isCallLive(part)
        ? "Delegating task"
        : isCallFailed(part)
          ? "Delegation failed"
          : "Delegated task",
    getFileName: (part) => delegateWorker(part),
  },
}

/** Anything without a renderer still gets an icon and something to read. */
const fallbackRenderer: ToolRenderer = {
  icon: Wrench01Icon,
  label: "Tool",
  getFileName: (part) => {
    const entries = Object.entries(args(part))
    if (entries.length !== 1) return undefined
    return str(entries[0][1])
  },
}

/**
 * How a connector's tool reads: the server's name and glyph lead, and the tool
 * it called is kept dim behind them, so "Excel" still says which call it was.
 */
function serverRenderer(server: string): ToolRenderer {
  return {
    icon: connectorIcon(server),
    label: server,
    getFileName: (part) => part.name,
  }
}

export function getToolRenderer(name: string, server?: string): ToolRenderer {
  if (server) return serverRenderer(server)
  return toolRenderers[name] ?? fallbackRenderer
}

function count(times: number, one: string, many: string): string {
  return `${times} ${times === 1 ? one : many}`
}

/** The phrase a run of same-named calls folds down to. */
function summaryPhrase(name: string, times: number): string {
  switch (name) {
    case "read_file":
      return `Read ${count(times, "file", "files")}`
    case "write_file":
      return `Wrote ${count(times, "file", "files")}`
    case "edit_file":
      return `Edited ${count(times, "file", "files")}`
    case "create_dir":
      return `Created ${count(times, "folder", "folders")}`
    case "list_dir":
      return `Listed ${count(times, "folder", "folders")}`
    case "run_command":
      return times === 1 ? "Ran command" : `Ran ${times} commands`
    case "glob":
      return times === 1 ? "Found files" : `Found files (${times})`
    case "grep":
      return times === 1 ? "Searched" : `Searched (${times})`
    default: {
      const label = getToolRenderer(name).label
      return times === 1 ? label : `${label} (${times})`
    }
  }
}

/**
 * The one line a run of tool calls folds down to: how many of each kind ran, in
 * the order they first ran, or how many failed.
 */
export function computeToolSummary(calls: TracePart[]): string {
  const failed = calls.filter(isCallFailed).length
  const done = calls.length - failed

  if (failed > 0 && done === 0) return `Failed (${failed})`
  if (failed > 0) return `${done} done, ${failed} failed`

  // Count per name, preserving first-use order, so the line reads in the order
  // things actually happened rather than alphabetically.
  const order: string[] = []
  const counts = new Map<string, number>()
  const servers = new Map<string, string>()
  for (const part of calls) {
    // A connector's calls fold by server, not by tool: "Excel (3)" says more
    // than the three separate tool names would, and it is the same name the
    // individual nodes carry.
    const key = part.server ? `server:${part.server}` : (part.name ?? "tool")
    if (!counts.has(key)) {
      order.push(key)
      if (part.server) servers.set(key, part.server)
    }
    counts.set(key, (counts.get(key) ?? 0) + 1)
  }

  const phrases = order.map((key) => {
    const times = counts.get(key) ?? 1
    const server = servers.get(key)
    if (server !== undefined) {
      return times === 1 ? server : `${server} (${times})`
    }
    return summaryPhrase(key, times)
  })

  // Cap the folded line so a long mixed run does not become a paragraph.
  const max = 3
  if (phrases.length > max) {
    return `${phrases.slice(0, max).join(", ")} +${phrases.length - max} more`
  }
  return phrases.join(", ")
}

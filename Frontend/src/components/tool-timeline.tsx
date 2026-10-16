import { useEffect, useRef, useState, type ReactNode } from "react"
import { cn } from "cn"

import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowDown01Icon, BrainIcon } from "@hugeicons/core-free-icons"

import { TimelineNode } from "@/components/timeline-node"
import {
  computeToolSummary,
  getToolRenderer,
  isCallFailed,
  isCallLive,
} from "@/lib/tool-registry"
import type { TracePart } from "@/types/chat"

/**
 * One run of work between two stretches of speech, folded away behind a single
 * line. The line says what the run did — "Read 2 files, Wrote 1 file" — and
 * shimmers while it is still going; opened, it is a timeline of the individual
 * calls and thoughts, each with its arguments and result a click away.
 *
 * A run that has finished folds itself back up shortly after the last call
 * lands, so a reply that ran six tools still reads as a sentence with one quiet
 * line of record above it. A reader who opens a run keeps it open.
 */
export function ToolTimeline({
  parts,
  completed,
  thinkingMs,
}: {
  /** The run's parts, in order: what the model thought and what it called. */
  parts: TracePart[]
  /** True once every call in the run has landed and no part is still arriving. */
  completed: boolean
  /** How long the model spent thinking, when the run holds a single thought. */
  thinkingMs?: number
}) {
  const [expanded, setExpanded] = useState(!completed)
  /** Which node's panel is open. `undefined` means the reader has not chosen. */
  const [detail, setDetail] = useState<string | null>()
  const userToggled = useRef(false)
  const wasCompleted = useRef(completed)

  useEffect(() => {
    // Only fold on the transition: a run that was already over when it first
    // rendered is a stored reply, and its reader's choices must stand.
    if (wasCompleted.current === completed) return
    wasCompleted.current = completed
    if (!completed || userToggled.current) return

    const timer = setTimeout(() => setExpanded(false), 600)
    return () => clearTimeout(timer)
  }, [completed])

  if (parts.length === 0) return null

  const calls = parts.filter((part) => part.kind === "tool")
  const failed = calls.some(isCallFailed)
  const summary =
    calls.length === 0
      ? completed
        ? "Thought"
        : "Thinking"
      : computeToolSummary(calls)

  // A failure opens itself: the reason is the first thing anyone wants, and it
  // stays open until the reader picks a panel of their own.
  const failedIndex = parts.findIndex(
    (part) => part.kind === "tool" && isCallFailed(part)
  )
  const autoOpen = failedIndex >= 0 ? nodeKey(parts[failedIndex], failedIndex) : null
  const openDetail = detail !== undefined ? detail : autoOpen

  return (
    <div className="w-full min-w-0">
      <button
        type="button"
        onClick={() => {
          userToggled.current = true
          setExpanded((open) => !open)
        }}
        aria-expanded={expanded}
        className={cn(
          "flex cursor-pointer items-center gap-1.5 rounded-md text-left text-[14px] leading-5 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-white/20",
          failed
            ? "text-red-400/80 hover:text-red-300"
            : "text-white/50 hover:text-white/70"
        )}
      >
        <span className={cn(!completed && !failed && "tl-shimmer")}>
          {summary}
        </span>
        <HugeiconsIcon
          icon={ArrowDown01Icon}
          strokeWidth={2}
          className={cn(
            "size-3 transition-transform duration-200",
            !expanded && "-rotate-90"
          )}
        />
      </button>

      <div
        className={cn(
          "grid transition-all duration-200 ease-in-out",
          expanded
            ? "grid-rows-[1fr] opacity-100"
            : "grid-rows-[0fr] opacity-0"
        )}
      >
        <div className="no-scrollbar overflow-hidden pt-1">
          {parts.map((part, index) => {
            const key = nodeKey(part, index)
            const isLast = index === parts.length - 1
            // The line runs down to whatever is happening now, and closes off
            // once the terminal node appears below it.
            const stopHere = isLast && !completed

            if (part.kind === "reasoning") {
              return (
                <TimelineNode
                  key={key}
                  icon={BrainIcon}
                  iconType={isLast && !completed ? "loading" : "default"}
                  label={completed ? "Thought" : "Thinking"}
                  durationMs={thinkingMs}
                  isLast={stopHere}
                  showLine={!stopHere}
                  detail={thoughtPanel(part.text ?? "")}
                  isDetailExpanded={openDetail === key}
                  onToggleDetail={() =>
                    setDetail(openDetail === key ? null : key)
                  }
                />
              )
            }

            const renderer = getToolRenderer(part.name ?? "")
            return (
              <TimelineNode
                key={key}
                icon={renderer.icon}
                iconType={
                  isCallFailed(part)
                    ? "error"
                    : isCallLive(part)
                      ? "loading"
                      : "default"
                }
                label={renderer.getLabel?.(part) ?? renderer.label}
                fileName={renderer.getFileName?.(part)}
                durationMs={part.durationMs}
                isLast={stopHere}
                showLine={!stopHere}
                detail={callPanel(part)}
                isDetailExpanded={openDetail === key}
                onToggleDetail={() => setDetail(openDetail === key ? null : key)}
              />
            )
          })}

          {completed && (
            <TimelineNode
              iconType={failed ? "error" : "check"}
              label={failed ? "Failed" : "Done"}
              isLast
              showLine={false}
            />
          )}
        </div>
      </div>
    </div>
  )
}

function nodeKey(part: TracePart, index: number): string {
  return part.callId ?? `part-${index}`
}

/** A call: the arguments it was given, and what it returned. */
function callPanel(part: TracePart): ReactNode {
  const failed = isCallFailed(part)
  const input = prettyJSON(part.input)
  const output = (part.error ?? part.output ?? "").trim()

  return (
    <div className="flex flex-col gap-1.5 pt-1 pb-0.5">
      {input !== "" && <Panel label="Input">{input}</Panel>}
      <Panel label={failed ? "Error" : "Output"} tone={failed ? "error" : "plain"}>
        {output !== "" ? output : failed ? "(no message)" : "(no output)"}
      </Panel>
    </div>
  )
}

/** A thought is prose, so it gets no monospace treatment. */
function thoughtPanel(text: string): ReactNode {
  return (
    <p className="no-scrollbar max-h-48 overflow-y-auto pt-1 pb-0.5 text-[12.5px] leading-relaxed whitespace-pre-wrap text-white/50">
      {text}
    </p>
  )
}

/** One labelled monospace block inside a node's panel. */
function Panel({
  label,
  tone = "plain",
  children,
}: {
  label: string
  tone?: "plain" | "error"
  children: string
}) {
  return (
    <div className="min-w-0">
      <p
        className={cn(
          "mb-1 font-mono text-[10px]",
          tone === "error" ? "text-red-400/70" : "text-white/40"
        )}
      >
        {label}
      </p>
      <pre
        className={cn(
          "no-scrollbar max-h-72 overflow-y-auto rounded-lg border p-3 font-mono text-[11px] leading-4 break-all whitespace-pre-wrap",
          tone === "error"
            ? "border-red-500/10 bg-red-500/5 text-red-300/70"
            : "border-white/10 bg-white/5 text-white/70"
        )}
      >
        {children}
      </pre>
    </div>
  )
}

/** Arguments as indented JSON, falling back to the raw payload. */
function prettyJSON(raw?: string): string {
  if (!raw || raw.trim() === "") return ""
  try {
    return JSON.stringify(JSON.parse(raw), null, 2) ?? raw
  } catch {
    return raw
  }
}

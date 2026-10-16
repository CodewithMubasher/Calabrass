import { cn } from "cn"

import { Markdown } from "@/components/markdown"
import { ToolTimeline } from "@/components/tool-timeline"
import { isCallLive } from "@/lib/tool-registry"
import type { TracePart, TurnTrace } from "@/types/chat"

/**
 * The record of an assistant turn, rendered inline in the reply: what the model
 * said, and the runs of work it did between those stretches of speech. Each run
 * folds to one line — "Read 2 files, Wrote 1 file" — and opens onto a timeline
 * of the calls and thoughts inside it.
 *
 * A finished turn renders from its stored `trace`. An in-flight one renders from
 * the same trace plus the deltas that have not reached a snapshot yet, so the
 * two look alike and the reply does not reflow when the turn is committed.
 */
export function AgentTrace({
  trace,
  content,
  reasoning,
  thinkingMs,
  streamText = "",
  streamReasoning = "",
  running = false,
}: {
  /** The ordered record of the turn, when the backend has sent one. */
  trace?: TurnTrace
  /** The stored reply, for a turn that has no trace. */
  content?: string
  /** The model's thinking, for a turn that has no trace. */
  reasoning?: string
  /** How long the model thought, in milliseconds. */
  thinkingMs?: number
  /** Text that has streamed in since the last trace snapshot. */
  streamText?: string
  /** Thinking that has streamed in since the last trace snapshot. */
  streamReasoning?: string
  /** True while the turn is still being generated. */
  running?: boolean
}) {
  const blocks = blocksFor({
    trace,
    content,
    reasoning,
    streamText,
    streamReasoning,
    running,
  })
  if (blocks.length === 0) return null

  // A turn with one stretch of thinking shows how long it took; with several,
  // there is no per-step timing to report, so the nodes stay plain.
  const thoughts = blocks.reduce(
    (total, block) =>
      total +
      (block.kind === "activity"
        ? block.items.filter((item) => item.part.kind === "reasoning").length
        : 0),
    0
  )

  return (
    <div className="flex w-full min-w-0 flex-col gap-2.5">
      {blocks.map((block, index) => {
        // Blocks are keyed by position: a live tail is replaced in place by the
        // snapshot that captures it, so the element under the reader's cursor
        // must survive the swap.
        if (block.kind === "text") {
          return (
            <div key={index} className="w-full min-w-0">
              <Markdown className={cn(block.live && running && "streaming-caret")}>
                {block.text}
              </Markdown>
            </div>
          )
        }
        return (
          <ToolTimeline
            key={index}
            parts={block.items.map((item) => item.part)}
            completed={block.items.every(settled)}
            thinkingMs={thoughts === 1 ? thinkingMs : undefined}
          />
        )
      })}
    </div>
  )
}

// --- the reply's blocks ------------------------------------------------------

/** One entry in the reply: a stretch of speech, or a run of work. */
type Block =
  | { kind: "text"; text: string; live: boolean }
  | { kind: "activity"; items: ActivityItem[] }

/** One step inside a run of work: a thought of the model's, or a tool call. */
interface ActivityItem {
  part: TracePart
  /** True for what has streamed in since the trace last captured anything. */
  live: boolean
}

interface BlocksInput {
  trace?: TurnTrace
  content?: string
  reasoning?: string
  streamText: string
  streamReasoning: string
  running: boolean
}

/**
 * Flattens a turn into the blocks the reply renders.
 *
 * The trace is the source of truth. Where it has not caught up — a step's text
 * and thinking arrive as deltas, and a snapshot only comes at the step's end —
 * the tail of each stream is appended, in the order a step produces them. Once
 * the snapshot lands, the tail is empty, so the block list is unchanged and
 * nothing moves.
 */
function blocksFor(input: BlocksInput): Block[] {
  const steps = input.trace?.steps ?? []
  const blocks: Block[] = []
  const run: ActivityItem[] = []

  const flush = () => {
    if (run.length === 0) return
    blocks.push({ kind: "activity", items: [...run] })
    run.length = 0
  }

  /** Speech ends the current run: the model is addressing the reader. */
  const say = (text: string, live: boolean) => {
    if (text.trim() === "") return
    flush()
    blocks.push({ kind: "text", text, live })
  }

  const act = (part: TracePart, live: boolean) => {
    if (part.kind === "reasoning" && (part.text ?? "").trim() === "") return
    run.push({ part, live })
  }

  if (steps.length === 0) {
    // No trace at all: either a stored reply from before the harness, or a turn
    // still on its first step, whose snapshot only comes at the step's end.
    // Whatever the stream holds beyond the stored text is the live part.
    const storedThought = input.reasoning ?? ""
    const storedText = input.content ?? ""
    const liveThought = tail(input.streamReasoning, storedThought)
    const liveText = tail(input.streamText, storedText)

    act(
      { kind: "reasoning", text: storedThought + liveThought },
      input.running || liveThought !== ""
    )
    say(storedText + liveText, input.running || liveText !== "")
    flush()
    return blocks
  }

  let said = ""
  let thought = ""

  for (const step of steps) {
    for (const part of step.parts) {
      if (part.kind === "text") {
        said += part.text ?? ""
        say(part.text ?? "", false)
        continue
      }
      if (part.kind === "reasoning") {
        thought += part.text ?? ""
      }
      act(part, false)
    }
  }

  // A provider can report the step but stream no deltas, leaving the trace
  // without the answer. The stored reply is the fallback for that.
  const answer = (input.content ?? "").trim()
  if (answer !== "" && !said.includes(answer)) say(input.content ?? "", false)

  act({ kind: "reasoning", text: tail(input.streamReasoning, thought) }, true)
  say(tail(input.streamText, said), true)
  flush()

  return blocks
}

/** Whether a step of a run has nothing left to happen. */
function settled(item: ActivityItem): boolean {
  if (item.live) return false
  return item.part.kind === "reasoning" || !isCallLive(item.part)
}

/**
 * The part of a stream that has arrived since the trace last captured any of it.
 * If the two have drifted apart for any reason, the whole stream is shown rather
 * than losing text.
 */
function tail(streamed: string, captured: string): string {
  if (streamed === "") return ""
  if (captured === "" || !streamed.startsWith(captured)) return streamed
  return streamed.slice(captured.length)
}

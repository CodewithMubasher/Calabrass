import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { AiBrain01Icon, Loading03Icon } from "@hugeicons/core-free-icons"

import {
  Trace,
  TraceCollapsible,
  TraceCollapsibleContent,
  TraceCollapsibleTrigger,
  TraceDescription,
  TraceIcon,
  TraceLine,
  TraceTitle,
} from "@/components/ui/trace"

interface ReasoningTraceProps {
  /** The model's thinking text. */
  text: string
  /** True while the model is still thinking. */
  running?: boolean
  /** How long the model thought, in milliseconds. */
  thinkingMs?: number
}

/**
 * The model's thinking, as a single Trace row.
 *
 * Follows the shadcn Reasoning pattern: it reads "Thinking" and opens while the
 * model thinks, then reads "Thought" with a duration and closes once it is
 * done. If the reader opens or closes it by hand, their choice wins.
 */
export function ReasoningTrace({
  text,
  running = false,
  thinkingMs,
}: ReasoningTraceProps) {
  const [open, setOpen] = useState<boolean>()

  const thinking = text.trim()
  if (thinking === "") return null

  const seconds =
    !running && thinkingMs && thinkingMs > 0
      ? Math.max(1, Math.round(thinkingMs / 1000))
      : undefined

  return (
    <Trace>
      <TraceCollapsible open={open ?? running} onOpenChange={setOpen}>
        <TraceCollapsibleTrigger>
          <TraceLine status={running ? "running" : undefined}>
            <TraceIcon>
              {running ? (
                <HugeiconsIcon
                  icon={Loading03Icon}
                  strokeWidth={2}
                  className="animate-spin"
                />
              ) : (
                <HugeiconsIcon icon={AiBrain01Icon} strokeWidth={2} />
              )}
            </TraceIcon>
            <TraceTitle>{running ? "Thinking" : "Thought"}</TraceTitle>
            {seconds !== undefined && (
              <TraceDescription>for {seconds}s</TraceDescription>
            )}
          </TraceLine>
        </TraceCollapsibleTrigger>
        <TraceCollapsibleContent className="whitespace-pre-wrap">
          {thinking}
        </TraceCollapsibleContent>
      </TraceCollapsible>
    </Trace>
  )
}

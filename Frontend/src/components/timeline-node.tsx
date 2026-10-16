import type { ReactNode } from "react"
import { cn } from "cn"

import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react"
import {
  Alert02Icon,
  ArrowRight01Icon,
  CheckmarkCircle02Icon,
  Loading03Icon,
  Wrench01Icon,
} from "@hugeicons/core-free-icons"

import { Badge } from "@/components/ui/badge"

/**
 * One step of a tool run: an icon on a connector line, the call in a pill, and
 * an optional panel below it. The line stops at the last node so a finished run
 * reads as a closed bracket rather than trailing off.
 */
export function TimelineNode({
  icon,
  iconType = "default",
  label,
  fileName,
  durationMs,
  detail,
  isDetailExpanded = false,
  onToggleDetail,
  isLast = false,
  showLine = true,
}: {
  /** The node's own icon; loading, error and check replace it. */
  icon?: IconSvgElement
  iconType?: "default" | "loading" | "error" | "check"
  label: string
  /** A dim snippet after the label: a file, a pattern, a command. */
  fileName?: string
  /** How long the call took, shown dimmed at the end of the pill. */
  durationMs?: number
  detail?: ReactNode
  isDetailExpanded?: boolean
  onToggleDetail?: () => void
  isLast?: boolean
  showLine?: boolean
}) {
  const failed = iconType === "error"
  const hasDetail = detail !== undefined && onToggleDetail !== undefined
  const duration = formatMs(durationMs)

  return (
    <div className="flex">
      <div className="-mb-2 mt-2 mr-2 flex w-4 flex-col items-center">
        <div className="flex-shrink-0">
          <NodeIcon icon={icon} type={iconType} />
        </div>
        <div
          className={cn(
            "min-h-5 w-px flex-1",
            showLine && !isLast
              ? failed
                ? "bg-red-500/20"
                : "bg-white/20"
              : "bg-transparent"
          )}
        />
      </div>

      <div className="mt-1.5 mb-2 min-w-0 flex-1">
        <button
          type="button"
          onClick={onToggleDetail}
          disabled={!hasDetail}
          aria-expanded={hasDetail ? isDetailExpanded : undefined}
          className={cn(
            "flex items-center gap-1.5 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-white/20",
            hasDetail ? "cursor-pointer hover:opacity-80" : "cursor-default"
          )}
        >
          <Badge
            variant="secondary"
            className={cn(
              "h-auto gap-1.5 px-2 py-0.5 text-[11px] leading-4",
              failed
                ? "border-red-500/20 bg-red-500/10 text-red-400"
                : "border-white/10 bg-white/10 text-white/80"
            )}
          >
            <span className="font-medium">{label}</span>
            {fileName !== undefined && (
              <span className="text-[10px] text-white/50">{fileName}</span>
            )}
            {duration !== "" && (
              <span className="text-[10px] tabular-nums text-white/35">
                {duration}
              </span>
            )}
          </Badge>
          {hasDetail && (
            <HugeiconsIcon
              icon={ArrowRight01Icon}
              strokeWidth={2}
              className={cn(
                "size-3 text-white/30 transition-transform duration-150",
                isDetailExpanded && "rotate-90"
              )}
            />
          )}
        </button>

        {hasDetail && (
          <div
            className={cn(
              "grid transition-all duration-200 ease-in-out",
              isDetailExpanded
                ? "mt-1 grid-rows-[1fr] opacity-100"
                : "grid-rows-[0fr] opacity-0"
            )}
          >
            <div className="no-scrollbar overflow-hidden">{detail}</div>
          </div>
        )}
      </div>
    </div>
  )
}

function NodeIcon({
  icon,
  type,
}: {
  icon?: IconSvgElement
  type: "default" | "loading" | "error" | "check"
}) {
  if (type === "check") {
    return (
      <HugeiconsIcon
        icon={CheckmarkCircle02Icon}
        strokeWidth={2}
        className="size-4 text-white/60"
      />
    )
  }
  if (type === "error") {
    return (
      <HugeiconsIcon
        icon={Alert02Icon}
        strokeWidth={2}
        className="size-4 text-red-400"
      />
    )
  }
  if (type === "loading") {
    return (
      <HugeiconsIcon
        icon={Loading03Icon}
        strokeWidth={2}
        className="size-4 animate-spin text-white/60 motion-reduce:animate-none"
      />
    )
  }
  return (
    <HugeiconsIcon
      icon={icon ?? Wrench01Icon}
      strokeWidth={2}
      className="size-4 text-white/60"
    />
  )
}

/** A node's runtime, the way a pill should read it. Sub-millisecond calls omit it. */
function formatMs(ms?: number): string {
  if (!ms || ms <= 0) return ""
  return ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`
}

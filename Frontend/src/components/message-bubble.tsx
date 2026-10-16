import { useState } from "react"
import { cn } from "cn"
import "./message-bubble.css"

import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react"
import {
  CheckmarkCircle02Icon,
  Copy01Icon,
  Delete02Icon,
  EllipsisIcon,
  RefreshIcon,
  ThumbsUpIcon,
} from "@hugeicons/core-free-icons"

import { AgentTrace } from "@/components/agent-trace"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Message,
  MessageAvatar,
  MessageContent,
} from "@/components/ui/message"
import type { ChatMessage } from "@/types/chat"

/**
 * The logo is not a circle, so the avatar must not clip it.
 * When `animated`, it spins and pulses for the duration of the stream.
 */
function AssistantAvatar({ animated = false }: { animated?: boolean }) {
  return (
    <MessageAvatar className="size-8 self-start overflow-visible rounded-none bg-transparent">
      <img
        src="/vite.svg"
        alt="Calabrass"
        className={cn("block size-6", animated && "streaming-logo")}
      />
    </MessageAvatar>
  )
}

/** How fast the finished reply was written, or null when it is unknown. */
function tokensPerSecond(message: ChatMessage): number | null {
  const tokens = message.outputTokens ?? 0
  const ms = message.durationMs ?? 0
  if (tokens <= 0 || ms <= 0) return null
  return tokens / (ms / 1000)
}

function ActionButton({
  icon,
  label,
  onClick,
  active = false,
}: {
  icon: IconSvgElement
  label: string
  onClick: () => void
  active?: boolean
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      title={label}
      className={cn(
        "flex size-7 cursor-pointer items-center justify-center rounded-lg text-white/40 transition-colors hover:bg-white/5 hover:text-white/80",
        active && "text-emerald-400 hover:text-emerald-300"
      )}
    >
      <HugeiconsIcon icon={icon} strokeWidth={2} className="size-4" />
    </button>
  )
}

/**
 * The row under a finished reply: rate it, copy it, ask for another, or open
 * the less common actions — with the generation speed on the far right.
 */
function MessageActions({
  message,
  onRetry,
  onDelete,
}: {
  message: ChatMessage
  onRetry?: () => void
  onDelete?: () => void
}) {
  const [liked, setLiked] = useState(false)
  const [copied, setCopied] = useState(false)
  const [copiedReasoning, setCopiedReasoning] = useState(false)

  async function copy(text: string, done: (value: boolean) => void) {
    try {
      await navigator.clipboard.writeText(text)
      done(true)
      window.setTimeout(() => done(false), 1500)
    } catch {
      // Clipboard access can be refused; leave the button as it was.
    }
  }

  const rate = tokensPerSecond(message)
  const reasoning = message.reasoning ?? ""

  return (
    <div className="mt-1.5 flex w-full items-center gap-0.5">
      <ActionButton
        icon={ThumbsUpIcon}
        label={liked ? "Remove like" : "Like this reply"}
        active={liked}
        onClick={() => setLiked((value) => !value)}
      />
      <ActionButton
        icon={copied ? CheckmarkCircle02Icon : Copy01Icon}
        label={copied ? "Copied" : "Copy this reply"}
        active={copied}
        onClick={() => void copy(message.content, setCopied)}
      />
      {onRetry ? (
        <ActionButton icon={RefreshIcon} label="Retry this reply" onClick={onRetry} />
      ) : null}

      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <button
              type="button"
              aria-label="More actions"
              title="More actions"
              className="flex size-7 cursor-pointer items-center justify-center rounded-lg text-white/40 transition-colors hover:bg-white/5 hover:text-white/80"
            >
              <HugeiconsIcon icon={EllipsisIcon} strokeWidth={2} className="size-4" />
            </button>
          }
        />
        <DropdownMenuContent className="w-44 rounded-xl" side="top" align="start">
          {reasoning !== "" ? (
            <DropdownMenuItem
              onClick={() => void copy(reasoning, setCopiedReasoning)}
            >
              <HugeiconsIcon icon={copiedReasoning ? CheckmarkCircle02Icon : Copy01Icon} strokeWidth={2} />
              <span>{copiedReasoning ? "Copied thinking" : "Copy thinking"}</span>
            </DropdownMenuItem>
          ) : null}
          {onDelete ? (
            <DropdownMenuItem variant="destructive" onClick={onDelete}>
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
              <span>Delete reply</span>
            </DropdownMenuItem>
          ) : null}
        </DropdownMenuContent>
      </DropdownMenu>

      {rate !== null ? (
        <span className="ml-auto pr-1 text-[11px] tabular-nums text-white/35">
          {rate.toFixed(1)} tok/s
        </span>
      ) : null}
    </div>
  )
}

export function MessageBubble({
  message,
  streaming = false,
  onRetry,
  onDelete,
}: {
  message: ChatMessage
  /** True while this reply is still arriving; it has no actions yet. */
  streaming?: boolean
  /** Only the newest reply can be regenerated. */
  onRetry?: () => void
  onDelete?: () => void
}) {
  const isUser = message.role === "user"
  const done =
    !isUser && !streaming && !message.pending && message.content !== ""

  return (
    <Message align={isUser ? "end" : "start"}>
      {!isUser && <AssistantAvatar animated={streaming} />}
      <MessageContent className={cn(isUser ? "items-end" : "items-start pt-1")}>
        {isUser ? (
          message.content !== "" && (
            <div className="w-fit max-w-[80%] min-w-0 rounded-2xl bg-primary px-4 py-2 text-sm whitespace-pre-wrap text-white">
              {message.content}
            </div>
          )
        ) : (
          // One component renders both the in-flight reply and the stored one,
          // so the turn does not reflow when it is committed: the trace is the
          // record either way, and only what has not reached a trace snapshot
          // yet is taken from the stream.
          <AgentTrace
            trace={message.trace}
            content={streaming ? undefined : message.content}
            reasoning={streaming ? undefined : message.reasoning}
            thinkingMs={message.thinkingMs}
            streamText={streaming ? message.content : undefined}
            streamReasoning={streaming ? message.reasoning : undefined}
            running={streaming}
          />
        )}
        {done && (
          <MessageActions
            message={message}
            onRetry={onRetry}
            onDelete={onDelete}
          />
        )}
      </MessageContent>
    </Message>
  )
}

export default MessageBubble

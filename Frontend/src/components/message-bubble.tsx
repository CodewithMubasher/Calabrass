import { HugeiconsIcon } from "@hugeicons/react"
import { Loading03Icon } from "@hugeicons/core-free-icons"
import { cn } from "cn"

import {
  Message,
  MessageAvatar,
  MessageContent,
} from "@/components/ui/message"
import type { ChatMessage } from "@/types/chat"

/**
 * The logo is not a circle, so the avatar must not clip it.
 */
function AssistantAvatar() {
  return (
    <MessageAvatar className="size-8 self-start overflow-visible rounded-none bg-transparent">
      <img src="/vite.svg" alt="Calabrass" className="size-7" />
    </MessageAvatar>
  )
}

export function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user"

  return (
    <Message align={isUser ? "end" : "start"}>
      {!isUser && <AssistantAvatar />}
      <MessageContent className={cn(isUser ? "items-end" : "items-start")}>
        <div
          className={cn(
            "w-fit max-w-[80%] rounded-2xl px-4 py-2 text-sm leading-relaxed whitespace-pre-wrap",
            isUser ? "bg-primary text-white" : "bg-white/5 text-white/90"
          )}
        >
          {message.content}
        </div>
      </MessageContent>
    </Message>
  )
}

/** The in-flight reply: revealed words plus a live tokens/sec readout. */
export function StreamingMessage({
  text,
  rate,
}: {
  text: string
  rate: number
}) {
  return (
    <Message align="start">
      <AssistantAvatar />
      <MessageContent className="items-start">
        <div className="w-fit max-w-[80%] rounded-2xl bg-white/5 px-4 py-2 text-sm leading-relaxed whitespace-pre-wrap text-white/90">
          {text}
          <span className="ml-0.5 inline-block h-3.5 w-[2px] translate-y-[2px] animate-pulse rounded-full bg-white/70" />
        </div>
        <div className="flex items-center gap-1.5 px-1 text-xs text-white/60">
          <HugeiconsIcon
            icon={Loading03Icon}
            strokeWidth={2}
            className="size-3 animate-spin"
          />
          <span>{rate > 0 ? `${rate} tokens/s` : "Generating"}</span>
        </div>
      </MessageContent>
    </Message>
  )
}

export default MessageBubble

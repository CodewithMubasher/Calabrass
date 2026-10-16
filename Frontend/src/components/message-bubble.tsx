import { cn } from "cn"
import "./message-bubble.css"

import { ReasoningTrace } from "@/components/reasoning-trace"
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

export function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user"
  const reasoning = message.reasoning ?? ""

  return (
    <Message align={isUser ? "end" : "start"}>
      {!isUser && <AssistantAvatar />}
      <MessageContent className={cn(isUser ? "items-end" : "items-start")}>
        {!isUser && reasoning !== "" && (
          <ReasoningTrace text={reasoning} thinkingMs={message.thinkingMs} />
        )}
        {message.content !== "" && (
          <div
            className={cn(
              "w-fit max-w-[80%] rounded-2xl px-4 py-2 text-sm leading-relaxed whitespace-pre-wrap",
              isUser ? "bg-primary text-white" : "bg-white/5 text-white/90"
            )}
          >
            {message.content}
          </div>
        )}
      </MessageContent>
    </Message>
  )
}

/**
 * The in-flight reply: the model's thinking as a Trace row, then the words
 * revealed so far plus a blinking caret.
 */
export function StreamingMessage({
  text,
  reasoning = "",
}: {
  text: string
  reasoning?: string
}) {
  return (
    <Message align="start">
      <AssistantAvatar animated />
      <MessageContent className="items-start">
        {reasoning.trim() !== "" && <ReasoningTrace text={reasoning} running />}
        {text !== "" && (
          <div className="w-fit max-w-[80%] rounded-2xl bg-white/5 px-4 py-2 text-sm leading-relaxed whitespace-pre-wrap text-white/90">
            {text}
            <span className="ml-0.5 inline-block h-3.5 w-[2px] translate-y-[2px] animate-pulse rounded-full bg-white/70" />
          </div>
        )}
      </MessageContent>
    </Message>
  )
}

export default MessageBubble

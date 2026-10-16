import { cn } from "cn"
import "./message-bubble.css"

import { Markdown } from "@/components/markdown"
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
      <MessageContent className={cn(isUser ? "items-end" : "items-start pt-1")}>
        {!isUser && reasoning !== "" && (
          <ReasoningTrace text={reasoning} thinkingMs={message.thinkingMs} />
        )}
        {message.content !== "" &&
          (isUser ? (
            <div className="w-fit max-w-[80%] min-w-0 rounded-2xl bg-primary px-4 py-2 text-sm whitespace-pre-wrap text-white">
              {message.content}
            </div>
          ) : (
            // Replies are Markdown and sit directly in the content column, at
            // the same width as the thinking rows above them.
            <div className="w-full min-w-0">
              <Markdown>{message.content}</Markdown>
            </div>
          ))}
      </MessageContent>
    </Message>
  )
}

/**
 * The in-flight reply: the model's thinking as a Trace row, then the answer as
 * it is written, with a caret pinned to the last rendered block.
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
      <MessageContent className="items-start pt-1">
        {reasoning.trim() !== "" && <ReasoningTrace text={reasoning} running />}
        {text !== "" && (
          <div className="w-full min-w-0">
            <Markdown className="streaming-caret">{text}</Markdown>
          </div>
        )}
      </MessageContent>
    </Message>
  )
}

export default MessageBubble

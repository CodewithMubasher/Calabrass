import { cn } from "cn"

import {
  Message,
  MessageAvatar,
  MessageContent,
} from "@/components/ui/message"
import type { ChatMessage } from "@/types/chat"

export function MessageBubble({ message }: { message: ChatMessage }) {
  const isUser = message.role === "user"

  return (
    <Message align={isUser ? "end" : "start"}>
      {!isUser && (
        <MessageAvatar className="size-8 self-start bg-transparent">
          <img src="/vite.svg" alt="Calabrass" className="size-7" />
        </MessageAvatar>
      )}
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

export default MessageBubble

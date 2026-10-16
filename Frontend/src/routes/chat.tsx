import { createRoute } from "@tanstack/react-router"
import { useEffect, useRef } from "react"

import { MessageBubble, StreamingMessage } from "@/components/message-bubble"
import { PromptInput } from "@/components/prompt-input"
import { MessageGroup } from "@/components/ui/message"
import {
  ensureChatLoaded,
  sendMessage,
  useChat,
  useChatError,
  useChatMessages,
  useHydrated,
  useIsStreaming,
  useStreamingText,
} from "@/lib/chat-store"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/chat/$chatId",
  component: ChatPage,
})

function ChatPage() {
  const { chatId } = Route.useParams()
  const hydrated = useHydrated()
  const chat = useChat(chatId)
  const messages = useChatMessages(chatId)
  const streaming = useIsStreaming(chatId)
  const streamingText = useStreamingText(chatId)
  const error = useChatError(chatId)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    void ensureChatLoaded(chatId)
  }, [chatId])

  const messageCount = messages?.length ?? 0
  const streamingLength = streamingText.length

  // Keep the newest message in view while the reply streams in.
  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [messageCount, streamingLength])

  if (!hydrated) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-white/50">
        Loading…
      </div>
    )
  }

  if (!chat) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-white/50">
        Chat not found.
      </div>
    )
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div
        ref={scrollRef}
        className="scrollbar-thin min-h-0 flex-1 overflow-y-auto"
      >
        <MessageGroup className="mx-auto w-full max-w-[780px] gap-4 px-4 py-6">
          {(messages ?? []).map((message) => (
            <MessageBubble key={message.id} message={message} />
          ))}
          {streaming && streamingText !== "" && (
            <StreamingMessage text={streamingText} />
          )}
        </MessageGroup>
      </div>
      {error ? (
        <p className="mx-auto w-full max-w-[780px] shrink-0 px-6 pb-1 text-sm text-red-400">
          {error}
        </p>
      ) : null}
      <div className="shrink-0">
        <PromptInput
          isInChat
          disabled={streaming}
          onSend={(value) => void sendMessage(chat.id, value)}
        />
      </div>
    </div>
  )
}

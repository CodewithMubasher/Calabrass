import { createRoute } from "@tanstack/react-router"
import { useEffect, useRef } from "react"

import { MessageBubble } from "@/components/message-bubble"
import { PromptInput } from "@/components/prompt-input"
import { MessageGroup } from "@/components/ui/message"
import {
  deleteReply,
  ensureChatLoaded,
  retryReply,
  sendMessage,
  updateChatMode,
  useChat,
  useChatError,
  useChatMessages,
  useHydrated,
  useIsStreaming,
  useStreamingMessage,
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
  const inFlight = useStreamingMessage(chatId)
  const error = useChatError(chatId)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    void ensureChatLoaded(chatId)
  }, [chatId])

  const messageCount = messages?.length ?? 0

  // Anything that grows the conversation: a message, a token, a thought, or a
  // step's worth of tool calls.
  const growth = [
    messageCount,
    inFlight?.content.length ?? 0,
    inFlight?.reasoning?.length ?? 0,
    (inFlight?.trace?.steps ?? []).reduce(
      (parts, step) => parts + step.parts.length,
      0
    ),
  ].join(":")

  // Keep the newest message in view while the reply streams in.
  useEffect(() => {
    const el = scrollRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [growth])

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
          {(messages ?? []).map((message, index) => {
            const list = messages ?? []
            const newestReply =
              message.role === "assistant" &&
              index === list.length - 1 &&
              !streaming

            return (
              <MessageBubble
                key={message.id}
                message={message}
                onRetry={
                  newestReply
                    ? () => void retryReply(chatId, message.id)
                    : undefined
                }
                onDelete={
                  message.role === "assistant"
                    ? () => void deleteReply(chatId, message.id)
                    : undefined
                }
              />
            )
          })}
          {inFlight && (
            <MessageBubble key={inFlight.id} message={inFlight} streaming />
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
          mode={chat?.mode ?? "build"}
          onModeChange={(newMode) => void updateChatMode(chatId, newMode)}
          onSend={(value) => void sendMessage(chat.id, value)}
        />
      </div>
    </div>
  )
}

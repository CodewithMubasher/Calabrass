import { useEffect, useRef } from "react"

import { MessageBubble } from "@/components/message-bubble"
import { PromptInput } from "@/components/prompt-input"
import { QuestionnairePanel } from "@/components/questionnaire-panel"
import { MessageGroup } from "@/components/ui/message"
import {
  deleteReply,
  ensureChatLoaded,
  retryReply,
  sendMessage,
  stopStream,
  useChat,
  useChatMessages,
  useHydrated,
  useIsStreaming,
  useStreamingMessage,
} from "@/lib/chat-store"
import { findPendingQuestionnaire, formatAnswers } from "@/lib/questionnaire"
import type { ChatMode } from "@/types/chat"

/**
 * One conversation, shown the same way whether it runs as an ordinary chat or
 * as a crew run.
 *
 * The thread does not care which address it is under: the messages, the trace
 * and the composer are the same in both, and only the mode differs. What the
 * mode changes is handed back to the caller, which is what puts the
 * conversation under the other address when the switch is moved.
 */
export function ChatThread({
  chatId,
  mode,
  onModeChange,
}: {
  chatId: string
  /** The mode the composer is in. */
  mode: ChatMode
  /** Called when the composer switch is moved. */
  onModeChange: (mode: ChatMode) => void
}) {
  const hydrated = useHydrated()
  const chat = useChat(chatId)
  const messages = useChatMessages(chatId)
  const streaming = useIsStreaming(chatId)
  const inFlight = useStreamingMessage(chatId)
  const scrollRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    void ensureChatLoaded(chatId)
  }, [chatId])

  const messageCount = messages?.length ?? 0

  // The questions the model asked and has not been answered yet. They are shown
  // above the composer, because the answers go back as the next message.
  const questions = findPendingQuestionnaire(messages ?? [], inFlight)

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
    const scroll = scrollRef.current
    if (scroll) scroll.scrollTop = scroll.scrollHeight
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
      {questions ? (
        <div className="shrink-0">
          <QuestionnairePanel
            questions={questions}
            disabled={streaming}
            onSubmit={(answers) =>
              void sendMessage(chatId, formatAnswers(questions, answers))
            }
          />
        </div>
      ) : null}
      <div className="shrink-0">
        <PromptInput
          isInChat
          attached={questions !== null}
          disabled={streaming}
          isStreaming={streaming}
          onStop={() => void stopStream(chatId)}
          mode={mode}
          onModeChange={onModeChange}
          onSend={(value) => void sendMessage(chat.id, value)}
        />
      </div>
    </div>
  )
}

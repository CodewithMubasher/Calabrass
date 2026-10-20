import { createRoute, useNavigate } from "@tanstack/react-router"
import { useState, useRef } from "react"

import { PromptInput, type PromptInputRef } from "@/components/prompt-input"
import { createChat, sendMessage } from "@/lib/chat-store"
import { notifyError } from "@/lib/toast"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  const navigate = useNavigate()
  const [sending, setSending] = useState(false)
  const promptInputRef = useRef<PromptInputRef>(null)

  const handleSend = (value: string) => {
    if (sending) return
    setSending(true)

    void (async () => {
      try {
        const mode = promptInputRef.current?.getMode?.() ?? "build"
        const chat = await createChat(undefined, undefined, mode)
        await navigate({ to: "/chat/$chatId", params: { chatId: chat.id } })
        // Fire and forget: the reply streams into the store and the chat page
        // renders it from there.
        void sendMessage(chat.id, value)
      } catch (err) {
        notifyError(
          err instanceof Error ? err.message : "Could not start a chat.",
          "Could not start the chat"
        )
      } finally {
        setSending(false)
      }
    })()
  }

  return (
    <div className="relative flex h-full flex-col">
      <PromptInput
        ref={promptInputRef}
        onSend={handleSend}
        disabled={sending}
      />
    </div>
  )
}

import { createRoute, useNavigate } from "@tanstack/react-router"
import { useState, useRef } from "react"

import { PromptInput, type PromptInputRef } from "@/components/prompt-input"
import { createChat, sendMessage } from "@/lib/chat-store"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  const navigate = useNavigate()
  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const promptInputRef = useRef<PromptInputRef>(null)

  const handleSend = (value: string) => {
    if (sending) return
    setSending(true)
    setError(null)

    void (async () => {
      try {
        const mode = promptInputRef.current?.getMode?.() ?? "build"
        const chat = await createChat(undefined, undefined, mode)
        await navigate({ to: "/chat/$chatId", params: { chatId: chat.id } })
        // Fire and forget: the reply streams into the store and the chat page
        // renders it from there.
        void sendMessage(chat.id, value)
      } catch (err) {
        setError(err instanceof Error ? err.message : "Could not start a chat.")
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
      {error ? (
        // On a phone the composer is at the foot of the screen, so the message
        // goes to the top of it rather than over the input.
        <p className="absolute inset-x-0 bottom-8 px-4 text-center text-sm text-red-400 max-md:bottom-auto max-md:top-4">
          {error}
        </p>
      ) : null}
    </div>
  )
}

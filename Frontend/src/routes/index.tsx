import { createRoute, useNavigate } from "@tanstack/react-router"
import { useState } from "react"

import { PromptInput } from "@/components/prompt-input"
import { createChat, sendMessage } from "@/lib/chat-store"
import { setComposerMode, useComposerMode } from "@/lib/mode-store"
import { notifyError } from "@/lib/toast"
import { usePreferredWorkspaceDir } from "@/lib/workspace-store"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  const navigate = useNavigate()
  const [sending, setSending] = useState(false)
  const mode = useComposerMode()
  const preferredWorkspaceDir = usePreferredWorkspaceDir()

  const handleSend = (value: string) => {
    if (sending) return
    setSending(true)

    void (async () => {
      try {
        // The mode the composer is in decides the address the conversation
        // opens at: an ordinary chat lives under /chat, a crew run under
        // /crew.
        const chat = await createChat(
          undefined,
          undefined,
          mode,
          preferredWorkspaceDir || undefined
        )
        await navigate({
          to: mode === "crew" ? "/crew/$chatId" : "/chat/$chatId",
          params: { chatId: chat.id },
        })
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
        onSend={handleSend}
        disabled={sending}
        mode={mode}
        onModeChange={setComposerMode}
      />
    </div>
  )
}

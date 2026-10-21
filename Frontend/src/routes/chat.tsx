import { createRoute, useNavigate } from "@tanstack/react-router"
import { useCallback, useEffect } from "react"

import { ChatThread } from "@/components/chat-thread"
import { updateChatMode, useChat } from "@/lib/chat-store"
import { Route as rootRoute } from "@/routes/__root"
import type { ChatMode } from "@/types/chat"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/chat/$chatId",
  component: ChatPage,
})

/**
 * An ordinary conversation. It lives under /chat, and the composer switch can
 * move it to /crew — which is what happens when the switch is turned.
 */
function ChatPage() {
  const { chatId } = Route.useParams()
  const navigate = useNavigate()
  const chat = useChat(chatId)

  // The address says which mode the conversation is in. A chat that was saved
  // as a crew run and then opened under /chat is an ordinary chat again, so the
  // row is brought back into line with the address it was opened at.
  useEffect(() => {
    if (chat && chat.mode !== "chat") void updateChatMode(chatId, "chat")
  }, [chat, chatId])

  // Moving the switch hands the conversation over to the other address: the
  // chat is saved as a crew run before the address changes, so the sidebar
  // points at where it now lives.
  const handleModeChange = useCallback(
    (mode: ChatMode) => {
      if (mode === "crew") {
        void updateChatMode(chatId, "crew")
        void navigate({ to: "/crew/$chatId", params: { chatId } })
        return
      }
      void updateChatMode(chatId, "chat")
    },
    [chatId, navigate]
  )

  return (
    <ChatThread
      chatId={chatId}
      mode="chat"
      onModeChange={handleModeChange}
    />
  )
}

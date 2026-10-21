import { createRoute, useNavigate } from "@tanstack/react-router"
import { useCallback, useEffect } from "react"

import { ChatThread } from "@/components/chat-thread"
import { updateChatMode, useChat } from "@/lib/chat-store"
import { Route as rootRoute } from "@/routes/__root"
import type { ChatMode } from "@/types/chat"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/crew/$chatId",
  component: CrewPage,
})

/**
 * A crew run: a long and complex job, kept apart from the ordinary
 * conversations so the two are told apart at a glance.
 *
 * It is the same conversation with a longer budget. The address is what says
 * so — under /crew the run is a crew run, and moving the composer switch
 * carries it back to /chat.
 */
function CrewPage() {
  const { chatId } = Route.useParams()
  const navigate = useNavigate()
  const chat = useChat(chatId)

  // The address is the mode, so the row is brought into line with it: a chat
  // opened under /crew is a crew run from then on.
  useEffect(() => {
    if (chat && chat.mode !== "crew") void updateChatMode(chatId, "crew")
  }, [chat, chatId])

  // Moving the switch back hands the conversation to the ordinary address.
  const handleModeChange = useCallback(
    (mode: ChatMode) => {
      if (mode === "chat") {
        void updateChatMode(chatId, "chat")
        void navigate({ to: "/chat/$chatId", params: { chatId } })
        return
      }
      void updateChatMode(chatId, "crew")
    },
    [chatId, navigate]
  )

  return (
    <ChatThread
      chatId={chatId}
      mode="crew"
      onModeChange={handleModeChange}
    />
  )
}

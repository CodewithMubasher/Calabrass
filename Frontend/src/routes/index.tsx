import { createRoute, useNavigate } from "@tanstack/react-router"

import { PromptInput } from "@/components/prompt-input"
import { createChat, sendMessage } from "@/lib/chat-store"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  const navigate = useNavigate()

  const handleSend = (value: string) => {
    const chat = createChat()
    sendMessage(chat.id, value)
    navigate({ to: "/chat/$chatId", params: { chatId: chat.id } })
  }

  return <PromptInput onSend={handleSend} />
}

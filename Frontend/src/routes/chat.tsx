import { createRoute } from "@tanstack/react-router"

import { MessageBubble } from "@/components/message-bubble"
import { PromptInput } from "@/components/prompt-input"
import { MessageGroup } from "@/components/ui/message"
import { sendMessage, useChats } from "@/lib/chat-store"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/chat/$chatId",
  component: ChatPage,
})

function ChatPage() {
  const { chatId } = Route.useParams()
  const chats = useChats()
  const chat = chats.find((item) => item.id === chatId)

  if (!chat) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-white/50">
        Chat not found.
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <div className="flex-1 overflow-y-auto">
        <MessageGroup className="mx-auto w-full max-w-[780px] gap-4 px-4 py-6">
          {chat.messages.map((message) => (
            <MessageBubble key={message.id} message={message} />
          ))}
        </MessageGroup>
      </div>
      <div className="shrink-0">
        <PromptInput isInChat onSend={(value) => sendMessage(chat.id, value)} />
      </div>
    </div>
  )
}

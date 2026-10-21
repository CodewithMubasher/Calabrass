import {
  createRoute,
  Outlet,
  useNavigate,
  useRouterState,
} from "@tanstack/react-router"
import { useEffect } from "react"

import { ChatThread } from "@/components/chat-thread"
import { updateChatMode, useChat } from "@/lib/chat-store"
import { isCrewPath } from "@/lib/routes"
import { Route as rootRoute } from "@/routes/__root"
import type { ChatMode } from "@/types/chat"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  // A layout route with no path of its own: it holds the conversation, and
  // both of its children sit inside it. The point is that it stays mounted
  // when the conversation moves from one address to the other — so the
  // composer's mode switch slides rather than jumping, which it could not do
  // if the whole thread were torn down and built again.
  id: "conversation",
  component: ConversationLayout,
})

function ConversationLayout() {
  // The address and the id both belong to the child route, so they are read
  // from the match that is deepest rather than from this one — a layout route
  // has no path of its own, and so has no params of its own either.
  const pathname = useRouterState({
    select: (state) =>
      state.matches.length > 0
        ? state.matches[state.matches.length - 1].pathname
        : state.location.pathname,
  })
  const chatId = useRouterState({
    select: (state) => {
      const last =
        state.matches.length > 0
          ? state.matches[state.matches.length - 1]
          : undefined
      const params = last?.params as { chatId?: string } | undefined
      return String(params?.chatId ?? "")
    },
  })
  const navigate = useNavigate()

  // The address says which mode the conversation is in: /chat is an ordinary
  // chat, /crew is a crew run.
  const mode: ChatMode = isCrewPath(pathname) ? "crew" : "chat"
  const chat = useChat(chatId)

  // The address is the mode, so the row is brought into line with it: a chat
  // opened under /crew is a crew run from then on, and one opened under /chat
  // is an ordinary conversation again.
  useEffect(() => {
    if (chat && chat.mode !== mode) void updateChatMode(chatId, mode)
  }, [chat, chatId, mode])

  // Moving the switch hands the conversation over to the other address. The
  // row is saved first, so the sidebar points at where it now lives.
  const handleModeChange = (next: ChatMode) => {
    void updateChatMode(chatId, next)
    void navigate({
      to: next === "crew" ? "/crew/$chatId" : "/chat/$chatId",
      params: { chatId },
    })
  }

  return (
    <>
      <ChatThread chatId={chatId} mode={mode} onModeChange={handleModeChange} />
      <Outlet />
    </>
  )
}

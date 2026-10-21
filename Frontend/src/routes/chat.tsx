import { createRoute } from "@tanstack/react-router"

import { Route as conversationRoute } from "@/routes/conversation"

export const Route = createRoute({
  getParentRoute: () => conversationRoute,
  path: "/chat/$chatId",
  // The conversation itself is the layout's work; this route only says where
  // an ordinary chat lives.
  component: () => null,
})

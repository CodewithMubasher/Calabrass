import { createRoute } from "@tanstack/react-router"

import { PromptInput } from "@/components/prompt-input"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: HomePage,
})

function HomePage() {
  return <PromptInput onSend={(value) => console.log("prompt:", value)} />
}

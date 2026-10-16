import { createRoute } from "@tanstack/react-router"

import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspaces",
  component: WorkspacesPage,
})

function WorkspacesPage() {
  return <div className="flex-1" />
}

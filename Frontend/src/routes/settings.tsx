import { createRoute } from "@tanstack/react-router"

import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

function SettingsPage() {
  return <div className="flex-1" />
}

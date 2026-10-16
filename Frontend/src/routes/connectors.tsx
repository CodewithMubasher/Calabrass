import { createRoute } from "@tanstack/react-router"

import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connectors",
  component: ConnectorsPage,
})

function ConnectorsPage() {
  return <div className="flex-1" />
}

import { createRouter } from "@tanstack/react-router"

import { Route as rootRoute } from "@/routes/__root"
import { Route as indexRoute } from "@/routes/index"
import { Route as workspacesRoute } from "@/routes/workspaces"
import { Route as connectorsRoute } from "@/routes/connectors"
import { Route as schedulesRoute } from "@/routes/schedules"
import { Route as settingsRoute } from "@/routes/settings"

const routeTree = rootRoute.addChildren([
  indexRoute,
  workspacesRoute,
  connectorsRoute,
  schedulesRoute,
  settingsRoute,
])

export const router = createRouter({
  routeTree,
  defaultPreload: "intent",
  scrollRestoration: true,
})

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router
  }
}

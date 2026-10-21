import { createRouter } from "@tanstack/react-router"

import { Route as rootRoute } from "@/routes/__root"
import { Route as indexRoute } from "@/routes/index"
import { Route as loginRoute } from "@/routes/login"
import { Route as signupRoute } from "@/routes/signup"
import { Route as workspacesRoute } from "@/routes/workspaces"
import { Route as workspaceViewRoute } from "@/routes/workspace-view"
import { Route as connectorsRoute } from "@/routes/connectors"
import { Route as agentsRoute } from "@/routes/agents"
import { Route as schedulesRoute } from "@/routes/schedules"
import { Route as autonomousRoute } from "@/routes/autonomous"
import { Route as providersRoute } from "@/routes/providers"
import { Route as usageRoute } from "@/routes/usage"
import { Route as calLiveRoute } from "@/routes/cal-live"
import { Route as chatRoute } from "@/routes/chat"
import { Route as crewRoute } from "@/routes/crew"

const routeTree = rootRoute.addChildren([
  indexRoute,
  chatRoute,
  crewRoute,
  loginRoute,
  signupRoute,
  calLiveRoute,
  workspacesRoute,
  workspaceViewRoute,
  connectorsRoute,
  agentsRoute,
  schedulesRoute,
  providersRoute,
  usageRoute,
  autonomousRoute,
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

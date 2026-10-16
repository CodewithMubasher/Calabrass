import { createRoute } from "@tanstack/react-router"

import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/schedules",
  component: SchedulesPage,
})

function SchedulesPage() {
  return <div className="flex-1" />
}

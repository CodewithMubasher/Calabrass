import { TimeScheduleIcon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty, PageShell } from "@/components/page-shell"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/schedules",
  component: SchedulesPage,
})

function SchedulesPage() {
  return (
    <PageShell
      title="Schedules"
      subtitle="Run prompts and tools on a timer"
    >
      <PageEmpty
        icon={TimeScheduleIcon}
        title="Nothing scheduled yet"
        hint="Schedules are not available yet"
      />
    </PageShell>
  )
}

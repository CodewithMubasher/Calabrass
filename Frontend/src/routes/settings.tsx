import { Settings02Icon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty, PageShell } from "@/components/page-shell"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

function SettingsPage() {
  return (
    <PageShell title="Settings" subtitle="Preferences for Calabrass">
      <PageEmpty
        icon={Settings02Icon}
        title="Nothing to set up yet"
        hint="Preferences are not available yet"
      />
    </PageShell>
  )
}

import { useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react"
import {
  Database01Icon,
  InformationCircleIcon,
  SlidersHorizontalIcon,
  SparklesIcon,
  UserAccountIcon,
  UserCircleIcon,
} from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageShell } from "@/components/page-shell"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/settings",
  component: SettingsPage,
})

/** The sections the settings page will hold, and what each one is about. */
const SECTIONS: { id: string; title: string; icon: IconSvgElement }[] = [
  { id: "profile", title: "Profile", icon: UserCircleIcon },
  { id: "general", title: "General", icon: SlidersHorizontalIcon },
  { id: "account", title: "Account", icon: UserAccountIcon },
  { id: "data", title: "User data", icon: Database01Icon },
  { id: "about", title: "About", icon: InformationCircleIcon },
]

/**
 * A section nav down the left — a card the width of the subtitle above it — and
 * the section itself filling everything to its right. Nothing is written into
 * the right card yet: each section says it is on its way, which is also what
 * tells the nav it is live.
 */
function SettingsPage() {
  const [sectionId, setSectionId] = useState(SECTIONS[0].id)
  const section = SECTIONS.find((entry) => entry.id === sectionId) ?? SECTIONS[0]

  return (
    <PageShell title="Settings" subtitle="Preferences for Calabrass">
      <div className="flex h-full items-start gap-6">
        <nav className="flex w-44 shrink-0 flex-col rounded-2xl border border-white/10 bg-white/5 p-1.5">
          {SECTIONS.map((entry) => {
            const active = entry.id === section.id
            return (
              <button
                key={entry.id}
                type="button"
                onClick={() => setSectionId(entry.id)}
                aria-current={active ? "true" : undefined}
                className={cn(
                  "flex w-full cursor-pointer items-center gap-2.5 rounded-xl px-2.5 py-2 text-left text-sm transition-colors",
                  active
                    ? "bg-white/10 text-white"
                    : "text-white/60 hover:bg-white/5 hover:text-white/90"
                )}
              >
                <HugeiconsIcon
                  icon={entry.icon}
                  strokeWidth={2}
                  className="size-4 shrink-0"
                />
                <span className="truncate">{entry.title}</span>
              </button>
            )
          })}
        </nav>

        <div className="flex h-full min-w-0 flex-1 items-center justify-center rounded-2xl border border-white/10 bg-white/5">
          <div className="text-center">
            <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-xl bg-white/5">
              <HugeiconsIcon
                icon={SparklesIcon}
                strokeWidth={1.8}
                className="size-5 text-white/30"
              />
            </div>
            <p className="text-sm text-white/40">Coming soon</p>
            <p className="mt-1 text-xs text-white/25">
              {section.title} settings are on the way
            </p>
          </div>
        </div>
      </div>
    </PageShell>
  )
}

import { createRootRoute, Outlet, useLocation } from "@tanstack/react-router"
import { useEffect } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  BubbleChatTemporaryIcon,
  Files01Icon,
  GiftIcon,
} from "@hugeicons/core-free-icons"

import { AppSidebar } from "@/components/app-sidebar"
import { ArtifactsPanel } from "@/components/artifacts-panel"
import { KeyboardShortcuts } from "@/components/keyboard-shortcuts"
import WaveBackground from "@/components/wave-background"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { hydrate } from "@/lib/chat-store"
import { loadModels } from "@/lib/model-store"
import { toggleArtifacts, usePanelState } from "@/lib/panel-store"
import { hasTopbar } from "@/lib/routes"

export const Route = createRootRoute({
  component: RootLayout,
})

function RootLayout() {
  const { pathname } = useLocation()
  // The home screen and a chat carry the bar across the top: every other page is
  // its own full-height surface, the way the reference project lays them out.
  // The artifacts rail rides with that bar, which is where its icon is.
  const showsTopbar = hasTopbar(pathname)
  const { artifactsOpen } = usePanelState()

  useEffect(() => {
    void hydrate()
    void loadModels()
  }, [])

  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <KeyboardShortcuts />
      <AppSidebar />
      <SidebarInset className="relative overflow-hidden bg-background">
        <WaveBackground fixed />
        <div className="relative z-10 flex min-h-0 flex-1 overflow-hidden">
          <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
            {showsTopbar ? (
              <header className="relative flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear">
                <div className="flex items-center gap-2 px-4">
                  <SidebarTrigger className="-ml-1 size-[29px]" />
                </div>

                {/* Centered badge */}
                <div className="pointer-events-none absolute inset-x-0 flex justify-center">
                  <Badge className="pointer-events-auto h-6 gap-1 bg-sky-50 px-2.5 text-xs font-medium text-sky-700 dark:bg-sky-950 dark:text-sky-300">
                    <HugeiconsIcon icon={GiftIcon} strokeWidth={2} />
                    Upgrade for free
                  </Badge>
                </div>

                {/* Right corner icons — same size as the sidebar trigger button */}
                <div className="ml-auto flex items-center gap-1 px-4">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Artifacts"
                    aria-expanded={artifactsOpen}
                    onClick={toggleArtifacts}
                    className="size-[29px]"
                  >
                    <HugeiconsIcon icon={Files01Icon} strokeWidth={2} />
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Messages"
                    className="size-[29px]"
                  >
                    <HugeiconsIcon
                      icon={BubbleChatTemporaryIcon}
                      strokeWidth={2}
                    />
                  </Button>
                </div>
              </header>
            ) : null}
            <div className="relative flex min-h-0 flex-1 flex-col">
              <Outlet />
            </div>
          </div>
          {showsTopbar ? <ArtifactsPanel open={artifactsOpen} /> : null}
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}

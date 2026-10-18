import {
  createRootRoute,
  Outlet,
  redirect,
  useLocation,
  useNavigate,
} from "@tanstack/react-router"
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
import { refreshSession, session } from "@/lib/auth"
import { loadModels } from "@/lib/model-store"
import { toggleArtifacts, usePanelState } from "@/lib/panel-store"
import { isStandalone, hasTopbar, chatIdFrom } from "@/lib/routes"

export const Route = createRootRoute({
  /**
   * The door, checked on every navigation: nothing but the login and sign-up
   * pages is served without a session, and those two are not served with one.
   * The session is a stored token; the layout below checks it against the
   * server once on load and sends an expired one back here. The local API is
   * still open on loopback, so this decides what the app shows rather than what
   * the machine allows.
   */
  beforeLoad: ({ location }) => {
    const signedIn = session() !== null
    if (isStandalone(location.pathname)) {
      if (signedIn) throw redirect({ to: "/" })
      return
    }
    if (!signedIn) throw redirect({ to: "/login" })
  },
  component: RootLayout,
})

function RootLayout() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  // The home screen and a chat carry the bar across the top: every other page is
  // its own full-height surface, the way the reference project lays them out.
  // The artifacts rail rides with that bar, which is where its icon is.
  const showsTopbar = hasTopbar(pathname)
  const { artifactsOpen } = usePanelState()

  useEffect(() => {
    void hydrate()
    void loadModels()
  }, [])

  // The stored token may have been revoked or expired since the last visit.
  // Check it once; the gate above already let this page render from storage.
  useEffect(() => {
    void refreshSession().then((current) => {
      if (!current && !isStandalone(window.location.pathname)) {
        void navigate({ to: "/login", replace: true })
      }
    })
  }, [navigate])

  // The login page stands on its own: nothing from the shell is around it, not
  // even the surface the rest of the app sits in. Its page brings its own.
  if (isStandalone(pathname)) {
    return <Outlet />
  }

  return (
    <SidebarProvider className="h-svh overflow-hidden">
      <KeyboardShortcuts />
      <AppSidebar />
      <SidebarInset className="relative overflow-hidden bg-background">
        {/* A desktop gets the field over the whole surface. A phone gets a low
            band across the foot of the screen instead: the heading in the
            middle of it has to read on black, and the one part of a phone that
            wants something behind it is the composer, docked at the bottom. */}
        <WaveBackground fixed className="max-md:hidden" />
        <WaveBackground
          fixed
          band
          speed={0.55}
          className="hidden max-md:block"
        />
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
            <div className="relative flex min-h-0 flex-1 scrollbar-thin flex-col overflow-y-auto">
              <Outlet />
            </div>
          </div>
          {showsTopbar ? (
            <ArtifactsPanel
              open={artifactsOpen}
              chatId={chatIdFrom(pathname)}
            />
          ) : null}
        </div>
      </SidebarInset>
    </SidebarProvider>
  )
}

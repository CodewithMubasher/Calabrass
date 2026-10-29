import {
  createRootRoute,
  Outlet,
  redirect,
  useNavigate,
  useRouterState,
} from "@tanstack/react-router"
import { useEffect } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ApertureIcon,
  Files01Icon,
} from "@hugeicons/core-free-icons"

import { AppSidebar } from "@/components/app-sidebar"
import { ApprovalDialog } from "@/components/approval-dialog"
import { ArtifactsPanel } from "@/components/artifacts-panel"
import { CrewPanel } from "@/components/crew-panel"
import { KeyboardShortcuts } from "@/components/keyboard-shortcuts"
import WaveBackground from "@/components/wave-background"
import { WorkspaceBadge } from "@/components/workspace-badge"
import { Button } from "@/components/ui/button"
import { Toaster } from "@/components/ui/toast"
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { watchApprovals } from "@/lib/approval-store"
import { hydrate } from "@/lib/chat-store"
import { refreshSession, session, useSession } from "@/lib/auth"
import { loadModels } from "@/lib/model-store"
import { toggleArtifacts, toggleCrew, usePanelState } from "@/lib/panel-store"
import { isStandalone, hasTopbar, chatIdFrom } from "@/lib/routes"

// Whether this run of the app has already asked the server about the stored
// token. Module scope on purpose: it is a property of the app's lifetime, not
// of any one render.
let sessionChecked = false

export const Route = createRootRoute({
  /**
   * The door, checked on every navigation: nothing but the login and sign-up
   * pages is served without a session, and those two are not served with one.
   * The session is a stored token; the layout below checks it against the
   * server once on load and sends an expired one back here. The local API is
   * still open on loopback, so this decides what the app shows rather than what
   * the machine allows.
   */
  beforeLoad: async ({ location }) => {
    const signedIn = session() !== null
    if (isStandalone(location.pathname)) {
      if (signedIn) throw redirect({ to: "/" })
      return
    }
    if (!signedIn) throw redirect({ to: "/login" })

    // Validate the token against the server once per run of the app, not on
    // every link click. This is a network call, and a navigation that waits on
    // it leaves the previous page on screen long enough to see. Later visits
    // rely on the stored session; the layout below sends an expired one back
    // here if the server rejects it.
    if (!sessionChecked) {
      const current = await refreshSession()
      if (!current) {
        throw redirect({ to: "/login", replace: true })
      }
      sessionChecked = true
    }
  },
  component: RootLayout,
})

function RootLayout() {
  // The path the outlet is actually showing, not the one the router is still
  // moving to. `useLocation` reports the target the moment a link is clicked,
  // while the outlet keeps the previous page until the new match commits — so
  // asking it here used to draw the bar and the artifacts rail over the old
  // page for a frame on the way to the home screen. Committed matches move in
  // step with the outlet, so the shell and the page always agree.
  const pathname = useRouterState({
    select: (state) =>
      state.matches.length > 0
        ? state.matches[state.matches.length - 1].pathname
        : state.location.pathname,
  })
  const navigate = useNavigate()
  // The home screen and a chat carry the bar across the top: every other page is
  // its own full-height surface, the way the reference project lays them out.
  // The artifacts rail rides with that bar, which is where its icon is.
  const showsTopbar = hasTopbar(pathname)
  const { artifactsOpen, crewOpen } = usePanelState()
  // Whether anyone is signed in, not who: the session check below rebuilds the
  // account object on every run, so keying on the object itself would re-run the
  // effect forever.
  const signedIn = useSession() !== null

  // Validate the session, then load the account's data. The login and sign-up
  // pages are reached without a session, so there is nothing to load there and
  // asking would only 401 — which used to leave the store unhydrated and every
  // chat stuck on "Loading…" until the app was restarted. Re-running when
  // someone signs in or out is what lets a fresh sign-in load its chats.
  useEffect(() => {
    let mounted = true
    void (async () => {
      const current = await refreshSession()
      if (!mounted) return
      if (!current) {
        if (!isStandalone(window.location.pathname)) {
          void navigate({ to: "/login", replace: true })
        }
        return
      }
      void hydrate()
      void loadModels()
    })()
    return () => {
      mounted = false
    }
  }, [navigate, signedIn])

  // Follow what the agent is waiting on a person to allow, for as long as
  // someone is signed in. Whichever window or turn raised it, it lands here.
  useEffect(() => {
    if (!signedIn) return
    return watchApprovals()
  }, [signedIn])

  // The login page stands on its own: nothing from the shell is around it, not
  // even the surface the rest of the app sits in. Its page brings its own.
  if (isStandalone(pathname)) {
    return (
      <>
        <Outlet />
        <Toaster />
      </>
    )
  }

  return (
    <>
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

                  {/* Centered badge: the folder this chat works in. Clicking it
                      moves the chat, and the AI and crew workers follow. */}
                  <div className="pointer-events-none absolute inset-x-0 flex justify-center">
                    <div className="pointer-events-auto">
                      <WorkspaceBadge chatId={chatIdFrom(pathname)} />
                    </div>
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
                    {/* The crew mark: the way into the Crew work rail, where
                        the long and complex work is gathered. */}
                    <Button
                      variant="ghost"
                      size="icon-sm"
                      aria-label="Crew work"
                      aria-expanded={crewOpen}
                      onClick={toggleCrew}
                      className="size-[29px]"
                    >
                      <HugeiconsIcon icon={ApertureIcon} strokeWidth={2} />
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
            {showsTopbar ? (
              <CrewPanel open={crewOpen} chatId={chatIdFrom(pathname)} />
            ) : null}
          </div>
        </SidebarInset>
      </SidebarProvider>
      {/* The notices: every error and every bit of feedback says itself here
          rather than on the page it happened on. */}
      <ApprovalDialog />
      <Toaster />
    </>
  )
}

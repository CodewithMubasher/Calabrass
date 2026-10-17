import { createRoute } from "@tanstack/react-router"

import { LoginForm } from "@/components/login-form"
import WaveBackground from "@/components/wave-background"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  component: LoginPage,
})

/**
 * shadcn's `login-02`, minus its cover photograph and its two halves: the mark
 * stays in the top-left corner where the block puts it, the form holds the
 * middle of the page, and the wave field runs behind both — what the block
 * fills with a picture is what our background already draws. The shell drops
 * the sidebar, the bar and the rail for this route (see `isStandalone`).
 */
function LoginPage() {
  return (
    <div className="relative flex min-h-svh flex-col items-center justify-center p-6 md:p-10">
      <WaveBackground fixed />

      <div className="absolute top-6 left-6 z-10 flex items-center gap-2 font-medium md:top-10 md:left-10">
        <span className="flex size-6 items-center justify-center rounded-md border border-white/10 bg-white/5">
          <img src="/vite.svg" alt="" className="size-3.5" />
        </span>
        Calabrass
      </div>

      <div className="relative z-10 w-full max-w-xs">
        <LoginForm />
      </div>
    </div>
  )
}

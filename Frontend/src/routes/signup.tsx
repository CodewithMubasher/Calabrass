import { createRoute } from "@tanstack/react-router"

import { SignupForm } from "@/components/signup-form"
import WaveBackground from "@/components/wave-background"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/signup",
  component: SignupPage,
})

/**
 * The same standalone surface as the login page: the mark in the top-left, the
 * wave field behind, and the form in the middle. The shell drops the sidebar,
 * the bar and the rail for this route (see `isStandalone`).
 */
function SignupPage() {
  return (
    <div className="relative flex min-h-svh flex-col items-center justify-center p-6 md:p-10">
      <WaveBackground fixed />

      <div className="absolute top-6 left-6 z-10 flex items-center gap-2 font-medium md:top-10 md:left-10">
        <img src="/vite.svg" alt="" className="size-5" />
        Calabrass
      </div>

      <div className="relative z-10 w-full max-w-xs">
        <SignupForm />
      </div>
    </div>
  )
}

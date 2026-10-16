import { createRoute } from "@tanstack/react-router"

import { CalLiveOrb } from "@/components/cal-live-orb"
import { useSpeech } from "@/lib/speech"
import { Route as rootRoute } from "@/routes/__root"

/** What Cal says when the stage opens. */
const GREETING = "Hi there, how can I help you today?"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/cal-live",
  component: CalLivePage,
})

function CalLivePage() {
  // The intro is pure CSS, so it runs again every time this page mounts.
  // `hostRef` is where the voice writes `--level`, which the orb reads for its
  // speaking pulse (custom properties inherit down to it).
  const { supported, speaking, speak, hostRef } = useSpeech(GREETING)

  return (
    <div
      ref={hostRef}
      className="flex min-h-0 flex-1 -translate-y-[5vh] flex-col items-center justify-center gap-[clamp(1.5rem,5vh,3rem)] px-6 py-8 text-center"
    >
      <button
        type="button"
        onClick={speak}
        disabled={!supported}
        aria-label="Hear Cal's greeting"
        className="cursor-pointer rounded-full outline-none focus-visible:ring-2 focus-visible:ring-white/40"
      >
        <CalLiveOrb />
      </button>

      <div className="max-w-md">
        <h1 className="text-xl font-semibold text-white">Cal Live</h1>
        <p className="mt-1 text-sm text-white/50">{GREETING}</p>
        {supported && (
          <p className="mt-2 text-xs text-white/30">
            {speaking ? "Speaking…" : "Tap the orb to hear it again"}
          </p>
        )}
      </div>
    </div>
  )
}

import { useEffect, useRef } from "react"
import { createRoute } from "@tanstack/react-router"
import { cn } from "cn"

import { CalLiveOrb } from "@/components/cal-live-orb"
import { useVoice } from "@/lib/voice"
import { Route as rootRoute } from "@/routes/__root"

/** What Cal says when the stage opens. */
const GREETING = "Hi there, how can I help you today?"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/cal-live",
  component: CalLivePage,
})

function CalLivePage() {
  // The microphone is open as soon as the stage is, so there is nothing to
  // click to start talking. The orb is the one control left: it pauses the
  // session, and clicking again carries on.
  const { phase, caption, turn, error, supported, toggle } = useVoice(GREETING)
  const hostRef = useRef<HTMLDivElement | null>(null)
  const logRef = useRef<HTMLDivElement | null>(null)

  // A long answer can still run past the box: keep its end in view.
  useEffect(() => {
    const log = logRef.current
    if (log) log.scrollTop = log.scrollHeight
  }, [turn, caption])

  // The orb breathes with the voice, the way the intro always did.
  useEffect(() => {
    const host = hostRef.current
    if (!host) return
    if (phase !== "speaking" && phase !== "thinking") {
      host.style.removeProperty("--level")
      return
    }
    const timer = window.setInterval(() => {
      const level =
        phase === "speaking" ? 0.45 + Math.random() * 0.5 : 0.12 + Math.random() * 0.15
      host.style.setProperty("--level", level.toFixed(3))
    }, 170)
    return () => {
      window.clearInterval(timer)
      host.style.removeProperty("--level")
    }
  }, [phase])

  return (
    <div
      ref={hostRef}
      className={cn(
        "flex min-h-0 flex-1 -translate-y-[5vh] flex-col items-center justify-center gap-[clamp(1.5rem,5vh,3rem)] px-6 py-8 text-center",
        `cal-live--${phase}`
      )}
    >
      <button
        type="button"
        onClick={toggle}
        disabled={!supported}
        aria-label={phase === "paused" ? "Carry on talking" : "Pause the conversation"}
        className="cursor-pointer rounded-full outline-none focus-visible:ring-2 focus-visible:ring-white/40"
      >
        <CalLiveOrb />
      </button>

      <div className="max-w-md text-center">
        <h1 className="text-xl font-semibold text-white">Cal Live</h1>

        {/* One exchange at a time: the question, then the answer, and the line
            still being heard while there is not one yet. The whole conversation
            is in the chat, not here. */}
        <div
          ref={logRef}
          className="scrollbar-thin mt-3 max-h-52 space-y-1.5 overflow-y-auto text-left"
        >
          {turn.user ? (
            <p className="text-xs leading-5 text-white/55">
              <span className="mr-1.5 text-[10px] tracking-wide text-white/30 uppercase">
                You
              </span>
              {turn.user}
            </p>
          ) : null}

          {turn.assistant ? (
            <p className="text-xs leading-5 text-white/90">
              <span className="mr-1.5 text-[10px] tracking-wide text-white/30 uppercase">
                Cal
              </span>
              {turn.assistant}
            </p>
          ) : null}

          {/* Live words while a question is still being heard, and the status
              when there is nothing to hear: paused, not connected, quiet. */}
          {caption ? (
            <p className="text-xs leading-5 text-white/45 italic">{caption}</p>
          ) : null}
        </div>

        {error ? <p className="mt-2 text-xs text-red-400">{error}</p> : null}
      </div>
    </div>
  )
}

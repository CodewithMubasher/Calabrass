import { useRef, useState, type PointerEvent } from "react"
import { Link } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import { ApertureIcon } from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { useIsMobile } from "@/hooks/use-mobile"
import { useChats } from "@/lib/chat-store"
import { setCrewOpen } from "@/lib/panel-store"
import type { Chat } from "@/types/chat"

/**
 * The Crew work rail: a right-hand panel that *pushes* the page instead of
 * covering it, which is why it sits beside the routed column in `__root.tsx`
 * rather than inside a page.
 *
 * It is the artifacts rail's twin, down to the way it comes in on a phone: the
 * outer element reserves the space while the panel inside keeps its full size
 * and is revealed by that width, so its contents never squash mid-transition.
 *
 * What it lists is the crew runs themselves — the conversations that were
 * started in crew mode, newest first. Each one is a link to the run, so the
 * rail is a way back into work that is already going rather than a place to
 * start it.
 */
export function CrewPanel({ open }: { open: boolean }) {
  const chats = useChats()
  const isMobile = useIsMobile()
  const sheetRef = useRef<HTMLElement>(null)

  /**
   * How far the sheet has been dragged, in pixels to the right of open, or null
   * when it is not being held. The sheet is shut at its own width and open at
   * zero, so a rightward drag is what puts it away and the same number answers
   * both directions.
   */
  const [slideX, setSlideX] = useState<number | null>(null)
  const swipe = useRef({
    x: 0,
    y: 0,
    /** Where the drag started: 0 open, the sheet's width shut. */
    from: 0,
    width: 0,
    /** What the first real movement said this drag is. */
    axis: "" as "" | "x" | "y",
    startedAt: 0,
  })

  /**
   * A swipe on a phone. It begins on the edge strip when the rail is shut and on
   * the rail itself when it is open; either way the sheet follows the finger,
   * and the first movement decides what the drag is — sideways is the sheet's,
   * downward is the list's scroll.
   *
   * Desktop never starts one: the rail there is pushed by the layout, not
   * dragged, and a stray mouse drag must not put it away.
   */
  const startSwipe = (event: PointerEvent<HTMLElement>) => {
    if (!isMobile) return
    const width = sheetRef.current?.offsetWidth ?? 0
    if (width === 0) return

    swipe.current = {
      x: event.clientX,
      y: event.clientY,
      from: open ? 0 : width,
      width,
      axis: "",
      startedAt: performance.now(),
    }
    setSlideX(swipe.current.from)
  }

  const moveSwipe = (event: PointerEvent<HTMLElement>) => {
    const gesture = swipe.current
    if (gesture.width === 0) return

    const dx = event.clientX - gesture.x
    const dy = event.clientY - gesture.y

    if (gesture.axis === "") {
      // A few pixels of slop first, so a tap stays a tap and a scroll starts
      // where the finger did.
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
      if (Math.abs(dx) <= Math.abs(dy)) {
        // Not ours. Dropping out of the gesture leaves the list its scroll.
        gesture.axis = "y"
        gesture.width = 0
        setSlideX(null)
        return
      }
      gesture.axis = "x"
    }

    // Neither past the middle nor off the edge: the sheet tracks the finger
    // within the two states it has.
    setSlideX(Math.min(gesture.width, Math.max(0, gesture.from + dx)))
  }

  const endSwipe = () => {
    const gesture = swipe.current
    const { width, from, axis, startedAt } = gesture
    gesture.width = 0

    if (axis !== "x") {
      setSlideX(null)
      return
    }

    const settled = slideX ?? from
    // Towards shut is a positive drag; towards open, negative.
    const towards = settled - from
    const flick = performance.now() - startedAt < 250 && Math.abs(towards) > 24

    // A flick is a decision on its own. Otherwise the sheet has to have been
    // carried past the middle for the state to change.
    setCrewOpen(flick ? towards < 0 : settled < width / 2)
    setSlideX(null)
  }

  // The runs the rail lists: the conversations that were started in crew mode.
  const runs = chats.filter((chat) => chat.mode === "crew")

  return (
    <>
      {/* The app behind the sheet, dimmed on a phone only, and tappable: a
          backdrop is what a drawer is put away with when the swipe is not the
          way the user reaches for. */}
      <div
        aria-hidden="true"
        onClick={() => setCrewOpen(false)}
        className={cn(
          "hidden max-md:fixed max-md:inset-0 max-md:z-30 max-md:block max-md:bg-black/60 max-md:transition-opacity max-md:duration-300",
          open ? "max-md:opacity-100" : "max-md:pointer-events-none max-md:opacity-0"
        )}
      />

      {/* Where a swipe in begins: a finger's width of the screen's right edge,
          which is empty gutter on every screen the rail can be opened from. */}
      {!open ? (
        <div
          aria-hidden="true"
          onPointerDown={startSwipe}
          onPointerMove={moveSwipe}
          onPointerUp={endSwipe}
          onPointerCancel={endSwipe}
          className="hidden max-md:fixed max-md:inset-y-0 max-md:right-0 max-md:z-30 max-md:block max-md:w-4 max-md:touch-none"
        />
      ) : null}

      <aside
        ref={sheetRef}
        aria-label="Crew work"
        aria-hidden={!open}
        inert={!open}
        onPointerDown={startSwipe}
        onPointerMove={moveSwipe}
        onPointerUp={endSwipe}
        onPointerCancel={endSwipe}
        style={
          slideX === null
            ? undefined
            : // `translate`, not `transform`: that is the property Tailwind
              // v4's translate utilities set, and with no transition the sheet
              // is under the finger rather than chasing after it.
            { translate: `${slideX}px`, transition: "none" }
        }
        className={cn(
          "flex h-full shrink-0 items-center overflow-hidden transition-[width] duration-300 ease-out",
          // A phone gives the rail no width to take, so it stops pushing and
          // comes in over the app instead, leaving a sliver of it showing so it
          // is plain the sheet slid over something.
          "max-md:fixed max-md:inset-y-0 max-md:right-0 max-md:z-40 max-md:w-[86%] max-md:max-w-[22rem] max-md:touch-pan-y max-md:transition-transform",
          open ? "w-[376px]" : "w-0 max-md:translate-x-full"
        )}
      >
        <div
          className={cn(
            "flex h-[calc(100%-2rem)] shrink-0 flex-col overflow-hidden rounded-2xl border border-white/15 bg-black/40 backdrop-blur-xl transition-[width] duration-300 ease-out",
            // 360px of panel beside a 16px gutter.
            "mr-4 w-[360px]",
            // A phone's sheet is the card itself, right up to the edge it slid
            // in from, with only the corners it came in on rounded.
            "max-md:mx-0 max-md:h-full max-md:w-full max-md:rounded-l-2xl max-md:rounded-r-none max-md:border-y-0 max-md:border-r-0"
          )}
        >
          <header className="flex h-11 shrink-0 items-center gap-2 px-3">
            <HugeiconsIcon
              icon={ApertureIcon}
              strokeWidth={2}
              className="size-4 text-white/50"
            />
            <span className="text-sm font-medium text-white">Crew work</span>
          </header>

          <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto p-3">
            {runs.length === 0 ? (
              <div className="flex h-full items-center justify-center px-6 text-center">
                <p className="text-xs text-white/40">
                  No crew runs yet. Turn the composer to Crew and send a task.
                </p>
              </div>
            ) : (
              <div className="flex flex-col gap-2">
                {runs.map((run) => (
                  <CrewRunCard key={run.id} run={run} />
                ))}
              </div>
            )}
          </div>
        </div>
      </aside>
    </>
  )
}

/** One crew run in the list: its name, and when it was last used. */
function CrewRunCard({ run }: { run: Chat }) {
  return (
    <Link
      to="/crew/$chatId"
      params={{ chatId: run.id }}
      className="flex cursor-pointer flex-col gap-1 rounded-xl border border-white/10 bg-white/5 px-3 py-2.5 transition-colors hover:border-white/20 hover:bg-white/10"
    >
      <span className="flex items-center gap-2">
        <HugeiconsIcon
          icon={ApertureIcon}
          strokeWidth={2}
          className="size-3.5 shrink-0 text-[#9D79FF]"
        />
        <span className="min-w-0 flex-1 truncate text-sm text-white/90">
          {run.title}
        </span>
      </span>
      <span className="text-[11px] text-white/35">
        {run.model || run.provider || "No model"}
      </span>
    </Link>
  )
}

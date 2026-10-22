import { useEffect, useRef, useState, type PointerEvent } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ApertureIcon } from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { ArtifactCard } from "@/components/artifact-card"
import { ArtifactDetail } from "@/components/artifact-detail"
import { useIsMobile } from "@/hooks/use-mobile"
import {
  setSelectedCrewPath,
  useCrewFiles,
  useSelectedCrewPath,
  watchCrew,
} from "@/lib/crew-store"
import { setCrewOpen } from "@/lib/panel-store"
import { api, startDownload } from "@/lib/api"

/**
 * The Crew work rail: the artifacts rail's twin, watching a different folder.
 *
 * .crew holds what the crew is doing — one todo per delegation, one report per
 * worker — and the backend pushes a new listing whenever either is written. A
 * file that appears while the rail is shut opens it on itself, which is what
 * makes a delegation visible the moment the Orchestrator hands it out.
 *
 * It stays mounted and animates its own width, so closing is as smooth as
 * opening, and the same maximize rule applies: the rail grows to fill the row
 * and the app column gives up its width rather than the panel lifting out.
 */
export function CrewPanel({
  open,
  chatId,
}: {
  open: boolean
  chatId: string
}) {
  const files = useCrewFiles(chatId)
  const selectedPath = useSelectedCrewPath(chatId)
  const [maximized, setMaximized] = useState(false)
  const isMobile = useIsMobile()
  const sheetRef = useRef<HTMLElement>(null)

  const setSelectedPath = (path: string | null) => {
    setSelectedCrewPath(chatId, path)
  }

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
      if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
      if (Math.abs(dx) <= Math.abs(dy)) {
        gesture.axis = "y"
        gesture.width = 0
        setSlideX(null)
        return
      }
      gesture.axis = "x"
    }

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
    const towards = settled - from
    const flick = performance.now() - startedAt < 250 && Math.abs(towards) > 24

    setCrewOpen(flick ? towards < 0 : settled < width / 2)
    setSlideX(null)
  }

  // Follow the crew folder whether or not the rail is open: a todo that lands
  // while it is shut is exactly the reason it should open. The stream is per
  // chat, so switching conversations starts a new one.
  useEffect(() => {
    if (!chatId) return
    return watchCrew(chatId)
  }, [chatId])

  function closeDetail() {
    setSelectedPath(null)
    setMaximized(false)
  }

  const selected = files.find((file) => file.path === selectedPath) ?? null

  return (
    <>
      <div
        aria-hidden="true"
        onClick={() => setCrewOpen(false)}
        className={cn(
          "hidden max-md:fixed max-md:inset-0 max-md:z-30 max-md:block max-md:bg-black/60 max-md:transition-opacity max-md:duration-300",
          open ? "max-md:opacity-100" : "max-md:pointer-events-none max-md:opacity-0"
        )}
      />

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
            : { translate: `${slideX}px`, transition: "none" }
        }
        className={cn(
          "flex h-full shrink-0 items-center overflow-hidden transition-[width] duration-300 ease-out",
          "max-md:fixed max-md:inset-y-0 max-md:right-0 max-md:z-40 max-md:w-[86%] max-md:max-w-[22rem] max-md:touch-pan-y max-md:transition-transform",
          open ? (maximized ? "w-full" : "w-[376px]") : "w-0 max-md:translate-x-full"
        )}
      >
        <div
          className={cn(
            "flex h-[calc(100%-2rem)] shrink-0 flex-col overflow-hidden rounded-2xl border border-white/15 bg-black/40 backdrop-blur-xl transition-[width] duration-300 ease-out",
            maximized ? "mx-4 w-[calc(100%-2rem)]" : "mr-4 w-[360px]",
            "max-md:mx-0 max-md:h-full max-md:w-full max-md:rounded-l-2xl max-md:rounded-r-none max-md:border-y-0 max-md:border-r-0"
          )}
        >
          {selected ? (
            <ArtifactDetail
              key={selected.path}
              chatId={chatId}
              file={selected}
              maximized={maximized}
              onToggleMaximize={() => setMaximized((value) => !value)}
              onClose={closeDetail}
            />
          ) : (
            <>
              <header className="flex h-11 shrink-0 items-center gap-2 px-3">
                <HugeiconsIcon
                  icon={ApertureIcon}
                  strokeWidth={2}
                  className="size-4 text-white/50"
                />
                <span className="text-sm font-medium text-white">Crew work</span>
              </header>

              <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto p-3">
                {files.length === 0 ? (
                  <div className="flex h-full items-center justify-center px-6 text-center">
                    <p className="text-xs text-white/40">
                      No crew work yet. The todo the Orchestrator hands out and
                      the report the worker sends back land here.
                    </p>
                  </div>
                ) : (
                  <div className="flex flex-col gap-2">
                    {files.map((file) => (
                      <ArtifactCard
                        key={file.path}
                        file={file}
                        onSelect={() => setSelectedPath(file.path)}
                        onDownload={() =>
                          startDownload(api.artifactFileUrl(chatId, file.path))
                        }
                      />
                    ))}
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      </aside>
    </>
  )
}

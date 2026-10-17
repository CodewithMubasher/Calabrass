import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Download01Icon, Note01Icon } from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { ArtifactCard } from "@/components/artifact-card"
import { ArtifactDetail } from "@/components/artifact-detail"
import { PLACEHOLDER_ARTIFACTS, type Artifact } from "@/lib/artifacts"

/**
 * The Artifacts rail: a right-hand panel that *pushes* the page instead of
 * covering it, which is why it sits beside the routed column in `__root.tsx`
 * rather than inside a page.
 *
 * It stays mounted and animates its own width, so closing is as smooth as
 * opening. The outer element reserves the space — 360px of panel plus its right
 * gutter — while the panel inside keeps its full size and is revealed by that
 * width, so its contents never squash mid-transition. The same applies to the
 * detail's maximize: the rail grows to fill the row and the app column gives up
 * its width, rather than the panel lifting out and covering it.
 *
 * Two views live in the one card: the list, and the artifact opened from it.
 * Dismissing either the rail or the detail returns to the list — and returns
 * the rail to its own width, since maximizing is part of reading a document.
 *
 * It is closed from the file icon in the topbar or the `d` key; the header's
 * download icon is the list's own action for whatever is selected there.
 *
 * The rows in `PLACEHOLDER_ARTIFACTS` are fakes that shape the UI. The real
 * list and the content behind each entry come from the artifacts store next.
 */
export function ArtifactsPanel({ open }: { open: boolean }) {
  const [selected, setSelected] = useState<Artifact | null>(null)
  const [maximized, setMaximized] = useState(false)

  // A closed rail is a dismissed detail: reopening always lands on the list
  // rather than inside whatever was open — and un-maximized, so the rail can
  // never come back covering the app.
  //
  // This tracks `open` during render instead of in an effect, which is what
  // React asks for when state has to follow a prop. Clearing on the way *in*
  // rather than on the way out is deliberate: the detail then stays on screen
  // for the whole exit, instead of flipping to the list while it slides away.
  const [wasOpen, setWasOpen] = useState(open)
  if (wasOpen !== open) {
    setWasOpen(open)
    if (open) {
      setSelected(null)
      setMaximized(false)
    }
  }

  function closeDetail() {
    setSelected(null)
    setMaximized(false)
  }

  return (
    <aside
      aria-label="Artifacts"
      aria-hidden={!open}
      inert={!open}
      className={cn(
        "flex h-full shrink-0 items-center overflow-hidden transition-[width] duration-300 ease-out",
        open ? (maximized ? "w-full" : "w-[376px]") : "w-0"
      )}
    >
      <div
        className={cn(
          "flex h-[calc(100%-2rem)] shrink-0 flex-col overflow-hidden rounded-2xl border border-white/15 bg-black/40 backdrop-blur-xl transition-[width] duration-300 ease-out",
          // 360px of panel beside a 16px gutter; maximized, it takes the row
          // and keeps that gutter on both sides.
          maximized ? "mx-4 w-[calc(100%-2rem)]" : "mr-4 w-[360px]"
        )}
      >
        {selected ? (
          <ArtifactDetail
            artifact={selected}
            maximized={maximized}
            onToggleMaximize={() => setMaximized((value) => !value)}
            onClose={closeDetail}
          />
        ) : (
          <>
            <header className="flex h-11 shrink-0 items-center gap-2 px-3">
              <HugeiconsIcon
                icon={Note01Icon}
                strokeWidth={2}
                className="size-4 text-white/50"
              />
              <span className="text-sm font-medium text-white">Artifacts</span>
              <button
                type="button"
                aria-label="Download"
                className="ml-auto grid size-7 cursor-pointer place-items-center rounded-lg text-white/50 outline-none transition-colors hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-white/20"
              >
                <HugeiconsIcon
                  icon={Download01Icon}
                  strokeWidth={2}
                  className="size-3.5"
                />
              </button>
            </header>

            <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto p-3">
              {PLACEHOLDER_ARTIFACTS.length === 0 ? (
                <div className="flex h-full items-center justify-center">
                  <p className="text-xs text-white/40">Nothing here yet.</p>
                </div>
              ) : (
                <div className="flex flex-col gap-2">
                  {PLACEHOLDER_ARTIFACTS.map((artifact) => (
                    <ArtifactCard
                      key={artifact.id}
                      title={artifact.title}
                      kind={artifact.kind}
                      extension={artifact.extension}
                      onSelect={() => setSelected(artifact)}
                    />
                  ))}
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </aside>
  )
}

import { HugeiconsIcon } from "@hugeicons/react"
import { Download01Icon, Note01Icon } from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { ArtifactCard } from "@/components/artifact-card"

/**
 * Fake rows, here only to shape the list. The real ones come from the artifacts
 * store in phase 2; title, kind and extension are what that API will return.
 */
const PLACEHOLDER_ARTIFACTS = [
  {
    id: "prd",
    title: "Product requirements",
    kind: "Document",
    extension: "PDF",
  },
  { id: "plan", title: "Release checklist", kind: "Plan", extension: "MD" },
  { id: "style", title: "Style reference", kind: "Image", extension: "PNG" },
]

/**
 * The Artifacts rail: a right-hand panel that *pushes* the page instead of
 * covering it, which is why it sits beside the routed column in `__root.tsx`
 * rather than inside a page.
 *
 * It stays mounted and animates its own width, so closing is as smooth as
 * opening. The outer element reserves the space — 360px of panel plus its right
 * gutter — while the panel inside keeps its full size and is revealed by that
 * width, so its contents never squash mid-transition.
 *
 * It is closed from the file icon in the topbar or the `d` key; the header's
 * right-hand icon is the download action for whatever is selected.
 *
 * This is the frame plus a fake list: the rows in `PLACEHOLDER_ARTIFACTS` exist
 * only to shape the UI. The real list and the document detail come next.
 */
export function ArtifactsPanel({ open }: { open: boolean }) {
  return (
    <aside
      aria-label="Artifacts"
      aria-hidden={!open}
      inert={!open}
      className={cn(
        "flex h-full shrink-0 items-center overflow-hidden transition-[width] duration-300 ease-out",
        open ? "w-[376px]" : "w-0"
      )}
    >
      <div className="mr-4 flex h-[calc(100%-2rem)] w-[360px] shrink-0 flex-col overflow-hidden rounded-2xl border border-white/15 bg-black/40 backdrop-blur-xl">
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
                />
              ))}
            </div>
          )}
        </div>
      </div>
    </aside>
  )
}

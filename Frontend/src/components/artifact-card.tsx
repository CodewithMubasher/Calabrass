import { HugeiconsIcon } from "@hugeicons/react"
import { Download01Icon, File01Icon } from "@hugeicons/core-free-icons"

import { ARTIFACT_DRAG_TYPE, encodeArtifactDrag } from "@/lib/artifact-drag"
import { kindFor, titleFrom, type ArtifactFile } from "@/lib/artifacts"

/**
 * One row in the artifacts rail: a placeholder preview tile, the file's name
 * with a "kind · extension" line under it, and the download action pinned to
 * the right edge.
 *
 * Deliberately quiet — outlined, no shadow, no fill, no gradients — so a list of
 * these reads as a list of pages rather than a wall of buttons. The tile's icon
 * is kept low-contrast because it stands in for a real preview.
 *
 * The tile is tucked into the card's base: it is bottom-aligned and pulled down
 * by exactly the card's padding, so it sits flush with the bottom edge with no
 * gap under it, while the text and the download keep the card's own centre. The
 * card's `overflow-hidden` trims the tile's square bottom corners to match the
 * card's rounding.
 *
 * The whole row opens the file, so it is the interactive element and the
 * download sits inside it. A real button cannot nest in a button, hence the
 * role: the row takes the keyboard itself, and the download swallows its own
 * click so the row does not also fire.
 *
 * The row also picks up: it is what carries a file to the composer, and the
 * browser's own drag image is a snapshot of it, so what follows the cursor is
 * the row as it appears here rather than a generic file.
 */
export function ArtifactCard({
  file,
  onSelect,
  onDownload,
}: {
  file: ArtifactFile
  onSelect: () => void
  onDownload: () => void
}) {
  const title = titleFrom(file.name)
  const subtitle = file.extension
    ? `${kindFor(file.extension)} · ${file.extension}`
    : kindFor(file.extension)

  return (
    <div
      role="button"
      tabIndex={0}
      draggable
      onDragStart={(event) => {
        event.dataTransfer.effectAllowed = "copy"
        event.dataTransfer.setData(ARTIFACT_DRAG_TYPE, encodeArtifactDrag(file))
      }}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault()
          onSelect()
        }
      }}
      title={file.path}
      className="group flex cursor-pointer items-center gap-4 overflow-hidden rounded-2xl border border-white/10 p-3.5 outline-none transition-colors hover:border-white/25 focus-visible:ring-2 focus-visible:ring-white/20"
    >
      {/* `-mb-3.5` is the card's `p-3.5`; the two move together. The bottom
          border is dropped so the tile reads as tucked into the card's base. */}
      <div className="grid size-14 shrink-0 -mb-3.5 self-end place-items-center rounded-t-xl border border-b-0 border-white/10 transition-colors group-hover:bg-white/5">
        <HugeiconsIcon
          icon={File01Icon}
          strokeWidth={1.5}
          className="size-5 text-white/35"
        />
      </div>

      <div className="min-w-0 flex-1 font-outfit">
        <p className="truncate text-sm font-medium text-white">{title}</p>
        <p className="truncate text-xs text-white/50">{subtitle}</p>
      </div>

      <button
        type="button"
        aria-label={`Download ${file.name}`}
        onClick={(event) => {
          // The click stops here either way, so the row's open handler does not
          // fire behind it.
          event.stopPropagation()
          onDownload()
        }}
        className="grid size-9 shrink-0 cursor-pointer place-items-center rounded-full text-white/50 outline-none transition-colors hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-white/20"
      >
        <HugeiconsIcon
          icon={Download01Icon}
          strokeWidth={1.5}
          className="size-5"
        />
      </button>
    </div>
  )
}

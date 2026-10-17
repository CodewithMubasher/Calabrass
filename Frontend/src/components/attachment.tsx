import { HugeiconsIcon } from "@hugeicons/react"
import { Cancel01Icon } from "@hugeicons/core-free-icons"

import type { AttachmentFile } from "@/lib/attachments"

/** Bytes, in the shortest form that is still honest about the size. */
function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

/** The muted line under the name: how long the file is, or how big. */
function metadataOf(file: AttachmentFile) {
  if (file.lines === undefined) return formatSize(file.size)
  return `${file.lines} ${file.lines === 1 ? "line" : "lines"}`
}

/**
 * One attached file in the composer: a flat card that says what the file is
 * called, how long it is, and what kind it is, with a way out in the corner.
 *
 * The close button sits on the card's edge rather than inside it, so the card
 * itself can stay even — the same padding on all four sides, the name, the
 * length, then the type tag with room of its own above it.
 */
export function Attachment({
  file,
  onRemove,
}: {
  file: AttachmentFile
  onRemove: () => void
}) {
  return (
    <div className="group relative w-32" title={file.name}>
      {/* One fixed size for every attachment, so a row of them is a row rather
          than a ragged edge, and the pill always lands in the same place. */}
      <div className="flex h-32 flex-col items-start rounded-xl border border-white/10 bg-white/5 p-3">
        <div className="w-full">
          <p className="w-full truncate text-sm font-semibold text-white">
            {file.name}
          </p>
          <p className="w-full truncate text-[11px] text-white/40">
            {metadataOf(file)}
          </p>
        </div>
        <span className="mt-auto rounded-full border border-white/10 px-2 py-0.5 text-[10px] font-medium tracking-wider text-white/50 uppercase">
          {file.extension || "File"}
        </span>
      </div>

      {/* Out of the way until the card is pointed at, or reached by keyboard. */}
      <button
        type="button"
        aria-label={`Remove ${file.name}`}
        onClick={onRemove}
        className="absolute -top-1.5 -right-1.5 grid size-5 cursor-pointer place-items-center rounded-full border border-white/10 bg-neutral-900 text-white/50 opacity-0 outline-none transition-opacity hover:bg-white/10 hover:text-white focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-white/20 group-hover:opacity-100"
      >
        <HugeiconsIcon
          icon={Cancel01Icon}
          strokeWidth={1.5}
          className="size-3"
        />
      </button>
    </div>
  )
}

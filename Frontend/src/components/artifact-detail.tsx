import { useEffect, useRef, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Cancel01Icon,
  Copy01Icon,
  File01Icon,
  Maximize01Icon,
  Minimize01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { CodeBlock } from "@/components/code-block"
import { MarkdownRenderer } from "@/components/markdown-renderer"
import { languageFor, previewFor, type Artifact } from "@/lib/artifacts"

/**
 * The rail's header buttons: square, quiet, and the same size as the download
 * icon the list header carries, so the two headers read as one surface.
 */
const HEADER_BUTTON =
  "grid size-7 cursor-pointer place-items-center rounded-lg text-white/50 outline-none transition-colors hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-white/20"

/**
 * One artifact, opened. The header takes the place of "Artifacts" — the file's
 * own icon, name and extension, then maximize, copy and close — and the body
 * takes the place of the rows.
 *
 * It returns the header and the body as two siblings rather than wrapping them,
 * because the panel's card is already the flex column that lays them out.
 * Keeping the frame in one place is what stops the two views from drifting.
 */
export function ArtifactDetail({
  artifact,
  maximized,
  onToggleMaximize,
  onClose,
}: {
  artifact: Artifact
  maximized: boolean
  onToggleMaximize: () => void
  onClose: () => void
}) {
  const preview = previewFor(artifact.extension)
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  // Nothing to put on the clipboard when there is no text to show either.
  const canCopy = preview !== "none"

  function handleCopy() {
    navigator.clipboard.writeText(artifact.content)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 2000)
  }

  return (
    <>
      <header className="flex h-11 shrink-0 items-center gap-2 px-3">
        <HugeiconsIcon
          icon={File01Icon}
          strokeWidth={2}
          className="size-4 shrink-0 text-white/50"
        />
        <span className="min-w-0 flex-1 truncate text-sm font-medium text-white">
          {artifact.title}
        </span>
        <span className="shrink-0 rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-medium tracking-wide text-white/50">
          {artifact.extension}
        </span>

        <div className="flex shrink-0 items-center gap-0.5">
          <button
            type="button"
            aria-label={maximized ? "Restore the rail" : "Maximize the rail"}
            aria-pressed={maximized}
            onClick={onToggleMaximize}
            className={HEADER_BUTTON}
          >
            <HugeiconsIcon
              icon={maximized ? Minimize01Icon : Maximize01Icon}
              strokeWidth={2}
              className="size-3.5"
            />
          </button>
          <button
            type="button"
            aria-label="Copy contents"
            onClick={handleCopy}
            disabled={!canCopy}
            className={cn(
              HEADER_BUTTON,
              "disabled:cursor-default disabled:opacity-30 disabled:hover:bg-transparent disabled:hover:text-white/50"
            )}
          >
            <HugeiconsIcon
              icon={copied ? Tick02Icon : Copy01Icon}
              strokeWidth={2}
              className="size-3.5"
            />
          </button>
          <button
            type="button"
            aria-label="Back to the list"
            onClick={onClose}
            className={HEADER_BUTTON}
          >
            <HugeiconsIcon
              icon={Cancel01Icon}
              strokeWidth={2}
              className="size-3.5"
            />
          </button>
        </div>
      </header>

      <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto p-5">
        {preview === "none" ? (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
            <div className="grid size-12 place-items-center rounded-xl border border-white/10">
              <HugeiconsIcon
                icon={File01Icon}
                strokeWidth={1.5}
                className="size-5 text-white/35"
              />
            </div>
            <p className="text-xs text-white/40">
              No preview for {artifact.extension} files yet.
            </p>
          </div>
        ) : preview === "code" ? (
          // The measure is capped for the same reason the reply caps it: a code
          // file read at full screen is a line nobody can follow.
          <div className="mx-auto w-full max-w-[42em]">
            <CodeBlock language={languageFor(artifact.extension)}>
              {artifact.content}
            </CodeBlock>
          </div>
        ) : (
          // Same container the reply uses, so a document here is typeset by the
          // same stylesheet, at the same size and measure.
          <div className="typeset typeset-chat mx-auto w-full max-w-[42em]">
            <MarkdownRenderer content={artifact.content} />
          </div>
        )}
      </div>
    </>
  )
}

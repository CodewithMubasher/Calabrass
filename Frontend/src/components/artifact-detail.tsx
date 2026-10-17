import { useEffect, useRef, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Cancel01Icon,
  Copy01Icon,
  Download01Icon,
  File01Icon,
  Maximize01Icon,
  Minimize01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { CodeBlock } from "@/components/code-block"
import { MarkdownRenderer } from "@/components/markdown-renderer"
import { api, startDownload } from "@/lib/api"
import {
  directoryOf,
  languageFor,
  previewFor,
  titleFrom,
  type ArtifactFile,
} from "@/lib/artifacts"

/**
 * The rail's header buttons: square, quiet, and the same size as the download
 * icon the list header carries, so the two headers read as one surface.
 */
const HEADER_BUTTON =
  "grid size-7 cursor-pointer place-items-center rounded-lg text-white/50 outline-none transition-colors hover:bg-white/10 hover:text-white focus-visible:ring-2 focus-visible:ring-white/20"

/**
 * One artifact, opened. The header takes the place of "Artifacts" — the file's
 * icon, folder, name and extension, then maximize, download, copy and close —
 * and the body takes the place of the rows.
 *
 * It returns the header and the body as two siblings rather than wrapping them,
 * because the panel's card is already the flex column that lays them out.
 * Keeping the frame in one place is what stops the two views from drifting.
 *
 * The file is read from disk, not carried in the listing: `modified` is part of
 * the key, so a file that changes while it is open is read again and the view
 * follows the agent as it writes. The panel mounts one of these per path, which
 * is what gives each file a clean slate.
 */
export function ArtifactDetail({
  chatId,
  file,
  maximized,
  onToggleMaximize,
  onClose,
}: {
  chatId: string
  file: ArtifactFile
  maximized: boolean
  onToggleMaximize: () => void
  onClose: () => void
}) {
  const preview = previewFor(file.extension)
  const [content, setContent] = useState("")
  const [loaded, setLoaded] = useState(false)
  const [binary, setBinary] = useState(false)
  const [truncated, setTruncated] = useState(false)
  const [failed, setFailed] = useState(false)
  const [copied, setCopied] = useState(false)
  const copyTimer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)

  useEffect(() => {
    return () => clearTimeout(copyTimer.current)
  }, [])

  useEffect(() => {
    let cancelled = false

    api
      .readArtifact(chatId, file.path)
      .then((result) => {
        if (cancelled) return
        setContent(result.content)
        setBinary(result.binary)
        setTruncated(result.truncated)
        setFailed(false)
        setLoaded(true)
      })
      .catch(() => {
        if (cancelled) return
        setFailed(true)
        setLoaded(true)
      })

    return () => {
      cancelled = true
    }
  }, [chatId, file.path, file.modified])

  // A file we cannot read as text has nothing to put on the clipboard either.
  const canCopy = loaded && !failed && !binary && preview !== "none"

  function handleCopy() {
    navigator.clipboard.writeText(content)
    setCopied(true)
    clearTimeout(copyTimer.current)
    copyTimer.current = setTimeout(() => setCopied(false), 2000)
  }

  const directory = directoryOf(file)
  const readable = loaded && !failed && !binary && preview !== "none"

  return (
    <>
      <header className="flex h-11 shrink-0 items-center gap-2 px-3">
        <HugeiconsIcon
          icon={File01Icon}
          strokeWidth={2}
          className="size-4 shrink-0 text-white/50"
        />
        <p className="flex min-w-0 flex-1 items-baseline gap-1.5" title={file.path}>
          {directory !== "" ? (
            <span className="truncate text-xs text-white/35">{directory}</span>
          ) : null}
          <span className="truncate text-sm font-medium text-white">
            {titleFrom(file.name)}
          </span>
        </p>
        {file.extension !== "" ? (
          <span className="shrink-0 rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-medium tracking-wide text-white/50">
            {file.extension}
          </span>
        ) : null}

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
            aria-label={`Download ${file.name}`}
            onClick={() => startDownload(api.artifactFileUrl(chatId, file.path))}
            className={HEADER_BUTTON}
          >
            <HugeiconsIcon
              icon={Download01Icon}
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
        {failed ? (
          <p className="text-xs text-white/40">
            {file.name} could not be read. It may have been moved or deleted.
          </p>
        ) : !loaded ? null : readable ? (
          // The measure is capped for the same reason the reply caps it: a file
          // read at full screen is a line nobody can follow.
          <div className="mx-auto w-full max-w-[42em]">
            {preview === "code" ? (
              <CodeBlock language={languageFor(file.extension)}>
                {content}
              </CodeBlock>
            ) : (
              // Same container the reply uses, so a document here is typeset by
              // the same stylesheet, at the same size and measure.
              <div className="typeset typeset-chat">
                <MarkdownRenderer content={content} />
              </div>
            )}
            {truncated ? (
              <p className="mt-4 text-xs text-white/40">
                Only the first 32 KB is shown. Download the file for the rest.
              </p>
            ) : null}
          </div>
        ) : (
          <div className="flex h-full flex-col items-center justify-center gap-3 text-center">
            <div className="grid size-12 place-items-center rounded-xl border border-white/10">
              <HugeiconsIcon
                icon={File01Icon}
                strokeWidth={1.5}
                className="size-5 text-white/35"
              />
            </div>
            <p className="text-xs text-white/40">
              No preview for{" "}
              {file.extension === "" ? "this file" : `${file.extension} files`} —
              download it to open it.
            </p>
          </div>
        )}
      </div>
    </>
  )
}

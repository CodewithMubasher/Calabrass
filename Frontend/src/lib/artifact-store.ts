import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { setArtifactsOpen } from "@/lib/panel-store"
import type { ArtifactFile } from "@/lib/artifacts"

/**
 * The workspace listing the rail draws, kept live.
 *
 * The backend owns the truth and pushes a new listing whenever the folder
 * changes — a file the agent writes, edits or deletes arrives as one more
 * snapshot. So there is nothing to poll and nothing to invalidate, and no
 * loading state to design: the rail draws the last listing it was given, and
 * the connection's own first snapshot replaces it a moment after it opens.
 */
interface ArtifactState {
  /** The chat the listing belongs to; empty until the first snapshot lands. */
  chatId: string
  files: ArtifactFile[]
}

let state: ArtifactState = { chatId: "", files: [] }

/**
 * What a caller sees while the listing for another chat is still on its way, so
 * the rail never shows one workspace's files under another workspace's name.
 */
const NO_FILES: ArtifactFile[] = []

const listeners = new Set<() => void>()

/** Previous file paths per chatId to detect new files. */
const previousFiles = new Map<string, Set<string>>()

function emit(next: ArtifactState) {
  state = next
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return state
}

/** The files of one workspace, as they were the last time the server spoke. */
export function useArtifactFiles(chatId: string): ArtifactFile[] {
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return snapshot.chatId === chatId ? snapshot.files : NO_FILES
}

/**
 * Follows one workspace until the returned function is called. Every snapshot
 * the server sends replaces the last one, which is the only way this store ever
 * changes — so a file appearing, being written again or being deleted is on
 * screen without anyone asking for it.
 */
export function watchArtifacts(chatId: string) {
  const source = new EventSource(api.artifactStreamUrl(chatId))

  source.addEventListener("artifacts", (event) => {
    const payload = JSON.parse((event as MessageEvent<string>).data) as {
      files?: ArtifactFile[]
    }
    const files = payload.files ?? []

    // Detect new files and auto-open panel
    const prevPaths = previousFiles.get(chatId) ?? new Set<string>()
    const newFiles = files.filter((f) => !prevPaths.has(f.path))
    previousFiles.set(chatId, new Set(files.map((f) => f.path)))

    // Auto-open artifact panel if new files appeared and panel is closed
    if (newFiles.length > 0) {
      // Check if panel is currently closed
      const currentState = getSnapshot()
      if (currentState.chatId === chatId) {
        // Panel is for this chat, check if it's open via panel-store
        // We'll trigger open regardless - panel-store will handle idempotency
        setArtifactsOpen(true)
      }
    }

    emit({ chatId, files })
  })

  return () => source.close()
}

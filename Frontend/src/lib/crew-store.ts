import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { setCrewOpen } from "@/lib/panel-store"
import type { ArtifactFile } from "@/lib/artifacts"

/**
 * The crew folder the rail draws, kept live.
 *
 * The backend owns the truth: .crew holds one todo per delegation and one report
 * per worker, and it pushes a new listing the moment either is written. So the
 * panel can open itself on the file that just appeared rather than making the
 * reader go looking for it.
 *
 * The first snapshot is a baseline, not news. A chat opened after a run has
 * finished already holds its files, and opening the rail unasked for them would
 * turn "closed by default" into "always open". Only a file that appears after
 * that first listing is worth interrupting for.
 */
interface CrewState {
  /** The chat the listing belongs to; empty until the first snapshot lands. */
  chatId: string
  files: ArtifactFile[]
  /** The file open in each chat's detail view. */
  selectedPaths: Record<string, string>
}

let state: CrewState = { chatId: "", files: [], selectedPaths: {} }

/** What a caller sees while another chat's listing is still on its way. */
const NO_FILES: ArtifactFile[] = []

const listeners = new Set<() => void>()

/** The file paths each chat had on the previous snapshot, with their stamps. */
const previousFiles = new Map<string, Map<string, string>>()

/** What counts as "the same file" across snapshots: its size and mtime. */
function fileStamp(file: ArtifactFile): string {
  return `${file.size}:${file.modified}`
}

function emit(next: Partial<CrewState>) {
  state = { ...state, ...next }
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

/** The crew files of one workspace, as they were the last time the server spoke. */
export function useCrewFiles(chatId: string): ArtifactFile[] {
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return snapshot.chatId === chatId ? snapshot.files : NO_FILES
}

/** The file currently open in the crew rail for a chat. */
export function useSelectedCrewPath(chatId: string): string | null {
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  return snapshot.selectedPaths[chatId] ?? null
}

/** Open one file in the crew rail, or close the detail with null. */
export function setSelectedCrewPath(chatId: string, path: string | null) {
  state = {
    ...state,
    selectedPaths: { ...state.selectedPaths, [chatId]: path ?? "" }
  }
  for (const listener of listeners) listener()
}

/**
 * Follows one workspace's crew folder until the returned function is called.
 * A todo or a report that appears after the baseline opens the rail on itself,
 * so the work is visible without the reader having to know it exists.
 */
export function watchCrew(chatId: string) {
  const source = new EventSource(api.crewStreamUrl(chatId))

  source.addEventListener("crew", (event) => {
    const payload = JSON.parse((event as MessageEvent<string>).data) as {
      files?: ArtifactFile[]
    }
    const files = payload.files ?? []

    const previous = previousFiles.get(chatId)
    previousFiles.set(
      chatId,
      new Map(files.map((file) => [file.path, fileStamp(file)]))
    )

    // The first listing is where this chat starts, not something that just
    // happened. Everything after it is news — including a todo or a report the
    // same worker overwrote on a later delegation, which is why the comparison
    // is by stamp and not just by path.
    if (previous !== undefined) {
      const appeared = files.filter(
        (file) => previous.get(file.path) !== fileStamp(file)
      )
      if (appeared.length > 0) {
        const newest = appeared.reduce((latest, file) =>
          file.modified > latest.modified ? file : latest
        )
        setCrewOpen(true)
        setSelectedCrewPath(chatId, newest.path)
      }
    }

    emit({ chatId, files })
  })

  return () => source.close()
}

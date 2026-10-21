import { useSyncExternalStore } from "react"

import type { ChatMode } from "@/types/chat"

/**
 * The mode the composer is in, shared by more than one tree.
 *
 * The composer's mode switch lives inside the composer, but the topbar's
 * aperture icon and the home screen both need to reach it: the icon hands the
 * next message to the crew, and the home screen's composer is the one it is
 * talking about. They are different components, so the mode lives here.
 */
let mode: ChatMode = "chat"

const listeners = new Set<() => void>()

function setMode(next: ChatMode) {
  if (next === mode) return
  mode = next
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return mode
}

/** The mode the composer is in. */
export function useComposerMode() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/** Put the composer in a mode. */
export function setComposerMode(next: ChatMode) {
  setMode(next)
}

/** Hand the next message to the crew, or bring it back to an ordinary chat. */
export function toggleComposerMode() {
  setMode(mode === "crew" ? "chat" : "crew")
}

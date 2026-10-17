import { useSyncExternalStore } from "react"

/**
 * Panel visibility that more than one tree needs to reach: the artifacts rail
 * is opened from the conversation's own toggle, from the file icon in the
 * topbar, and from a key, and those three live in different components.
 */
interface PanelState {
  artifactsOpen: boolean
}

let state: PanelState = { artifactsOpen: false }

const listeners = new Set<() => void>()

function setState(patch: Partial<PanelState>) {
  state = { ...state, ...patch }
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

export function usePanelState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export function toggleArtifacts() {
  setState({ artifactsOpen: !state.artifactsOpen })
}

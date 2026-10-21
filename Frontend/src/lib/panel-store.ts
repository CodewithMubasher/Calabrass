import { useSyncExternalStore } from "react"

/**
 * Panel visibility that more than one tree needs to reach: the artifacts rail
 * is opened from the conversation's own toggle, from the file icon in the
 * topbar, and from a key, and those three live in different components.
 *
 * The crew rail is the same kind of thing: it is opened from the aperture icon
 * in the topbar and from the `c` key.
 */
interface PanelState {
  artifactsOpen: boolean
  crewOpen: boolean
}

let state: PanelState = { artifactsOpen: false, crewOpen: false }

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

/** Put the rail away or bring it back, for callers that know which they mean —
 *  a swipe lands on one of the two rather than on the other. */
export function setArtifactsOpen(open: boolean) {
  setState({ artifactsOpen: open })
}

export function toggleCrew() {
  setState({ crewOpen: !state.crewOpen })
}

/** Put the crew rail away or bring it back. */
export function setCrewOpen(open: boolean) {
  setState({ crewOpen: open })
}

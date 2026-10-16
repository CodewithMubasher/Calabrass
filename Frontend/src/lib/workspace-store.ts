import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import type { WorkspaceInput, WorkspaceRow } from "@/types/chat"

interface WorkspaceState {
  workspaces: WorkspaceRow[]
  loading: boolean
  error: string | null
  /** Workspace ids with a change in flight. */
  pending: string[]
}

let state: WorkspaceState = {
  workspaces: [],
  loading: false,
  error: null,
  pending: [],
}

const listeners = new Set<() => void>()

function setState(patch: Partial<WorkspaceState>) {
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

/** Subscribe to the workspace list and its changes. */
export function useWorkspacesState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

function reason(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

let inFlight = false

/** Fetch the folders the agent may work in. */
export async function loadWorkspaces() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true, error: null })

  try {
    setState({ workspaces: await api.listWorkspaces(), loading: false })
  } catch (err) {
    setState({
      loading: false,
      error: reason(err, "Could not load workspaces."),
    })
  } finally {
    inFlight = false
  }
}

/** Dismiss the error banner on the workspaces page. */
export function clearWorkspacesError() {
  setState({ error: null })
}

/**
 * Add a workspace. The server also makes the folder, so the workspace is
 * usable the moment it comes back. Throws with the server's message so the
 * dialog can show it next to the form.
 */
export async function createWorkspace(input: WorkspaceInput) {
  setState({ workspaces: await api.createWorkspace(input), error: null })
}

/** Forget a workspace. Its files stay on disk. Throws on failure. */
export async function deleteWorkspace(workspaceId: string) {
  if (state.pending.includes(workspaceId)) return
  setState({ pending: [...state.pending, workspaceId], error: null })

  try {
    setState({ workspaces: await api.deleteWorkspace(workspaceId) })
  } finally {
    setState({ pending: state.pending.filter((id) => id !== workspaceId) })
  }
}

import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { clearNotice, notifyError, notifySuccess, reasonFor } from "@/lib/toast"
import type { WorkspaceInput, WorkspaceRow } from "@/types/chat"

interface WorkspaceState {
  workspaces: WorkspaceRow[]
  loading: boolean
  /** Workspace ids with a change in flight. */
  pending: string[]
}

let state: WorkspaceState = {
  workspaces: [],
  loading: false,
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

let inFlight = false

/** Fetch the folders the agent may work in. */
export async function loadWorkspaces() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true })

  try {
    setState({ workspaces: await api.listWorkspaces(), loading: false })
    clearNotice("workspaces-load")
  } catch (err) {
    setState({ loading: false })
    notifyError(
      reasonFor(err, "Could not load workspaces."),
      "Could not load workspaces",
      "workspaces-load"
    )
  } finally {
    inFlight = false
  }
}

/**
 * Add a workspace. The server also makes the folder, so the workspace is
 * usable the moment it comes back. Throws with the server's message so the
 * dialog can show it next to the form.
 */
export async function createWorkspace(input: WorkspaceInput) {
  setState({ workspaces: await api.createWorkspace(input) })
  notifySuccess(`${input.name} was added.`, "Workspace added")
}

/** Forget a workspace. Its files stay on disk. Throws on failure. */
export async function deleteWorkspace(workspaceId: string) {
  if (state.pending.includes(workspaceId)) return
  const name =
    state.workspaces.find((workspace) => workspace.id === workspaceId)?.name ??
    "The workspace"
  setState({ pending: [...state.pending, workspaceId] })

  try {
    setState({ workspaces: await api.deleteWorkspace(workspaceId) })
    notifySuccess(`${name} was removed.`, "Workspace removed")
  } finally {
    setState({ pending: state.pending.filter((id) => id !== workspaceId) })
  }
}

import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { clearNotice, notifyError, notifySuccess, reasonFor } from "@/lib/toast"
import type { WorkspaceInput, WorkspaceRow } from "@/types/chat"

interface WorkspaceState {
  workspaces: WorkspaceRow[]
  loading: boolean
  /** Workspace ids with a change in flight. */
  pending: string[]
  /** The folder a chat started from the home screen will use. */
  preferredDir: string
}

/** Keeps the home-screen pick across a reload until the person changes it. */
const PREFERRED_DIR_KEY = "calabrass.workspaceDir"

function storedPreferredDir() {
  try {
    return localStorage.getItem(PREFERRED_DIR_KEY) ?? ""
  } catch {
    return ""
  }
}

let state: WorkspaceState = {
  workspaces: [],
  loading: false,
  pending: [],
  preferredDir: storedPreferredDir(),
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

/** The folder the next chat from the home screen should use. */
export function usePreferredWorkspaceDir() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).preferredDir
}

/** Pick the folder the next chat from the home screen will use. */
export function setPreferredWorkspaceDir(dir: string) {
  try {
    localStorage.setItem(PREFERRED_DIR_KEY, dir)
  } catch {
    // A blocked storage does not stop the pick from working in this run.
  }
  setState({ preferredDir: dir })
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
  const workspaces = await api.createWorkspace(input)
  setState({ workspaces })
  notifySuccess(`${input.name} was added.`, "Workspace added")

  const name = input.name.trim().toLowerCase()
  return (
    workspaces.find(
      (workspace) => workspace.name.trim().toLowerCase() === name
    ) ?? workspaces[workspaces.length - 1]
  )
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

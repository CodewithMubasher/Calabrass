import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { clearNotice, notifyError, notifySuccess, reasonFor } from "@/lib/toast"
import type { AgentInput, AgentRow } from "@/types/chat"

interface AgentState {
  agents: AgentRow[]
  loading: boolean
}

let state: AgentState = {
  agents: [],
  loading: false,
}

const listeners = new Set<() => void>()

function setState(patch: Partial<AgentState>) {
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

/** Subscribe to the agent list and its changes. */
export function useAgentsState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

let inFlight = false

/** Fetch the sub-agents this machine has saved. */
export async function loadAgents() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true })

  try {
    setState({ agents: await api.listAgents(), loading: false })
    clearNotice("agents-load")
  } catch (err) {
    setState({ loading: false })
    notifyError(
      reasonFor(err, "Could not load agents."),
      "Could not load agents",
      "agents-load"
    )
  } finally {
    inFlight = false
  }
}

/**
 * Add an agent. Throws with the server's message so the dialog can show it next
 * to the form.
 */
export async function createAgent(input: AgentInput) {
  setState({ agents: await api.createAgent(input) })
  notifySuccess(`${input.name} was added.`, "Agent added")
}

/** Save an agent in full. Throws on failure. */
export async function saveAgent(agentId: string, input: AgentInput) {
  setState({ agents: await api.saveAgent(agentId, input) })
  notifySuccess(`${input.name} was saved.`, "Agent saved")
}

/** Remove an agent. Throws on failure. */
export async function deleteAgent(agentId: string) {
  const name =
    state.agents.find((agent) => agent.id === agentId)?.name ?? "The agent"
  setState({ agents: await api.deleteAgent(agentId) })
  notifySuccess(`${name} was removed.`, "Agent removed")
}

/** Reset a built-in agent to its shipped form. Throws on failure. */
export async function resetAgent(agentId: string) {
  const name =
    state.agents.find((agent) => agent.id === agentId)?.name ?? "The agent"
  setState({ agents: await api.resetAgent(agentId) })
  notifySuccess(`${name} was reset to default.`, "Agent reset")
}

import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import type { AgentInput, AgentRow } from "@/types/chat"

interface AgentState {
  agents: AgentRow[]
  loading: boolean
  error: string | null
}

let state: AgentState = {
  agents: [],
  loading: false,
  error: null,
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

function reason(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

let inFlight = false

/** Fetch the sub-agents this machine has saved. */
export async function loadAgents() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true, error: null })

  try {
    setState({ agents: await api.listAgents(), loading: false })
  } catch (err) {
    setState({
      loading: false,
      error: reason(err, "Could not load agents."),
    })
  } finally {
    inFlight = false
  }
}

/** Dismiss the error banner on the agents page. */
export function clearAgentsError() {
  setState({ error: null })
}

/**
 * Add an agent. Throws with the server's message so the dialog can show it next
 * to the form.
 */
export async function createAgent(input: AgentInput) {
  setState({ agents: await api.createAgent(input), error: null })
}

/** Save an agent in full. Throws on failure. */
export async function saveAgent(agentId: string, input: AgentInput) {
  setState({ agents: await api.saveAgent(agentId, input), error: null })
}

/** Remove an agent. Throws on failure. */
export async function deleteAgent(agentId: string) {
  setState({ agents: await api.deleteAgent(agentId), error: null })
}

/** Fetch a single agent by ID. */
export async function getAgent(agentId: string): Promise<AgentRow> {
  return api.getAgent(agentId)
}

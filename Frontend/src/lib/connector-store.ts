import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import type { ConnectorInput, ConnectorRow } from "@/types/chat"

interface ConnectorState {
  connectors: ConnectorRow[]
  loading: boolean
  error: string | null
  /** Connector ids with a change in flight. */
  pending: string[]
}

let state: ConnectorState = {
  connectors: [],
  loading: false,
  error: null,
  pending: [],
}

const listeners = new Set<() => void>()

function setState(patch: Partial<ConnectorState>) {
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

/** Subscribe to the connector list and its changes. */
export function useConnectorsState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

function reason(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

let inFlight = false

/** Fetch the MCP servers this machine has registered. */
export async function loadConnectors() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true, error: null })

  try {
    setState({ connectors: await api.listConnectors(), loading: false })
  } catch (err) {
    setState({
      loading: false,
      error: reason(err, "Could not load connectors."),
    })
  } finally {
    inFlight = false
  }
}

/** Dismiss the error banner on the connectors page. */
export function clearConnectorsError() {
  setState({ error: null })
}

/**
 * Register an MCP server. Throws with the server's message so the dialog can
 * show it next to the form.
 */
export async function createConnector(input: ConnectorInput) {
  setState({ connectors: await api.createConnector(input), error: null })
}

/** Turn a connector on or off. Throws on failure. */
export async function setConnectorEnabled(
  connectorId: string,
  enabled: boolean
) {
  if (state.pending.includes(connectorId)) return

  const previous = state.connectors
  setState({
    connectors: previous.map((c) =>
      c.id === connectorId ? { ...c, enabled } : c
    ),
    pending: [...state.pending, connectorId],
    error: null,
  })

  try {
    setState({ connectors: await api.setConnectorEnabled(connectorId, enabled) })
  } catch (err) {
    setState({
      connectors: previous,
      error: reason(err, "Could not update the connector."),
    })
  } finally {
    setState({ pending: state.pending.filter((id) => id !== connectorId) })
  }
}

/** Remove a connector. Throws on failure. */
export async function deleteConnector(connectorId: string) {
  setState({ connectors: await api.deleteConnector(connectorId), error: null })
}

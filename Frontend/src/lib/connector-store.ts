import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { clearNotice, notifyError, notifySuccess, reasonFor } from "@/lib/toast"
import type { ConnectorInput, ConnectorRow } from "@/types/chat"

interface ConnectorState {
  connectors: ConnectorRow[]
  loading: boolean
  /** Connector ids with a change in flight. */
  pending: string[]
}

let state: ConnectorState = {
  connectors: [],
  loading: false,
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

let inFlight = false

/** Fetch the MCP servers this machine has registered. */
export async function loadConnectors() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true })

  try {
    setState({ connectors: await api.listConnectors(), loading: false })
    clearNotice("connectors-load")
  } catch (err) {
    setState({ loading: false })
    notifyError(
      reasonFor(err, "Could not load connectors."),
      "Could not load connectors",
      "connectors-load"
    )
  } finally {
    inFlight = false
  }
}

/**
 * Register an MCP server. Throws with the server's message so the dialog can
 * show it next to the form.
 */
export async function createConnector(input: ConnectorInput) {
  setState({ connectors: await api.createConnector(input) })
  notifySuccess(`${input.name} was added.`, "Connector added")
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
  })

  try {
    setState({ connectors: await api.setConnectorEnabled(connectorId, enabled) })
    clearNotice("connector-update")
  } catch (err) {
    setState({ connectors: previous })
    notifyError(
      reasonFor(err, "Could not update the connector."),
      "Could not update the connector",
      "connector-update"
    )
  } finally {
    setState({ pending: state.pending.filter((id) => id !== connectorId) })
  }
}

/** Remove a connector. Throws on failure. */
export async function deleteConnector(connectorId: string) {
  const name =
    state.connectors.find((connector) => connector.id === connectorId)?.name ??
    "The connector"
  setState({ connectors: await api.deleteConnector(connectorId) })
  notifySuccess(`${name} was removed.`, "Connector removed")
}

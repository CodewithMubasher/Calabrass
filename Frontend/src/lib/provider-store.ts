import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { loadModels } from "@/lib/model-store"
import type { ProviderInput, ProviderRow } from "@/types/chat"

interface ProviderState {
  providers: ProviderRow[]
  loading: boolean
  error: string | null
  /** Provider ids with a toggle in flight. */
  pending: string[]
}

let state: ProviderState = {
  providers: [],
  loading: false,
  error: null,
  pending: [],
}

const listeners = new Set<() => void>()

function setState(patch: Partial<ProviderState>) {
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

/** Subscribe to the provider list and its toggles. */
export function useProvidersState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

function reason(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

let inFlight = false

/** Fetch the providers the backend can serve. */
export async function loadProviders() {
  if (inFlight) return
  inFlight = true
  setState({ loading: true, error: null })

  try {
    setState({ providers: await api.listProviders(), loading: false })
  } catch (err) {
    setState({
      loading: false,
      error: reason(err, "Could not load providers."),
    })
  } finally {
    inFlight = false
  }
}

/**
 * Turn a provider on or off. The switch flips straight away, then the server's
 * answer replaces the optimistic row. Models are reloaded either way, because
 * turning a provider off has to drop it from the composer's model menu.
 */
export async function setProviderEnabled(providerID: string, enabled: boolean) {
  if (state.pending.includes(providerID)) return

  const previous = state.providers
  setState({
    providers: previous.map((p) =>
      p.id === providerID ? { ...p, enabled } : p
    ),
    pending: [...state.pending, providerID],
    error: null,
  })

  try {
    setState({ providers: await api.updateProvider(providerID, { enabled }) })
  } catch (err) {
    setState({
      providers: previous,
      error: reason(err, "Could not update the provider."),
    })
  } finally {
    setState({ pending: state.pending.filter((id) => id !== providerID) })
    await loadModels()
  }
}

/**
 * Add a custom, OpenAI-compatible provider. Throws with the server's message so
 * the dialog can show it next to the form.
 */
export async function createProvider(input: ProviderInput) {
  setState({ providers: await api.createProvider(input), error: null })
  await loadModels()
}

/** Save edits to a provider. Throws on failure. */
export async function saveProvider(providerID: string, input: ProviderInput) {
  setState({ providers: await api.updateProvider(providerID, input), error: null })
  await loadModels()
}

/** Remove a custom provider. Throws on failure. */
export async function deleteProvider(providerID: string) {
  setState({ providers: await api.deleteProvider(providerID), error: null })
  await loadModels()
}

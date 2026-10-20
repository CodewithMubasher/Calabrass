import { useSyncExternalStore } from "react"

import { api, ApiError } from "@/lib/api"
import { loadModels } from "@/lib/model-store"
import { clearNotice, notifyError, notifySuccess, reasonFor } from "@/lib/toast"
import type { ProviderInput, ProviderRow } from "@/types/chat"

interface ProviderState {
  providers: ProviderRow[]
  loading: boolean
  /** Provider ids with a toggle in flight. */
  pending: string[]
}

let state: ProviderState = {
  providers: [],
  loading: false,
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

function is401(err: unknown): err is ApiError {
  return err instanceof ApiError && err.status === 401
}

let inFlight = false

/** Fetch the providers the backend can serve. */
export async function loadProviders(force = false) {
  if (!force && inFlight) return
  inFlight = true
  setState({ loading: true })

  try {
    setState({ providers: await api.listProviders(), loading: false })
    clearNotice("providers-load")
  } catch (err) {
    if (is401(err)) throw err
    setState({ loading: false })
    notifyError(
      reasonFor(err, "Could not load providers."),
      "Could not load providers",
      "providers-load"
    )
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
  })

  try {
    setState({ providers: await api.updateProvider(providerID, { enabled }) })
    clearNotice("provider-update")
  } catch (err) {
    if (is401(err)) throw err
    setState({ providers: previous })
    notifyError(
      reasonFor(err, "Could not update the provider."),
      "Could not update the provider",
      "provider-update"
    )
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
  try {
    setState({ providers: await api.createProvider(input) })
  } catch (err) {
    if (is401(err)) throw err
    throw err
  }
  notifySuccess(`${input.name ?? "The provider"} was added.`, "Provider added")
  await loadModels()
}

/** Save edits to a provider. Throws on failure. */
export async function saveProvider(providerID: string, input: ProviderInput) {
  try {
    setState({ providers: await api.updateProvider(providerID, input) })
  } catch (err) {
    if (is401(err)) throw err
    throw err
  }
  notifySuccess("The provider was saved.", "Provider saved")
  await loadModels()
}

/** Remove a custom provider. Throws on failure. */
export async function deleteProvider(providerID: string) {
  const name =
    state.providers.find((provider) => provider.id === providerID)?.name ??
    "The provider"
  try {
    setState({ providers: await api.deleteProvider(providerID) })
  } catch (err) {
    if (is401(err)) throw err
    throw err
  }
  notifySuccess(`${name} was removed.`, "Provider removed")
  await loadModels()
}

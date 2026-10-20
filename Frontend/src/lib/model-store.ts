import { useSyncExternalStore } from "react"

import { api, ApiError } from "@/lib/api"
import { clearNotice, notifyError } from "@/lib/toast"
import type { ModelSelection, ProviderInfo } from "@/types/chat"

const STORAGE_KEY = "calabrass-model"

interface ModelState {
  providers: ProviderInfo[]
  loading: boolean
  selected: ModelSelection | null
}

function loadSelection(): ModelSelection | null {
  if (typeof window === "undefined") return null
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as ModelSelection
    if (parsed?.provider && parsed?.model) return parsed
  } catch {
    // Ignore malformed values.
  }
  return null
}

function persistSelection(selection: ModelSelection) {
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(selection))
  } catch {
    // Ignore quota / private-mode write errors.
  }
}

let state: ModelState = {
  providers: [],
  loading: false,
  selected: loadSelection(),
}

const listeners = new Set<() => void>()

function setState(patch: Partial<ModelState>) {
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

/** Subscribe to the model catalog and the current selection. */
export function useModelState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

export function getSelectedModel(): ModelSelection | null {
  return state.selected
}

export function selectModel(selection: ModelSelection) {
  persistSelection(selection)
  setState({ selected: selection })
}

function isKnownSelection(selection: ModelSelection, providers: ProviderInfo[]) {
  return providers.some(
    (p) =>
      p.id === selection.provider && p.models.some((m) => m.id === selection.model)
  )
}

function pickDefault(providers: ProviderInfo[]): ModelSelection | null {
  for (const provider of providers) {
    if (provider.models.length === 0) continue
    return {
      provider: provider.id,
      model: provider.defaultModel || provider.models[0].id,
    }
  }
  return null
}

/** Fetch the live model catalog from the backend. */
export async function loadModels() {
  if (state.loading) return
  setState({ loading: true })

  try {
    const providers = await api.listModels()
    const keepCurrent =
      state.selected !== null && isKnownSelection(state.selected, providers)
    const selected = keepCurrent ? state.selected : pickDefault(providers)

    if (selected) persistSelection(selected)
    setState({ providers, selected, loading: false })
    clearNotice("models-load")
  } catch (err) {
    // Don't store 401 errors — the global API handler clears the session
    // and redirects to login. Storing the error here races with the redirect
    // and shows "session expired" in the dropdown before navigation.
    if (err instanceof ApiError && err.status === 401) {
      throw err
    }
    setState({ loading: false })
    notifyError(
      err instanceof Error ? err.message : "Could not load models.",
      "Could not load models",
      "models-load"
    )
  }
}

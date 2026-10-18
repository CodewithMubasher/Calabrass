import { useEffect, useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import type { UsageSnapshot } from "@/types/chat"

interface UsageState {
  snapshot: UsageSnapshot | null
  loading: boolean
  error: string | null
}

let state: UsageState = {
  snapshot: null,
  loading: false,
  error: null,
}

const listeners = new Set<() => void>()

function setState(patch: Partial<UsageState>) {
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

function reason(err: unknown) {
  if (err instanceof Error && err.message) return err.message
  return "Could not load usage"
}

/** The last usage snapshot and how the latest refresh went. */
export function useUsageState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

let inFlight: Promise<void> | null = null

/**
 * Ask the backend for a fresh snapshot. Overlapping calls — a poll and a focus
 * event, say — share the one request rather than stacking up.
 */
export function loadUsage(): Promise<void> {
  if (inFlight) return inFlight

  setState({ loading: true })
  inFlight = api
    .getUsage()
    .then((snapshot) => setState({ snapshot, loading: false, error: null }))
    .catch((err: unknown) => setState({ loading: false, error: reason(err) }))
    .finally(() => {
      inFlight = null
    })
  return inFlight
}

export function clearUsageError() {
  setState({ error: null })
}

/** How often an open Usage page asks for new figures. */
const REFRESH_MS = 5_000

/**
 * Keep a snapshot live while a page is watching: once on mount, every few
 * seconds after that, and again whenever the window comes back to the front.
 */
export function useLiveUsage() {
  const snapshot = useUsageState()

  useEffect(() => {
    void loadUsage()

    const timer = window.setInterval(() => {
      if (!document.hidden) void loadUsage()
    }, REFRESH_MS)

    const onFocus = () => {
      void loadUsage()
    }
    window.addEventListener("focus", onFocus)

    return () => {
      window.clearInterval(timer)
      window.removeEventListener("focus", onFocus)
    }
  }, [])

  return snapshot
}

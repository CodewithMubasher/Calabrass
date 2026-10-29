import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { notifyError } from "@/lib/toast"
import type { Approval, ApprovalDecision } from "@/types/chat"

/**
 * What the agent is waiting on a person to allow, kept live.
 *
 * The host owns the truth and pushes the whole list whenever it changes, so a
 * request raised by a crew worker, a voice turn or a turn in another window
 * shows up here too: any face can answer, and the first answer wins.
 */
interface ApprovalState {
  approvals: Approval[]
  /** Ids with an answer in flight, so a double click cannot answer twice. */
  answering: string[]
}

let state: ApprovalState = { approvals: [], answering: [] }
const listeners = new Set<() => void>()

function setState(patch: Partial<ApprovalState>) {
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

export function useApprovals() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/** Follow the waiting approvals while the app is open. Returns the closer. */
export function watchApprovals(): () => void {
  const source = new EventSource(api.approvalStreamUrl())

  source.addEventListener("approvals", (event) => {
    try {
      const payload = JSON.parse((event as MessageEvent).data as string) as {
        approvals?: Approval[]
      }
      setState({ approvals: payload.approvals ?? [] })
    } catch {
      // A frame that cannot be read is not worth dropping the connection over:
      // the next snapshot puts the list right.
    }
  })

  // A transport failure arrives without a payload; EventSource reconnects by
  // itself and the first frame after that is the full list again.
  return () => {
    source.close()
    setState({ approvals: [], answering: [] })
  }
}

/** Answer one approval. The list updates from the stream, not from here. */
export async function decide(id: string, decision: ApprovalDecision) {
  if (state.answering.includes(id)) return
  setState({ answering: [...state.answering, id] })
  try {
    setState({ approvals: await api.decideApproval(id, decision) })
  } catch (err) {
    // Already answered somewhere else, or it timed out: the stream will drop it.
    notifyError(
      err instanceof Error ? err.message : "Your answer could not be sent.",
      "Could not answer",
      "approval-answer"
    )
  } finally {
    setState({ answering: state.answering.filter((a) => a !== id) })
  }
}

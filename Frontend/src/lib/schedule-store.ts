import { useSyncExternalStore } from "react"

import { api } from "@/lib/api"
import { clearNotice, notifyError, notifySuccess, reasonFor } from "@/lib/toast"
import type { ScheduledTask, ScheduledTaskInput, ScheduledTaskPatch } from "@/types/chat"

/**
 * The task list the Schedules page draws, kept live.
 *
 * The backend owns the truth and pushes the whole list whenever it changes, so
 * a task the model booked a moment ago — or one that has just run — appears
 * without a refresh. Between pushes the store is the last snapshot it was given,
 * which is all the page needs: the times it shows are worked out from the rows.
 */
interface ScheduleState {
  tasks: ScheduledTask[]
  /** Task ids with a change in flight. */
  pending: string[]
}

let state: ScheduleState = { tasks: [], pending: [] }

const listeners = new Set<() => void>()

function setState(patch: Partial<ScheduleState>) {
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

/** Subscribe to the task list and its changes. */
export function useSchedulesState() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
}

/**
 * Follow the task list for as long as the page is open. Returns the function
 * that closes the connection, which is what a `useEffect` hands back.
 */
export function watchSchedules(): () => void {
  const source = new EventSource(api.scheduleStreamUrl())

  source.addEventListener("schedules", (event) => {
    try {
      const payload = JSON.parse((event as MessageEvent).data as string) as {
        tasks?: ScheduledTask[]
      }
      setState({ tasks: payload.tasks ?? [] })
      clearNotice("schedules-stream")
    } catch {
      // A frame we cannot read is not worth dropping the connection over: the
      // next snapshot will put the list right.
    }
  })

  // The server reports a failed read as an event; a transport failure arrives
  // without a payload, and EventSource reconnects by itself.
  source.addEventListener("error", (event) => {
    const data = (event as MessageEvent).data
    if (typeof data !== "string" || data === "") return
    try {
      const payload = JSON.parse(data) as { message?: string }
      notifyError(
        payload.message ?? "The task list could not be read.",
        "Could not read the task list",
        "schedules-stream"
      )
    } catch {
      // Ignore it; the next snapshot is the authority.
    }
  })

  return () => source.close()
}

/** Book a task. Throws with the server's message so the dialog can show it. */
export async function createSchedule(input: ScheduledTaskInput) {
  setState({ tasks: await api.createSchedule(input) })
  notifySuccess("The task was booked.", "Task booked")
}

/** Change a task's prompt, its time, or whether it is armed. */
export async function updateSchedule(taskId: string, patch: ScheduledTaskPatch) {
  setState({ tasks: await api.updateSchedule(taskId, patch) })
  notifySuccess("The task was updated.", "Task updated")
}

/** Turn a task on or off, which is the switch on its card. */
export async function setScheduleEnabled(taskId: string, enabled: boolean) {
  if (state.pending.includes(taskId)) return

  const previous = state.tasks
  setState({
    tasks: previous.map((task) =>
      task.id === taskId ? { ...task, enabled } : task
    ),
    pending: [...state.pending, taskId],
  })

  try {
    setState({ tasks: await api.updateSchedule(taskId, { enabled }) })
    clearNotice("schedule-update")
  } catch (err) {
    setState({ tasks: previous })
    notifyError(
      reasonFor(err, "Could not update the task."),
      "Could not update the task",
      "schedule-update"
    )
  } finally {
    setState({ pending: state.pending.filter((id) => id !== taskId) })
  }
}

/** Remove a task. Throws on failure. */
export async function deleteSchedule(taskId: string) {
  setState({ tasks: await api.deleteSchedule(taskId) })
  notifySuccess("The task was removed.", "Task removed")
}

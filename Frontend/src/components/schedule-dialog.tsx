import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { createSchedule, deleteSchedule, updateSchedule } from "@/lib/schedule-store"
import { notifyError } from "@/lib/toast"
import type { ScheduledTask } from "@/types/chat"

// The dialogs sit on the same dark panel as the reference project's, with our
// own popover token standing in for its neutral-900.
const SURFACE = "sm:max-w-md bg-popover border border-white/10"

const FIELD =
  "h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-white/60">{label}</span>
      {children}
    </label>
  )
}

/**
 * Books a task, or edits one that is already booked.
 *
 * The two are the same form: a task is a message and a time, and an edit changes
 * either of them. Mounted only while it is open, so the fields start from the
 * task without an effect.
 */
export function ScheduleDialog({
  task,
  onOpenChange,
}: {
  /** The task being edited; omitted when booking a new one. */
  task?: ScheduledTask
  onOpenChange: (open: boolean) => void
}) {
  const [prompt, setPrompt] = useState(task?.prompt ?? "")
  const [runAt, setRunAt] = useState(() => toInputValue(task?.runAt ?? defaultRunAt()))
  const [confirming, setConfirming] = useState(false)
  const [saving, setSaving] = useState(false)

  const at = fromInputValue(runAt)
  const ready = prompt.trim() !== "" && at > 0

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    setSaving(true)
    try {
      if (task) {
        await updateSchedule(task.id, { prompt: prompt.trim(), runAt: at })
      } else {
        await createSchedule({ prompt: prompt.trim(), runAt: at })
      }
      onOpenChange(false)
    } catch (err) {
      notifyError(
        err instanceof Error ? err.message : "Could not save the task.",
        "Could not save the task"
      )
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    if (!task || saving) return

    setSaving(true)
    try {
      await deleteSchedule(task.id)
      onOpenChange(false)
    } catch (err) {
      notifyError(
        err instanceof Error ? err.message : "Could not remove the task.",
        "Could not remove the task"
      )
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className={SURFACE}>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="text-white/90">
              {task ? "Edit task" : "Schedule a task"}
            </DialogTitle>
          </DialogHeader>

          <div className="py-2">
            <div className="mb-4">
              <Field label="Message">
                <textarea
                  value={prompt}
                  onChange={(event) => setPrompt(event.target.value)}
                  placeholder="Shut the laptop down"
                  autoFocus
                  rows={3}
                  className="w-full resize-none rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white/80 placeholder:text-white/30 focus:border-white/20 outline-none"
                />
              </Field>
              <p className="mt-2 text-xs text-white/30">
                Run in this conversation when the time comes, with every tool.
              </p>
            </div>
            <Field label="Run at">
              <Input
                type="datetime-local"
                value={runAt}
                onChange={(event) => setRunAt(event.target.value)}
                className={FIELD}
              />
            </Field>
          </div>

          <DialogFooter className="gap-2">
            {task ? (
              <Button
                type="button"
                variant={confirming ? "destructive" : "ghost"}
                className={confirming ? "mr-auto" : "mr-auto text-red-400 hover:text-red-300"}
                disabled={saving}
                onClick={() => {
                  // Two steps rather than one: the task is a moment's work to
                  // book again, but there is no undo once it is gone.
                  if (confirming) {
                    void remove()
                  } else {
                    setConfirming(true)
                  }
                }}
              >
                {confirming ? (saving ? "Removing…" : "Really delete?") : "Delete"}
              </Button>
            ) : null}
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={saving || !ready}>
              {saving ? "Saving…" : task ? "Save" : "Schedule"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/** Ten minutes from now, on the minute, which is what a quick task wants. */
function defaultRunAt(): number {
  const at = new Date(Date.now() + 10 * 60_000)
  at.setSeconds(0, 0)
  return at.getTime()
}

/** A moment as a `datetime-local` field reads it. */
function toInputValue(ms: number): string {
  const at = new Date(ms)
  const pad = (value: number) => String(value).padStart(2, "0")
  return `${at.getFullYear()}-${pad(at.getMonth() + 1)}-${pad(at.getDate())}T${pad(at.getHours())}:${pad(at.getMinutes())}`
}

/**
 * The other way round. The field carries no zone, which is exactly right: the
 * scheduler runs on this machine's clock.
 */
function fromInputValue(value: string): number {
  const at = new Date(value).getTime()
  return Number.isNaN(at) ? 0 : at
}

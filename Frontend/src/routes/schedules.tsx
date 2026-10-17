import { useEffect, useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon, TimeScheduleIcon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty, PageShell } from "@/components/page-shell"
import { ScheduleDialog } from "@/components/schedule-dialog"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import {
  clearSchedulesError,
  setScheduleEnabled,
  useSchedulesState,
  watchSchedules,
} from "@/lib/schedule-store"
import type { ScheduledTask } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/schedules",
  component: SchedulesPage,
})

/**
 * The tasks the scheduler will run, as cards. A card says what will be done,
 * when, and how the last run went; clicking one opens it for editing, and the
 * switch on it arms or pauses the task without opening anything.
 */
function SchedulesPage() {
  const { tasks, error, pending } = useSchedulesState()
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<ScheduledTask | null>(null)
  const now = useNow()

  useEffect(watchSchedules, [])

  return (
    <PageShell
      title="Schedules"
      subtitle="Run prompts and tools on a timer"
      error={error}
      onDismissError={clearSchedulesError}
      actions={
        <Button
          variant="default"
          className="gap-2 rounded-full"
          onClick={() => setAdding(true)}
        >
          <HugeiconsIcon icon={Add01Icon} strokeWidth={2} className="size-4" />
          <span>New task</span>
        </Button>
      }
    >
      {tasks.length === 0 ? (
        <PageEmpty
          icon={TimeScheduleIcon}
          title="Nothing scheduled yet"
          hint="Book a message and a time, or just ask the assistant to"
        />
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {tasks.map((task) => (
            <ScheduleCard
              key={task.id}
              task={task}
              now={now}
              pending={pending.includes(task.id)}
              onOpen={() => setEditing(task)}
            />
          ))}
        </div>
      )}

      {adding ? (
        <ScheduleDialog
          onOpenChange={(open) => {
            if (!open) setAdding(false)
          }}
        />
      ) : null}
      {editing ? (
        <ScheduleDialog
          task={editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
        />
      ) : null}
    </PageShell>
  )
}

function ScheduleCard({
  task,
  now,
  pending,
  onOpen,
}: {
  task: ScheduledTask
  now: number
  pending: boolean
  onOpen: () => void
}) {
  const state = stateOf(task)

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault()
          onOpen()
        }
      }}
      className="flex cursor-pointer flex-col rounded-2xl border border-white/10 bg-white/5 p-4 text-left outline-none transition-colors hover:border-white/20 focus-visible:ring-2 focus-visible:ring-white/20"
    >
      <div className="flex items-start justify-between gap-3">
        <h3 className="line-clamp-2 text-sm font-medium text-white/90">
          {task.prompt}
        </h3>
        {/* The switch is inside the card, not inside its click target: turning
            a task off should not also open it. */}
        <span
          className="shrink-0"
          onClick={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
        >
          <Switch
            checked={task.enabled}
            disabled={pending}
            onCheckedChange={(checked) =>
              void setScheduleEnabled(task.id, checked)
            }
            aria-label={task.enabled ? "Pause this task" : "Resume this task"}
          />
        </span>
      </div>

      <div className="mt-auto flex items-center gap-2 pt-3">
        <span
          className={cn(
            "rounded-full px-1.5 py-0.5 text-[10px]",
            state.className
          )}
        >
          {state.label}
        </span>
        <span className="text-[11px] text-white/30">
          {whenText(task, now)}
        </span>
      </div>
    </div>
  )
}

/** How a task reads at a glance, and the colour that says it. */
function stateOf(task: ScheduledTask): { label: string; className: string } {
  if (task.lastStatus === "error") {
    return { label: "Failed", className: "bg-red-500/10 text-red-400" }
  }
  if (task.lastStatus === "missed") {
    return { label: "Missed", className: "bg-amber-500/10 text-amber-400" }
  }
  if (task.lastRunAt > 0) {
    return { label: "Ran", className: "bg-emerald-500/10 text-emerald-400" }
  }
  if (!task.enabled) {
    return { label: "Paused", className: "bg-white/5 text-white/30" }
  }
  return { label: "Scheduled", className: "bg-sky-500/10 text-sky-400" }
}

/** When a task is due, or when it last ran. */
function whenText(task: ScheduledTask, now: number): string {
  if (task.lastRunAt > 0) {
    return `ran ${since(task.lastRunAt, now)}`
  }
  return `${until(task.runAt, now)} · ${clockOf(task.runAt)}`
}

function clockOf(ms: number): string {
  return new Date(ms).toLocaleString(undefined, {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  })
}

function until(ms: number, now: number): string {
  const gap = ms - now
  if (gap <= 0) return "due"
  return `in ${span(gap)}`
}

function since(ms: number, now: number): string {
  const gap = now - ms
  if (gap < 60_000) return "just now"
  return `${span(gap)} ago`
}

function span(ms: number): string {
  const minutes = Math.round(ms / 60_000)
  if (minutes < 1) return "under a minute"
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? "" : "s"}`
  const hours = Math.round(minutes / 60)
  if (hours < 36) return `${hours} hour${hours === 1 ? "" : "s"}`
  const days = Math.round(hours / 24)
  return `${days} day${days === 1 ? "" : "s"}`
}

/**
 * The clock the relative labels are read against, kept ticking: without it
 * "in 10 minutes" would still say so an hour later.
 */
function useNow(intervalMs = 30_000) {
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), intervalMs)
    return () => clearInterval(timer)
  }, [intervalMs])

  return now
}

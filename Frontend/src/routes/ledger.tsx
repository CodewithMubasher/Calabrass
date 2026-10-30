import { useCallback, useEffect, useState } from "react"
import { createRoute } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import { PageShell } from "@/components/page-shell"
import { api } from "@/lib/api"
import { notifyError } from "@/lib/toast"
import type { LedgerEntry } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/ledger",
  component: LedgerPage,
})

type Filter = "all" | "unattended" | "refused"

const FILTERS: { id: Filter; label: string }[] = [
  { id: "all", label: "Everything" },
  { id: "unattended", label: "While I was away" },
  { id: "refused", label: "Refused" },
]

const PAGE = 100

/**
 * The run ledger: what the agent did that needed trust, and how each decision
 * ended. It is the answer to "what happened while I was away?", and it never
 * shows a credential: the host removes them before a row is written.
 */
function LedgerPage() {
  const [filter, setFilter] = useState<Filter>("all")
  const [entries, setEntries] = useState<LedgerEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [more, setMore] = useState(false)

  const load = useCallback(
    async (before?: number) => {
      setLoading(true)
      try {
        const rows = await api.listLedger({
          before,
          limit: PAGE,
          unattended: filter === "unattended",
          refused: filter === "refused",
        })
        setEntries((current) => (before ? [...current, ...rows] : rows))
        setMore(rows.length === PAGE)
      } catch (err) {
        notifyError(
          err instanceof Error ? err.message : "The ledger could not be loaded.",
          "Could not load the ledger",
          "ledger-load"
        )
      } finally {
        setLoading(false)
      }
    },
    [filter]
  )

  useEffect(() => {
    void load()
  }, [load])

  const days = groupByDay(entries)

  return (
    <PageShell
      title="Ledger"
      subtitle="What the agent did that needed your trust, and how each one ended."
      actions={
        <div className="flex gap-1 rounded-full border border-white/10 bg-white/5 p-1">
          {FILTERS.map((f) => (
            <button
              key={f.id}
              type="button"
              onClick={() => setFilter(f.id)}
              className={
                "rounded-full px-3 py-1 text-xs transition-colors " +
                (filter === f.id ? "bg-white/15 text-white" : "text-white/50 hover:text-white/80")
              }
            >
              {f.label}
            </button>
          ))}
        </div>
      }
    >
      {entries.length === 0 && !loading ? (
        <p className="py-16 text-center text-sm text-white/40">
          {filter === "all"
            ? "Nothing yet. When the agent runs a command or a scheduled task runs, it shows up here."
            : "Nothing matches that."}
        </p>
      ) : (
        <div className="grid gap-6">
          {days.map((day) => (
            <section key={day.label}>
              <h2 className="mb-2 text-xs font-medium tracking-wide text-white/35 uppercase">
                {day.label}
              </h2>
              <ul className="divide-y divide-white/5 rounded-2xl border border-white/10 bg-white/[0.03]">
                {day.entries.map((entry) => (
                  <Row key={entry.id} entry={entry} />
                ))}
              </ul>
            </section>
          ))}
          {more ? (
            <div className="flex justify-center">
              <Button
                type="button"
                variant="ghost"
                disabled={loading}
                onClick={() => void load(entries[entries.length - 1]?.at)}
              >
                {loading ? "Loading…" : "Show older"}
              </Button>
            </div>
          ) : null}
        </div>
      )}
    </PageShell>
  )
}

function Row({ entry }: { entry: LedgerEntry }) {
  const tone = outcomeTone(entry.outcome)
  return (
    <li className="flex items-start gap-3 px-4 py-3">
      <time className="w-14 shrink-0 pt-0.5 text-xs text-white/35 tabular-nums">
        {new Date(entry.at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
      </time>
      <div className="min-w-0 flex-1">
        <p className="font-mono text-sm break-words text-white/80">{entry.summary}</p>
        <p className="mt-0.5 text-xs text-white/35">
          {entry.kind === "run" ? "unattended run" : tierName(entry.tier)}
          {entry.unattended && entry.kind !== "run" ? " · nobody was watching" : ""}
        </p>
      </div>
      <span className={"shrink-0 rounded-full px-2.5 py-0.5 text-xs " + tone}>
        {outcomeLabel(entry)}
      </span>
    </li>
  )
}

function tierName(tier: number): string {
  switch (tier) {
    case 2:
      return "ran a command"
    case 3:
      return "reached outside the project"
    case 4:
      return "acted on the outside world"
    default:
      return "action"
  }
}

function outcomeLabel(entry: LedgerEntry): string {
  switch (entry.outcome) {
    case "approved":
      return "you allowed it"
    case "remembered":
      return "allowed (remembered)"
    case "allowed":
      return entry.unattended ? "pre-approved" : "allowed"
    case "refused":
      return "you refused it"
    case "unattended-refused":
      return "refused: not pre-approved"
    case "timed-out":
      return "nobody answered"
    case "completed":
      return "finished"
    case "failed":
      return "failed"
    case "missed":
      return "missed"
    default:
      return entry.outcome
  }
}

function outcomeTone(outcome: string): string {
  switch (outcome) {
    case "refused":
    case "unattended-refused":
    case "timed-out":
    case "failed":
    case "missed":
      return "bg-red-500/15 text-red-300"
    case "approved":
    case "remembered":
    case "allowed":
    case "completed":
      return "bg-emerald-500/15 text-emerald-300"
    default:
      return "bg-white/10 text-white/60"
  }
}

function groupByDay(entries: LedgerEntry[]): { label: string; entries: LedgerEntry[] }[] {
  const groups: { label: string; entries: LedgerEntry[] }[] = []
  const today = new Date().toDateString()
  const yesterday = new Date(Date.now() - 86_400_000).toDateString()
  for (const entry of entries) {
    const when = new Date(entry.at)
    const key = when.toDateString()
    const label =
      key === today
        ? "Today"
        : key === yesterday
          ? "Yesterday"
          : when.toLocaleDateString([], { weekday: "long", month: "long", day: "numeric" })
    const last = groups[groups.length - 1]
    if (last && last.label === label) last.entries.push(entry)
    else groups.push({ label, entries: [entry] })
  }
  return groups
}

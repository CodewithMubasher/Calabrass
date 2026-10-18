import type { ReactNode } from "react"
import { useMemo, useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowDown01Icon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageShell } from "@/components/page-shell"
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { clearUsageError, useLiveUsage } from "@/lib/usage-store"
import type { UsageDay, UsageProvider, UsageTotals } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/usage",
  component: UsagePage,
})

type Option = { value: string; label: string }

const ZERO: UsageTotals = {
  requests: 0,
  inputTokens: 0,
  outputTokens: 0,
  totalTokens: 0,
}

const fullNumber = new Intl.NumberFormat("en-US")

function full(value: number): string {
  return fullNumber.format(value)
}

/** A shortened figure for the small line: "800", "15.9k", "1.2M". */
function compact(value: number): string {
  if (value >= 1_000_000) return `${trim(value / 1_000_000)}M`
  if (value >= 1_000) return `${trim(value / 1_000)}k`
  return full(value)
}

function trim(value: number): string {
  return value >= 10
    ? String(Math.round(value))
    : value.toFixed(1).replace(/\.0$/, "")
}

/**
 * A ring that fills clockwise to the given percentage. It is the progress mark
 * beside each usage row, drawn small enough to sit inline with the text.
 */
function CircularGauge({ percentage }: { percentage: number }) {
  const normalized = Math.min(Math.max(percentage, 0), 100)
  const circumference = 2 * Math.PI * 42.5
  const strokePercent = (normalized / 100) * circumference

  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      width="16"
      strokeWidth="2"
      viewBox="0 0 100 100"
      className="size-4 -rotate-90"
    >
      <circle
        cx="50"
        cy="50"
        r="42.5"
        strokeWidth="12"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="opacity-20"
        stroke="currentColor"
        style={{ strokeDasharray: `${circumference} ${circumference}` }}
      />
      <circle
        cx="50"
        cy="50"
        r="42.5"
        strokeWidth="12"
        strokeLinecap="round"
        strokeLinejoin="round"
        stroke="currentColor"
        className="transition-all duration-300"
        style={{ strokeDasharray: `${strokePercent} ${circumference}` }}
      />
    </svg>
  )
}

/** One panel in the usage grid, on the same translucent surface as the rest. */
function UsageCard({
  className,
  children,
}: {
  className?: string
  children?: ReactNode
}) {
  return (
    <div
      className={cn(
        "flex flex-col rounded-2xl border border-white/10 bg-white/5 p-4",
        className
      )}
    >
      {children}
    </div>
  )
}

/**
 * Tokens used today. The free tiers cap tokens per minute rather than per day,
 * so the small line is the room left in that minute, not a daily balance.
 */
function TokensCard({
  used,
  remaining,
  limited,
}: {
  used: number
  remaining: number
  limited: boolean
}) {
  return (
    <UsageCard className="min-h-20 flex-row items-start justify-between gap-4">
      <div className="min-w-0">
        <h2 className="text-sm font-medium text-white/80">Tokens used</h2>
        <p
          className="mt-1 text-xs text-white/40"
          title={
            limited
              ? "Free-tier token room left in the current minute"
              : undefined
          }
        >
          {limited ? `Remaining: ${compact(remaining)} /min` : "Today"}
        </p>
      </div>
      <span className="shrink-0 text-3xl font-medium whitespace-nowrap text-white/60 tabular-nums">
        {full(used)}
      </span>
    </UsageCard>
  )
}

/** The request count for today, with the day's allowance still available. */
function RequestsCard({
  used,
  remaining,
}: {
  used: number
  remaining: number | null
}) {
  return (
    <UsageCard className="min-h-20 flex-row items-start justify-between gap-4">
      <div className="min-w-0">
        <h2 className="text-sm font-medium text-white/80">Total requests</h2>
        <p className="mt-1 text-xs text-white/40">
          {remaining === null ? "Today" : `Remaining: ${compact(remaining)}`}
        </p>
      </div>
      <span className="shrink-0 text-3xl font-medium whitespace-nowrap text-white/60 tabular-nums">
        {full(used)}
      </span>
    </UsageCard>
  )
}

/** One provider's request row: a gauge, its name, and where it stands. */
function RequestRow({ provider }: { provider: UsageProvider }) {
  const used = provider.requests
  const limit = provider.rpdLimit
  const percentage = limit > 0 ? (used / limit) * 100 : 0

  return (
    <li className="flex items-center gap-3 px-1 py-1">
      <span className="shrink-0 text-primary">
        <CircularGauge percentage={percentage} />
      </span>
      <span className="min-w-0 truncate text-sm text-white/80">
        {provider.name}
      </span>
      <span
        className="ml-auto shrink-0 font-mono text-xs font-medium text-white/40 tabular-nums"
        title={
          limit > 0
            ? `${full(used)} of ${full(limit)} requests used today`
            : "No published free-tier request limit"
        }
      >
        {limit > 0 ? `${full(used)}/${full(limit)} RPD` : `${full(used)} requests`}
      </span>
    </li>
  )
}

/**
 * The request figures for every provider. Every provider is listed in full —
 * the page scrolls as one, so nothing is hidden behind a second scrollbar.
 */
function TotalRequestsCard({
  providers,
  className,
}: {
  providers: UsageProvider[]
  className?: string
}) {
  return (
    <UsageCard className={cn("min-h-40", className)}>
      <h2 className="shrink-0 text-sm font-medium text-white/80">
        Overall usage
      </h2>
      <ul className="mt-3 flex flex-col gap-1">
        {providers.map((provider) => (
          <RequestRow key={provider.id} provider={provider} />
        ))}
      </ul>
    </UsageCard>
  )
}

function weekday(date: string): string {
  const parsed = new Date(`${date}T00:00:00`)
  if (Number.isNaN(parsed.getTime())) return date
  return parsed.toLocaleDateString("en-US", { weekday: "short" })
}

/**
 * A week of daily tokens and requests, as paired bars in the logo colours.
 *
 * The two series are scaled against their own busiest day: real token counts
 * are thousands of times the request counts, and sharing one scale would leave
 * the request bars invisible. The absolute numbers sit in each bar's tooltip.
 */
function WeeklyUsageCard({ daily }: { daily: UsageDay[] }) {
  const maxTokens = Math.max(1, ...daily.map((day) => day.totalTokens))
  const maxRequests = Math.max(1, ...daily.map((day) => day.requests))
  const empty = daily.every(
    (day) => day.totalTokens === 0 && day.requests === 0
  )

  return (
    <UsageCard className="min-h-64 sm:col-span-2">
      <h2 className="text-sm font-medium text-white/80">Weekly usage</h2>

      <div className="mt-4 flex flex-1 flex-col">
        <div className="relative flex h-44 items-end gap-2 border-b border-white/10 pb-px">
          {[0.25, 0.5, 0.75, 1].map((step) => (
            <div
              key={step}
              aria-hidden="true"
              className="pointer-events-none absolute inset-x-0 border-t border-dashed border-white/5"
              style={{ bottom: `${step * 100}%` }}
            />
          ))}

          {empty ? (
            <p className="pointer-events-none absolute inset-0 flex items-center justify-center text-xs text-white/25">
              No model calls yet
            </p>
          ) : null}

          {daily.map((day) => (
            <div
              key={day.date}
              title={`${weekday(day.date)} ${day.date}: ${full(day.totalTokens)} tokens, ${full(day.requests)} requests`}
              className="group flex h-full flex-1 items-end justify-center gap-[2px]"
            >
              <div
                className="w-full max-w-8 rounded-t-md bg-[#5B7FFF] opacity-80 transition-opacity group-hover:opacity-100"
                style={{ height: `${(day.totalTokens / maxTokens) * 100}%` }}
              />
              <div
                className="w-full max-w-8 rounded-t-md bg-[#FF8FB8] opacity-80 transition-opacity group-hover:opacity-100"
                style={{ height: `${(day.requests / maxRequests) * 100}%` }}
              />
            </div>
          ))}
        </div>

        <div className="mt-2 flex gap-2 text-[0.65rem] text-white/30 tabular-nums">
          {daily.map((day) => (
            <span key={day.date} className="flex-1 text-center">
              {weekday(day.date)}
            </span>
          ))}
        </div>

        <div className="mt-3 flex items-center gap-4 text-xs text-white/50">
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-[#5B7FFF]" />
            Tokens
          </span>
          <span className="flex items-center gap-1.5">
            <span className="size-2 rounded-full bg-[#FF8FB8]" />
            Requests
          </span>
        </div>
      </div>
    </UsageCard>
  )
}

/**
 * What the models have been used for, refreshed while the page is open.
 *
 * Two short panels top-left, the request total down the right-hand side for
 * the full height, and one wide panel under the first two. The provider picker
 * in the corner narrows every card, not just the rows.
 */
function UsagePage() {
  const { snapshot, error } = useLiveUsage()
  const [providerId, setProviderId] = useState("")

  const sorted = useMemo(
    () =>
      [...(snapshot?.providers ?? [])].sort((a, b) =>
        a.name.localeCompare(b.name)
      ),
    [snapshot]
  )

  const options = useMemo<Option[]>(
    () => [
      { value: "", label: "All providers" },
      ...sorted.map((provider) => ({
        value: provider.id,
        label: provider.name,
      })),
    ],
    [sorted]
  )

  const selected =
    sorted.find((provider) => provider.id === providerId) ?? null
  const totals = selected ?? snapshot?.today ?? ZERO
  const daily = selected ? selected.daily : (snapshot?.daily ?? [])
  const rpdLimit = selected ? selected.rpdLimit : (snapshot?.limits.rpd ?? 0)
  const tpmLimit = selected ? selected.tpmLimit : (snapshot?.limits.tpm ?? 0)
  const minuteTokens = selected
    ? selected.minute.totalTokens
    : (snapshot?.minute.totalTokens ?? 0)
  const requestsRemaining =
    rpdLimit > 0 ? Math.max(0, rpdLimit - totals.requests) : null
  const tokensRemaining = Math.max(0, tpmLimit - minuteTokens)

  const visibleProviders = providerId
    ? sorted.filter((provider) => provider.id === providerId)
    : sorted

  return (
    <PageShell
      title="Usage"
      subtitle="Live token and request usage across your providers"
      error={error}
      onDismissError={clearUsageError}
      actions={
        <>
          <span
            title="Refreshes every 5 seconds"
            className="hidden items-center gap-1.5 text-[0.65rem] font-medium text-white/30 sm:flex"
          >
            <span className="size-1.5 animate-pulse rounded-full bg-[#6BFFE0]" />
            Live
          </span>
          <Select
            value={providerId}
            onValueChange={(next) => setProviderId(next ?? "")}
            items={options}
          >
            <SelectTrigger
              aria-label="Filter usage by provider"
              className="h-8 w-44 rounded-full"
            >
              <SelectValue placeholder="All providers" />
              <SelectIcon>
                <HugeiconsIcon
                  icon={ArrowDown01Icon}
                  strokeWidth={2}
                  className="size-3.5"
                />
              </SelectIcon>
            </SelectTrigger>
            <SelectContent>
              {options.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {option.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </>
      }
    >
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <TokensCard
          used={totals.totalTokens}
          remaining={tokensRemaining}
          limited={tpmLimit > 0}
        />
        <RequestsCard used={totals.requests} remaining={requestsRemaining} />
        <TotalRequestsCard
          providers={visibleProviders}
          className="sm:col-start-3 sm:row-span-2 sm:row-start-1"
        />
        <WeeklyUsageCard daily={daily} />
      </div>
    </PageShell>
  )
}

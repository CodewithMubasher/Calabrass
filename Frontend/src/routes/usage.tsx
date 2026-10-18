import type { ReactNode } from "react"
import { useEffect, useMemo, useState } from "react"
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
import { loadProviders, useProvidersState } from "@/lib/provider-store"
import type { ProviderRow } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/usage",
  component: UsagePage,
})

type Option = { value: string; label: string }

/** The day's allowance each provider's row is measured against. */
const REQUESTS_PER_DAY = 20

/**
 * Placeholder request counts until a usage endpoint exists. They are spread
 * across the providers so the card reads like a real day; a provider beyond
 * the end of the list starts at zero rather than inventing a number.
 */
const SAMPLE_REQUESTS = [12, 9, 7, 5, 4, 3]

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

/** One provider's request row: a gauge, its name, and where it stands. */
function RequestRow({
  provider,
  index,
}: {
  provider: ProviderRow
  index: number
}) {
  const used = SAMPLE_REQUESTS[index] ?? 0
  const percentage = (used / REQUESTS_PER_DAY) * 100

  return (
    <li className="flex items-center gap-3 px-1 py-1">
      <span className="shrink-0 text-primary">
        <CircularGauge percentage={percentage} />
      </span>
      <span className="min-w-0 truncate text-sm text-white/80">
        {provider.name}
      </span>
      <span className="ml-auto shrink-0 font-mono text-xs font-medium text-white/40 tabular-nums">
        {used}/{REQUESTS_PER_DAY} RPD
      </span>
    </li>
  )
}

/**
 * The request figures for every provider. Every provider is listed in full —
 * the page scrolls as one, so nothing is hidden behind a second scrollbar.
 */
function TotalRequestsCard({ providers }: { providers: ProviderRow[] }) {
  return (
    <UsageCard className="min-h-40">
      <h2 className="shrink-0 text-sm font-medium text-white/80">
        Total request
      </h2>
      <ul className="mt-3 flex flex-col gap-1">
        {providers.map((provider, index) => (
          <RequestRow key={provider.id} provider={provider} index={index} />
        ))}
      </ul>
    </UsageCard>
  )
}

/**
 * What the models have been used for, once there is something to show.
 *
 * Three panels across the top and one wide panel under them; only the request
 * total has content so far. The provider picker in the corner narrows both the
 * cards and the rows inside them.
 */
function UsagePage() {
  const { providers } = useProvidersState()
  const [providerId, setProviderId] = useState("")

  useEffect(() => {
    void loadProviders()
  }, [])

  const sorted = useMemo(
    () => [...providers].sort((a, b) => a.name.localeCompare(b.name)),
    [providers]
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

  const visibleProviders = providerId
    ? sorted.filter((provider) => provider.id === providerId)
    : sorted

  return (
    <PageShell
      title="Usage"
      subtitle="See where your tokens go"
      actions={
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
      }
    >
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <UsageCard className="min-h-40" />
        <UsageCard className="min-h-40" />
        <TotalRequestsCard providers={visibleProviders} />
        <UsageCard className="min-h-64 sm:col-span-3" />
      </div>
    </PageShell>
  )
}

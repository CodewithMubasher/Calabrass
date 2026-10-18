import { useEffect, useMemo, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowDown01Icon, PieChartIcon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty, PageShell } from "@/components/page-shell"
import {
  Select,
  SelectContent,
  SelectIcon,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { loadProviders, useProvidersState } from "@/lib/provider-store"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/usage",
  component: UsagePage,
})

type Option = { value: string; label: string }

/**
 * What the models have been used for, once there is something to show.
 *
 * For now the page is the same frame as the rest with a placeholder in the
 * middle; the provider picker in the corner is where the figures will be
 * scoped from.
 */
function UsagePage() {
  const { providers } = useProvidersState()
  const [providerId, setProviderId] = useState("")

  useEffect(() => {
    void loadProviders()
  }, [])

  const options = useMemo<Option[]>(
    () => [
      { value: "", label: "All providers" },
      ...providers.map((provider) => ({
        value: provider.id,
        label: provider.name,
      })),
    ],
    [providers]
  )

  const selected = options.find((option) => option.value === providerId)

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
      <PageEmpty
        icon={PieChartIcon}
        title={
          providerId
            ? `No usage for ${selected?.label ?? "this provider"} yet`
            : "No usage to show yet"
        }
        hint="Token and model usage will appear here once you start chatting"
      />
    </PageShell>
  )
}

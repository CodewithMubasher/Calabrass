import { useEffect, useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react"
import {
  Add01Icon,
  ChatGptIcon,
  ClaudeIcon,
  DeepseekIcon,
  GoogleGeminiIcon,
  Grok02Icon,
  MistralIcon,
  Search01Icon,
  ThreeDScaleIcon,
} from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Switch } from "@/components/ui/switch"
import {
  AddProviderDialog,
  ProviderDetailDialog,
} from "@/components/provider-dialog"
import {
  loadProviders,
  setProviderEnabled,
  useProvidersState,
} from "@/lib/provider-store"
import type { ProviderRow } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/providers",
  component: ProvidersPage,
})

// Icons are a client concern: the backend only sends provider ids. A provider
// it does not know yet falls back to the generic mark.
const PROVIDER_ICONS: Record<string, IconSvgElement> = {
  google: GoogleGeminiIcon,
  groq: Grok02Icon,
  openai: ChatGptIcon,
  anthropic: ClaudeIcon,
  mistral: MistralIcon,
  deepseek: DeepseekIcon,
}

/** The muted middle line of a card. Empty when there is nothing to warn about. */
function describe(provider: ProviderRow) {
  if (!provider.connected) {
    return provider.custom
      ? "No base URL configured"
      : "No API key configured"
  }
  if (!provider.enabled) return "Turned off — its models are hidden"
  if (provider.modelCount === 0) {
    return provider.custom
      ? "No chat models found at this endpoint"
      : "No chat models available for this key"
  }
  return ""
}

/** The status dot: green when usable, amber when off, grey when unconfigured. */
function statusOf(provider: ProviderRow) {
  if (!provider.connected) {
    return {
      label: provider.custom ? "Not configured" : "No API key",
      dot: "bg-white/25",
    }
  }
  if (!provider.enabled) {
    return { label: "Off", dot: "bg-amber-400" }
  }
  return { label: "Connected", dot: "bg-emerald-400" }
}

function ProvidersPage() {
  const { providers, loading, error, pending } = useProvidersState()
  const [query, setQuery] = useState("")
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<ProviderRow | null>(null)

  useEffect(() => {
    void loadProviders()
  }, [])

  const needle = query.trim().toLowerCase()
  const visible = needle
    ? providers.filter((p) => p.name.toLowerCase().includes(needle))
    : providers

  return (
    <div className="scrollbar-thin min-h-0 flex-1 overflow-y-auto">
      <div className="mx-auto w-full max-w-4xl px-4 py-6">
        <div className="flex items-center justify-between gap-4">
          <h1 className="min-w-0 truncate text-xl font-semibold text-white">
            Providers
          </h1>
          <div className="flex shrink-0 items-center gap-2">
            <div className="relative">
              <HugeiconsIcon
                icon={Search01Icon}
                strokeWidth={2}
                className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-white/40"
              />
              <Input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search providers"
                aria-label="Search providers"
                className="w-56 pl-8"
              />
            </div>
            <Button className="gap-1.5" onClick={() => setAdding(true)}>
              <HugeiconsIcon icon={Add01Icon} strokeWidth={2} />
              New provider
            </Button>
          </div>
        </div>

        <p className="mt-1 text-sm text-white/50">
          Connect the model providers Calabrass can use.
        </p>

        {error ? (
          <p className="mt-4 text-sm text-red-400">{error}</p>
        ) : null}

        {loading && providers.length === 0 ? (
          <p className="mt-6 text-sm text-white/40">Loading providers…</p>
        ) : null}

        <div className="mt-6 grid gap-4 sm:grid-cols-2">
          {visible.map((provider) => {
            const status = statusOf(provider)
            const description = describe(provider)
            const busy = pending.includes(provider.id)

            return (
              <Card
                key={provider.id}
                // Clicking anywhere on the card opens its detail dialog, so it
                // has to behave like a button.
                role="button"
                tabIndex={0}
                aria-label={`Open ${provider.name}`}
                onClick={() => setEditing(provider)}
                onKeyDown={(event) => {
                  if (event.key !== "Enter" && event.key !== " ") return
                  event.preventDefault()
                  setEditing(provider)
                }}
                className={cn(
                  // The prompt input on the home screen, verbatim: a
                  // translucent white wash with a hairline border.
                  "cursor-pointer rounded-2xl border border-white/10 bg-white/5 backdrop-blur-[2px] transition-colors",
                  // Ring comes from the Card primitive; the border replaces it.
                  "ring-0 hover:border-white/30",
                  "focus-visible:ring-2 focus-visible:ring-white/40 focus-visible:outline-none",
                  !provider.enabled && "opacity-70"
                )}
              >
                <CardHeader>
                  <CardTitle className="flex items-center gap-2.5 text-white">
                    <span className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-white/5 ring-1 ring-white/10">
                      <HugeiconsIcon
                        icon={PROVIDER_ICONS[provider.id] ?? ThreeDScaleIcon}
                        strokeWidth={1.8}
                        className="size-4.5"
                      />
                    </span>
                    {provider.name}
                  </CardTitle>
                  <CardAction
                    // The switch is its own control: toggling it must not open
                    // the detail dialog.
                    onClick={(event) => event.stopPropagation()}
                    onKeyDown={(event) => event.stopPropagation()}
                  >
                    <Switch
                      checked={provider.enabled}
                      disabled={busy}
                      onCheckedChange={(next) =>
                        void setProviderEnabled(provider.id, next)
                      }
                      aria-label={`Turn ${provider.name} ${provider.enabled ? "off" : "on"}`}
                    />
                  </CardAction>
                  {description !== "" ? (
                    <CardDescription className="text-white/45">
                      {description}
                    </CardDescription>
                  ) : null}
                </CardHeader>
                <CardContent className="flex items-center gap-2 text-xs text-white/45">
                  <span className={cn("size-1.5 rounded-full", status.dot)} />
                  <span>{status.label}</span>
                  {/* Its models are gone from the picker while it is off, so
                      there is nothing to count here either. */}
                  {provider.enabled && provider.modelCount > 0 ? (
                    <span className="ml-auto">
                      {provider.modelCount} chat models
                    </span>
                  ) : null}
                </CardContent>
              </Card>
            )
          })}

          {!loading && visible.length === 0 ? (
            <p className="col-span-full py-10 text-center text-sm text-white/40">
              {needle
                ? `No providers match “${query.trim()}”.`
                : "No providers are available yet."}
            </p>
          ) : null}
        </div>
      </div>

      {adding ? (
        <AddProviderDialog
          onOpenChange={(open) => {
            if (!open) setAdding(false)
          }}
        />
      ) : null}
      {editing ? (
        <ProviderDetailDialog
          key={editing.id}
          provider={editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
        />
      ) : null}
    </div>
  )
}

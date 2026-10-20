import { useEffect, useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react"
import {
  Add01Icon,
  BotIcon,
  ChatGptIcon,
  CloudIcon,
  CpuIcon,
  DeepseekIcon,
  GoogleGeminiIcon,
  Grok02Icon,
  MistralIcon,
  MoreHorizontalIcon,
  PowerIcon,
  PowerOffIcon,
  RocketIcon,
  Search02Icon,
  TerminalIcon,
  ThreeDScaleIcon,
  UserGroupIcon,
} from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import {
  PageEmpty,
  PageLoading,
  PageShell,
  SearchInput,
} from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  AddProviderDialog,
  ProviderDetailDialog,
} from "@/components/provider-dialog"
import {
  clearProvidersError,
  deleteProvider,
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
  anthropic: BotIcon,
  mistral: MistralIcon,
  deepseek: DeepseekIcon,
  openrouter: ThreeDScaleIcon,
  nvidia: CpuIcon,
  cloudflare: CloudIcon,
  ollama: TerminalIcon,
  together: UserGroupIcon,
  cohere: BotIcon,
  perplexity: Search02Icon,
  xai: RocketIcon,
}

/** The middle line of a card: the endpoint the provider calls. */
function describe(provider: ProviderRow) {
  if (provider.baseUrl) return provider.baseUrl
  return "No base URL configured"
}

/** The chip: green when the provider can be used, grey when it cannot. */
function statusOf(provider: ProviderRow) {
  if (!provider.connected) {
    return { label: "No key", chip: "bg-white/5 text-white/30" }
  }
  if (!provider.enabled) {
    return { label: "Inactive", chip: "bg-white/5 text-white/30" }
  }
  return {
    label: "Active",
    chip: "bg-emerald-500/10 text-emerald-400",
  }
}

function ProvidersPage() {
  const { providers, loading, error, pending } = useProvidersState()
  const [query, setQuery] = useState("")
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<ProviderRow | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    void loadProviders()
  }, [])

  const remove = async (providerId: string) => {
    setActionError(null)
    try {
      await deleteProvider(providerId)
    } catch (err) {
      setActionError(
        err instanceof Error ? err.message : "Could not remove the provider."
      )
    }
  }

  const needle = query.trim().toLowerCase()
  const visible = needle
    ? providers.filter((p) => p.name.toLowerCase().includes(needle))
    : providers

  return (
    <PageShell
      title="Providers"
      subtitle="Save an API key for any AI provider and use its models"
      error={actionError ?? error}
      onDismissError={() => {
        setActionError(null)
        clearProvidersError()
      }}
      actions={
        <>
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="Search providers..."
          />
          <Button
            variant="default"
            className="gap-2 rounded-full"
            onClick={() => setAdding(true)}
          >
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} className="size-4" />
            <span>New provider</span>
          </Button>
        </>
      }
    >
      {loading && providers.length === 0 ? (
        <PageLoading />
      ) : visible.length === 0 ? (
        <PageEmpty
          icon={BotIcon}
          title={
            needle ? "No providers match your search" : "No providers yet"
          }
          hint={needle ? undefined : "Add a provider to start using AI models"}
        />
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {visible.map((provider) => {
            const status = statusOf(provider)

            return (
              <div
                key={provider.id}
                className="relative flex flex-col rounded-2xl border border-white/10 bg-white/5 p-4 transition-colors hover:border-white/20"
              >
                <div className="flex items-start justify-between">
                  <div className="flex min-w-0 items-center gap-3">
                    <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-white/5">
                      <HugeiconsIcon
                        icon={PROVIDER_ICONS[provider.id] ?? ThreeDScaleIcon}
                        strokeWidth={1.8}
                        className="size-4 text-white/50"
                      />
                    </div>
                    <div className="min-w-0">
                      <h3 className="truncate text-sm font-medium text-white/90">
                        {provider.name}
                      </h3>
                      <p className="mt-0.5 truncate font-mono text-xs text-white/40">
                        {describe(provider)}
                      </p>
                    </div>
                  </div>

                  <div className="flex shrink-0 items-center gap-2">
                    <button
                      type="button"
                      onClick={() =>
                        void setProviderEnabled(provider.id, !provider.enabled)
                      }
                      disabled={pending.includes(provider.id)}
                      title={provider.enabled ? "Disable" : "Enable"}
                      aria-label={`Turn ${provider.name} ${provider.enabled ? "off" : "on"}`}
                      className={cn(
                        "cursor-pointer rounded-lg p-1.5 transition-colors disabled:pointer-events-none disabled:opacity-50",
                        provider.enabled
                          ? "bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20"
                          : "bg-white/5 text-white/30 hover:bg-white/10"
                      )}
                    >
                      <HugeiconsIcon
                        icon={provider.enabled ? PowerIcon : PowerOffIcon}
                        strokeWidth={2}
                        className="size-3.5"
                      />
                    </button>

                    <DropdownMenu>
                      <DropdownMenuTrigger
                        render={
                          <button
                            aria-label={`${provider.name} actions`}
                            className="cursor-pointer rounded-lg p-1 transition-colors hover:bg-white/10"
                          />
                        }
                      >
                        <HugeiconsIcon
                          icon={MoreHorizontalIcon}
                          strokeWidth={2}
                          className="size-4 text-white/50"
                        />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end">
                        <DropdownMenuItem
                          onClick={() => setEditing(provider)}
                        >
                          <span>Edit</span>
                        </DropdownMenuItem>
                        <DropdownMenuItem
                          variant="destructive"
                          onClick={() => void remove(provider.id)}
                        >
                          <span>Delete</span>
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                </div>

                <div className="mt-auto flex flex-wrap items-center gap-2 pt-3">
                  <span
                    className={cn(
                      "rounded-full px-1.5 py-0.5 text-[10px]",
                      status.chip
                    )}
                  >
                    {status.label}
                  </span>
                  <span
                    className="max-w-40 truncate font-mono text-[11px] text-white/40"
                    title={provider.defaultModel || "No default model"}
                  >
                    {provider.defaultModel || "No default model"}
                  </span>
                  <span className="ml-auto font-mono text-[11px] text-white/30">
                    {provider.modelCount} models
                  </span>
                </div>
              </div>
            )
          })}
        </div>
      )}

      {adding ? (
        <AddProviderDialog
          onOpenChange={(next) => {
            if (!next) setAdding(false)
          }}
        />
      ) : null}
      {editing ? (
        <ProviderDetailDialog
          key={editing.id}
          provider={editing}
          onOpenChange={(next) => {
            if (!next) setEditing(null)
          }}
        />
      ) : null}
    </PageShell>
  )
}

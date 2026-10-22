import { useEffect, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon, BotIcon, Delete02Icon, RotateCwIcon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { AgentDialog } from "@/components/agent-dialog"
import {
  PageEmpty,
  PageLoading,
  PageShell,
  SearchInput,
} from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { loadAgents, useAgentsState, deleteAgent, resetAgent } from "@/lib/agent-store"
import { loadConnectors, useConnectorsState } from "@/lib/connector-store"
import type { AgentRow } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents",
  component: AgentsPage,
})

function AgentCard({
  agent,
  connectorName,
  onClick,
  onDelete,
  onReset,
}: {
  agent: AgentRow
  connectorName: string
  onClick: () => void
  onDelete: () => void
  onReset: () => void
}) {
  return (
    <div
      className="flex flex-col rounded-2xl border border-white/10 bg-white/5 p-4 text-left transition-colors hover:border-white/20 cursor-pointer"
      onClick={onClick}
    >
      <span className="flex w-full items-start gap-3">
        <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-white/5">
          <HugeiconsIcon
            icon={BotIcon}
            strokeWidth={1.8}
            className="size-4 text-white/50"
          />
        </span>
        <span className="min-w-0 flex-1">
          <span className="flex items-center gap-2">
            <span className="block truncate text-sm font-medium text-white/90">
              {agent.name}
            </span>
            {agent.builtin && (
              <span className="shrink-0 rounded-full border border-[#9D79FF]/40 bg-[#9D79FF]/15 px-2 py-0.5 text-[10px] text-[#9D79FF]">
                Built-in
              </span>
            )}
          </span>
          <span
            title={
              agent.model
                ? `${agent.provider} · ${agent.model}`
                : "No default model"
            }
            className="mt-0.5 block truncate text-xs text-white/40"
          >
            {agent.model || "No default model"}
          </span>
        </span>
        <span className="shrink-0 rounded-full border border-white/10 px-2 py-0.5 text-[11px] text-white/50">
          {agent.mode === "crew" ? "Crew" : "Chat"}
        </span>
      </span>

      <span className="mt-3 line-clamp-2 min-h-8 text-xs leading-relaxed text-white/40">
        {agent.prompt || "No prompt yet."}
      </span>

      <span className="mt-3 flex flex-wrap items-center gap-2 text-[11px] text-white/35">
        {!agent.builtin && (
          <span className="rounded-full bg-white/5 px-2 py-0.5">
            {connectorName || "No connector"}
          </span>
        )}
        {agent.altModel ? (
          <span
            title={`${agent.altProvider} · ${agent.altModel}`}
            className="max-w-full truncate rounded-full bg-white/5 px-2 py-0.5"
          >
            Fallback · {agent.altModel}
          </span>
        ) : null}
        {agent.systemPrompt ? (
          <span className="rounded-full bg-primary/20 px-2 py-0.5 text-primary">
            Custom System Prompt
          </span>
        ) : null}
        {agent.builtin ? (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto gap-1 text-[11px] text-white/60 hover:text-white"
            onClick={(e) => { e.stopPropagation(); onReset(); }}
          >
            <HugeiconsIcon icon={RotateCwIcon} strokeWidth={1.8} className="size-3.5" />
            Reset to default
          </Button>
        ) : (
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto gap-1 text-[11px] text-white/60 hover:text-white hover:text-red-400"
            onClick={(e) => { e.stopPropagation(); onDelete(); }}
          >
            <HugeiconsIcon icon={Delete02Icon} strokeWidth={1.8} className="size-3.5" />
            Delete
          </Button>
        )}
      </span>
    </div>
  )
}

function AgentsPage() {
  const { agents, loading } = useAgentsState()
  const { connectors } = useConnectorsState()
  const [query, setQuery] = useState("")
  const [adding, setAdding] = useState(false)
  const [editing, setEditing] = useState<AgentRow | null>(null)

  useEffect(() => {
    void loadAgents()
    void loadConnectors()
  }, [])

  const needle = query.trim().toLowerCase()
  const visible = needle
    ? agents.filter((agent) => agent.name.toLowerCase().includes(needle))
    : agents

  const connectorNames = new Map(
    connectors.map((connector) => [connector.id, connector.name])
  )

  return (
    <PageShell
      title="Crew work"
      subtitle="The workers the crew is made of"
      actions={
        <>
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="Search workers..."
          />
          <Button
            variant="default"
            className="gap-2 rounded-full"
            onClick={() => setAdding(true)}
          >
            <HugeiconsIcon
              icon={Add01Icon}
              strokeWidth={2}
              className="size-4"
            />
            <span>New worker</span>
          </Button>
        </>
      }
    >
      {loading && agents.length === 0 ? (
        <PageLoading />
      ) : visible.length === 0 ? (
        <PageEmpty
          icon={BotIcon}
          title={needle ? "No workers match your search" : "No workers yet"}
          hint={
            needle
              ? undefined
              : "Create one to give the crew another pair of hands"
          }
        />
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {visible.map((agent) => (
            <AgentCard
              key={agent.id}
              agent={agent}
              connectorName={connectorNames.get(agent.connectorIds?.[0]) ?? ""}
              onClick={() => setEditing(agent)}
              onDelete={() => void deleteAgent(agent.id)}
              onReset={() => void resetAgent(agent.id)}
            />
          ))}
        </div>
      )}

      {adding ? <AgentDialog onOpenChange={setAdding} /> : null}
      {editing ? (
        <AgentDialog
          key={editing.id}
          agent={editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
        />
      ) : null}
    </PageShell>
  )
}
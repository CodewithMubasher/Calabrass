import { useEffect, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Add01Icon, BotIcon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { AgentDialog } from "@/components/agent-dialog"
import {
  PageEmpty,
  PageLoading,
  PageShell,
  SearchInput,
} from "@/components/page-shell"
import { Button } from "@/components/ui/button"
import { loadAgents, useAgentsState } from "@/lib/agent-store"
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
}: {
  agent: AgentRow
  connectorName: string
  onClick: () => void
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
          <span className="block truncate text-sm font-medium text-white/90">
            {agent.name}
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
          {agent.mode === "plan" ? "Plan" : "Build"}
        </span>
      </span>

      <span className="mt-3 line-clamp-2 min-h-8 text-xs leading-relaxed text-white/40">
        {agent.prompt || "No prompt yet."}
      </span>

      <span className="mt-3 flex flex-wrap items-center gap-2 text-[11px] text-white/35">
        <span className="rounded-full bg-white/5 px-2 py-0.5">
          {connectorName || "No connector"}
        </span>
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
      title="Agents"
      subtitle="Give the assistant roles of its own"
      actions={
        <>
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="Search agents..."
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
            <span>New agent</span>
          </Button>
        </>
      }
    >
      {loading && agents.length === 0 ? (
        <PageLoading />
      ) : visible.length === 0 ? (
        <PageEmpty
          icon={BotIcon}
          title={needle ? "No agents match your search" : "No agents yet"}
          hint={
            needle
              ? undefined
              : "Create one to give the assistant a role of its own"
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
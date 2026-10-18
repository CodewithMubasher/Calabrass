import { useEffect, useMemo, useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  ArrowLeftIcon,
  BotIcon,
  Cancel01Icon,
  CogIcon,
  CopyIcon,
  Edit02Icon,
  SparklesIcon,
} from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty, PageLoading } from "@/components/page-shell"
import { PromptInput } from "@/components/prompt-input"
import { Alert, AlertAction } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Textarea } from "@/components/ui/textarea"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { createChat, sendMessage, useChats } from "@/lib/chat-store"
import { useAgentsState } from "@/lib/agent-store"
import { useModelState } from "@/lib/model-store"
import { useConnectorsState } from "@/lib/connector-store"
import type { AgentRow, AgentMode } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"
import { saveAgent } from "@/lib/agent-store"
import { cn } from "cn"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/agents/$agentId",
  component: AgentRoute,
})

function AgentRoute() {
  const { agentId } = Route.useParams()
  return <AgentView key={agentId} agentId={agentId} />
}

/** One agent: a composer that starts a chat inside it, its recents, and the
 *  agent's configuration cards beside them. */
function AgentView({ agentId }: { agentId: string }) {
  const navigate = useNavigate()
  const { agents, loading, error: listError } = useAgentsState()
  const { providers } = useModelState()
  const { connectors } = useConnectorsState()
  const chats = useChats()

  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [editing, setEditing] = useState<AgentRow | null>(null)

  const agent = useMemo(
    () => agents.find((item) => item.id === agentId),
    [agents, agentId]
  )

  const connector = useMemo(
    () => connectors.find((c) => c.id === agent?.connectorId),
    [connectors, agent?.connectorId]
  )

  const agentChats = useMemo(
    () =>
      agent
        ? chats.filter((chat) => chat.agentId === agent.id)
        : [],
    [chats, agent]
  )

  async function handleSend(message: string) {
    if (!agent || sending) return

    setSending(true)
    setError(null)
    try {
      const chat = await createChat(undefined, agent.id)
      await navigate({ to: "/chat/$chatId", params: { chatId: chat.id } })
      void sendMessage(chat.id, message)
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Could not start a chat with this agent."
      )
    } finally {
      setSending(false)
    }
  }

  function formatDate(at: number): string {
    if (!at) return ""
    return new Date(at).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    })
  }

  function getModeLabel(mode: AgentMode): string {
    return mode === "plan" ? "Plan" : "Build"
  }

  function getModeColor(mode: AgentMode): string {
    return mode === "plan" ? "border-[#FF8FB8]/40 bg-[#FF8FB8]/15 text-[#FF8FB8]" : "border-[#5B7FFF]/40 bg-[#5B7FFF]/15 text-[#5B7FFF]"
  }

  if (loading && !agent) {
    return <PageLoading />
  }

  return (
    <div className="scrollbar-thin flex h-full flex-col overflow-auto">
      <div className="shrink-0 px-14 pt-8">
        <div className="flex items-center gap-3">
          <button
            type="button"
            aria-label="Back to agents"
            className="cursor-pointer rounded-lg p-2 transition-colors hover:bg-white/10"
            onClick={() => navigate({ to: "/agents" })}
          >
            <HugeiconsIcon icon={ArrowLeftIcon} strokeWidth={2} className="size-5 text-white/60" />
          </button>
          <div>
            <h1 className="text-2xl font-semibold text-white/90">{agent?.name ?? "Untitled"}</h1>
            <p className="text-sm text-white/50">
              {connector ? `MCP: ${connector.name}` : "No MCP connector"}
            </p>
          </div>
        </div>
      </div>

      {error || listError ? (
        <Alert variant="destructive" className="mx-14 mt-4 shrink-0">
          <AlertAction onClick={() => setError(null)} aria-label="Dismiss">
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
          </AlertAction>
          {error ?? listError}
        </Alert>
      ) : null}

      {!agent ? (
        <div className="flex-1">
          <PageEmpty
            icon={BotIcon}
            title="This agent no longer exists"
            hint="It may have been removed from the list"
          />
        </div>
      ) : (
        <div className="mt-8 flex items-start gap-10 px-14">
          <div className="flex min-w-0 flex-1 flex-col">
            <PromptInput
              onSend={handleSend}
              heading={`Chat with ${agent.name}`}
              compact
              disabled={sending}
            />

            <div className="mt-8">
              <h3 className="mb-3 text-xs font-medium text-white/40">Recents</h3>
              {agentChats.length === 0 ? (
                <p className="text-xs text-white/30">
                  No chats yet with this agent
                </p>
              ) : (
                <div className="space-y-1">
                  {agentChats.map((chat) => (
                    <button
                      key={chat.id}
                      type="button"
                      onClick={() =>
                        navigate({
                          to: "/chat/$chatId",
                          params: { chatId: chat.id },
                        })
                      }
                      className="w-full flex cursor-pointer items-center justify-between gap-2 px-3 py-2 text-left rounded-lg bg-white/5 transition-colors hover:bg-white/10"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-white/5">
                          <HugeiconsIcon icon={SparklesIcon} strokeWidth={2} className="size-4 text-white/50" />
                        </div>
                        <span className="truncate text-sm text-white/90">{chat.title}</span>
                      </div>
                      <span className="shrink-0 text-xs text-white/40">{formatDate(chat.createdAt)}</span>
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="-mt-3 flex w-[320px] flex-col gap-3">
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">
                  Agent Configuration
                </h3>
                <button
                  type="button"
                  onClick={() => setEditing(agent)}
                  className="cursor-pointer rounded-lg p-1 transition-colors hover:bg-white/10"
                >
                  <HugeiconsIcon icon={Edit02Icon} strokeWidth={2} className="size-4 text-white/50" />
                </button>
              </div>

              <div className="space-y-3">
                <div>
                  <label className="mb-1 block text-xs text-white/40">Mode</label>
                  <span
                    className={cn(
                      "inline-flex items-center rounded-full px-2 py-0.5 text-[11px] font-medium",
                      getModeColor(agent.mode)
                    )}
                  >
                    {getModeLabel(agent.mode)}
                  </span>
                </div>

                <div>
                  <label className="mb-1 block text-xs text-white/40">Default Model</label>
                  <p className="text-xs font-mono text-white/70 truncate">
                    {agent.provider} · {agent.model}
                  </p>
                </div>

                {agent.altModel && (
                  <div>
                    <label className="mb-1 block text-xs text-white/40">Alternative Model</label>
                    <p className="text-xs font-mono text-white/50 truncate">
                      {agent.altProvider} · {agent.altModel}
                    </p>
                  </div>
                )}

                {connector && (
                  <div>
                    <label className="mb-1 block text-xs text-white/40">MCP Connector</label>
                    <div className="flex items-center gap-2">
                      <HugeiconsIcon icon={CogIcon} strokeWidth={2} className="size-4 text-white/50" />
                      <span className="text-xs text-white/70">{connector.name}</span>
                    </div>
                  </div>
                )}

                <div>
                  <label className="mb-1 block text-xs text-white/40">System Prompt</label>
                  <div className="rounded-lg bg-white/5 p-2 max-h-32 overflow-y-auto">
                    <pre className="whitespace-pre-wrap text-xs text-white/60 font-mono">
                      {agent.systemPrompt || agent.prompt || "(uses agent prompt)"}
                    </pre>
                  </div>
                </div>

                <div>
                  <label className="mb-1 block text-xs text-white/40">Agent Prompt</label>
                  <div className="rounded-lg bg-white/5 p-2 max-h-32 overflow-y-auto">
                    <pre className="whitespace-pre-wrap text-xs text-white/50 font-mono">
                      {agent.prompt || "(empty)"}
                    </pre>
                  </div>
                </div>
              </div>
            </div>

            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">
                  Quick Actions
                </h3>
                <HugeiconsIcon icon={SparklesIcon} strokeWidth={2} className="size-4 text-white/40" />
              </div>
              <div className="space-y-2">
                <button
                  type="button"
                  onClick={() => handleSend("")}
                  disabled={sending}
                  className="w-full flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-white/80 transition-colors hover:bg-white/10 disabled:opacity-50"
                >
                  <HugeiconsIcon icon={Add01Icon} strokeWidth={2} className="size-4 text-white/50" />
                  <span>New Chat</span>
                </button>
                <button
                  type="button"
                  onClick={() => setEditing(agent)}
                  className="w-full flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-white/80 transition-colors hover:bg-white/10"
                >
                  <HugeiconsIcon icon={Edit02Icon} strokeWidth={2} className="size-4 text-white/50" />
                  <span>Edit Agent</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    const prompt = agent.systemPrompt || agent.prompt
                    navigator.clipboard.writeText(prompt)
                  }}
                  className="w-full flex cursor-pointer items-center gap-2 rounded-lg px-3 py-2 text-left text-sm text-white/80 transition-colors hover:bg-white/10"
                >
                  <HugeiconsIcon icon={CopyIcon} strokeWidth={2} className="size-4 text-white/50" />
                  <span>Copy System Prompt</span>
                </button>
              </div>
            </div>
          </div>
        </div>
      )}

      {editing ? (
        <AgentDialog
          key={editing.id}
          agent={editing}
          onOpenChange={(open) => {
            if (!open) setEditing(null)
          }}
          providers={providers}
        />
      ) : null}
    </div>
  )
}

/** Dialog for editing the agent from the agent view. */
function AgentDialog({
  agent,
  onOpenChange,
  providers,
}: {
  agent: AgentRow
  onOpenChange: (open: boolean) => void
  providers: import("@/types/chat").ProviderInfo[]
}) {
  const [name, setName] = useState(agent.name)
  const [prompt, setPrompt] = useState(agent.prompt)
  const [systemPrompt, setSystemPrompt] = useState(agent.systemPrompt)
  const [mode, setMode] = useState<AgentMode>(agent.mode)
  const [connectorId, setConnectorId] = useState(agent.connectorId)
  const [model, setModel] = useState(
    `${agent.provider}::${agent.model}`
  )
  const [altModel, setAltModel] = useState(
    agent.altProvider && agent.altModel ? `${agent.altProvider}::${agent.altModel}` : ""
  )
  const { error: modelStoreError } = useModelState()
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const { connectors } = useConnectorsState()

  // Load data on mount
  useEffect(() => {
    void (async () => {
      const { loadModels } = await import("@/lib/model-store")
      const { loadConnectors } = await import("@/lib/connector-store")
      loadModels()
      loadConnectors()
    })()
  }, [])

  const connectorOptions = useMemo(() => {
    const options: { value: string; label: string }[] = [
      { value: "", label: "No connector" },
      ...connectors.map((connector) => ({
        value: connector.id,
        label: connector.name,
      })),
    ]
    if (connectorId && !options.some((o) => o.value === connectorId)) {
      options.push({ value: connectorId, label: "Unavailable connector" })
    }
    return options
  }, [connectors, connectorId])

  // Build model options from providers
  const modelOptions = useMemo(() => {
    return providers.flatMap((provider) =>
      provider.models.map((m) => ({
        value: `${provider.id}::${m.id}`,
        label: `${provider.name} · ${m.label}`,
      }))
    )
  }, [providers])

  const altOptions = useMemo(() => [
    { value: "", label: "No fallback" },
    ...modelOptions,
  ], [modelOptions])

  const ready = name.trim() !== ""

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    const [provider, modelId] = model.split("::")
    const [altProvider, altModelId] = altModel.split("::")
    const input = {
      name: name.trim(),
      prompt: prompt.trim(),
      systemPrompt: systemPrompt.trim(),
      mode,
      connectorId,
      provider: provider || "",
      model: modelId || "",
      altProvider: altProvider || "",
      altModel: altModelId || "",
    }

    setSaving(true)
    setError(null)
    try {
      await saveAgent(agent.id, input)
      onOpenChange(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the agent.")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl bg-popover border border-white/10">
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle className="text-white/90">Edit Agent</DialogTitle>
            <DialogDescription className="text-white/50">
              Change what this agent is for, and how it works.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-4">
            <label className="block">
              <span className="mb-2 block text-sm text-white/60">Name</span>
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="Researcher"
                autoFocus
                className="w-full h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30 rounded-lg outline-none focus:border-white/20"
              />
            </label>

            <label className="block">
              <span className="mb-2 block text-sm text-white/60">Agent Prompt</span>
              <Textarea
                value={prompt}
                onChange={(e) => setPrompt(e.target.value)}
                placeholder="You research a question, weigh the sources, and answer with citations."
                rows={5}
              />
            </label>

            <label className="block">
              <span className="mb-2 block text-sm text-white/60">System Prompt (replaces global)</span>
              <Textarea
                value={systemPrompt}
                onChange={(e) => setSystemPrompt(e.target.value)}
                placeholder="Optional: custom system prompt that completely replaces the global one. Leave empty to use the agent prompt above."
                rows={4}
              />
            </label>

            <div className="grid grid-cols-2 gap-4">
              <label className="block">
                <span className="mb-2 block text-sm text-white/60">Mode</span>
                <select
                  value={mode}
                  onChange={(e) => setMode(e.target.value as AgentMode)}
                  className="w-full h-9 border-white/10 bg-white/5 px-3 text-white/80 rounded-lg outline-none focus:border-white/20"
                >
                  <option value="build">Build</option>
                  <option value="plan">Plan</option>
                </select>
              </label>
              <label className="block">
                <span className="mb-2 block text-sm text-white/60">MCP</span>
                <select
                  value={connectorId}
                  onChange={(e) => setConnectorId(e.target.value)}
                  className="w-full h-9 border-white/10 bg-white/5 px-3 text-white/80 rounded-lg outline-none focus:border-white/20"
                >
                  {connectorOptions.map((opt) => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
              </label>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <label className="block">
                <span className="mb-2 block text-sm text-white/60">Default Model</span>
                <select
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  className="w-full h-9 border-white/10 bg-white/5 px-3 text-white/80 rounded-lg outline-none focus:border-white/20"
                >
                  {modelStoreError && <option value="">Models unavailable</option>}
                  {modelOptions.map((opt) => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
              </label>
              <label className="block">
                <span className="mb-2 block text-sm text-white/60">Alternative Model</span>
                <select
                  value={altModel}
                  onChange={(e) => setAltModel(e.target.value)}
                  className="w-full h-9 border-white/10 bg-white/5 px-3 text-white/80 rounded-lg outline-none focus:border-white/20"
                >
                  {altOptions.map((opt) => (
                    <option key={opt.value} value={opt.value}>{opt.label}</option>
                  ))}
                </select>
              </label>
            </div>
          </div>

          {modelStoreError && <p className="text-xs text-white/40">{modelStoreError}</p>}
          {error && <p className="text-sm text-red-400">{error}</p>}

          <DialogFooter className="sm:justify-between">
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              <Button
                type="button"
                variant="ghost"
                onClick={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={saving || !ready}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
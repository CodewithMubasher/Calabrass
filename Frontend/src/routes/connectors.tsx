import { useEffect, useState } from "react"
import { cn } from "cn"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  MoreHorizontalIcon,
  Plug01Icon,
  PowerIcon,
  PowerOffIcon,
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
import { ConnectorDialog } from "@/components/connector-dialog"
import {
  clearConnectorsError,
  deleteConnector,
  loadConnectors,
  setConnectorEnabled,
  useConnectorsState,
} from "@/lib/connector-store"
import type { ConnectorRow } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/connectors",
  component: ConnectorsPage,
})

function formatDate(at: number): string {
  if (!at) return ""
  return new Date(at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function ConnectorsPage() {
  const { connectors, loading, error, pending } = useConnectorsState()
  const [query, setQuery] = useState("")
  const [adding, setAdding] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    void loadConnectors()
  }, [])

  const needle = query.trim().toLowerCase()
  const visible = needle
    ? connectors.filter((c) => c.name.toLowerCase().includes(needle))
    : connectors

  const remove = async (connector: ConnectorRow) => {
    setActionError(null)
    try {
      await deleteConnector(connector.id)
    } catch (err) {
      setActionError(
        err instanceof Error ? err.message : "Could not remove the connector."
      )
    }
  }

  return (
    <PageShell
      title="Connectors"
      subtitle="Connect MCP servers to extend AI capabilities"
      error={actionError ?? error}
      onDismissError={() => {
        setActionError(null)
        clearConnectorsError()
      }}
      actions={
        <>
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="Search connectors..."
          />
          <Button
            variant="default"
            className="gap-2 rounded-full"
            onClick={() => setAdding(true)}
          >
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} className="size-4" />
            <span>New connector</span>
          </Button>
        </>
      }
    >
      {loading ? (
        <PageLoading />
      ) : visible.length === 0 ? (
        <PageEmpty
          icon={Plug01Icon}
          title={needle ? "No connectors match your search" : "No connectors yet"}
          hint={needle ? undefined : "Add an MCP server to get started"}
        />
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {visible.map((connector) => (
            <div
              key={connector.id}
              className="relative flex flex-col rounded-2xl border border-white/10 bg-white/5 p-4 transition-colors hover:border-white/20"
            >
              <div className="flex items-start justify-between">
                <div className="flex min-w-0 items-center gap-3">
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-white/5">
                    <HugeiconsIcon
                      icon={Plug01Icon}
                      strokeWidth={2}
                      className="size-4 text-white/50"
                    />
                  </div>
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-medium text-white/90">
                      {connector.name}
                    </h3>
                    <p
                      title={connector.command}
                      className="mt-0.5 truncate font-mono text-xs text-white/40"
                    >
                      {connector.command}
                    </p>
                  </div>
                </div>

                <div className="flex shrink-0 items-center gap-2">
                  <button
                    type="button"
                    onClick={() =>
                      void setConnectorEnabled(connector.id, !connector.enabled)
                    }
                    disabled={pending.includes(connector.id)}
                    title={connector.enabled ? "Disable" : "Enable"}
                    aria-label={`Turn ${connector.name} ${connector.enabled ? "off" : "on"}`}
                    className={cn(
                      "cursor-pointer rounded-lg p-1.5 transition-colors disabled:pointer-events-none disabled:opacity-50",
                      connector.enabled
                        ? "bg-emerald-500/10 text-emerald-400 hover:bg-emerald-500/20"
                        : "bg-white/5 text-white/30 hover:bg-white/10"
                    )}
                  >
                    <HugeiconsIcon
                      icon={connector.enabled ? PowerIcon : PowerOffIcon}
                      strokeWidth={2}
                      className="size-3.5"
                    />
                  </button>

                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <button
                          aria-label={`${connector.name} actions`}
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
                        variant="destructive"
                        onClick={() => void remove(connector)}
                      >
                        <span>Delete</span>
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </div>

              <div className="mt-auto flex items-center gap-2 pt-3">
                <span
                  className={cn(
                    "rounded-full px-1.5 py-0.5 text-[10px]",
                    connector.enabled
                      ? "bg-emerald-500/10 text-emerald-400"
                      : "bg-white/5 text-white/30"
                  )}
                >
                  {connector.enabled ? "Active" : "Inactive"}
                </span>
                <span className="text-[11px] text-white/30">
                  {formatDate(connector.createdAt)}
                </span>
              </div>
            </div>
          ))}
        </div>
      )}

      {adding ? (
        <ConnectorDialog
          onOpenChange={(next) => {
            if (!next) setAdding(false)
          }}
        />
      ) : null}
    </PageShell>
  )
}

import { useEffect, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  MoreHorizontalIcon,
} from "@hugeicons/core-free-icons"
import { createRoute, useNavigate } from "@tanstack/react-router"

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
import { NewWorkspaceDialog } from "@/components/workspace-dialog"
import {
  deleteWorkspace,
  loadWorkspaces,
  useWorkspacesState,
} from "@/lib/workspace-store"
import { notifyError } from "@/lib/toast"
import type { WorkspaceRow } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspaces",
  component: WorkspacesPage,
})

function formatDate(at: number): string {
  if (!at) return ""
  return new Date(at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

function WorkspacesPage() {
  const navigate = useNavigate()
  const { workspaces, loading } = useWorkspacesState()
  const [query, setQuery] = useState("")
  const [adding, setAdding] = useState(false)

  useEffect(() => {
    void loadWorkspaces()
  }, [])

  const needle = query.trim().toLowerCase()
  const visible = needle
    ? workspaces.filter((w) => w.name.toLowerCase().includes(needle))
    : workspaces

  const remove = async (workspace: WorkspaceRow) => {
    try {
      await deleteWorkspace(workspace.id)
    } catch (err) {
      notifyError(
        err instanceof Error ? err.message : "Could not remove the workspace.",
        "Could not remove the workspace"
      )
    }
  }

  return (
    <PageShell
      title="Workspaces"
      subtitle="Create, organize and manage your projects"
      actions={
        <>
          <SearchInput
            value={query}
            onChange={setQuery}
            placeholder="Search workspaces..."
          />
          <Button
            variant="default"
            className="gap-2 rounded-full"
            onClick={() => setAdding(true)}
          >
            <HugeiconsIcon icon={Add01Icon} strokeWidth={2} className="size-4" />
            <span>New workspace</span>
          </Button>
        </>
      }
    >
      {loading ? (
        <PageLoading />
      ) : visible.length === 0 ? (
        <PageEmpty
          icon={Add01Icon}
          title={needle ? "No workspaces match your search" : "No workspaces yet"}
          hint={needle ? undefined : "Create one to get started"}
        />
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {visible.map((workspace) => (
            <div
              key={workspace.id}
              role="button"
              tabIndex={0}
              aria-label={`Open ${workspace.name}`}
              onClick={() =>
                navigate({
                  to: "/workspaces/$workspaceId",
                  params: { workspaceId: workspace.id },
                })
              }
              onKeyDown={(event) => {
                if (event.key !== "Enter" && event.key !== " ") return
                event.preventDefault()
                navigate({
                  to: "/workspaces/$workspaceId",
                  params: { workspaceId: workspace.id },
                })
              }}
              className="relative flex cursor-pointer flex-col rounded-2xl border border-white/10 bg-white/5 p-4 transition-colors hover:border-white/20 focus-visible:border-white/20 focus-visible:outline-none"
            >
              <div
                className="absolute top-3 right-3"
                // The menu is its own control: opening it must not open the
                // workspace behind it.
                onClick={(event) => event.stopPropagation()}
                onKeyDown={(event) => event.stopPropagation()}
              >
                <DropdownMenu>
                  <DropdownMenuTrigger
                    render={
                      <button
                        aria-label={`${workspace.name} actions`}
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
                      disabled={workspace.builtin}
                      onClick={() => void remove(workspace)}
                    >
                      <span>Delete</span>
                    </DropdownMenuItem>
                  </DropdownMenuContent>
                </DropdownMenu>
              </div>

              <h3 className="pr-6 text-sm font-medium text-white/90">
                {workspace.name}
              </h3>
              {workspace.description ? (
                <p className="mt-1 line-clamp-2 text-xs text-white/50">
                  {workspace.description}
                </p>
              ) : null}
              <div className="mt-auto flex items-center gap-2 pt-3">
                <span className="text-[11px] text-white/30">
                  {formatDate(workspace.createdAt)}
                </span>
                {workspace.builtin ? (
                  <span className="text-[11px] text-white/30">Default</span>
                ) : null}
              </div>
            </div>
          ))}
        </div>
      )}

      {adding ? (
        <NewWorkspaceDialog
          onOpenChange={(next) => {
            if (!next) setAdding(false)
          }}
        />
      ) : null}
    </PageShell>
  )
}

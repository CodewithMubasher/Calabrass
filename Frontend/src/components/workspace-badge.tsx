import { useEffect } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  FolderCodeIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { updateChatWorkspace, useChat } from "@/lib/chat-store"
import { notifyError } from "@/lib/toast"
import {
  loadWorkspaces,
  setPreferredWorkspace,
  usePreferredWorkspaceId,
  useWorkspacesState,
} from "@/lib/workspace-store"
import type { WorkspaceRow } from "@/types/chat"

/**
 * The folder the AI works in for the current chat. Clicking it lists the
 * account's workspaces, and picking one moves the chat there. On the home
 * screen the pick is remembered for the next chat.
 *
 * The label is the folder's name, never its full path.
 */
export function WorkspaceBadge({ chatId }: { chatId: string }) {
  const { workspaces, loading } = useWorkspacesState()
  const preferredId = usePreferredWorkspaceId()
  const chat = useChat(chatId)

  useEffect(() => {
    void loadWorkspaces()
  }, [])

  const defaultWorkspace = workspaces.find((workspace) => workspace.builtin)
  const chatDir = chat?.workspaceDir ?? ""
  const preferred = workspaces.find((workspace) => workspace.id === preferredId)

  const current: WorkspaceRow | undefined = chatId
    ? chatDir
      ? workspaces.find((workspace) => workspace.dir === chatDir)
      : defaultWorkspace
    : preferred ?? defaultWorkspace

  const label = folderLabel(current, chatId ? chatDir : current?.dir ?? "")

  async function choose(workspaceId: string) {
    if (workspaceId === current?.id) return

    try {
      if (chatId) {
        await updateChatWorkspace(chatId, workspaceId)
      } else {
        setPreferredWorkspace(workspaceId)
      }
    } catch (err) {
      // updateChatWorkspace already put the reason on screen; home picks are
      // local and only fail if the menu itself did.
      if (!chatId) {
        notifyError(
          err instanceof Error ? err.message : "Could not pick the workspace.",
          "Could not pick the workspace"
        )
      }
    }
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <button
            type="button"
            aria-label={`Change workspace, currently ${label}`}
            title={current?.path || label}
            className="inline-flex h-6 max-w-[180px] cursor-pointer items-center gap-1 rounded-2xl bg-sky-50 px-2.5 text-xs font-medium text-sky-700 transition-colors hover:bg-sky-100 dark:bg-sky-950 dark:text-sky-300 dark:hover:bg-sky-900"
          />
        }
      >
        <HugeiconsIcon
          icon={FolderCodeIcon}
          strokeWidth={2}
          className="size-3 shrink-0"
        />
        <span className="truncate">{label}</span>
      </DropdownMenuTrigger>

      <DropdownMenuContent align="center" className="w-56">
        <DropdownMenuLabel>
          {chatId ? "Workspace for this chat" : "Workspace for a new chat"}
        </DropdownMenuLabel>
        {loading && workspaces.length === 0 ? (
          <DropdownMenuItem disabled>
            <span>Loading workspaces…</span>
          </DropdownMenuItem>
        ) : workspaces.length === 0 ? (
          <DropdownMenuItem disabled>
            <span>No workspaces yet</span>
          </DropdownMenuItem>
        ) : (
          workspaces.map((workspace) => (
            <DropdownMenuItem
              key={workspace.id}
              onClick={() => void choose(workspace.id)}
            >
              <HugeiconsIcon icon={FolderCodeIcon} strokeWidth={2} />
              <span className="truncate">{workspace.name}</span>
              {workspace.id === current?.id ? (
                <HugeiconsIcon
                  icon={Tick02Icon}
                  strokeWidth={2}
                  className="ml-auto text-sky-400"
                />
              ) : null}
            </DropdownMenuItem>
          ))
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

/** The badge's short name: the workspace's, or the last part of its folder. */
function folderLabel(workspace: WorkspaceRow | undefined, dir: string) {
  if (workspace) return `../${workspace.name}`
  const name = dir.split(/[\\/]/).filter(Boolean).pop()
  return name ? `../${name}` : "../Default"
}

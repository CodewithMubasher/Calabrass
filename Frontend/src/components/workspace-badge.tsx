import { useEffect, useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { FolderCodeIcon } from "@hugeicons/core-free-icons"

import { updateChatWorkspacePath, useChat } from "@/lib/chat-store"
import { notifyError } from "@/lib/toast"
import {
  loadWorkspaces,
  setPreferredWorkspaceDir,
  usePreferredWorkspaceDir,
  useWorkspacesState,
} from "@/lib/workspace-store"

interface FolderPickerWindow extends Window {
  go?: {
    main?: {
      FolderPicker?: {
        PickFolder?: (startIn?: string) => Promise<string>
      }
    }
  }
  showDirectoryPicker?: (options?: {
    mode?: "read" | "readwrite"
  }) => Promise<FileSystemDirectoryHandle>
}

/**
 * The folder the AI works in for the current chat. Clicking it opens the
 * desktop's native folder dialog, so the Go side gets the real path and the AI
 * and crew workers open that exact directory. On the home screen the pick is
 * remembered for the next chat.
 *
 * The label is the folder and the one above it, never the full path.
 */
export function WorkspaceBadge({ chatId }: { chatId: string }) {
  const { workspaces } = useWorkspacesState()
  const preferredDir = usePreferredWorkspaceDir()
  const chat = useChat(chatId)
  const [picking, setPicking] = useState(false)

  useEffect(() => {
    void loadWorkspaces()
  }, [])

  const defaultWorkspace = workspaces.find((workspace) => workspace.builtin)
  const currentDir = chatId ? chat?.workspaceDir ?? "" : preferredDir
  const isDefault =
    !currentDir ||
    currentDir === "default" ||
    currentDir === defaultWorkspace?.path
  const dir = currentDir || defaultWorkspace?.path || ""
  const label = isDefault ? "../workspace/default" : folderLabel(dir)

  async function pickFolder() {
    if (picking) return
    setPicking(true)

    try {
      const picked = await openFolderPicker(dir)
      if (!picked) return

      if (chatId) {
        await updateChatWorkspacePath(chatId, picked)
      } else {
        setPreferredWorkspaceDir(picked)
      }
    } catch (err) {
      // Closing the dialog is not a failure.
      if (err instanceof DOMException && err.name === "AbortError") return

      notifyError(
        err instanceof Error ? err.message : "Could not open the folder.",
        "Could not open the folder"
      )
    } finally {
      setPicking(false)
    }
  }

  return (
    <button
      type="button"
      onClick={() => void pickFolder()}
      disabled={picking}
      aria-label={`Change workspace, currently ${label}`}
      aria-busy={picking}
      title={dir || label}
      className="inline-flex h-6 max-w-[220px] cursor-pointer items-center gap-1 rounded-2xl bg-pink-50 px-2.5 text-xs font-medium text-pink-700 transition-colors hover:bg-pink-100 disabled:opacity-60 dark:bg-pink-950 dark:text-pink-300 dark:hover:bg-pink-900"
    >
      <HugeiconsIcon
        icon={FolderCodeIcon}
        strokeWidth={2}
        className="size-3 shrink-0"
      />
      <span className="truncate">{picking ? "Opening…" : label}</span>
    </button>
  )
}

/**
 * The native picker that returns a real path. A browser's
 * showDirectoryPicker() only gives a sandboxed handle with a name; that cannot
 * tell the Go side which folder to open, so the desktop binding is used first.
 */
async function openFolderPicker(startIn: string): Promise<string | null> {
  const picker = (window as FolderPickerWindow).go?.main?.FolderPicker?.PickFolder
  if (typeof picker === "function") {
    const path = await picker(startIn)
    return path?.trim() || null
  }

  const browserPicker = (window as FolderPickerWindow).showDirectoryPicker
  if (typeof browserPicker === "function") {
    // Open the native browser dialog, then explain why its result is not
    // enough: the page is never told the absolute path.
    await browserPicker.call(window, { mode: "readwrite" })
    throw new Error(
      "The browser hides the folder's path. Run the desktop app to open a real folder as the AI workspace."
    )
  }

  throw new Error("This app cannot open a folder picker here.")
}

/**
 * The badge's short name: the folder and the one above it, never the full
 * path. `C:\Users\me\Desktop\Calabrass` reads as `../Desktop/Calabrass`, and a
 * workspace folder inside the library reads as `../workspace/landing-page`.
 */
function folderLabel(dir: string) {
  if (!dir) return "../workspace/default"

  const absolute =
    /^[a-zA-Z]:[\\/]/.test(dir) || dir.startsWith("/") || dir.startsWith("\\\\")
  if (!absolute) {
    return dir === "default" ? "../workspace/default" : `../workspace/${dir}`
  }

  const parts = dir.split(/[\\/]+/).filter(Boolean)
  if (parts.length >= 2) {
    return `../${parts[parts.length - 2]}/${parts[parts.length - 1]}`
  }
  if (parts.length === 1) return `../${parts[0]}`
  return "../workspace/default"
}

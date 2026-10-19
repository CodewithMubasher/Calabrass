import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Cancel01Icon,
  Folder01Icon,
  GitBranchIcon,
  Search01Icon,
} from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty } from "@/components/page-shell"
import { PromptInput } from "@/components/prompt-input"
import { Alert, AlertAction } from "@/components/ui/alert"
import {
  Attachment,
  AttachmentBadge,
  AttachmentContent,
  AttachmentDescription,
  AttachmentTitle,
} from "@/components/ui/attachment"
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb"
import { createChat, sendMessage, useChats } from "@/lib/chat-store"
import { api } from "@/lib/api"
import { useWorkspacesState } from "@/lib/workspace-store"
import type { WorkspaceGitInfo } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspaces/$workspaceId",
  component: WorkspaceRoute,
})

// Keyed by the workspace so the per-workspace cards below start fresh instead
// of remembering the folder the user was last on.
function WorkspaceRoute() {
  const { workspaceId } = Route.useParams()
  return <WorkspaceView key={workspaceId} workspaceId={workspaceId} />
}

/** Context files live on this machine, keyed by workspace. */
interface ContextFile {
  id: string
  name: string
  size: number
}

const ACCEPTED_EXTENSIONS = [
  ".txt", ".md", ".json", ".csv", ".py", ".js", ".ts", ".tsx", ".jsx",
  ".html", ".css", ".go", ".rs", ".java", ".c", ".cpp", ".yml", ".yaml",
  ".pdf", ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".log",
]

function readJson<T>(key: string, fallback: T): T {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return fallback
    return JSON.parse(raw) as T
  } catch {
    return fallback
  }
}

function writeJson(key: string, value: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage is best effort; the cards still work for this session.
  }
}

function isAcceptedFile(name: string) {
  return ACCEPTED_EXTENSIONS.includes("." + name.split(".").pop()?.toLowerCase())
}

function extensionBadge(name: string) {
  return name.split(".").pop()?.toUpperCase() ?? "FILE"
}

function formatFileSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(at: number) {
  if (!at) return ""
  return new Date(at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
  })
}

/**
 * One workspace: a composer that starts a chat inside it, its recents, and the
 * two cards beside them (git, context).
 */
function WorkspaceView({ workspaceId }: { workspaceId: string }) {
  const navigate = useNavigate()
  const { workspaces, loading, error: listError } = useWorkspacesState()
  const chats = useChats()

  const workspace = useMemo(
    () => workspaces.find((item) => item.id === workspaceId),
    [workspaces, workspaceId]
  )

  const [error, setError] = useState<string | null>(null)
  const [sending, setSending] = useState(false)
  const [git, setGit] = useState<WorkspaceGitInfo | null>(null)
  const [gitError, setGitError] = useState<string | null>(null)
  const [contextFiles, setContextFiles] = useState<ContextFile[]>(() =>
    readJson<ContextFile[]>(`calabrass-context-${workspaceId}`, [])
  )
  const fileInputRef = useRef<HTMLInputElement>(null)

  const workspaceChats = useMemo(
    () =>
      workspace
        ? chats.filter((chat) => chat.workspaceDir === workspace.dir)
        : [],
    [chats, workspace]
  )

  // The Git card describes the folder's own repository rather than a remembered
  // URL, so the server is asked to read it each time the workspace is opened.
  useEffect(() => {
    if (!workspace) return
    let live = true
    api
      .getWorkspaceGit(workspace.id)
      .then((info) => {
        if (!live) return
        setGit(info)
        setGitError(null)
      })
      .catch((err) => {
        if (!live) return
        setGitError(
          err instanceof Error ? err.message : "Could not read the repository."
        )
      })
    return () => {
      live = false
    }
  }, [workspace])

  async function handleSend(message: string) {
    if (!workspace || sending) return

    setSending(true)
    setError(null)
    try {
      // The chat is created in this workspace, so the agent works here from the
      // first message on.
      const chat = await createChat(workspace.id)
      await navigate({ to: "/chat/$chatId", params: { chatId: chat.id } })
      void sendMessage(chat.id, message)
    } catch (err) {
      setError(
        err instanceof Error
          ? err.message
          : "Could not start a chat in this workspace."
      )
    } finally {
      setSending(false)
    }
  }

  function addContextFiles(added: ContextFile[]) {
    if (added.length === 0) return
    const next = [...contextFiles, ...added]
    setContextFiles(next)
    writeJson(`calabrass-context-${workspaceId}`, next)
  }

  function removeContextFile(id: string) {
    const next = contextFiles.filter((file) => file.id !== id)
    setContextFiles(next)
    writeJson(`calabrass-context-${workspaceId}`, next)
  }

  const title = workspace?.name ?? "Untitled"

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="size-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
      </div>
    )
  }

  return (
    <div className="scrollbar-thin flex h-full flex-col overflow-auto">
      <div className="shrink-0 px-14 pt-8">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink
                render={
                  <button
                    type="button"
                    aria-label="Back to workspaces"
                    className="cursor-pointer"
                  />
                }
                onClick={() => navigate({ to: "/workspaces" })}
              >
                Workspaces
              </BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="text-white/70">{title}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </div>

      {error || listError ? (
        <Alert variant="destructive" className="mx-14 mt-4 shrink-0">
          <AlertAction
            onClick={() => setError(null)}
            aria-label="Dismiss"
          >
            <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
          </AlertAction>
          {error ?? listError}
        </Alert>
      ) : null}

      {!workspace ? (
        <div className="flex-1">
          <PageEmpty
            icon={Folder01Icon}
            title="This workspace no longer exists"
            hint="It may have been removed from the list"
          />
        </div>
      ) : (
        <div className="mt-8 flex items-start gap-10 px-14">
          <div className="flex min-w-0 flex-1 flex-col">
            <div className="-mt-8">
              <PromptInput
                onSend={handleSend}
                heading={`Ready to work on ${title}`}
                compact
                disabled={sending}
              />
            </div>

            <div className="mt-8">
              <h3 className="mb-3 text-xs font-medium text-white/40">Recents</h3>
              {workspaceChats.length === 0 ? (
                <p className="text-xs text-white/30">
                  No chats yet in this workspace
                </p>
              ) : (
                <div className="space-y-1">
                  {workspaceChats.map((chat) => (
                    <Attachment
                      key={chat.id}
                      orientation="horizontal"
                      className="min-h-0 w-full min-w-0 cursor-pointer bg-transparent transition-colors hover:bg-white/2"
                      onClick={() =>
                        navigate({
                          to: "/chat/$chatId",
                          params: { chatId: chat.id },
                        })
                      }
                    >
                      <AttachmentContent className="flex w-full items-center justify-between px-3 py-0.5">
                        <AttachmentTitle className="truncate text-sm text-white/90">
                          {chat.title}
                        </AttachmentTitle>
                        <AttachmentDescription className="ml-3 shrink-0">
                          {formatDate(chat.createdAt)}
                        </AttachmentDescription>
                      </AttachmentContent>
                    </Attachment>
                  ))}
                </div>
              )}
            </div>
          </div>

          <div className="mt-20 flex w-[320px] flex-col gap-3">
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">
                  Git &amp; Github
                </h3>
                <HugeiconsIcon
                  icon={GitBranchIcon}
                  strokeWidth={2}
                  className="size-4 text-white/40"
                />
              </div>

              <div className="space-y-3">
                {gitError ? (
                  <p className="text-xs text-red-400">{gitError}</p>
                ) : !git ? (
                  <p className="text-xs text-white/30">
                    Reading the repository…
                  </p>
                ) : !git.isRepo ? (
                  <p className="text-xs text-white/40">
                    This folder is not a git repository yet.
                  </p>
                ) : (
                  <>
                    <div>
                      <span className="mb-1 block text-xs text-white/40">
                        Git remote url
                      </span>
                      <p
                        title={git.remoteUrl}
                        className="truncate text-sm text-white/70"
                      >
                        {git.remoteUrl || "No remote configured"}
                      </p>
                    </div>

                    <div className="flex items-center justify-between text-xs">
                      <span className="text-white/40">Total commits</span>
                      <span className="font-medium text-white/70">
                        {git.totalCommits}
                      </span>
                    </div>

                    {git.latestCommit ? (
                      <div className="space-y-1.5">
                        <span className="text-xs text-white/40">
                          Latest commit
                        </span>
                        <div className="flex items-start justify-between gap-2">
                          <div className="flex min-w-0 items-start gap-2">
                            <span className="shrink-0 font-mono text-xs text-primary">
                              {git.latestCommit.hash}
                            </span>
                            <span className="line-clamp-2 text-xs text-white/60">
                              {git.latestCommit.message}
                            </span>
                          </div>
                          <span className="shrink-0 text-xs text-white/30">
                            {formatDate(Date.parse(git.latestCommit.date))}
                          </span>
                        </div>
                      </div>
                    ) : (
                      <p className="text-xs text-white/30">No commits yet.</p>
                    )}
                  </>
                )}
              </div>
            </div>

            <div className="flex flex-col rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">Context</h3>
                <div className="flex items-center gap-1">
                  <button
                    type="button"
                    className="cursor-pointer rounded-lg p-1 transition-colors hover:bg-white/10"
                  >
                    <HugeiconsIcon
                      icon={Search01Icon}
                      strokeWidth={2}
                      className="size-4 text-white/50"
                    />
                  </button>
                  <button
                    type="button"
                    className="cursor-pointer rounded-lg p-1 transition-colors hover:bg-white/10"
                    onClick={() => fileInputRef.current?.click()}
                  >
                    <HugeiconsIcon
                      icon={Add01Icon}
                      strokeWidth={2}
                      className="size-4 text-white/50"
                    />
                  </button>
                </div>
              </div>
              <input
                ref={fileInputRef}
                type="file"
                multiple
                accept={ACCEPTED_EXTENSIONS.join(",")}
                onChange={(event) => {
                  const added: ContextFile[] = Array.from(
                    event.target.files ?? []
                  )
                    .filter((file) => isAcceptedFile(file.name))
                    .map((file) => ({
                      id: crypto.randomUUID(),
                      name: file.name,
                      size: file.size,
                    }))
                  addContextFiles(added)
                  event.target.value = ""
                }}
                className="hidden"
              />
              {contextFiles.length === 0 ? (
                <p className="text-xs text-white/30">No files added yet</p>
              ) : (
                <div className="grid grid-cols-2 gap-2">
                  {contextFiles.map((file) => (
                    <Attachment
                      key={file.id}
                      orientation="vertical"
                      className="w-full cursor-pointer bg-transparent transition-colors hover:bg-white/5"
                    >
                      <AttachmentContent className="relative px-2 py-1.5">
                        <button
                          type="button"
                          onClick={() => removeContextFile(file.id)}
                          aria-label={`Remove ${file.name}`}
                          className="absolute top-1 right-1 cursor-pointer rounded-md p-0.5 opacity-0 transition-all group-hover/attachment:opacity-100 hover:bg-white/10"
                        >
                          <HugeiconsIcon
                            icon={Cancel01Icon}
                            strokeWidth={2}
                            className="size-3 text-white/50"
                          />
                        </button>
                        <AttachmentTitle className="line-clamp-2 text-white/90">
                          {file.name}
                        </AttachmentTitle>
                        <AttachmentDescription className="text-white/50">
                          {formatFileSize(file.size)}
                        </AttachmentDescription>
                        <AttachmentBadge className="border-white/10 bg-white/5 text-white/50">
                          {extensionBadge(file.name)}
                        </AttachmentBadge>
                      </AttachmentContent>
                    </Attachment>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

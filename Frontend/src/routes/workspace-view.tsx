import { useEffect, useMemo, useRef, useState } from "react"
import { useNavigate } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Add01Icon,
  Cancel01Icon,
  ComputerTerminal01Icon,
  Folder01Icon,
  GitBranchIcon,
  Search01Icon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"

import { PageEmpty } from "@/components/page-shell"
import { PromptInput } from "@/components/prompt-input"
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
import { setComposerMode, useComposerMode } from "@/lib/mode-store"
import { useWorkspacesState } from "@/lib/workspace-store"
import { notifyError } from "@/lib/toast"
import type { WorkspaceGitInfo } from "@/types/chat"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/workspaces/$workspaceId",
  component: WorkspaceRoute,
})

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
 * three cards beside them (git, context, command).
 */
function WorkspaceView({ workspaceId }: { workspaceId: string }) {
  const navigate = useNavigate()
  const { workspaces, loading } = useWorkspacesState()
  const chats = useChats()

  const workspace = useMemo(
    () => workspaces.find((item) => item.id === workspaceId),
    [workspaces, workspaceId]
  )

  const [sending, setSending] = useState(false)
  const [git, setGit] = useState<WorkspaceGitInfo | null>(null)
  const [remoteDraft, setRemoteDraft] = useState("")
  const [cloning, setCloning] = useState(false)
  const [cmdDraft, setCmdDraft] = useState("")
  const [cmdExitCode, setCmdExitCode] = useState<number | null>(null)
  const [cmdRunning, setCmdRunning] = useState(false)
  // The mode the composer is in: a crew run opens under its own address.
  const mode = useComposerMode()
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

  useEffect(() => {
    setRemoteDraft("")
    if (!workspace) return
    let live = true
    api
      .getWorkspaceGit(workspace.id)
      .then((info) => {
        if (!live) return
        setGit(info)
      })
      .catch((err) => {
        if (!live) return
        notifyError(
          err instanceof Error ? err.message : "Could not read the repository.",
          "Could not read the repository"
        )
      })
    return () => {
      live = false
    }
  }, [workspace])

  async function cloneRepo() {
    if (!workspace) return
    const remote = remoteDraft.trim()
    if (remote === "") return
    setCloning(true)
    try {
      setGit(await api.cloneWorkspaceRepo(workspace.id, remote))
    } catch (err) {
      notifyError(
        err instanceof Error ? err.message : "Could not clone the repository.",
        "Could not clone the repository"
      )
    } finally {
      setCloning(false)
    }
  }

  async function runCommand() {
    if (!workspace || cmdRunning) return
    const command = cmdDraft.trim()
    if (command === "") return

    setCmdRunning(true)
    setCmdExitCode(null)
    try {
      const result = await api.runWorkspaceCommand(workspace.id, command)
      setCmdExitCode(result.exitCode)
    } catch (err) {
      setCmdExitCode(-1)
    } finally {
      setCmdRunning(false)
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
                onSend={async (message) => {
                  if (!workspace || sending) return
                  setSending(true)
                  try {
                    const chat = await createChat(workspace.id, undefined, mode)
                    await navigate({
                      to: mode === "crew" ? "/crew/$chatId" : "/chat/$chatId",
                      params: { chatId: chat.id },
                    })
                    void sendMessage(chat.id, message)
                  } catch (err) {
                    notifyError(
                      err instanceof Error
                        ? err.message
                        : "Could not start a chat in this workspace.",
                      "Could not start the chat"
                    )
                  } finally {
                    setSending(false)
                  }
                }}
                heading={`Ready to work on ${title}`}
                compact
                disabled={sending}
                mode={mode}
                onModeChange={setComposerMode}
              />
            </div>

            <div className="mt-8">
              <h3 className="mb-3 text-xs font-medium text-white/40">Recents</h3>
              {workspaceChats.length === 0 ? (
                <p className="text-xs text-white/30">No chats yet in this workspace</p>
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
            {/* Git Card */}
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">Git & Github</h3>
                <HugeiconsIcon icon={GitBranchIcon} strokeWidth={2} className="size-4 text-white/40" />
              </div>
              <div className="space-y-3">
                {!git ? (
                  <p className="text-xs text-white/30">Reading the repository…</p>
                ) : git.isRepo ? (
                  <>
                    <div>
                      <span className="mb-1 block text-xs text-white/40">Git remote url</span>
                      <div className="flex items-center justify-between gap-2">
                        <span title={git.remoteUrl} className="truncate text-sm text-white/70">
                          {git.remoteUrl || "No remote configured"}
                        </span>
                        {git.visibility ? (
                          <span className="shrink-0 rounded-full border border-white/10 px-2 py-0.5 text-[10px] text-white/50">
                            {git.visibility === "private" ? "Private" : "Public"}
                          </span>
                        ) : null}
                      </div>
                    </div>
                    <div className="flex items-center justify-between text-xs">
                      <span className="text-white/40">Total commits</span>
                      <span className="font-medium text-white/70">{git.totalCommits}</span>
                    </div>
                    {git.latestCommit ? (
                      <div className="space-y-1.5">
                        <span className="text-xs text-white/40">Latest commit</span>
                        <div className="flex items-start justify-between gap-2">
                          <div className="flex min-w-0 items-start gap-2">
                            <span className="shrink-0 font-mono text-xs text-primary">{git.latestCommit.hash}</span>
                            <span className="line-clamp-2 text-xs text-white/60">{git.latestCommit.message}</span>
                          </div>
                          <span className="shrink-0 text-xs text-white/30">{formatDate(Date.parse(git.latestCommit.date))}</span>
                        </div>
                      </div>
                    ) : (
                      <p className="text-xs text-white/30">No commits yet.</p>
                    )}
                  </>
                ) : (
                  <div>
                    <label htmlFor="git-clone-url" className="mb-1 block text-xs text-white/40">Git remote url</label>
                    <input
                      id="git-clone-url"
                      type="url"
                      placeholder="https://github.com/user/repo.git"
                      value={remoteDraft}
                      onChange={(event) => setRemoteDraft(event.target.value)}
                      onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); void cloneRepo() } }}
                      disabled={cloning}
                      autoComplete="off"
                      spellCheck={false}
                      className="w-full rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-white/80 transition-colors outline-none placeholder:text-white/30 focus:border-white/20 disabled:opacity-60"
                    />
                    {cloning ? (
                      <p className="mt-1 text-xs text-white/40">Cloning…</p>
                    ) : (
                      <p className="mt-1 text-xs text-white/30">Press Enter to clone it into this workspace</p>
                    )}
                  </div>
                )}
              </div>
            </div>

            {/* Context Card */}
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">Context</h3>
                <div className="flex items-center gap-1">
                  <button type="button" className="cursor-pointer rounded-lg p-1 transition-colors hover:bg-white/10">
                    <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="size-4 text-white/50" />
                  </button>
                  <button type="button" className="cursor-pointer rounded-lg p-1 transition-colors hover:bg-white/10" onClick={() => fileInputRef.current?.click()}>
                    <HugeiconsIcon icon={Add01Icon} strokeWidth={2} className="size-4 text-white/50" />
                  </button>
                </div>
              </div>
              <input ref={fileInputRef} type="file" multiple accept={ACCEPTED_EXTENSIONS.join(",")} onChange={(event) => {
                const added: ContextFile[] = Array.from(event.target.files ?? []).filter((file) => isAcceptedFile(file.name)).map((file) => ({ id: crypto.randomUUID(), name: file.name, size: file.size }))
                addContextFiles(added)
                event.target.value = ""
              }} className="hidden" />
              {contextFiles.length === 0 ? (
                <p className="text-xs text-white/30">No files added yet</p>
              ) : (
                <div className="grid grid-cols-2 gap-2">
                  {contextFiles.map((file) => (
                    <Attachment key={file.id} orientation="vertical" className="w-full cursor-pointer bg-transparent transition-colors hover:bg-white/5">
                      <AttachmentContent className="relative px-2 py-1.5">
                        <button type="button" onClick={() => removeContextFile(file.id)} aria-label={`Remove ${file.name}`} className="absolute top-1 right-1 cursor-pointer rounded-md p-0.5 opacity-0 transition-all group-hover/attachment:opacity-100 hover:bg-white/10">
                          <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} className="size-3 text-white/50" />
                        </button>
                        <AttachmentTitle className="line-clamp-2 text-white/90">{file.name}</AttachmentTitle>
                        <AttachmentDescription className="text-white/50">{formatFileSize(file.size)}</AttachmentDescription>
                        <AttachmentBadge className="border-white/10 bg-white/5 text-white/50">{extensionBadge(file.name)}</AttachmentBadge>
                      </AttachmentContent>
                    </Attachment>
                  ))}
                </div>
              )}
            </div>

            {/* Command Card */}
            <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
              <div className="mb-3 flex items-center justify-between">
                <h3 className="text-sm font-medium text-white/90">Command</h3>
                <HugeiconsIcon icon={ComputerTerminal01Icon} strokeWidth={2} className="size-4 text-white/40" />
              </div>
              <div className="space-y-3">
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    placeholder="Enter command (e.g., npm install, dir, git status)"
                    value={cmdDraft}
                    onChange={(event) => setCmdDraft(event.target.value)}
                    onKeyDown={(event) => { if (event.key === "Enter" && !cmdRunning) { event.preventDefault(); void runCommand() } }}
                    disabled={cmdRunning}
                    autoComplete="off"
                    spellCheck={false}
                    className="flex-1 rounded-lg border border-white/10 bg-white/5 px-3 py-1.5 text-sm text-white/80 transition-colors outline-none placeholder:text-white/30 focus:border-white/20 disabled:opacity-60"
                  />
                  {cmdRunning ? (
                    <button type="button" disabled className="shrink-0 rounded-lg p-2 text-white/40" aria-label="Running">
                      <HugeiconsIcon icon={Search01Icon} strokeWidth={2} className="size-4 animate-spin" />
                    </button>
                  ) : cmdExitCode !== null ? (
                    <button type="button" className={`shrink-0 rounded-lg p-2 transition-colors ${cmdExitCode === 0 ? "text-emerald-400 hover:bg-emerald-500/10" : "text-red-400 hover:bg-red-500/10"}`} aria-label={cmdExitCode === 0 ? "Success" : "Failed"}>
                      <HugeiconsIcon icon={cmdExitCode === 0 ? Tick02Icon : Cancel01Icon} strokeWidth={2} className="size-4" />
                    </button>
                  ) : null}
                </div>
                {cmdExitCode !== null && (
                  <div className="rounded-lg border border-white/10 bg-white/5 p-3">
                    <span className={`text-xs font-medium ${cmdExitCode === 0 ? "text-emerald-400" : "text-red-400"}`}>
                      {cmdExitCode === 0 ? "Exit code: 0 (success)" : `Exit code: ${cmdExitCode} (failed)`}
                    </span>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
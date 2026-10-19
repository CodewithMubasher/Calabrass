import type {
  AgentInput,
  AgentRow,
  Chat,
  ChatDetail,
  ChatMessage,
  ConnectorInput,
  ConnectorRow,
  ModelSelection,
  ProviderInfo,
  ProviderInput,
  ProviderRow,
  ScheduledTask,
  ScheduledTaskInput,
  ScheduledTaskPatch,
  TurnTrace,
  Usage,
  UsageSnapshot,
  WorkspaceGitInfo,
  WorkspaceInput,
  WorkspaceRow,
} from "@/types/chat"
import { getToken, type AuthSession, type AuthUser } from "@/lib/session"
import { clearSession } from "@/lib/session"
import { navigate } from "@/lib/navigation"

const BASE = import.meta.env.VITE_API_BASE || "/api"

export class ApiError extends Error {
  status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = "ApiError"
    this.status = status
  }
}

async function readError(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string }
    if (body?.error) return body.error
  } catch {
    // Non-JSON error body; fall through to the generic message.
  }
  return `Request failed with status ${res.status}`
}

let isHandling401 = false

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const token = getToken()
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init?.headers,
    },
  })
  if (!res.ok) {
    const message = await readError(res)
    const error = new ApiError(message, res.status)
    // Global 401 handling: if any API call returns 401, the session is invalid.
    // Clear local session and redirect to login (once per session).
    if (res.status === 401 && !isHandling401) {
      isHandling401 = true
      clearSession()
      // Use a small delay to let the current render complete, then navigate.
      setTimeout(() => {
        navigate({ to: "/login", replace: true })
        isHandling401 = false
      }, 0)
    }
    throw error
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

/** Query for the artifact endpoints: a chat scopes them to its workspace, and
 *  no chat means the account's default one. The bearer token is a query
 *  parameter because EventSource and download links cannot set headers. */
function artifactQuery(chatId: string, extra: Record<string, string> = {}) {
  const params = new URLSearchParams(extra)
  if (chatId) params.set("chatId", chatId)
  const token = getToken()
  if (token) params.set("access_token", token)
  return params.toString()
}

export const api = {
  health: () => request<{ status: string; providers: Record<string, boolean> }>("/health"),

  signUp: (input: { name: string; email: string; password: string }) =>
    request<AuthSession>("/auth/signup", {
      method: "POST",
      body: JSON.stringify(input),
    }),

  signIn: (input: { email: string; password: string }) =>
    request<AuthSession>("/auth/login", {
      method: "POST",
      body: JSON.stringify(input),
    }),

  me: () => request<{ user: AuthUser }>("/auth/me"),

  signOut: (token: string) =>
    request<void>("/auth/logout", {
      method: "POST",
      headers: { Authorization: `Bearer ${token}` },
    }),

  /** A fresh usage snapshot: totals, allowances and the last seven days. */
  getUsage: () => request<UsageSnapshot>("/usage"),

  listModels: () =>
    request<{ providers: ProviderInfo[] }>("/models").then((r) => r.providers ?? []),

  listProviders: () =>
    request<{ providers: ProviderRow[] }>("/providers").then(
      (r) => r.providers ?? []
    ),

  updateProvider: (providerId: string, input: ProviderInput) =>
    request<{ providers: ProviderRow[] }>(`/providers/${providerId}`, {
      method: "PUT",
      body: JSON.stringify(input),
    }).then((r) => r.providers ?? []),

  createProvider: (input: ProviderInput) =>
    request<{ providers: ProviderRow[] }>("/providers", {
      method: "POST",
      body: JSON.stringify(input),
    }).then((r) => r.providers ?? []),

  deleteProvider: (providerId: string) =>
    request<{ providers: ProviderRow[] }>(`/providers/${providerId}`, {
      method: "DELETE",
    }).then((r) => r.providers ?? []),

  listWorkspaces: () =>
    request<{ workspaces: WorkspaceRow[] }>("/workspaces").then(
      (r) => r.workspaces ?? []
    ),

  createWorkspace: (input: WorkspaceInput) =>
    request<{ workspaces: WorkspaceRow[] }>("/workspaces", {
      method: "POST",
      body: JSON.stringify(input),
    }).then((r) => r.workspaces ?? []),

  deleteWorkspace: (workspaceId: string) =>
    request<{ workspaces: WorkspaceRow[] }>(`/workspaces/${workspaceId}`, {
      method: "DELETE",
    }).then((r) => r.workspaces ?? []),

  /** A workspace folder's own git state: remote, commit count and latest commit. */
  getWorkspaceGit: (workspaceId: string) =>
    request<WorkspaceGitInfo>(`/workspaces/${workspaceId}/git`),

  /** Clone a repository into a workspace, and answer with the state that follows. */
  cloneWorkspaceRepo: (workspaceId: string, remoteUrl: string) =>
    request<WorkspaceGitInfo>(`/workspaces/${workspaceId}/git/clone`, {
      method: "POST",
      body: JSON.stringify({ remoteUrl }),
    }),

  listChats: () => request<{ chats: Chat[] }>("/chats").then((r) => r.chats ?? []),

  getChat: (chatId: string) =>
    request<{ chat: ChatDetail }>(`/chats/${chatId}`).then((r) => r.chat),

  createChat: (selection?: ModelSelection | null, workspaceId?: string, agentId?: string, mode?: "build" | "plan") =>
    request<{ chat: Chat }>("/chats", {
      method: "POST",
      body: JSON.stringify({
        ...(selection ?? {}),
        ...(workspaceId ? { workspaceId } : {}),
        ...(agentId ? { agentId } : {}),
        ...(mode ? { mode } : {}),
      }),
    }).then((r) => r.chat),

  updateChat: (chatId: string, patch: { title?: string; pinned?: boolean; mode?: "build" | "plan" }) =>
    request<{ chat: Chat }>(`/chats/${chatId}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }).then((r) => r.chat),

  listConnectors: () =>
    request<{ connectors: ConnectorRow[] }>("/connectors").then(
      (r) => r.connectors ?? []
    ),

  createConnector: (input: ConnectorInput) =>
    request<{ connectors: ConnectorRow[] }>("/connectors", {
      method: "POST",
      body: JSON.stringify(input),
    }).then((r) => r.connectors ?? []),

  setConnectorEnabled: (connectorId: string, enabled: boolean) =>
    request<{ connectors: ConnectorRow[] }>(`/connectors/${connectorId}`, {
      method: "PATCH",
      body: JSON.stringify({ enabled }),
    }).then((r) => r.connectors ?? []),

  deleteConnector: (connectorId: string) =>
    request<{ connectors: ConnectorRow[] }>(`/connectors/${connectorId}`, {
      method: "DELETE",
    }).then((r) => r.connectors ?? []),

  listAgents: () =>
    request<{ agents: AgentRow[] }>("/agents").then((r) => r.agents ?? []),

  getAgent: (agentId: string) =>
    request<{ agent: AgentRow }>(`/agents/${agentId}`).then((r) => r.agent),

  createAgent: (input: AgentInput) =>
    request<{ agents: AgentRow[] }>("/agents", {
      method: "POST",
      body: JSON.stringify(input),
    }).then((r) => r.agents ?? []),

  saveAgent: (agentId: string, input: AgentInput) =>
    request<{ agents: AgentRow[] }>(`/agents/${agentId}`, {
      method: "PUT",
      body: JSON.stringify(input),
    }).then((r) => r.agents ?? []),

  deleteAgent: (agentId: string) =>
    request<{ agents: AgentRow[] }>(`/agents/${agentId}`, {
      method: "DELETE",
    }).then((r) => r.agents ?? []),

  deleteMessage: (chatId: string, messageId: string) =>
    request<{ deleted: boolean }>(`/chats/${chatId}/messages/${messageId}`, {
      method: "DELETE",
    }),

  listSchedules: () =>
    request<{ tasks: ScheduledTask[] }>("/schedules").then((r) => r.tasks ?? []),

  createSchedule: (input: ScheduledTaskInput) =>
    request<{ tasks: ScheduledTask[] }>("/schedules", {
      method: "POST",
      body: JSON.stringify(input),
    }).then((r) => r.tasks ?? []),

  updateSchedule: (taskId: string, patch: ScheduledTaskPatch) =>
    request<{ tasks: ScheduledTask[] }>(`/schedules/${taskId}`, {
      method: "PATCH",
      body: JSON.stringify(patch),
    }).then((r) => r.tasks ?? []),

  deleteSchedule: (taskId: string) =>
    request<{ tasks: ScheduledTask[] }>(`/schedules/${taskId}`, {
      method: "DELETE",
    }).then((r) => r.tasks ?? []),

  /** The task list as a stream: it arrives complete, and again on every change.
   *  EventSource cannot send an Authorization header, so the token rides in the
   *  URL. */
  scheduleStreamUrl: () => {
    const params = new URLSearchParams()
    const token = getToken()
    if (token) params.set("access_token", token)
    const query = params.toString()
    return `${BASE}/schedules/stream${query ? `?${query}` : ""}`
  },

  deleteChat: (chatId: string) =>
    request<{ deleted: boolean }>(`/chats/${chatId}`, { method: "DELETE" }),

  /** The workspace listing, as a stream: it arrives complete, and again as soon
   *  as the folder changes. */
  artifactStreamUrl: (chatId: string) =>
    `${BASE}/artifacts/stream?${artifactQuery(chatId)}`,

  /** A file's text for the detail view. A binary file answers with
   *  `binary: true` instead of bytes, and a long one is cut short. */
  readArtifact: (chatId: string, path: string) =>
    request<{
      binary: boolean
      content: string
      truncated: boolean
      size: number
      preview?: string
      sheets?: Array<{ name: string; rows: string[][] }>
    }>(`/artifacts/content?${artifactQuery(chatId, { path })}`),

  /** One file as it is on disk, which is what the browser saves. */
  artifactFileUrl: (chatId: string, path: string) =>
    `${BASE}/artifacts/file?${artifactQuery(chatId, { path })}`,

  /** Every file in the workspace, in one archive. */
  artifactsArchiveUrl: (chatId: string) =>
    `${BASE}/artifacts/archive?${artifactQuery(chatId)}`,
}

/**
 * Start a download without leaving the page. The artifact endpoints answer with
 * Content-Disposition, so the browser saves the response rather than navigating
 * to it — a link is all it takes.
 */
export function startDownload(url: string) {
  const link = document.createElement("a")
  link.href = url
  link.rel = "noopener"
  document.body.append(link)
  link.click()
  link.remove()
}

export interface StreamHandlers {
  onChat?: (chat: Chat) => void
  onMessage?: (message: ChatMessage) => void
  onReasoning?: (text: string) => void
  onToken?: (text: string) => void
  /** A whole snapshot of the turn's trace, re-sent on every change. */
  onTrace?: (trace: TurnTrace) => void
  onDone?: (payload: {
    message: ChatMessage
    usage: Usage
    interrupted: boolean
    /** How many model turns the harness used. */
    steps?: number
    /** The harness ran out of steps mid-task. */
    stepsExhausted?: boolean
  }) => void
  onError?: (message: string) => void
}

/**
 * POST a user turn and consume the Server-Sent Events reply stream.
 * Events: chat, message, reasoning*, token*, trace*, done|error.
 *
 * With `regenerate`, the backend drops the trailing assistant reply and writes
 * a new one from the same user turn; no user message or `message` event is
 * produced.
 */
export async function streamMessage(
  chatId: string,
  body: { content: string; regenerate?: boolean } & Partial<ModelSelection>,
  handlers: StreamHandlers,
  signal?: AbortSignal
): Promise<void> {
  const token = getToken()
  const res = await fetch(`${BASE}/chats/${chatId}/messages`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify(body),
    signal,
  })

  if (!res.ok) {
    throw new ApiError(await readError(res), res.status)
  }
  if (!res.body) {
    throw new ApiError("Streaming is not supported in this browser.", 0)
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })

    let boundary = buffer.indexOf("\n\n")
    while (boundary !== -1) {
      dispatchSSE(buffer.slice(0, boundary), handlers)
      buffer = buffer.slice(boundary + 2)
      boundary = buffer.indexOf("\n\n")
    }
  }

  if (buffer.trim() !== "") {
    dispatchSSE(buffer, handlers)
  }
}

function dispatchSSE(frame: string, handlers: StreamHandlers) {
  let name = "message"
  const dataLines: string[] = []

  for (const rawLine of frame.split("\n")) {
    const line = rawLine.replace(/\r$/, "")
    if (line === "" || line.startsWith(":")) continue

    if (line.startsWith("event:")) {
      name = line.slice(6).trim()
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice(5).replace(/^ /, ""))
    }
  }

  if (dataLines.length === 0) return

  let payload: unknown
  try {
    payload = JSON.parse(dataLines.join("\n"))
  } catch {
    return
  }

  switch (name) {
    case "chat":
      handlers.onChat?.(payload as Chat)
      break
    case "message":
      handlers.onMessage?.(payload as ChatMessage)
      break
    case "reasoning":
      handlers.onReasoning?.((payload as { text: string }).text ?? "")
      break
    case "token":
      handlers.onToken?.((payload as { text: string }).text ?? "")
      break
    case "trace":
      handlers.onTrace?.((payload as { trace: TurnTrace }).trace)
      break
    case "done":
      handlers.onDone?.(
        payload as {
          message: ChatMessage
          usage: Usage
          interrupted: boolean
          steps?: number
          stepsExhausted?: boolean
        }
      )
      break
    case "error":
      handlers.onError?.((payload as { message: string }).message ?? "Generation failed")
      break
    default:
      break
  }
}

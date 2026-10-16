import type {
  Chat,
  ChatDetail,
  ChatMessage,
  ModelSelection,
  ProviderInfo,
  Usage,
} from "@/types/chat"

const BASE = "/api"

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

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  })
  if (!res.ok) {
    throw new ApiError(await readError(res), res.status)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  health: () => request<{ status: string; providers: Record<string, boolean> }>("/health"),

  listModels: () =>
    request<{ providers: ProviderInfo[] }>("/models").then((r) => r.providers ?? []),

  listChats: () => request<{ chats: Chat[] }>("/chats").then((r) => r.chats ?? []),

  getChat: (chatId: string) =>
    request<{ chat: ChatDetail }>(`/chats/${chatId}`).then((r) => r.chat),

  createChat: (selection?: ModelSelection | null) =>
    request<{ chat: Chat }>("/chats", {
      method: "POST",
      body: JSON.stringify(selection ?? {}),
    }).then((r) => r.chat),

  deleteChat: (chatId: string) =>
    request<{ deleted: boolean }>(`/chats/${chatId}`, { method: "DELETE" }),
}

export interface StreamHandlers {
  onChat?: (chat: Chat) => void
  onMessage?: (message: ChatMessage) => void
  onReasoning?: (text: string) => void
  onToken?: (text: string) => void
  onDone?: (payload: {
    message: ChatMessage
    usage: Usage
    interrupted: boolean
  }) => void
  onError?: (message: string) => void
}

/**
 * POST a user turn and consume the Server-Sent Events reply stream.
 * Events: chat, message, reasoning*, token*, done|error.
 */
export async function streamMessage(
  chatId: string,
  body: { content: string } & Partial<ModelSelection>,
  handlers: StreamHandlers,
  signal?: AbortSignal
): Promise<void> {
  const res = await fetch(`${BASE}/chats/${chatId}/messages`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream" },
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
    case "done":
      handlers.onDone?.(
        payload as { message: ChatMessage; usage: Usage; interrupted: boolean }
      )
      break
    case "error":
      handlers.onError?.((payload as { message: string }).message ?? "Generation failed")
      break
    default:
      break
  }
}

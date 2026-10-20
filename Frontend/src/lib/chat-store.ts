import { useSyncExternalStore } from "react"

import { api, streamMessage, ApiError } from "@/lib/api"
import { getSelectedModel } from "@/lib/model-store"
import {
  ASK_QUESTIONS_TOOL,
  ASK_TEST_QUESTIONS,
} from "@/lib/questionnaire"
import { notifyError, notifyWarning } from "@/lib/toast"
import type { Chat, ChatMessage, TurnTrace } from "@/types/chat"

/**
 * The test command. It shows the questionnaire without asking a model, so the
 * form can be tried on its own.
 */
function isAskTestCommand(text: string): boolean {
  const trimmed = text.trim().toLowerCase()
  return trimmed === "/ask" || trimmed.startsWith("/ask ")
}

/** Detect if a message is an image generation request */
function isImageGenerationRequest(text: string): boolean {
  const lower = text.toLowerCase().trim()
  return (
    lower.startsWith("/image") ||
    lower.startsWith("/generate") ||
    lower.startsWith("generate image") ||
    lower.startsWith("create image") ||
    lower.startsWith("make an image") ||
    lower.startsWith("draw") ||
    lower.startsWith("paint")
  )
}

/** Extract the prompt from an image generation request */
function extractImagePrompt(text: string): string {
  const lower = text.toLowerCase().trim()
  // Remove common prefixes
  let prompt = text
  const prefixes = [
    "/image",
    "/generate",
    "generate image",
    "create image",
    "make an image",
    "make image",
    "draw",
    "paint",
  ]
  for (const prefix of prefixes) {
    if (lower.startsWith(prefix)) {
      prompt = text.slice(prefix.length).trim()
      // Remove leading "of", "a", "an", "the"
      prompt = prompt.replace(/^(of|a|an|the)\s+/i, "").trim()
      break
    }
  }
  return prompt || "A beautiful scene"
}

interface ChatState {
  chats: Chat[]
  messages: Record<string, ChatMessage[]>
  streaming: Record<string, string>
  reasoning: Record<string, string>
  traces: Record<string, TurnTrace>
  hydrated: boolean
  /** Abort controllers for in-flight streaming requests, keyed by chatId. */
  abortControllers: Record<string, AbortController>
  /**
   * Chats whose pending questionnaire came from the /ask test command. The
   * answers to one of those are kept here rather than sent to a model.
   */
  askTests: Record<string, boolean>
}

let state: ChatState = {
  chats: [],
  messages: {},
  streaming: {},
  reasoning: {},
  traces: {},
  hydrated: false,
  abortControllers: {},
  askTests: {},
}

const listeners = new Set<() => void>()

function setState(patch: Partial<ChatState>) {
  state = { ...state, ...patch }
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function getSnapshot() {
  return state
}

// --- selectors --------------------------------------------------------------

export function useChats() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).chats
}

export function useChat(chatId: string) {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).chats.find(
    (chat) => chat.id === chatId
  )
}

export function useChatMessages(chatId: string) {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).messages[chatId]
}

export function useIsStreaming(chatId: string) {
  return (
    useSyncExternalStore(subscribe, getSnapshot, getSnapshot).streaming[chatId] !==
    undefined
  )
}

/**
 * The reply that is still arriving, shaped like a stored message so the same
 * component renders both. Its id is fixed for the life of the turn, and it is
 * gone the moment the real reply is committed.
 */
export function useStreamingMessage(chatId: string): ChatMessage | undefined {
  const snapshot = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  if (snapshot.streaming[chatId] === undefined) return undefined

  return {
    id: `streaming:${chatId}`,
    chatId,
    role: "assistant",
    content: snapshot.streaming[chatId] ?? "",
    reasoning: snapshot.reasoning[chatId] ?? "",
    trace: snapshot.traces[chatId],
    createdAt: 0,
    pending: true,
  }
}

export function useHydrated() {
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot).hydrated
}

// --- helpers ----------------------------------------------------------------

function errorMessage(err: unknown) {
  return err instanceof Error ? err.message : "Something went wrong."
}

function is401(err: unknown): err is ApiError {
  return err instanceof ApiError && err.status === 401
}

function without<T>(record: Record<string, T>, key: string) {
  const next: Record<string, T> = {}
  for (const k of Object.keys(record)) {
    if (k !== key) next[k] = record[k]
  }
  return next
}

function upsertChat(chats: Chat[], next: Chat): Chat[] {
  // Pinned chats lead the list, then the most recently used ones — the same
  // order the backend returns.
  return [next, ...chats.filter((chat) => chat.id !== next.id)].sort(
    (a, b) => Number(b.pinned) - Number(a.pinned) || b.updatedAt - a.updatedAt
  )
}

function replacePending(list: ChatMessage[], message: ChatMessage): ChatMessage[] {
  const index = list.findIndex((item) => item.pending)
  if (index === -1) return [...list, message]
  const next = [...list]
  next[index] = message
  return next
}

// --- actions ----------------------------------------------------------------

/** Load the sidebar list. Safe to call more than once. */
export async function hydrate() {
  try {
    const chats = await api.listChats()
    setState({ chats, hydrated: true })
  } catch (err) {
    if (is401(err)) throw err
    setState({ hydrated: true })
    notifyError(errorMessage(err), "Could not reach the server", "backend")
  }
}

/** Load one conversation, unless we already hold its messages locally. */
export async function ensureChatLoaded(chatId: string) {
  if (state.messages[chatId]) return

  try {
    const detail = await api.getChat(chatId)
    setState({
      chats: upsertChat(state.chats, detail),
      messages: { ...state.messages, [chatId]: detail.messages ?? [] },
    })
  } catch (err) {
    if (is401(err)) throw err
    setState({ messages: { ...state.messages, [chatId]: [] } })
    notifyError(
      errorMessage(err),
      "Could not open the chat",
      `chat-load-${chatId}`
    )
  }
}

export async function createChat(workspaceId?: string, agentId?: string, mode?: "build" | "plan"): Promise<Chat> {
  try {
    const chat = await api.createChat(getSelectedModel(), workspaceId, agentId, mode)
    setState({
      chats: upsertChat(state.chats, chat),
      messages: { ...state.messages, [chat.id]: [] },
    })
    return chat
  } catch (err) {
    if (is401(err)) throw err
    throw err
  }
}

export async function sendMessage(chatId: string, content: string) {
  const text = content.trim()
  if (!text) return

  // The test command asks for no model at all: the questionnaire is put
  // together here and shown above the composer.
  if (isAskTestCommand(text)) {
    const now = Date.now()
    const asked: ChatMessage = {
      id: `local-${now}`,
      chatId,
      role: "user",
      content: text,
      createdAt: now,
      pending: true,
    }
    const questionnaire: ChatMessage = {
      id: `local-${now}-ask`,
      chatId,
      role: "assistant",
      content: "Before I start, a few things I need to know:",
      trace: {
        steps: [
          {
            number: 1,
            parts: [
              {
                kind: "tool",
                callId: `local-${now}-ask`,
                name: ASK_QUESTIONS_TOOL,
                input: JSON.stringify({ questions: ASK_TEST_QUESTIONS }),
                status: "ok",
              },
            ],
          },
        ],
      },
      createdAt: now,
      pending: true,
    }

    setState({
      messages: {
        ...state.messages,
        [chatId]: [...(state.messages[chatId] ?? []), asked, questionnaire],
      },
      askTests: { ...state.askTests, [chatId]: true },
    })
    return
  }

  // The answers to a test questionnaire are kept here: the test asks no model
  // anything, and neither do the answers to it.
  if (state.askTests[chatId]) {
    const answered: ChatMessage = {
      id: `local-${Date.now()}`,
      chatId,
      role: "user",
      content: text,
      createdAt: Date.now(),
      pending: true,
    }
    setState({
      messages: {
        ...state.messages,
        [chatId]: [...(state.messages[chatId] ?? []), answered],
      },
      askTests: without(state.askTests, chatId),
    })
    return
  }

  // Check if this is an image generation request
  if (isImageGenerationRequest(text)) {
    const imagePrompt = extractImagePrompt(text)
    
    const optimistic: ChatMessage = {
      id: `local-${Date.now()}`,
      chatId,
      role: "user",
      content: text,
      createdAt: Date.now(),
      pending: true,
    }

    // Create a special assistant message that will render the image generation placeholder
    const assistantMessage: ChatMessage = {
      id: `local-${Date.now()}-assistant`,
      chatId,
      role: "assistant",
      content: `__IMAGE_GENERATION__:${imagePrompt}`,
      createdAt: Date.now(),
      pending: true,
    }

    setState({
      messages: {
        ...state.messages,
        [chatId]: [...(state.messages[chatId] ?? []), optimistic, assistantMessage],
      },
      // Don't set streaming for image generation - no inFlight message needed
      reasoning: { ...state.reasoning, [chatId]: "" },
      traces: without(state.traces, chatId),
    })
    return
  }

  const optimistic: ChatMessage = {
    id: `local-${Date.now()}`,
    chatId,
    role: "user",
    content: text,
    createdAt: Date.now(),
    pending: true,
  }

  setState({
    messages: {
      ...state.messages,
      [chatId]: [...(state.messages[chatId] ?? []), optimistic],
    },
    streaming: { ...state.streaming, [chatId]: "" },
    reasoning: { ...state.reasoning, [chatId]: "" },
    traces: without(state.traces, chatId),
  })

  await runStream(chatId, text)
}

/**
 * Drop a reply and ask for a new one from the user turn it answered. Only the
 * last reply is retryable, so the backend can replace it in place.
 */
export async function retryReply(chatId: string, messageId: string) {
  const list = state.messages[chatId] ?? []
  const index = list.findIndex((message) => message.id === messageId)
  if (index === -1) return

  // The user turn this reply was answering.
  const prompt = list
    .slice(0, index)
    .reverse()
    .find((message) => message.role === "user")
  if (!prompt) return

  setState({
    messages: {
      ...state.messages,
      [chatId]: list.filter((message) => message.id !== messageId),
    },
    streaming: { ...state.streaming, [chatId]: "" },
    reasoning: { ...state.reasoning, [chatId]: "" },
    traces: without(state.traces, chatId),
  })

  await runStream(chatId, prompt.content, true)
}

/** Remove one turn from a chat. */
export async function deleteReply(chatId: string, messageId: string) {
  const previous = state.messages[chatId] ?? []
  setState({
    messages: {
      ...state.messages,
      [chatId]: previous.filter((message) => message.id !== messageId),
    },
  })

  try {
    await api.deleteMessage(chatId, messageId)
  } catch (err) {
    setState({ messages: { ...state.messages, [chatId]: previous } })
    notifyError(errorMessage(err), "Could not remove the reply")
  }
}

/** Retitle a chat. */
export async function renameChat(chatId: string, title: string) {
  const next = title.trim()
  if (!next) return

  const previous = state.chats
  const current = previous.find((chat) => chat.id === chatId)
  if (!current || current.title === next) return

  setState({
    chats: upsertChat(previous, { ...current, title: next }),
  })

  try {
    const chat = await api.updateChat(chatId, { title: next })
    setState({ chats: upsertChat(state.chats, chat) })
  } catch (err) {
    setState({ chats: previous })
    notifyError(errorMessage(err), "Could not rename the chat")
  }
}

/**
 * Pin or unpin a chat. The flip shows straight away, and is put back if the
 * server refuses it.
 */
export async function togglePinChat(chatId: string, pinned: boolean) {
  const previous = state.chats
  const current = previous.find((chat) => chat.id === chatId)
  if (!current || current.pinned === pinned) return

  setState({
    chats: upsertChat(previous, { ...current, pinned }),
  })

  try {
    const chat = await api.updateChat(chatId, { pinned })
    setState({ chats: upsertChat(state.chats, chat) })
  } catch (err) {
    if (is401(err)) throw err
    setState({ chats: previous })
    notifyError(errorMessage(err), "Could not pin the chat")
  }
}

/** Update the chat's mode (build/plan). */
export async function updateChatMode(chatId: string, mode: "build" | "plan") {
  const previous = state.chats
  const current = previous.find((chat) => chat.id === chatId)
  if (!current || current.mode === mode) return

  setState({
    chats: upsertChat(previous, { ...current, mode }),
  })

  try {
    const chat = await api.updateChat(chatId, { mode })
    setState({ chats: upsertChat(state.chats, chat) })
  } catch (err) {
    if (is401(err)) throw err
    setState({ chats: previous })
    notifyError(errorMessage(err), "Could not change the mode")
  }
}

/** Remember a chat the server created, so the sidebar shows it. */
export function rememberChat(chat: Chat) {
  setState({ chats: upsertChat(state.chats, chat) })
}

/** The shared streaming path, for a fresh turn and for a regeneration. */
async function runStream(chatId: string, content: string, regenerate = false) {
  const selection = getSelectedModel()

  // Create abort controller for this stream
  const controller = new AbortController()
  setState({ abortControllers: { ...state.abortControllers, [chatId]: controller } })

  try {
    await streamMessage(
      chatId,
      { content, regenerate, ...(selection ?? {}) },
      {
        onChat: (chat) => setState({ chats: upsertChat(state.chats, chat) }),

        // No user message is echoed back when regenerating.
        onMessage: (message) =>
          setState({
            messages: {
              ...state.messages,
              [chatId]: replacePending(state.messages[chatId] ?? [], message),
            },
          }),

        onReasoning: (delta) =>
          setState({
            reasoning: {
              ...state.reasoning,
              [chatId]: (state.reasoning[chatId] ?? "") + delta,
            },
          }),

        onToken: (delta) =>
          setState({
            streaming: {
              ...state.streaming,
              [chatId]: (state.streaming[chatId] ?? "") + delta,
            },
          }),

        onTrace: (trace) =>
          setState({ traces: { ...state.traces, [chatId]: trace } }),

        onDone: ({ message, interrupted, steps = 0, stepsExhausted }) => {
          const notice = interrupted
            ? "The reply was interrupted."
            : stepsExhausted
              ? `The harness stopped after ${steps} steps, so the task may be unfinished.`
              : ""
          setState({
            messages: {
              ...state.messages,
              [chatId]: [...(state.messages[chatId] ?? []), message],
            },
            streaming: without(state.streaming, chatId),
            reasoning: without(state.reasoning, chatId),
            traces: without(state.traces, chatId),
            abortControllers: without(state.abortControllers, chatId),
          })
          if (notice) {
            notifyWarning(notice, "The reply did not finish", `chat-${chatId}`)
          }
        },

        onError: (message) => {
          setState({
            streaming: without(state.streaming, chatId),
            reasoning: without(state.reasoning, chatId),
            traces: without(state.traces, chatId),
            abortControllers: without(state.abortControllers, chatId),
          })
          notifyError(message, "The reply failed", `chat-${chatId}`)
        },
      },
      controller.signal
    )
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") {
      // Stream was aborted by user - state already cleaned up in stopStream()
      return
    }
    setState({
      streaming: without(state.streaming, chatId),
      reasoning: without(state.reasoning, chatId),
      traces: without(state.traces, chatId),
      abortControllers: without(state.abortControllers, chatId),
    })
    notifyError(errorMessage(err), "The reply failed", `chat-${chatId}`)
  }
}

/** Abort the in-flight stream for a chat. */
export function stopStream(chatId: string) {
  const controller = state.abortControllers[chatId]
  if (controller) {
    controller.abort()
    // Immediately clean up all streaming state so UI resets (send button, enabled input, spinner stops)
    setState({
      streaming: without(state.streaming, chatId),
      reasoning: without(state.reasoning, chatId),
      traces: without(state.traces, chatId),
      abortControllers: without(state.abortControllers, chatId),
    })
  }
}

export async function deleteChat(chatId: string) {
  const previous = state.chats
  setState({
    chats: state.chats.filter((chat) => chat.id !== chatId),
    messages: without(state.messages, chatId),
    streaming: without(state.streaming, chatId),
    reasoning: without(state.reasoning, chatId),
    traces: without(state.traces, chatId),
    askTests: without(state.askTests, chatId),
  })

  try {
    await api.deleteChat(chatId)
  } catch (err) {
    setState({ chats: previous })
    notifyError(errorMessage(err), "Could not remove the chat")
  }
}
